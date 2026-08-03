#!/usr/bin/env python3
"""Wikidata (query.wikidata.org/sparql) -> raw xref harvest, two-phase.

BACKGROUND -- why this is two-phase and not one query per page (see
build.sh header / pipelines/README.md for the full writeup):

A single query that both (a) restricts to taxa via `wdt:P31 wd:Q16521`
and (b) attaches several OPTIONAL properties reliably exceeds WDQS's 60s
query timeout once the page is more than a few hundred rows -- measured
live against the real endpoint while developing this pipeline:
  - `?item wdt:P961 ?v ; wdt:P31 wd:Q16521 .` alone (a plain COUNT, no
    OPTIONALs) already costs ~30-45s: the P31=Q16521 join is expensive
    regardless of LIMIT/OFFSET, because Blazegraph has to materialize the
    intersection of two large sets (908799 P961-holders x millions of
    P31=Q16521 items) before it can apply LIMIT.
  - Adding OPTIONAL blocks for the other 7 properties multiplies that
    cost further -- reliably 504/502/truncated-JSON above ~1000 rows/page.
  - By contrast, a *single*-predicate scan with NO join and NO OPTIONALs,
    `SELECT ?item ?v WHERE { ?item wdt:P961 ?v } LIMIT 20000 OFFSET N`,
    consistently completes in 3-25s even at N=800000+ -- WDQS can stream
    a single predicate's index range without a join.
  - And a `VALUES ?item { wd:Q1 wd:Q2 ... }` query with many OPTIONALs
    completes in 1-3s for 1000 items, because each OPTIONAL is now a
    point-lookup against a small, explicit item set rather than a join
    against the whole graph.

So: PHASE 1 harvests the *seed set* -- every item carrying P961 (IPNI) or
P5037 (POWO), the two properties we can actually join our `xref.powo`
column against -- via plain single-predicate paged scans (no P31 filter).
PHASE 2 re-visits that seed set in VALUES-batches of BATCH_SIZE items and
attaches the other 7 properties (P846/P14607/P10585/P12380/P12100/P7715/
P3151) via OPTIONAL, which is now cheap because the item set is small and
explicit.

We deliberately DROP the live `wdt:P31 wd:Q16521` filter from Phase 1 for
cost reasons and instead rely on an empirical finding from the same
investigation: a direct COUNT of `?item wdt:P961 ?v ; wdt:P31 wd:Q16521`
returned 907654, against a plain `?item wdt:P961 ?v` COUNT of 908799 --
i.e. 99.87% of P961-holders are already typed as Q16521 (taxon). The
~0.13% (≈1145 items) that are P961/P5037-bearing but NOT typed as a
Wikidata taxon (e.g. a handful of disambiguation/redirect edge cases) are
accepted as noise here rather than paying the ~30-45s-per-page join tax on
every single page of a multi-hour crawl. This is a pipeline-level
engineering trade-off (not a correctness requirement of hostus itself,
which only ever reads the canonical CSV) and is documented here and in
pipelines/README.md.

Resumability: every phase writes an NDJSON file plus an `.offset`
checkpoint after each successful page/batch, so re-running this script
(e.g. because the calling shell hit its own timeout) picks up where it
left off instead of re-fetching from scratch. This matters because a full
Phase 1 + Phase 2 crawl over ~1M items is expected to take well over the
harness's per-command timeout, so it is invoked repeatedly until it
reports DONE.

Politeness: sequential requests only, retry with exponential backoff on
429/5xx/timeout/truncated-JSON, honoring `Retry-After` on 429.
"""
import json
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

from common import resolve_join_id

ENDPOINT = "https://query.wikidata.org/sparql"
UA = (
    "hostus-pipeline-wikidata/0.1 "
    "(https://github.com/jobrunner/hostus; SP4 xref bulk harvest; "
    "contact: jo.brunner@mayflower.de)"
)

# Phase 1: single-predicate scan, no join, no OPTIONALs -- see module
# docstring for why this shape is the only one that stays well under the
# 60s WDQS timeout at useful page sizes.
SEED_PROPS = ["P961", "P5037"]
SEED_PAGE_SIZE = 20000

# Phase 2: VALUES-batched enrichment of the seed set.
ENRICH_PROPS = ["P846", "P14607", "P10585", "P12380", "P12100", "P7715", "P3151"]
ENRICH_BATCH_SIZE = 500

MAX_RETRIES = 6


