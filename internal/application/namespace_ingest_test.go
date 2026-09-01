package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/namelist"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// nameListRowSource adapts a *namelist.Dataset into
// application.NameRowSource — the same boundary-respecting bridge
// traitsRowSource uses, since application never imports
// internal/adapters/namelist directly (depguard).
type nameListRowSource struct{ ds *namelist.Dataset }

func (s nameListRowSource) Rows() []application.NameRow {
	out := make([]application.NameRow, 0, len(s.ds.Rows))
	for _, r := range s.ds.Rows {
		out = append(out, application.NameRow{Taxon: r.Taxon, SourceID: r.SourceID, Status: r.Status})
	}
	return out
}

// sliceRowSource is the minimal NameRowSource for the cases a CSV fixture
// cannot express (a duplicate ext_id, an ambiguous name).
type sliceRowSource []application.NameRow

func (s sliceRowSource) Rows() []application.NameRow { return s }

// eurosBackboneVersion/eurosMeta name the real "eurosl" native space once,
// shared by the Stufe-2 tests below — not repeated per test, so goconst
// (whole-package, both test and non-test files) does not correlate this
// literal with match.go's unrelated "eurosl" occurrences and misreport an
// issue there.
var eurosBackboneVersion = domain.BackboneVersion{ID: "eurosl", Version: "v1"}
var eurosMeta = domain.NameSpaceMeta{ID: "eurosl", Version: "v1"}

var floravegMeta = domain.NameSpaceMeta{
	ID:             "floraveg",
	Version:        "2023-01-03",
	SourceURL:      "https://files.ibot.cas.cz/cevs/downloads/floraveg/Life_form.xlsx",
	ManifestSHA:    "deadbeef",
	Redistribution: domain.RedistributionUnknown,
}

func loadFloraVegFixture(t *testing.T) nameListRowSource {
	t.Helper()
	ds, err := namelist.Read("../adapters/namelist/testdata/floraveg-sample.csv")
	if err != nil {
		t.Fatalf("namelist.Read(floraveg-sample.csv): unexpected error: %v", err)
	}
	return nameListRowSource{ds: ds}
}

// festucaOvinaConceptID is the WCVP sample's accepted Festuca ovina concept —
// the one all three FloraVeg spellings of that name crosswalk onto.
const festucaOvinaConceptID = "wcvp:concept:415853"

// TestIngestNameSpace_RowRoundTrips is the core round-trip: a FloraVeg row
// goes in as a name string and comes back out attached to a WCVP concept,
// with the space's own spelling and ext_id preserved verbatim.
func TestIngestNameSpace_RowRoundTrips(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	if _, err := application.IngestNameSpace(ctx, repo, loadFloraVegFixture(t), floravegMeta); err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}

	entries, err := repo.NameSpaceEntries(ctx, festucaOvinaConceptID, nil)
	if err != nil {
		t.Fatalf("NameSpaceEntries: unexpected error: %v", err)
	}
	// All three FloraVeg spellings of Festuca ovina land on the SAME concept
	// under their own SeqIDs — the source document's own UC4 example, and the
	// reason name_space_entry is keyed by ext_id rather than by concept.
	// Status comes straight from the source list; the fixture marks all three
	// spellings accepted. It is what lets ResolveTargetSpace pick a determinate
	// name when a concept carries several of a space's spellings, as here.
	want := []domain.NameSpaceEntry{
		{Space: "floraveg", ExtID: "5647", Name: "Festuca ovina", Aggregate: false, Status: "accepted", Resolution: ""},
		{Space: "floraveg", ExtID: "5648", Name: "Festuca ovina aggr.", Aggregate: true, Status: "accepted", Resolution: string(domain.RuleAggregateToNominate)},
		{Space: "floraveg", ExtID: "5649", Name: "Festuca ovina s. l.", Aggregate: true, Status: "accepted", Resolution: string(domain.RuleAggregateToNominate)},
	}
	if len(entries) != len(want) {
		t.Fatalf("NameSpaceEntries: got %d entries, want %d (%+v)", len(entries), len(want), entries)
	}
	for i, w := range want {
		if entries[i] != w {
			t.Errorf("entry %d = %+v, want %+v", i, entries[i], w)
		}
	}
}

// TestIngestNameSpace_ReportCountsAndSamplesLoss pins the standing rule: loss
// is counted AND sampled, never silently dropped, and the three sub-counts
// account for every row.
func TestIngestNameSpace_ReportCountsAndSamplesLoss(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	report, err := application.IngestNameSpace(ctx, repo, loadFloraVegFixture(t), floravegMeta)
	if err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}

	if report.Space != "floraveg" {
		t.Errorf("report.Space = %q, want %q", report.Space, "floraveg")
	}
	if got, want := report.Rows, 5; got != want {
		t.Errorf("report.Rows = %d, want %d", got, want)
	}
	if got, want := report.Matched, 3; got != want {
		t.Errorf("report.Matched = %d, want %d", got, want)
	}
	if got, want := report.Unmatched, 2; got != want {
		t.Errorf("report.Unmatched = %d, want %d", got, want)
	}
	if report.Ambiguous != 0 {
		t.Errorf("report.Ambiguous = %d, want 0", report.Ambiguous)
	}
	if sum := report.Matched + report.Unmatched + report.Ambiguous; sum != report.Rows {
		t.Errorf("Matched+Unmatched+Ambiguous = %d, want Rows = %d", sum, report.Rows)
	}
	// Coverage is smaller than Matched: three rows, one concept.
	if got, want := report.Concepts, 1; got != want {
		t.Errorf("report.Concepts = %d, want %d", got, want)
	}
	if got, want := strings.Join(report.UnmatchedSample, ","), "Abies alba,Acer opalus aggr."; got != want {
		t.Errorf("report.UnmatchedSample = %q, want %q", got, want)
	}
	if report.Redistribution != string(domain.RedistributionUnknown) {
		t.Errorf("report.Redistribution = %q, want %q", report.Redistribution, domain.RedistributionUnknown)
	}
}

