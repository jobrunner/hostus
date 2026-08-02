#!/usr/bin/env python3
"""P8b: measure whether P8's two-hop concept-relation method survives at scale.

Subcommands (all resumable, all cached on disk under .cache/):

    index    page the flat /taxon list once and build a local name index
    draw     draw the reproducible stratified sample (writes sample.tsv)
    crawl    fetch /portal/taxon/{uuid}/taxonRelationships per sampled concept
    direct   fetch /taxon/{uuid}/relationsToThisTaxon for concepts with relations
    probe    bounded follow-up crawl for dangling relationships (hand-analysis)
    analyze  compute every number reported in docs/research/cdm-sample.md

Crawl etiquette is not optional here: one honest User-Agent, >= 1s between
request starts, single threaded, backoff on 429/5xx, and a disk cache so a
re-run costs the server nothing. If the honest UA is ever refused the crawl
aborts loudly instead of disguising itself.
"""

import argparse
import json
import os
import random
import re
import sys
import time
import urllib.error
import urllib.request
from collections import Counter, defaultdict

API = "https://api.cybertaxonomy.org/rl_standardliste"
UA = ("hostus/2.0 (+https://github.com/jobrunner/hostus; "
      "jo.brunner@mayflower.de) taxonomic-concept-research")
HERE = os.path.dirname(os.path.abspath(__file__))
CACHE = os.path.join(HERE, ".cache")
SEED = 20260802
MIN_INTERVAL = 1.0

_last_request = [0.0]
LATENCIES = []


class Refused(Exception):
    """The honest User-Agent was refused -- hard stop, never work around it."""


def _sleep_to_rate_limit():
    delta = time.monotonic() - _last_request[0]
    if delta < MIN_INTERVAL:
        time.sleep(MIN_INTERVAL - delta)


def get_json(path, attempt=0):
    """One rate-limited GET against the CDM API. Returns parsed JSON."""
    url = API + path
    _sleep_to_rate_limit()
    req = urllib.request.Request(url, headers={
        "User-Agent": UA,
        "Accept": "application/json",
    })
    started = time.monotonic()
    _last_request[0] = started
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            body = resp.read()
        elapsed = time.monotonic() - started
        LATENCIES.append(elapsed)
        return json.loads(body.decode("utf-8"))
    except urllib.error.HTTPError as exc:
        if exc.code in (401, 403):
            raise Refused(
                "HTTP %d for the honest User-Agent on %s. STOP. Do not retry "
                "with a browser UA -- report this instead." % (exc.code, url))
        if exc.code == 404:
            return None
        if exc.code == 429 or exc.code >= 500:
            if attempt >= 4:
                raise
            backoff = 5 * (2 ** attempt)
            sys.stderr.write("  HTTP %d on %s, backing off %ds\n"
                             % (exc.code, path, backoff))
            time.sleep(backoff)
            return get_json(path, attempt + 1)
        raise
    except (urllib.error.URLError, TimeoutError) as exc:
        if attempt >= 4:
            raise
        backoff = 5 * (2 ** attempt)
        sys.stderr.write("  %s on %s, backing off %ds\n" % (exc, path, backoff))
        time.sleep(backoff)
        return get_json(path, attempt + 1)


def cached(relpath, path):
    """get_json(path) but memoised on disk at .cache/<relpath>."""
    full = os.path.join(CACHE, relpath)
    if os.path.exists(full):
        with open(full, "r", encoding="utf-8") as fh:
            return json.load(fh)
    data = get_json(path)
    os.makedirs(os.path.dirname(full), exist_ok=True)
    with open(full, "w", encoding="utf-8") as fh:
        json.dump(data, fh)
    return data


# --------------------------------------------------------------- name parsing

RANK_MARKERS = {"subsp.", "ssp.", "var.", "f.", "subvar.", "subf.", "nothosubsp.",
                "nothovar.", "cv.", "sect.", "ser.", "×", "x"}
# Tokens that terminate the name part even though they are lowercase: the
# dataset appends taxonomic qualifiers ("syn.", "auct.", "s. l.", "agg.")
# after the author, and "L." (Linnaeus) must never be mistaken for the "l."
# of "s. l." -- an earlier version of this heuristic did exactly that and
# produced canonical names like "Acrostichum ilvense l. syn.".
STOP_TOKENS = {"syn.", "auct.", "s.", "l.", "str.", "agg.", "sensu", "p."}


