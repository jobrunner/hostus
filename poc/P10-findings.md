# PoC P10 — FloraVeg.EU/EUNIS-ESy vs EIVE/Euro+Med namespace divergence (Phase 0, Task 0.10, Gate SP3/UC4)

## Goal

Verify the assumption behind hostus 2.0's two-namespace design (spec §4.4:
"floraveg … nicht mit EIVE/EuroSL verwechseln"; §D.4): that the FloraVeg.EU /
EUNIS-ESy taxonomic namespace and the EIVE / EuroSL (Euro+Med-aligned)
namespace are genuinely different taxon concepts for at least some
Sandtrockenrasen-critical genera, so that trait values keyed in one namespace
must not be silently merged with/attributed to the other.

## Method

`poc/p10_namespace/compare.sh`, run inside the Nix dev shell
(`nix develop -c bash poc/p10_namespace/compare.sh`), reuses the EIVE 1.0
download from PoC P6 (`poc/data/eive_sm08.xlsx`, sheet `mainTable`, column
`TaxonConcept`) and fetches the EUNIS-ESy expert system v2021-06-01 from
Zenodo (`zenodo.org/api/records/4812736`, CC BY 4.0, file
`EUNIS-ESy-2021-06-01.txt`) into `poc/data/` (gitignored, reused by future
PoCs). ESy's `SECTION 1: Species aggregation` is the exact machine-readable
artifact for this question: for every accepted vegetation-plot concept it
lists all name-strings (synonyms, segregates, varieties, subspecies) folded
into that one concept — i.e. ESy's own taxonomic aggregation rules, not a
guess.

Tested groups: *Festuca ovina* agg./segregates (*F. filiformis*, *F. lemanii*,
*F. guestfalica*) and *Thymus praecox* agg. (incl. *T. jankae*), the two
genera named in spec §D.4 as most divergence-prone. Full captured run output:
`poc/p10_namespace/compare_output.txt`.

## Result: concrete divergence found

**EIVE (Euro+Med-aligned) treats `Festuca lemanii` as an independent
trait-bearing taxon concept** — its own row in `mainTable` with its own
`TaxonConcept` string and its own `EIVEres-M/N/R/L/T` indicator values,
separate from `Festuca ovina`, `Festuca ovina aggr.`, and `Festuca
filiformis` (all four are separate EIVE rows).

**EUNIS-ESy folds `Festuca lemanii` into the accepted concept `Festuca
ovina`** — it is not a top-level entry in ESy's Section 1 at all, but appears
as an indented synonym/segregate line under `Festuca ovina`, alongside
`Festuca ovina aggr.` itself and `Festuca guestfalica * guestfalica`/`*
hirtula`:

```
Festuca ovina                                             -  0
     Festuca ovina subsp. guestfalica                        0
     Festuca ovina subsp. hirtula                            0
     Festuca ovina subsp. molinieri                          0
     Festuca ovina subsp. ovina                              0
     Festuca ovina subsp. ovina var. ovina                   0
     Festuca ovina subsp. sudetica                           0
     Festuca ovina var. duriuscula                           0
     Festuca ovina (dood)                                    0
     Festuca ovina aggr.                                     0
     Festuca lemanii                                         0
     Festuca lemanii auct.                                   0
     Festuca indigesta subsp. alleizettei                    0
     Festuca guestfalica * guestfalica                       0
     Festuca guestfalica * hirtula                           0
```

`Festuca filiformis` is a *separate* accepted ESy concept (matching EIVE on
that point) but with a different segregate list again — ESy folds `Festuca
ovina var. capillata` and `Festuca ovina var. tenuifolia` into it, names that
do not appear at all as independent rows in EIVE's table:

```
Festuca filiformis                                        -  0
     Festuca ovina var. capillata                            0
     Festuca ovina var. tenuifolia                           0
```

**Concrete divergence, stated plainly:** the bare string `"Festuca lemanii"`
is a **stand-alone, independently trait-scored taxon** in EIVE/Euro+Med, but
a **synonym subsumed into `Festuca ovina`** in EUNIS-ESy/FloraVeg. A system
that joined a floraveg/ESy-side habitat or vegetation record for "Festuca
ovina" against the EIVE trait table by bare name only would silently pick up
`Festuca ovina`'s indicator values and never see the distinct `Festuca
lemanii` values EIVE actually publishes — or, in the opposite join direction,
would wrongly treat an ESy "Festuca ovina" occurrence record as informative
about the separately-scored EIVE concept `Festuca lemanii`.

`Festuca ovina subsp. guestfalica` is consistent between namespaces (both
treat `guestfalica` as an infraspecific rank under `ovina`, not — as the task
brief's naive hypothesis suggested — as a stand-alone species `Festuca
guestfalica`); that part of the group is *not* a divergence and is reported
honestly as such.

*Thymus praecox* was checked as the second, more divergence-prone genus named
in §D.4: EIVE and ESy align closely here (`Thymus praecox`, `subsp. jankae`,
`subsp. ligusticus`, `subsp. polytrichus`, `subsp. widderi` all present on
both sides at the same rank), except that ESy additionally folds a top-level
segregate name `Thymus jankae` (and its varieties) into `Thymus praecox`,
while EIVE has no independent `Thymus jankae` row at all (only the
subspecies-rank `Thymus praecox subsp. jankae`) — a milder version of the
same aggregation-boundary pattern seen in *Festuca*, but not as clear-cut a
divergence since EIVE simply lacks that name rather than scoring it
separately.

## What this means for hostus

1. **Trait values must be stored keyed to (namespace, name-string), not to a
   name-string alone.** The same string `"Festuca ovina"` means "this species
   as EIVE/Euro+Med delimits it" in one table and "this species *plus*
   `F. lemanii`, `F. ovina aggr.`, `F. guestfalica` hybrids, etc." in the
   other. Storing one flat `traits[taxon_name]` map and populating it from
   both EIVE and ESy/FloraVeg-derived sources would silently corrupt data for
   exactly the narrow-niche Sandtrockenrasen segregates (*Festuca*, *Thymus*)
   the spec calls out as most trait-sensitive.
2. **A cross-namespace join needs an explicit aggregation/synonymy table**,
   not a bare-name equality join — confirming the P6 finding that none of
   EIVE/Tichý/Midolo carry a resolvable external ID, compounded here by the
   discovery that even where names look identical, their *concept scope*
   differs between namespaces.
3. **Two independently-served endpoints/branches per spec §4.4 remain the
   correct architecture**: hostus 2.0 should expose FloraVeg/ESy-side and
   EIVE/Euro+Med-side trait/classification data through clearly labelled,
   non-merged surfaces, and any future `POST /v1/translate`-style
   concept-mapping feature (SP5, PoC P8's Wisskirchen linkage) must model this
   as an explicit synonymy relation (potentially many-to-one, as seen with
   `Festuca lemanii` → `Festuca ovina`), not a name-string join.

## Verdict

**🟢 (namespaces genuinely diverge — two-namespace design justified)**

A concrete, reproducible divergence was found in the exact genus (*Festuca*)
the spec's own risk note names: `Festuca lemanii` is an independent
trait-bearing concept in EIVE/Euro+Med but a synonym folded into `Festuca
ovina` in EUNIS-ESy/FloraVeg. `Thymus praecox` shows a milder version of the
same pattern. This confirms the spec's §4.4/§D.4 concern is not theoretical:
keeping the two namespaces distinct (rather than merging on bare name
strings) is necessary to avoid mis-attributing trait data for exactly the
narrow-niche taxa where it matters most.