// TestIngestNameSpace_AggregatesAreCountedSeparately pins the number UC4
// actually needs: WCVP carries no aggregate-marked names, so every aggregate
// that resolves does so through the FLAGGED aggregate-to-nominate rule, and a
// headline match rate that hid that would misrepresent exactly these rows.
func TestIngestNameSpace_AggregatesAreCountedSeparately(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	report, err := application.IngestNameSpace(ctx, repo, loadFloraVegFixture(t), floravegMeta)
	if err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}

	// "Festuca ovina aggr.", "Festuca ovina s. l." and "Acer opalus aggr." —
	// the unmatched one counts too, because the denominator of "how many
	// aggregates resolve" is all of them.
	if got, want := report.Aggregates, 3; got != want {
		t.Errorf("report.Aggregates = %d, want %d", got, want)
	}
	if got, want := report.AggregatesMatched, 2; got != want {
		t.Errorf("report.AggregatesMatched = %d, want %d", got, want)
	}
	if got, want := strings.Join(report.FlaggedSample, ","), "Festuca ovina aggr.,Festuca ovina s. l."; got != want {
		t.Errorf("report.FlaggedSample = %q, want %q", got, want)
	}

	if len(report.Normalized) != 1 {
		t.Fatalf("report.Normalized = %+v, want exactly one rule", report.Normalized)
	}
	rc := report.Normalized[0]
	if rc.Rule != domain.RuleAggregateToNominate || rc.Rows != 2 || rc.Taxa != 2 || !rc.Flagged {
		t.Errorf("report.Normalized[0] = %+v, want aggregate_to_nominate rows=2 taxa=2 flagged", rc)
	}
}

// TestIngestNameSpace_SpaceIsRecordedEvenWithoutMatches pins that a space
// that resolves nothing is still visible as ingested — and still visible to
// the redistribution gate, which is the case that matters.
func TestIngestNameSpace_SpaceIsRecordedEvenWithoutMatches(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	src := sliceRowSource{{Taxon: "Nothing matches this", SourceID: "1"}}
	report, err := application.IngestNameSpace(ctx, repo, src, floravegMeta)
	if err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}
	if report.Matched != 0 || report.Unmatched != 1 {
		t.Errorf("report = %+v, want 0 matched / 1 unmatched", report)
	}

	spaces, err := repo.NameSpaces(ctx)
	if err != nil {
		t.Fatalf("NameSpaces: unexpected error: %v", err)
	}
	if len(spaces) != 1 {
		t.Fatalf("NameSpaces: got %d, want 1", len(spaces))
	}
	want := domain.NameSpaceMeta{
		ID:             "floraveg",
		Version:        "2023-01-03",
		SourceURL:      floravegMeta.SourceURL,
		ManifestSHA:    "deadbeef",
		Redistribution: domain.RedistributionUnknown,
	}
	if spaces[0] != want {
		t.Errorf("NameSpaces[0] = %+v, want %+v", spaces[0], want)
	}
}

