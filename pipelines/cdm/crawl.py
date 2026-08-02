#!/usr/bin/env python3
"""CDM `rl_standardliste` -> raw concept + relation harvest. Three phases.

    phase A  concepts   52 requests   /portal/taxon?pageSize=1000&pageIndex=N
    phase C  tree       ~n_internal   /classification/{c}/childNodes,
                                      /taxonNode/{n}/childNodes
    phase B  to-ends    51466         /taxon/{u}/relationsToThisTaxon

WHY THIS SHAPE (measured live while building this pipeline, 2026-08-02 --
it is CHEAPER than the shape Task 1 costed, and strictly more informative):

Task 1 costed the full crawl as `/portal/taxon/{uuid}/taxonRelationships` for
all 51466 concepts plus a `relationsToThisTaxon` direction lookup for the
~55% that have relations -- roughly 80000 requests, 22-30 h. That was the
right plan against the endpoints Task 1 knew about. Probing further turned up
a better one: the FLAT PORTAL LISTING `/portal/taxon?pageSize=1000&pageIndex=N`
carries, inline, per concept:

  * `name.nameCache` / `name.titleCache` (scientific name + authorship),
  * `name.rank.representation_L10n` (the RAW CDM rank vocabulary),
  * `secSource.citation.{uuid,titleCache}` (Task 1's finding, confirmed),
  * `taxonNodes[]` (node uuid + the tree's classification uuid),
  * and `relationsFromThisTaxon[]` -- every OUTGOING relation with its
    relationship uuid, type label, symbol and `conceptRelationship` flag.

Measured on page 0 (1000 concepts): 492 distinct relationship uuids, holder
histogram {1: 492} -- i.e. the listing gives each edge exactly ONCE, at its
`from` end. So 52 requests replace 51466 relationship requests AND supply the
direction for free; only the `to` end still costs one request per concept.

New budget: 52 + ~n_internal + 51466 requests. At Task 1's measured
max(1s, latency) = 1.139 s/request that is ~16-19 h, inside Task 1's 22-30 h
envelope rather than beyond it. Nothing is given up: relation type, symbol,
`conceptRelationship` and direction all come from phase A.

Phase C exists only because the canonical concepts CSV has a `parent_uuid`
column. There is no bulk taxon-node endpoint (`/taxonNode?pageSize=..` -> 404,
`/checklist/export` -> `records: []` on every page, still, as in P8), so the
18 classification trees are walked: one request per node that HAS children.
Leaves cost nothing -- they arrive inside their parent's response.

Resumability (a 16-20 h run WILL be interrupted):
  * phase A caches each raw page as gzipped JSON, written to a temp file and
    renamed, so a kill can never leave a half-written page that reads back as
    truth. Distillation into concepts/outgoing NDJSON is checkpointed by page
    count.
  * phase B and C append one flushed NDJSON line per unit of work and resume
    from the SET of units already present in that log, not from a positional
    offset. A truncated trailing line (kill between write and flush) is
    dropped on read and the unit is simply re-fetched.
  * re-running is therefore free and cannot corrupt partial state.

Crawl etiquette lives in common.py and is not optional: one honest
User-Agent, >= 1s between request starts, single threaded, backoff on
429/5xx, hard stop (never a browser UA) if the honest UA is refused.
"""

import argparse
import os
import sys
import time

from common import (Progress, Refused, append_ndjson, cached_gz, get_json,
                    read_ndjson, request_stats, rewrite_clean)

PAGE_SIZE = 1000
NODE_PAGE_SIZE = 1000


# --------------------------------------------------------------- phase A

def _distil_concept(rec):
    name = rec.get("name") or {}
    name_cache = (name.get("nameCache") or "").strip()
    title = (name.get("titleCache") or "").strip()
    authorship = (name.get("authorshipCache") or "").strip()
    if not authorship and name_cache and title.startswith(name_cache):
        authorship = title[len(name_cache):].strip()
    if not name_cache:
        name_cache = title
    rank = ((name.get("rank") or {}).get("representation_L10n") or "").strip()
    secsrc = rec.get("secSource") or {}
    cit = (secsrc.get("citation") or {}) if secsrc else {}
    nodes = []
    for node in (rec.get("taxonNodes") or []):
        nodes.append({
            "n": node.get("uuid") or "",
            "c": ((node.get("classification") or {}).get("uuid") or ""),
        })
    return {
        "u": rec["uuid"],
        "name": name_cache,
        "auth": authorship,
        "rank": rank,
        "doubtful": bool(rec.get("doubtful")),
        "sec_uuid": cit.get("uuid") or "",
        "sec_title": cit.get("titleCache") or "",
        "nodes": nodes,
    }


