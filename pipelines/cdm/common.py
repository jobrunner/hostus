#!/usr/bin/env python3
"""Shared crawl etiquette + the hand-curated sec. -> classification crosswalk.

Lifted (deliberately, near-verbatim) from `poc/p08b_cdm_sample/cdm_sample.py`,
the probe whose measurement is written up in `docs/research/cdm-sample.md`.
Everything here is the productionised form of something that probe already
proved on 909 real requests:

  * exactly ONE honest User-Agent, never a browser disguise,
  * `Refused` -- 401/403 on the honest UA is a HARD STOP, not a signal to
    retry with different headers,
  * >= 1s between request STARTS (so the real per-request cost is
    max(1s, latency), which is what Task 1's wall-clock projection assumed),
  * exponential backoff on 429/5xx/timeouts,
  * `CROSSWALK` + `assert_crosswalk()` -- the uuid-keyed, asserted mapping
    from a concept's `secSource` reference title onto one of the 18
    classification uuids. The assertion stays; it is the only thing standing
    between a typo and silently dropping WISSKIRCHEN & HAEUPLER, the hub of
    the whole dataset.
"""

import gzip
import json
import os
import sys
import time
import urllib.error
import urllib.request

API = "https://api.cybertaxonomy.org/rl_standardliste"

# BINDING (owner decision, see task brief): this exact User-Agent on every
# request. If it is refused, STOP and report -- never substitute a browser UA.
UA = ("hostus/2.0 (+https://github.com/jobrunner/hostus; "
      "jo.brunner@mayflower.de) taxonomic-concept-research")

MIN_INTERVAL = 1.0
MAX_ATTEMPTS = 5
TIMEOUT = 300

_last_request = [0.0]
_stats = {"requests": 0, "bytes": 0, "seconds": 0.0}


class Refused(Exception):
    """The honest User-Agent was refused -- hard stop, never work around it."""


def request_stats():
    return dict(_stats)


def _sleep_to_rate_limit():
    delta = time.monotonic() - _last_request[0]
    if delta < MIN_INTERVAL:
        time.sleep(MIN_INTERVAL - delta)


def get_json(path, attempt=0):
    """One rate-limited GET against the CDM API. Returns parsed JSON or None
    on 404."""
    url = API + path
    _sleep_to_rate_limit()
    req = urllib.request.Request(url, headers={
        "User-Agent": UA,
        "Accept": "application/json",
    })
    started = time.monotonic()
    _last_request[0] = started
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            body = resp.read()
        _stats["requests"] += 1
        _stats["bytes"] += len(body)
        _stats["seconds"] += time.monotonic() - started
        return json.loads(body.decode("utf-8"))
    except urllib.error.HTTPError as exc:
        if exc.code in (401, 403):
            raise Refused(
                "HTTP %d for the honest User-Agent on %s. STOP. Do not retry "
                "with a browser UA -- report this instead." % (exc.code, url))
        if exc.code == 404:
            _stats["requests"] += 1
            return None
        if exc.code == 429 or exc.code >= 500:
            if attempt >= MAX_ATTEMPTS - 1:
                raise
            backoff = 5 * (2 ** attempt)
            sys.stderr.write("  HTTP %d on %s, backing off %ds\n"
                             % (exc.code, path, backoff))
            time.sleep(backoff)
            return get_json(path, attempt + 1)
        raise
    except (urllib.error.URLError, TimeoutError, json.JSONDecodeError) as exc:
        if attempt >= MAX_ATTEMPTS - 1:
            raise
        backoff = 5 * (2 ** attempt)
        sys.stderr.write("  %s on %s, backing off %ds\n" % (exc, path, backoff))
        time.sleep(backoff)
        return get_json(path, attempt + 1)


def cached_gz(cache_dir, relpath, path):
    """get_json(path), memoised on disk as gzipped JSON.

    Written to a temp file and renamed, so an interrupt can never leave a
    half-written cache entry that a later run would read back as truth.
    """
    full = os.path.join(cache_dir, relpath)
    if os.path.exists(full):
        with gzip.open(full, "rt", encoding="utf-8") as fh:
            return json.load(fh)
    data = get_json(path)
    os.makedirs(os.path.dirname(full), exist_ok=True)
    tmp = full + ".tmp"
    with gzip.open(tmp, "wt", encoding="utf-8") as fh:
        json.dump(data, fh)
    os.replace(tmp, full)
    return data


# --------------------------------------------------------------- append log