// TestIngestNameSpace_EntriesAreFilterableBySpace pins the spaces argument,
// which is what a /v1/match target_space will select on.
func TestIngestNameSpace_EntriesAreFilterableBySpace(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	if _, err := application.IngestNameSpace(ctx, repo, loadFloraVegFixture(t), floravegMeta); err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}

	got, err := repo.NameSpaceEntries(ctx, festucaOvinaConceptID, []string{"floraveg"})
	if err != nil {
		t.Fatalf("NameSpaceEntries(floraveg): unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("NameSpaceEntries(floraveg) = %d entries, want 3", len(got))
	}

	none, err := repo.NameSpaceEntries(ctx, festucaOvinaConceptID, []string{"germansl"})
	if err != nil {
		t.Fatalf("NameSpaceEntries(germansl): unexpected error: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("NameSpaceEntries(germansl) = %+v, want empty", none)
	}
}

// TestIngestNameSpace_UnknownConceptIsNotFound pins that an unknown concept
// and a known concept with no entries are never conflated.
func TestIngestNameSpace_UnknownConceptIsNotFound(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	if _, err := repo.NameSpaceEntries(ctx, "wcvp:concept:does-not-exist", nil); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("NameSpaceEntries(unknown) error = %v, want domain.ErrNotFound", err)
	}
	entries, err := repo.NameSpaceEntries(ctx, festucaOvinaConceptID, nil)
	if err != nil {
		t.Fatalf("NameSpaceEntries(known, no entries): unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("NameSpaceEntries(known, no entries) = %+v, want empty", entries)
	}
}

// TestIngestNameSpace_ReIngestIsIdempotent pins that running the same space
// twice does not duplicate its entries — name_space_entry's (space, ext_id)
// key makes the second run a replace, and the second run must not report the
// replaced rows as duplicates either (the duplicate counter is per-run).
func TestIngestNameSpace_ReIngestIsIdempotent(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	for i := range 2 {
		report, err := application.IngestNameSpace(ctx, repo, loadFloraVegFixture(t), floravegMeta)
		if err != nil {
			t.Fatalf("IngestNameSpace run %d: unexpected error: %v", i, err)
		}
		if report.DuplicateExtIDs != 0 {
			t.Errorf("run %d: report.DuplicateExtIDs = %d, want 0", i, report.DuplicateExtIDs)
		}
	}

	entries, err := repo.NameSpaceEntries(ctx, festucaOvinaConceptID, nil)
	if err != nil {
		t.Fatalf("NameSpaceEntries: unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("NameSpaceEntries after two runs = %d entries, want 3", len(entries))
	}
}

// TestIngestNameSpace_DuplicateExtIDIsCountedNotOverwritten pins that a
// source emitting the same stable id twice loses the second row VISIBLY. An
// INSERT OR REPLACE would otherwise silently repoint the entry, which is
// exactly the silent-loss class this project counts rather than absorbs.
func TestIngestNameSpace_DuplicateExtIDIsCountedNotOverwritten(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	src := sliceRowSource{
		{Taxon: "Festuca ovina", SourceID: "5647"},
		{Taxon: "Festuca duriuscula", SourceID: "5647"},
	}
	report, err := application.IngestNameSpace(ctx, repo, src, floravegMeta)
	if err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}
	if got, want := report.DuplicateExtIDs, 1; got != want {
		t.Errorf("report.DuplicateExtIDs = %d, want %d", got, want)
	}
	if got, want := strings.Join(report.DuplicateSample, ","), "5647"; got != want {
		t.Errorf("report.DuplicateSample = %q, want %q", got, want)
	}

	entries, err := repo.NameSpaceEntries(ctx, festucaOvinaConceptID, nil)
	if err != nil {
		t.Fatalf("NameSpaceEntries: unexpected error: %v", err)
	}
	// The FIRST row keeps the slot; the second is reported, not written.
	if len(entries) != 1 || entries[0].Name != "Festuca ovina" {
		t.Errorf("NameSpaceEntries = %+v, want exactly the first row (Festuca ovina)", entries)
	}
}

// TestIngestNameSpace_AmbiguousNameIsSkippedNotGuessed pins that a name whose
// key answers with two DISTINCT concepts is counted, sampled and skipped —
// never attached to whichever concept came back first.
func TestIngestNameSpace_AmbiguousNameIsSkippedNotGuessed(t *testing.T) {
	repo := &fakeNameSpaceRepo{
		matches: map[string][]output.MatchCandidate{
			"ambiguous name": {
				{Concept: domain.Concept{ID: "c-1"}},
				{Concept: domain.Concept{ID: "c-2"}},
			},
		},
	}
	src := sliceRowSource{{Taxon: "Ambiguous name", SourceID: "1"}}

	report, err := application.IngestNameSpace(context.Background(), repo, src, floravegMeta)
	if err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}
	if got, want := report.Ambiguous, 1; got != want {
		t.Errorf("report.Ambiguous = %d, want %d", got, want)
	}
	if report.Matched != 0 {
		t.Errorf("report.Matched = %d, want 0 — an ambiguous name must never be attached", report.Matched)
	}
	if got, want := strings.Join(report.AmbiguousSample, ","), "Ambiguous name"; got != want {
		t.Errorf("report.AmbiguousSample = %q, want %q", got, want)
	}
	if len(repo.tx.entries) != 0 {
		t.Errorf("wrote %+v, want nothing", repo.tx.entries)
	}
}

// TestIngestNameSpace_NoRepositoryReadWhileTransactionOpen pins the
// project-wide two-phase invariant. The sqlite adapter runs with
// SetMaxOpenConns(1), so a read issued after BeginTraitIngest waits forever
// for a second connection — a real deadlock in "hostus ingest", not a test
// artifact.
func TestIngestNameSpace_NoRepositoryReadWhileTransactionOpen(t *testing.T) {
	repo := &fakeNameSpaceRepo{
		matches: map[string][]output.MatchCandidate{
			"festuca ovina": {{Concept: domain.Concept{ID: "c-1"}}},
		},
	}
	src := sliceRowSource{
		{Taxon: "Festuca ovina", SourceID: "1"},
		{Taxon: "Nothing here", SourceID: "2"},
	}

	if _, err := application.IngestNameSpace(context.Background(), repo, src, floravegMeta); err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}
	if repo.readsAfterBegin != 0 {
		t.Errorf("%d repository read(s) while the ingest transaction was open, want 0", repo.readsAfterBegin)
	}
}

