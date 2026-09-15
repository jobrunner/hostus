#!/usr/bin/env bash
# Run gremlins against a slim COPY of the working tree instead of the working
# tree itself, then exec it with the arguments given.
#
# WHY THIS EXISTS — measured, not hypothetical:
# gremlins copies the whole module directory into a fresh workdir FOR EVERY
# MUTANT (workdir.go: os.Mkdir(dst, mode) + copy). It knows nothing about
# .gitignore and nothing about the difference between source and data, so it
# happily duplicates whatever else lives next to the code. In this repo that
# is `out/` (locally built databases, 8.5 GB) and `poc/` (a separate Go module
# with 6.5 GB of spike data) — 16 GB per mutant, times ~380 mutants per run.
# Three runs in one session left 451 GB / 318 GB of temp copies behind and
# once filled the disk outright; under that pressure a run even reported a
# FALSE `Not covered: 3`, i.e. the gate itself became untrustworthy.
#
# A slim copy of the same tree measures 5.6 MB — a factor of ~2900. The
# databases stay exactly where they are and stay usable; only the mutation run
# moves out of their way.
#
# Why a copy and not `git worktree`: an implementer runs the gate on work that
# is not committed yet, including brand-new, still-untracked test files. A
# worktree would silently test the last commit instead — a gate that reports
# on the wrong code is worse than no gate. rsync of the live tree keeps
# uncommitted and untracked files and drops only the excluded paths.
set -euo pipefail

if [ "$#" -lt 1 ]; then
	echo "usage: $0 <gremlins-arg>..." >&2
	exit 2
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"

# EXCLUDES are data and build output only — never source, never testdata.
#
# `poc/` is safe to drop wholesale because it is a SEPARATE Go module
# (poc/go.mod), so no package under test can reach into it.
#
# `pipelines/` is NOT: internal/adapters/sqlite/translate_test.go reads
# pipelines/cdm/fixtures/*.csv at run time, and those fixtures are tracked.
# Only the generated bulk below it goes — the per-pipeline `output/` (canonical
# CSVs, rebuilt by build.sh) and `.cache/` (downloaded sources). Together
# ~390 MB of the 392 MB that directory holds; the 216 KB of tracked scripts and
# fixtures stay.
#
# Verified before excluding: no test under internal/ or cmd/ opens a file
# under `poc/`, `out/`, `pipelines/*/output/` or `pipelines/*/.cache/`.
EXCLUDES=(
	--exclude=.git/
	--exclude=out/
	--exclude=poc/
	--exclude=pipelines/*/output/
	--exclude=pipelines/*/.cache/
	--exclude=.go
	--exclude=/hostus
	--exclude=coverage/
	--exclude=build/
	--exclude=dist/
	--exclude=site/
	--exclude=.worktrees/
	--exclude=.superpowers/
	--exclude=.playwright-mcp/
)

if ! command -v rsync >/dev/null 2>&1; then
	echo "mutation-sandbox: rsync not found — running gremlins in the working tree." >&2
	echo "  On a tree carrying large untracked data this can fill the disk (see this file's header)." >&2
	exec gremlins "$@"
fi

sandbox="$(mktemp -d "${TMPDIR:-/tmp}/hostus-mutation-XXXXXX")"
cleanup() { rm -rf "$sandbox"; }
trap cleanup EXIT INT TERM

rsync -a "${EXCLUDES[@]}" "$repo_root"/ "$sandbox"/

# Report what was kept, so a surprising gate result can be traced back to a
# surprising sandbox rather than being silently trusted.
printf 'mutation-sandbox: %s (%s)\n' "$sandbox" "$(du -sh "$sandbox" | cut -f1)" >&2

cd "$sandbox"
gremlins "$@"