def split_sec(title_cache):
    """'Abies alba Mill. sec. HEGI: ...' -> ('Abies alba Mill.', 'HEGI: ...')."""
    parts = title_cache.split(" sec. ", 1)
    if len(parts) == 2:
        return parts[0].strip(), parts[1].strip()
    return title_cache.strip(), ""


def canonical_name(full_name):
    """Strip the author string: 'Abies alba Mill.' -> 'Abies alba'.

    Heuristic, deliberately conservative: keep the leading genus token, then
    keep further tokens while they are lowercase epithets, hybrid signs or
    rank markers. The first token that looks like an author (capitalised,
    parenthesised, a year) ends the name.
    """
    name = full_name.lstrip("?").strip()
    tokens = name.split()
    if not tokens:
        return ""
    out = [tokens[0]]
    for tok in tokens[1:]:
        low = tok.lower()
        if low in STOP_TOKENS:
            break
        if low in RANK_MARKERS or tok in ("×", "x"):
            out.append(low)
            continue
        if tok.startswith("("):
            break
        if re.match(r"^\d", tok):
            break
        if tok[:1].isupper():
            break
        out.append(low)
    return " ".join(out)


def genus_of(canonical):
    return canonical.split(" ")[0] if canonical else ""


# ------------------------------------------------------------------ index step

INDEX_TSV = os.path.join(CACHE, "index.tsv")  # bulk, never committed
PAGE_SIZE = 1000


def cmd_index(_args):
    page = 0
    rows = []
    total = None
    while True:
        data = cached("taxonlist/page_%04d.json" % page,
                      "/taxon?pageSize=%d&pageIndex=%d" % (PAGE_SIZE, page))
        total = data["count"]
        recs = data.get("records") or []
        if not recs:
            break
        for rec in recs:
            title = rec.get("titleCache") or ""
            name, sec = split_sec(title)
            secsrc = rec.get("secSource") or {}
            cit = (secsrc.get("citation") or {}) if secsrc else {}
            rows.append([
                rec["uuid"],
                name,
                canonical_name(name),
                sec,
                cit.get("uuid") or "",
                cit.get("titleCache") or "",
                "1" if rec.get("doubtful") else "0",
            ])
        sys.stderr.write("  page %d/%d, %d rows\n"
                         % (page, data["pagesAvailable"] - 1, len(rows)))
        page += 1
        if page >= data["pagesAvailable"]:
            break
    os.makedirs(CACHE, exist_ok=True)
    with open(INDEX_TSV, "w", encoding="utf-8") as fh:
        fh.write("uuid\tname\tcanonical\tsec_title\tsec_citation_uuid\t"
                 "sec_citation_title\tdoubtful\n")
        for row in rows:
            fh.write("\t".join(c.replace("\t", " ") for c in row) + "\n")
    print("indexed %d concepts (API count=%s) -> %s" % (len(rows), total, INDEX_TSV))


def load_index():
    rows = []
    with open(INDEX_TSV, "r", encoding="utf-8") as fh:
        header = fh.readline().rstrip("\n").split("\t")
        for line in fh:
            rows.append(dict(zip(header, line.rstrip("\n").split("\t"))))
    return rows


# ------------------------------------------------------------------- draw step

SAMPLE_TSV = os.path.join(HERE, "sample.tsv")
TARGET_MIN, TARGET_MAX = 300, 500
MAX_PER_GENUS = 3


