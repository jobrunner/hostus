package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// seedFloraVegEntries writes one name_space row plus the three FloraVeg
// spellings of Festuca ovina onto the seeded Corynephorus concept, through a
// real IngestTx — exercising UpsertNameSpace/AddNameSpaceEntry end to end
// rather than poking rows in directly.
//
// The three-spellings shape is the point (see schema.sql's note on
// name_space_entry's primary key): one concept, three ext_ids, two of them
// aggregate-marked and reached through a normalisation rule.
func seedFloraVegEntries(t *testing.T, db *DB) {
	t.Helper()
	const conceptID = corynephorusID

	ctx := context.Background()

	tx, err := db.BeginIngest(ctx, seedBackboneVersion)
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	if err := tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "floraveg", Version: "2023-01-03",
		License: "", SourceURL: "https://example.org/floraveg",
		ManifestSHA: "deadbeef", Redistribution: domain.RedistributionUnknown,
	}); err != nil {
		t.Fatalf("UpsertNameSpace: unexpected error: %v", err)
	}
	entries := []domain.NameSpaceEntry{
		{Space: "floraveg", ExtID: "5647", Name: "Festuca ovina"},
		{Space: "floraveg", ExtID: "5648", Name: "Festuca ovina aggr.", Aggregate: true, Resolution: string(domain.RuleAggregateToNominate)},
		{Space: "floraveg", ExtID: "5649", Name: "Festuca ovina s. l.", Aggregate: true, Resolution: string(domain.RuleAggregateToNominate)},
	}
	for _, e := range entries {
		if err := tx.AddNameSpaceEntry(conceptID, e); err != nil {
			t.Fatalf("AddNameSpaceEntry(%s): unexpected error: %v", e.ExtID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

// TestAddNameSpaceEntry_IndexesAggregateAliasesIntoFTS pins that a RESOLVED
// aggregate name-space spelling is indexed into fts_name as an alias for its
// concept, flagged is_aggregate=1, so suggest can find (and badge) the
// aggregate spelling. The non-aggregate spelling ("Festuca ovina", 5647) is
// NOT indexed — only aggregate-marked entries become aliases.
func TestAddNameSpaceEntry_IndexesAggregateAliasesIntoFTS(t *testing.T) {
	db := openSeededDB(t)
	seedFloraVegEntries(t, db)
	ctx := context.Background()

	// Both aggregate spellings (5648, 5649) are indexed as is_aggregate=1
	// aliases for the concept; the non-aggregate 5647 is not.
	var n int
	if err := db.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM fts_name_map WHERE concept_id = ? AND is_aggregate = 1`,
		corynephorusID).Scan(&n); err != nil {
		t.Fatalf("counting aggregate aliases: %v", err)
	}
	if n != 2 {
		t.Fatalf("is_aggregate=1 alias count = %d, want 2", n)
	}

	// The non-aggregate entry (5647 "Festuca ovina") is NOT indexed: this
	// seed's concept has no own-name FTS row (seed.sql builds none), so any
	// is_aggregate=0 alias for it could only come from a wrongly-indexed
	// non-aggregate name-space entry. There must be none.
	var nonAgg int
	if err := db.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM fts_name_map WHERE concept_id = ? AND is_aggregate = 0`,
		corynephorusID).Scan(&nonAgg); err != nil {
		t.Fatalf("counting non-aggregate aliases: %v", err)
	}
	if nonAgg != 0 {
		t.Errorf("is_aggregate=0 alias count = %d, want 0 (non-aggregate entries must not be indexed)", nonAgg)
	}

	// fts_name is contentless, so verify searchability the only way that
	// works: a MATCH on the aggregate spelling resolves to an is_aggregate=1
	// row for the concept.
	var conceptID string
	var isAgg int
	err := db.sql.QueryRowContext(ctx, `
		SELECT m.concept_id, m.is_aggregate
		FROM fts_name_map m
		WHERE m.rowid IN (SELECT rowid FROM fts_name WHERE fts_name MATCH ?)
		  AND m.is_aggregate = 1`, `"festuca ovina aggr"*`).Scan(&conceptID, &isAgg)
	if err != nil {
		t.Fatalf("MATCH on aggregate spelling found no is_aggregate alias: %v", err)
	}
	if conceptID != corynephorusID || isAgg != 1 {
		t.Errorf("aggregate MATCH resolved to (%q, is_aggregate=%d), want (%q, 1)", conceptID, isAgg, corynephorusID)
	}
}

// TestNameSpaceEntries_RoundTripsEverySpellingOfOneConcept pins the whole
// write→read path, including the two fields the ordinary case makes
// invisible: the verbatim name (never folded to the match key) and the
// aggregate flag.
func TestNameSpaceEntries_RoundTripsEverySpellingOfOneConcept(t *testing.T) {
	db := openSeededDB(t)
	seedFloraVegEntries(t, db)

	got, err := db.NameSpaceEntries(context.Background(), corynephorusID, nil)
	if err != nil {
		t.Fatalf("NameSpaceEntries: unexpected error: %v", err)
	}
	want := []domain.NameSpaceEntry{
		{Space: "floraveg", ExtID: "5647", Name: "Festuca ovina"},
		{Space: "floraveg", ExtID: "5648", Name: "Festuca ovina aggr.", Aggregate: true, Resolution: string(domain.RuleAggregateToNominate)},
		{Space: "floraveg", ExtID: "5649", Name: "Festuca ovina s. l.", Aggregate: true, Resolution: string(domain.RuleAggregateToNominate)},
	}
	if len(got) != len(want) {
		t.Fatalf("NameSpaceEntries = %d entries, want %d (%+v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], w)
		}
	}
}

// TestNameSpaceEntries_ExactMatchStoresNullResolution pins the "absence is
// information" rule at the storage layer: an exact canonical match writes SQL
// NULL, not the empty string, and reads back as "".
func TestNameSpaceEntries_ExactMatchStoresNullResolution(t *testing.T) {
	db := openSeededDB(t)
	seedFloraVegEntries(t, db)

	var isNull bool
	if err := db.sql.QueryRow(
		`SELECT resolution IS NULL FROM name_space_entry WHERE space = 'floraveg' AND ext_id = '5647'`,
	).Scan(&isNull); err != nil {
		t.Fatalf("querying resolution: unexpected error: %v", err)
	}
	if !isNull {
		t.Error("resolution for an exactly-matched entry is not NULL — an empty string would be an assertion, not an absence")
	}
}

// TestNameSpaceEntries_FilterBySpace pins the argument a /v1/match
// target_space will select on, in both directions.
func TestNameSpaceEntries_FilterBySpace(t *testing.T) {
	db := openSeededDB(t)
	seedFloraVegEntries(t, db)
	ctx := context.Background()

	got, err := db.NameSpaceEntries(ctx, corynephorusID, []string{"floraveg"})
	if err != nil {
		t.Fatalf("NameSpaceEntries(floraveg): unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("NameSpaceEntries(floraveg) = %d entries, want 3", len(got))
	}

	none, err := db.NameSpaceEntries(ctx, corynephorusID, []string{"germansl"})
	if err != nil {
		t.Fatalf("NameSpaceEntries(germansl): unexpected error: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("NameSpaceEntries(germansl) = %+v, want empty", none)
	}
}

// TestNameSpaceEntries_UnknownConceptIsNotFound pins that a nonexistent
// concept and a known concept with no entries are never conflated — the same
// contract Traits keeps.
func TestNameSpaceEntries_UnknownConceptIsNotFound(t *testing.T) {
	db := openSeededDB(t)
	ctx := context.Background()

	if _, err := db.NameSpaceEntries(ctx, "c-does-not-exist", nil); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("NameSpaceEntries(unknown) error = %v, want domain.ErrNotFound", err)
	}

	got, err := db.NameSpaceEntries(ctx, corynephorusID, nil)
	if err != nil {
		t.Fatalf("NameSpaceEntries(known, no entries): unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("NameSpaceEntries(known, no entries) = %+v, want empty", got)
	}
}

// TestNameSpaces_ListsProvenance pins that the space's provenance survives
// the round trip — in particular the redistribution value, which is the field
// the bundle gate reads, and the empty license FloraVeg genuinely has.
func TestNameSpaces_ListsProvenance(t *testing.T) {
	db := openSeededDB(t)
	ctx := context.Background()

	empty, err := db.NameSpaces(ctx)
	if err != nil {
		t.Fatalf("NameSpaces (before ingest): unexpected error: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("NameSpaces (before ingest) = %+v, want empty", empty)
	}

	seedFloraVegEntries(t, db)

	got, err := db.NameSpaces(ctx)
	if err != nil {
		t.Fatalf("NameSpaces: unexpected error: %v", err)
	}
	want := domain.NameSpaceMeta{
		ID: "floraveg", Version: "2023-01-03", License: "",
		SourceURL: "https://example.org/floraveg", ManifestSHA: "deadbeef",
		Redistribution: domain.RedistributionUnknown,
	}
	if len(got) != 1 || got[0] != want {
		t.Errorf("NameSpaces = %+v, want exactly [%+v]", got, want)
	}
}

// TestNameSpaceEntries_ClosedDBSurfacesQueryError pins the query/iteration
// error paths, which no happy-path test reaches.
func TestNameSpaceEntries_ClosedDBSurfacesQueryError(t *testing.T) {
	db := openSeededDB(t)
	seedFloraVegEntries(t, db)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: unexpected error: %v", err)
	}

	if _, err := db.NameSpaceEntries(context.Background(), corynephorusID, nil); err == nil {
		t.Error("NameSpaceEntries on a closed database: want an error, got nil")
	}
	if _, err := db.NameSpaces(context.Background()); err == nil {
		t.Error("NameSpaces on a closed database: want an error, got nil")
	}
}

// TestAddNameSpaceEntry_UnknownConceptViolatesForeignKey pins that an entry
// can never dangle: concept_id is an FK onto taxon_concept, which is why
// application.IngestNameSpace resolves every name BEFORE opening its
// transaction.
func TestAddNameSpaceEntry_UnknownConceptViolatesForeignKey(t *testing.T) {
	db := openSeededDB(t)
	ctx := context.Background()

	tx, err := db.BeginIngest(ctx, seedBackboneVersion)
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "floraveg", Version: "2023-01-03", ManifestSHA: "deadbeef",
		Redistribution: domain.RedistributionUnknown,
	}); err != nil {
		t.Fatalf("UpsertNameSpace: unexpected error: %v", err)
	}
	err = tx.AddNameSpaceEntry("c-does-not-exist", domain.NameSpaceEntry{
		Space: "floraveg", ExtID: "1", Name: "Nowhere taxon",
	})
	if err == nil {
		t.Error("AddNameSpaceEntry for an unknown concept: want a foreign-key error, got nil")
	}
}

// TestUpsertNameSpace_UnknownSpaceViolatesForeignKey pins the other end of
// the same guarantee: an entry cannot name a space that was never recorded,
// so the bundle gate can never miss a contributing space.
func TestUpsertNameSpace_UnknownSpaceViolatesForeignKey(t *testing.T) {
	db := openSeededDB(t)
	ctx := context.Background()

	tx, err := db.BeginIngest(ctx, seedBackboneVersion)
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	err = tx.AddNameSpaceEntry(corynephorusID, domain.NameSpaceEntry{
		Space: "never-recorded", ExtID: "1", Name: "Festuca ovina",
	})
	if err == nil {
		t.Error("AddNameSpaceEntry for an unrecorded space: want a foreign-key error, got nil")
	}
}