// TestIngestNameSpace_DistinctNamesAreResolvedOnce pins the phase-1 cache: a
// space listing the same name twice must cost one lookup, not two.
func TestIngestNameSpace_DistinctNamesAreResolvedOnce(t *testing.T) {
	repo := &fakeNameSpaceRepo{
		matches: map[string][]output.MatchCandidate{
			"festuca ovina": {{Concept: domain.Concept{ID: "c-1"}}},
		},
	}
	src := sliceRowSource{
		{Taxon: "Festuca ovina", SourceID: "1"},
		{Taxon: "Festuca ovina", SourceID: "2"},
	}

	if _, err := application.IngestNameSpace(context.Background(), repo, src, floravegMeta); err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}
	if got, want := repo.matchCalls, 1; got != want {
		t.Errorf("MatchExact called %d time(s), want %d", got, want)
	}
}

// TestIngestNameSpace_WriteFailuresRollBack pins that no partial name space
// is ever committed: each of the four write steps, when it fails, rolls the
// transaction back and surfaces the error.
func TestIngestNameSpace_WriteFailuresRollBack(t *testing.T) {
	for _, failOn := range []string{"space", "entry", "finalize"} {
		t.Run(failOn, func(t *testing.T) {
			repo := &fakeNameSpaceRepo{
				matches: map[string][]output.MatchCandidate{
					"festuca ovina": {{Concept: domain.Concept{ID: "c-1"}}},
				},
			}
			repo.failOn = failOn
			src := sliceRowSource{{Taxon: "Festuca ovina", SourceID: "1"}}

			if _, err := application.IngestNameSpace(context.Background(), repo, src, floravegMeta); err == nil {
				t.Fatalf("IngestNameSpace: want error when %s fails, got nil", failOn)
			}
			if !repo.tx.rolled {
				t.Error("transaction was not rolled back")
			}
			if repo.tx.committed {
				t.Error("transaction was committed despite the failure")
			}
		})
	}
}

// TestIngestNameSpace_ResolveFailureOpensNoTransaction pins that a
// repository error during phase 1 costs no transaction at all: "write
// nothing" is guaranteed structurally, not by a rollback.
func TestIngestNameSpace_ResolveFailureOpensNoTransaction(t *testing.T) {
	repo := &fakeNameSpaceRepo{matchErr: errors.New("boom")}
	src := sliceRowSource{{Taxon: "Festuca ovina", SourceID: "1"}}

	if _, err := application.IngestNameSpace(context.Background(), repo, src, floravegMeta); err == nil {
		t.Fatal("IngestNameSpace: want error, got nil")
	}
	if repo.txOpened {
		t.Error("an ingest transaction was opened despite a phase-1 failure")
	}
}

// TestIngestNameSpace_NameSpacesFailureIsWrappedAndOpensNoTransaction pins
// nativeSpaceSet's error path (Stufe 2, spec 2026-09-01 B2): a failing
// Repository.NameSpaces call is the FIRST thing resolveNameSpaceNames does,
// before MatchExact ever runs — so it must surface, wrapped, through
// IngestNameSpace, exactly like the phase-1 MatchExact failure above, and
// cost no transaction either.
func TestIngestNameSpace_NameSpacesFailureIsWrappedAndOpensNoTransaction(t *testing.T) {
	repo := &fakeNameSpaceRepo{namespacesErr: errors.New("boom")}
	src := sliceRowSource{{Taxon: "Festuca ovina", SourceID: "1"}}

	_, err := application.IngestNameSpace(context.Background(), repo, src, floravegMeta)
	if err == nil {
		t.Fatal("IngestNameSpace: want error, got nil")
	}
	// IngestNameSpace wraps resolveNameSpaceNames' own error
	// ("application: resolving names for name space %q: %w"), which in turn
	// wraps nativeSpaceSet's ("application: loading name-space set: %w") —
	// both wrap layers must be present, not just "an error happened".
	if !strings.Contains(err.Error(), "resolving names for name space") {
		t.Errorf("err = %q, want it to contain %q", err.Error(), "resolving names for name space")
	}
	if !strings.Contains(err.Error(), "loading name-space set") {
		t.Errorf("err = %q, want it to contain %q", err.Error(), "loading name-space set")
	}
	if !errors.Is(err, repo.namespacesErr) {
		t.Errorf("err = %v, want it to wrap %v", err, repo.namespacesErr)
	}
	if repo.txOpened {
		t.Error("an ingest transaction was opened despite a NameSpaces failure")
	}
	if repo.matchCalls != 0 {
		t.Errorf("matchCalls = %d, want 0 — NameSpaces must fail before MatchExact ever runs", repo.matchCalls)
	}
}

// TestIngestNameSpace_BeginFailureIsSurfaced pins the remaining error path.
func TestIngestNameSpace_BeginFailureIsSurfaced(t *testing.T) {
	repo := &fakeNameSpaceRepo{beginErr: errors.New("boom")}
	src := sliceRowSource{{Taxon: "Festuca ovina", SourceID: "1"}}

	report, err := application.IngestNameSpace(context.Background(), repo, src, floravegMeta)
	if err == nil {
		t.Fatal("IngestNameSpace: want error, got nil")
	}
	// The report still identifies the space, so a caller logging it on the
	// error path is not left with an anonymous failure.
	if report.Space != "floraveg" {
		t.Errorf("report.Space = %q, want %q", report.Space, "floraveg")
	}
}

