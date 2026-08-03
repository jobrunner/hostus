-- hostus 2.0 local taxonomy index schema (spec §4.3). Applied verbatim,
-- server-side and in the offline bundle. Every statement is
-- IF NOT EXISTS so Open() can apply it idempotently against an
-- already-initialized database.

PRAGMA foreign_keys = ON;

-- Artifact/version metadata.
CREATE TABLE IF NOT EXISTS backbone_version (
  id             TEXT PRIMARY KEY,   -- e.g. "wcvp"
  version        TEXT NOT NULL,      -- "2026-06-15" (period/version, never "latest")
  license        TEXT,
  source_url     TEXT,
  ingested_at    TEXT NOT NULL,
  manifest_sha   TEXT NOT NULL,      -- checksum of the validated manifest
  redistribution TEXT NOT NULL DEFAULT 'unknown' -- allowed|restricted|unknown (domain.Redistribution); gates ExportBundle, never local ingest
);

-- Nomenclature.
--
-- canonical_fold stores domain.Canonicalize(canonical): lower-cased,
-- whitespace-collapsed, diacritic-folded per the same table the FTS5
-- unicode61 tokenizer uses (see fts_parity_internal_test.go). It is a
-- plain stored column, not a SQLite GENERATED column, because the fold
-- logic needs domain.Canonicalize's Go-side diacritic table (see
-- read.go/db.go); every writer of this table (IngestTx.UpsertName) must
-- populate it. Exact-match lookups (MatchExact) key on this column
-- instead of canonical directly, because SQLite's own LOWER() only folds
-- ASCII case and would silently miss diacritic-bearing names.
CREATE TABLE IF NOT EXISTS name (
  id             TEXT PRIMARY KEY,
  canonical      TEXT NOT NULL,
  canonical_fold TEXT NOT NULL DEFAULT '',
  authorship     TEXT,
  rank           TEXT NOT NULL,      -- FAMILY|GENUS|...|SUBFORM|NOTHOSUBSPECIES|NOTHOVARIETY|NOTHOFORM|OTHER (domain.Rank)
  ipni_id        TEXT,
  published_in   TEXT,
  nom_status     TEXT,               -- NULL|nom_nud|nom_superfl|pro_syn|...
  basionym_id    TEXT REFERENCES name(id),
  -- rank_verbatim carries the original source "taxonrank" spelling
  -- (e.g. WCVP's "proles") when rank = 'OTHER' — the one case where the
  -- canonical Rank value alone has thrown that spelling away by
  -- collapsing it into the catch-all. NULL/empty for every other rank,
  -- since Rank itself already identifies the canonical spelling exactly
  -- there. See domain.Name.RankVerbatim / domain.ParseRankLenient.
  rank_verbatim  TEXT
);

CREATE INDEX IF NOT EXISTS idx_name_canonical_fold ON name(canonical_fold);
CREATE INDEX IF NOT EXISTS idx_name_basionym_id ON name(basionym_id);

-- Hardening Task 2 (2026-08-01): FK child-column indexes.
--
-- Every REFERENCES column below was, until this task, unindexed except
-- idx_name_canonical_fold (which isn't even on an FK). With
-- `INSERT OR REPLACE` (an implicit DELETE+INSERT) and
-- `PRAGMA foreign_keys=ON`, SQLite must find every row in every
-- referencing table that points at the deleted parent key before it can
-- allow the delete — without an index on the child column, that's a full
-- table scan per insert, and the scanned table grows with every row
-- ingested: quadratic total cost. Measured on the real WCVP backbone
-- (docs/research/reality-check.md, "nach Hardening" has the post-fix
-- numbers): 50k/100k/200k taxa took 65 s / 293 s / 1.338 s without these
-- indexes (×4.5 per doubling — quadratic) vs. 5 s / 11 s / 25 s with them
-- (×2.2 per doubling — linear). The full WCVP ingest was manually killed
-- after 22 min 48 s without a single committed row on the unindexed
-- schema; with these indexes it completes in 276.70 s.
--
-- The honest cost side: an index is extra B-tree pages written on every
-- insert and extra bytes on disk — the reality-check measurement put
-- that at ~18% larger DB files (23.9→28.4 MB at 50k rows, 97.7→114.9 MB
-- at 200k rows). That cost is real but is dwarfed by the FK-scan cost it
-- removes (a ~50× wall-clock win at 200k rows), so it is not a close
-- call.
--
-- Not every REFERENCES column needs its own index: a column that is
-- already the LEADING column of that table's PRIMARY KEY already has a
-- SQLite-maintained B-tree keyed on it, so a second single-column index
-- would be pure write/space cost with no read benefit. Skipped for that
-- reason: concept_name.concept_id (PK is (concept_id, name_id)),
-- distribution.concept_id (PK is (concept_id, area_scheme, area_code)),
-- vernacular.concept_id (PK is (concept_id, lang, name)),
-- trait_value.concept_id (PK is (concept_id, vocab, vocab_version, dim)),
-- and concept_relation.from_concept (PK is
-- (from_concept, to_concept, relation, source) since SP5 widened it — see
-- the note on that table below; from_concept is still the LEADING column,
-- so the conclusion is unchanged). Every other FK child column gets
-- an explicit index below, next to the table it lives on.

-- Taxonomy.
CREATE TABLE IF NOT EXISTS taxon_concept (
  id             TEXT PRIMARY KEY,
  backbone_id    TEXT NOT NULL REFERENCES backbone_version(id),
  accepted_name  TEXT NOT NULL REFERENCES name(id),
  rank           TEXT NOT NULL,
  parent_id      TEXT REFERENCES taxon_concept(id),
  sec_reference  TEXT,              -- bibliographic sec. identity
  status         TEXT NOT NULL,     -- accepted|...
  -- rank_verbatim mirrors name.rank_verbatim's rule, for the concept's own
  -- rank (which is always the same value as its accepted name's rank, but
  -- copied independently here since Concept and Name are separate rows/
  -- structs — see domain.Concept.RankVerbatim).
  rank_verbatim  TEXT
);

CREATE INDEX IF NOT EXISTS idx_taxon_concept_backbone_id ON taxon_concept(backbone_id);
CREATE INDEX IF NOT EXISTS idx_taxon_concept_accepted_name ON taxon_concept(accepted_name);
CREATE INDEX IF NOT EXISTS idx_taxon_concept_parent_id ON taxon_concept(parent_id);

-- Concept <-> name (accepted + synonyms, typed).
CREATE TABLE IF NOT EXISTS concept_name (
  concept_id   TEXT NOT NULL REFERENCES taxon_concept(id),
  name_id      TEXT NOT NULL REFERENCES name(id),
  role         TEXT NOT NULL,       -- accepted|synonym
  homotypic    INTEGER,             -- 1=homotypic (basionym/recombination), 0=heterotypic
  PRIMARY KEY (concept_id, name_id)
);

-- concept_id is already the PK's leading column (skipped, see comment
-- above); name_id is not, so the ingest's FK check on name deletes would
-- scan concept_name in full without this.
CREATE INDEX IF NOT EXISTS idx_concept_name_name_id ON concept_name(name_id);

-- Cross-reference-source provenance metadata: one row per ingested xref
-- source (e.g. the Wikidata bridge-hub harvest), the xref counterpart of
-- backbone_version/trait_vocabulary. It is what lets an ingested database
-- answer "which harvest are these xrefs from?" (version + manifest_sha)
-- and what ExportBundle's redistribution gate joins against.
CREATE TABLE IF NOT EXISTS xref_source (
  id             TEXT PRIMARY KEY,   -- e.g. "wikidata-bridge"
  version        TEXT NOT NULL,      -- harvest date/version, never "latest"
  license        TEXT,
  source_url     TEXT,
  ingested_at    TEXT NOT NULL,
  manifest_sha   TEXT NOT NULL,      -- checksum of the validated manifest
  redistribution TEXT NOT NULL DEFAULT 'unknown' -- allowed|restricted|unknown (domain.Redistribution); gates ExportBundle, never local ingest
);

-- Cross-references to external authorities.
--
-- source is the xref_source this row was harvested from, and is NULL for
-- xrefs the backbone ingest itself derives from a taxon row (e.g. the powo
-- ids WCVP carries): those are already covered by the backbone's own
-- redistribution value via taxon_concept.backbone_id, so attributing them
-- to a synthetic xref source would only double-count the same gate.
--
-- Attribution is last-writer-wins: source records one origin, but AddXref
-- is INSERT OR REPLACE. Two xref sources emitting the same (authority,
-- ext_id) for the same concept is deliberately not a conflict, so the row
-- keeps whichever source ingested last. Ingesting an 'allowed' source after
-- a 'restricted' one therefore clears the bundle gate for the rows they
-- share. Unreachable today (wikidata is the only xref source); the fix if a
-- second one lands is a xref_source_link(concept_id, authority, ext_id,
-- source) join table rather than a single column.
CREATE TABLE IF NOT EXISTS xref (
  concept_id   TEXT NOT NULL REFERENCES taxon_concept(id),
  authority    TEXT NOT NULL,       -- powo|colxr|euromed|gbif|wikidata|wfo|inat|floraveg
  ext_id       TEXT NOT NULL,
  source       TEXT REFERENCES xref_source(id),
  PRIMARY KEY (authority, ext_id)
);

-- concept_id is NOT the PK's leading column here (PK is
-- (authority, ext_id)), so it needs its own index.
CREATE INDEX IF NOT EXISTS idx_xref_concept_id ON xref(concept_id);