def append_ndjson(fh, record):
    """One record, one write, one flush -- the unit of resumable progress.

    A kill between write and flush can leave a truncated final line; that is
    why read_ndjson() below drops an unparseable LAST line instead of dying.
    Any earlier line is always complete.
    """
    fh.write(json.dumps(record, ensure_ascii=False) + "\n")
    fh.flush()


def read_ndjson(path):
    """Yield records, tolerating exactly one truncated trailing line."""
    if not os.path.exists(path):
        return
    with open(path, "r", encoding="utf-8") as fh:
        lines = fh.read().splitlines()
    for i, line in enumerate(lines):
        if not line.strip():
            continue
        try:
            yield json.loads(line)
        except ValueError:
            if i == len(lines) - 1:
                sys.stderr.write(
                    "  note: dropped truncated trailing line in %s "
                    "(interrupted run) -- it will be re-fetched\n" % path)
                return
            raise


def rewrite_clean(path):
    """Rewrite an NDJSON log without its truncated trailing line, if any.

    Called before re-opening a log for append so the next appended record
    cannot be glued onto a half-written one.
    """
    if not os.path.exists(path):
        return
    with open(path, "r", encoding="utf-8") as fh:
        lines = fh.read().splitlines()
    if not lines:
        return
    try:
        json.loads(lines[-1])
        return
    except ValueError:
        pass
    tmp = path + ".tmp"
    with open(tmp, "w", encoding="utf-8") as fh:
        for line in lines[:-1]:
            fh.write(line + "\n")
    os.replace(tmp, path)


# ------------------------------------------------------------------ progress

class Progress:
    def __init__(self, label, total, every=200):
        self.label = label
        self.total = total
        self.every = every
        self.done = 0
        self.start = time.monotonic()

    def tick(self, n=1, force=False):
        self.done += n
        if not force and self.done % self.every:
            return
        elapsed = time.monotonic() - self.start
        rate = self.done / elapsed if elapsed else 0.0
        remaining = max(0, self.total - self.done)
        eta = remaining / rate if rate else 0.0
        print("  %s %d/%d  elapsed %s  eta %s"
              % (self.label, self.done, self.total,
                 _hms(elapsed), _hms(eta)), flush=True)


def _hms(seconds):
    seconds = int(seconds)
    return "%d:%02d:%02d" % (seconds // 3600, (seconds % 3600) // 60,
                             seconds % 60)


# --------------------------------------------- sec. -> classification crosswalk
#
# The CDM API exposes NO machine link between a concept's secSource citation
# and the 18 classifications: the Classification objects carry only a
# titleCache (no reference uuid), those titleCaches are written differently
# from the corresponding Reference titleCaches, and /classification/{uuid} is
# unrouted (404). Exact title equality covers 448 of 51466 concepts (0.9%).
#
# NOTE (measured while building this pipeline, and the reason the crosswalk is
# still authoritative here): a concept's `taxonNodes[].classification.uuid`
# IS machine-readable, but it answers a DIFFERENT question -- where the
# concept is PLACED in a tree, not which `sec.` reference space it belongs to.
# `Abies alba sec. Wisskirchen & Haeupler 1998`
# (872088a4-95f4-472c-ae79-a29028bb3fbf) carries two taxon nodes, one of them
# in the FloraWeb classification. Using tree placement as the sec. space would
# therefore be wrong. convert.py reports the agreement rate between the two as
# a diagnostic; the crosswalk decides.
#
# Keyed on the Reference titleCache, mapping onto the *classification uuid*,
# never onto another display string -- an earlier version of this table mapped
# onto titles and two of them were elided ("... 1998"), silently matching no
# classification at all, including the hub. assert_crosswalk() makes that class
# of typo impossible to ship.
CLS_WISSKIRCHEN = "4ea7fe85-4a02-47a0-949f-5e623f0c6216"
CLS_ROTHMALER = "1f41816b-1715-4428-9e57-8065081f24f2"
CLS_HEGI = "b3725bbc-d05c-4769-b64c-01c4d49194e5"
CLS_OBERDORFER = "a51d7b77-20f2-4285-88c3-cc31920935df"
CLS_EHRENDORFER = "45006c81-98ff-4f27-ad1b-011bae53910a"
CLS_FLORAWEB = "dc2692d4-81e2-4829-9794-40de9ea56696"
CLS_FLORA_EUROPAEA = "c047b968-689e-4363-8fc8-7da802785edd"
CLS_SCHMEIL_FITSCHEN = "03ba2da0-1e67-4dce-bb55-59058d4b0f26"
CLS_MED_CHECKLIST = "1f83ed0e-6bc0-4461-99d3-cfb333820f0f"
CLS_BRUMMITT = "4ad43f6f-0c86-4b67-8a41-93b710ad0364"
CLS_NCU = "7063b329-c502-4414-acc2-0f4b20d3a6bb"
CLS_ROSTANSKI = "7ef87c1f-b9fd-4fd6-9a2c-436030cdf533"
CLS_WH_SYN_FAKTEN = "cde3c428-73af-4882-9ac7-16a6ac56077e"
CLS_FLORAWEB_SYN_FAKTEN = "f4950cd9-67d3-42a5-b8d8-2a0a2dcec871"
CLS_ANDERE_AUCT = "978d0b0f-61a8-408c-9303-bb7eda5958a1"
CLS_ANDERE_SL = "02077b2e-2d80-498a-9901-250429a158a3"
CLS_ANDERE_SSTR = "6d1dd1c4-9ee9-4ff7-abc3-e5285fb73873"
# The 18th classification, "Andere Referenzen (fuer Synonyme p. p.)", is
# referenced by NO secSource anywhere in the 51466 concepts -- deliberately
# unmapped, not forgotten.
CLS_UNMAPPED = {"e2289284-7cba-42a9-9f96-59fead7e5aef"}

