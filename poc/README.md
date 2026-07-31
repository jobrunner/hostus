# PoC / Annahmen-Verifikation (Phase 0)

Throwaway code verifying spec assumptions against real data. NOT part of `make verify` (own module). Each PoC has a findings file `PXX-findings.md` with a 🟢/🔴 verdict; see GATE.md for the roll-up.

Data downloads land in `poc/data/` (gitignored). Fixtures for reuse go under the service module`s `internal/adapters/*/testdata/`.