-- Vernacular names (German per Buttler et al. 2018 or similar).
CREATE TABLE IF NOT EXISTS vernacular (
  concept_id   TEXT NOT NULL REFERENCES taxon_concept(id),
  lang         TEXT NOT NULL,       -- de|en|...
  name         TEXT NOT NULL,
  preferred    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (concept_id, lang, name)
);

-- Distribution (reference-area ranking).
CREATE TABLE IF NOT EXISTS distribution (
  concept_id   TEXT NOT NULL REFERENCES taxon_concept(id),
  area_scheme  TEXT NOT NULL,       -- wgsrpd_l3|euromed|bayern
  area_code    TEXT NOT NULL,
  PRIMARY KEY (concept_id, area_scheme, area_code)
);

-- Indicator/trait values (pointer to concept + vocabulary version, not the
-- numbers as ground truth). value is always present (domain.TraitValue.Value
-- is a plain float64, never a pointer); niche_width/n_systems are nullable
-- because EIVE provides them and Tichý/Midolo do not — NULL there means "this
-- vocabulary does not provide this datum", never a stand-in for 0 (see
-- domain.TraitValue's doc comment).
CREATE TABLE IF NOT EXISTS trait_value (
  concept_id    TEXT NOT NULL REFERENCES taxon_concept(id),
  vocab         TEXT NOT NULL,      -- eive|tichy2023|midolo2023
  vocab_version TEXT NOT NULL,
  dim           TEXT NOT NULL,      -- M|N|R|L|T|S
  value         REAL NOT NULL,
  niche_width   REAL,               -- EIVE only; NULL for Tichý/Midolo
  n_systems     INTEGER,            -- EIVE only; NULL for Tichý/Midolo
  -- resolution records HOW the vocabulary's taxon name was crosswalked onto
  -- concept_id: NULL for the ordinary case (an exact canonical match), else
  -- the name of the deterministic normalisation rule that was needed
  -- (domain.NormalizationRule: hybrid_spacing, aggregate_to_nominate,
  -- autonym, orthography_genitive, ...). Two of those rules equate two
  -- circumscriptions that are NOT identical — an aggregate is wider than
  -- its nominate species, an autonym narrower than its species — so a
  -- consumer must be able to tell such a value apart from a directly
  -- matched one. Same "absence is information" rule as niche_width above:
  -- NULL means "no normalisation was needed", never "unknown".
  -- See domain.TraitValue.Resolution / domain.NormalizationRule.Flagged.
  resolution    TEXT,
  PRIMARY KEY (concept_id, vocab, vocab_version, dim)
);

