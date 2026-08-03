# Fixture provenance

`wikidata-sample.csv` is a cut of the real canonical CSV produced by
`pipelines/wikidata/build.sh` (`pipelines/wikidata/output/wikidata-xref-canonical.csv`,
Wikidata Query Service, harvested 2026-08-02), plus a small number of
hand-added synthetic rows exercising paths the real 1,709,127-row output does
not (T1 verified zero dead join_ids and, at spot-check scale, no conflicts).

**Real rows (verbatim, unmodified)**: every row for `join_id` `226649-1`
(*Jacobaea vulgaris*), `396681-1` (*Corynephorus canescens*) and `331174-2`
(*Corynephorus*, genus) — the same three concepts the WCVP test fixture
(`internal/adapters/wcvp/testdata/wcvp-sample`) carries `powo` xrefs for, so
`xref.Read`'s output resolves against the seeded application-layer fixture
via the existing `powo` xref, exactly as the real ID-based join does.

**Synthetic rows (appended, clearly out of the real join_id range)**, each
added to exercise one ingest path that the plan requires tested:

- `powo|396681-1|wikidata|Q900003|Q900003` — a SECOND Wikidata item
  carrying the SAME `join_id` (`396681-1`) as the real `Q159953` row above.
  This is case (b) from the plan: one concept legitimately receiving two
  distinct ids for the same authority (`wikidata`) — not a conflict, both
  written.
- `powo|999999-9|inat|900001|Q900001` — a `join_id` (`999999-9`) that is not
  any fixture concept's `powo` xref, so it stays Unmatched.
- `powo|396681-1|inat|900002|Q900002` and
  `powo|226649-1|inat|900002|Q900002` — the SAME `(authority, ext_id)` pair
  (`inat`/`900002`) claimed by two DIFFERENT `join_id`s, resolving to two
  DIFFERENT concepts. This is case (a): a genuine conflict, skipped and
  reported, never silently resolved to either concept.

Licence: Wikidata is CC0 (`redistribution: allowed`); see
`pipelines/README.md`.