def _http(method, params, timeout=90):
    """POST or GET to WDQS with retry/backoff on transient failures."""
    last_err = None
    for attempt in range(MAX_RETRIES):
        try:
            headers = {"User-Agent": UA, "Accept": "application/sparql-results+json"}
            data = urllib.parse.urlencode(params).encode("utf-8")
            if method == "GET":
                url = f"{ENDPOINT}?{urllib.parse.urlencode(params)}"
                req = urllib.request.Request(url, headers=headers)
            else:
                headers["Content-Type"] = "application/x-www-form-urlencoded"
                req = urllib.request.Request(ENDPOINT, data=data, headers=headers, method="POST")
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                body = resp.read()
                return json.loads(body)
        except urllib.error.HTTPError as e:
            last_err = e
            wait = 2 * (attempt + 1)
            if e.code == 429:
                retry_after = e.headers.get("Retry-After")
                if retry_after:
                    try:
                        wait = max(wait, int(retry_after))
                    except ValueError:
                        pass
            elif e.code not in (500, 502, 503, 504):
                raise
            time.sleep(wait)
        except (urllib.error.URLError, TimeoutError, json.JSONDecodeError, ConnectionError) as e:
            last_err = e
            time.sleep(2 * (attempt + 1))
    raise RuntimeError(f"request failed after {MAX_RETRIES} retries: {last_err}")


def _qid(uri):
    return uri.rsplit("/", 1)[1]


def seed_paths(cache_dir, prop):
    return (f"{cache_dir}/seed_{prop}.ndjson", f"{cache_dir}/seed_{prop}.offset")


def run_seed_phase(cache_dir, prop):
    """Page a single property (no join) until a short page signals the end."""
    ndjson_path, offset_path = seed_paths(cache_dir, prop)
    try:
        with open(offset_path, encoding="utf-8") as f:
            offset = int(f.read().strip())
    except FileNotFoundError:
        offset = 0

    if offset == -1:  # sentinel: already complete
        with open(ndjson_path, encoding="utf-8") as f:
            count = sum(1 for _ in f)
        print(f"seed[{prop}]: already complete, {count} rows")
        return

    query = f"SELECT ?item ?v WHERE {{ ?item wdt:{prop} ?v . }} LIMIT {SEED_PAGE_SIZE} OFFSET {{OFFSET}}"

    with open(ndjson_path, "a", encoding="utf-8") as out:
        while True:
            q = query.replace("{OFFSET}", str(offset))
            result = _http("GET", {"query": q, "format": "json"})
            rows = result["results"]["bindings"]
            for b in rows:
                item = _qid(b["item"]["value"])
                val = b["v"]["value"]
                out.write(json.dumps({"item": item, "v": val}) + "\n")
            out.flush()
            offset += len(rows)
            with open(offset_path, "w", encoding="utf-8") as of:
                of.write(str(offset))
            print(f"seed[{prop}]: fetched page, offset now {offset} (+{len(rows)})")
            if len(rows) < SEED_PAGE_SIZE:
                break

    with open(offset_path, "w", encoding="utf-8") as of:
        of.write("-1")
    print(f"seed[{prop}]: DONE, {offset} rows")


def seed_phase_complete(cache_dir, prop):
    _, offset_path = seed_paths(cache_dir, prop)
    try:
        with open(offset_path, encoding="utf-8") as f:
            return f.read().strip() == "-1"
    except FileNotFoundError:
        return False


def load_seed_union(cache_dir):
    """Merge both seed NDJSON files into {qid: {P961: v, P5037: v}}."""
    merged = {}
    for prop in SEED_PROPS:
        ndjson_path, _ = seed_paths(cache_dir, prop)
        try:
            with open(ndjson_path, encoding="utf-8") as f:
                for line in f:
                    rec = json.loads(line)
                    merged.setdefault(rec["item"], {})[prop] = rec["v"]
        except FileNotFoundError:
            pass
    return merged


def load_joinable_ids(cache_dir):
    """The set of `xref.powo` ext_ids (bare IPNI ids) from the real concept
    DB, if a prior step has written them to .cache/powo_ext_ids.txt -- see
    build.sh. Restricting enrichment to this set is a deliberate scope
    decision (see module docstring / pipelines/README.md): a Wikidata item
    whose IPNI/POWO id isn't one of ours can never be joined, so enriching
    it costs crawl time for zero eventual benefit. Returns None (no
    restriction -- enrich the full seed union) if the file doesn't exist.
    """
    path = f"{cache_dir}/powo_ext_ids.txt"
    try:
        with open(path, encoding="utf-8") as f:
            return {line.strip() for line in f if line.strip()}
    except FileNotFoundError:
        return None


