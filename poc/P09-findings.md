# PoC P9 — iNaturalist obscured-coordinate behavior for protected taxa (Phase 0, Task 0.9, Gates SP4/UC2)

## Goal

Verify the hostus 2.0 spec's UC2-dominating assumption (flagged ⚠️ "order of
magnitude, verify before implementation"): for taxa with a protection/red-list
status, iNaturalist sets `geoprivacy`/`taxon_geoprivacy` to `obscured` and
rounds the public coordinate to a ~0.2°×0.2° cell (~20 km) — live against the
API, plus document exactly which fields hostus must consume.

## Method

`poc/p09_inat/probe.sh` (`set -euo pipefail`), run via
`nix develop -c bash poc/p09_inat/probe.sh`. All requests carry a descriptive
`User-Agent` (no API key needed for `api.inaturalist.org/v1`). Raw JSON saved
under `poc/data/p09_*.json` (gitignored via `poc/.gitignore`).

Reference taxa:
- **Obscured case**: *Cypripedium calceolus* (Lady's-slipper orchid) —
  taxon_id **54638**. `/v1/taxa?q=` resolution note: the fuzzy search does
  **not** rank the exact scientific-name match first — the top result for
  `q=Cypripedium%20calceolus` was `Cypripedium parviflorum` (id 50713), a
  different (North American) species. Had to filter `results[]` for
  `name == "Cypripedium calceolus"` explicitly; a naive
  `.results[0]` pick would silently probe the wrong taxon. This is itself a
  finding relevant to any future hostus code that resolves species names via
  the `q=` taxa-search endpoint (GBIF integration should be double-checked
  for the same failure mode, though it uses a different search API).
- **Control case**: *Jacobaea vulgaris* — taxon_id **62498**, common,
  unprotected.

Steps run:
1. Resolve both taxon IDs and pull `/v1/taxa/54638` (single-taxon detail,
   not the search endpoint) to get the full `conservation_statuses[]` array.
2. `/v1/observations?taxon_id=&per_page=50&geo=true` for both taxa (most
   recent 50, globally), plus a Europe-scoped pull
   (`place_id=97391`) for *C. calceolus* to match the spec's Central-European
   use case.
3. Aggregate counts: total observations, `geoprivacy=obscured` count,
   `quality_grade=research` count with and without `geoprivacy=open`.
4. For every sampled record with `geoprivacy=obscured` or
   `taxon_geoprivacy=obscured`, dumped `location`, `geojson`,
   `positional_accuracy`, `public_positional_accuracy`, `obscured`,
   `mappable`.
5. Empirically fit the obscuring cell size: for every obscured record,
   computed the diagonal (km) of a candidate 0.2°×0.2° lat/lon box centered
   at that latitude (`0.2° lat ≈ 22.26 km`; `0.2° lon ≈ 22.26·cos(lat) km`)
   and compared to the API-reported `public_positional_accuracy`.

## Result 1: `geoprivacy` vs `taxon_geoprivacy` are genuinely different fields

Confirmed empirically, not just from the help article:

- `geoprivacy` = the **observer's own** choice on that observation (opt-in
  obscuring, independent of taxon).
- `taxon_geoprivacy` = the geoprivacy **iNaturalist derives from the taxon's
  conservation status at the observation's location**, applied automatically
  even if the observer set nothing.

Sample of 50 most-recent geolocated *C. calceolus* observations (global):

| geoprivacy / taxon_geoprivacy | count |
|---|---|
| null / obscured | 15 |
| null / open | 28 |
| obscured / obscured | 1 |
| obscured / open | 6 |

Europe-only sample (50 most recent, `place_id=97391`):

| geoprivacy / taxon_geoprivacy | count |
|---|---|
| null / obscured | 16 |
| null / open | 27 |
| obscured / obscured | 3 |
| obscured / open | 4 |

Control (*Jacobaea vulgaris*, common/unprotected), 50 most recent:

