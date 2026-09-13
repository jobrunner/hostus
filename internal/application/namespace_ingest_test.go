package application_test

import (
	"context"
	"errors"
	"reflect"
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

// TestIngestNameSpace_AcceptedBearerHomonymNowTieBreaksHere replaces the
// former TestIngestNameSpace_HomonymStaysAmbiguousHere. That test's own
// fixture — one candidate holding the name as ACCEPTED (c-bearer), the other
// only as a synonym (c-other) — is exactly the accepted+synonym class spec
// 2026-09-04 decided to rescue: resolveNameSpaceNames now runs under
// policyResolveAcceptedBearer, so this name resolves to c-bearer via the
// tier-1 tie-break instead of staying ambiguous. This is the SPEC-INTENDED
// behavior change (decision 1: tier 1 activated, tier 2 stays out), not a
// silent inheritance — see acceptedBearerWinner's and
// policyResolveAcceptedBearer's doc comments for the measured evidence.
func TestIngestNameSpace_AcceptedBearerHomonymNowTieBreaksHere(t *testing.T) {
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
	if report.Ambiguous != 0 || report.Matched != 1 {
		t.Errorf("Ambiguous/Matched = %d/%d, want 0/1: tier 1 (accepted bearer) resolves this now",
			report.Ambiguous, report.Matched)
	}
	if report.TieBroken != 1 {
		t.Errorf("TieBroken = %d, want 1", report.TieBroken)
	}
	if len(repo.tx.entries) != 1 || repo.tx.entries[0].conceptID != "c-bearer" {
		t.Fatalf("entries = %+v, want exactly one entry on c-bearer", repo.tx.entries)
	}
	if repo.tx.entries[0].entry.Resolution != "accepted_bearer_tiebreak" {
		t.Errorf("Resolution = %q, want accepted_bearer_tiebreak", repo.tx.entries[0].entry.Resolution)
	}
}

// TestIngestNameSpace_SecReferenceCandidateDoesNotCauseAmbiguous pins the
// policyPreferBackbone fix: a sec.-reference-space concept (e.g. one of
// CDM's Standardliste sec. spaces) sharing a name with a backbone (WCVP)
// concept must NOT count toward "this name is ambiguous" — the sec.
// candidate is dropped and the backbone concept wins outright, with no
// tie-break involved (contrast with
// TestIngestNameSpace_AcceptedBearerHomonymNowTieBreaksHere just above,
// whose two candidates are BOTH backbone concepts and now resolve via the
// tier-1 accepted-bearer tie-break instead).
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

// TestIngestNameSpace_AcceptedBearerTieBreakResolvesHomonym pins the spec's
// core decision (2026-09-04): a spelling held as ACCEPTED by exactly one
// concept and as a mere synonym by others resolves to the accepted bearer —
// the Abies-alba class (4716 eurosl folds measured 2026-09-01). The outcome
// must be fully auditable: report counter, sample, and resolution marker.
func TestIngestNameSpace_AcceptedBearerTieBreakResolvesHomonym(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		// Concept A: bears "Abies alba" as its ACCEPTED name.
		{TaxonID: "a1", AcceptedTaxonID: "a1", Accepted: true, Canonical: "Abies alba", Rank: "SPECIES", Status: "Accepted"},
		// Concept B: a different accepted taxon...
		{TaxonID: "b1", AcceptedTaxonID: "b1", Accepted: true, Canonical: "Picea otherica", Rank: "SPECIES", Status: "Accepted"},
		// ...that holds "Abies alba" (the later homonym) only as a SYNONYM.
		{TaxonID: "s1", AcceptedTaxonID: "b1", Accepted: false, Canonical: "Abies alba", Rank: "SPECIES", Status: "Illegitimate"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) { return fakeRowSource{taxa: taxa}, nil }
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	report, err := application.IngestNameSpace(ctx, repo,
		sliceRowSource{{Taxon: "Abies alba", SourceID: "e1", Status: "accepted"}},
		domain.NameSpaceMeta{ID: "eurosl", Version: "v1"})
	if err != nil {
		t.Fatalf("IngestNameSpace: %v", err)
	}
	if report.Matched != 1 || report.Ambiguous != 0 {
		t.Fatalf("matched/ambiguous = %d/%d, want 1/0", report.Matched, report.Ambiguous)
	}
	if report.TieBroken != 1 {
		t.Errorf("TieBroken = %d, want 1", report.TieBroken)
	}
	if len(report.TieBrokenSample) != 1 || report.TieBrokenSample[0] != "Abies alba" {
		t.Errorf("TieBrokenSample = %v, want [Abies alba]", report.TieBrokenSample)
	}
	entries, err := repo.NameSpaceEntries(ctx, "wcvp:concept:a1", []string{"eurosl"})
	if err != nil || len(entries) != 1 {
		t.Fatalf("NameSpaceEntries(a1) = %v, %v — der Eintrag muss am accepted-Träger hängen", entries, err)
	}
	if entries[0].Resolution != "accepted_bearer_tiebreak" {
		t.Errorf("Resolution = %q, want accepted_bearer_tiebreak", entries[0].Resolution)
	}
}

// TestIngestNameSpace_SynonymOnlyHomonymStaysAmbiguous pins the tie-break's
// lower boundary: a spelling NO candidate bears as accepted stays ambiguous
// (4580 measured eurosl folds are this genuinely undecidable class).
func TestIngestNameSpace_SynonymOnlyHomonymStaysAmbiguous(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		// Concept A: accepted under a DIFFERENT name...
		{TaxonID: "a1", AcceptedTaxonID: "a1", Accepted: true, Canonical: "Alpha genuina", Rank: "SPECIES", Status: "Accepted"},
		// ...carrying "Shared synonymum" only as a synonym.
		{TaxonID: "as1", AcceptedTaxonID: "a1", Accepted: false, Canonical: "Shared synonymum", Rank: "SPECIES", Status: "Synonym"},
		// Concept B: accepted under yet another different name...
		{TaxonID: "b1", AcceptedTaxonID: "b1", Accepted: true, Canonical: "Beta genuina", Rank: "SPECIES", Status: "Accepted"},
		// ...also carrying "Shared synonymum" only as a synonym.
		{TaxonID: "bs1", AcceptedTaxonID: "b1", Accepted: false, Canonical: "Shared synonymum", Rank: "SPECIES", Status: "Synonym"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) { return fakeRowSource{taxa: taxa}, nil }
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	report, err := application.IngestNameSpace(ctx, repo,
		sliceRowSource{{Taxon: "Shared synonymum", SourceID: "e1", Status: "synonym"}},
		domain.NameSpaceMeta{ID: "eurosl", Version: "v1"})
	if err != nil {
		t.Fatalf("IngestNameSpace: %v", err)
	}
	if report.Ambiguous != 1 || report.Matched != 0 {
		t.Fatalf("matched/ambiguous = %d/%d, want 0/1", report.Matched, report.Ambiguous)
	}
	if report.TieBroken != 0 {
		t.Errorf("TieBroken = %d, want 0", report.TieBroken)
	}
	entries, err := repo.NameSpaceEntries(ctx, "wcvp:concept:a1", []string{"eurosl"})
	if err != nil || len(entries) != 0 {
		t.Fatalf("NameSpaceEntries(a1) = %v, %v — kein Eintrag erwartet", entries, err)
	}
}

// TestIngestNameSpace_TwoAcceptedBearersStayAmbiguous pins the upper
// boundary: several accepted bearers must NOT be rescued (no tier 2 in the
// crosswalk — spec decision 1) and stay ambiguous.
func TestIngestNameSpace_TwoAcceptedBearersStayAmbiguous(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		// Both concepts hold "Duplex nomen" as their own ACCEPTED name — a
		// genuine, undecidable ambiguity with two real claimants.
		{TaxonID: "a1", AcceptedTaxonID: "a1", Accepted: true, Canonical: "Duplex nomen", Rank: "SPECIES", Status: "Accepted"},
		{TaxonID: "b1", AcceptedTaxonID: "b1", Accepted: true, Canonical: "Duplex nomen", Rank: "SPECIES", Status: "Accepted"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) { return fakeRowSource{taxa: taxa}, nil }
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	report, err := application.IngestNameSpace(ctx, repo,
		sliceRowSource{{Taxon: "Duplex nomen", SourceID: "e1", Status: "accepted"}},
		domain.NameSpaceMeta{ID: "eurosl", Version: "v1"})
	if err != nil {
		t.Fatalf("IngestNameSpace: %v", err)
	}
	if report.Ambiguous != 1 || report.Matched != 0 {
		t.Fatalf("matched/ambiguous = %d/%d, want 0/1", report.Matched, report.Ambiguous)
	}
	if report.TieBroken != 0 {
		t.Errorf("TieBroken = %d, want 0", report.TieBroken)
	}
}

// TestIngestNameSpace_TieBrokenAcceptedSpellingWinsTargetSpaceChoice pins a
// consequence of the tier-1 tie-break (spec 2026-09-04, "Risiko &
// Rückholbarkeit"): a concept that already carries a name-space entry from a
// PLAIN match — the source's own status "synonym", so
// NameSpaceEntry.AcceptedInSpace() is false — can gain a SECOND entry via
// the accepted-bearer tie-break whose source status IS "accepted". Since
// domain.ResolveTargetSpace/pickSpelling prefers AcceptedInSpace() and never
// looks at Resolution, that tie-broken entry now wins the target-space
// spelling domain.ResolveTargetSpace hands to /v1/translate, /v1/match
// (target_space) and /v1/suggest — changing a spelling choice that used to
// be arbitrary-but-stable into one the accepted-bearer tie-break can move.
// This is the source's own accepted spelling winning, which is intended
// (see the spec's risk section) — this test is what makes that outcome an
// asserted decision rather than an unpinned side effect.
func TestIngestNameSpace_TieBrokenAcceptedSpellingWinsTargetSpaceChoice(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		// Concept A: accepted under "Abies alba"...
		{TaxonID: "a1", AcceptedTaxonID: "a1", Accepted: true, Canonical: "Abies alba", Rank: "SPECIES", Status: "Accepted"},
		// ...and also carrying an ordinary synonym, "Abies pectinata".
		{TaxonID: "as1", AcceptedTaxonID: "a1", Accepted: false, Canonical: "Abies pectinata", Rank: "SPECIES", Status: "Synonym"},
		// Concept B: a different accepted taxon...
		{TaxonID: "b1", AcceptedTaxonID: "b1", Accepted: true, Canonical: "Picea otherica", Rank: "SPECIES", Status: "Accepted"},
		// ...that holds "Abies alba" (the later homonym) only as a SYNONYM —
		// the tie-break class from TestIngestNameSpace_AcceptedBearerTieBreakResolvesHomonym.
		{TaxonID: "s1", AcceptedTaxonID: "b1", Accepted: false, Canonical: "Abies alba", Rank: "SPECIES", Status: "Illegitimate"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) { return fakeRowSource{taxa: taxa}, nil }
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// Row 1: an ORDINARY single-candidate match, source status "synonym" —
	// lands on concept A directly, no tie-break involved.
	// Row 2: the homonym "Abies alba", source status "accepted" — resolves
	// to concept A via the tier-1 tie-break, giving concept A a SECOND
	// eurosl entry whose Status IS "accepted".
	report, err := application.IngestNameSpace(ctx, repo,
		sliceRowSource{
			{Taxon: "Abies pectinata", SourceID: "e1", Status: "synonym"},
			{Taxon: "Abies alba", SourceID: "e2", Status: "accepted"},
		},
		domain.NameSpaceMeta{ID: "eurosl", Version: "v1"})
	if err != nil {
		t.Fatalf("IngestNameSpace: %v", err)
	}
	if report.Matched != 2 || report.TieBroken != 1 {
		t.Fatalf("Matched/TieBroken = %d/%d, want 2/1", report.Matched, report.TieBroken)
	}

	entries, err := repo.NameSpaceEntries(ctx, "wcvp:concept:a1", []string{"eurosl"})
	if err != nil || len(entries) != 2 {
		t.Fatalf("NameSpaceEntries(a1) = %v, %v — want both entries on the tie-broken bearer", entries, err)
	}

	choice, policy := domain.ResolveTargetSpace(false, entries)
	if choice.Name != "Abies alba" {
		t.Errorf("ResolveTargetSpace name = %q, want %q (the tie-broken, source-accepted spelling)", choice.Name, "Abies alba")
	}
	if policy != "" {
		t.Errorf("ResolveTargetSpace policy = %q, want empty (plain species)", policy)
	}
}

// assertClosedEntry finds entries' member named wantName and asserts it
// carries the "source_synonymy_closure" resolution marker and wantStatus,
// failing the test otherwise. Split out of
// TestIngestNameSpace_SourceSynonymyClosesUnattachedAccepted to keep that
// test's cyclomatic complexity within the linter's bound. wantResolution is
// not a parameter (every call site wants the same marker — an unparam-flagged
// parameter would just restate it).
func assertClosedEntry(t *testing.T, entries []domain.NameSpaceEntry, wantName, wantStatus string) {
	t.Helper()
	const wantResolution = "source_synonymy_closure"
	for i := range entries {
		if entries[i].Name != wantName {
			continue
		}
		if entries[i].Resolution != wantResolution {
			t.Errorf("Resolution = %q, want %q", entries[i].Resolution, wantResolution)
		}
		if entries[i].Status != wantStatus {
			t.Errorf("Status = %q, want %q (the row's own source status, untouched by closure)", entries[i].Status, wantStatus)
		}
		return
	}
	t.Fatalf("entries = %+v, want one named %q", entries, wantName)
}

// TestIngestNameSpace_SourceSynonymyClosesUnattachedAccepted pins spec
// 2026-09-13 decision 3 (the Inula-hirta class, 3719 of 47989 concepts on
// the real index): a source row the name crosswalk cannot place (its
// spelling has no accepted bearer in WCVP) is attached to the ONE concept
// its own source-synonymy group already resolved to.
//
// Fixture (WCVP): accepted "Pentanema hirtum" (ph1) additionally holds
// "Inula hirta" only as a synonym; a second, unrelated concept ("Beta
// genuina", b1) ALSO holds "Inula hirta" only as a synonym — the same
// two-synonym-bearer construction TestIngestNameSpace_SynonymOnlyHomonymStaysAmbiguous
// uses, so "Inula hirta" resolves to Ambiguous by name alone, with no
// accepted bearer to tie-break to.
//
// Name space rows mirror the real eurosl-canonical convention (measured:
// 53.643/53.643 accepted rows carry an EMPTY accepted_taxon — the group key
// for an accepted row is its own name):
//
//	{Taxon:"Pentanema hirtum", AcceptedTaxon:"Inula hirta", Status:"synonymobjective", SourceID:"e-syn"}
//	{Taxon:"Inula hirta",      AcceptedTaxon:"",            Status:"accepted",         SourceID:"e-acc"}
//
// Both rows share the group key Canonicalize("Inula hirta"). "Pentanema
// hirtum" resolves normally (its own accepted name, unambiguous) to ph1;
// "Inula hirta" stays Ambiguous by itself. The group's resolved members
// point to exactly ONE concept (ph1), so the Ambiguous row is closed there.
func TestIngestNameSpace_SourceSynonymyClosesUnattachedAccepted(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		{TaxonID: "ph1", AcceptedTaxonID: "ph1", Accepted: true, Canonical: "Pentanema hirtum", Rank: "SPECIES", Status: "Accepted"},
		// ph1 also holds "Inula hirta", but only as a synonym.
		{TaxonID: "phsyn", AcceptedTaxonID: "ph1", Accepted: false, Canonical: "Inula hirta", Rank: "SPECIES", Status: "Synonym"},
		{TaxonID: "b1", AcceptedTaxonID: "b1", Accepted: true, Canonical: "Beta genuina", Rank: "SPECIES", Status: "Accepted"},
		// b1 ALSO holds "Inula hirta" only as a synonym — no accepted bearer
		// exists for the spelling, so it stays genuinely ambiguous by name.
		{TaxonID: "bsyn", AcceptedTaxonID: "b1", Accepted: false, Canonical: "Inula hirta", Rank: "SPECIES", Status: "Synonym"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) { return fakeRowSource{taxa: taxa}, nil }
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	report, err := application.IngestNameSpace(ctx, repo,
		sliceRowSource{
			{Taxon: "Pentanema hirtum", SourceID: "e-syn", Status: "synonymobjective", AcceptedTaxon: "Inula hirta"},
			{Taxon: "Inula hirta", SourceID: "e-acc", Status: "accepted", AcceptedTaxon: ""},
		},
		domain.NameSpaceMeta{ID: "eurosl", Version: "v1"})
	if err != nil {
		t.Fatalf("IngestNameSpace: %v", err)
	}
	if report.Matched != 2 || report.Ambiguous != 0 || report.Unmatched != 0 {
		t.Fatalf("matched/ambiguous/unmatched = %d/%d/%d, want 2/0/0", report.Matched, report.Ambiguous, report.Unmatched)
	}
	if report.SynonymyClosed != 1 {
		t.Errorf("SynonymyClosed = %d, want 1", report.SynonymyClosed)
	}
	if len(report.SynonymyClosedSample) != 1 || report.SynonymyClosedSample[0] != "Inula hirta" {
		t.Errorf("SynonymyClosedSample = %v, want [Inula hirta]", report.SynonymyClosedSample)
	}

	entries, err := repo.NameSpaceEntries(ctx, "wcvp:concept:ph1", []string{"eurosl"})
	if err != nil || len(entries) != 2 {
		t.Fatalf("NameSpaceEntries(ph1) = %v, %v — beide Zeilen müssen an ph1 attached sein", entries, err)
	}
	assertClosedEntry(t, entries, "Inula hirta", "accepted")

	// Habitatus-Gewinn: with the accepted eurosl entry now present,
	// ResolveTargetSpace picks "Inula hirta" (Status accepted), not the
	// merely-synonym "Pentanema hirtum" entry.
	choice, policy := domain.ResolveTargetSpace(false, entries)
	if choice.Name != "Inula hirta" {
		t.Errorf("ResolveTargetSpace name = %q, want %q (the closed, source-accepted spelling)", choice.Name, "Inula hirta")
	}
	if choice.Status != "accepted" {
		t.Errorf("ResolveTargetSpace status = %q, want %q", choice.Status, "accepted")
	}
	if policy != "" {
		t.Errorf("ResolveTargetSpace policy = %q, want empty (plain species)", policy)
	}
}

// TestIngestNameSpace_SynonymyClosureRefusesAmbiguousGroups pins the
// closure pass's refusal to guess: a group whose already-resolved members
// point to TWO distinct concepts offers no single answer, so an
// unresolved member of the SAME group stays open (kein Raten).
func TestIngestNameSpace_SynonymyClosureRefusesAmbiguousGroups(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		{TaxonID: "a1", AcceptedTaxonID: "a1", Accepted: true, Canonical: "Concept Alpha", Rank: "SPECIES", Status: "Accepted"},
		{TaxonID: "b1", AcceptedTaxonID: "b1", Accepted: true, Canonical: "Concept Beta", Rank: "SPECIES", Status: "Accepted"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) { return fakeRowSource{taxa: taxa}, nil }
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	report, err := application.IngestNameSpace(ctx, repo,
		sliceRowSource{
			{Taxon: "Concept Alpha", SourceID: "e1", Status: "synonym", AcceptedTaxon: "Group Z"},
			{Taxon: "Concept Beta", SourceID: "e2", Status: "synonym", AcceptedTaxon: "Group Z"},
			{Taxon: "Mystery Name", SourceID: "e3", Status: "accepted", AcceptedTaxon: "Group Z"},
		},
		domain.NameSpaceMeta{ID: "eurosl", Version: "v1"})
	if err != nil {
		t.Fatalf("IngestNameSpace: %v", err)
	}
	if report.SynonymyClosed != 0 {
		t.Errorf("SynonymyClosed = %d, want 0 (group resolves to two distinct concepts)", report.SynonymyClosed)
	}
	if report.Unmatched != 1 || report.Matched != 2 {
		t.Fatalf("matched/unmatched = %d/%d, want 2/1", report.Matched, report.Unmatched)
	}
	if len(report.UnmatchedSample) != 1 || report.UnmatchedSample[0] != "Mystery Name" {
		t.Errorf("UnmatchedSample = %v, want [Mystery Name]", report.UnmatchedSample)
	}
}

// TestIngestNameSpace_SynonymyClosureNeverOverridesResolved pins that the
// closure pass never touches a row the ordinary crosswalk already resolved,
// even when the rest of its source-synonymy group points elsewhere: two
// rows in the SAME group each resolve, by name alone, to their OWN distinct
// concept — the group therefore offers no single target — and each keeps
// exactly the concept it resolved to.
func TestIngestNameSpace_SynonymyClosureNeverOverridesResolved(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		{TaxonID: "a1", AcceptedTaxonID: "a1", Accepted: true, Canonical: "Concept Gamma", Rank: "SPECIES", Status: "Accepted"},
		{TaxonID: "b1", AcceptedTaxonID: "b1", Accepted: true, Canonical: "Concept Delta", Rank: "SPECIES", Status: "Accepted"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) { return fakeRowSource{taxa: taxa}, nil }
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	report, err := application.IngestNameSpace(ctx, repo,
		sliceRowSource{
			{Taxon: "Concept Gamma", SourceID: "e1", Status: "accepted", AcceptedTaxon: "Group W"},
			{Taxon: "Concept Delta", SourceID: "e2", Status: "synonym", AcceptedTaxon: "Group W"},
		},
		domain.NameSpaceMeta{ID: "eurosl", Version: "v1"})
	if err != nil {
		t.Fatalf("IngestNameSpace: %v", err)
	}
	if report.SynonymyClosed != 0 {
		t.Errorf("SynonymyClosed = %d, want 0 (neither row was ever open)", report.SynonymyClosed)
	}
	if report.Matched != 2 {
		t.Fatalf("Matched = %d, want 2", report.Matched)
	}

	gammaEntries, err := repo.NameSpaceEntries(ctx, "wcvp:concept:a1", []string{"eurosl"})
	if err != nil || len(gammaEntries) != 1 || gammaEntries[0].Name != "Concept Gamma" {
		t.Fatalf("NameSpaceEntries(a1) = %v, %v — want its OWN row, untouched", gammaEntries, err)
	}
	deltaEntries, err := repo.NameSpaceEntries(ctx, "wcvp:concept:b1", []string{"eurosl"})
	if err != nil || len(deltaEntries) != 1 || deltaEntries[0].Name != "Concept Delta" {
		t.Fatalf("NameSpaceEntries(b1) = %v, %v — want its OWN row, untouched", deltaEntries, err)
	}
}

// TestIngestNameSpace_SynonymyClosureClosesAllOpenMembersOfOneGroup pins
// the real eurosl scenario a single-member fixture cannot show: one group
// can carry MORE than one open (per-name unresolvable) row at once — an
// accepted-status row plus another synonym spelling, both unmatched by
// name — and BOTH close to the SAME concept in the SAME run once the
// group's one other member resolves it. Neither member of the pair is
// second-guessed against the other; each is independently marked
// synonymyClosed via the same singleTargetConcept lookup.
func TestIngestNameSpace_SynonymyClosureClosesAllOpenMembersOfOneGroup(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		// Concept M: the group's ONE resolved member's accepted name.
		{TaxonID: "m1", AcceptedTaxonID: "m1", Accepted: true, Canonical: "Alpha resolved", Rank: "SPECIES", Status: "Accepted"},
		// Deliberately NO taxon at all for "Beta unresolved"/"Gamma
		// unresolved" — both stay Unmatched by name alone.
	}
	readerFor := func(application.Backbone) (application.RowSource, error) { return fakeRowSource{taxa: taxa}, nil }
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	report, err := application.IngestNameSpace(ctx, repo,
		sliceRowSource{
			{Taxon: "Alpha resolved", SourceID: "e1", Status: "synonym", AcceptedTaxon: "Group M"},
			{Taxon: "Beta unresolved", SourceID: "e2", Status: "accepted", AcceptedTaxon: "Group M"},
			{Taxon: "Gamma unresolved", SourceID: "e3", Status: "synonym", AcceptedTaxon: "Group M"},
		},
		domain.NameSpaceMeta{ID: "eurosl", Version: "v1"})
	if err != nil {
		t.Fatalf("IngestNameSpace: %v", err)
	}
	if report.Matched != 3 || report.Unmatched != 0 || report.Ambiguous != 0 {
		t.Fatalf("matched/unmatched/ambiguous = %d/%d/%d, want 3/0/0", report.Matched, report.Unmatched, report.Ambiguous)
	}
	if report.SynonymyClosed != 2 {
		t.Fatalf("SynonymyClosed = %d, want 2 (both open members of the group)", report.SynonymyClosed)
	}
	wantSample := []string{"Beta unresolved", "Gamma unresolved"}
	if !reflect.DeepEqual(report.SynonymyClosedSample, wantSample) {
		t.Errorf("SynonymyClosedSample = %v, want %v", report.SynonymyClosedSample, wantSample)
	}

	entries, err := repo.NameSpaceEntries(ctx, "wcvp:concept:m1", []string{"eurosl"})
	if err != nil || len(entries) != 3 {
		t.Fatalf("NameSpaceEntries(m1) = %v, %v — want all three rows attached to the ONE resolved concept", entries, err)
	}
	assertClosedEntry(t, entries, "Beta unresolved", "accepted")
	assertClosedEntry(t, entries, "Gamma unresolved", "synonym")
}