def cmd_draw(_args):
    rows = load_index()
    groups = defaultdict(list)
    for row in rows:
        if row["canonical"]:
            groups[row["canonical"]].append(row)

    def stratum(members):
        secs = {m["sec_citation_uuid"] for m in members if m["sec_citation_uuid"]}
        n = len(secs) if secs else len(members)
        if n >= 8:
            return "A_many_sec"
        if n >= 3:
            return "B_mid_sec"
        if n == 2:
            return "C_two_sec"
        return "D_single_sec"

    # A homonym here = one canonical name carried by >= 2 distinct author
    # strings, i.e. the case where "match the partner by name" can go wrong.
    def is_homonym(members):
        authors = {m["name"] for m in members}
        return len(authors) >= 2

    buckets = defaultdict(list)
    for canon, members in groups.items():
        key = stratum(members)
        if is_homonym(members):
            key = "E_homonym"
        buckets[key].append(canon)

    rng = random.Random(SEED)
    quota = {"A_many_sec": 0.30, "B_mid_sec": 0.30, "C_two_sec": 0.15,
             "D_single_sec": 0.10, "E_homonym": 0.15}

    picked = []
    genus_count = Counter()
    total = 0
    for key in sorted(quota):
        want = int(TARGET_MAX * quota[key])
        candidates = sorted(buckets.get(key, []))
        rng.shuffle(candidates)
        got = 0
        for canon in candidates:
            if got >= want:
                break
            gen = genus_of(canon)
            if genus_count[gen] >= MAX_PER_GENUS:
                continue
            members = groups[canon]
            if total + len(members) > TARGET_MAX:
                continue
            picked.append((key, canon, members))
            genus_count[gen] += 1
            got += len(members)
            total += len(members)
    with open(SAMPLE_TSV, "w", encoding="utf-8") as fh:
        fh.write("stratum\tcanonical\tuuid\tname\tsec_citation_uuid\tsec_title\n")
        for key, canon, members in sorted(picked):
            for m in sorted(members, key=lambda r: r["uuid"]):
                fh.write("\t".join([key, canon, m["uuid"], m["name"],
                                    m["sec_citation_uuid"], m["sec_title"]]) + "\n")
    per_stratum = Counter()
    for key, _canon, members in picked:
        per_stratum[key] += len(members)
    print("sample: %d concepts in %d name groups, %d distinct genera"
          % (total, len(picked), len(genus_count)))
    for key in sorted(per_stratum):
        print("  %-14s %4d concepts" % (key, per_stratum[key]))
    if not TARGET_MIN <= total <= TARGET_MAX:
        print("WARNING: sample size outside the 300-500 target", file=sys.stderr)


def load_sample():
    rows = []
    with open(SAMPLE_TSV, "r", encoding="utf-8") as fh:
        header = fh.readline().rstrip("\n").split("\t")
        for line in fh:
            rows.append(dict(zip(header, line.rstrip("\n").split("\t"))))
    return rows


# ------------------------------------------------------------- crawl / direct

def rels_of(uuid):
    data = cached("rel/%s.json" % uuid, "/portal/taxon/%s/taxonRelationships" % uuid)
    return data or []


def cmd_crawl(_args):
    rows = load_sample()
    for i, row in enumerate(rows, 1):
        rels = rels_of(row["uuid"])
        if i % 25 == 0 or i == len(rows):
            sys.stderr.write("  %d/%d concepts crawled\n" % (i, len(rows)))
        del rels
    print("crawled %d concepts" % len(rows))


def cmd_direct(_args):
    rows = load_sample()
    done = 0
    for row in rows:
        if not rels_of(row["uuid"]):
            continue
        cached("to/%s.json" % row["uuid"],
               "/taxon/%s/relationsToThisTaxon?pageSize=200" % row["uuid"])
        done += 1
        if done % 25 == 0:
            sys.stderr.write("  %d direction lookups\n" % done)
    print("direction lookups: %d" % done)


def incoming_rel_uuids(uuid):
    path = os.path.join(CACHE, "to", "%s.json" % uuid)
    if not os.path.exists(path):
        return None
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    if not data:
        return set()
    return {r["uuid"] for r in (data.get("records") or [])}


# --------------------------------------------------------------- follow-up probe

PROBE_BUDGET = 160


def cmd_probe(args):
    """Bounded follow-up crawl to hand-categorise dangling relationships.

    For each dangling relationship the partner was not among the concepts
    sharing the same canonical name. The two cheap hypotheses are (a) the
    partner carries a *different* name (genus transfer, misapplied name) and
    (b) the partner shares the canonical name but sits outside the drawn
    sample. (b) cannot happen here because whole name groups are drawn, so
    this probe tests (a) by crawling concepts that share only the terminal
    epithet, plus every concept of the sampled genera.
    """
    index = load_index()
    sample = load_sample()
    sampled = {r["uuid"] for r in sample}
    unresolved = _resolution(sample)["dangling_details"]
    epithets = set()
    genera = set()
    for item in unresolved:
        canon = item["canonical"]
        toks = canon.split()
        if len(toks) >= 2:
            epithets.add(toks[-1])
        genera.add(toks[0])
    candidates = []
    for row in index:
        if row["uuid"] in sampled:
            continue
        toks = row["canonical"].split()
        if not toks:
            continue
        if toks[-1] in epithets or toks[0] in genera:
            candidates.append(row)
    rng = random.Random(SEED)
    rng.shuffle(candidates)
    budget = args.budget or PROBE_BUDGET
    chosen = candidates[:budget]
    with open(os.path.join(CACHE, "probe_candidates.tsv"), "w",
              encoding="utf-8") as fh:
        fh.write("uuid\tname\tcanonical\tsec_title\n")
        for row in chosen:
            fh.write("\t".join([row["uuid"], row["name"], row["canonical"],
                                row["sec_title"]]) + "\n")
    for i, row in enumerate(chosen, 1):
        rels_of(row["uuid"])
        if i % 25 == 0:
            sys.stderr.write("  probe %d/%d\n" % (i, len(chosen)))
    print("follow-up probe crawled %d extra concepts (%d epithets, %d genera)"
          % (len(chosen), len(epithets), len(genera)))