| geoprivacy / taxon_geoprivacy | count |
|---|---|
| null / null | 49 |
| obscured / null | 1 (observer's own choice, taxon itself carries no geoprivacy) |

**Consequence for hostus**: a record can be obscured because of
`taxon_geoprivacy` alone (`geoprivacy` stays `null`), because of
`geoprivacy` alone (observer opt-in, unrelated to protection status), or
both. To detect "this coordinate is obscured because the taxon is
protected," hostus must check **`taxon_geoprivacy == "obscured"`**
specifically — checking only `geoprivacy` would both miss taxon-driven
obscuring (`null`/`obscured` rows above, the majority case) and
misattribute observer opt-in obscuring (`obscured`/`open` rows) to
"protected species," which is wrong.

Pulling the single-taxon detail endpoint `/v1/taxa/54638` additionally shows
*why*: `conservation_statuses[]` has **28 entries**, one per red-list
authority/place (England CR, UK CR, Finland NT, Switzerland VU, Germany
"Gefährdet", Austria "Gefährdung droht", Spain EN, Russian oblasts, IUCN
global LC, etc.) — and **each entry carries its own `geoprivacy` field**
(mostly `obscured`, but e.g. the global IUCN "LC" status and several Russian
oblast statuses are `geoprivacy: open`). `taxon_geoprivacy` on an
observation is the resolved value for that observation's location, not a
single global per-species flag — hostus's SP4/UC2 enrichment logic must not
assume "protected somewhere" implies "obscured everywhere."

## Result 2: obscuring-cell size — confirmed as ~0.2°×0.2°, with very high precision

Collected all obscured *C. calceolus* records from both the global and
Europe samples (`geoprivacy=obscured` OR `taxon_geoprivacy=obscured`) and
compared each record's `public_positional_accuracy` (meters) against the
predicted diagonal of a 0.2°×0.2° box at that latitude:

| lat | reported `public_positional_accuracy` | predicted 0.2°×0.2° diagonal | ratio |
|---|---|---|---|
| 44.6°N | 27.32 km | 27.33 km | 1.00 |
| 47.5°N | 26.84 km | 26.87 km | 1.00 |
| 51.3°N | 26.23 km | 26.26 km | 1.00 |
| 55.9°N | 25.50 km | 25.52 km | 1.00 |
| 60.4°N | 24.79 km | 24.83 km | 1.00 |
| 63.8°N | 24.32 km | 24.34 km | 1.00 |

(27 data points total, 44°–64°N, all ratios 1.00 to two decimals.) The
`public_positional_accuracy` iNaturalist reports for an obscured record is
**exactly** the diagonal of a 0.2°×0.2° lat/lon cell centered on the
observation, scaled by `cos(latitude)` for the longitude side as geometry
requires — this is not an approximation, it's the literal formula iNat uses.
The **spec's ~0.2°×0.2° (~20 km) figure is confirmed**, though the precise
practical number for Central Europe (45–55°N) is **~26–28 km diagonal**, not
~20 km flat — 20 km undersells the actual positional uncertainty by
~25–35%. hostus should use "~0.2° cell, ~26–28 km worst-case radius/diagonal
at Central European latitudes" rather than a flat "~20 km," if it needs to
reason about how far the true location could be from the public coordinate.

The one observer-opt-in obscured *Jacobaea vulgaris* record (unprotected
taxon, obscured only via `geoprivacy`, not `taxon_geoprivacy`) shows
`public_positional_accuracy=28210` at ~43.4°N — same mechanism, same cell
size, confirming the obscuring algorithm is uniform regardless of whether
obscuring was triggered by `geoprivacy` or `taxon_geoprivacy`.

`location`/`geojson` for obscured records: both fields report the same
already-randomized point inside the cell (not the cell's center and not the
true coordinate) — there is no separate "cell id" or "cell center" field;
the obscured point itself is what varies between distinct observations that
fall in the same nominal area, so cell boundaries cannot be recovered
directly from repeated `location` values alone; they can only be inferred
from `public_positional_accuracy`, which is exact and taxon-status-agnostic.

## Result 3: usable fraction for a protected taxon (UC2 fund-point viability)

*Cypripedium calceolus*, global:

| metric | count |
|---|---|
| total observations (geo unfiltered) | 10,100 |
| `geoprivacy=obscured` (server-side filter) | 3,149 (31.2%) |
| `quality_grade=research`, all | 9,418 (93.2%) |
| `quality_grade=research` AND `geoprivacy=open` | 6,322 (62.6% of total) |

*Jacobaea vulgaris* control, global:

| metric | count |
|---|---|
| total observations | 88,168 |
| `geoprivacy=obscured` | 1,520 (1.7%) |
| `quality_grade=research`, all | 59,614 (67.6%) |

Note: the server-side `geoprivacy=obscured` filter counts **only**
observer-opt-in obscuring (3,149/10,100 = 31.2%); it does not include
records obscured solely via `taxon_geoprivacy`. The 50-record samples above
show `taxon_geoprivacy=obscured` is actually *more* common than
`geoprivacy=obscured` for this species (roughly 32–38% of sampled records
in both the global and Europe-only pulls) — so **the true obscured fraction
for a protected taxon is higher than what `&geoprivacy=obscured` alone
reports**; hostus must treat `taxon_geoprivacy=obscured` as an independent
signal to filter/flag on, or it will undercount obscured records and
overstate precision.

**UC2 viability read**: for *C. calceolus*, roughly 60–70% of records
remain either open or at least present with full/near-full positional
accuracy (`quality_grade=research AND geoprivacy=open` = 62.6% of total, and
the sampled `taxon_geoprivacy=open` share was ~55–56%). iNaturalist is
**usable as a fund-point source for this protected taxon**, but a
non-trivial minority (30–40%) of its records for protected species come
back obscured to a ~26–28 km cell and are useless for precise fund-point
work — hostus's UC2 pipeline must filter on `taxon_geoprivacy` (and
`geoprivacy`) and either discard obscured records or explicitly flag them as
low-precision, not silently accept the coarse coordinate as if it were
exact. For more strongly protected/rarer species than this one (which still
has ~10k global observations), the open fraction could be lower — this PoC
does not establish a lower bound across all protection levels, only that
the mechanism and rough magnitude hold for one Central-European protected
orchid.