-- Trait-vocabulary provenance metadata: one row per ingested (vocab,
-- version) pair, joined onto trait_value reads to surface VocabVersion and
-- the Taxonomy namespace (see domain.TraitSet.Taxonomy) each vocabulary's
-- values are harmonized against.
CREATE TABLE IF NOT EXISTS trait_vocabulary (
  vocab          TEXT NOT NULL,      -- eive|tichy2023|midolo2023
  version        TEXT NOT NULL,
  taxonomy       TEXT NOT NULL,      -- euromed-aligned|floraveg-eunis-aligned|...
  license        TEXT,
  source_url     TEXT,
  ingested_at    TEXT NOT NULL,
  redistribution TEXT NOT NULL DEFAULT 'unknown', -- allowed|restricted|unknown (domain.Redistribution); gates ExportBundle, never local ingest
  PRIMARY KEY (vocab, version)
);

-- sec. reference spaces (SP5). One row per bibliographic reference frame a
-- concept's circumscription is stated in — "Wisskirchen & Haeupler 1998",
-- "HEGI: Illustrierte Flora von Mitteleuropa", "TUTIN et al.: Flora
-- Europaea", ... The CDM rl_standardliste harvest carries 18 of them.
--
-- taxon_concept.sec_reference stores the id of one of these rows, but is
-- deliberately NOT declared a foreign key onto it: the column predates this
-- table (SP1) and every backbone ingest before SP5 wrote the empty string
-- there rather than NULL, which an FK would now reject. The lookup is a
-- plain join instead.
CREATE TABLE IF NOT EXISTS sec_reference (
  id     TEXT PRIMARY KEY,   -- CDM citation uuid
  title  TEXT NOT NULL       -- the citation as the source spells it
);

CREATE INDEX IF NOT EXISTS idx_taxon_concept_sec_reference ON taxon_concept(sec_reference);