// TestIngestNameSpace_SynonymyClosureRefusesSynonymOnlyAnchor pins the
// accepted-role anchor guard (spec 2026-09-13, fix round 2): a group whose
// only resolved member reached its concept through a SYNONYM-role name must
// NOT close the group, even though that concept is the group's single
// candidate.
//
// This mirrors a real full-ingest finding (run 2026-09-13): the germansl
// bryophyte group "Syntrichia sinensis" was closed entirely onto
// wcvp:concept:34724 (Caryopteris incana var. incana, a FLOWERING PLANT)
// because its only anchor, "Barbula sinensis", matched WCVP's cross-kingdom
// homonym SYNONYM name "Barbula sinensis" — Barbula being both a moss genus
// (absent from WCVP entirely) and a Lamiaceae synonym genus. Fixture below
// reconstructs that shape: concept x1 holds "Barbula sinensis" only as a
// synonym (never as its accepted name), so the anchor's matchedAccepted is
// false and the group must stay open.
//
// The underlying crosswalk defect this exposes — "Barbula sinensis" itself
// matching a WCVP homonym in the ORDINARY, non-closure resolve — is
// deliberately OUT OF SCOPE here; this test only pins that the closure pass
// does not amplify it onto a whole group.
func TestIngestNameSpace_SynonymyClosureRefusesSynonymOnlyAnchor(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		{TaxonID: "x1", AcceptedTaxonID: "x1", Accepted: true, Canonical: "Concept X (flowering plant)", Rank: "SPECIES", Status: "Accepted"},
		// x1 holds "Barbula sinensis" only as a SYNONYM — never as its
		// accepted name. This is the cross-kingdom homonym collision.
		{TaxonID: "xsyn", AcceptedTaxonID: "x1", Accepted: false, Canonical: "Barbula sinensis", Rank: "SPECIES", Status: "Synonym"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) { return fakeRowSource{taxa: taxa}, nil }
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	report, err := application.IngestNameSpace(ctx, repo,
		sliceRowSource{
			// Resolves (single candidate, synonym role) to x1 — but NOT via
			// an accepted-role name.
			{Taxon: "Barbula sinensis", SourceID: "g1", Status: "synonym", AcceptedTaxon: "Syntrichia sinensis"},
			// No WCVP taxon at all carries this spelling — stays Unmatched,
			// and must NOT be rescued by x1 despite the group having only
			// one resolved member.
			{Taxon: "Syntrichia sinensis", SourceID: "g2", Status: "accepted", AcceptedTaxon: ""},
		},
		domain.NameSpaceMeta{ID: "germansl", Version: "v1"})
	if err != nil {
		t.Fatalf("IngestNameSpace: %v", err)
	}
	if report.SynonymyClosed != 0 {
		t.Errorf("SynonymyClosed = %d, want 0 (the group's only anchor matched a synonym-role name, not accepted)", report.SynonymyClosed)
	}
	if report.Matched != 1 || report.Unmatched != 1 {
		t.Fatalf("matched/unmatched = %d/%d, want 1/1", report.Matched, report.Unmatched)
	}
	if len(report.UnmatchedSample) != 1 || report.UnmatchedSample[0] != "Syntrichia sinensis" {
		t.Errorf("UnmatchedSample = %v, want [Syntrichia sinensis]", report.UnmatchedSample)
	}

	entries, err := repo.NameSpaceEntries(ctx, "wcvp:concept:x1", []string{"germansl"})
	if err != nil || len(entries) != 1 || entries[0].Name != "Barbula sinensis" {
		t.Fatalf("NameSpaceEntries(x1) = %v, %v — want only the anchor's own row, \"Syntrichia sinensis\" must stay unattached", entries, err)
	}
}

