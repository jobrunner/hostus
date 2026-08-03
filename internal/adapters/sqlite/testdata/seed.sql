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
-- something. "Festuca ovinaxy" (length-diff 2 from the query, still within
-- the window) is the fix-round-1 addition pinning
-- TestMatchFuzzyCandidates_OrdersByClosestLengthFirst: with limit=1 among
-- three admissible candidates at different length-diffs, the query must
-- deterministically return one of the diff-0 rows, never the diff-2 one —
-- proving fuzzyCandidateNameIDs' ORDER BY (not an arbitrary SQLite subset)
-- decides which row survives the LIMIT.
INSERT INTO name (id, canonical, canonical_fold, authorship, rank, ipni_id, published_in, nom_status, basionym_id) VALUES
  ('n-aira-canescens',           'aira canescens',           'aira canescens',           'L.',           'SPECIES', NULL, NULL, NULL, NULL),
  ('n-corynephorus-canescens',   'corynephorus canescens',   'corynephorus canescens',   '(L.) P.Beauv.','SPECIES', 'urn:lsid:ipni.org:names:391847-1', NULL, NULL, 'n-aira-canescens'),
  ('n-weingaertneria-canescens', 'Wéingaertneria canéscens', 'weingaertneria canescens', '(L.) Bernh.',  'SPECIES', NULL, NULL, NULL, 'n-aira-canescens'),
  ('n-jacobaea-vulgaris',        'jacobaea vulgaris',        'jacobaea vulgaris',        'Gaertn.',      'SPECIES', NULL, NULL, NULL, NULL),
  ('n-senecio-jacobaea',         'senecio jacobaea',         'senecio jacobaea',         'L.',           'SPECIES', NULL, NULL, NULL, NULL),
  ('n-festuca-ovona',            'Festuca ovona',            'festuca ovona',            'Test',         'SPECIES', NULL, NULL, NULL, NULL),
  ('n-festuca-ovena',            'Festuca ovena',            'festuca ovena',            'Test',         'SPECIES', NULL, NULL, NULL, NULL),
  ('n-festuca-ovinaxy',          'Festuca ovinaxy',          'festuca ovinaxy',          'Test',         'SPECIES', NULL, NULL, NULL, NULL),
  ('n-abies-alba',               'Abies alba',               'abies alba',               'Mill.',        'SPECIES', NULL, NULL, NULL, NULL);

INSERT INTO taxon_concept (id, backbone_id, accepted_name, rank, parent_id, sec_reference, status) VALUES
  ('c-corynephorus-canescens', 'wcvp', 'n-corynephorus-canescens', 'SPECIES', NULL, 'WCVP (2026)', 'ACCEPTED'),
  ('c-jacobaea-vulgaris',      'wcvp', 'n-jacobaea-vulgaris',      'SPECIES', NULL, 'WCVP (2026)', 'ACCEPTED'),
  ('c-festuca-ovona',          'wcvp', 'n-festuca-ovona',          'SPECIES', NULL, 'WCVP (2026)', 'ACCEPTED'),
  ('c-festuca-ovena',          'wcvp', 'n-festuca-ovena',          'SPECIES', NULL, 'WCVP (2026)', 'ACCEPTED'),
  ('c-festuca-ovinaxy',        'wcvp', 'n-festuca-ovinaxy',        'SPECIES', NULL, 'WCVP (2026)', 'ACCEPTED'),
  ('c-abies-alba',             'wcvp', 'n-abies-alba',             'SPECIES', NULL, 'WCVP (2026)', 'ACCEPTED');

INSERT INTO concept_name (concept_id, name_id, role, homotypic) VALUES
  ('c-corynephorus-canescens', 'n-corynephorus-canescens',   'accepted', NULL),
  ('c-corynephorus-canescens', 'n-aira-canescens',           'synonym',  1),
  ('c-corynephorus-canescens', 'n-weingaertneria-canescens', 'synonym',  1),
  ('c-jacobaea-vulgaris',      'n-jacobaea-vulgaris',        'accepted', NULL),
  ('c-jacobaea-vulgaris',      'n-senecio-jacobaea',         'synonym',  1),
  ('c-festuca-ovona',          'n-festuca-ovona',            'accepted', NULL),
  ('c-festuca-ovena',          'n-festuca-ovena',            'accepted', NULL),
  ('c-festuca-ovinaxy',        'n-festuca-ovinaxy',          'accepted', NULL),
  ('c-abies-alba',             'n-abies-alba',               'accepted', NULL);

