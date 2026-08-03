#!/usr/bin/env bash
#
# debt-guard.sh — fast, test-free technical-debt ratchet checks.
#
#   1. Suppression budget: total #nosec + //nolint may not exceed the baseline
#      in .debt-budget (ratchet down only — new suppressions force a justified
#      bump or, better, a fix).
#   2. No new debt markers: TODO/FIXME/HACK/XXX comment markers are kept at
#      zero (the codebase has none; this keeps it that way).
#
# poc/** and third_party/** are excluded from both checks (experimental /
# vendored code that isn't held to the same bar).
#
# A third guard (hardcoded source-file extensions bypassing a domain-level
# allowlist, e.g. ".gpkg"/".zip") lived here in the porting source (ortus);
# not applicable to hostus yet — add it back once an equivalent domain
# boundary exists.
#
# Usage: scripts/debt-guard.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
BUDGET_FILE=".debt-budget"
status=0

[ -f "$BUDGET_FILE" ] || {
  echo "debt-guard: baseline file not found: $BUDGET_FILE (run from repo root)" >&2
  exit 2
}

# count <pattern> — number of matching directive lines in first-party *.go,
# excluding poc/** and third_party/**.
# Tolerant of zero matches: grep exits 1 with no hits, which would abort the
# pipeline under `set -euo pipefail`, so swallow it and still emit a count.
count() {
  { grep -rn "$1" --include='*.go' . || true; } \
    | { grep -v '/\.go/mod/' || true; } \
    | { grep -v '^\./poc/' || true; } \
    | { grep -v '^\./third_party/' || true; } \
    | { grep -c '' || true; } | tr -d ' '
}

# 1. Suppression budget — count the actual directive forms (//nolint, #nosec),
# not the bare substring, so prose can't inflate the number.
nosec=$(count '#nosec')
nolint=$(count '//nolint')
total=$((nosec + nolint))
baseline=$(grep -vE '^\s*#|^\s*$' "$BUDGET_FILE" | head -1 | tr -d ' ')

echo "suppressions: #nosec=$nosec //nolint=$nolint total=$total (baseline $baseline)"
if [ "$total" -gt "$baseline" ]; then
  echo "  ▼ debt-guard: FAIL — suppressions grew past the baseline." >&2
  echo "    Remove a suppression (preferred), or justify a bump in .debt-budget in the PR." >&2
  status=1
elif [ "$total" -lt "$baseline" ]; then
  echo "  ✓ suppressions dropped — lower the baseline in .debt-budget to $total to lock it in."
fi

# 2. No new debt markers -----------------------------------------------------
# Leading-marker form only ("// TODO", "// FIXME:") so prose like "...a bug,"
# doesn't false-positive. poc/** and third_party/** are exempt.
markers=$(grep -rnE '//[[:space:]]*(TODO|FIXME|HACK|XXX)([[:space:]:(]|$)' --include='*.go' . \
  | grep -v '/\.go/mod/' \
  | grep -v '^\./poc/' \
  | grep -v '^\./third_party/' || true)
if [ -n "$markers" ]; then
  echo "  ▼ debt-guard: FAIL — debt markers found (keep them out of the tree; track in docs):" >&2
  echo "$markers" | sed 's/^/      /' >&2
  status=1
else
  echo "debt markers: none"
fi

[ "$status" -eq 0 ] && echo "debt-guard: OK"
exit "$status"