def _distil_outgoing(rec):
    out = []
    for rel in (rec.get("relationsFromThisTaxon") or []):
        rtype = rel.get("type") or {}
        out.append({
            "f": rec["uuid"],
            "r": rel.get("uuid") or "",
            "t": rtype.get("representation_L10n") or "",
            "s": rtype.get("symbol") or "",
            "cr": bool(rtype.get("conceptRelationship")),
        })
    return out


def phase_a(cache, deadline=None, max_pages=None):
    """Page the flat portal listing. 52 requests for the whole dataset."""
    concepts_path = os.path.join(cache, "concepts.ndjson")
    outgoing_path = os.path.join(cache, "outgoing.ndjson")
    offset_path = os.path.join(cache, "phase_a.pages_done")

    done_pages = 0
    if os.path.exists(offset_path):
        with open(offset_path, encoding="utf-8") as fh:
            raw = fh.read().strip() or "0"
        if raw == "-1":  # sentinel: the listing was paged to its end
            print("phase A: already complete", flush=True)
            return True
        done_pages = int(raw)

    page = done_pages
    pages_available = None
    started = time.monotonic()
    if done_pages:
        print("phase A: resuming at page %d" % done_pages, flush=True)

    # Distillation is append-only and page-checkpointed; truncate any partial
    # trailing line before appending so records cannot be glued together.
    rewrite_clean(concepts_path)
    rewrite_clean(outgoing_path)

    with open(concepts_path, "a", encoding="utf-8") as cf, \
            open(outgoing_path, "a", encoding="utf-8") as of:
        while True:
            data = cached_gz(cache, "portal/page_%04d.json.gz" % page,
                             "/portal/taxon?pageSize=%d&pageIndex=%d"
                             % (PAGE_SIZE, page))
            if data is None:
                raise RuntimeError("portal page %d returned 404" % page)
            pages_available = data["pagesAvailable"]
            recs = data.get("records") or []
            for rec in recs:
                append_ndjson(cf, _distil_concept(rec))
                for row in _distil_outgoing(rec):
                    append_ndjson(of, row)
            page += 1
            with open(offset_path, "w", encoding="utf-8") as fh:
                fh.write(str(page))
            print("phase A: page %d/%d (%d concepts, count=%d)"
                  % (page, pages_available, len(recs), data["count"]),
                  flush=True)
            if page >= pages_available or not recs:
                with open(offset_path, "w", encoding="utf-8") as fh:
                    fh.write("-1")
                break
            if max_pages is not None and page >= max_pages:
                print("phase A: page cap %d reached (validation slice)"
                      % max_pages, flush=True)
                break
            if deadline and time.monotonic() >= deadline:
                print("phase A: time budget reached at page %d/%d"
                      % (page, pages_available), flush=True)
                return False
    print("phase A: DONE, %d pages in %.0fs"
          % (page, time.monotonic() - started), flush=True)
    return True


def load_concepts(cache):
    return list(read_ndjson(os.path.join(cache, "concepts.ndjson")))


# --------------------------------------------------------------- phase C

def _node_dto(dto):
    return {
        "n": dto.get("uuid") or "",
        "p": dto.get("parentUUID") or "",
        "t": dto.get("taxonUuid") or "",
        "c": dto.get("classificationUUID") or "",
        "st": dto.get("taxonStatus") or "",
        "kids": int(dto.get("taxonomicChildrenCount") or 0),
    }


