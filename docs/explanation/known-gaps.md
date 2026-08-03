
## Mutationstest für `internal/adapters/telemetry` blockiert in CI nicht

**Stand:** 2026-08-03 · **Betrifft:** `.github/workflows/mutation.yml`

Das Paket meldet sein Ergebnis, lässt den Job aber nicht rot werden
(`continue-on-error` nur für diesen Matrix-Eintrag).

**Warum.** gremlins kompiliert das Paket je Mutant neu, und dieses eine zieht das
komplette OpenTelemetry-SDK. Ein einzelner Mutant auf einer Kapazitätsangabe
(`memory.go:173`, `make(map[string]string, rec.NumAttrs()+len(r.attrs))`) sprengt
die 7 GB eines `ubuntu-latest`-Runners: im Log ticken die Mutanten in ~1,75 s
durch, dann 60 s Stille, dann „the runner has received a shutdown signal" und
exit 143. Nacheinander erfolglos versucht: eigener Runner je Paket (Matrix),
`GOFLAGS=-p=1`, `GOMEMLIMIT=4GiB`, `--workers 1`. Jede Maßnahme half, keine
reichte.

**Das Paket ist nicht ungeprüft.** Lokal läuft es vollständig durch:
53 killed / 1 lived / 2 not covered, **98,15 % efficacy**. Der eine Überlebende
ist genau jene Kapazitätsangabe — ein äquivalenter Mutant, den kein Test töten
kann, weil `make(m, n)` nur die Vorab-Allokation betrifft.

**Was es wirklich lösen würde**, in absteigender Präferenz: einen Runner mit mehr
RAM (`ubuntu-latest-4-core` o. ä.); oder die Kapazitätsangabe streichen
(`make(map[string]string)`), was den Mutanten ersatzlos entfernt und messbar
nichts kostet — dieselbe Lösung, die SP6 für ein `1+len()+len()` gewählt hat.

**Bis dahin:** vor einer Änderung an `internal/adapters/telemetry` lokal
`make mutation PKG=./internal/adapters/telemetry` laufen lassen.