# ------------------------------------------------- exhaustive genus deep-dive

# Three small genera that carry dangling relationships in the sample. Crawling
# them EXHAUSTIVELY (every concept in the dataset with that genus, in every
# sec. space) turns "the partner was not found" from a guess into a verdict:
# either the partner is inside the genus under a different name, or it is not
# in the genus at all.
DEEPDIVE_GENERA = ["Coronilla", "Dorycnium", "Persicaria"]


def cmd_deepdive(_args):
    index = load_index()
    targets = [r for r in index if genus_of(r["canonical"]) in DEEPDIVE_GENERA]
    print("deep-dive over %d concepts in %s" % (len(targets), DEEPDIVE_GENERA))
    for i, row in enumerate(targets, 1):
        rels_of(row["uuid"])
        if i % 25 == 0:
            sys.stderr.write("  %d/%d\n" % (i, len(targets)))

    by_uuid = {r["uuid"]: r for r in index}
    holders = defaultdict(set)
    for row in targets:
        for rel in rels_of(row["uuid"]):
            holders[rel["uuid"]].add(row["uuid"])
    sample = load_sample()
    res = _resolution(sample)
    print("\ndangling relationships of sampled concepts in these genera, "
          "resolved against the exhaustive genus crawl:")
    hit = miss = 0
    for item in res["dangling_details"]:
        if genus_of(item["canonical"]) not in DEEPDIVE_GENERA:
            continue
        others = holders.get(item["rel"], set()) - {item["concept"]}
        if others:
            hit += 1
            for o in others:
                partner = by_uuid[o]
                print("  FOUND  %-34s %-22s -> %-34s %s"
                      % (item["name"][:34], item["type"][:22],
                         partner["name"][:34], partner["sec_title"][:40]))
        else:
            miss += 1
            print("  STILL MISSING  %-34s %-22s (sec. %s)"
                  % (item["name"][:34], item["type"][:22], item["sec"][:34]))
    print("\n  partner inside the genus: %d, partner outside the genus: %d"
          % (hit, miss))


# --------------------------------------------------- genus-transfer crosscheck

# The deep-dive left 12 dangling relationships whose partner is not even in
# the same genus. Both affected names have an obvious genus-transfer partner
# in the dataset. These two extra name groups (9 concepts) test that directly.
CROSSCHECK_NAMES = ["Securigera varia", "Polygonum persicaria"]


def cmd_crosscheck(_args):
    index = load_index()
    extra = [r for r in index if r["canonical"] in CROSSCHECK_NAMES]
    print("crosscheck over %d concepts: %s" % (len(extra), CROSSCHECK_NAMES))
    for row in extra:
        rels_of(row["uuid"])
    holders = defaultdict(set)
    for row in extra:
        for rel in rels_of(row["uuid"]):
            holders[rel["uuid"]].add(row["uuid"])
    by_uuid = {r["uuid"]: r for r in index}
    sample = load_sample()
    res = _resolution(sample)
    hit = 0
    for item in res["dangling_details"]:
        for other in holders.get(item["rel"], set()) - {item["concept"]}:
            hit += 1
            partner = by_uuid[other]
            print("  RESOLVED ACROSS GENERA  %-30s (sec. %-28s) --%s--> "
                  "%-30s (sec. %s)"
                  % (item["name"][:30], item["sec"][:28], item["type"],
                     partner["name"][:30], partner["sec_title"][:28]))
    print("  previously dangling relationships now resolved: %d" % hit)


# ------------------------------------------------------------------- analysis