def phase_c(cache, max_requests=None, deadline=None):
    """Walk the 18 classification trees to learn every node's parent.

    One request per node that has children; leaves arrive free inside their
    parent's response. Resume is driven by the set of already-expanded node
    uuids in expanded.ndjson, so an interrupt re-fetches nothing.
    """
    nodes_path = os.path.join(cache, "tree_nodes.ndjson")
    expanded_path = os.path.join(cache, "tree_expanded.ndjson")
    rewrite_clean(nodes_path)
    rewrite_clean(expanded_path)

    known = {}
    for rec in read_ndjson(nodes_path):
        known[rec["n"]] = rec
    expanded = {rec["n"] for rec in read_ndjson(expanded_path)}

    cls = get_json("/classification")
    cls_recs = cls.get("records") if isinstance(cls, dict) else cls
    cls_uuids = [c["uuid"] for c in cls_recs]
    print("phase C: %d classifications, %d nodes known, %d expanded"
          % (len(cls_uuids), len(known), len(expanded)), flush=True)

    requests_made = 0

    def budget_left():
        if max_requests is not None and requests_made >= max_requests:
            return False
        if deadline and time.monotonic() >= deadline:
            return False
        return True

    with open(nodes_path, "a", encoding="utf-8") as nf, \
            open(expanded_path, "a", encoding="utf-8") as ef:

        def record(dtos):
            fresh = []
            for dto in dtos:
                rec = _node_dto(dto)
                if not rec["n"] or rec["n"] in known:
                    continue
                known[rec["n"]] = rec
                append_ndjson(nf, rec)
                fresh.append(rec)
            return fresh

        queue = []
        for cuuid in cls_uuids:
            key = "cls:" + cuuid
            if key in expanded:
                continue
            if not budget_left():
                print("phase C: budget reached (roots)", flush=True)
                return False
            roots = get_json("/classification/%s/childNodes" % cuuid) or []
            requests_made += 1
            record(roots if isinstance(roots, list) else roots.get("records") or [])
            append_ndjson(ef, {"n": key})
            expanded.add(key)

        # Anything with children and not yet expanded is work.
        for rec in list(known.values()):
            if rec["kids"] > 0 and rec["n"] not in expanded:
                queue.append(rec["n"])

        prog = Progress("phase C: expanded", len(queue), every=100)
        while queue:
            if not budget_left():
                print("phase C: budget reached, %d nodes still queued"
                      % len(queue), flush=True)
                return False
            nuuid = queue.pop()
            if nuuid in expanded:
                continue
            page = 0
            while True:
                data = get_json("/taxonNode/%s/childNodes?pageSize=%d"
                                "&pageIndex=%d" % (nuuid, NODE_PAGE_SIZE, page))
                requests_made += 1
                if data is None:
                    break
                recs = (data.get("records") if isinstance(data, dict)
                        else data) or []
                fresh = record(recs)
                for rec in fresh:
                    if rec["kids"] > 0:
                        queue.append(rec["n"])
                        prog.total += 1
                if not isinstance(data, dict):
                    break
                page += 1
                if page >= (data.get("pagesAvailable") or 1):
                    break
            append_ndjson(ef, {"n": nuuid})
            expanded.add(nuuid)
            prog.tick()
    prog.tick(0, force=True)
    print("phase C: DONE, %d nodes, %d expanded, %d requests this run"
          % (len(known), len(expanded), requests_made), flush=True)
    return True


# --------------------------------------------------------------- phase B

def phase_b(cache, concept_uuids, deadline=None, max_requests=None):
    """Fetch the incoming relationship uuids for every concept.

    This is the only per-concept phase and therefore the long pole. Resume is
    by set membership in incoming.ndjson, so a re-run re-fetches nothing.
    """
    path = os.path.join(cache, "incoming.ndjson")
    rewrite_clean(path)
    done = {rec["t"] for rec in read_ndjson(path)}
    todo = [u for u in concept_uuids if u not in done]
    print("phase B: %d concepts, %d already done, %d to fetch"
          % (len(concept_uuids), len(concept_uuids) - len(todo), len(todo)),
          flush=True)
    if not todo:
        print("phase B: DONE (nothing to fetch)", flush=True)
        return True

    prog = Progress("phase B:", len(todo), every=200)
    made = 0
    with open(path, "a", encoding="utf-8") as fh:
        for uuid in todo:
            if deadline and time.monotonic() >= deadline:
                print("phase B: time budget reached after %d of %d"
                      % (prog.done, len(todo)), flush=True)
                return False
            if max_requests is not None and made >= max_requests:
                print("phase B: request budget reached after %d of %d"
                      % (prog.done, len(todo)), flush=True)
                return False
            data = get_json("/taxon/%s/relationsToThisTaxon?pageSize=200"
                            % uuid)
            made += 1
            rels = []
            if isinstance(data, dict):
                rels = [r["uuid"] for r in (data.get("records") or [])
                        if r.get("uuid")]
            elif isinstance(data, list):
                rels = [r["uuid"] for r in data if r.get("uuid")]
            append_ndjson(fh, {"t": uuid, "r": rels})
            prog.tick()
    prog.tick(0, force=True)
    print("phase B: DONE", flush=True)
    return True


