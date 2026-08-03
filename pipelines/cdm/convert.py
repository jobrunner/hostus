#!/usr/bin/env python3
"""crawl.py's raw harvest -> the two canonical CDM CSVs + the run summary.

CANONICAL CONTRACT 1 -- cdm-concepts-canonical.csv (pipe-delimited):
    concept_uuid|scientific_name|authorship|rank|status|sec_uuid|sec_title|
    classification_uuid|parent_uuid

CANONICAL CONTRACT 2 -- cdm-relations-canonical.csv (pipe-delimited):
    from_uuid|to_uuid|relation_type|relation_symbol|is_concept_relation|
    relationship_uuid

`relation_type` and `rank` carry the RAW CDM vocabulary
("Congruent to", "Included in or Includes or Overlaps", "Species",
"Subspecies", ...). They are deliberately NOT mapped onto hostus's
vocabulary here. Mapping is a domain decision, it belongs in SP5 Task 3
where it is testable and where an unknown value fails loudly. This is the
`domain.ParseRank` lesson: SP1 assumed 6 ranks, WCVP had 34, and the full
ingest aborted after 5.4 s.

RELATION RESOLUTION -- the global edge map, per docs/research/cdm-sample.md:

  P8 found a relation's partner among the concepts sharing the same accepted
  name. Task 1 measured that at 75.9%, with the failures overwhelmingly genus
  transfers (Coronilla varia = Securigera varia) where the partner cannot
  share the name. The name restriction is therefore DROPPED. Instead every
  relationship uuid is looked up in one global map over the WHOLE crawl:

      from_holders[rel_uuid]  -- concepts listing rel_uuid in phase A's
                                 relationsFromThisTaxon (the `from` end)
      to_holders[rel_uuid]    -- concepts listing rel_uuid in phase B's
                                 relationsToThisTaxon (the `to` end)

  resolved  = exactly one `from` and exactly one `to`
  dangling  = exactly one holder in total (partner not crawled / not in the
              /taxon listing at all)
  ambiguous = two or more holders on the SAME side

BINDING FALSIFIER (docs/research/cdm-sample.md 2, and the task brief):

  Task 1's ~100% resolution figure is a PROJECTION resting on one premise --
  a relationship uuid is a binary edge identity. Measured support: 602 uuids
  over 782 concepts, holder histogram {1: 346, 2: 256}; of the 256 two-holder
  uuids, 202 had direction on both ends and all 202 split into exactly one
  `from` and one `to`, zero anomalies. The premise must be able to FAIL:

    * if ANY relationship uuid acquires a THIRD holder, this script exits
      non-zero and writes no CSV. The uuid is then not an edge identity and
      the whole resolution model has to be rethought.
    * the residual ONE-HOLDER count is always reported. On a full crawl it
      must go towards zero; if it does not, there are relations pointing at
      concepts outside the /taxon listing and /translate's completeness must
      be capped accordingly.

`classification_uuid` comes from the hand-curated, uuid-keyed CROSSWALK in
common.py, gated by assert_crosswalk(). A concept's
`taxonNodes[].classification.uuid` is machine-readable but answers a
different question (tree PLACEMENT, not `sec.` reference space) -- the
agreement rate between the two is printed as a diagnostic.
"""

import argparse
import csv
import os
import sys
from collections import Counter, defaultdict

from common import CROSSWALK, assert_crosswalk, cached_gz, read_ndjson

CONCEPTS_HEADER = ["concept_uuid", "scientific_name", "authorship", "rank",
                   "status", "sec_uuid", "sec_title", "classification_uuid",
                   "parent_uuid"]
RELATIONS_HEADER = ["from_uuid", "to_uuid", "relation_type", "relation_symbol",
                    "is_concept_relation", "relationship_uuid"]

# The reference concept P8 and Task 1 both used as their anchor.
REFERENCE_CONCEPT = "872088a4-95f4-472c-ae79-a29028bb3fbf"

# The six relation types Task 1 measured on its 500-concept stratified sample
# (docs/research/cdm-sample.md 4). Kept only to flag values the sample never
# saw -- nothing here is a mapping, and nothing is filtered on it.
TASK1_RELATION_TYPES = {
    "Congruent to",
    "Includes",
    "Overlaps",
    "Included in or Includes or Overlaps",
    "is pro parte synonym for",
    "is misapplied name for",
}