def _resolution(sample):
    """The two-hop join, exactly as P8 described it.

    Candidate partners for concept X are the other concepts sharing X's
    canonical name; a relationship resolves when exactly one of them carries
    the same relationship uuid.
    """
    by_canon = defaultdict(list)
    for row in sample:
        by_canon[row["canonical"]].append(row)

    rel_cache = {row["uuid"]: rels_of(row["uuid"]) for row in sample}

    exact = ambiguous = dangling = 0
    dangling_details = []
    ambiguous_details = []
    per_concept = {}
    for row in sample:
        rels = rel_cache[row["uuid"]]
        per_concept[row["uuid"]] = len(rels)
        siblings = [s for s in by_canon[row["canonical"]] if s["uuid"] != row["uuid"]]
        for rel in rels:
            ruuid = rel["uuid"]
            hits = [s for s in siblings if any(r["uuid"] == ruuid
                                               for r in rel_cache[s["uuid"]])]
            rtype = (rel.get("type") or {}).get("representation_L10n", "?")
            if len(hits) == 1:
                exact += 1
            elif len(hits) > 1:
                ambiguous += 1
                ambiguous_details.append({
                    "concept": row["uuid"], "canonical": row["canonical"],
                    "rel": ruuid, "type": rtype,
                    "candidates": [h["uuid"] for h in hits]})
            else:
                dangling += 1
                dangling_details.append({
                    "concept": row["uuid"], "name": row["name"],
                    "canonical": row["canonical"], "sec": row["sec_title"],
                    "rel": ruuid, "type": rtype,
                    "conceptRelationship": (rel.get("type") or {}).get(
                        "conceptRelationship"),
                    "n_siblings": len(siblings)})
    return {"exact": exact, "ambiguous": ambiguous, "dangling": dangling,
            "dangling_details": dangling_details,
            "ambiguous_details": ambiguous_details,
            "per_concept": per_concept, "rel_cache": rel_cache}


# The CDM API exposes NO machine link between a concept's secSource citation
# and the 18 classifications: the Classification objects carry only a
# titleCache (no reference uuid), and those titleCaches are written
# differently from the corresponding Reference titleCaches. This crosswalk is
# therefore hand-built -- it is small (17 entries cover the bulk of the
# dataset) but it is curation, not data.
CROSSWALK = {
    "Wisskirchen & Haeupler 1998: Standardliste der Farn- und Blütenpflanzen "
    "Deutschlands":
        "WISSKIRCHEN & HAEUPLER, Standardliste ... 1998",
    "Schubert, R. & Vent, W. (eds.) 1990: Exkursionsflora von Deutschland, "
    "begr. von Werner Rothmaler, 8. Aufl., 4. Kritischer Band: 811. \u2013 "
    "Berlin: Volk und Wissen":
        "ROTHMALER, Exkursionsflora von Deutschland, 8. Aufl., Kritischer Band",
    "HEGI: Illustrierte Flora von Mitteleuropa, Aufl. 2 u. 3":
        "HEGI, Illustrierte Flora von Mitteleuropa, Aufl. 2 u. 3",
    "OBERDORFER: Pflanzensoziologische Exkursionsflora, ed. 7":
        "OBERDORFER, Pflanzensoziologische Exkursionsflora, 7. Aufl",
    "EHRENDORFER: Liste der Gefäßpflanzen Mitteleuropas, 2. Aufl":
        "EHRENDORFER, Liste der Gefäßpflanzen Mitteleuropas, 2. Aufl",
    "BfN: FloraWeb DB": "BfN, FloraWeb DB",
    "TUTIN et al.: Flora Europaea": "FLORA EUROPAEA, Tutin et al.",
    "SCHMEIL-FITSCHEN: Flora von Deutschland und angrenzenden Ländern, "
    "89. Aufl":
        "SCHMEIL-FITSCHEN, Flora von Deutschland ..., 89. Aufl",
    "Greuter & al.: Med-Checklist bisher Bde. 1, 3 und 4":
        "GREUTER & al., Med-Checklist bisher Bde. 1, 3 und 4",
    "BRUMMITT 1992: Vascular Plant Families and Genera. – Kew":
        "BRUMMITT, Vascular Plant Families and Genera. 1992",
    "Greuter & al. 1993: Names in Current Use for Extant Plant Genera. – "
    "Königstein":
        "Greuter & al., Names in Current Use for Extant Plant Genera. 1993",
    "Rostanski: Rostanski (Oenothera)":
        "ROSTANSKI in Wisskirchen & Haeupler 1998",
    "R. Wisskirchen & H. Haeupler 1998: Standardliste (fuer Synonyme mit "
    "Fakten)":
        "Wisskirchen & H. Haeupler (fuer Synonyme mit Fakten)",
    "BfN: FloraWeb DB (fuer Synonyme mit Fakten)":
        "BfN, FloraWeb DB (fuer Synonyme mit Fakten) ",
    "Andere Referenzen (fuer auct. Synonyme)":
        "Andere Referenzen (fuer auct. Synonyme)",
    "Andere Referenzen (fuer Synonyme s. l.)":
        "Andere Referenzen (fuer Synonyme s. l.), ",
    "Andere Referenzen (fuer Synonyme s. str.)":
        "Andere Referenzen (fuer Synonyme s. str.), ",
}


