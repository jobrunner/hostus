#!/usr/bin/env bash
#
# Documentation-drift harness, ported from ortus's
# .claude/skills/doc-drift-check/scripts/check-doc-drift.sh and adapted to
# hostus's SP0 state.
#
# ortus embeds its OpenAPI spec in the Go binary (internal/adapters/http/
# openapi.yaml) and diffs it against the api/ copy, plus a routes<->spec
# contract test. hostus has neither yet: the only spec is the hand-written
# baseline at api/openapi/openapi.yaml (added in S14), and there is no
# code-generated/embedded copy or contract test to compare it against —
# both are an SP1+ concern once the real taxonomy endpoints and
# code-generation exist. Checks 1 and 3 below are therefore soft no-ops
# until that lands (see the comment at each check); checks 2 (spec parses)
# and 5 (mkdocs --strict) are fully functional today.
#
# Usage:
#   doc-drift-check.sh          # full gate (all checks it can run)
#   doc-drift-check.sh --fast   # only the instant checks (2); for a pre-PR hook
set -uo pipefail

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
cd "$PROJECT_DIR" || exit 1

EMBEDDED="internal/adapters/http/openapi.yaml"
API_COPY="api/openapi/openapi.yaml"

FAST=0
for arg in "$@"; do
  case "$arg" in
    --fast) FAST=1 ;;
  esac
done

fail=0
note() { printf '%s\n' "$*" >&2; }
bad()  { printf '  x %s\n' "$*" >&2; fail=1; }
ok()   { printf '  + %s\n' "$*" >&2; }
skip() { printf '  - %s (skipped: %s)\n' "$1" "$2" >&2; }

note "== doc-drift harness =="

# --- 1. OpenAPI: embedded == api/ copy --------------------------------------
# No-op until SP1+ introduces a code-generated/embedded spec (there is only
# one hand-written copy today: $API_COPY).
if [ -f "$EMBEDDED" ]; then
  if diff -q "$EMBEDDED" "$API_COPY" >/dev/null 2>&1; then
    ok "OpenAPI copies identical"
  else
    bad "OpenAPI copies differ — the api/ copy is stale. Resync: cp $EMBEDDED $API_COPY"
  fi
else
  skip "OpenAPI embedded/api-copy sync" "no embedded spec yet (SP1+); $API_COPY is the sole source of truth"
fi

# --- 2. OpenAPI parses + local $refs resolve --------------------------------
if [ ! -f "$API_COPY" ]; then
  bad "OpenAPI baseline missing ($API_COPY)"
elif command -v python3 >/dev/null 2>&1; then
  if python3 - "$API_COPY" <<'PY'
import sys, re, yaml
p = sys.argv[1]
txt = open(p).read()
try:
    d = yaml.safe_load(txt)
except Exception as e:
    print("parse error: %s" % e); sys.exit(1)
refs = set(re.findall(r"\$ref: '#/components/schemas/([A-Za-z0-9]+)'", txt))
schemas = set((d.get('components', {}).get('schemas') or {}).keys())
missing = sorted(refs - schemas)
if missing:
    print("unresolved schema $refs: %s" % ", ".join(missing)); sys.exit(1)
sys.exit(0)
PY
  then ok "OpenAPI parses; all schema \$refs resolve"
  else bad "OpenAPI invalid (see message above)"
  fi
else
  skip "OpenAPI parse/ref check" "python3 not found"
fi

if [ "$FAST" = 1 ]; then
  [ "$fail" = 0 ] && note "== fast gate OK ==" || note "== fast gate FAILED =="
  exit "$fail"
fi

# --- 3. routes <-> spec contract test ---------------------------------------
# No-op until a TestRoutesMatchOpenAPISpec-style contract test exists
# (SP1+, once the spec is code-generated from the handlers).
if [ -f "$EMBEDDED" ] && command -v go >/dev/null 2>&1; then
  if go test ./internal/adapters/http/ -run TestRoutesMatchOpenAPISpec -count=1 >/tmp/doc-drift-contract.log 2>&1; then
    ok "routes <-> spec contract test passes"
  else
    bad "routes <-> spec contract test FAILED:"; sed 's/^/      /' /tmp/doc-drift-contract.log >&2
  fi
else
  skip "routes <-> spec contract test" "no contract test yet (SP1+)"
fi

# --- 4. oasdiff: no breaking (ERR) changes vs origin/master -----------------
OASDIFF="$(command -v oasdiff || echo "$(go env GOPATH 2>/dev/null)/bin/oasdiff")"
if [ -x "$OASDIFF" ] && git rev-parse --verify -q origin/master >/dev/null 2>&1; then
  if git show origin/master:"$API_COPY" > /tmp/doc-drift-base.yaml 2>/dev/null; then
    if "$OASDIFF" breaking /tmp/doc-drift-base.yaml "$API_COPY" --fail-on ERR >/tmp/doc-drift-oasdiff.log 2>&1; then
      ok "oasdiff: no breaking changes vs origin/master"
    else
      bad "oasdiff: breaking change(s) vs origin/master:"; sed 's/^/      /' /tmp/doc-drift-oasdiff.log >&2
    fi
  else
    skip "oasdiff breaking check" "no baseline spec on origin/master yet"
  fi
else
  skip "oasdiff breaking check" "oasdiff or origin/master unavailable"
fi

# --- 5. mkdocs --strict ------------------------------------------------------
if command -v uvx >/dev/null 2>&1; then
  if uvx --with mkdocs-material mkdocs build --strict >/tmp/doc-drift-mkdocs.log 2>&1; then
    ok "mkdocs --strict builds"
    rm -rf site 2>/dev/null || true
  else
    bad "mkdocs --strict failed (broken links / nav):"; tail -5 /tmp/doc-drift-mkdocs.log | sed 's/^/      /' >&2
  fi
else
  skip "mkdocs --strict build" "uvx not found"
fi

if [ "$fail" = 0 ]; then
  note "== doc-drift harness: GREEN (mechanical drift = 0) =="
else
  note "== doc-drift harness: DRIFT DETECTED =="
fi
exit "$fail"