def target_qids(cache_dir):
    """The qids enrichment should cover: the seed union, optionally
    restricted to items joinable against the concept DB.

    "Joinable" here means EITHER raw property value (P961, or P5037 with
    its LSID prefix stripped) equals one of our `xref.powo` ext_ids -- NOT
    whichever one convert.py's P961-preference tie-break would pick for
    the CSV's join_id column. Those are different questions: an item
    where P961 and stripped-P5037 disagree and only the P5037 side
    happens to match our concept table is still genuinely joinable (via
    P5037), even though the tie-break rule would report P961's value in
    the output row. Using the tie-break result here instead would
    under-count the joinable set on every such disagreement.
    """
    from common import strip_lsid

    seed = load_seed_union(cache_dir)
    joinable_ids = load_joinable_ids(cache_dir)
    if joinable_ids is None:
        return sorted(seed.keys(), key=lambda q: int(q[1:]))

    qids = []
    for qid, vals in seed.items():
        p961 = vals.get("P961")
        p5037_bare = strip_lsid(vals.get("P5037"))
        if (p961 and p961 in joinable_ids) or (p5037_bare and p5037_bare in joinable_ids):
            qids.append(qid)
    return sorted(qids, key=lambda q: int(q[1:]))


def already_enriched(cache_dir):
    """Set of qids already present in enriched.ndjson -- resume is driven
    by this set membership, not a positional offset, so it stays correct
    even if the target qid list is later narrowed (e.g. to the joinable
    subset) after some enrichment already ran against the full seed union.
    """
    done = set()
    try:
        with open(f"{cache_dir}/enriched.ndjson", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                done.add(json.loads(line)["item"])
    except FileNotFoundError:
        pass
    return done


def run_enrich_phase(cache_dir, deadline=None):
    """Enrich target_qids() not yet in enriched.ndjson, in batches of
    ENRICH_BATCH_SIZE, stopping (cleanly, mid-batch-boundary) once
    time.monotonic() >= deadline if a deadline is given -- this is what
    lets the crawl be run as several bounded foreground chunks instead of
    one long-running process."""
    qids = target_qids(cache_dir)
    done = already_enriched(cache_dir)
    remaining = [q for q in qids if q not in done]

    print(f"enrich: target={len(qids)} already_done={len(qids) - len(remaining)} remaining={len(remaining)}")
    if not remaining:
        print(f"enrich: already complete, {len(qids)} items")
        return True

    optionals = "\n  ".join(f"OPTIONAL {{ ?item wdt:{p} ?{p} }}" for p in ENRICH_PROPS)
    select_vars = " ".join(f"?{p}" for p in ENRICH_PROPS)
    ndjson_path = f"{cache_dir}/enriched.ndjson"

    batches = [remaining[i : i + ENRICH_BATCH_SIZE] for i in range(0, len(remaining), ENRICH_BATCH_SIZE)]

    with open(ndjson_path, "a", encoding="utf-8") as out:
        for i, batch in enumerate(batches):
            if deadline is not None and time.monotonic() >= deadline:
                print(f"enrich: time budget reached, {i}/{len(batches)} remaining batches done this run")
                return False

            values = " ".join(f"wd:{q}" for q in batch)
            query = (
                f"SELECT ?item {select_vars} WHERE {{\n"
                f"  VALUES ?item {{ {values} }}\n"
                f"  {optionals}\n"
                f"}}"
            )
            result = _http("POST", {"query": query, "format": "json"})
            rows = result["results"]["bindings"]
            by_item = {}
            for b in rows:
                item = _qid(b["item"]["value"])
                rec = by_item.setdefault(item, {})
                for p in ENRICH_PROPS:
                    if p in b:
                        rec[p] = b[p]["value"]
            for q in batch:
                out.write(json.dumps({"item": q, **by_item.get(q, {})}) + "\n")
            out.flush()
            if (i + 1) % 20 == 0 or i + 1 == len(batches):
                print(f"enrich: batch {i + 1}/{len(batches)} done ({len(batch)} items)")

    print(f"enrich: DONE, {len(qids)} items")
    return True


def main():
    cache_dir = sys.argv[1]
    time_budget = float(sys.argv[2]) if len(sys.argv) > 2 else None
    deadline = time.monotonic() + time_budget if time_budget is not None else None

    for prop in SEED_PROPS:
        if not seed_phase_complete(cache_dir, prop):
            run_seed_phase(cache_dir, prop)

    if not all(seed_phase_complete(cache_dir, p) for p in SEED_PROPS):
        print("PHASE=seed INCOMPLETE")
        return

    seed = load_seed_union(cache_dir)
    print(f"seed union: {len(seed)} distinct items (P961 or P5037)")

    enrich_done = run_enrich_phase(cache_dir, deadline)

    if enrich_done:
        print("DONE")
    else:
        print("PHASE=enrich INCOMPLETE")


if __name__ == "__main__":
    main()