def cmd_analyze(_args):
    sample = load_sample()
    index = load_index()
    res = _resolution(sample)
    rel_cache = res["rel_cache"]
    n = len(sample)

    print("== 0. sample ==")
    strata = Counter(r["stratum"] for r in sample)
    print("concepts: %d, name groups: %d, genera: %d"
          % (n, len({r["canonical"] for r in sample}),
             len({genus_of(r["canonical"]) for r in sample})))
    for k in sorted(strata):
        print("  %-14s %4d" % (k, strata[k]))

    print("\n== 1. relation density ==")
    with_rel = sum(1 for r in sample if rel_cache[r["uuid"]])
    total_rel = sum(len(v) for v in rel_cache.values())
    print("concepts with >=1 relationship: %d/%d = %.1f%%"
          % (with_rel, n, 100.0 * with_rel / n))
    print("relationships total: %d, mean per concept: %.2f, "
          "mean per related concept: %.2f"
          % (total_rel, total_rel / n, (total_rel / with_rel) if with_rel else 0))
    dist = Counter(len(v) for v in rel_cache.values())
    print("relations-per-concept histogram: %s"
          % ", ".join("%d:%d" % (k, dist[k]) for k in sorted(dist)))
    print("per stratum:")
    for k in sorted(strata):
        sub = [r for r in sample if r["stratum"] == k]
        wr = sum(1 for r in sub if rel_cache[r["uuid"]])
        print("  %-14s %3d/%3d = %5.1f%%" % (k, wr, len(sub), 100.0 * wr / len(sub)))

    print("\n== 2. two-hop resolution ==")
    tot = res["exact"] + res["ambiguous"] + res["dangling"]
    for label, val in (("exactly one partner", res["exact"]),
                       ("ambiguous (>1 candidate)", res["ambiguous"]),
                       ("dangling (0 candidates)", res["dangling"])):
        print("  %-26s %5d / %d = %5.1f%%"
              % (label, val, tot, 100.0 * val / tot if tot else 0))

    # Upper bound: match the relationship uuid against every crawled concept,
    # not just the name-sharing ones. Shows how much the name restriction costs.
    holders = defaultdict(set)
    for path in ("rel",):
        base = os.path.join(CACHE, path)
        for fn in os.listdir(base):
            uuid = fn[:-5]
            with open(os.path.join(base, fn), "r", encoding="utf-8") as fh:
                for rel in json.load(fh) or []:
                    holders[rel["uuid"]].add(uuid)
    g_exact = g_amb = g_dangle = 0
    for row in sample:
        for rel in rel_cache[row["uuid"]]:
            others = holders[rel["uuid"]] - {row["uuid"]}
            if len(others) == 1:
                g_exact += 1
            elif len(others) > 1:
                g_amb += 1
            else:
                g_dangle += 1
    print("  upper bound over the whole crawled set (name restriction dropped):")
    print("    exactly one %d, ambiguous %d, dangling %d" % (g_exact, g_amb, g_dangle))

    print("\n== 3. dangling / unresolved sample (hand-analysis input) ==")
    for item in res["dangling_details"][:20]:
        print("  %s | %s | sec. %s | type=%s conceptRel=%s siblings=%d"
              % (item["rel"][:8], item["name"][:42], item["sec"][:34],
                 item["type"], item["conceptRelationship"], item["n_siblings"]))
    print("  (total dangling: %d)" % res["dangling"])
    print("  ambiguous cases:")
    for item in res["ambiguous_details"][:15]:
        print("    %s | %s | type=%s | %d candidates"
              % (item["rel"][:8], item["canonical"], item["type"],
                 len(item["candidates"])))

    print("\n== 4. relation-type distribution ==")
    types = Counter()
    for rels in rel_cache.values():
        for rel in rels:
            t = rel.get("type") or {}
            types[(t.get("conceptRelationship"), t.get("representation_L10n"),
                   t.get("symbol"), t.get("symmetric"))] += 1
    print("  conceptRel | representation_L10n | symbol | symmetric | count")
    for key, cnt in types.most_common():
        print("  %-10s | %-28s | %-16s | %-9s | %d"
              % (key[0], key[1], key[2], key[3], cnt))
    schema = {"congruent": "Congruent to", "includes": "Includes",
              "included_in": "Is included in", "overlaps": "Overlaps",
              "disjoint": "Excludes"}
    known = set(schema.values())
    unmapped = sorted({k[1] for k in types if k[1] not in known})
    print("  values NOT in the SP1 vocabulary (congruent|includes|included_in|"
          "overlaps|disjoint): %s" % (unmapped or "none"))

    print("\n== 4b. direction (relationsToThisTaxon) ==")
    inc_known = inc_missing = 0
    dir_counts = Counter()
    for row in sample:
        inc = incoming_rel_uuids(row["uuid"])
        if inc is None:
            inc_missing += len(rel_cache[row["uuid"]])
            continue
        for rel in rel_cache[row["uuid"]]:
            inc_known += 1
            t = (rel.get("type") or {}).get("representation_L10n", "?")
            dir_counts[(t, "to" if rel["uuid"] in inc else "from")] += 1
    print("  classified %d relationship ends (%d without a direction lookup)"
          % (inc_known, inc_missing))
    for key, cnt in sorted(dir_counts.items()):
        print("  %-28s %-5s %d" % (key[0], key[1], cnt))

    print("\n== 5. sec. resolvability ==")
    no_sec = [r for r in sample if not r["sec_citation_uuid"]]
    print("sampled concepts without a structured secSource citation: %d/%d"
          % (len(no_sec), n))
    cls = cached("classification.json", "/classification")
    cls_recs = cls.get("records") if isinstance(cls, dict) else cls
    cls_titles = {}
    for c in cls_recs:
        cls_titles[c["uuid"]] = c.get("titleCache", "")
    print("classifications in the dataset: %d" % len(cls_titles))
    cit_title = {r["sec_citation_uuid"]: r["sec_citation_title"] for r in index}
    def sec_of(row):
        return cit_title.get(row["sec_citation_uuid"], row["sec_title"])
    exact = sum(1 for r in sample if sec_of(r) in set(cls_titles.values()))
    print("sample concepts whose sec. title is EXACTLY a classification "
          "titleCache: %d/%d" % (exact, n))
    print("index-wide exact title matches: %d/%d"
          % (sum(1 for r in index
                 if r["sec_citation_title"] in set(cls_titles.values())),
             len(index)))
    sample_secs = Counter((r["sec_citation_uuid"], sec_of(r)) for r in sample)
    print("distinct sec. references in the sample: %d (whole dataset: %d)"
          % (len(sample_secs),
             len({r["sec_citation_uuid"] for r in index})))
    matched = sum(c for (_u, t), c in sample_secs.items() if t in CROSSWALK)
    print("sample concepts whose sec. maps onto a classification via the "
          "hand crosswalk: %d/%d = %.1f%%" % (matched, n, 100.0 * matched / n))
    print("index-wide: %d/%d = %.1f%%"
          % (sum(1 for r in index if r["sec_citation_title"] in CROSSWALK),
             len(index),
             100.0 * sum(1 for r in index
                         if r["sec_citation_title"] in CROSSWALK) / len(index)))
    for (u, t), c in sample_secs.most_common():
        print("  %-4s %-46s %4d  %s"
              % ("OK" if t in CROSSWALK else "MISS", t[:46], c, u[:8]))
    print("\n  the 18 classification titleCaches:")
    for u, t in sorted(cls_titles.items(), key=lambda kv: kv[1]):
        print("    %s  %s" % (u[:8], t))

    print("\n== 6. index-wide context ==")
    all_groups = defaultdict(set)
    for row in index:
        if row["canonical"]:
            all_groups[row["canonical"]].add(row["sec_citation_uuid"])
    sizes = Counter(len(v) for v in all_groups.values())
    print("distinct canonical names in the whole dataset: %d" % len(all_groups))
    print("name-group size histogram (distinct sec. per name): %s"
          % ", ".join("%d:%d" % (k, sizes[k]) for k in sorted(sizes)))

    # The sample deliberately over-represents names that occur in many sec.
    # spaces. Re-weighting the per-stratum densities by the strata's real
    # share of the 51.466 concepts gives the honest dataset-wide estimate.
    members = defaultdict(list)
    for row in index:
        if row["canonical"]:
            members[row["canonical"]].append(row)
    pop = Counter()
    for canon, mem in members.items():
        secs = {m["sec_citation_uuid"] for m in mem if m["sec_citation_uuid"]}
        k = len(secs) if secs else len(mem)
        if len({m["name"] for m in mem}) >= 2:
            key = "E_homonym"
        elif k >= 8:
            key = "A_many_sec"
        elif k >= 3:
            key = "B_mid_sec"
        elif k == 2:
            key = "C_two_sec"
        else:
            key = "D_single_sec"
        pop[key] += len(mem)
    total_pop = sum(pop.values())
    est = 0.0
    print("\nstratum shares in the full dataset vs. sampled density:")
    for k in sorted(pop):
        sub = [r for r in sample if r["stratum"] == k]
        dens = (sum(1 for r in sub if rel_cache[r["uuid"]]) / len(sub)) if sub else 0
        est += pop[k] * dens
        print("  %-14s %6d concepts (%5.1f%% of dataset), sampled density "
              "%5.1f%%" % (k, pop[k], 100.0 * pop[k] / total_pop, 100.0 * dens))
    print("  re-weighted dataset-wide relation density estimate: %.1f%% "
          "(~%d of %d concepts)"
          % (100.0 * est / total_pop, int(est), total_pop))