# --------------------------------------------------------------- poc import

def import_poc_cache(cache, poc_dir):
    """Lift the 308 relationsToThisTaxon responses P8b already cached.

    Zero requests. Used to give the bounded validation run real partner ends
    without asking the server for anything it has already answered.
    """
    src = os.path.join(poc_dir, "to")
    if not os.path.isdir(src):
        print("import: no %s, nothing to lift" % src, flush=True)
        return 0
    import json as _json
    path = os.path.join(cache, "incoming.ndjson")
    rewrite_clean(path)
    done = {rec["t"] for rec in read_ndjson(path)}
    added = 0
    with open(path, "a", encoding="utf-8") as fh:
        for fn in sorted(os.listdir(src)):
            if not fn.endswith(".json"):
                continue
            uuid = fn[:-5]
            if uuid in done:
                continue
            with open(os.path.join(src, fn), encoding="utf-8") as sf:
                data = _json.load(sf)
            rels = []
            if isinstance(data, dict):
                rels = [r["uuid"] for r in (data.get("records") or [])
                        if r.get("uuid")]
            append_ndjson(fh, {"t": uuid, "r": rels})
            added += 1
    print("import: lifted %d cached relationsToThisTaxon responses from %s"
          % (added, src), flush=True)
    return added


# --------------------------------------------------------------------- main

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("cache_dir")
    ap.add_argument("--time-budget", type=float, default=None,
                    help="stop cleanly after N seconds; re-run to continue")
    ap.add_argument("--max-pages", type=int, default=None,
                    help="validation only: stop phase A after N pages")
    ap.add_argument("--max-concepts", type=int, default=None,
                    help="validation only: restrict phases B/C to the first N "
                         "concepts of phase A")
    ap.add_argument("--max-tree-requests", type=int, default=None,
                    help="validation only: cap phase C requests")
    ap.add_argument("--max-incoming-requests", type=int, default=None,
                    help="validation only: cap phase B requests")
    ap.add_argument("--import-poc", default=None,
                    help="validation only: lift poc/p08b_cdm_sample/.cache/to")
    ap.add_argument("--skip-tree", action="store_true",
                    help="skip phase C (parent_uuid stays empty)")
    args = ap.parse_args()

    cache = args.cache_dir
    os.makedirs(cache, exist_ok=True)
    deadline = (time.monotonic() + args.time_budget
                if args.time_budget else None)

    if not phase_a(cache, deadline, args.max_pages):
        print("PHASE=a INCOMPLETE", flush=True)
        return 1

    concepts = load_concepts(cache)
    if args.max_concepts:
        concepts = concepts[:args.max_concepts]
    concept_uuids = [c["u"] for c in concepts]
    print("phase A yielded %d concepts (scope for B/C: %d)"
          % (len(load_concepts(cache)), len(concept_uuids)), flush=True)

    if args.import_poc:
        import_poc_cache(cache, args.import_poc)

    if not args.skip_tree:
        if not phase_c(cache, args.max_tree_requests, deadline):
            print("PHASE=c INCOMPLETE", flush=True)
            return 1

    if not phase_b(cache, concept_uuids, deadline,
                   args.max_incoming_requests):
        print("PHASE=b INCOMPLETE", flush=True)
        return 1

    stats = request_stats()
    print("requests=%d bytes=%d net_seconds=%.0f"
          % (stats["requests"], stats["bytes"], stats["seconds"]), flush=True)
    print("DONE", flush=True)
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Refused as exc:
        sys.stderr.write("\nCRAWL ABORTED: %s\n" % exc)
        sys.exit(2)
    except KeyboardInterrupt:
        sys.stderr.write("\ninterrupted -- progress is on disk, re-run to "
                         "continue\n")
        sys.exit(130)