INSERT INTO xref (concept_id, authority, ext_id) VALUES
  ('c-corynephorus-canescens', 'powo',  '396681-1'),
  ('c-corynephorus-canescens', 'colxr', 'YQW8'),
  ('c-jacobaea-vulgaris',      'powo',  '226649-1');

INSERT INTO distribution (concept_id, area_scheme, area_code) VALUES
  ('c-corynephorus-canescens', 'wgsrpd_l3', 'GER'),
  ('c-corynephorus-canescens', 'wgsrpd_l3', 'FRA');

-- SP6 Task 3 (GET /v1/concept/{id}/synonyms) additions.
--
-- c-uc5-corynephorus is a REDUCED, name-disambiguated replica of the real
-- index's Corynephorus canescens (wcvp:concept:405825, 26 synonyms): one
-- synonym per UC5 outcome, so the publication filter, the exclusion
-- summary and the ranking order are all decided by real-shaped data rather
-- than by invented statuses. Every canonical carries a "uc5 " prefix so it
-- cannot collide with the MatchExact/Suggest fixtures above.
--
--   n-uc5-aira-canescens          homotypic 1, IS the accepted name's
--                                 basionym  -> publishable, ranks FIRST
--   n-uc5-avena-canescens         homotypic 1                 -> publishable
--   n-uc5-weingaertneria-canescens homotypic 1                -> publishable
--   n-uc5-aira-breviculmis        homotypic NULL (unknown)    -> publishable,
--                                 ranks after the homotypic block
--   n-uc5-corynephorus-incanescens ", nom. illeg. superfl."   -> nom_status
--                                 (the UC5 worked example's exclusion)
--   n-uc5-aira-triflora           ", pro syn."                -> nom_status
--   n-uc5-var-andinus             VARIETY + ", nom. nud."     -> nom_status
--                                 (a defect outranks the rank rule)
--   n-uc5-sensu-auct              ", sensu auct."             -> unclassified
--   n-uc5-var-montana             VARIETY                     -> rank
--   n-uc5-f-pallidus              FORM                        -> rank
--   n-uc5-proles                  OTHER + rank_verbatim 'proles' -> publishable
--                                 (6.409 synonym rows rank OTHER, 3.731 of
--                                 them with a verbatim spelling; NONE is
--                                 excluded by rank=species, so they reach
--                                 publication lists and must not render as a
--                                 bare "OTHER")
--
-- c-uc5-genus is an accepted concept with NO synonyms at all — the
-- "empty list, not a 404" case.
--
-- c-uc5-heterotypic carries the only concept_name.homotypic = 0 row in any
-- fixture. It is NOT faithful to the measured index (which has 271.821
-- rows at 1, 1.133.475 at NULL and ZERO at 0 — see domain.Typification):
-- it exists solely to pin the repository scan, because an adapter that
-- collapsed a stored 0 onto NULL, or onto a pointer-to-true, would
-- otherwise be indistinguishable from a correct one on this corpus.
INSERT INTO name (id, canonical, canonical_fold, authorship, rank, ipni_id, published_in, nom_status, basionym_id) VALUES
  ('n-uc5-aira-canescens',           'uc5 aira canescens',            'uc5 aira canescens',            'L.',                  'SPECIES', NULL, NULL, NULL,                     NULL),
  ('n-uc5-corynephorus-canescens',   'uc5 corynephorus canescens',    'uc5 corynephorus canescens',    '(L.) P.Beauv.',       'SPECIES', NULL, NULL, NULL,                     'n-uc5-aira-canescens'),
  ('n-uc5-avena-canescens',          'uc5 avena canescens',           'uc5 avena canescens',           '(L.) Weber',          'SPECIES', NULL, NULL, NULL,                     NULL),
  ('n-uc5-weingaertneria-canescens', 'uc5 weingaertneria canescens',  'uc5 weingaertneria canescens',  '(L.) Bernh.',         'SPECIES', NULL, NULL, NULL,                     NULL),
  ('n-uc5-aira-breviculmis',         'uc5 aira breviculmis',          'uc5 aira breviculmis',          'Loisel.',             'SPECIES', NULL, NULL, NULL,                     NULL),
  ('n-uc5-corynephorus-incanescens', 'uc5 corynephorus incanescens',  'uc5 corynephorus incanescens',  'Bubani',              'SPECIES', NULL, NULL, ', nom. illeg. superfl.', NULL),
  ('n-uc5-aira-triflora',            'uc5 aira triflora',             'uc5 aira triflora',             'Willd. ex Steud.',    'SPECIES', NULL, NULL, ', pro syn.',             NULL),
  ('n-uc5-var-andinus',              'uc5 corynephorus canescens var. andinus', 'uc5 corynephorus canescens var. andinus', 'Hack. ex Sodiro', 'VARIETY', NULL, NULL, ', nom. nud.', NULL),
  ('n-uc5-sensu-auct',               'uc5 corynephorus fallax',       'uc5 corynephorus fallax',       'auct.',               'SPECIES', NULL, NULL, ', sensu auct.',          NULL),
  ('n-uc5-var-montana',              'uc5 corynephorus canescens var. montana', 'uc5 corynephorus canescens var. montana', 'Cout.', 'VARIETY', NULL, NULL, NULL, NULL),
  ('n-uc5-f-pallidus',               'uc5 corynephorus canescens f. pallidus',  'uc5 corynephorus canescens f. pallidus',  '(Beckh.) Soó', 'FORM', NULL, NULL, NULL, NULL),
  ('n-uc5-proles',                   'uc5 corynephorus articulatus',  'uc5 corynephorus articulatus',  'Desf.',               'OTHER',   NULL, NULL, NULL,                     NULL),
  ('n-uc5-genus',                    'uc5 corynephorus',              'uc5 corynephorus',              'P.Beauv.',            'GENUS',   NULL, NULL, NULL,                     NULL),
  ('n-uc5-heterotypic-accepted',     'uc5 jacobaea vulgaris',         'uc5 jacobaea vulgaris',         'Gaertn.',             'SPECIES', NULL, NULL, NULL,                     NULL),
  ('n-uc5-heterotypic-synonym',      'uc5 senecio jacobaea',          'uc5 senecio jacobaea',          'L.',                  'SPECIES', NULL, NULL, NULL,                     NULL);

INSERT INTO taxon_concept (id, backbone_id, accepted_name, rank, parent_id, sec_reference, status) VALUES
  ('c-uc5-corynephorus', 'wcvp', 'n-uc5-corynephorus-canescens', 'SPECIES', NULL, 'WCVP (2026)', 'ACCEPTED'),
  ('c-uc5-genus',        'wcvp', 'n-uc5-genus',                  'GENUS',   NULL, 'WCVP (2026)', 'ACCEPTED'),
  ('c-uc5-heterotypic',  'wcvp', 'n-uc5-heterotypic-accepted',   'SPECIES', NULL, 'WCVP (2026)', 'ACCEPTED');

INSERT INTO concept_name (concept_id, name_id, role, homotypic) VALUES
  ('c-uc5-corynephorus', 'n-uc5-corynephorus-canescens',   'accepted', NULL),
  ('c-uc5-corynephorus', 'n-uc5-aira-canescens',           'synonym',  1),
  ('c-uc5-corynephorus', 'n-uc5-avena-canescens',          'synonym',  1),
  ('c-uc5-corynephorus', 'n-uc5-weingaertneria-canescens', 'synonym',  1),
  ('c-uc5-corynephorus', 'n-uc5-aira-breviculmis',         'synonym',  NULL),
  ('c-uc5-corynephorus', 'n-uc5-corynephorus-incanescens', 'synonym',  NULL),
  ('c-uc5-corynephorus', 'n-uc5-aira-triflora',            'synonym',  NULL),
  ('c-uc5-corynephorus', 'n-uc5-var-andinus',              'synonym',  NULL),
  ('c-uc5-corynephorus', 'n-uc5-sensu-auct',               'synonym',  NULL),
  ('c-uc5-corynephorus', 'n-uc5-var-montana',              'synonym',  NULL),
  ('c-uc5-corynephorus', 'n-uc5-f-pallidus',               'synonym',  NULL),
  ('c-uc5-corynephorus', 'n-uc5-proles',                   'synonym',  NULL),
  ('c-uc5-genus',        'n-uc5-genus',                    'accepted', NULL),
  ('c-uc5-heterotypic',  'n-uc5-heterotypic-accepted',     'accepted', NULL),
  ('c-uc5-heterotypic',  'n-uc5-heterotypic-synonym',      'synonym',  0);

UPDATE name SET rank_verbatim = 'proles' WHERE id = 'n-uc5-proles';