def cmd_latency(_args):
    """Print latency stats gathered from a fresh (uncached) measurement run."""
    path = os.path.join(CACHE, "latency.txt")
    if not os.path.exists(path):
        print("no latency.txt -- run `probe.sh measure-latency` first")
        return
    vals = sorted(float(x) for x in open(path, encoding="utf-8") if x.strip())
    if not vals:
        return
    mean = sum(vals) / len(vals)
    p95 = vals[min(len(vals) - 1, int(round(0.95 * (len(vals) - 1))))]
    print("n=%d mean=%.3fs p50=%.3fs p95=%.3fs max=%.3fs"
          % (len(vals), mean, vals[len(vals) // 2], p95, vals[-1]))
    # The limiter sleeps to 1s measured from the request START, so the real
    # cost of a request is max(1s, latency) -- not 1s + latency. The observed
    # figure below is the one that matched the wall clock of this run.
    observed = sum(max(MIN_INTERVAL, v) for v in vals) / len(vals)
    print("observed cost per request (max(1s, latency)): %.3fs" % observed)
    for label, per_req in (("1 req/s floor", 1.0),
                           ("observed max(1s,latency)", observed),
                           ("pessimistic 1s + mean lat", 1.0 + mean),
                           ("worst case 1s + p95 lat", 1.0 + p95)):
        for label2, reqs in (("51466 concepts", 51466),
                             ("51466 x2 (rel + direction)", 2 * 51466)):
            hours = reqs * per_req / 3600.0
            print("  %-24s %-28s %6.1f h" % (label, label2, hours))


def main():
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest="cmd", required=True)
    for name, fn in (("index", cmd_index), ("draw", cmd_draw),
                     ("crawl", cmd_crawl), ("direct", cmd_direct),
                     ("probe", cmd_probe), ("deepdive", cmd_deepdive),
                     ("crosscheck", cmd_crosscheck),
                     ("analyze", cmd_analyze),
                     ("latency", cmd_latency)):
        p = sub.add_parser(name)
        p.set_defaults(func=fn)
        if name == "probe":
            p.add_argument("--budget", type=int, default=PROBE_BUDGET)
    args = parser.parse_args()
    try:
        args.func(args)
    except Refused as exc:
        sys.stderr.write("\nCRAWL ABORTED: %s\n" % exc)
        sys.exit(2)
    finally:
        if LATENCIES:
            with open(os.path.join(CACHE, "latency.txt"), "a",
                      encoding="utf-8") as fh:
                for val in LATENCIES:
                    fh.write("%.4f\n" % val)


if __name__ == "__main__":
    main()
