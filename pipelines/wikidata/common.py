"""Shared helpers between crawl.py and convert.py (LSID normalisation +
the P961-vs-P5037 join-id resolution rule).

Kept in one place so the crawler's joinable-subset filter and the
converter's canonical-CSV join_id column can never drift out of sync.
"""

LSID_PREFIX = "urn:lsid:ipni.org:names:"


def strip_lsid(value):
    if value and value.startswith(LSID_PREFIX):
        return value[len(LSID_PREFIX):]
    return value


def resolve_join_id(p961, p5037_raw, joinable_ids=None):
    """Returns (join_id, disagreed) for one item's P961/P5037 values.

    join_id is None if neither property is present (should not happen for
    a genuine seed-set item).

    When P961 and stripped-P5037 AGREE, or only one is present, there is
    no ambiguity. When they DISAGREE, emitting a fixed "P961 always wins"
    value is wrong whenever P961's value is not actually one of our
    concepts' `xref.powo` ids and only the P5037 side is: that would emit
    a join_id that can never resolve, silently orphaning every one of
    that item's other authority rows even though the item genuinely is
    joinable via P5037 -- fix-round-1 finding, see task-1-report.md
    ("dead join key" bug). So when `joinable_ids` is given and the two
    values disagree, whichever one is actually IN that set is emitted;
    P961 remains the fallback only when both match, neither matches, or
    no `joinable_ids` restriction is supplied at all (e.g. a run of this
    pipeline in its unrestricted, general-purpose mode has no oracle to
    prefer one over the other, so the documented "P961 is more direct"
    rationale applies as originally written).
    """
    p5037_bare = strip_lsid(p5037_raw) if p5037_raw else None
    if p961 and p5037_bare:
        if p961 == p5037_bare:
            return p961, False
        if joinable_ids is not None:
            p961_ok = p961 in joinable_ids
            p5037_ok = p5037_bare in joinable_ids
            if p5037_ok and not p961_ok:
                return p5037_bare, True
        return p961, True
    if p961:
        return p961, False
    if p5037_bare:
        return p5037_bare, False
    return None, False