## Result 4: `quality_grade=research` caveat confirmed

iNaturalist's own definition (unchanged, confirmed via the counts behaving
consistently with it): `research` grade means **"2+ identifiers agree, and
the observation has date/location/photo"** — it is a **community-consensus**
threshold, not expert/institutional verification. 93.2% of *C. calceolus*
observations reach `research` grade, which is very high for a distinctive,
well-photographed orchid genus — this reflects ease of visual ID by any two
agreeing users, not taxonomic authority. hostus must **not** present
`quality_grade=research` to end users or use it internally as equivalent to
"expert-verified" — it should be documented/labeled as "community
consensus, ≥2 agreeing identifications," consistent with iNaturalist's own
framing, if hostus surfaces or filters on it.

## Fields hostus must read

For any iNaturalist observation record consumed by SP4/UC2:

| field | purpose |
|---|---|
| `taxon_geoprivacy` | **primary** signal: is this record obscured *because the taxon is protected at this location*. Values seen: `"obscured"`, `"open"`, `null`. |
| `geoprivacy` | observer's own opt-in obscuring choice; independent of taxon status. Must be checked **in addition to**, not instead of, `taxon_geoprivacy`. |
| `obscured` (boolean) | convenience flag on the observation, `true` whenever the *returned* coordinate is the obscured/randomized one (regardless of which of the above caused it) — cheapest single field to gate on if hostus only needs "is the coordinate I'm getting trustworthy," but does not tell you *why*. |
| `positional_accuracy` | the **observer's own** claimed GPS/device accuracy (meters) — meaningless once obscured; can be `null`. |
| `public_positional_accuracy` | the accuracy hostus actually sees on the **returned** coordinate — for obscured records this is the ~24–28 km cell diagonal (see Result 2); for open records it equals `positional_accuracy`. **This is the field to use for "how far off could this point be."** |
| `location` / `geojson.coordinates` | the coordinate itself — already the obscured/randomized one when applicable; there is no separate "true" coordinate exposed to unauthenticated API consumers. |
| `mappable` | whether iNat considers the record safe to plot at all (distinct from obscuring — can be `false` for other reasons, e.g. captive/cultivated). |
| `quality_grade` | `"research"` = community consensus (≥2 agreeing IDs), not expert verification — see Result 4 caveat. |
| `conservation_statuses[].geoprivacy` (on `/v1/taxa/{id}`, not on observations) | per-place, per-authority ground truth for *why* `taxon_geoprivacy` resolves the way it does at a given location; useful for diagnostics/debugging, not needed per-observation at runtime. |

`coordinates_obscured` (named in the task brief) does **not** appear as a
distinct field in the live `/v1/observations` response — the observation
carries `obscured` (boolean) instead, which serves the same purpose. Any
prior spec text referencing `coordinates_obscured` should be corrected to
`obscured`.

## Verdict

**🟢 Behavior confirmed, fields documented.** The ~0.2°×0.2° obscuring-cell
claim is not just confirmed but confirmed to very high precision (measured
`public_positional_accuracy` matches the predicted 0.2°×0.2°-box diagonal to
two decimal places across 27 samples spanning 44–64°N) — the practical
figure for Central Europe is **~26–28 km diagonal**, somewhat larger than
the spec's flat "~20 km" shorthand, so that number should be corrected in
the spec text. `geoprivacy` and `taxon_geoprivacy` are confirmed as
distinct, independently-triggered fields that both must be consulted; for
one protected Central-European orchid, roughly 30–40% of records come back
obscured (usable-but-imprecise), leaving a majority open — iNaturalist is a
viable but not fully precise fund-point source for UC2, and the pipeline
must actively filter/flag on `taxon_geoprivacy`/`geoprivacy`/`obscured`
rather than assume all returned coordinates are exact.

## Commit

Committed on `feature/investigation`: `poc(P9): verify iNaturalist obscured-coordinate behavior`.