def _clean(value):
    """Normalise one field value before it goes to csv.writer.

    THE REAL CSV CONTRACT, so nobody downstream guesses wrong: both files are
    written by `csv.writer(delimiter="|")` with Python's default
    QUOTE_MINIMAL, i.e. RFC-4180 quoting with `"` as the quote character. A
    field containing a double quote IS quoted and its quotes ARE doubled: the
    name `Achillea millefolium "Sammelart"` is emitted as the field
    `<Q>Achillea millefolium <Q><Q>Sammelart<Q><Q><Q>` where `<Q>` is one
    double-quote character (spelled out because a literal example cannot be
    written inside this docstring).

    237 of the 51466 concepts hit this. A consumer must therefore use a real
    CSV reader configured with `Comma = '|'` (Go: `encoding/csv` with
    `r.Comma = '|'`) -- **never** `strings.Split(line, "|")`. Same convention
    as pipelines/wikidata/convert.py.

    What this function does is narrower than "escaping":
      * newlines and carriage returns become spaces (they would otherwise be
        quoted into multi-line records, which is legal CSV but awkward), and
      * a literal `|` is replaced by `/`. That substitution is LOSSY
        CORRUPTION, not escaping -- the original character is gone. It is
        latent belt-and-braces only: 0 of the 51466 concepts and 0 relation
        fields contain a `|` today. If that ever changes, the right fix is to
        delete this substitution and rely on the quoting the writer already
        does, not to keep silently mangling values.
    """
    return (value or "").replace("|", "/").replace("\n", " ").replace(
        "\r", " ").strip()