// --- fakes -----------------------------------------------------------------

type nameSpaceEntryWrite struct {
	conceptID string
	entry     domain.NameSpaceEntry
}

// classificationWrite records one UpsertClassification call.
type classificationWrite struct {
	conceptID                    string
	family, orderName, className string
}

// vernacularWrite records one AddVernacularName call.
type vernacularWrite struct {
	conceptID string
	name      domain.VernacularName
}

// fakeNameSpaceTx records what IngestNameSpace writes and can fail on demand.
type fakeNameSpaceTx struct {
	failOn          string
	spaces          []domain.NameSpaceMeta
	entries         []nameSpaceEntryWrite
	classifications []classificationWrite
	vernaculars     []vernacularWrite
	committed       bool
	rolled          bool
}

func (t *fakeNameSpaceTx) UpsertNameSpace(meta domain.NameSpaceMeta) error {
	if t.failOn == "space" {
		return errors.New("boom")
	}
	t.spaces = append(t.spaces, meta)
	return nil
}

func (t *fakeNameSpaceTx) AddNameSpaceEntry(conceptID string, e domain.NameSpaceEntry) error {
	if t.failOn == "entry" {
		return errors.New("boom")
	}
	t.entries = append(t.entries, nameSpaceEntryWrite{conceptID: conceptID, entry: e})
	return nil
}

func (t *fakeNameSpaceTx) UpsertClassification(conceptID string, family, orderName, className string) error {
	if t.failOn == "classification" {
		return errors.New("boom")
	}
	t.classifications = append(t.classifications, classificationWrite{conceptID: conceptID, family: family, orderName: orderName, className: className})
	return nil
}

func (t *fakeNameSpaceTx) AddVernacularName(conceptID string, v domain.VernacularName) error {
	if t.failOn == "vernacular" {
		return errors.New("boom")
	}
	t.vernaculars = append(t.vernaculars, vernacularWrite{conceptID: conceptID, name: v})
	return nil
}

func (t *fakeNameSpaceTx) Finalize() error {
	if t.failOn == "finalize" {
		return errors.New("boom")
	}
	return nil
}

func (t *fakeNameSpaceTx) Commit() error   { t.committed = true; return nil }
func (t *fakeNameSpaceTx) Rollback() error { t.rolled = true; return nil }

func (t *fakeNameSpaceTx) UpsertName(domain.Name) error                      { return nil }
func (t *fakeNameSpaceTx) UpsertConcept(domain.Concept) error                { return nil }
func (t *fakeNameSpaceTx) LinkName(string, string, string, *bool) error      { return nil }
func (t *fakeNameSpaceTx) AddXref(string, domain.Xref, string) error         { return nil }
func (t *fakeNameSpaceTx) AddDistribution(string, domain.Distribution) error { return nil }
func (t *fakeNameSpaceTx) UpsertArea(domain.Area) error                      { return nil }
func (t *fakeNameSpaceTx) UpsertSecReference(domain.SecReference) error      { return nil }
func (t *fakeNameSpaceTx) UpsertXrefSource(domain.XrefSourceMeta) error      { return nil }
func (t *fakeNameSpaceTx) AddConceptRelation(string, string, domain.Relation, string) error {
	return nil
}
func (t *fakeNameSpaceTx) AddAggregateMember(string, string) error { return nil }
func (t *fakeNameSpaceTx) ResolveNameSpaceMember(string, string) (string, error) {
	return "", nil
}

// fakeNameSpaceRepo answers MatchExact from a canned map and counts both how
// many lookups happened and how many of them happened while the ingest
// transaction was open (which must stay zero — see the two-phase test).
type fakeNameSpaceRepo struct {
	tx       fakeNameSpaceTx
	matches  map[string][]output.MatchCandidate
	matchErr error
	beginErr error
	// namespacesErr, when set, is what NameSpaces returns instead of its
	// default nil,nil — the fake's only way to exercise nativeSpaceSet's
	// error path (resolveNameSpaceNames wraps it before any repository read
	// or transaction is attempted).
	namespacesErr   error
	failOn          string
	matchCalls      int
	readsAfterBegin int
	txOpen          bool
	txOpened        bool
}

func (r *fakeNameSpaceRepo) MatchExact(_ context.Context, canon string) ([]output.MatchCandidate, error) {
	if r.txOpen {
		r.readsAfterBegin++
	}
	if r.matchErr != nil {
		return nil, r.matchErr
	}
	r.matchCalls++
	return r.matches[canon], nil
}

func (r *fakeNameSpaceRepo) BeginTraitIngest(context.Context) (output.IngestTx, error) {
	if r.beginErr != nil {
		return nil, r.beginErr
	}
	r.tx.failOn = r.failOn
	r.txOpen = true
	r.txOpened = true
	return &r.tx, nil
}

func (r *fakeNameSpaceRepo) BeginIngest(context.Context, domain.BackboneVersion) (output.IngestTx, error) {
	return &r.tx, nil
}