// TestIngestNameSpace_SynonymyClosureAcceptsTieBrokenAnchor pins the other
// side of the accepted-role guard: a tie-broken anchor qualifies as a
// closure target — acceptedBearerWinner's winner IS the accepted bearer by
// construction (spec 2026-09-04), so tieBroken == true always implies
// matchedAccepted == true.
func TestIngestNameSpace_SynonymyClosureAcceptsTieBrokenAnchor(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		// Concept A: bears "Nomen typicum" as its ACCEPTED name.
		{TaxonID: "a1", AcceptedTaxonID: "a1", Accepted: true, Canonical: "Nomen typicum", Rank: "SPECIES", Status: "Accepted"},
		// Concept B: a different accepted taxon...
		{TaxonID: "b1", AcceptedTaxonID: "b1", Accepted: true, Canonical: "Beta genuina", Rank: "SPECIES", Status: "Accepted"},
		// ...that holds "Nomen typicum" only as a SYNONYM — the homonym tie
		// acceptedBearerWinner resolves to concept A.
		{TaxonID: "bsyn", AcceptedTaxonID: "b1", Accepted: false, Canonical: "Nomen typicum", Rank: "SPECIES", Status: "Illegitimate"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) { return fakeRowSource{taxa: taxa}, nil }
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	report, err := application.IngestNameSpace(ctx, repo,
		sliceRowSource{
			// Tie-broken anchor: resolves to concept A via acceptedBearerWinner.
			{Taxon: "Nomen typicum", SourceID: "e1", Status: "accepted", AcceptedTaxon: "Group T"},
			// No WCVP taxon at all — stays Unmatched by itself, but the
			// group's one resolved (tie-broken) member points to A alone.
			{Taxon: "Unresolved Companion", SourceID: "e2", Status: "synonym", AcceptedTaxon: "Group T"},
		},
		domain.NameSpaceMeta{ID: "eurosl", Version: "v1"})
	if err != nil {
		t.Fatalf("IngestNameSpace: %v", err)
	}
	if report.TieBroken != 1 {
		t.Errorf("TieBroken = %d, want 1", report.TieBroken)
	}
	if report.SynonymyClosed != 1 {
		t.Fatalf("SynonymyClosed = %d, want 1 (the tie-broken anchor qualifies as an accepted-role anchor)", report.SynonymyClosed)
	}
	if len(report.SynonymyClosedSample) != 1 || report.SynonymyClosedSample[0] != "Unresolved Companion" {
		t.Errorf("SynonymyClosedSample = %v, want [Unresolved Companion]", report.SynonymyClosedSample)
	}

	entries, err := repo.NameSpaceEntries(ctx, "wcvp:concept:a1", []string{"eurosl"})
	if err != nil || len(entries) != 2 {
		t.Fatalf("NameSpaceEntries(a1) = %v, %v — both rows must be attached to the tie-broken bearer", entries, err)
	}
	assertClosedEntry(t, entries, "Unresolved Companion", "synonym")
}
