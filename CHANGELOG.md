# Changelog

Alle wesentlichen Änderungen an diesem Projekt werden in dieser Datei dokumentiert.

Das Format basiert auf [Keep a Changelog](https://keepachangelog.com/de/1.0.0/),
und dieses Projekt folgt [Semantic Versioning](https://semver.org/lang/de/).

## [Unreleased]

hostus wird vom zustandslosen GBIF-Autosuggest-Proxy zum lokalen
Multi-Backbone-Namens- und Merkmalsservice umgebaut (siehe
`docs/superpowers/specs/2026-07-31-hostus-2.0-architecture.md`). Dieser
Abschnitt sammelt den Abschluss von **SP0 (Harness & Skelett)**, **SP1
(Foundation)** und **SP2 (Suggest + Offline-Bundle)**: SP1 liefert das
lokale SQLite/FTS5-Rückgrat selbst — Ingest eines WCVP/POWO-DwC-A-Manifests
(`hostus ingest`) in eine versionierte lokale Datenbank, gruppiert nach
akzeptierten Concepts mit ihren Synonymen, sowie die ersten drei
`/v1`-Leseendpunkte (`GET /v1/concept/{id}`, `GET /v1/xref`, `POST
/v1/match`) darauf, an die `/health/ready` jetzt gekoppelt ist. SP2 baut
darauf den Frontend-Autosuggest-Endpunkt (`GET /v1/suggest`, gebiets- und
rangfiltert, priorisiert) sowie den gebietsgescopten Offline-Export
(`hostus bundle`) für feldeinsatztaugliches, vollständig
netzwerkunabhängiges Serving. **SP3 (Traits + Fuzzy-Match)** ergänzt
ökologische Merkmalswerte (EIVE, Tichý et al. 2023, Midolo et al. 2023) und
ein zweites Klassifikationsmerkmal — Namens-Ähnlichkeitsabgleich für
Vegetationsaufnahme-Importe mit Tippfehlern:

- Drei Trait-Pipelines (`pipelines/{eive,tichy,midolo}/build.sh`,
  xlsx/csv → gemeinsames kanonisches CSV-Format) plus Reader
  (`internal/adapters/traits`) und Fixtures, gegen die echten
  Zenodo-Downloads gezogen; `internal/domain.ScaleFor` dokumentiert pro
  (Vokabular, Dimension) Min/Max/Normalisierungsstatus anhand der
  tatsächlich gemessenen Wertebereiche (Tichý T/M reichen bis 12, nicht
  wie beim „klassischen" Ellenberg-Schema bis 9)
- Namens-Crosswalk-Ingest (`application.IngestTraits`,
  `trait_value`/`trait_vocabulary`-Tabellen): jede Trait-Tabellenzeile trägt
  nur einen rohen Taxon-Namensstring (keine WCVP-/POWO-ID), der Abgleich
  gegen den ingestierten Backbone ist daher verlustbehaftet — `hostus
  ingest` gibt `matched`/`unmatched`/`ambiguous` je Vokabular UND eine
  Stichprobe der nicht zugeordneten Namen aus, statt den Verlust
  stillschweigend zu verschlucken
- `GET /v1/concept/{id}/traits?vocab=` — Merkmalswerte gruppiert PRO
  Vokabular (nie zusammengeführt, da deren Taxonomie-Namensräume
  nachweislich divergieren), jeder Wert trägt seine eigene `scale`
  (nicht das ganze Set), da selbst innerhalb eines Vokabulars die Skalen
  zwischen Dimensionen abweichen (Tichý T: 1–12 vs. L: 1–9)
- `POST /v1/match`: neue Klassifikation `fuzzy` (normalisierte
  Levenshtein-Ähnlichkeit, Schwelle 0.85) als Auffangnetz, nachdem
  `exact`/`exact_author`/`aggregate_alias` nichts fanden — `fuzzy` setzt
  `requires_review` IMMER auf `true`, unabhängig von der Ähnlichkeits-Score
- `GET /v1/concept/{id}`: `classification` (Eltern-Kette, root-first,
  tiefenbegrenzt) und `synonyms[].homotypic` (nur `true`, wenn die
  Basionym-Verknüpfung ein gemeinsames Basionym beweist, sonst fehlt das
  Feld statt fälschlich `false` zu behaupten)
- `hostus bundle`: die im "Bekannte Einschränkungen"-Abschnitt (unten)
  angekündigte Gefahrenstelle — ein gebietsgescoptes Bundle, dessen
  Concept/Name auf ein NICHT mitkopiertes Parent/Basionym außerhalb des
  Gebiets verweist — ist jetzt eingetreten (SP3 befüllt `parent_id`
  erstmals) und behoben: `ExportBundle` NULLt gebietsfremde
  Selbstreferenzen, statt mit einem FK-Fehler abzubrechen;
  `trait_value`/`trait_vocabulary` werden ebenfalls gebietsgescopt mit
  exportiert, ein Bundle bleibt also vollständig offline abfragbar
  (Concept, Suggest, Traits) — end-to-end belegt in
  `internal/app/integration_test.go`
- **Bewusster Lizenz-Scope-Schnitt (PoC R1)**: Euro+Med PlantBase,
  GermanSL, EuroSL und die FloraVeg.EU-Downloads werden NICHT ingestiert —
  für keine der vier Quellen ließ sich eine belastbare
  Weiterverbreitungslizenz feststellen (`docs/research/quellenregister.md`).
  Der taxonomische Anschluss für EIVE/Tichý/Midolo läuft deshalb direkt
  gegen den WCVP/POWO-Backbone, nicht über eine dieser vier Quellen als
  Vermittlungsschicht — dokumentiert in
  `docs/how-to/trait-ingest.md`, nicht stillschweigend übergangen.

Weitere Backbones (COL XR) sowie `translate` und `/openapi` als generierte
Spezifikation folgen in SP4+. `release-please` cuttet daraus das nächste
`2.0.0-alpha.N`-Release; bis dahin akkumulieren PRs hier.

### Fixed
- Trait-Ingest schreibt keine `backbone_version`-Zeilen mehr: er lief zuvor
  über `BeginIngest` mit einer synthetischen `trait:<vokabular>`-Version,
  die im `backbone_versions`-Provenienzblock JEDER `/v1/suggest`- und
  `/v1/match`-Antwort auftauchte (ein Trait-Vokabular als taxonomischer
  Backbone ausgewiesen) und `/health/ready` verfälschte. Neuer Port
  `output.Repository.BeginTraitIngest` öffnet die Transaktion ohne
  Backbone-Eintrag.
- Trait-Ingest gleicht die `(vocab, version)`-Identität aus Manifest und
  kanonischer CSV jetzt zeilenweise ab und bricht bei Abweichung mit einer
  Fehlermeldung ab, die beide Seiten nennt, statt Werte unter fremder
  Identität zu speichern (ein `id: eive`, das auf Tichýs CSV zeigt, hätte
  dessen 1–12-Werte auf EIVEs normalisierter 0–10-Skala ausgeliefert);
  persistiert wird die im Manifest gepinnte Version. `dataset.example.yaml`
  pinnte für Tichý/Midolo Versionen, die die Pipelines gar nicht emittieren
  (`1.0` statt `2.0`/`3`) — korrigiert.
- `dataset.example.yaml` verweist für Tichý/Midolo nicht mehr auf
  `floraveg.eu/download/...` (Lizenz ungeklärt, PoC-R1-Scope-Schnitt),
  sondern auf die Zenodo-DOIs der CC-BY-4.0-Originaldatensätze — der Wert
  wird in `trait_vocabulary.source_url` persistiert, über die API
  ausgeliefert und in jedes Offline-Bundle kopiert.
- Trait-CSV-Reader: die Kurzzeilen-Prüfung ging von genau sieben Spalten
  aus und verursachte einen Panic bei einem Header mit ZUSÄTZLICHEN Spalten (die der Reader
  mit `FieldsPerRecord = -1` ausdrücklich zulässt); sie orientiert sich
  jetzt an der tatsächlichen Position der gesuchten Spalten.
- `POST /v1/match`: der Aggregat-Pfad wählte bei mehreren Treffern
  `candidates[0]`, auch wenn diese auf VERSCHIEDENE Concepts zeigten. Er
  wendet jetzt dieselbe Prüfung an wie der exact- und der fuzzy-Pfad:
  `requires_review: true`, alle Kandidaten gelistet, kein geratenes
  `concept_id`.
- Fuzzy-Prefilter der SQLite-Adapter: das GLOB-Präfix aus dem ersten
  Zeichen der Anfrage wird escaped — ein führendes `[` erzeugte zuvor eine
  unterminierte Zeichenklasse (fand nichts), ein führendes `*`/`?` machte
  den Prefilter zum No-op (voller Tabellenscan).
- `docs/how-to/trait-ingest.md` nannte einen Pfad
  (`pipelines/tichy/output/tichy2023-canonical.csv`), den die Pipeline nicht
  erzeugt.
- `google.golang.org/grpc` auf v1.82.1 angehoben (behebt GO-2026-6061 in der
  OTLP-gRPC-Exporter-Kette von `internal/adapters/telemetry`).
- Go-Toolchain auf 1.26.5 angehoben (`go.mod` toolchain-Direktive,
  Dockerfile-Builder-Image) — behebt drei aufrufbare Standardbibliotheks-CVEs
  (GO-2026-5856 `crypto/tls`, GO-2026-5039 `net/textproto`, GO-2026-5037
  `crypto/x509`). `govulncheck ./...` meldet danach 0 aufrufbare
  Vulnerabilities; `make security-check` ist grün.
- v1-GBIF-Proxy-Wortlaut aus operator-/user-facing Oberflächen entfernt
  (CLI-Hilfetext, CORS-Kommentar, README/OpenAPI/Docs, gomodguard-
  Begründungen), die dem SP1-SQLite-Plan widersprachen.
- `example.env`/`docker-compose.yml` auf die tatsächlichen `HOSTUS_*`-Keys
  umgestellt; `docker-compose.yml`s Port-Mapping (`HOSTUS_SERVER_PORT`)
  reparlert, das zuvor mit dem falschen `PORT`-Var auseinanderlief.
- Tote v1-Prometheus-Metriken (`hostus_cache_hits_total`,
  `hostus_cache_misses_total`, `hostus_gbif_errors_total`) entfernt, die nie
  inkrementiert wurden; `hostus_rate_limit_rejects_total` und
  `hostus_load_shedding_active` an die aktive Rate-Limit-/Load-Shed-
  Middleware angeschlossen, statt dauerhaft bei Null zu stehen.
- Totes `internal/cache` (kein Importer, ungestoppter Cleanup-Ticker/
  Goroutine-Leak) entfernt — passte nicht zum SP1-SQLite-Design.
- `hostus serve` loggt jetzt zusätzlich zum RingLog (MCP) auch auf stderr,
  inkl. einer Startzeile mit der Listen-Adresse.
- `internal/adapters/sqlite/suggest.go`: der SQL-seitige Fetch-Budget-Query
  (`ORDER BY score ASC LIMIT ?`, bm25-only) konnte in_area-Kandidaten
  (Ranking-Priorität 2 laut Spezifikation §B.1, also ÜBER dem bm25-Score)
  aus dem Kandidaten-Set werfen, bevor `domain.RankSuggestions` überhaupt
  lief — bei einem breiten Prefix mit mehr Treffern als das Budget
  (`max(20, 4×limit)`) konnten dadurch gebietsfremde Treffer vor
  gebietseigenen landen, genau die §B.1-Inversion, die die
  In-Area-Priorisierung verhindern soll. Fix: `ORDER BY in_area DESC, score
  ASC LIMIT ?`, damit in_area-Zeilen das Budget-Fenster überleben; die
  vollständige Mehrschlüssel-Sortierung bleibt weiterhin
  `domain.RankSuggestions`s Aufgabe.
- `GET /v1/suggest`: die 400-Fehlermeldung bei einem unbekannten
  `rank`-Token verkettete `"unknown rank: "` mit `domain.ParseRank`s
  eigenem Fehlertext (`domain: unknown taxon rank "foo"`) und leakte so
  interne Domain-Fehlerformulierungen in die HTTP-Antwort; die Meldung
  benennt das Token jetzt direkt (`unknown rank "foo"`).

### Bekannte Einschränkungen (SP1/SP2, behoben in SP3)
- ~~`ExportBundle` (`internal/adapters/sqlite/bundle.go`) kopiert Zeilen 1:1
  (`copyRows`), ohne Fremdschlüssel-Ziele auf Gebietszugehörigkeit zu
  prüfen: Ein gebietsgescoptes Bundle kopiert nur `taxon_concept`/`name`-
  Zeilen der im Gebiet liegenden Concepts. Solange `taxon_concept.parent_id`
  und `name.basionym_id` (SP1/SP2) für jedes ausgewählte Backbone durchweg
  NULL sind, bleibt das folgenlos. Sobald eine künftige SP diese Spalten
  erstmals befüllt, kann ein Concept/Name außerhalb des Gebiets auf einen
  NICHT mitkopierten Parent/Basionym verweisen — das gebietsgescopte Bundle
  würde dann mit einem FK-Fehler abbrechen. Wer diese Spalten zuerst befüllt,
  muss `ExportBundle` gleichzeitig anpassen (gebietsfremde Selbstreferenzen
  auf NULL setzen, oder die referenzierten Ancestor-Zeilen mit ins Bundle
  ziehen), statt das als Feld-Crash neu zu entdecken.~~ **Eingetreten und
  behoben in SP3**: `taxon_concept.parent_id`/`name.basionym_id` werden jetzt
  befüllt (Classification/Homotypic); `ExportBundle` NULLt gebietsfremde
  Selbstreferenzen statt einen FK-Fehler zu werfen — siehe SP3-Abschnitt oben
  und `internal/app/integration_test.go`s
  `TestIntegration_OfflineBundleConceptSuggestTraitsOffline`.

### Added
- Hexagonales Skelett (`internal/domain`, `application`, `ports/{input,output}`,
  `adapters/{http,mcp,telemetry}`, `app`, `config`) mit `depguard`/`gomodguard`-
  erzwungenen Schichtgrenzen
- OTel-Setup (Traces + Metrics, OTLP-Exporter) inkl. In-Memory-Span-/Log-
  Ringpuffer-Exporter, `otelmux`-instrumentierte Middleware-Kette
  (Request-ID → Logging → Rate-Limiting → Load-Shedding → Timeouts → CORS →
  Metrics)
- Stdio-Debug-MCP (`hostus mcp`, `modelcontextprotocol/go-sdk`): `get_recent_logs`,
  `tail_errors`, `get_trace`, `list_spans` — read-only Logs/Spans für Claude Code
- Cobra-CLI (`serve` als Default, `version`, sowie Stubs für `ingest`,
  `validate`, `bundle` — Implementierung folgt in SP1/SP2)
- viper-Konfiguration mit `HOSTUS_`-Präfix, Priorität `config.yaml` <
  `HOSTUS_*`-Env-Var < CLI-Flag (kein Dotenv-Loader im Binary)
- Fehler-Envelope um `NOT_FOUND` und `UNRESOLVABLE` erweitert (für die
  künftigen `/v1/concept`, `/v1/match`, `/v1/xref`-Endpunkte)
- `verify`-Gate (`fmt-check vet lint test arch debt-guard` + Compile-Check) als
  kanonischer Grün-Check, Tech-Debt-Ratchet-Trio (`.debt-budget`,
  `.coverage-floors`, `scripts/{debt-guard,coverage-gate}.sh`)
- golangci-lint v2 mit hexagonalen `depguard`-Grenzen, govulncheck, gosec,
  go-licenses-Allowlist
- Nix-Flake + direnv (`go_1_26`, projektlokaler GOPATH/GOCACHE), CGO-frei
- CI-Workflows portiert aus ortus (ohne Last-Test-Stack): `ci`, `docker-release`,
  `release-please`, `mutation`, `fuzz`, `codecharta`, `openapi-diff`,
  `commitlint`, `vuln-scan`, `actions-security` (zizmor), `dependabot-auto-merge`,
  `update-skills-submodule`
- `third_party/claude-skills`-Submodul (`jobrunner/claude-skills`, Branch `main`)
  + `.claude/hooks`/`.githooks` (pre-commit: fmt-check → build → debt-guard)
- MkDocs-Material/Diátaxis-Dokumentationsgerüst (Tutorials/How-to/Referenz/
  Erklärung) + OpenAPI-Baseline für den Doc-Drift-Check
- Sechs neue ADRs (`docs/explanation/decisions/0009`–`0014`): lokaler
  Multi-Backbone-Index, SQLite/FTS5-Persistenz, versionierter Artefaktvertrag
  (`dataset.yaml`), hexagonale Architektur, OpenTelemetry von Tag 1,
  stdio-Debug-MCP
- `.goreleaser.yml` (portiert aus ortus, `release.mode: append`, CGO-frei,
  linux/darwin × amd64/arm64)
- **SP1**: lokales SQLite/FTS5-Rückgrat (`internal/adapters/sqlite`):
  `taxon_concept`/`taxon_name`/`concept_name`/`xref`/`distribution`/
  `backbone_version`-Schema, `output.Repository`-Port (`Concept`,
  `ConceptByXref`, `MatchExact`, `BackboneVersions`, `BeginIngest`/`IngestTx`)
- **SP1**: versionierter Dataset-Manifest-Adapter (`internal/adapters/manifest`,
  `dataset.yaml`, schema-validiert, SHA-inhaltsadressiert) und WCVP/POWO-
  DwC-A-Reader (`internal/adapters/wcvp`)
- **SP1**: `application.Ingest` — liest ein Manifest-Backbone in zwei Durchgängen
  ein (Namen + akzeptierte Concepts, dann Synonym-Verknüpfung unter ihr
  akzeptiertes Concept), meldet `names`/`concepts`/`synonyms`/`orphaned`
  je Backbone; `hostus ingest --dataset <manifest> --db <file>` als CLI-Einstieg
- **SP1**: `application.MatchNames` — klassifiziert verbatime Namen als
  `exact`, `exact_author`, `aggregate_alias` oder `unresolvable`
  (Autoren-Mehrdeutigkeit liefert Kandidaten statt Fehlklassifikation)
- **SP1**: `GET /v1/concept/{id}`, `GET /v1/xref`, `POST /v1/match` auf dem
  neuen Repository; `GET /health/ready` jetzt an das Vorhandensein
  mindestens eines eingelesenen Backbones gekoppelt (vorher immer 200)
- **SP1**: `internal/app/integration_test.go` (`integration`-Build-Tag,
  `make test-integration`) — treibt den kompletten Ingest→Serve→Query-Fluss
  über einen echten `httptest.Server` end-to-end
- **SP1**: OpenAPI-Baseline (`api/openapi/openapi.yaml`) und
  `docs/reference/http-api.md` um die drei neuen `/v1`-Endpunkte ergänzt
  (Concept-/Match-DTOs, Fehler-Envelope-Schema)
- **SP2**: `GET /v1/suggest?q=&area=&rank=&limit=` — FTS5-Präfix-Autosuggest
  über den lokalen Index (`internal/adapters/sqlite`: `fts_name`/
  `fts_name_map`, bei `hostus ingest` befüllt), priorisiert nach §B.1
  (Präfix-Treffer vor Nicht-Treffer, im Gebiet vor nicht im Gebiet, akzeptiert
  vor Synonym, breitere vor feineren Rängen, bm25-Score aufsteigend) via
  `application.Suggest`/`domain.RankSuggestions`; fehlendes/leeres `q` liefert
  `400 INVALID_QUERY`
- **SP2**: `hostus bundle --db <db> --area <code> --out <bundle> [--snapshot
  v1]` — exportiert einen gebietsgescopten, eigenständigen SQLite/FTS5-
  Auszug (`sqlite.ExportBundle`) inkl. neu aufgebautem FTS-Index und
  `bundle_meta`-Provenienz; das Bundle ist danach vollständig offline per
  `hostus serve` bedienbar (kein Zugriff auf die Quell-Datenbank oder ein
  Netzwerk nötig) — siehe `docs/how-to/offline-bundle.md`
- **SP2**: SQLite-Verbindung auf `journal_mode=WAL` + `busy_timeout=5000`
  umgestellt (`internal/adapters/sqlite/db.go`), damit ein laufender
  `serve`-Reader nicht von einem parallelen `ingest`/`bundle`-Writer
  blockiert wird
- **SP2**: `application.MatchNames`-Autoren-Mehrdeutigkeit markiert
  mehrdeutige Treffer jetzt mit `requires_review: true` statt sie
  stillschweigend als `exact_author` durchzuwinken
- **SP2**: `internal/app/integration_test.go` um `GET /v1/suggest`
  (area-priorisiert, `q` fehlt → 400) sowie einen eigenen
  Offline-Bundle-Test erweitert: `sqlite.ExportBundle` scoped auf `area=AUT`,
  danach `application.Suggest` ausschließlich gegen die exportierte
  Bundle-Datei (keine Quell-Datenbank, kein HTTP-Server) — belegt den
  Feldeinsatz-Anspruch end-to-end
- **SP2**: OpenAPI (`/v1/suggest`) und `docs/reference/http-api.md` auf den
  tatsächlichen Handler-Stand reconciled; neue Anleitung
  `docs/how-to/offline-bundle.md` für den `hostus bundle`-CLI-Workflow
- **Reality-Check T1**: `redistribution`-Gate — jeder Backbone- und
  Trait-Vokabular-Eintrag im Manifest trägt jetzt ein Pflichtfeld
  `redistribution: allowed|restricted|unknown` (`internal/domain.Redistribution`,
  schema-validiert). Lokales `hostus ingest` bleibt für jede Quelle
  uneingeschränkt möglich (mit `hinweis:`-Zeile für nicht-`allowed`-Quellen);
  `hostus bundle` verweigert den Export dagegen standardmäßig, sobald eine
  nicht-`allowed`-Quelle zum Export-Scope beiträgt (Fehlermeldung nennt
  Quelle + Wert), und `--force-include-restricted` übersteuert das bewusst,
  protokolliert die betroffenen Quell-IDs aber unauslöschlich in
  `bundle_meta.restricted_sources` — ein Dokumentationsversprechen wird so
  durch eine maschinelle Prüfung ersetzt (siehe
  `docs/how-to/trait-ingest.md`, `docs/how-to/offline-bundle.md`)
- **Reality-Check T2–T4**: Volldaten-Messkampagne gegen den echten
  WCVP/POWO-Dump (1.448.984 Zeilen) plus EIVE/Tichý/Midolo und die vier
  Kandidaten-Brückenquellen Euro+Med, EuroSL, GermanSL, FloraVeg
  (`poc/measure/`, Ergebnisse + Verdikte in `docs/research/reality-check.md`,
  repo-lokal, nicht in der MkDocs-Navigation). Kernbefunde: der
  Serienstand-Ingest ist an Volldaten **nicht einsatzfähig** (bricht nach
  5,37 s an unbekannten WCVP-Rängen ab, oder läuft quadratisch und wurde
  nach 22:48 min ohne einen einzigen committeten Datensatz abgebrochen) —
  mit acht zusätzlichen FK-Indizes und erweiterter Rangunterstützung sinkt
  derselbe Volldaten-Ingest auf 276,70 s; die Zeigerwert-Abdeckung ist auf
  Taxon-Ebene 87,76–96,41 % für die Taxa, die EIVE/Tichý/Midolo tatsächlich
  führen (die oft zitierte 2,64 %-„Abdeckung" bezieht sich auf den globalen
  WCVP-Nenner und ist kein Defektbefund); das gebietsgescopte Offline-Bundle
  ist mit 108,9 MB Faktor 5,4 über der Spec-Annahme von 10–20 MB, und ein
  Mehrgebiets- oder Voll-Bundle ist mit der heutigen CLI nicht exportierbar;
  die vier lizenzunklaren Brückenquellen liefern zusammen real nur 51
  zusätzliche EIVE-Taxa (≈0,34 %), weil Euro+Med und FloraVeg keinen Rang
  und keinen aufgelösten Akzeptiert-Link führen — bessere
  Namensnormalisierung (Aggregate, Hybridzeichen, Autonyme, Orthographie)
  ist laut Stichprobe der lohnendere Hebel als die Lizenzklärung

### Changed
- `Dockerfile`: Build-Stage injiziert `main.Version`/`main.Commit`/`main.BuildDate` per Ldflags (statt `main.version` aus `VERSION`-Datei) — identische Variablenpfade wie im Makefile, damit `hostus version` im Image echte Build-Infos zeigt
- `Dockerfile`: `USER` auf numerische UID:GID (`65532:65532`, distroless "nonroot") statt Namen umgestellt (hadolint DL3066)
- `flake.nix`: `hadolint` zum devShell hinzugefügt (Dockerfile-Linting reproduzierbar verfügbar, auch für CI in S12)
- `docker-compose.yml`: Kommentar zum Healthcheck ergänzt (Exec-Form-Prozess-Check, kein HTTP-Probe — Grund siehe unten)
- `CLAUDE.md`: Reconciled auf hostus 2.0 — Projektbeschreibung, erlaubte
  Bibliotheken, API-Endpunkte, Fehlercodes und Architektur-Abschnitt spiegeln
  jetzt den Multi-Backbone-Service statt des v1-GBIF-Proxys; Git-/CI/CD-
  Workflow-Regeln unverändert
- `architecture/adrs.md`: ADR-001, ADR-003, ADR-008 als **Superseded**
  markiert (Verweis auf ADR-0009/-0010); ADR-004 (Go + Minimal-Dependencies)
  im Wortlaut auf den hostus-2.0-Stack reconciled

### Removed
- `internal/gbif`, GBIF-gestützter `suggest`-Handler und der GBIF-spezifische
  Taxonomie-Mapper (Architektur-Inversion laut Spec Abschnitt 6)
- `Dockerfile`: `HEALTHCHECK`/`COPY VERSION` entfernt. `gcr.io/distroless/static` hat weder Shell noch HTTP-Client (kein curl/wget) — ein In-Container-Probe gegen `GET /health/live` ist technisch nicht möglich. Liveness wird stattdessen extern vom Orchestrator gegen `GET /health/live` geprüft. Ein `hostus health`-Self-Probe-Subcommand wäre eine Option, aber ungetestete Go-Logik außerhalb dieses Tasks — bewusst nicht umgesetzt

## [0.2.4] - 2026-06-16

### Added
- **CI**: Trivy-Filesystem-Scan im Security-Job (`go.sum`, Dockerfile, IaC). Schwellwert: HIGH/CRITICAL, ignoriert unfixable Findings, blockt PR bei Treffern
- **Release**: Trivy-Image-Scan post-build (HIGH/CRITICAL → Release kippt)
- **Release**: Automatische SBOM-Generierung im CycloneDX- und SPDX-Format, angehängt als Release-Asset
- **Release**: BuildKit-Attestationen (`sbom: true`, `provenance: mode=max`) als OCI-Manifest am Image — abrufbar via `docker buildx imagetools inspect`
- **CI/Release**: SARIF-Upload der Trivy-Findings nach GitHub Security ("Code scanning alerts")
- `.trivyignore`-Datei (leer) für künftige bewusst akzeptierte Findings

### Changed
- Permissions `security-events: write` in `ci.yml` und `release.yml` (SARIF-Upload)

## [0.2.3] - 2026-06-13

### Fixed
- Port-Mapping in docker-compose.yml korrigiert (intern und extern dynamisch)

### Changed
- `docker-compose.yml` verwendet jetzt Registry-Image (`ghcr.io/jobrunner/hostus:latest`)

### Added
- `docker-compose.local.yml` als Override für lokale Image-Builds
- Make-Targets: `start` (Registry-Image), `start-local` (lokaler Build), `stop`

## [0.2.2] - 2026-06-13

### Changed
- GitHub Actions auf Node-24-fähige Major-Versionen angehoben (GitHub erzwingt Node 24 ab 2026-06-16):
  - `actions/checkout`: v4 → v6
  - `actions/setup-go`: v5 → v6
  - `actions/upload-artifact`: v4 → v7
  - `golangci/golangci-lint-action`: v7 → v9
  - `docker/setup-qemu-action`: v3 → v4
  - `docker/setup-buildx-action`: v3 → v4
  - `docker/login-action`: v3 → v4
  - `docker/build-push-action`: v6 → v7
  - `softprops/action-gh-release`: v2 → v3

## [0.2.1] - 2026-06-13

### Fixed
- Release-Workflow (`release.yml`): Permission `contents: write` ergänzt, damit `softprops/action-gh-release` GitHub Releases anlegen darf (vorher 403 "Resource not accessible by integration")

## [0.2.0] - 2026-06-13

### Changed
- Go-Toolchain von 1.24 auf 1.26 angehoben (längeres Security-Support-Fenster bis ~Feb 2027)
- `flake.nix`: `pkgs.go_1_24` → `pkgs.go_1_26` (liefert aktuell 1.26.3; zieht automatisch auf 1.26.4 nach, sobald nixpkgs-unstable nachzieht)
- `flake.lock` auf nixpkgs nixos-unstable @ 2026-06-10 aktualisiert
- `go.mod`: `go 1.24.11` → `go 1.26.0` (Mindestanforderung)
- `Dockerfile`: Build-Image `golang:1.24-alpine` → `golang:1.26.4-alpine` (enthält Stdlib-Fixes für GO-2026-5037 und GO-2026-5039)
- CI (`.github/workflows/ci.yml`): `go-version` 1.24 → 1.26.4 in allen Jobs
- CI: `golangci-lint-action` Version v2.7.2 → v2.12.2 (synchron zur Nix-Toolchain)
- Doku in `CLAUDE.md` und `README.dev.md` auf Go 1.26 aktualisiert

### Fixed
- `Makefile`-Target `fmt` formatiert nur noch Projektquellen (`cmd`, `internal`), nicht den GOMODCACHE-Pfad `.go/mod`
- `.golangci.yml`: `goconst` und `noctx` für Test-Dateien ausgeschlossen, `goconst` zusätzlich für `internal/api/openapi.go` (OpenAPI-Datenliteral)

## [0.1.1] - 2025-01-14

### Fixed
- golangci-lint v2 Konfigurationsformat korrigiert
- CI-Pipeline auf golangci-lint-action v7 aktualisiert

### Changed
- Claude Code lokale Einstellungen in .gitignore aufgenommen

## [0.1.0] - 2025-01-13

### Added
- Initiale Projektstruktur
- Go-Modul mit Abhängigkeiten (gorilla/mux, viper, prometheus)
- Konfiguration via CLI, Environment und .env
- GBIF-Client für Taxonomie-Abfragen
- In-Memory Cache mit TTL
- REST-API Endpoint `/api/v1/taxa/suggest`
- OpenAPI-Spezifikation (Code-first mit swaggo)
- Middleware-Chain: Request-ID, Logging, Rate-Limiting, Load-Shedding, Timeouts, CORS, Metrics
- Prometheus Metrics unter `/metrics`
- Dockerfile (Multi-Arch, Distroless)
- docker-compose für lokale Entwicklung
- GitHub Actions für CI/CD