func (r *fakeNameSpaceRepo) Concept(context.Context, string) (*domain.Concept, []output.SynonymName, []domain.Xref, []domain.Distribution, error) {
	return nil, nil, nil, nil, nil
}
func (r *fakeNameSpaceRepo) SynonymCandidates(context.Context, string) ([]domain.SynonymCandidate, error) {
	return nil, nil
}
func (r *fakeNameSpaceRepo) Classification(context.Context, string) ([]domain.ClassificationEntry, error) {
	return nil, nil
}
func (r *fakeNameSpaceRepo) ConceptByXref(context.Context, string, string) (*domain.Concept, error) {
	return nil, nil
}
func (r *fakeNameSpaceRepo) ConceptIDsByXref(context.Context, string, []string) (map[string]string, error) {
	return nil, nil
}
func (r *fakeNameSpaceRepo) ExistingConceptIDs(context.Context, []string) (map[string]bool, error) {
	return nil, nil
}
func (r *fakeNameSpaceRepo) Areas(context.Context) ([]domain.Area, error) { return nil, nil }

func (r *fakeNameSpaceRepo) SecReferences(context.Context) ([]domain.SecReference, error) {
	return nil, nil
}
func (r *fakeNameSpaceRepo) SecReferenceByID(context.Context, string) (domain.SecReference, error) {
	return domain.SecReference{}, nil
}
func (r *fakeNameSpaceRepo) ConceptRelationsInSec(context.Context, string, string) (output.ConceptRelations, error) {
	return output.ConceptRelations{}, nil
}
func (r *fakeNameSpaceRepo) MatchFuzzyCandidates(context.Context, string, int, string, string) ([]output.MatchCandidate, error) {
	return nil, nil
}
func (r *fakeNameSpaceRepo) BackboneVersions(context.Context) ([]domain.BackboneVersion, error) {
	return nil, nil
}
func (r *fakeNameSpaceRepo) BuildDistributionClosure(context.Context) error {
	return nil
}
func (r *fakeNameSpaceRepo) NameSpaceEntries(context.Context, string, []string) ([]domain.NameSpaceEntry, error) {
	return nil, nil
}
func (r *fakeNameSpaceRepo) AggregateMembers(context.Context, string) ([]string, error) {
	return nil, nil
}
func (r *fakeNameSpaceRepo) AggregatesByMember(context.Context, string) ([]string, error) {
	return nil, nil
}
func (r *fakeNameSpaceRepo) VernacularNames(context.Context, string) ([]domain.VernacularName, error) {
	return nil, nil
}
func (r *fakeNameSpaceRepo) AggregateConcepts(context.Context, string, []domain.Rank) ([]output.AggregateConceptSummary, error) {
	return nil, nil
}
func (r *fakeNameSpaceRepo) WriteConceptAgreement(context.Context, []domain.ConceptAgreementPair) error {
	return nil
}
func (r *fakeNameSpaceRepo) ConceptAgreement(context.Context, string) (*domain.ConceptAgreementPair, error) {
	return nil, nil
}
func (r *fakeNameSpaceRepo) NameSpaces(context.Context) ([]domain.NameSpaceMeta, error) {
	if r.namespacesErr != nil {
		return nil, r.namespacesErr
	}
	return nil, nil
}
func (r *fakeNameSpaceRepo) Suggest(context.Context, string, output.SuggestOpts) ([]domain.SuggestItem, error) {
	return nil, nil
}

// TestIngestNameSpace_CarriesTheSourceStatus pins the wiring whose absence made
// every target-space name arbitrary: the source list states which of its
// spellings is accepted, and that statement has to survive the reader -> DTO ->
// entry path. It was dropped at the DTO boundary, so a concept holding several
// of a space's names had no way to say which one to report.
func TestIngestNameSpace_CarriesTheSourceStatus(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	if _, err := application.IngestNameSpace(ctx, repo, loadFloraVegFixture(t), floravegMeta); err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}

	entries, err := repo.NameSpaceEntries(ctx, festucaOvinaConceptID, []string{"floraveg"})
	if err != nil {
		t.Fatalf("NameSpaceEntries: unexpected error: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no floraveg entries stored for the Festuca ovina concept")
	}
	for _, e := range entries {
		if e.Status != "accepted" {
			t.Errorf("entry %s carries status %q, want %q from the source list", e.ExtID, e.Status, "accepted")
		}
		if !e.AcceptedInSpace() {
			t.Errorf("entry %s does not report as accepted, so it could not win a target-space tie", e.ExtID)
		}
	}
}