-- Concept relations (UC6, populated by SP5's CDM ingest).
--
-- The `relation` vocabulary below is MEASURED, not assumed. Until SP5 this
-- column's comment read `congruent|includes|included_in|overlaps|disjoint`
-- — five values SP1 assumed. A full crawl of the CDM rl_standardliste
-- (26.346 relations, pipelines/cdm/cdm.summary.txt) corrected that
-- assumption in both directions: `disjoint` NEVER occurs and has been
-- dropped, `included_in` never occurs either but is retained as the
-- documented inverse of `includes` (the source only ever states the
-- `Includes` direction, and hostus stores the stated direction verbatim
-- rather than materialising a mirror row), and three values the assumed
-- five did not have DO occur — `pro_parte`, `misapplied` and the genuinely
-- uncertain `includes_or_included_in_or_overlaps` (CDM's ⊂⊃⊕), which is
-- never collapsed onto `overlaps`. `not_congruent` occurs exactly once in
-- 26.346 rows. See internal/domain.Relation / ParseRelation, which is the
-- single place this vocabulary is parsed and which fails loudly rather than
-- coercing an unmapped value.
--
-- `relation` is part of the PRIMARY KEY. It was not in SP1's, which meant
-- two DIFFERENT relation types asserted between the same concept pair by
-- the same source would silently overwrite one another under
-- INSERT OR REPLACE. CDM does emit two edges per pair (see the misapplied +
-- congruent pair on Pinus abies -> Abies alba in the fixture), so this is a
-- real case, not a hypothetical one. Legacy databases are migrated by
-- migrateConceptRelationPK in db.go.
CREATE TABLE IF NOT EXISTS concept_relation (
  from_concept  TEXT NOT NULL REFERENCES taxon_concept(id),
  to_concept    TEXT NOT NULL REFERENCES taxon_concept(id),
  relation      TEXT NOT NULL,      -- see the vocabulary note above (domain.Relation)
  source        TEXT,               -- the backbone/source id asserting it, e.g. "cdm"
  PRIMARY KEY (from_concept, to_concept, relation, source)
);

-- from_concept is the PK's leading column (skipped, see comment above);
-- to_concept is not, so it needs its own index.
CREATE INDEX IF NOT EXISTS idx_concept_relation_to_concept ON concept_relation(to_concept);

-- Full-text/prefix search.
--
-- fts_name is a "contentless" FTS5 table (content=''): FTS5 stores only the
-- inverted index, not the row data, so every fts_name row needs an external
-- place that maps its rowid back to the taxon_concept it indexes. fts_name
-- rowid <-> concept mapping strategy:
--   fts_name_map(rowid, concept_id) is a normal table with an
--   INTEGER PRIMARY KEY (a rowid alias, since taxon_concept.id is TEXT and
--   cannot itself alias SQLite's rowid). To index a concept, first
--   INSERT INTO fts_name_map(concept_id) VALUES (?) and take
--   last_insert_rowid(), then
--   INSERT INTO fts_name(rowid, canonical, vernacular_de) VALUES (<that
--   rowid>, ?, ?). To resolve a search hit back to a concept:
--   SELECT m.concept_id FROM fts_name f JOIN fts_name_map m
--   ON m.rowid = f.rowid WHERE fts_name MATCH ?.
-- Population of this table is out of scope for this task (it belongs to the
-- suggest/autocomplete use case); the table exists so the schema is
-- complete and the mapping is exercised by tests here.
CREATE TABLE IF NOT EXISTS fts_name_map (
  rowid       INTEGER PRIMARY KEY,
  concept_id  TEXT NOT NULL REFERENCES taxon_concept(id)
);

-- rowid IS the table's own INTEGER PRIMARY KEY, but concept_id is a
-- separate, unindexed FK column — needs its own index.
CREATE INDEX IF NOT EXISTS idx_fts_name_map_concept_id ON fts_name_map(concept_id);

CREATE VIRTUAL TABLE IF NOT EXISTS fts_name USING fts5(
  canonical, vernacular_de,
  content='',                       -- external content, ids via rowid mapping
  tokenize='unicode61 remove_diacritics 2'
);

-- Bundle provenance. Created (empty) in every database this schema is
-- applied to, but only ever populated by an offline bundle export (see
-- internal/adapters/sqlite/bundle.go) — the server-side hostus.sqlite this
-- schema also backs simply never gets a row here.
-- restricted_sources is a comma-joined, sorted list of backbone/trait-
-- vocabulary ids whose redistribution was NOT "allowed" but were included
-- anyway via --force-include-restricted (see ExportBundle in bundle.go).
-- Empty ('') means either the bundle was exported with no --force flag (in
-- which case ExportBundle would have refused if any source were
-- restricted, so an exported bundle with restricted_sources='' is provably
-- clean) or every contributing source was itself "allowed".
CREATE TABLE IF NOT EXISTS bundle_meta (
  snapshot_version     TEXT NOT NULL,
  area                 TEXT NOT NULL,
  created_at           TEXT NOT NULL,  -- RFC3339 timestamp
  source_manifest_sha  TEXT NOT NULL,
  restricted_sources   TEXT NOT NULL DEFAULT ''
);
