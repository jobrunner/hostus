#!/usr/bin/env python3
"""Euro+Med (api.cybertaxonomy.org/euromed, CDM REST) -> canonical name-list CSV.

PROBE RESULT (see pipelines/euromed/build.sh header and
docs/superpowers/.../task-2-report.md for the full writeup): the
purpose-built bulk-export endpoints (`checklist/export`,
`checklist/exportCSV`) are broken on this CDM instance exactly like PoC P8
found for the Wisskirchen instance -- they report a correct `count` but
return zero records (export) or a 0-byte body after redirect (exportCSV).
The flat, paged `/taxon` listing DOES work correctly (verified: distinct
pages via the `pageIndex` query param -- NOT `pageNumber`, which is silently
ignored and always returns page 0 -- return distinct, correctly-offset
records), so this script crawls that endpoint page by page.

What this endpoint gives us, and what it does NOT:
  - `titleCache`: the taxon name INCLUDING author and a "sec./syn. sec."
    citation suffix, e.g. `"Allium odorum Kar. & Kir., nom. illeg., syn.
    sec. 2010: ..."`. We strip the sec./syn. citation tail via regex to
    get a name+author string; we do NOT attempt to further strip the
    author (unlike GermanSL/EuroSL, this source has no separate
    TaxonName/NameAuthor columns to draw on).
  - `class`: "Taxon" (accepted-status concept) vs "Synonym" -> status.
  - NO explicit rank field on this listing (rank is only visible per-name
    on the CDM name object, which is not embedded here and would need one
    extra HTTP call per record -- 167k+ additional requests, infeasible in
    the time budget). Left empty; not guessed.
  - NO accepted-name link for synonym rows: resolving a Synonym's accepted
    Taxon requires either `/taxon/{uuid}/synonyms` on the accepted side (one
    call per ~65k accepted taxa) or a homotypic-group cross-reference walk
    (PoC P8's two-hop method) -- both infeasible here for the same reason.
    Left empty; not guessed. This is the one real gap in this source's
    canonical list relative to GermanSL/EuroSL.

Politeness: sequential requests only, one at a time, `pageSize=500`
(observed ~5-6s per page given how verbose each Taxon record is -- this
alone spaces requests out; no concurrency is used).
"""
import csv
import json
import re
import sys
import time
import urllib.request

BASE = "https://api.cybertaxonomy.org/euromed/taxon"
UA = "hostus-pipeline-euromed/0.1 (research; local evaluation only)"
PAGE_SIZE = 500

SEC_TAIL = re.compile(r",?\s*(syn\.\s*)?sec\.\s.*$", re.IGNORECASE)


def clean_name(title_cache):
    return SEC_TAIL.sub("", title_cache or "").strip()


def fetch_page(page_index, retries=3):
    url = f"{BASE}?pageSize={PAGE_SIZE}&pageIndex={page_index}"
    last_err = None
    for attempt in range(retries):
        try:
            req = urllib.request.Request(url, headers={"User-Agent": UA})
            with urllib.request.urlopen(req, timeout=60) as resp:
                return json.load(resp)
        except Exception as e:  # noqa: BLE001 - want to retry on any transient error
            last_err = e
            time.sleep(2 * (attempt + 1))
    raise RuntimeError(f"failed to fetch page {page_index} after {retries} tries: {last_err}")


def main():
    out_path, resume_log = sys.argv[1:3]

    first = fetch_page(0)
    total = first["count"]
    pages_available = first["pagesAvailable"]
    print(f"total_count={total} pages_available={pages_available} page_size={PAGE_SIZE}")

    row_count = 0
    taxa = set()
    statuses = {}

    mode = "w"
    start_page = 0
    with open(out_path, mode, newline="", encoding="utf-8") as f:
        w = csv.writer(f, delimiter="|")
        w.writerow(["taxon", "rank", "status", "accepted_taxon", "source_id"])

        page = first
        page_index = 0
        while True:
            for rec in page["records"]:
                title = rec.get("titleCache") or ""
                taxon = clean_name(title)
                if not taxon or taxon == "???":
                    continue
                cls = rec.get("class")
                status = "synonym" if cls == "Synonym" else "accepted"
                source_id = rec.get("uuid") or ""
                w.writerow([taxon, "", status, "", source_id])
                row_count += 1
                taxa.add(taxon)
                statuses[status] = statuses.get(status, 0) + 1

            with open(resume_log, "a", encoding="utf-8") as lg:
                lg.write(f"{page_index}\n")

            page_index += 1
            if page_index >= pages_available:
                break
            page = fetch_page(page_index)

    print(f"rows={row_count} taxa={len(taxa)}")
    print("statuses=" + ",".join(f"{k}:{v}" for k, v in sorted(statuses.items(), key=lambda kv: -kv[1])))
    print(f"pages_fetched={page_index + 1}/{pages_available}")


if __name__ == "__main__":
    main()