def load_concepts_deduped(cache):
    """Concepts from phase A, de-duplicated on `concept_uuid`.

    Phase A checkpoints per PAGE, after the whole page has been distilled. A
    kill 600 records into a 1000-record page therefore leaves the checkpoint
    at the previous page, and the resumed run replays that page in full --
    `rewrite_clean()` only drops the one torn trailing line, so ~599 records
    are appended a second time. Phases B and C cannot do this because their
    replay is guarded by the SET of units already present; phase A is the odd
    one out.

    Left un-deduplicated, that would put duplicate `concept_uuid` primary
    keys into the canonical CSV and inflate `concepts_fetched`. Relations are
    unaffected (`from_holders` is a set), so the falsifier can never be
    tripped by a phase-A replay.

    Deduplicating here rather than in the crawler is deliberate: the crawler
    must stay append-only and cheap, and a running crawl must never need to
    be restarted to pick up this fix. First record for a uuid wins; the
    replay writes identical content, so which one wins does not matter.
    """
    seen = {}
    duplicates = 0
    for rec in read_ndjson(os.path.join(cache, "concepts.ndjson")):
        if rec["u"] in seen:
            duplicates += 1
            continue
        seen[rec["u"]] = rec
    return list(seen.values()), duplicates


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("cache_dir")
    ap.add_argument("out_dir")
    args = ap.parse_args()
    cache, outdir = args.cache_dir, args.out_dir
    os.makedirs(outdir, exist_ok=True)

    concepts, dup_concepts = load_concepts_deduped(cache)
    outgoing = list(read_ndjson(os.path.join(cache, "outgoing.ndjson")))
    incoming = list(read_ndjson(os.path.join(cache, "incoming.ndjson")))
    nodes = list(read_ndjson(os.path.join(cache, "tree_nodes.ndjson")))

    print("source=%s (/portal/taxon listing + /taxon/{u}/relationsToThisTaxon"
          " + classification tree walk)"
          % "https://api.cybertaxonomy.org/rl_standardliste")
    print("license=NONE FOUND anywhere (portal, API, payloads) "
          "redistribution=unknown -- local evaluation only")
    print("concepts_fetched=%d" % len(concepts))
    print("duplicate_concept_records_dropped=%d  (phase A checkpoints per "
          "page, so an interrupted page replays in full on resume; deduped "
          "on concept_uuid here)" % dup_concepts)
    print("concepts_with_incoming_lookup=%d" % len(incoming))

    # ------------------------------------------------------ crosswalk gate
    cls = cached_gz(cache, "classification.json.gz", "/classification")
    cls_recs = cls.get("records") if isinstance(cls, dict) else cls
    cls_titles = {c["uuid"]: c.get("titleCache", "") for c in cls_recs}
    print("classifications=%d" % len(cls_titles))
    print(assert_crosswalk(cls_titles))

    # ------------------------------------------------------ tree -> parents
    node_by_uuid = {n["n"]: n for n in nodes}
    parent_taxon = {}
    # A concept can sit in several trees and each node carries its own
    # taxonStatus. Collect ALL of them and resolve deterministically
    # (lexicographically lowest) rather than first-wins: the tree walk's
    # traversal order depends on how the run was interrupted, and first-wins
    # would make the output non-reproducible byte-for-byte. Only "Accepted"
    # occurs today; a conflict is counted and reported, not hidden.
    node_statuses = defaultdict(set)
    for node in nodes:
        parent = node_by_uuid.get(node["p"])
        if parent and parent.get("t"):
            parent_taxon[node["n"]] = parent["t"]
        if node.get("st"):
            node_statuses[node["t"]].add(node["st"])
    node_status = {t: sorted(s)[0] for t, s in node_statuses.items()}
    status_conflicts = sorted(t for t, s in node_statuses.items()
                              if len(s) > 1)
    print("tree_nodes=%d nodes_with_known_parent=%d"
          % (len(nodes), len(parent_taxon)))
    if status_conflicts:
        print("!! %d concept(s) carry DIFFERENT taxonStatus values in "
              "different classifications; the lexicographically lowest is "
              "emitted. First few: %s"
              % (len(status_conflicts),
                 ", ".join("%s=%s" % (t, sorted(node_statuses[t]))
                           for t in status_conflicts[:5])))

    # -------------------------------------------------- global edge map
    # Built BEFORE anything is written, so the falsifier below can refuse to
    # emit any CSV at all if the edge-identity premise breaks.
    from_holders = defaultdict(set)
    edge_meta = {}
    for row in outgoing:
        from_holders[row["r"]].add(row["f"])
        edge_meta.setdefault(row["r"], row)
    to_holders = defaultdict(set)
    for row in incoming:
        for ruuid in row["r"]:
            to_holders[ruuid].add(row["t"])

    all_uuids = set(from_holders) | set(to_holders)
    holders = {u: (from_holders.get(u, set()) | to_holders.get(u, set()))
               for u in all_uuids}
    hist = Counter(len(v) for v in holders.values())
    print("relations_found=%d" % len(all_uuids))
    print("holders_per_relationship_uuid=" + ", ".join(
        "%d:%d" % (k, hist[k]) for k in sorted(hist)))

    # ------------------------------------------------ BINDING FALSIFIER
    third = sorted(u for u, hs in holders.items() if len(hs) >= 3)
    if third:
        print("\n!! FALSIFIED: %d relationship uuid(s) acquired a THIRD "
              "holder. A relationship uuid is therefore NOT a binary edge "
              "identity and the whole resolution model of "
              "docs/research/cdm-sample.md 2 must be rethought. No CSV "
              "written." % len(third), file=sys.stderr)
        for u in third[:10]:
            print("   %s holders=%s" % (u, sorted(holders[u])),
                  file=sys.stderr)
        return 3
    print("falsifier=PASS no relationship uuid acquired a third holder "
          "(max holders seen: %d)" % (max(hist) if hist else 0))

    # ------------------------------------------------------- concepts CSV
    concepts_path = os.path.join(outdir, "cdm-concepts-canonical.csv")
    cw_hits = 0
    cls_agree = cls_disagree = cls_nonode = 0
    multi_node = 0
    with_parent = 0
    doubtful_count = 0
    foreign_node_cls = 0
    rank_counts = Counter()
    status_counts = Counter()
    tmp = concepts_path + ".tmp"
    with open(tmp, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh, delimiter="|", lineterminator="\n")
        writer.writerow(CONCEPTS_HEADER)
        for rec in concepts:
            cls_uuid = CROSSWALK.get(rec["sec_title"], "")
            if cls_uuid:
                cw_hits += 1
            node_cls = {n["c"] for n in rec["nodes"] if n["c"]}
            if len(rec["nodes"]) > 1:
                multi_node += 1
            # Diagnostic, deliberately narrow: does the crosswalked sec.
            # space appear AMONG the concept's tree placements at all?
            # It is NOT a check that tree placement equals the sec. space --
            # a concept placed in both W&H and FloraWeb counts as "among"
            # for whichever of the two the crosswalk picked, which is exactly
            # why "not_among" is near zero and why tree placement still
            # cannot be used as the sec. space. `also_in_foreign_cls` below
            # measures that second phenomenon separately.
            if not node_cls:
                cls_nonode += 1
            elif cls_uuid and cls_uuid in node_cls:
                cls_agree += 1
            elif cls_uuid:
                cls_disagree += 1
            if cls_uuid and node_cls - {cls_uuid}:
                foreign_node_cls += 1

            # One parent column, several possible tree placements: prefer the
            # node sitting in the crosswalked sec. space, else the first node
            # in a stable (uuid-sorted) order. Deterministic either way.
            chosen = ""
            ordered = sorted(rec["nodes"], key=lambda n: n["n"])
            for node in ordered:
                if cls_uuid and node["c"] == cls_uuid:
                    chosen = node["n"]
                    break
            if not chosen and ordered:
                chosen = ordered[0]["n"]
            parent = parent_taxon.get(chosen, "")
            if parent:
                with_parent += 1

            # `status` carries ONLY the raw TaxonNodeDto.taxonStatus, and is
            # EMPTY where the tree walk has not reached the concept. An
            # earlier version synthesised "Doubtful" from the CDM boolean and
            # fell back to "Accepted" for everything else -- which meant
            # 51464 of 51466 rows asserted a status that was never measured,
            # in a CSV whose whole contract is "raw vocabulary, mapping
            # happens in Task 3". Empty is honest; a downstream reader can
            # tell "not observed" from "observed accepted".
            #
            # The CDM `Taxon.doubtful` boolean is a DIFFERENT field, not a
            # taxonStatus value, so it is not folded in here. It is reported
            # as its own summary counter below so the information is not lost.
            status = node_status.get(rec["u"], "")
            rank_counts[rec["rank"] or "(empty)"] += 1
            status_counts[status or "(not observed)"] += 1
            if rec["doubtful"]:
                doubtful_count += 1

            writer.writerow([
                _clean(rec["u"]), _clean(rec["name"]), _clean(rec["auth"]),
                _clean(rec["rank"]), _clean(status), _clean(rec["sec_uuid"]),
                _clean(rec["sec_title"]), _clean(cls_uuid), _clean(parent),
            ])
    os.replace(tmp, concepts_path)
    total = len(concepts) or 1
    print("concepts_csv=%s rows=%d" % (concepts_path, len(concepts)))
    print("sec_mapped_via_crosswalk=%d/%d = %.1f%%"
          % (cw_hits, len(concepts), 100.0 * cw_hits / total))
    print("concepts_with_parent_uuid=%d/%d = %.1f%%"
          % (with_parent, len(concepts), 100.0 * with_parent / total))
    print("concepts_with_multiple_taxon_nodes=%d" % multi_node)
    print("sec_space_among_taxon_node_classifications: among=%d not_among=%d "
          "no_node_yet=%d  (diagnostic only, and a WEAK one: it asks whether "
          "the crosswalked sec. space is one of the concept's tree "
          "placements, NOT whether placement equals the sec. space)"
          % (cls_agree, cls_disagree, cls_nonode))
    print("concepts_also_placed_in_a_FOREIGN_classification=%d  (this is why "
          "taxonNodes cannot be used as the sec. space; the crosswalk "
          "decides)" % foreign_node_cls)
    print("rank_distribution(raw CDM)=" + ", ".join(
        "%s:%d" % kv for kv in rank_counts.most_common()))
    print("status_distribution=" + ", ".join(
        "%s:%d" % kv for kv in status_counts.most_common())
        + "  (raw TaxonNodeDto.taxonStatus where the tree walk reached the "
          "concept, empty otherwise -- nothing is synthesised)")
    print("doubtful_concepts=%d  (CDM Taxon.doubtful boolean -- a DIFFERENT "
          "field, deliberately NOT folded into the status column)"
          % doubtful_count)

    # ------------------------------------------------------ relations CSV
    resolved = ambiguous = dangling = 0
    orphan_to = 0
    type_counts = Counter()
    resolved_type_counts = Counter()
    relations_path = os.path.join(outdir, "cdm-relations-canonical.csv")
    tmp = relations_path + ".tmp"
    with open(tmp, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh, delimiter="|", lineterminator="\n")
        writer.writerow(RELATIONS_HEADER)
        for ruuid in sorted(all_uuids):
            froms = sorted(from_holders.get(ruuid, set()))
            tos = sorted(to_holders.get(ruuid, set()))
            meta = edge_meta.get(ruuid)
            rtype = meta["t"] if meta else ""
            symbol = meta["s"] if meta else ""
            # Empty, not "false", when the edge was only ever seen from its
            # `to` side: phase A is the only source of the type object, so
            # without it `conceptRelationship` is UNKNOWN. Emitting "false"
            # would report an unobserved value as a measurement -- and
            # "false" is meaningful here (it is what marks a misapplied name
            # as not belonging in the concept-relation table at all).
            if meta is None:
                is_cr = ""
            else:
                is_cr = "true" if meta["cr"] else "false"
            type_counts[rtype or "(unknown - to-end only)"] += 1
            if len(froms) > 1 or len(tos) > 1:
                ambiguous += 1
            elif froms and tos:
                resolved += 1
                resolved_type_counts[rtype] += 1
            else:
                dangling += 1
                if not froms:
                    orphan_to += 1
            writer.writerow([
                _clean(froms[0] if len(froms) == 1 else ""),
                _clean(tos[0] if len(tos) == 1 else ""),
                _clean(rtype), _clean(symbol), is_cr, _clean(ruuid),
            ])
    os.replace(tmp, relations_path)
    print("relations_csv=%s rows=%d" % (relations_path, len(all_uuids)))

    tot = len(all_uuids) or 1
    print("resolved=%d/%d = %.1f%%" % (resolved, len(all_uuids),
                                       100.0 * resolved / tot))
    print("ambiguous=%d/%d = %.1f%%" % (ambiguous, len(all_uuids),
                                        100.0 * ambiguous / tot))
    print("dangling=%d/%d = %.1f%%" % (dangling, len(all_uuids),
                                       100.0 * dangling / tot))
    print("residual_one_holder_uuids=%d  (must go towards zero on a FULL "
          "crawl; a stubborn remainder means relations point at concepts "
          "outside the /taxon listing and /translate completeness must be "
          "capped)" % hist.get(1, 0))
    print("orphan_to_end_only=%d  (edge seen only from its `to` side -- its "
          "`from` concept is not in the /portal/taxon listing)" % orphan_to)
    print("relation_type_distribution(raw CDM)=" + ", ".join(
        "%s:%d" % kv for kv in type_counts.most_common()))
    # Task 1 measured SIX relation types on a 500-concept stratified sample.
    # Anything beyond those is a value the sample never saw; it is reported
    # here, NOT mapped, so Task 3's mapper is confronted with it and can fail
    # loudly on it. Same failure mode as domain.ParseRank (6 assumed ranks,
    # 34 real ones, ingest aborted after 5.4 s).
    unseen = sorted(set(type_counts) - TASK1_RELATION_TYPES)
    if unseen:
        print("!! relation types NOT seen in Task 1's sample (new at full "
              "scope, must be handled by Task 3's mapper): "
              + ", ".join("%s:%d" % (u, type_counts[u]) for u in unseen))
    else:
        print("relation_types_beyond_task1_sample=none")
    print("resolved_relation_type_distribution(raw CDM)=" + ", ".join(
        "%s:%d" % kv for kv in resolved_type_counts.most_common()))

    # ------------------------------------------------- reference concept
    ref = [c for c in concepts if c["u"] == REFERENCE_CONCEPT]
    if ref:
        rec = ref[0]
        print("reference_concept=%s %s %s | rank=%s | sec=%s | cls=%s"
              % (REFERENCE_CONCEPT, rec["name"], rec["auth"], rec["rank"],
                 rec["sec_title"], CROSSWALK.get(rec["sec_title"], "-")))
        by_uuid = {c["u"]: c for c in concepts}

        def label(uuid):
            other = by_uuid.get(uuid)
            if not other:
                return uuid
            return "%s %s sec. %s" % (other["name"], other["auth"],
                                      other["sec_title"][:38])

        out_rows = [r for r in outgoing if r["f"] == REFERENCE_CONCEPT]
        in_uuids = [u for r in incoming if r["t"] == REFERENCE_CONCEPT
                    for u in r["r"]]
        print("reference_concept_relations: from=%d to=%d"
              % (len(out_rows), len(in_uuids)))
        for row in out_rows:
            partner = sorted(to_holders.get(row["r"], set()))
            print("  --%s(%s)--> %s" % (row["t"], row["s"],
                                        label(partner[0]) if partner
                                        else "(partner not yet crawled)"))
        for ruuid in in_uuids:
            partner = sorted(from_holders.get(ruuid, set()))
            meta = edge_meta.get(ruuid)
            print("  <--%s(%s)-- %s"
                  % (meta["t"] if meta else "?", meta["s"] if meta else "?",
                     label(partner[0]) if partner
                     else "(partner not yet crawled)"))
    else:
        print("reference_concept=NOT PRESENT in this slice (%s)"
              % REFERENCE_CONCEPT)
    return 0


if __name__ == "__main__":
    sys.exit(main())
