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


def resolve_join_id(p961, p5037_raw):
    """Returns (join_id, disagreed) for one item's P961/P5037 values.

    join_id is None if neither property is present (should not happen for
    a genuine seed-set item). P961 wins on disagreement -- see
    pipelines/README.md for the rationale (it is already bare; P5037
    needs LSID-prefix stripping to reach the same form).
    """
    p5037_bare = strip_lsid(p5037_raw) if p5037_raw else None
    if p961 and p5037_bare:
        return p961, p961 != p5037_bare
    if p961:
        return p961, False
    if p5037_bare:
        return p5037_bare, False
    return None, False
