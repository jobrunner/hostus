#!/usr/bin/env python3
"""Merge crawl.py's raw NDJSON harvest into the canonical xref CSV + summary.

Canonical CSV contract (pipe-delimited, one row per (item x authority)):
    join_authority|join_id|authority|ext_id|wikidata_qid

- join_authority is always `powo` -- the key hostus's existing `xref` table
  already carries (the WCVP `dynamicproperties.powoid`, which IS the bare
  IPNI id, e.g. `396681-1`).
- join_id is that bare IPNI id, taken from P961 directly (already bare) or
  from P5037 with its `urn:lsid:ipni.org:names:` LSID prefix stripped. When
  BOTH are present and (after stripping) DISAGREE, P961 wins (it needs no
  string surgery to reach the bare form, and is therefore the more direct
  match against `xref.powo`) -- but every such disagreement is counted and
  reported, never silently dropped. See pipelines/README.md.
- authority maps onto hostus's existing `xref.authority` vocabulary:
      P846/P14607 (GBIF legacy/new) -> gbif   (P14607 preferred when both
                                                present, per PoC P7's
                                                "migration in progress"
                                                finding -- both are queried,
                                                the newer one wins)
      P10585 (Catalogue of Life)    -> colxr
      P12380 (Euro+Med PlantBase)   -> euromed
      P12100 (FloraVeg.EU)          -> floraveg  *** NAME, not an id -- see
                                                     caveat below ***
      P7715  (World Flora Online)   -> wfo
      P3151  (iNaturalist)          -> inat
  A `wikidata` row (ext_id = the QID itself) is always emitted for every
  item, so a consumer can resolve back to the source item.
- P12100/FloraVeg.EU is a NAME STRING (e.g. "Corynephorus canescens"), not
  an opaque identifier like every other authority here. It is still emitted
  (best-effort enrichment), but a consumer must not treat it as joinable by
  id the way gbif/colxr/wfo/inat ids are -- flagged loudly here and in
  pipelines/README.md so nobody downstream assumes otherwise.
"""
import csv
import json
import sys

from common import resolve_join_id, strip_lsid

# Wikidata property -> hostus xref.authority, in preference order for GBIF
# (P14607 "new" wins over P846 "legacy" when both are present).
GBIF_PROPS = ["P14607", "P846"]
SIMPLE_AUTHORITY_MAP = {
    "P10585": "colxr",
    "P12380": "euromed",
    "P12100": "floraveg",
    "P7715": "wfo",
    "P3151": "inat",
}


def load_seed(cache_dir):
    seed = {}
    for prop, key in (("P961", "p961"), ("P5037", "p5037")):
        try:
            with open(f"{cache_dir}/seed_{prop}.ndjson", encoding="utf-8") as f:
                for line in f:
                    rec = json.loads(line)
                    seed.setdefault(rec["item"], {})[key] = rec["v"]
        except FileNotFoundError:
            pass
    return seed


def load_enriched(cache_dir):
    enriched = {}
    try:
        with open(f"{cache_dir}/enriched.ndjson", encoding="utf-8") as f:
            for line in f:
                rec = json.loads(line)
                enriched[rec.pop("item")] = rec
    except FileNotFoundError:
        pass
    return enriched


def load_joinable_ids(path):
    """The set of `xref.powo` ext_ids from the real concept DB (bare IPNI
    ids), used to restrict the harvest to items our current index can
    actually join against -- see build.sh / pipelines/README.md for why."""
    if not path:
        return None
    with open(path, encoding="utf-8") as f:
        return {line.strip() for line in f if line.strip()}