// TestIngestNameSpace_WritesClassificationOntoMatchedConcept pins Task 4:
// once a row resolves, its Family/OrderName/ClassName (already walked up the
// source's own parent chain by the caller — see internal/app/ingest.go's
// classificationFor) land on taxon_concept via UpsertClassification, and are
// readable back through repo.Concept(). Deviates from the brief's literal
// example (which seeds its own "Salsola kali" concept via an unspecified
// seededNamespaceRepo helper): reusing seededMatchRepo's real WCVP fixture
// and its already-known festucaOvinaConceptID gets the same coverage without
// inventing a second concept-seeding helper for one test.
func TestIngestNameSpace_WritesClassificationOntoMatchedConcept(t *testing.T) {
	repo := seededMatchRepo(t)
	ctx := context.Background()

	src := sliceRowSource{{
		Taxon: "Festuca ovina", SourceID: "1408c0e8", Status: "accepted",
		Family: "Poaceae", OrderName: "Poales", ClassName: "Liliopsida",
	}}

	report, err := application.IngestNameSpace(ctx, repo, src, floravegMeta)
	if err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}
	if report.Matched != 1 {
		t.Fatalf("report.Matched = %d, want 1", report.Matched)
	}

	var concept *domain.Concept
	if c, _, _, _, err := repo.Concept(ctx, festucaOvinaConceptID); err != nil {
		t.Fatalf("Concept: unexpected error: %v", err)
	} else {
		concept = c
	}
	if concept.Family != "Poaceae" {
		t.Errorf("concept.Family = %q, want %q", concept.Family, "Poaceae")
	}
	if concept.OrderName != "Poales" {
		t.Errorf("concept.OrderName = %q, want %q", concept.OrderName, "Poales")
	}
	if concept.ClassName != "Liliopsida" {
		t.Errorf("concept.ClassName = %q, want %q", concept.ClassName, "Liliopsida")
	}
}

// TestIngestNameSpace_WritesVernacularNameOntoMatchedConcept pins the
// VernacularDE half of Task 4: a matched row's German common name reaches
// tx.AddVernacularName exactly once, tagged "de", for the resolved
// concept — asserted against the fake tx (application has no read-back
// query for `vernacular`; the sqlite adapter's own round-trip is pinned
// separately in internal/adapters/sqlite/namespace_test.go).
func TestIngestNameSpace_WritesVernacularNameOntoMatchedConcept(t *testing.T) {
	repo := &fakeNameSpaceRepo{
		matches: map[string][]output.MatchCandidate{
			"festuca ovina": {{Concept: domain.Concept{ID: "c-1"}}},
		},
	}
	src := sliceRowSource{{Taxon: "Festuca ovina", SourceID: "1", VernacularDE: "Schaf-Schwingel"}}

	if _, err := application.IngestNameSpace(context.Background(), repo, src, floravegMeta); err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}
	if len(repo.tx.vernaculars) != 1 {
		t.Fatalf("vernaculars = %+v, want exactly one write", repo.tx.vernaculars)
	}
	got := repo.tx.vernaculars[0]
	if got.conceptID != "c-1" || got.name != (domain.VernacularName{Language: "de", Name: "Schaf-Schwingel"}) {
		t.Errorf("vernaculars[0] = %+v, want conceptID=c-1 name={de Schaf-Schwingel}", got)
	}
}

// TestIngestNameSpace_HomonymStaysAmbiguousHere is a boundary on a change made
// elsewhere. The trait crosswalk resolves a homonym to its genuine bearer
// (accepted, then homotypic) instead of dropping it — measured, and worth it
// there. Name-space ingest shares the same resolver, so it would have inherited
// that silently: no counter in NameSpaceIngestReport, no line in the CLI
// report, and no measurement of what it does to the entries
// domain.ResolveTargetSpace picks a target-space SPELLING from (a space could
// gain a second entry for one concept, and /v1/translate would then have to
// choose between "Inula hirta" and "Pentanema hirtum").
//
// So this path keeps refusing, deliberately, until someone measures it. The
// test exists to make that a decision rather than an oversight.
func TestIngestNameSpace_HomonymStaysAmbiguousHere(t *testing.T) {
	repo := &fakeNameSpaceRepo{
		matches: map[string][]output.MatchCandidate{
			"homonymus testicus": {
				{Concept: domain.Concept{ID: "c-bearer"}, Role: "accepted"},
				{Concept: domain.Concept{ID: "c-other"}, Role: "synonym"},
			},
		},
	}
	src := sliceRowSource{{Taxon: "Homonymus testicus", SourceID: "1"}}

	report, err := application.IngestNameSpace(context.Background(), repo, src, floravegMeta)
	if err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}
	if report.Ambiguous != 1 || report.Matched != 0 {
		t.Errorf("Ambiguous/Matched = %d/%d, want 1/0: the tie-break is the trait crosswalk's, not this path's",
			report.Ambiguous, report.Matched)
	}
	if len(repo.tx.entries) != 0 {
		t.Errorf("wrote %+v, want nothing", repo.tx.entries)
	}
}

// TestIngestNameSpace_SecReferenceCandidateDoesNotCauseAmbiguous pins the
// policyPreferBackbone fix: a sec.-reference-space concept (e.g. one of
// CDM's Standardliste sec. spaces) sharing a name with a backbone (WCVP)
// concept must NOT count toward "this name is ambiguous" — the sec.
// candidate is dropped and the backbone concept wins outright, with no
// tie-break involved (contrast with TestIngestNameSpace_HomonymStaysAmbiguousHere
// just above, whose two candidates are BOTH backbone concepts and must stay
// ambiguous).
func TestIngestNameSpace_SecReferenceCandidateDoesNotCauseAmbiguous(t *testing.T) {
	repo := &fakeNameSpaceRepo{
		matches: map[string][]output.MatchCandidate{
			"festuca ovina": {
				{Concept: domain.Concept{ID: "wcvp:concept:415853"}, Role: "accepted"},
				{Concept: domain.Concept{ID: "cdm:concept:x", SecReference: "cdm-sec-1"}, Role: "accepted"},
			},
		},
	}
	src := sliceRowSource{{Taxon: "Festuca ovina", SourceID: "1"}}

	report, err := application.IngestNameSpace(context.Background(), repo, src, floravegMeta)
	if err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}
	if report.Matched != 1 || report.Ambiguous != 0 {
		t.Errorf("Matched/Ambiguous = %d/%d, want 1/0: the sec.-reference candidate must be dropped, not counted",
			report.Matched, report.Ambiguous)
	}
	if len(repo.tx.entries) != 1 || repo.tx.entries[0].conceptID != "wcvp:concept:415853" {
		t.Errorf("wrote %+v, want a single entry attached to wcvp:concept:415853", repo.tx.entries)
	}
}

