-- Fixture for internal/adapters/sqlite read tests (Task 3): two concepts
-- with distinct shapes so Concept/ConceptByXref/MatchExact each get
-- something real to join against.
--
--   * Corynephorus canescens (L.) P.Beauv. — accepted, basionym Aira
--     canescens L., plus the non-basionymic recombination Weingaertneria
--     canescens (L.) Bernh. as a second synonym; two xrefs (powo, colxr);
--     two WGSRPD-L3 distribution rows.
--   * Jacobaea vulgaris Gaertn. — accepted, one synonym (Senecio jacobaea
--     L.), one xref (powo). No distribution rows, to exercise the
--     "concept with an empty distribution slice" path.
--
-- The Weingaertneria synonym's `canonical` is deliberately stored with a
-- mix of diacritics and non-ASCII casing ("Wéingaertneria canéscens")
-- while its `canonical_fold` is the plain-ASCII fold ("weingaertneria
-- canescens") domain.Canonicalize produces — this is what
-- MatchExact_*Diacritic* regression tests key off, to prove matches are
-- found via canonical_fold rather than SQLite's ASCII-only LOWER().
-- canonical_fold values below must equal domain.Canonicalize(canonical);
-- they are NOT derived by SQLite (see schema.sql's comment on the name
-- table for why).

INSERT INTO backbone_version (id, version, license, source_url, ingested_at, manifest_sha)
VALUES ('wcvp', '2026-06-15', 'CC-BY-4.0', 'https://example.org/wcvp.zip', '2026-07-31T00:00:00Z', 'deadbeef');

-- Task 6 (fuzzy matching) additions: a "festuca" trio whose canonicals are
-- all 13 runes long (same length as the query "festuca ovina" the
-- MatchFuzzyCandidates tests use) and share its first letter, so both
-- prefilter dimensions (first letter, length window) legitimately admit
-- ovona/ovena — and a wholly unrelated "abies alba" (different first
-- letter AND different length) to prove the prefilter actually excludes
-- something.
INSERT INTO name (id, canonical, canonical_fold, authorship, rank, ipni_id, published_in, nom_status, basionym_id) VALUES
  ('n-aira-canescens',           'aira canescens',           'aira canescens',           'L.',           'SPECIES', NULL, NULL, NULL, NULL),
  ('n-corynephorus-canescens',   'corynephorus canescens',   'corynephorus canescens',   '(L.) P.Beauv.','SPECIES', 'urn:lsid:ipni.org:names:391847-1', NULL, NULL, 'n-aira-canescens'),
  ('n-weingaertneria-canescens', 'Wéingaertneria canéscens', 'weingaertneria canescens', '(L.) Bernh.',  'SPECIES', NULL, NULL, NULL, 'n-aira-canescens'),
  ('n-jacobaea-vulgaris',        'jacobaea vulgaris',        'jacobaea vulgaris',        'Gaertn.',      'SPECIES', NULL, NULL, NULL, NULL),
  ('n-senecio-jacobaea',         'senecio jacobaea',         'senecio jacobaea',         'L.',           'SPECIES', NULL, NULL, NULL, NULL),
  ('n-festuca-ovona',            'Festuca ovona',            'festuca ovona',            'Test',         'SPECIES', NULL, NULL, NULL, NULL),
  ('n-festuca-ovena',            'Festuca ovena',            'festuca ovena',            'Test',         'SPECIES', NULL, NULL, NULL, NULL),
  ('n-abies-alba',               'Abies alba',               'abies alba',               'Mill.',        'SPECIES', NULL, NULL, NULL, NULL);

INSERT INTO taxon_concept (id, backbone_id, accepted_name, rank, parent_id, sec_reference, status) VALUES
  ('c-corynephorus-canescens', 'wcvp', 'n-corynephorus-canescens', 'SPECIES', NULL, 'WCVP (2026)', 'ACCEPTED'),
  ('c-jacobaea-vulgaris',      'wcvp', 'n-jacobaea-vulgaris',      'SPECIES', NULL, 'WCVP (2026)', 'ACCEPTED'),
  ('c-festuca-ovona',          'wcvp', 'n-festuca-ovona',          'SPECIES', NULL, 'WCVP (2026)', 'ACCEPTED'),
  ('c-festuca-ovena',          'wcvp', 'n-festuca-ovena',          'SPECIES', NULL, 'WCVP (2026)', 'ACCEPTED'),
  ('c-abies-alba',             'wcvp', 'n-abies-alba',             'SPECIES', NULL, 'WCVP (2026)', 'ACCEPTED');

INSERT INTO concept_name (concept_id, name_id, role, homotypic) VALUES
  ('c-corynephorus-canescens', 'n-corynephorus-canescens',   'accepted', NULL),
  ('c-corynephorus-canescens', 'n-aira-canescens',           'synonym',  1),
  ('c-corynephorus-canescens', 'n-weingaertneria-canescens', 'synonym',  1),
  ('c-jacobaea-vulgaris',      'n-jacobaea-vulgaris',        'accepted', NULL),
  ('c-jacobaea-vulgaris',      'n-senecio-jacobaea',         'synonym',  1),
  ('c-festuca-ovona',          'n-festuca-ovona',            'accepted', NULL),
  ('c-festuca-ovena',          'n-festuca-ovena',            'accepted', NULL),
  ('c-abies-alba',             'n-abies-alba',               'accepted', NULL);

INSERT INTO xref (concept_id, authority, ext_id) VALUES
  ('c-corynephorus-canescens', 'powo',  '396681-1'),
  ('c-corynephorus-canescens', 'colxr', 'YQW8'),
  ('c-jacobaea-vulgaris',      'powo',  '226649-1');

INSERT INTO distribution (concept_id, area_scheme, area_code) VALUES
  ('c-corynephorus-canescens', 'wgsrpd_l3', 'GER'),
  ('c-corynephorus-canescens', 'wgsrpd_l3', 'FRA');