def main():
    cache_dir, out_path = sys.argv[1:3]
    joinable_ids_path = sys.argv[3] if len(sys.argv) > 3 else None

    seed = load_seed(cache_dir)
    enriched = load_enriched(cache_dir)
    joinable_ids = load_joinable_ids(joinable_ids_path)

    rows_written = 0
    total_items = 0
    items_with_both = 0
    disagreements = 0
    disagreement_items = []
    populated = {
        "wikidata": 0,
        "gbif": 0,
        "colxr": 0,
        "euromed": 0,
        "floraveg": 0,
        "wfo": 0,
        "inat": 0,
    }
    raw_property_counts = {
        "P961": 0,
        "P5037": 0,
        "P846": 0,
        "P14607": 0,
        "P10585": 0,
        "P12380": 0,
        "P12100": 0,
        "P7715": 0,
        "P3151": 0,
    }

    with open(out_path, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f, delimiter="|")
        w.writerow(["join_authority", "join_id", "authority", "ext_id", "wikidata_qid"])

        for qid, s in seed.items():
            p961 = s.get("p961")
            p5037_raw = s.get("p5037")

            join_id, disagreed = resolve_join_id(p961, p5037_raw, joinable_ids)
            if join_id is None:
                # Cannot happen: seed set is defined as P961-or-P5037 holders.
                continue

            if joinable_ids is not None:
                # "Joinable" means EITHER raw value matches -- checked
                # independently of which one resolve_join_id ultimately
                # picked as join_id, so a P961-vs-P5037 disagreement where
                # only the P5037 side matches our concept table still
                # counts (and, per the fix-round-1 correction, resolve_join_id
                # now emits that matching P5037 value rather than the dead
                # P961 one). See crawl.py's target_qids() docstring.
                p5037_bare = strip_lsid(p5037_raw)
                if not ((p961 and p961 in joinable_ids) or (p5037_bare and p5037_bare in joinable_ids)):
                    continue  # not joinable against our current concept index

            if p961:
                raw_property_counts["P961"] += 1
            if p5037_raw:
                raw_property_counts["P5037"] += 1
            if p961 and p5037_raw:
                items_with_both += 1
            if disagreed:
                disagreements += 1
                if len(disagreement_items) < 20:
                    disagreement_items.append((qid, p961, strip_lsid(p5037_raw)))

            total_items += 1

            def emit(authority, ext_id):
                nonlocal rows_written
                w.writerow(["powo", join_id, authority, ext_id, qid])
                rows_written += 1
                populated[authority] += 1

            emit("wikidata", qid)

            e = enriched.get(qid, {})

            for prop in GBIF_PROPS:
                if e.get(prop):
                    raw_property_counts[prop] += 1
            gbif_val = None
            for prop in GBIF_PROPS:  # P14607 preferred over P846
                if e.get(prop):
                    gbif_val = e[prop]
                    break
            if gbif_val:
                emit("gbif", gbif_val)

            for prop, authority in SIMPLE_AUTHORITY_MAP.items():
                val = e.get(prop)
                if val:
                    raw_property_counts[prop] += 1
                    emit(authority, val)

    print(f"seed_union_total={len(seed)}")
    if joinable_ids is not None:
        print(f"joinable_ids_total={len(joinable_ids)}")
    print(f"total_items={total_items} rows={rows_written}")
    print(
        "populated="
        + ",".join(f"{k}:{v}" for k, v in populated.items())
    )
    if total_items:
        print(
            "fill_rate_pct="
            + ",".join(f"{k}:{v / total_items * 100:.1f}" for k, v in populated.items())
        )
    print(
        "raw_property_counts="
        + ",".join(f"{k}:{v}" for k, v in raw_property_counts.items())
    )
    pct = (disagreements / items_with_both * 100) if items_with_both else 0.0
    print(
        f"p961_p5037_disagreements={disagreements} of items_with_both={items_with_both} "
        f"({pct:.3f}% of items carrying both P961 and P5037)"
    )
    for qid, p961, p5037_bare in disagreement_items:
        print(f"  disagreement sample: {qid} P961={p961} P5037(stripped)={p5037_bare}")


if __name__ == "__main__":
    main()