// TestIngestNameSpace_NativeConceptDoesNotShadowBackboneGenus pinnt den
// Fall-B-Befund des Audits (2026-09-01, Spec B2): eurosl legt native
// GENUS-Konzepte auch für Gattungen an, die WCVP führt ("Abies", "Acer",
// 2866 gemessene Folds). Ein danach gecrosswalkter Name-Space (germansl)
// muss die Gattung trotzdem auf das WCVP-Konzept auflösen — gemessen
// verlor germansl ~544 Gattungs-Einträge (417 vs. 961 auf identischer
// Liste), rein reihenfolgeabhängig.
func TestIngestNameSpace_NativeConceptDoesNotShadowBackboneGenus(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()

	// 1. WCVP-artiges Backbone mit der Gattung.
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		{TaxonID: "g1", AcceptedTaxonID: "g1", Accepted: true, Canonical: "Abies", Rank: "GENUS", Status: "Accepted"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) {
		return fakeRowSource{taxa: taxa}, nil
	}
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// 2. eurosl Fall B: natives GENUS-Konzept gleichen Namens.
	native := staticNativeRows{
		{Taxon: "Abies", SourceID: "e1", Rank: "Genus", Status: "accepted"},
	}
	if _, err := application.IngestNativeSpace(ctx, repo, native, eurosBackboneVersion, domain.RankRoot, noMemberLinks); err != nil {
		t.Fatalf("IngestNativeSpace: %v", err)
	}
	// eurosl muss auch als name_space registriert sein, damit nativeSpaceSet
	// es kennt — im echten Ingest passiert das durch eurosls eigenen
	// Fall-A-Lauf (IngestNameSpace -> UpsertNameSpace).
	if _, err := application.IngestNameSpace(ctx, repo, sliceRowSource{}, eurosMeta); err != nil {
		t.Fatalf("IngestNameSpace(eurosl): %v", err)
	}

	// 3. germansl Fall A: "Abies" muss aufs WCVP-Konzept auflösen, nicht
	//    ambiguous sein.
	report, err := application.IngestNameSpace(ctx, repo,
		sliceRowSource{{Taxon: "Abies", SourceID: "g-1", Status: "accepted"}},
		domain.NameSpaceMeta{ID: "germansl", Version: "v1"})
	if err != nil {
		t.Fatalf("IngestNameSpace(germansl): %v", err)
	}
	if report.Ambiguous != 0 || report.Matched != 1 {
		t.Fatalf("report = matched %d / ambiguous %d, want 1/0", report.Matched, report.Ambiguous)
	}
	entries, err := repo.NameSpaceEntries(ctx, "wcvp:concept:g1", []string{"germansl"})
	if err != nil || len(entries) != 1 {
		t.Fatalf("NameSpaceEntries(wcvp:concept:g1) = %v, %v — der Eintrag muss am WCVP-Konzept hängen", entries, err)
	}
}

// TestIngestNameSpace_NativeOnlyNameStillResolves pinnt die
// Fallback-Invariante: eine Gattung, die NUR als natives Konzept existiert
// (Moos-Gattung "Abietinella" — WCVP führt keine Moose), muss weiterhin auf
// dieses native Konzept auflösen; Stufe 2 darf sie nicht verwerfen.
func TestIngestNameSpace_NativeOnlyNameStillResolves(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	native := staticNativeRows{
		{Taxon: "Abietinella", SourceID: "e2", Rank: "Genus", Status: "accepted"},
	}
	if _, err := application.IngestNativeSpace(ctx, repo, native, eurosBackboneVersion, domain.RankRoot, noMemberLinks); err != nil {
		t.Fatalf("IngestNativeSpace: %v", err)
	}
	if _, err := application.IngestNameSpace(ctx, repo, sliceRowSource{}, eurosMeta); err != nil {
		t.Fatalf("IngestNameSpace(eurosl): %v", err)
	}
	report, err := application.IngestNameSpace(ctx, repo,
		sliceRowSource{{Taxon: "Abietinella", SourceID: "g-2", Status: "accepted"}},
		domain.NameSpaceMeta{ID: "germansl", Version: "v1"})
	if err != nil {
		t.Fatalf("IngestNameSpace(germansl): %v", err)
	}
	if report.Matched != 1 {
		t.Fatalf("report.Matched = %d, want 1 — native-only Namen müssen weiter auflösen", report.Matched)
	}
}