CROSSWALK = {
    "Wisskirchen & Haeupler 1998: Standardliste der Farn- und Blütenpflanzen "
    "Deutschlands": CLS_WISSKIRCHEN,
    "Schubert, R. & Vent, W. (eds.) 1990: Exkursionsflora von Deutschland, "
    "begr. von Werner Rothmaler, 8. Aufl., 4. Kritischer Band: 811. – "
    "Berlin: Volk und Wissen": CLS_ROTHMALER,
    "HEGI: Illustrierte Flora von Mitteleuropa, Aufl. 2 u. 3": CLS_HEGI,
    "OBERDORFER: Pflanzensoziologische Exkursionsflora, ed. 7": CLS_OBERDORFER,
    "EHRENDORFER: Liste der Gefäßpflanzen Mitteleuropas, 2. Aufl":
        CLS_EHRENDORFER,
    "BfN: FloraWeb DB": CLS_FLORAWEB,
    "TUTIN et al.: Flora Europaea": CLS_FLORA_EUROPAEA,
    "SCHMEIL-FITSCHEN: Flora von Deutschland und angrenzenden Ländern, "
    "89. Aufl": CLS_SCHMEIL_FITSCHEN,
    "Greuter & al.: Med-Checklist bisher Bde. 1, 3 und 4": CLS_MED_CHECKLIST,
    "BRUMMITT 1992: Vascular Plant Families and Genera. – Kew": CLS_BRUMMITT,
    "Greuter & al. 1993: Names in Current Use for Extant Plant Genera. – "
    "Königstein": CLS_NCU,
    "Rostanski: Rostanski (Oenothera)": CLS_ROSTANSKI,
    "R. Wisskirchen & H. Haeupler 1998: Standardliste (fuer Synonyme mit "
    "Fakten)": CLS_WH_SYN_FAKTEN,
    "BfN: FloraWeb DB (fuer Synonyme mit Fakten)": CLS_FLORAWEB_SYN_FAKTEN,
    "Andere Referenzen (fuer auct. Synonyme)": CLS_ANDERE_AUCT,
    "Andere Referenzen (fuer Synonyme s. l.)": CLS_ANDERE_SL,
    "Andere Referenzen (fuer Synonyme s. str.)": CLS_ANDERE_SSTR,
}


def assert_crosswalk(cls_titles):
    """Every crosswalk target must be one of the 18 real classification uuids,
    and every classification must be either targeted or explicitly unmapped.

    Fails loudly. Do not downgrade this to a warning.
    """
    known = set(cls_titles)
    bad = sorted(v for v in set(CROSSWALK.values()) if v not in known)
    if bad:
        raise AssertionError("crosswalk targets that are not classification "
                             "uuids: %s" % bad)
    targeted = set(CROSSWALK.values())
    untargeted = sorted(known - targeted - CLS_UNMAPPED)
    if untargeted:
        raise AssertionError("classifications neither targeted nor explicitly "
                             "unmapped: %s" % untargeted)
    return ("crosswalk assertion: %d entries, all targets are real "
            "classification uuids; %d/%d classifications targeted, "
            "%d explicitly unmapped"
            % (len(CROSSWALK), len(targeted), len(known), len(CLS_UNMAPPED)))
