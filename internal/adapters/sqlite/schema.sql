-- hostus 2.0 local taxonomy index schema (spec §4.3). Applied verbatim,
-- server-side and in the offline bundle. Every statement is
-- IF NOT EXISTS so Open() can apply it idempotently against an
-- already-initialized database.

PRAGMA foreign_keys = ON;

-- Artifact/version metadata.
CREATE TABLE IF NOT EXISTS backbone_version (
  id            TEXT PRIMARY KEY,   -- e.g. "wcvp"
  version       TEXT NOT NULL,      -- "2026-06-15" (period/version, never "latest")
  license       TEXT,
  source_url    TEXT,
  ingested_at   TEXT NOT NULL,
  manifest_sha  TEXT NOT NULL       -- checksum of the validated manifest
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
  rank           TEXT NOT NULL,      -- FAMILY|GENUS|SPECIES|SUBSPECIES|VARIETY|FORM
  ipni_id        TEXT,
  published_in   TEXT,
  nom_status     TEXT,               -- NULL|nom_nud|nom_superfl|pro_syn|...
  basionym_id    TEXT REFERENCES name(id)
);

CREATE INDEX IF NOT EXISTS idx_name_canonical_fold ON name(canonical_fold);

-- Taxonomy.
CREATE TABLE IF NOT EXISTS taxon_concept (
  id             TEXT PRIMARY KEY,
  backbone_id    TEXT NOT NULL REFERENCES backbone_version(id),
  accepted_name  TEXT NOT NULL REFERENCES name(id),
  rank           TEXT NOT NULL,
  parent_id      TEXT REFERENCES taxon_concept(id),
  sec_reference  TEXT,              -- bibliographic sec. identity
  status         TEXT NOT NULL      -- accepted|...
);

-- Concept <-> name (accepted + synonyms, typed).
CREATE TABLE IF NOT EXISTS concept_name (
  concept_id   TEXT NOT NULL REFERENCES taxon_concept(id),
  name_id      TEXT NOT NULL REFERENCES name(id),
  role         TEXT NOT NULL,       -- accepted|synonym
  homotypic    INTEGER,             -- 1=homotypic (basionym/recombination), 0=heterotypic
  PRIMARY KEY (concept_id, name_id)
);

-- Cross-references to external authorities.
CREATE TABLE IF NOT EXISTS xref (
  concept_id   TEXT NOT NULL REFERENCES taxon_concept(id),
  authority    TEXT NOT NULL,       -- powo|colxr|euromed|gbif|wikidata|wfo|inat|floraveg
  ext_id       TEXT NOT NULL,
  PRIMARY KEY (authority, ext_id)
);

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
-- numbers as ground truth). Created here but unused until SP3/SP5.
CREATE TABLE IF NOT EXISTS trait_value (
  concept_id    TEXT NOT NULL REFERENCES taxon_concept(id),
  vocab         TEXT NOT NULL,      -- eive|tichy2023|midolo2023
  vocab_version TEXT NOT NULL,
  dim           TEXT NOT NULL,      -- M|N|R|L|T|S
  value         REAL,
  niche_width   REAL,               -- EIVE only
  n_systems     INTEGER,
  PRIMARY KEY (concept_id, vocab, vocab_version, dim)
);

-- Concept relations (UC6). Created here but unused until SP3/SP5.
CREATE TABLE IF NOT EXISTS concept_relation (
  from_concept  TEXT NOT NULL REFERENCES taxon_concept(id),
  to_concept    TEXT NOT NULL REFERENCES taxon_concept(id),
  relation      TEXT NOT NULL,      -- congruent|includes|included_in|overlaps|disjoint
  source        TEXT,               -- e.g. wisskirchen-1998
  PRIMARY KEY (from_concept, to_concept, source)
);

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

CREATE VIRTUAL TABLE IF NOT EXISTS fts_name USING fts5(
  canonical, vernacular_de,
  content='',                       -- external content, ids via rowid mapping
  tokenize='unicode61 remove_diacritics 2'
);
