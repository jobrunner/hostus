# Changelog

Alle wesentlichen Änderungen an diesem Projekt werden in dieser Datei dokumentiert.

Das Format basiert auf [Keep a Changelog](https://keepachangelog.com/de/1.0.0/),
und dieses Projekt folgt [Semantic Versioning](https://semver.org/lang/de/).

## [2.3.2-alpha.0](https://github.com/jobrunner/hostus/compare/v2.3.1-alpha.0...v2.3.2-alpha.0) (2026-08-19)


### Bug Fixes

* **test:** reject malformed hex so the contrast check cannot pass vacuously ([8041f48](https://github.com/jobrunner/hostus/commit/8041f48a3e71995b3fc12beee978d3179d181d4f))

## [2.3.1-alpha.0](https://github.com/jobrunner/hostus/compare/v2.3.0-alpha.0...v2.3.1-alpha.0) (2026-08-19)


### Bug Fixes

* **ui:** remove four measured accessibility barriers from the console ([4877a23](https://github.com/jobrunner/hostus/commit/4877a230b6e11a0ab566b9c2b7f1819dbd0a76fa))
* **ui:** remove four measured accessibility barriers from the console ([81f3b94](https://github.com/jobrunner/hostus/commit/81f3b9452ef004d28fb3bc2657bd97bcf4fb4de3))
* **ui:** scope the a11y fixes to the cases that actually need them ([15b35e6](https://github.com/jobrunner/hostus/commit/15b35e6a5a93360ba5aeb6d7778665173dfe027c))

## [2.3.0-alpha.0](https://github.com/jobrunner/hostus/compare/v2.2.0-alpha.0...v2.3.0-alpha.0) (2026-08-19)


### Features

* **suggest:** entry_backbone filter, consistent with POST /v1/match ([d785de3](https://github.com/jobrunner/hostus/commit/d785de30bd2214fa1241754564e4116b3db95f12))
* **suggest:** entry_backbone filter, consistent with POST /v1/match ([c9e8c6d](https://github.com/jobrunner/hostus/commit/c9e8c6db6a6c5dd17e94b5fa0370c8db2e39ff6d))
* **ui:** show the running hostus version in the console footer ([3e80e5d](https://github.com/jobrunner/hostus/commit/3e80e5dcb4b941c48b932ade33dc9c69c4c40224))
* **ui:** show the running hostus version in the console footer ([9a06dc3](https://github.com/jobrunner/hostus/commit/9a06dc30b843828ecd461692037c1dbf5fd3b9f9))

## [2.2.0-alpha.0](https://github.com/jobrunner/hostus/compare/v2.1.1-alpha.0...v2.2.0-alpha.0) (2026-08-17)


### Features

* **match:** homotypic tie-break — resolve a synonym name to its genuine bearer ([f9f7888](https://github.com/jobrunner/hostus/commit/f9f7888d5d5ac5de03a7af956782adaff53f0014))
* **match:** homotypic tie-break — resolve a synonym name to its genuine bearer ([e9c15a3](https://github.com/jobrunner/hostus/commit/e9c15a34bc06f113473be59aa56b636b7a500f9e))

## [2.1.1-alpha.0](https://github.com/jobrunner/hostus/compare/v2.1.0-alpha.0...v2.1.1-alpha.0) (2026-08-16)


### Bug Fixes

* **serve:** never build distribution_effective on the Open/serve path ([6ba2cce](https://github.com/jobrunner/hostus/commit/6ba2cce3a5ac121a1e774e67f9750a70c71e6132))
* **serve:** never build distribution_effective on the Open/serve path (startup outage) ([322a10b](https://github.com/jobrunner/hostus/commit/322a10bd280c2949f86a31b2a6a8743a0220768a))

## [2.1.0-alpha.0](https://github.com/jobrunner/hostus/compare/v2.0.0-alpha.0...v2.1.0-alpha.0) (2026-08-16)


### Features

* **areas:** GET /v1/areas + region combobox (name + code) ([7a0a2e0](https://github.com/jobrunner/hostus/commit/7a0a2e002ec32bebc683c7bc2e44dc3789b77c11))
* **areas:** GET /v1/areas with self-sourced names + console combobox ([54833bf](https://github.com/jobrunner/hostus/commit/54833bf87e48c4350ddb8cea5b33d0bd4918d09e))
* **concept:** emit sec {id,title} for sec-bearing concepts ([2194b55](https://github.com/jobrunner/hostus/commit/2194b558254fac1fa6d1380d76ba66092dbbde27))
* **config:** ui.enabled toggle, default on ([b32e26d](https://github.com/jobrunner/hostus/commit/b32e26d0ead523e49dd4ba03d635511eee37f22c))
* **console:** name the source in the sec column + clearable comboboxes ([6eb48ac](https://github.com/jobrunner/hostus/commit/6eb48ac50aad25e6a545f2b56a566af0cd97486a))
* **console:** sec. reference + field help; in_area as positive presence evidence ([57768b9](https://github.com/jobrunner/hostus/commit/57768b9ffbde4702b1d6bf19020e63cb422ac787))
* **console:** show sec reference in Panels 1+2 and add field explanations ([9725e97](https://github.com/jobrunner/hostus/commit/9725e97cadba470c4ecee883f385c76d7dd6f961))
* **console:** show sec reference title in Panel 4 (Translate) too ([2ff0321](https://github.com/jobrunner/hostus/commit/2ff03211e7d1a1885b0fe305ace9f7803846a492))
* **domain:** StripAggregateMarkers reusing the aggregate marker set ([927369a](https://github.com/jobrunner/hostus/commit/927369ad59fc7ed29590f884d37923fde30c7a68))
* **floraveg:** ingest the FloraVeg name space ([f1c0d2a](https://github.com/jobrunner/hostus/commit/f1c0d2ab2fbc3113dc003e4d437c530e1cf1a466))
* **http:** expose SuggestItem.aggregate + console badge ([ebd573f](https://github.com/jobrunner/hostus/commit/ebd573f6afa49eccf1c0640a995583c2bf61db6d))
* **ingest:** nom_status drift signal + TaxonRow mapper guard ([f962f0b](https://github.com/jobrunner/hostus/commit/f962f0b56fcbf7b76f91c8ee37db515b5e79c958))
* **ingest:** nom_status drift signal + TaxonRow mapper guard (debt batch 2) ([bf74acc](https://github.com/jobrunner/hostus/commit/bf74acc760d3084b3ff17e0c69b677162cb1d239))
* **ingest:** rebuild distribution closure after all backbones ([41ab355](https://github.com/jobrunner/hostus/commit/41ab35540a97e08fbb815dc7820bf2e5c4b6ab72))
* **match:** entry_backbone/entry_sec resolution filter ([0364e1b](https://github.com/jobrunner/hostus/commit/0364e1be675d6f0296bb92b576fcf3407de3c2e2))
* **match:** target_space and aggregate_policy for UC4 ([a88f55b](https://github.com/jobrunner/hostus/commit/a88f55bda9ac8dfae890c8ccff9d4c2f6eab4bf7))
* **openapi:** deterministic schema-content check (DTOs vs component schemas) ([ec90b36](https://github.com/jobrunner/hostus/commit/ec90b36ce79d8d6b28207be89965d72425832ef2))
* **openapi:** deterministic schema-content check (DTOs vs component schemas) ([2b83a51](https://github.com/jobrunner/hostus/commit/2b83a51739ac170c1e1af4612253b9f1d4e2ea05))
* **openapi:** route&lt;-&gt;spec contract test closes the drift gap (SP6) ([17146e0](https://github.com/jobrunner/hostus/commit/17146e093432af393b46fe035d2ce82aaac40eef))
* **openapi:** route&lt;-&gt;spec contract test closes the hand-maintained-spec drift gap ([5f18818](https://github.com/jobrunner/hostus/commit/5f188185acc519a018c7a84c03426626f6530feb))
* **port:** add BuildDistributionClosure to Repository ([e16b348](https://github.com/jobrunner/hostus/commit/e16b34837444c9702fcb7fc13e1bbd58c16ccf63))
* **sec:** add GET /v1/sec to list sec. reference spaces ([b516877](https://github.com/jobrunner/hostus/commit/b516877dc4272d4cd14caf189714e7e251b752a9))
* **sec:** add GET /v1/sec to list sec. reference spaces (SP8) ([e3a6990](https://github.com/jobrunner/hostus/commit/e3a69909a38f9cd472f99ac2019a80f34fd26620))
* **sp5:** sec resolution filter + sec output ([956d669](https://github.com/jobrunner/hostus/commit/956d669f6cd4c50d6de199ec1611a693e6b83c36))
* **sp9:** UC4 target_space and aggregate_policy ([82f0243](https://github.com/jobrunner/hostus/commit/82f02436b4cbbd895e9274aa600b4996c0b19fd1))
* **sqlite:** BuildDistributionClosure (own ∪ resolved name-fallback) ([465e892](https://github.com/jobrunner/hostus/commit/465e8927c7b16003873d9d8042a94520c4d3ad8e))
* **sqlite:** self-heal distribution closure on Open ([0239710](https://github.com/jobrunner/hostus/commit/0239710f20ae1355e9b203b3958792ef1c49ca71))
* **suggest:** emit sec {id,title} per candidate ([ce2b839](https://github.com/jobrunner/hostus/commit/ce2b839afcc8985c4b0d181ecd6321a3551a4a6b))
* **suggest:** in_area as positive presence evidence (WCVP-name fallback), never a false "nein" ([a8a0800](https://github.com/jobrunner/hostus/commit/a8a0800e365648f81ebb5fb597cc5decb8c8bc35))
* **suggest:** index aggregate name-space aliases + carry an aggregate flag ([a76ca8e](https://github.com/jobrunner/hostus/commit/a76ca8e7b16e5481826ff8a2de9595a01e3d8166))
* **suggest:** marker-insensitive aggregate suggest with aggregate badge ([4423cf6](https://github.com/jobrunner/hostus/commit/4423cf6f1b161035817e3efb1e4ee7b8210f73a0))
* **translate:** entry_backbone/entry_sec for the verbatim entry ([fef967e](https://github.com/jobrunner/hostus/commit/fef967ed820d4a23a6edb9071e3ccef8c18306ee))
* **ui:** embedded single-page test console ([5b86c24](https://github.com/jobrunner/hostus/commit/5b86c24ebc43f96cd87fb4eb84abd5a20faf3a82))


### Bug Fixes

* **bundle:** carry the area name table into exported bundles ([18107af](https://github.com/jobrunner/hostus/commit/18107af611442e14b9e6ab9b702d99fcc198aa5c))
* **console+suggest:** review findings — sec fallback on empty title, guard test, doc list ([2b9f95c](https://github.com/jobrunner/hostus/commit/2b9f95c977b0b52e3153365338e73577bc098726))
* **dev:** guard .go cleanup against symlinks (Copilot review) ([e5f7b1d](https://github.com/jobrunner/hostus/commit/e5f7b1de2ef7b501a9d046e85330a2b52de919e5))
* **dev:** pin Go 1.26.6 + GOTOOLCHAIN=local so local make mutation works ([9980d7b](https://github.com/jobrunner/hostus/commit/9980d7bb304f0b441ffd50bcc0db8919b52991dc))
* **dev:** pin Go to 1.26.6 + GOTOOLCHAIN=local so local mutation testing works ([eabc378](https://github.com/jobrunner/hostus/commit/eabc37811d06f0f900f3a70733764668ef114f19))
* **dev:** remove in-tree .go before gremlins so make mutation is robust ([bed069d](https://github.com/jobrunner/hostus/commit/bed069dcf346633064e09a586c7465d3d618474e))
* **dev:** remove in-tree $PWD/.go before gremlins so make mutation is robust ([6198fd9](https://github.com/jobrunner/hostus/commit/6198fd982c2cf665e575710c4f05aba9c1562d03))
* **match:** push sec filter into the fuzzy prefilter, before the LIMIT ([700270b](https://github.com/jobrunner/hostus/commit/700270b4a438a4a5f975dfbe4ecd9cdfaf2466a9))
* **openapi:** cover /v1/sec body + assert top-level schema type ([e137a15](https://github.com/jobrunner/hostus/commit/e137a151bb1dda67b0058dec01a8d6f50ce82cfa))
* **security:** bump Go toolchain to 1.26.6 (6 stdlib CVEs) ([96ca260](https://github.com/jobrunner/hostus/commit/96ca260f6130b6c0fba6e120b7a721ca3f0db8d3))
* **security:** bump Go toolchain to 1.26.6 for 6 stdlib CVEs ([d6c60ac](https://github.com/jobrunner/hostus/commit/d6c60ac3c96acf6a1de43742ee7820c4b096eccf))
* **sqlite:** fail loudly on schema drift at Open ([77443e4](https://github.com/jobrunner/hostus/commit/77443e488d731e62f732b77891bf346242798630))
* **sqlite:** fail loudly on schema drift at Open instead of 500 per request ([1f45e14](https://github.com/jobrunner/hostus/commit/1f45e14aa391faf157b7a47756feff8e178b6453))
* **suggest:** guard in_area name-fallback against empty canonical_fold (review) ([df6de15](https://github.com/jobrunner/hostus/commit/df6de152336befb00d1a2423f552d432ba2d4402))
* **suggest:** in_area area= query ~30s → ~0.2s (cause of 502s) ([d5e409c](https://github.com/jobrunner/hostus/commit/d5e409cef82ad7eef78108d63e900dfefc6c01a8))
* **suggest:** preserve in-area recall in the match pool (sparse-area fix) ([86896f3](https://github.com/jobrunner/hostus/commit/86896f3194c25078ad6e7872d9da3a502c1638a8))
* **suggest:** stop in_area name-fallback from scanning the whole WCVP backbone ([9eb740a](https://github.com/jobrunner/hostus/commit/9eb740a160a63c52c769e53cfe20bff0ad3d816d))


### Performance Improvements

* **suggest:** bm25 relevance pool — broad short prefixes ~1.8s → ~0.16s ([da92215](https://github.com/jobrunner/hostus/commit/da92215f670241b659c03bc64aa1cbaecbd6746f))
* **suggest:** cap FTS matches to a bm25 relevance pool for broad short prefixes ([c253a31](https://github.com/jobrunner/hostus/commit/c253a3102f4b28e2fddc6bbbabc3bbf584a53edd))
* **suggest:** resolve in_area via distribution_effective (fast + fully correct) ([62a5c27](https://github.com/jobrunner/hostus/commit/62a5c27aa9a06e31c9dec1cf79c6b4e09e6d5500))

## [Unreleased]

### Added (Testkonsole: Konzept- und Namensräume auswählbar)
- **Suggest und Match haben jetzt Bedienelemente für die Räume**, die die API
  längst kann: Konzeptraum (`entry_backbone`, Auswahl inkl. „Alle"), Namensraum
  (`target_space`) und bei Match zusätzlich die Quell-Flora (`entry_sec`). Die
  Auswahllisten werden aus dem Index gefüllt (`GET /v1/backbones`,
  `GET /v1/spaces`) statt fest verdrahtet — welche Räume vorliegen, entscheidet
  das Manifest des Deployments.
- **Suggest zeigt den Namen im gewählten Namensraum als eigene Spalte.** Damit
  sieht man beim Tippen, welcher Kandidat sich z. B. nach `eurosl` (Euro+Med,
  ESy-kompatibel) übertragen lässt — die leere Zelle ist dabei die eigentliche
  Aussage.
- **Translate steht jetzt als Panel 3 direkt hinter dem Konzept**, auf das es
  sich bezieht. Die Beschriftungen benennen den Unterschied, den die API
  verwischt: `target_space` meint bei Translate eine **Flora**
  (`sec.`-Referenz), bei Suggest/Match einen **Namensraum** (`eurosl`).

### Fixed (Zielraum-Name war willkürlich)
- **Der Name im Zielraum ist jetzt bestimmt statt zufällig.** Ein Namensraum
  bildet viele eigene Schreibweisen auf **ein** Backbone-Konzept ab; gemessen am
  realen Euro+Med-Index hatten von 43.545 Konzepten mit eurosl-Eintrag nur
  51,5 % genau einen Namen — der Rest zwei bis 391. Die Quelle sagt, welcher
  akzeptiert ist, aber der Ingest verwarf dieses Feld an der DTO-Grenze. Damit
  lieferte auch `POST /v1/match` seit SP9 einen beliebigen Namen (gemessen:
  „Hyssopus ruber", einer von 23; Gattung *Inula* → „Codonocephalum"). Der
  Status wird jetzt ingestiert und beide Auflösungswege bevorzugen den
  akzeptierten Namen: **Bestimmtheit 51,5 % → 88,3 %** (16.015 zuvor
  willkürliche Konzepte). Der Rest ist in der Quelle selbst mehrdeutig.
  Bestehende Indizes öffnen weiter (`ALTER TABLE`-Migration) und verhalten sich
  wie bisher, **bis neu ingestiert wird**.

### Added (Mutationstest für die Barrierefreiheits-Prüfungen)
- **Die a11y-Prüfungen werden jetzt selbst geprüft.** `make mutation`
  (gremlins) erreicht sie nicht: es mutiert Go-Quelltext, die geprüften
  Eigenschaften stehen aber in `style.css`, `index.html` und `app.js`, die per
  `go:embed` eingebunden sind. Das Gate meldete für das http-Paket
  „Not covered = 0“, ohne je einen Mutanten für die beseitigten Barrieren zu
  erzeugen — die Prüfungen wurden von einem Harness benotet, das sie nie
  getestet hat. Die beim Audit von Hand zurückgenommenen Rückschritte sind
  deshalb jetzt als Code hinterlegt: **14 Mutanten**, jeder einer Prüfung
  zugeordnet, die ihn melden muss. Zwei Meta-Eigenschaften sichern das Gate
  selbst ab — eine entkernte Prüfung fliegt auf, und ein Mutant, dessen
  Suchtext im Asset verschwindet, meldet sich als Blindgänger statt still
  durchzurutschen. Zusätzlich wird der Kontrast des Bedienelement-Randes jetzt
  **gerechnet** statt als Zeichenkette verglichen, sodass ein still
  aufgehellter Farbwert auffällt.

### Fixed (Testkonsole: Barrierefreiheit, WCAG 2.2 AA)
- **Vier gemessene Barrieren in der Testkonsole beseitigt.** Audit mit dem
  `web-accessibility-audit`-Harness (statisches Gate, axe-core im echten DOM,
  plus die manuellen Tastatur-/Zoom-/Bewegungs-Durchgänge):
  - **Breite Tabellen waren nur mit der Maus scrollbar** (WCAG 2.1.1; von axe
    als `serious` gemeldet, sichtbar erst nachdem Ergebnisse geladen sind). Die
    scrollbaren Container sind jetzt Tab-Stopps.
  - **Fehlermeldungen erschienen lautlos** (WCAG 4.1.3). Alle vier Panels
    melden Fehler über dieselbe Stelle, die jetzt `role="alert"` trägt; die
    Suggest-Zusammenfassung ist `role="status"` (bewusst sie und nicht die
    Ergebnistabelle, die sonst bei jedem Tastendruck komplett vorgelesen würde).
  - **Der Rand von Eingabefeldern hatte 1,52:1** statt der geforderten 3:1
    (WCAG 1.4.11). Bedienelemente nutzen jetzt ein eigenes Token
    (`--control-line`, 3,59:1); dekorative Trennlinien dürfen zart bleiben.
  - **Die Checkbox maß 13 px** statt der geforderten 24×24 CSS-Pixel
    (WCAG 2.2 SC 2.5.8).
  Der Tab-Stopp entsteht dabei **nur, solange eine Tabelle wirklich überläuft**
  (per `ResizeObserver` fortlaufend entschieden) — sonst wäre jede passende
  Tabelle ein stummer Halt in der Tab-Reihenfolge. Die Ansagen laufen über
  **eine** knappe, nur für Screenreader sichtbare Live-Region („30 Treffer.")
  statt über die ausführliche Auswertung: die würde eine Tipp-Suche nach jeder
  Eingabepause komplett erneut vorlesen.
  Zusätzlich ein expliziter `:focus-visible`-Ring — der Browser-Default ist auf
  den dunklen Buttons schwach, und die Scroll-Container sind neuerdings
  Tab-Stopps. Geprüft: axe meldet 0 Verstöße im Start- **und** im interaktiven
  Zustand, kein Bedienelement unter 24×24, bei 200 % Textgröße kein
  waagerechter Überlauf, keine Tastaturfalle. Fünf Go-Tests halten die
  Eigenschaften fest, damit sie nicht still zurückfallen.

### Added (Testkonsole: Version im Footer)
- **Die Testkonsole zeigt die laufende hostus-Version im Footer.** Wer zwei
  Deployments vergleicht, sieht damit auf der Seite selbst, welcher Build
  antwortet — ohne Terminal-Zugriff auf den Host. Die Version ist dieselbe, die
  `hostus version` meldet (per LDFLAGS gestempelt); ein ungestempelter Build
  (`go run`, Build ohne Makefile) zeigt `dev`. Der Wert wird HTML-escaped
  eingesetzt und fließt in den ETag der Seite ein, damit ein Browser nach einem
  Update nicht den Footer der Vorversion weiterzeigt.

### Added (Suggest: `entry_backbone` — nur Konzepte einer Backbone vorschlagen)
- **`GET /v1/suggest` akzeptiert `entry_backbone`** (z. B. `wcvp`) — derselbe
  Filter, den `POST /v1/match` unter diesem Namen schon anbietet. Bisher ließ
  sich Suggest nur nach Gebiet (`area`) einschränken, nicht nach Backbone; ein
  Name kann aber je CDM-`sec.`-Referenz einmal vorkommen und verdrängt das
  einzelne WCVP-Konzept von der Ergebnisseite. Gemessen am realen Index:
  `q=Inula&area=GER&limit=20` lieferte **1** WCVP- und 19 CDM-Konzepte (dasselbe
  „Inula" aus verschiedenen deutschen Floren); mit `entry_backbone=wcvp` sind es
  **20** WCVP-Konzepte. Der Filter greift in der SQL-Abfrage, also **vor** dem
  Limit — clientseitiges Nachfiltern behielte bestenfalls eine Zeile. Eine nicht
  ingestierte Backbone liefert `400 INVALID_QUERY` (statt einer leeren Liste,
  die wie „Pflanze unbekannt" aussähe), mit derselben Meldung wie bei `match`.

### Added (Match: homotype Entmehrdeutigung — Synonym-Namen lösen zum echten Namensträger auf)
- **`/v1/match` bricht einen Namens-Tie über die Homotypie auf.** Ist ein
  verbatim Name Synonym unter mehreren Konzepten, gewinnt jetzt das Konzept, in
  dem er der **akzeptierte Name oder ein homotypes Synonym** ist (gleicher
  nomenklatorischer Typus = der echte Namensträger, z. B. *Inula hirta* L. ≡
  *Pentanema hirtum*), statt eines mehrdeutigen `unresolvable`. Zwei echte
  Namensträger (z. B. mehrere CDM-Floren mit akzeptiertem „Inula hirta“, ohne
  `entry_backbone`-Scope) bleiben bewusst mehrdeutig. `MatchExact` reicht dazu
  `concept_name.homotypic` als `MatchCandidate.Homotypic` durch. Mit
  `entry_backbone=wcvp` löst „Inula hirta“ so sauber auf *Pentanema hirtum* auf;
  `target_space=eurosl` liefert zusätzlich den Euro+Med-Namen — der Baustein für
  Listen → Euro+Med/ESy.

### Fixed (serve startet nicht mehr — Container blockiert/OOMt beim Closure-Build)
- **`distribution_effective` wird NIE mehr auf dem serve/`Open`-Pfad gebaut.**
  Seit v2.1.0-alpha.0 baute `sqlite.Open` die Closure selbstheilend, falls leer —
  aber `hostus serve` ruft `Open` **vor** dem HTTP-Listener auf (`app.openRepo`).
  Auf einer DB ohne Closure blockierte der ~55-s-Build (2,8 M-Zeilen-Join+Insert
  in einer Transaktion) den Start, bevor eine einzige Logzeile oder ein Listener
  entstand; in einem speicherbegrenzten Container **OOM-killte** er den Container
  → Restart-Loop, „kommt nicht hoch", Reverse-Proxy ohne Upstream, Log-Viewer
  zeigt „No log line". Der Build ist ein **Build-Artefakt** und läuft jetzt nur
  noch beim **Ingest** (`app.Ingest` → `BuildDistributionClosure`). `Open` macht
  nur noch Schema+Migrationen; serve listet sofort (~0,7 s). Eine DB ohne Closure
  liefert `in_area=false` (fail-safe) bis zum nächsten Ingest — statt gar nicht
  zu starten.
- **Offline-Bundle liefert die Closure fertig mit.** `ExportBundle` baut
  `distribution_effective` (area-gescopt) in das Bundle, da der Consumer sie
  nicht mehr beim Open aufbaut — ein serviertes Bundle liefert so korrektes
  `in_area` ohne Startzeit-Build.

### Changed (Suggest: breite Kurz-Präfixe schnell UND für alle Gebiete korrekt)
- **`in_area` als vorberechnete Verbreitungs-Closure (`distribution_effective`)
  + bm25-Pool.** Ein 2-Zeichen-Präfix wie „ca" matcht am vollen Index ~104k
  Namen; das Joinen, Gruppieren und `in_area`-Berechnen über *alle* kostete
  ~1,8 s (der FTS-Scan selbst ist mit ~12 ms billig — die Kosten stecken im
  Downstream pro Zeile). Zwei zusammenwirkende Änderungen:
  - Die FTS-`matches`-CTE behält nur die Top-`suggestMatchPool` (5000) Treffer
    nach bm25-Relevanz. Für Queries unter der Pool-Größe (Poa, care, praktisch
    alle normalen Queries) ist das ein **No-op**. Damit bei gesetztem `area`
    kein in-area-Treffer mit schwacher Relevanz von Seite 1 fällt (`in_area` ist
    der *primäre* Sortier-Schlüssel; kritisch in **sparse Gebieten**), wird der
    Pool mit allen in-area-Präfix-Treffern **vereinigt**.
  - Der zuvor teure Laufzeit-`in_area`-Namens-Fallback (CDM-Konzepte ohne eigene
    Verbreitung → gleichnamiger WCVP-Zwilling) ist **deterministisch** und wird
    jetzt **einmal beim Ingest** (und selbstheilend beim `Open`) in die
    abgeleitete Tabelle `distribution_effective` (eigene ∪ aufgelöste Zwillings-
    Gebiete, mit `origin`) vorberechnet. `in_area` ist damit zur Laufzeit ein
    indizierter Punktlookup; die ~30-s-Subquery und die beiden `|| ''`-
    Planner-Hacks aus `suggest.go` entfallen. Die Union deckt so **eigene *und*
    CDM**-in-area-Treffer ab.
  Gemessen an Realdaten (`full.sqlite`, `distribution_effective` = 1,98 M `own`
  + 2,80 M `name`): `ca&area=GER` ~1,8 s → **~0,43 s** warm; sparse Gebiete voll
  korrekt — `pa&area=PHX` **50/50 in-area** (vorher 40, die 10 CDM-Taxa wie
  *Panicum dactylon* wieder da), `ca&area=PHX` voll; normale Queries unverändert
  (Poa 0,02 s). „ca" (und jedes Kurz-Präfix) bleibt erlaubt. Der Closure-Build
  läuft einmalig (~55 s auf `full.sqlite`) beim ersten `Open`/Ingest; bestehende
  DBs heilen sich selbst, kein Re-Ingest nötig. (Folge-Schritt: `suggestMatchPool`
  verkleinern, da der Recall nun durch die Closure-Union garantiert ist.)

### Fixed (Suggest `area=…` ~30 s → ~0,2 s; Ursache von 502ern)
- **`in_area`-Namens-Ableitung trieb über den falschen Index.** Die bei
  gesetztem `area` aktive korrelierte Subquery (derselbe Name bei WCVP im
  Gebiet?) wurde von SQLite über `idx_taxon_concept_backbone_id`
  (`backbone_id='wcvp'`) getrieben — ein nahezu die ganze `taxon_concept`-Tabelle
  treffender Filter — und scannte so das komplette WCVP-Backbone **pro
  Kandidatenzeile** (~30 s auf dem vollen Index). Hinter einem Reverse-Proxy
  wurde daraus ein **502/504**; ein Remote-Call findet im Serve-Pfad nicht statt.
  Ein `wtc.backbone_id || ''` (wert-erhaltende Konkatenation → Ausdruck statt Spalte) neutralisiert diesen Index, sodass der
  Planner über den selektiven `idx_name_canonical_fold`-Einstieg geht. Ergebnisse
  identisch. Gemessen an Realdaten: `suggest?...&area=GER` fällt von ~30 s
  (Timeout) auf 0,04–0,38 s.

### Fixed (Dev-Umgebung: lokales `make mutation` bricht nicht mehr ab)
- **Go im Flake auf 1.26.6 gepinnt + `GOTOOLCHAIN=local`.** nixpkgs' `go_1_26`
  liefert 1.26.3; die go.mod-Direktive `toolchain go1.26.6` ließ jeden
  go-Aufruf per `GOTOOLCHAIN=auto` das 1.26.6-Toolchain nachladen und in einen
  re-exec'ten Prozess springen. Dieser Kindprozess erbt `GOPATH`/`GOMODCACHE`
  **nicht** und legt einen **read-only** Modul-Cache unter `$PWD/.go` an.
  gremlins (mutation testing) kopiert pro Mutant das komplette Modulverzeichnis
  — inklusive `.go` — in ein temporäres Workdir; die read-only Verzeichnisse
  brechen den Kopiervorgang mit `permission denied` ab (`gremlins@v0.5.1`
  `workdir.go`, `os.Mkdir(dst, mode)`), sodass `make mutation` lokal panickte.
  Das Flake pinnt go nun via `overrideAttrs` direkt auf 1.26.6 (kein Re-Exec
  nötig, lokal dieselbe Version wie CI) und setzt `GOTOOLCHAIN=local`, was die
  Re-Exec — die häufigste `$PWD/.go`-Quelle (jeder `go build`/`test`) —
  eliminiert. Der devShell-Eintritt selbst kann `$PWD/.go` weiterhin einmalig
  anlegen; damit `make mutation` unabhängig davon zuverlässig läuft, entfernt
  das `mutation`-Target `$PWD/.go` unmittelbar vor gremlins (No-op in CI, wo es
  nicht existiert). Verifiziert: plain `nix develop -c make mutation
  PKG=./internal/adapters/sqlite` läuft grün (`Killed 314, Not covered 0`, 319
  Mutanten). CI ist unberührt (nutzt `setup-go`/`go-version-file`, nicht das
  Flake).

### Security (Go-Toolchain 1.26.5 → 1.26.6)
- **`toolchain go1.26.6` in `go.mod`** behebt sechs von govulncheck gemeldete
  Standardbibliothek-Schwachstellen, alle in go1.26.6 gefixt: GO-2026-6218
  (`net/url`), GO-2026-6091 (`html/template`), GO-2026-6090 (`crypto/tls`),
  GO-2026-6089 + GO-2026-5026 (`net/http`), GO-2026-5972 (`encoding/asn1`).
  Kein Code-Change nötig — die betroffenen Pfade (`net/http`-Serve,
  `crypto/tls`, `html/template`) sind Standardbibliothek. CI baut über
  `setup-go`/`go-version-file: go.mod`; lokal zieht `GOTOOLCHAIN=auto` das
  1.26.6-Toolchain nach. `govulncheck ./...` ist danach clean.

### Added (Testkonsole — sec.-Referenz, Feld-Erläuterungen; `in_area` als Präsenzbeleg)
- **sec.-Referenz in den Panels.** Suggest (Panel 1) und Match (Panel 2) zeigen
  jetzt eine eigene **sec.-Spalte**: den Titel der Konzept-Referenz (`sec.`),
  sonst die Herkunft (`WCVP` bzw. `CDM (ohne sec.)`) statt eines nackten „–";
  Panel 4 (Translate) zeigt den Referenz-**Namen** statt der UUID. Voller Titel
  im `title`-Tooltip, wenn die Zelle kürzt.
- **Feld-Erläuterungen.** Jede Tabellen-/Detail-Überschrift trägt einen
  ⓘ-Tooltip, und je Panel klappt eine „Felder erklärt"-Legende auf (beides aus
  einer Quelle `FIELD_DOCS`); CDM und WCVP werden ausgeschrieben (Common Data
  Model / Cybertaxonomy-EDIT bzw. World Checklist of Vascular Plants, Kew).
- **Kombobox-Löschen.** Alle Komboboxen (Suchbegriff, Gebiet, Translate-Ziel)
  sind `type="search"` und lassen sich per nativem „X" leeren (kein DEL nötig).
- **`in_area` ist ein positiver Präsenzbeleg, nie ein falsches „nein".**
  Verbreitung ist Präsenz-Datum, ein fehlender Datensatz belegt keine
  Abwesenheit. `in_area` ist jetzt „ja", wenn das Konzept selbst im Gebiet
  verbreitet ist **oder** — für ein Konzept ohne eigene Verbreitung (die CDM
  sec.-Konzepte) — derselbe akzeptierte Name (akzeptiert oder als Synonym) auf
  einem WCVP-Konzept im Gebiet vorkommt; sonst „keine Angabe" (nie „nein").
  OpenAPI (`SuggestItem.in_area`) + `http-api.md` beschreiben die
  Präsenz-Semantik. Gemessen an Realdaten: alle CDM-„Inula hirta"-Konzepte
  melden GER=ja über WCVPs *Pentanema hirtum/britannica*.

### Added (`GET /v1/areas` — Region-Kombobox mit Namen + Codes)
- **Neuer Endpunkt `GET /v1/areas`** listet jedes Verbreitungsgebiet mit Daten
  als `{code, name, scheme}` (id-sortiert, `[]` nie `null`, `500` bei
  Repo-Fehler). Die Gebietsnamen werden beim Ingest **selbst beschafft**: die
  bisher verworfene `Locality`-Spalte des WCVP-Distributionsdumps landet in
  einer neuen `area`-Lookup-Tabelle (`INSERT OR IGNORE`, erster nicht-leerer
  Name je (scheme, code)). `Repository.Areas` listet nur Codes **mit Daten**
  (`DISTINCT area_code` LEFT JOIN Name). OpenAPI (`Area`/`AreaListResponse`,
  contract-getestet) + `http-api.md`.
- **Testkonsole: Gebiets-Kombobox.** Das Gebiets-Feld ist jetzt an eine
  `<datalist>` gebunden, die aus `GET /v1/areas` mit „Germany (GER)" gefüllt
  wird; eine getippte Bezeichnung wird client-seitig auf den Code aufgelöst, so
  dass „Germany" **und** „GER" funktionieren. Server-`?area=` bleibt
  code-basiert (plus die Aliase `DE/AT/CH`). Gemessen an Realdaten: 381 Gebiete,
  GER→Germany, FRA→France, AUT→Austria.

### Added (Suggest — Aggregat-Schreibweisen finden das Taxon)
- **Marker-insensitives Aggregat-Suggest.** Der FTS-Query streift den
  Aggregat-Marker ab (`domain.StripAggregateMarkers`, wiederverwendete
  Marker-Definition), sodass `agg./aggr./s.l.` gleichwertig sind und die Basis
  finden. Zusätzlich werden aufgelöste aggregatmarkierte FloraVeg-Namen als
  `fts_name`-Aliase indexiert (`fts_name_map.is_aggregate`); Suggest reicht
  `MAX(is_aggregate)` als `SuggestItem.aggregate` durch (DTO/OpenAPI/`http-api.md`),
  und die Testkonsole zeigt ein „agg."-Badge. Gemessen an Realdaten (WCVP +
  FloraVeg): „Achillea millefolium agg." und „… aggr." liefern beide
  *Achillea millefolium* mit `aggregate=true`. Da FloraVeg-Aggregate auf die
  Nominatart zeigen, ist der Treffer die Nominatart mit Badge, kein separater
  Aggregat-Eintrag.

### Added (OpenAPI — deterministischer Schema-Inhalt-Check)
- **`TestOpenAPISchemasMatchDTOs`** (`internal/adapters/http`) reflektiert über
  jeden Request/Response-DTO und gleicht **rekursiv** jedes der 28
  Component-Schemas ab: Property-Namen, required-Status (`omitempty` ⇔
  `required`), Skalartypen, Array-Element-/Map-Wert-Typen und die
  `$ref`/Inline-Objekt-Verschachtelung. Damit ist neben Pfad+Methode nun auch
  der **Schema-Inhalt** driftsicher: ein umbenanntes, um-typisiertes oder
  (nicht-)optional gemachtes DTO-Feld ohne Spec-Anpassung lässt CI rot werden.
  Nutzt den bereits vorhandenen `go.yaml.in/yaml/v3`-Parser (kein neuer Dep).
  doc-drift Check 3 führt beide OpenAPI-Tests aus. Enum-Werte/Descriptions
  bleiben bewusst außen vor (Prosa, kein Struktur-Vertrag).

### Added (OpenAPI — Routen↔Spec-Contract-Test schließt das Drift-Risiko)
- **`TestRoutesMatchOpenAPISpec`** (`internal/adapters/http`) gleicht jede vom
  Router gemountete Route beidseitig gegen Pfad+Methode in
  `api/openapi/openapi.yaml` ab (mux-`Walk` gegen einen dependency-freien
  Spec-Scanner). Eine undokumentierte neue Route — oder ein Spec-Pfad ohne
  Route — lässt CI rot werden. `scripts/doc-drift-check.sh` führt ihn als
  Check 3 aus; der frühere `skip`-Zweig entfällt. Die handgepflegte Spec kann
  damit **nicht mehr still driften**. Die wörtliche „codegeneriert"-Regel
  bleibt bewusst zurückgestellt (keine schweren Deps); der Contract-Test
  schließt das eigentliche Risiko. Siehe known-gaps.

### Changed (Doku — ESy-Regelwerk-Sondierung, SP9)
- **Spike `docs/research/sp9-esy-spike.md`**: das EUNIS-ESy-Regelwerk ist
  beschaffbar (Zenodo DOI 10.5281/zenodo.3841729, **CC BY 4.0**, ein 1,6-MB-
  Textfile), maschinell parsbar (formale Grammatik, 169 Artengruppen /
  304 Habitat-Regeln, R-Referenzparser Bruelheide et al. 2021), und **66,4 %**
  seiner Art-Namen liegen verbatim im ingestierten FloraVeg-Namensraum. Klare
  Scope-Grenze: volle Plot-Klassifikation braucht Deckungs-/Standortdaten und
  liegt außerhalb eines Namensdienstes; die beantwortbare Frage „ist dieser
  Name eine diagnostische Art?" hängt allein am Regelwerk. known-gaps- und
  SP9/UC4-Verdikt-Dokument entsprechend aktualisiert; `esy_diagnostic_relevance`
  bleibt bis zum Ingest-SP korrekt `not_determinable`.

### Added (`GET /v1/sec` — verfügbare `sec.`-Referenzräume auflisten, SP8)
- **Neuer Endpunkt `GET /v1/sec`** liefert alle ingestierten `sec.`-Räume als
  `{"sec_references":[{"id","title"}]}` (id-sortiert; leere Liste als `[]`,
  nie `null`). Schließt den known-gaps-Befund „kein Endpunkt listet die
  `sec.`-Räume": ein für `POST /v1/translate`/`POST /v1/match` geratenes
  `target_space`/`entry_sec` war von einem leeren Ergebnis nicht zu
  unterscheiden. Der Datenpfad (`repo.SecReferences`) existierte bereits; neu
  sind Handler + Route. Gemessen an Realdaten: 119 Räume.
- **Testkonsole bietet eine Auswahlliste statt Freitext.** Das
  `target_space`-Feld ist jetzt an eine `<datalist>` gebunden, die beim Laden
  aus `GET /v1/sec` gefüllt wird (fällt bei Fehler auf Freitext zurück).

### Changed (Doku — Offline-Bundle-Größe real nachgemessen)
- **`docs/how-to/offline-bundle.md` mit gemessener aktueller GER-Bundle-Größe.**
  Frischer WCVP-+-Trait-Ingest, `--area GER`: **89,2 MiB (93,5 MB)** (11.583
  Konzepte, 169.670 Namen). Das ist +8,15 MiB gegenüber der 81,05-MiB-Messung
  nach Task 4 (reality-check M5.3); da beide gegen verschiedene Ingest-DBs
  gemessen wurden, ist der Gesamtdelta nicht rein den Spalten zuzuschreiben —
  aber die in SP6 befüllten Spalten `nom_status`/`rank_verbatim` sind nur auf
  20.688 bzw. 2.243 der 169.670 Namen gesetzt und kosten damit höchstens
  einstellige MiB, nicht die in den known-gaps grob geschätzten ~50 MB
  (gemessen statt geschätzt). Das Bundle bleibt kleiner als die 108,9-MB-M5.2-
  Baseline (Distribution-Kürzung überwiegt) und Faktor ~4,7–9,4 über dem
  10–20-MB-Spec-Ziel. reality-check M5.3 um eine datierte Nachmessung ergänzt;
  known-gaps-Eintrag entfernt.

### Changed (CI — Per-PR-Mutationstests laufen deterministisch durch)
- **Mutation-Workflow in zwei Stufen getrennt.** `gremlins` rekompiliert das
  Paket je Mutant; `internal/adapters/telemetry` (OTel-SDK) und
  `internal/adapters/sqlite` (`modernc.org/sqlite`) treiben den
  `go build`-Subprozess je auf ~6+ GB RSS und sprengen den 7-GB-`ubuntu-latest`
  → `exit 143` (OOM) oder Thrashing bis zum 60-min-Timeout, ein
  nicht-deterministischer Ausgang, der PRs blockierte. In PR #35 wurde belegt,
  dass ein Swap-Headroom-Schritt den telemetry-OOM **nicht** verhindert (der
  OTel-Compile-Peak ist aktives Working-Set, das der OOM-Killer vor dem Swap
  trifft) und sqlite auf Swap 15–60 min thrasht. Konsequenz: Der **Per-PR-Job**
  (`mutation`) fährt nur noch die **leichten** Pakete — jedes terminiert
  deterministisch in Minuten, alle blockierend, kein `continue-on-error` mehr.
  Die zwei schweren Pakete laufen in einem separaten Job (`mutation-heavy`)
  **nur** auf wöchentlichem Cron/`workflow_dispatch`, nie auf PRs, sodass ein
  OOM keinen PR mehr flaky macht. Entwickler-Gate für die zwei bleibt lokal
  (`make mutation PKG=…`, läuft dort vollständig durch). Größerer Runner
  (`ubuntu-latest-4-core`) wäre der einfachere Nachfolger, ist aber Org-Sache.

### Added (Schulden-Batch 2 — stiller Verlust: `nom_status`-Drift-Signal + `TaxonRow`-Mapper-Guard)
- **`hostus ingest` meldet die vier `nom_status`-Urteile pro Backbone.** Neue
  Zeile `nom_status: absent=… acceptable=… disqualifying=… unclassified=…`
  plus eine nach Häufigkeit sortierte Stichprobe der unklassifizierten,
  **normalisierten** Statuswerte (kleingeschrieben, Whitespace kollabiert —
  wie `domain.NormalizeNomStatus`, nicht die Rohzellen).
  `domain.ClassifyNomStatus` wird über jede **Nicht-akzeptiert-Zeile**
  (Synonyme inkl. verwaister; accepted rows zählen bewusst nicht) gelegt und in
  `BackboneReport.NomStatus*`/`UnclassifiedNomStatusSample` getallyt. Damit
  wird ein WCVP-Bump, der neue Statuswerte einführt, als **Anstieg von
  `unclassified` sichtbar**, statt still dort zu landen und lautlos
  zurückgehalten zu werden — dieselbe Disziplin wie bei `ranks: other=…` und
  `matched/unmatched/ambiguous`.
- **Guard-Test gegen stillen Spaltenverlust im `TaxonRow`-Mapper.**
  `wcvpRowSource.Taxa()` (die eine Kopie, die Produktionscode aufruft) bildet
  jetzt in einem White-Box-Test jede Quellspalte auf einen eigenen Sentinel
  ab und vergleicht die gesamte `application.TaxonRow`-Struktur — fällt eine
  Mapping-Zeile weg, wird das Feld leer und der Test benennt genau die
  verlorene Spalte. Schließt die Fehlerklasse „geparst, dann von einer
  Mapping-Zeile fallen gelassen" für den einzigen produktiv genutzten Mapper.
- Beide zugehörigen Einträge in `docs/explanation/known-gaps.md` entfernt.

### Fixed (poc-in-verify geschlossen; telemetry-Mutation verbessert)
- **`poc/` wird von `make verify` abgedeckt.** Neues `poc-check`-Target
  (`cd poc && go build ./... && go vet ./...`) — das eigene Messmodul war
  bisher von keinem verify-Schritt erfasst (eigenes Modul, kein go.work), so
  konnte toter/kaputter Harness-Code unbemerkt bleiben. Bewusst OHNE die
  Hexagon-Lint-Regeln (die nur für den Laufzeit-Code gelten).
- **`internal/adapters/telemetry`-Mutation verbessert (nicht geschlossen).**
  Die `make()`-Kapazitätsangabe in `RingLog.Handle` wurde zu
  `make(map[string]string)` — entfernt einen äquivalenten LIVED-Mutanten,
  lokale Efficacy jetzt 100 % (56/0/0). Der CI-OOM auf dem 7-GB-Runner besteht
  aber fort (Neukompilierung des OTel-SDK je Mutant, in PR #33 reproduziert),
  daher bleibt `continue-on-error` — der echte Fix ist ein größerer Runner.
  known-gap entsprechend aktualisiert.

### Added (SP5 — `sec.`-Auflösungsfilter und `sec`-Ausgabe)
- **`entry_backbone`/`entry_sec` auf `POST /v1/match` und `POST /v1/translate`.**
  Im Multi-Backbone-Index (WCVP + CDMs ~119 `sec.`-Räumen) liegt derselbe Name
  mehrfach, sodass gängige Namen mehrdeutig (`unresolvable`) blieben — der
  Grund, warum `target_space` und der `/v1/translate`-`verbatim`-Einstieg kaum
  bedienbar waren. Neu: `application.MatchFilter{Backbone, Sec}` verwirft
  `MatchExact`-Kandidaten nach `Concept.BackboneID`/`SecReference` (im
  Application-Layer, Port unverändert), UND-verknüpft. Ohne Filter byteweise
  die alte Form. Unbekannter `entry_backbone`/`entry_sec` → `400 INVALID_QUERY`,
  benennt den Wert. Bei `/v1/translate` nur auf dem `verbatim`-Pfad (bei
  `concept_id` ignoriert). Ersetzt das nie implementierte `sec_hint`-Feld.
- **`sec` `{id, title}` in `GET /v1/concept/{id}` und `GET /v1/suggest`.** Für
  ein sec-tragendes (CDM-)Concept present, für WCVP weggelassen (SP1-Form
  unverändert) — macht gleichnamige Konzepte unterscheidbar.
- **Gemessen** (`docs/research/sp5-sec-filter.md`, realer WCVP+CDM-Index):
  `entry_sec` löst **99,67 %** der (Name, `sec.`)-Kombis eindeutig auf (nur 167
  von 51.167 bleiben, echte CDM-Dubletten); `entry_backbone=wcvp` macht 12.979
  bisher mehrdeutige Namen eindeutig. Der Filter beseitigt die Mehrdeutigkeit,
  verschiebt sie nicht — die known-gap-Auflage ist erfüllt.
- Behebt beide SP5-known-gaps (`/v1/translate`-`verbatim` praktisch tot; kein
  `sec.`-Feld in suggest/concept); Einträge aus `docs/explanation/known-gaps.md`
  entfernt.

### Removed (redundante Euro+Med-REST-Pipeline stillgelegt)
- **`pipelines/euromed/` (REST-Crawl) entfernt** — `build.sh` und `crawl.py`.
  Der flache `/euromed/taxon`-Endpunkt liefert nur einen autorenbehafteten
  `titleCache` **ohne Rang und ohne Accepted-Verknüpfung** (verifiziert:
  `nameUsage`/`concept`/`conceptId` sind `null`; `docs/research/reality-check.md`
  M6 misst die euromed-CSV mit 0 Zeilen mit Rang / 0 mit `accepted_taxon`).
- **Euro+Med wird stattdessen über die `eurosl`-Pipeline bezogen.**
  `EuroSL.sqlite` ist dasselbe CDM-Datenset — die einzige Datentabelle heißt
  `EuroPlusMed.Plantae`, `AccordingTo` steht auf jeder Zeile auf
  `api.cybertaxonomy.org/euromed` — nur **strukturiert**: bare `TaxonName`,
  `TaxonRank` und **85.396 Accepted-Links** (reality-check M6). Auf jeder
  servierbaren Dimension dominiert EuroSL, also keine *nutzbare* Deckung
  verloren (es sind verschieden datierte Snapshots desselben Datensets, kein
  bewiesenes Superset — die überzähligen REST-Zeilen sind genau die
  rang-/link-losen, unbrauchbaren). `dataset.example.yaml` führt Euro+Med
  jetzt als Namensraum `eurosl` statt als (unbrauchbaren) Backbone;
  `pipelines/README.md` dokumentiert die Stilllegung.

### Fixed (Schema-Drift wird beim Start laut erkannt)
- **`sqlite.Open` scheitert jetzt laut bei Schema-Drift statt still pro
  Request.** Bisher legte `Open` das Schema mit `CREATE TABLE IF NOT EXISTS`
  an — das ergänzt einer alten Datenbank fehlende *Tabellen*, aber nie
  fehlende *Spalten*. Ein vor SP3 gebauter Index ohne
  `trait_value.resolution` lieferte deshalb auf **jedem** Konzept einen
  `500 INTERNAL_ERROR` bei `GET /v1/concept/{id}/traits`, während
  `/health/ready` weiter `200` meldete. Neu: `verifySchemaColumns` prüft beim
  Öffnen jede Tabelle gegen das eingebettete Schema (angewandt auf eine frische
  In-Memory-Referenz, also **kein** zweiter handgepflegter Spaltenkatalog) und
  bricht mit einer Meldung ab, die Tabelle und fehlende Spalte(n) nennt sowie
  auf `hostus ingest` verweist. Der Check ist einseitig (fehlende Spalten
  scheitern, zusätzliche sind erlaubt), sodass ein älteres Binary eine neuere
  DB weiter öffnet. Behebt die dokumentierte SP8-Lücke; der bevorzugte
  „laut scheitern"-Ansatz, nicht eine dritte Ad-hoc-Migration.

### Added (SP9, Task 2 — `target_space` und `aggregate_policy` auf `/v1/match`)
- **`POST /v1/match` nimmt ein optionales `target_space`** (aktuell nur
  `floraveg`) und liefert je Treffer drei zusätzliche Felder. Ohne
  `target_space` ist die Antwort **byteweise** die SP1-Form — durch einen Test
  gepinnt, weil UC3/UC6 denselben Endpunkt nutzen. Ein unbekannter
  `target_space` ist `400 INVALID_QUERY` und nennt den Raum, kein stiller
  No-Op (`application.ErrUnknownTargetSpace`).
- **`aggregate_policy` ist dreiwertig, nicht boolesch:** `known` (der Zielraum
  führt das Aggregat als eigenes Taxon; ESy-Name in `target_space_name`),
  `unresolvable` (die Anfrage IST ein Aggregat, der Zielraum kennt nur
  Kleinarten — „nicht entscheidbar", nicht „nicht erfüllt"; Deckung darf nicht
  verteilt werden, kein Name), und **abwesend** (gar kein Aggregat im Spiel).
  Ein `known` für jede Art hätte das Feld bedeutungslos gemacht.
- **`esy_diagnostic_relevance` ist konspikuierend abwesend:** bei gesetztem
  `target_space` **immer present**, **immer** `not_determinable` — ein
  selbsterklärender String, niemals `null` und nie fehlend, damit ihn kein
  Konsument als falsy-„nicht relevant" liest (genau der False Negative, den
  UC4 verhindern soll). Das ESy-Regelwerk ist nicht ingestiert — als bekannte
  Lücke in `docs/explanation/known-gaps.md` dokumentiert.
- **Wiederverwendung statt zweitem Pfad:** die Policy stammt aus derselben
  Aggregat-Prädikatsfunktion (`domain.IsAggregateName`) und demselben
  SP3-Crosswalk wie der Ingest. Neu: reine `domain.ResolveTargetSpace`,
  `application.MatchInSpace`, die drei Felder in OpenAPI und
  `docs/reference/http-api.md`.

### Added (SP9, Task 3 — e2e, Anleitung, Verdikt)
- **e2e unter dem `integration`-Tag** (`TestIntegration_MatchTargetSpaceFloraVeg`):
  ingestiert WCVP + FloraVeg und löst eine Beispielaufnahme über echtes HTTP
  auf, mit **spezifischen** Namen und Policies für alle drei Zustände (`known`,
  abwesend, `unresolvable`) und dem ESy-Sentinel auf jedem Treffer; plus ein
  400-Test für unbekannten `target_space`.
- **Neue Anleitung `docs/how-to/aggregate-uc4.md`** (deutsch) mit der
  durchgerechneten Aufnahme aus dem Quelldokument und einem expliziten
  **„Was fehlt"**-Abschnitt zur nicht bestimmbaren `esy_diagnostic_relevance`.
- **Verdikt `docs/research/sp9-uc4-verdict.md`: hält mit Auflagen** — mit dem
  gemessenen Befund, dass `known` über einem WCVP-only-Backbone praktisch
  unerreichbar ist (WCVP führt keine Aggregat-Konzepte), sodass
  `aggregate_policy` heute vor allem als `unresolvable`-Signal wertvoll ist.

### Added (SP9, Task 1 — FloraVeg-Namensraum ingestieren)
- **Namensräume als eigene Quellenart.** Ein Namensraum ist eine Checkliste,
  die NAMEN beiträgt und keine Taxonomie — keine Synonymie, keine
  Elternkette, keine externe ID zum Joinen. Er ist damit weder ein Backbone
  (er erzeugt keine `taxon_concept`-Zeilen und darf nicht in
  `backbone_version` landen) noch ein Trait-Vokabular. Neu:
  `internal/adapters/namelist` (Reader für den geteilten
  Namenslisten-CSV-Vertrag aus `pipelines/README.md`), `domain.NameSpaceMeta`
  / `domain.NameSpaceEntry` / `domain.IsAggregateName`, die Tabellen
  `name_space` + `name_space_entry`, `application.IngestNameSpace`, der
  Manifest-Abschnitt `name_spaces:` und die Verdrahtung in `hostus ingest`.
- **`dataset.example.yaml` korrigiert:** `floraveg` stand unter `backbones:`
  mit `path: ./backbones/floraveg` und war damit **nicht ingestierbar** —
  `internal/app.readerFor` liest jeden Backbone-Eintrag durch den
  WCVP-DwC-A-Reader, die Pipeline liefert aber eine einzelne kanonische CSV.
  Der Eintrag steht jetzt unter `name_spaces:`.
- **Der Crosswalk ist die SP3-Maschinerie, kein zweiter Pfad:**
  `IngestNameSpace` löst über dasselbe unveränderte `resolveTraitName`
  (`domain.NameCandidates`-Leiter) auf wie der Trait-Ingest, strikt
  zweiphasig (auflösen ohne offene Transaktion, dann nur schreiben — der
  Adapter läuft mit `SetMaxOpenConns(1)`).
- **`name_space_entry` ist auf `(space, ext_id)` geschlüsselt, nicht auf das
  Konzept.** FloraVeg schreibt *Festuca ovina* unter drei SeqIDs dreifach
  (`Festuca ovina`, `… aggr.`, `… s. l.`), alle drei fallen auf dasselbe
  WCVP-Konzept — auf das Konzept zu schlüsseln würde genau die
  Unterscheidung wegwerfen, die UC4s `aggregate_policy` treffen muss.
- **Verlust sichtbar:** `hostus ingest` gibt pro Namensraum
  matched/unmatched/ambiguous/concepts, eine getrennte Aggregatzeile, die
  Normalisierungsregeln und vier begrenzte Stichproben aus; ein doppelter
  `ext_id` wird gezählt statt still überschrieben.

### Fixed (SP9, Task 1 — Loch im Redistribution-Gate)
- **`hostus bundle` ließ Namensraum-Daten ungeprüft durch.**
  `findRestrictedSources` fragte nur `backbone_version`, `trait_vocabulary`
  und `xref_source` ab — eine neue Quellenart war ihm schlicht unbekannt,
  dieselbe Klasse wie das SP4-Xref-Loch. Neu: vierte Abfrage über
  `name_space_entry → name_space`, plus **konzept-gescopte** Kopie beider
  Tabellen (wie bei `sec_reference`: `name_space_entry.name` ist geernteter
  Inhalt, kein Quellen-Metadatum). FloraVeg (`redistribution: unknown`) wird
  damit standardmäßig verweigert und unter `--force-include-restricted` in
  `bundle_meta.restricted_sources` protokolliert. Gepinnt auf Adapter- UND
  Kompositionswurzel-Ebene (echter `app.Ingest` + echter `app.Bundle`).

### Measured (SP9, Task 1 — FloraVeg-Crosswalk gegen den realen WCVP-Index)
- Von **16.402** FloraVeg-Namen lösen **14.050 (85,7 %)** auf ein
  WCVP-Konzept auf; 357 (2,2 %) bleiben unmatched, **1.995 (12,2 %)** sind
  **ambiguous** — die Mehrdeutigkeit ist die eigentliche Auflage, nicht die
  Nichttreffer, exakt wie bei den SP3-Trait-Vokabularen.
- Von **309** Aggregaten lösen **246 (79,6 %)** auf — jeder davon über die
  *markierte* Regel `aggregate_to_nominate`, weil WCVP null
  aggregatmarkierte Namen führt. (Es sind 309, nicht 308: `Dryopteris
  affinis s. lat.` trägt die Langform.)
- **13.473 WCVP-Konzepte** bekommen ein FloraVeg-Gegenstück — **3,06 %** von
  440.534 Konzepten (3,56 % der 368.928 Arten-Konzepte).
- Vollständige Messung samt Kommandos: `docs/research/floraveg-namespace.md`.

### Added (SP8, Task 3 — e2e-Test und Anleitung zur Testkonsole)
- **e2e unter dem `integration`-Tag** (`TestIntegration_TestConsoleToggle`):
  fährt die echte Komposition zweimal gegen dieselbe ingestierte Datenbank
  vor einem echten TCP-Listener hoch — einmal mit Konsole, einmal ohne.
  Geprüft werden `/` (HTML, `ETag`, alle vier Panels), der CSP-Header
  (`default-src 'self'`, beide Hashes, kein externer Origin), SPA-Deep-Links,
  die Einzel-Assets, die vollständige 404-Oberfläche bei abgeschalteter
  Konsole und **14 API-Sonden byteweise identisch** in beiden
  Schalterstellungen (inkl. 404 unter `/v1` und 405 bei falscher Methode).
- **Neue Anleitung `docs/how-to/test-console.md`** (deutsch): Start, der
  Schalter in allen drei Prioritätsstufen inkl. Deployment-Abschaltung, wozu
  die vier Panels da sind, ausdrücklich **was die Konsole nicht ist** (kein
  Produkt-UI, nicht authentifiziert, nicht für Exposition gehärtet) und ein
  Abschnitt „Was du erwarten solltest", damit ein bekannter Mangel nicht für
  einen kaputten Build gehalten wird.
- **`docs/research/suggest-quality.md`** um die Hand-Beobachtung aus dem
  Konsolentest ergänzt: 0 von 30 präfixbeginnenden Treffern bei `ca`, 17/90
  über drei Präfixe, alle Scores identisch (`-3.736`), getroffen wird das
  Epitheton statt des Namensanfangs — es gibt keine Ordnung zu justieren,
  es muss erst eine entstehen.
- **`docs/explanation/known-gaps.md`** um zwei Einträge ergänzt:
  `/v1/concept/{id}/traits` liefert 500 auf jedem Index ohne
  `trait_value.resolution`, während `/health/ready` 200 meldet; und kein
  Endpunkt listet die `sec.`-Referenzräume, weshalb `target_space` ein
  Freitextfeld bleibt.

### Added (SP8, Task 2 — eingebettete Testkonsole)
- **Die Testkonsole als `embed.FS`-Asset** in
  `internal/adapters/http/assets/`, eingebettet im HTTP-Adapter selbst: die
  Assets sind ein Implementierungsdetail dieses Adapters, kein zweiter
  Adapter, also braucht es dafür auch keine depguard-Ausnahme.
  `.golangci.yml` bleibt unverändert. Ausgeliefert wird **ein** in sich geschlossenes
  HTML-Dokument — CSS und JavaScript werden hineingeschrieben, nicht
  nachgeladen, weil die Konsole sich den globalen 20-rps-Token-Bucket mit
  der API teilt. Kein CDN, keine Web-Schrift, keine externe Referenz;
  Vanilla HTML/CSS/JS ohne Build-Schritt.
- **`Content-Security-Policy: default-src 'self'`** mit
  `script-src`/`style-src` als **SHA-256-Hash** der eingebetteten Blöcke
  statt `'unsafe-inline'`, dazu `base-uri`/`form-action`/`frame-ancestors`/
  `object-src` auf `'none'`. Keine Inline-Handler, kein `eval`.
- **Vier Panels** (deutschsprachig): Suggest mit Präfix- und
  Rangmix-Auswertung je Trefferliste, Konzept (Klassifikation, Xrefs je
  Autorität mit **allen** IDs, Verbreitung, Synonyme, Traits,
  `?relevance=publication`), Match und Translate. Eine leere
  Translate-Antwort wird als **Aussage** („keine Relation erfasst")
  gerendert, nie als Fehler.
- **Pfadregeln:** `/` liefert die Konsole; `/assets/app.js` und
  `/assets/style.css` liefern die Einzel-Assets mit eigenem `Content-Type`
  und `ETag`; ein unbekannter Pfad **außerhalb** `/v1`, `/health`,
  `/metrics`, `/openapi` liefert per GET/HEAD ebenfalls die Konsole
  (SPA-Deep-Link), ein unbekannter Pfad **darunter** behält seine 404.
  Ein 405 bleibt ein 405.
- **Die SPA-Weiche liegt in derselben Middleware-Kette** wie alles andere:
  gorilla/mux umhüllt `NotFoundHandler` nicht selbst, deshalb wird die
  Kette einmal als Slice gebaut und beidseitig angewendet.

### Added (SP8, Task 1 — Schalter für die eingebettete Testkonsole)
- **Neuer Konfigurationsschlüssel `ui.enabled`** (Default **an**) mit
  `UIConfig` in `internal/config`, Umgebungsvariable `HOSTUS_UI_ENABLED`
  und CLI-Flag `serve --ui` / `--ui=false`. Die Prioritätsleiter
  (config.yaml < Umgebung < Flag) ist mit Tests festgenagelt; ein
  Kurz-Alias wurde bewusst **nicht** eingeführt.
- **Router-Verhalten:** ist die Konsole an, hängt unter `/` ein
  Platzhalter-Handler (die echten Assets folgen in SP8 Task 2) innerhalb
  der bestehenden Middleware-Kette; ist sie aus, wird **nichts**
  registriert — `/` und jeder Asset-Pfad sind 404. Ein Test vergleicht die
  komplette API-Oberfläche (`/v1/*`, `/health/*`, `/openapi`) mit und ohne
  Konsole byteweise.
- **Dokumentation:** `docs/reference/configuration.md`,
  `config.yaml.example` und `example.env` um den Schlüssel ergänzt.

### Added (SP7, Task 1 — Suggest-Latenz und -Zusammensetzung gemessen)
- **Neue Messung
  [`docs/research/suggest-quality.md`](docs/research/suggest-quality.md)**
  zu `GET /v1/suggest`, gegen den vollen echten Index (440.534 Konzepte),
  alle Latenzen als Band über 5 Läufe. Vier Szenarien, p95-Mediane:
  Baseline **237,1 ms**, Mitteleuropa-Gebiet **408,6 ms (+72 %)**,
  Cap 2000 **64,6 ms (−73 %)**, Gebiet + Cap **72,0 ms (−70 %)**.
  - **Die Gebietseinschränkung allein macht die Abfrage langsamer**, weil
    sie heute ein korreliertes `EXISTS` je Kandidatenkonzept ist und nach
    der Kandidatenbildung läuft (`ca` überschreitet damit die Sekunde).
    Zusammen mit dem Cap ist sie bezahlbar.
  - **Das Flooding ist belegt:** die Top 10 für `ac`, `ca` und `al`
    enthalten **je 10 Arten und null Gattungen**, und nur **9 von 30**
    Treffern beginnen überhaupt mit dem Präfix.
  - **Die Ursache ist `bm25` auf Präfixanfragen:** 32.936–100.029
    FTS-Zeilen verteilen sich auf **11–12 verschiedene Score-Werte**;
    *Acalypha*, *Acer* und *Achillea* haben denselben Score und liegen
    nur durch Tie-Break-Rauschen auf Position 137/272/410.
  - **Gegen die Planannahme:** ein Cap vor der Rangdiversität kostet bei
    **37 von 38 Präfixen null Gattungen** (Ausnahme `ca`: 239 von 1.032).
    Das Ordnungsargument „Diversität vor Cap" trägt mit dieser Zahl nicht.
  - **Kontrollen zur Query-Form** (Review): auch die schlankeren Varianten
    kosten **+64 % bis +79 %** p95 — die Produktionsform (nur `in_area`,
    keine Restriktion) liegt bei **383,5 ms (+64 %)**. Schon das bloße
    Berechnen von `in_area` für die Sortierung ist der teure Teil.
  - **Das Abnahmekriterium „*Acer* in den Top 10 für `ac`" ist ohne
    Gebietsscoping nicht erreichbar:** 142 `Ac*`-Gattungen global gegen
    **18** in Mitteleuropa. Es steht darum nach Scope getrennt im Dokument.
- **`poc/measure/suggestquality`** (neuer Harness, misst die Szenarien
  direkt gegen die Produktions-SQL, Index strikt lesend geöffnet) und
  **`--runs` für `poc/measure/latency`**, das die p50/p95 jetzt als Band
  über mehrere vollständige Läufe ausgibt statt als Einzellauf.
- **Neue bekannte Lücke
  [„`poc/` wird von `make verify` weder kompiliert noch gelintet"](docs/explanation/known-gaps.md)**
  — `poc/` ist ein eigenes Modul ohne `go.work` und im Debt-Guard
  ausgenommen, weshalb die Messharnesse ungeprüft bleiben. SP7/Task 1 hat
  den Preis gezeigt: ein toter Statusvergleich im Harness fiel erst im
  Review auf (behoben, ohne Auswirkung auf die Zahlen).
### Added (SP5, Task 5 — Volllauf, End-to-End-Beweis und UC6-Verdikt)
- **Neuer `integration`-Test
  `TestIntegration_TranslateBetweenSecSpaces`** (`internal/app/integration_test.go`):
  ingestiert WCVP- und CDM-Fixture über `app.Ingest`, serviert sie über
  `app.New`/`app.Router` hinter einem echten Listener und prüft
  `POST /v1/translate` über echtes HTTP. Er pinnt **konkrete Konzept-IDs
  und den Relationstyp**, nicht nur den Status 200: die `sec.`-Trennung
  (ein Name → zwei Konzepte), `congruent` mit `is_equality` und
  Richtungsangabe `target_to_source`, `includes` als ausdrücklich **keine**
  Gleichsetzung, die Abwesenheit der verworfenen
  `is misapplied name for`-Zeile, die leere `no_relation_recorded`-Antwort
  für ein WCVP-Konzept und 404 bei unbekanntem Zielraum.
- **SP5-Verdikt in
  [`docs/research/reality-check.md`](docs/research/reality-check.md)**, mit
  dem Befehl hinter jeder Zahl. Voller Ingest (WCVP-Volldump + CDM-Ernte in
  eine frische Datenbank): **283,92 s**, 440.534 WCVP- und 51.466
  CDM-Konzepte, 26.002 von 26.346 Relationen geschrieben (die 344
  Differenz sind genau die verworfenen misapplied-Zeilen), **0
  Reader-Fehler, 0 unaufgelöste Relationsenden, 0 unaufgelöste Eltern**
  trotz 9.897 Vorwärtsverweisen auf Elternzeilen — beide Review-Befunde
  (Elternreihenfolge, Fehlergrenze des Readers) sind am Volldatensatz
  belegt behoben.
  - **Die UC6-Zahl: 4.461 von 440.534 WCVP-Konzepten (1,01 %)** haben
    überhaupt eine CDM-Gegenseite mit Relation; über den Endpunkt sind es
    in einer 300er-Stichprobe **0** (265× `UNRESOLVABLE` wegen echter
    Namensmehrdeutigkeit über `sec.`-Räume hinweg, 35× Auflösung auf das
    relationslose WCVP-Konzept). Gegenprobe mit 200 CDM-Konzepten:
    **200/200 übersetzt**. Der Endpunkt ist in Ordnung, die Brücke
    zwischen den Namensräumen fehlt.
  - Relationsgraph: **60,90 %** der CDM-Konzepte tragen mindestens eine
    Relation, 39,10 % sind isoliert. 119 verschiedene `sec.`-Referenzen,
    11 davon decken 96,77 % der Konzepte.
  - **Lizenz unverändert bindend**: keine Lizenzangabe auffindbar, aus
    urheberrechtlich geschützter Florenliteratur abgeleitet,
    `redistribution: unknown` — nur lokale Auswertung, kein öffentlicher
    Betrieb von `/v1/translate` auf diesen Daten ohne schriftliche
    Freigabe von BGBM/EDIT.
  - **Ursache der 265 gemessen statt erschlossen**: `POST /v1/match` auf
    denselben 300 Namen liefert **265× „Mehrdeutiger Treffer"** und **0×**
    „kein eindeutiger Treffer" — es ist ausnahmslos Mehrdeutigkeit.
    `Abies alba Mill.` ist **acht** CDM-Konzepte plus das WCVP-Konzept,
    also neun gleich starke Kandidaten; über alle Namensformen sind es bis
    zu **zehn** CDM-Konzepte (über beide Backbones 16).
  - **Nebenbefund für das nächste Milestone**: der CDM-Ingest bringt die
    einzigen **629 FAMILY-Konzepte** des Systems (WCVP hat 0). Sie sind
    über `/v1/suggest` und `/v1/concept` unverändert erreichbar, aber
    keines trägt eine Relation, und beide Endpunkte geben kein `sec.`-Feld
    aus — namensgleiche Familien aus verschiedenen Referenzräumen sind in
    der Antwort nicht unterscheidbar.

### Changed (SP5, Task 5 — Review)
- **Zwei Produktionskommentare korrigiert**, die eine Zahl nannten, die
  dieser PR um rund das 30-Fache widerlegt:
  `internal/application/cdm_ingest.go` (`writeCDM`) und
  `internal/adapters/sqlite/cdm_test.go` begründeten den zweiten
  `parent_id`-Unterlauf mit „312 von 697 Zeilen". Gemessen am vollen
  Artefakt sind es **9.897 von 33.731 (29,34 %)** — knapp ein Drittel, nicht
  eine Handvoll. Die alte Zahl hätte den Zweiphasen-Schreibpfad wie
  Überkonstruktion aussehen lassen.
- **[`docs/how-to/sec-translate-uc6.md`](docs/how-to/sec-translate-uc6.md)
  warnt jetzt vor dem `verbatim`-Einstieg**, statt ihn kommentarlos
  anzubieten: 265 von 300 `UNRESOLVABLE`, mit der Begründung, dass ein
  geteilter Name über `sec.`-Räume hinweg **bauartbedingt** mehrdeutig ist
  und die Verweigerung deshalb richtiges Verhalten ist, kein Fehler. Plus
  der Handlungsanweisung, den Namen vorab selbst aufzulösen und
  `concept_id` zu schicken.
- **Zwei Einträge in
  [`docs/explanation/known-gaps.md`](docs/explanation/known-gaps.md)** — die
  SP5-Auflagen standen bislang nur im Reality-Check: der tote
  `verbatim`-Pfad und **`sec` fehlt in den Antworten von `/v1/suggest` und
  `/v1/concept`** (zwei `Asteraceae`-Treffer mit identischer Anzeige,
  identischem Kanonischen und identischem Score `-17.556039197970815`,
  unterscheidbar nur an der UUID).
- **Reality-Check nachgeschärft**: die Ursache der 265 ist jetzt über
  `/v1/match` belegt statt behauptet; die Stichprobenbezugsgröße ist
  korrigiert (nur **196 der 300** haben eine CDM-Seite, die *Flora
  Europaea* überhaupt erreicht — die 0 bleibt, die Vergleichsgröße ändert
  sich); die Mehrdeutigkeits-Obergrenze ist richtiggestellt (acht statt
  neun CDM-Konzepte für `Abies alba`, global bis zu zehn); und die
  200/200-Gegenprobe sagt jetzt ausdrücklich, was sie **nicht** zeigt (sie
  steigt über `concept_id` ein und umgeht damit genau den Pfad, an dem die
  300 scheitern).

### Added (SP6, Task 4 — Offenlegung, End-to-End-Beweis und Verdikt)
- **Neue Anleitung
  [„Publikationsfähige Synonyme filtern (UC5)"](docs/how-to/synonyms-uc5.md)**
  mit dem durchgerechneten *Corynephorus-canescens*-Beispiel und einem
  ausdrücklichen Abschnitt **„Was dieser Filter nicht kann"**. UC5 nennt
  fünf Relevanzkriterien; SP6 liefert zwei vollständig, eines teilweise
  und **zwei überhaupt nicht**. Das steht jetzt in der Anleitung des
  Endpunkts selbst und nicht in einer Forschungsdatei: Wer
  `relevance=publication` liest und regionale Filterung annimmt,
  veröffentlicht die falsche Synonymliste.
  - **Keine regionale Filterung** („im Bezugsraum verwendet"):
    `distribution` hängt am *Konzept*, ein Synonym ist ein *Name* — mit
    dem aktuellen Schema nicht ausdrückbar. Die Anleitung nennt, was es
    bräuchte (eine Name-×-Gebiet-Relation mit Quelle je Zeile).
  - **Keine Filterung nach Standardwerken**: ROTHMALER, OBERDORFER, HEGI
    und SCHMEIL-FITSCHEN sind vier der 18 CDM-Klassifikationen aus SP5,
    aber `redistribution: unknown` und die Ernte ist unfertig.
  - **Die Typisierung ist ein Zwei-Wege-Split**: `homotypic = 1` auf
    271.821 Synonymzeilen, NULL (= *unbekannt*) auf 692.941,
    `heterotypic` auf **0**.
  - **Ein fehlender `nom_status` ist kein Unbedenklichkeitsnachweis** —
    die Spalte ist auf 99.252 von 1.448.984 Namen (6,85 %) belegt.
  - Zwei benannte Ranglücken: **SUBSPECIES wird spec-treu nicht
    ausgeschlossen** (45.526 Synonymnamen) und 190 Nothotaxon-Zeilen
    passieren `rank=species`.
  - Offener **fachlicher** Punkt, gemessen: fünf Werte halten **1.697
    Namen** zurück, davon 1.117 mit `, sensu auct.`. Sollen
    Fehlanwendungen als *auct. non* geführt werden, ist das eine Zeile in
    `nomStatusGuards`, keine Codeänderung.
- **End-to-End-Beweis über echtes HTTP** in
  `internal/app/integration_test.go`: `ingest -> serve -> GET
  /v1/concept/{id}/synonyms`, gefiltert gegen ungefiltert, mit
  Namensprüfungen statt bloßer Zählungen. *Festuca ovina* verliert genau
  *Avena dura* (`", nom. illeg. superfl."`) und *Festuca ovina* var.
  *vulgaris* (`", not validly publ."`) und behält *Avena ovina*, *Bromus
  ovinus* und *Festuca duriuscula*; bei *Corynephorus canescens* fallen
  drei Rangausschlüsse weg, während subsp. *maritimus* bleibt — die
  SUBSPECIES-Lücke ist damit als Test festgeschrieben, nicht nur
  dokumentiert. Das Ausschluss-Summary muss die Differenz erklären.
- **SP6-Verdikt in `docs/research/reality-check.md`**, gemessen mit dem
  echten Domänencode über alle 964.762 Synonymzeilen: Der Filter ändert
  bei **103.674 von 236.030** Konzepten die Antwort und **24.918 Konzepte
  (10,6 %)** bleiben ohne ein einziges publikationsfähiges Synonym.
  Verdikt: **hält mit Auflagen**.
- **Der Zielkorridor ist mit Kontrollgruppe gemessen — und taugt nicht als
  Beleg für den Filter.** Ungefiltert liegen bereits **70,92 %** der
  Konzepte bei ein bis drei Synonymen, gefiltert **69,20 %**: Auf UC5s
  eigener Zielgröße ist der Filter **netto negativ**. Er zieht 20.854
  Konzepte aus `> 3` heraus (68.642 → 47.788) und erzeugt dabei 24.918
  Nullen; der Modalwert bleibt **1** (40,40 % → 41,38 %). Sein Nutzen ist
  die Entfernung nomenklatorisch untauglicher Namen (89.836 Zeilen mit
  belegtem Defekt), nicht das Treffen des Korridors. Die frühere Fassung
  führte die 69,2 % ohne Vergleichsgröße als Beleg *für* den Filter — eine
  Zahl ohne Kontrollgruppe ist kein Argument.
- **Die Urteilsverteilung über alle 964.762 Synonymzeilen** steht jetzt in
  Anleitung und Reality-Check: `absent` 872.270 / `disqualifying` 89.836 /
  `unclassified` 2.309 / **`acceptable` 347**. Für ganze **347 Zeilen** im
  Korpus behauptet die Quelle, der Name sei nomenklatorisch in Ordnung —
  jede publikationsfähige Liste besteht zu über 99,9 % aus Namen, über die
  niemand etwas gesagt hat.
- **Die vollständige 36-Zeilen-`nom_status`-Regeltabelle** steht jetzt in
  `docs/reference/http-api.md` — bisher versprach die Anleitung sie dort
  zweimal, ohne dass es sie gab. Sie ist gegen `domain.NomStatusRules()`
  festgenagelt (`internal/domain/nomstatus_doc_test.go`, Token/Urteil/
  Trefferzahl zeilenweise), womit auch die bis dahin unbelegte Begründung
  des exportierten Accessors („für den Doku-Generator", den es nicht gibt)
  eingelöst ist.
- **Doppelt ausgeschlossene Synonyme** sind jetzt im clientseitigen Vertrag
  erklärt, nicht nur im Reality-Check: Präzedenz bucht sie unter
  `nom_status`, weshalb `excluded.rank` die Zahl der *nur* rangbedingt
  zurückgehaltenen Synonyme ist — im Beispiel `rank: 16` bei 17
  infraspezifischen Synonymen, korpusweit 14.202 Zeilen.
- **`docs/explanation/known-gaps.md`** (neu, in der Navigation): fünf
  bewusst nicht behobene Befunde mit Auswirkung und nächstem Schritt —
  kein Drift-Signal für das `nom_status`-Vokabular, fehlender
  Mutanten-Mindestboden im Gate, die überholte 108,9-MB-Bundle-Angabe, der
  vierfach kopierte `TaxonRow`-Mapper und 316 Zeilen handgepflegtes
  OpenAPI ohne Drift-Prüfung. Sie standen bisher nur im SDD-Ledger, das
  das nächste Teilprojekt nicht liest.

### Fixed (SP6, Task 4 — Review-Nachlauf)
- **Das neue `Not covered`-Gate lief in der CI auf keinem einzigen
  SP6-Paket.** `.github/workflows/mutation.yml` fuhr `config`, `httperr`,
  `adapters/telemetry`, `adapters/http`, `app`, `adapters/mcp` und
  `cmd/hostus` — die gesamte SP6-Logik liegt aber in `internal/domain`,
  `internal/application` und `internal/adapters/sqlite`, und `make verify`
  ruft `mutation` gar nicht auf. Das Gate war also manuell nachgewiesen und
  nirgends verdrahtet: Ein Rückfall bei den hochgezogenen Bedingungen in
  `internal/domain/synonym.go` wäre unbemerkt geblieben. Die drei Pakete
  laufen jetzt mit (gemessene Laufzeit lokal: 30 s / 62 s / 154 s, rund
  vier Minuten zusätzlich bei 60 Minuten Job-Timeout).
- **`internal/domain/synonym.go`, Regel `type`:** Die Notiz behauptete,
  alle 1.099 Zellen mit `type` seien Mangelaussagen. `", type variety."`
  (1 Name, *Helichrysum bracteatum* var. *chrysanthum*) ist eine
  taxonomische Anmerkung. Notiz auf 1.098 von 1.099 korrigiert, der
  Einzelfall in Code und Referenz benannt; der Name wird weiterhin
  zurückgehalten (die sichere Richtung), die saubere Auflösung wäre ein
  Guard.

### Changed (SP6, Task 4 — Mutations-Gate)
- **`make mutation` erzwingt jetzt `Not covered: 0`.** Ein überlebender
  Mutant heißt „ein Test läuft durch die Zeile, prüft aber zu lasch" und
  ist begründbar; ein NOT-COVERED-Mutant heißt „kein Test führt diesen
  Code aus" und ist es nie. Der harte Fehlschlag hängt deshalb genau an
  `Not covered` und nicht an einer Efficacy-Schwelle, die gegen die
  bestehenden dokumentiert-gerechtfertigten Überlebenden spröde wäre.
  Fehlt die Zeile `Not covered: N` im Report, bricht das Target ebenfalls
  ab — ein Gate ohne Signal darf nicht grün melden. Genau eine Ausnahme:
  `No results to report.` bei gesetztem `PKG` (ein Paket ohne mutierbare
  Stelle, z. B. `./internal/httperr`) ist eine gültige Antwort; ohne `PKG`
  ist es die bekannte gremlins-Grenze bei `./...` und damit ein Fehlschlag.
- **Zwei Altlasten, die das neue Gate aufgedeckt hat, sind behoben** —
  beide waren „kein Test führt diesen Code aus", keiner davon neu:
  - `internal/app/bundle.go`: `app.Bundle` war nur über `cmd/hostus`
    getestet, also aus Sicht von `make mutation PKG=./internal/app` gar
    nicht. Neues `internal/app/bundle_test.go` deckt Erfolgs- und
    Fehlerpfad im eigenen Paket ab (`Not covered: 1 -> 0`).
  - `internal/adapters/telemetry/telemetry.go`: der `otlpEnabled`-Zweig von
    `Setup` wurde von keinem Test betreten. Neuer Test mit aktiviertem OTLP
    gegen einen toten Endpunkt (hermetisch — die Exporter bauen ihren
    Client lazy und wählen nichts) (`Not covered: 2 -> 0`).
- **`internal/domain/synonym.go`: zwei Bedingungspaare aus den `case`-Armen
  hochgezogen.** Gos Coverage-Modell beendet den gezählten Block am `{`
  eines tag-losen `switch`; eine Bedingung *im* `case`-Arm liegt damit in
  keinem gezählten Block, und `make mutation` meldete sechs Mutanten in
  `ClassifyNomStatus`/`judgeSynonym` dauerhaft als NOT COVERED — kein Test
  hätte das beheben können. Als eigene Zuweisungen (`anyDisqualifying`,
  `anyAcceptable`, `disqualifying`, `unclassified`) werden sie mutiert und
  von der bestehenden Testsuite **gekillt**: `./internal/domain` steht
  jetzt bei `Killed: 123, Lived: 8, Not covered: 0`.

### Added (SP6, Task 3 — `GET /v1/concept/{id}/synonyms`)
- **Neuer Endpunkt `GET /v1/concept/{id}/synonyms?relevance=&rank=&max=`**
  (UC5): „Das Problem ist Filterung, nicht Beschaffung." Er wendet das
  Relevanzmodell aus Task 2 auf die Synonyme eines Concepts an. Am realen
  Index liefert *Corynephorus canescens* (`wcvp:concept:405825`) mit
  `relevance=publication&rank=species&max=3` genau die drei erwarteten
  Namen — *Aira canescens* L. führt als Basionym, *Corynephorus
  incanescens* Bubani (`", nom. illeg. superfl."`) ist ausgeschlossen.
- **Standard ist die UNGEFILTERTE Liste** (`relevance=all`). Der Filter
  hält bei *Corynephorus canescens* 20 von 26 Synonymen zurück und muss
  deshalb ausdrücklich angefordert werden; `/v1/concept/{id}` liefert
  dieselben Synonyme ungefiltert, und zwei Endpunkte mit
  unterschiedlicher Zeilenzahl auf dieselbe Frage lesen sich als Fehler.
  Beide Modi tragen dieselbe Begründung pro Synonym und dieselbe Bilanz.
- **Jeder Ausschluss ist sichtbar.** `summary` beschreibt immer das
  Concept, nie die ausgelieferte Seite: `total`, `publishable`, `absent`,
  `excluded` (Anzahl je Regel) und die unklassifizierten Rohwerte. Ein
  Filter, der 20 von 26 Synonymen entfernt, ohne das zu sagen, ist von
  einer kaputten Abfrage nicht zu unterscheiden.
- **`max` kappt IMMER nach dem Ranking**, nie davor — `max=3` liefert die
  drei besten Synonyme, nie drei beliebige. Bereich `[0, 2000]`; `0` und
  ein fehlender Parameter bedeuten „keine Kappung". Die Obergrenze liegt
  über dem gemessenen Maximum von 1.127 Synonymen pro Concept.
- **Ein gültiger, aber nicht unterstützter `rank` wird abgelehnt**
  (`400`, unter Nennung des Werts), nicht still ignoriert: UC5 definiert
  nur für `species` eine Ausschlussmenge, und eine unbeabsichtigt
  ungefilterte Antwort wäre die gefährlichere Variante.
- **`output.Repository.SynonymCandidates`** (neu) trägt `nom_status`, das
  dreiwertige `homotypic` und `is_basionym` pro Synonym. `is_basionym`
  wird im SQLite-Adapter aufgelöst (`accepted_name.basionym_id = name.id`,
  113.642 Synonymzeilen im gemessenen Index) und durch einen
  Adapter-Test festgenagelt — bliebe das Flag überall `false`, würde
  UC5-Regel 4 still zum No-op, und nichts in `internal/domain` könnte das
  bemerken. `Concept()` und damit `/v1/concept/{id}` bleiben unverändert.
- **`rank_verbatim` wird durchgereicht.** 6.409 Synonymzeilen ranken als
  `OTHER`, 3.731 davon mit einer erfassten Schreibweise (`proles` 2.338,
  `lusus` 658, `microgène` 336, `Convariety` 184, `grex` 41). Keine davon
  wird von `rank=species` ausgeschlossen — sie landen also in
  Publikationslisten, wo ein blankes `OTHER` nichts aussagt. Gleiche
  Begründung wie bei `conceptDTO.rank_verbatim`.
- Dokumentation: `docs/reference/http-api.md` und
  `api/openapi/openapi.yaml` (handgepflegt, dokumentierte Abweichung seit
  S14). Beide benennen ausdrücklich, dass `typification: heterotypic` auf
  dem aktuellen Index nicht auftreten kann (`concept_name.homotypic` ist
  1 oder NULL, nie 0).

### Fixed (SP6, Task 3)
- **Zwei von drei Test-Row-Sources ließen `nom_status`/`published_in`
  fallen.** `internal/adapters/http`, `internal/adapters/sqlite` und
  `internal/application` besitzen je einen eigenen Mapper von
  `wcvp.TaxonRow` auf `application.TaxonRow`; zwei davon kannten die in
  Task 1 ergänzten Felder nicht, so dass jedes Fixture-Synonym
  nomenklatorisch sauber aussah. Alle drei spiegeln jetzt wieder das echte
  Mapping aus `internal/app/ingest.go` (alle zwölf Felder).

### Added (SP6, Task 2 — Publikations-Relevanzmodell für Synonyme)
- **`internal/domain/synonym.go`**: das reine Entscheidungsmodell für
  `GET /v1/concept/{id}/synonyms` (UC5) — ohne I/O, damit die Regeln
  isoliert prüfbar bleiben. `domain.RankSynonyms` liefert **jeden**
  Kandidaten zurück (auch die ausgeschlossenen), jeweils mit dem Grund.
- **`domain.ClassifyNomStatus` bewertet per Token-Containment, nicht per
  Gleichheit.** Das in Task 1 gemessene Vokabular (1.304 distinkte Werte,
  1.225 davon mit < 10 Treffern) lässt weder ein geschlossenes Enum noch
  fail-loud-Parsing zu. Das Lehrbeispiel aus UC5, *Corynephorus
  incanescens* Bubani (`wcvp:name:405842`), trägt `", nom. illeg.
  superfl."` und **nicht** `", nom. superfl."` — ein Gleichheitsvergleich
  würde genau den Fall verfehlen, mit dem UC5 erklärt wird.
- **`unclassified` ist ein eigenes Urteil und wird zurückgehalten**, nicht
  stillschweigend publiziert: ein erfasster, aber nicht klassifizierbarer
  Status wird aus der Publikationsliste genommen, behält seinen Rohwert und
  wird in `domain.SummarizeSynonyms` gezählt. Ein *fehlender* Status
  (`absent`, 1.349.732 Namen) ist davon strikt getrennt und bleibt
  publizierbar.
- **`domain.Typification` ist dreiwertig.** `concept_name.homotypic` ist
  auf 692.941 Zeilen `NULL` — das heißt *unbekannt*, nicht „heterotypisch“.
  Sortierung: bekannt homotypisch → unbekannt → bekannt heterotypisch, und
  welcher der drei Fälle vorlag, steht im Ergebnis. **Hinweis:** im
  aktuellen Index gibt es keine einzige `homotypic = 0`-Zeile, `heterotypic`
  ist heute also unerreichbar (siehe Task-2-Report §4).
- **Ein literales Fragezeichen schlägt jede andere Regel.** Ist die Quelle
  selbst unsicher, löst hostus die Unsicherheit nicht auf: alle 13 Namen mit
  `?` im `nom_status` (`", not validly publ.?"` 8, `", an nom. valid.?"` 4,
  `", nom. superfl. ?"` 1) werden `unclassified`. Die Regel ist bewusst
  generisch — eine wertspezifische Variante stufte `", nom. superfl. ?"`
  über das blanke `superfl` als `disqualifying` ein und bewertete damit
  dieselbe erkenntnistheoretische Lage gegenteilig.
- **`domain.BotanicalOpenItems`** benennt die fünf Werte, die eine
  botanische statt einer technischen Entscheidung brauchen (Fragezeichen 13,
  `sensu auct.` 1.117, `tentatively listed as a synonym` 290, `fossil name`
  274, `isonym` 13) — sie werden ausgewiesen, nicht geraten.
- `SummarizeSynonyms` zählt zusätzlich `Absent` (wie viele publizierbare
  Synonyme allein auf einem *fehlenden* Status beruhen); `Excluded` ist
  immer allokiert.
- Rang-Ausschluss (`VARIETY`, `SUBVARIETY`, `FORM`, `SUBFORM`) ist
  **Aufrufer-gesteuert** (`domain.RanksBelowSpecies()`), nicht fest
  verdrahtet.

### Fixed (SP6, Task 1 — nomenklatorischer Status und Publikation)
- **`nom_status` und `published_in` gingen beim Ingest verloren.** Der
  WCVP-Reader las `nomenclaturalstatus`/`namepublishedin`, `domain.Name`
  hatte beide Felder, der SQLite-Adapter schrieb beide Spalten — aber die
  DTO `application.TaxonRow` und der Mapper in `pass1AcceptedAndNames`
  kannten sie nicht. Im echten Index waren beide Spalten deshalb bei **0
  von 1.448.984** Namen belegt. Jetzt: 99.252 Namen (6,85 %) mit
  `nom_status`, 1.448.934 (99,997 %) mit `published_in`. Das ist die
  Voraussetzung für den Publikations-Relevanzfilter von
  `GET /v1/concept/{id}/synonyms` (UC5).
- Beide Spalten werden als SQL `NULL` gespeichert, wenn die Quelle nichts
  liefert — ein leerer Quellwert wird nicht zu einem Platzhalter.
- Das tatsächlich gemessene `nom_status`-Vokabular (1.304 distinkte Werte,
  Verteilung, Präfix-Artefakt `", "`, Mehrfachstatus je Zelle) ist in
  `docs/research/reality-check.md`, Abschnitt „SP6 Task 1", dokumentiert.

### Added (SP5, Task 4 — `POST /v1/translate`)
- **`POST /v1/translate`**: Übersetzung eines Konzepts zwischen
  `sec.`-Referenzräumen (UC6). Einstieg über `concept_id` **oder**
  `verbatim` (dieselbe Auflösung wie `POST /v1/match`, inklusive der Regel,
  dass ein Fuzzy-Treffer **immer** `requires_review: true` setzt) plus
  `target_space`. Die Antwort ist die **abgeleitete** `sec_inference`-
  Struktur (Architektur-Spec §4.3) — keine persistierte Tabelle.
- **Nur `congruent` ist eine Gleichsetzung.** `domain.Relation.IsEquality`
  hält diese Regel an genau einer Stelle; jeder Kandidat trägt
  `is_equality` explizit (auch `false` — ein fehlendes Feld läse sich wie
  „unbekannt") plus eine deutschsprachige Begründung. `overlaps` und das
  bewusst unbestimmte ⊂⊃⊕ (`includes_or_included_in_or_overlaps`) bleiben
  als das sichtbar, was sie sind, und werden nicht eingeebnet.
- **Ehrliche Richtung**: hostus speichert Relationen nur in der
  Quellrichtung, deshalb liefert jeder Kandidat die gespeicherte Aussage
  (`statement`), deren Relation (`stored_relation`), die Richtung
  (`direction`) und die richtungssichere quellenseitige Lesart
  (`relation_from_source` + `has_inverse`). Ein Feld namens `relation` gibt
  es **bewusst nicht**: CDM emittiert ausschließlich die
  `Includes`-Richtung, eingehende Kanten sind also häufig, und ein Client
  mit `if c.relation == "includes"` läse eine eingehende Kante genau
  verkehrt herum. `relation_from_source` ist immer vorhanden und
  ausdrücklich `null` (nicht weggelassen), wenn keine sinnvolle Umkehrung
  existiert (eingehende `pro_parte`-Kante) — hostus erfindet keine.
- **Genau ein Hop**, ohne Konfigurationsmöglichkeit: eine transitive Kette
  ist über dieses Vokabular nicht allgemein gültig (`overlaps ∘ overlaps`
  sagt nichts, `⊂⊃⊕ ∘ irgendwas` ist undefiniert). `max_hops` steht auf
  jeder Antwort; `max_hops != 1` wird mit `400 INVALID_QUERY` **benannt**
  abgelehnt statt still zu einer Ein-Hop-Antwort zu degradieren.
- **Keine Relation ist eine Antwort, kein Fehler**: `200` mit
  `result: "no_relation_recorded"` und leerem (nie weggelassenem)
  `candidates`. Ein Namenstreffer wird **nie** ersatzweise als Übersetzung
  ausgeliefert. Optional (`include_name_candidates: true`) erscheinen
  namensgleiche Konzepte unter dem eigenen Schlüssel
  `unrelated_name_candidates`, ohne Relationsfeld und mit
  `requires_review: true`.
- Fehlerabbildung: unbekannte `concept_id` **oder** unbekannter
  `target_space` → `404 NOT_FOUND` (ein Tippfehler im Zielraum darf nicht
  wie „keine Relation erfasst" aussehen); nicht auflösbares `verbatim` →
  `422 UNRESOLVABLE`.
- Neue Repository-Methoden `SecReferenceByID` und `ConceptRelationsInSec`
  (Ein-Hop-Kanten in **beiden** gespeicherten Richtungen, Selbstkanten
  ausgeschlossen, deterministisch sortiert; liefert das Quellkonzept mit,
  damit „Id unbekannt" und „keine Relationen" von einer Abfrage entschieden
  werden).
- Doku: `docs/how-to/sec-translate-uc6.md` (deutsch — Lizenzlage der
  CDM-Daten, Ein-Hop-Grenze, „keine Antwort heißt keine erfasste Relation")
  und `docs/reference/http-api.md`; OpenAPI-Baseline um `/v1/translate`
  erweitert.

### Added (SP5, Task 3 — `sec.` als erstklassige Konzeptdimension)
- `domain.SecReference` und `domain.Relation` mit strikter `ParseRelation`:
  Das Relationsvokabular ist jetzt **gemessen statt angenommen**. Die fünf
  Werte aus SP1 (`congruent|includes|included_in|overlaps|disjoint`) waren
  eine Annahme; die Vollernte über 26.346 Relationen korrigierte sie in
  beide Richtungen — `disjoint` kommt **nie** vor und ist entfallen, dafür
  kamen `pro_parte`, `misapplied`, das genuin **unsichere** ⊂⊃⊕
  (`includes_or_included_in_or_overlaps`, wird **nicht** auf `overlaps`
  eingeebnet) und `not_congruent` (genau 1 Zeile von 26.346) hinzu. Ein
  unbekannter Wert bricht laut ab und nennt den Wert — die
  `ParseRank`-Lektion, diesmal ohne lenienten Zwilling, weil eine Relation
  eine wissenschaftliche Aussage ist und kein beschreibendes Metadatum.
- `internal/adapters/cdm`: `ReadConcepts`/`ReadRelations` für die beiden
  kanonischen CDM-CSVs (Pipe-CSV mit RFC-4180-Quoting, gesammelte Fehler mit
  Zeilennummern, keine Panics). `is_concept_relation` ist ein **Tri-State**
  (`true`/`false`/leer = unbekannt), leerer `status` bleibt leer.
- `application.IngestCDM`: strikt zweiphasiger Ingest der CDM-Konzepte als
  **zweiter Backbone** (`cdm:concept:<uuid>`, `sec_reference` gefüllt) —
  bewusst eigene Zeilen neben den WCVP-Konzepten desselben Namens.
  Misapplied-Name-Zeilen (`conceptRelationship: false`) sind **keine**
  Konzeptrelationen und werden verworfen, gezählt und bemustert;
  Relationen werden **nur in der Quellrichtung** gespeichert (Inversion ist
  über `domain.Relation.Inverse` eine Abfragezeit-Frage), nicht auflösbare
  Enden werden gezählt und bemustert, brechen den Ingest aber nie ab.
- Schema: neue Tabelle `sec_reference`, Index auf
  `taxon_concept(sec_reference)`, erweitertes `relation`-Vokabular und ein
  auf `(from_concept, to_concept, relation, source)` verbreiterter
  Primärschlüssel von `concept_relation` — vorher überschrieben sich zwei
  verschiedene Relationstypen desselben Paares still. Altbestände werden
  beim `Open` per `PRAGMA table_info`-geprüfter Migration umgebaut.
- Manifest: `concept_sources:` mit `redistribution: unknown` (CDM hat
  **keine** auffindbare Lizenz), schema-validiert wie jede andere Quelle;
  `hostus ingest` gibt Konzeptquellen samt aller Verlustzähler aus.
- Offline-Bundle: `sec_reference` (auf die Referenzräume der kopierten
  Konzepte gescopet, weil `title` geernteter Inhalt ist) und
  `concept_relation` (beidseitig gescopet) werden mitkopiert; das
  Redistributionsgatter verweigert einen Bundle-Export mit CDM-Daten per
  Default und protokolliert die Quelle unter `--force-include-restricted`.

### Fixed (SP5, Task 3 — Review)
- CDM-Ingest schreibt `parent_id` in einem **zweiten Unterlauf** (wie der
  WCVP-Pfad). Vorher brach der Ingest ab, sobald ein Kind vor seinem Elter
  in der Datei stand — gemessen am **vollen** Artefakt betrifft das 9.897
  der 33.731 Zeilen mit `parent_uuid` (29,34 %; die früher genannten
  „312 von 697" stammten aus einer Teilmessung und sind um rund das
  30-Fache zu niedrig, siehe SP5 Task 5). Die Fixture kann das nicht
  zeigen, daher ein SQLite-Test mit umgekehrter Reihenfolge.
- Der Umbau des `concept_relation`-Primärschlüssels läuft jetzt in **einer
  Transaktion**, prüft die Bedingung darin erneut, räumt eine
  Scratch-Tabelle vorher weg und verifiziert das Ergebnis per
  `PRAGMA foreign_key_check`. Beide Abbruchfenster der vorigen Fassung
  (unöffenbare Datenbank bzw. stiller Datenverlust) sind zusätzlich
  rückwärts reparierbar: eine liegengebliebene Scratch-Tabelle wird beim
  `Open` zurückgeführt.
- `internal/adapters/cdm`: Zeilennummern kommen aus `csv.Reader.FieldPos`
  bzw. `csv.ParseError` statt aus einem Satzzähler (der bei mehrzeiligen
  gequoteten Feldern driftet). Beschädigte Sätze brechen den Durchlauf
  **nicht** mehr ab — ein `csv.ParseError` verbraucht Eingabe, also wird
  jede spätere Zeile weiter gelesen; begrenzt (und dann ein **harter**
  Fehler) ist nur noch der Lesevorgang, der **gar keine Eingabe
  verbraucht** (klebriger I/O-Fehler). Vorher hätten 20 beschädigte Zeilen
  den Rest der Datei still verworfen und trotzdem Erfolg gemeldet.
- Die Wiederherstellung einer abgebrochenen `concept_relation`-Migration
  läuft jetzt **innerhalb** des `foreign_keys=OFF`-Fensters und prüft
  danach `PRAGMA foreign_key_check`. Vorher machte eine Scratch-Zeile mit
  nicht auflösbarem Ende die Datenbank dauerhaft unöffenbar — mit einem
  opaken Treiberfehler statt der benannten Meldung.
- Beide Migrationstransaktionen nutzen `BEGIN IMMEDIATE` (sie lesen und
  schreiben; unter WAL scheitert die Aufwertung eines einfachen `BEGIN`
  sonst mit `SQLITE_BUSY_SNAPSHOT`, das `busy_timeout` nicht wiederholt),
  und ein fehlgeschlagenes `PRAGMA foreign_keys = ON` verdeckt den
  Migrationsfehler nicht mehr (`errors.Join`).
- `internal/app/integration_test.go` an die neue `app.Ingest`-Signatur
  angepasst; `make test-integration` kompiliert und läuft wieder.

### Added (SP5, Vorarbeit)
- Pipeline `pipelines/cdm/` (`build.sh`, `crawl.py`, `convert.py`,
  `common.py`, README): resumierbare Ernte der 51.466 CDM-Konzepte aus
  `rl_standardliste` in 18 `sec.`-Referenzräumen plus des Konzept­
  relationsgraphen, ausgegeben als zwei kanonische, pipe-getrennte CSVs
  (`cdm-concepts-canonical.csv`, `cdm-relations-canonical.csv`). Trägt
  bewusst das **rohe** CDM-Vokabular in `rank` und `relation_type` (22 Ränge,
  7 Relationstypen gemessen — einer mehr als Task 1s Stichprobe sah); das
  Mapping gehört nach Task 3, wo ein unbekannter Wert laut abbricht
  (`ParseRank`-Lektion). Auflösung der Relationen über eine globale
  Kanten-Map ohne P8s Namensrestriktion, mit dem verbindlichen Falsifikator
  aus `docs/research/cdm-sample.md`: Abbruch ohne CSV, sobald eine
  Relations-UUID einen dritten Halter bekommt, plus Meldung der
  verbleibenden Ein-Halter-UUIDs. Crawl-Etikette (ein ehrlicher User-Agent,
  ≤ 1 req/s, harter Stopp statt Browser-UA bei 401/403, Backoff, Plattencache)
  ist verbindlich implementiert. `status` trägt ausschließlich das rohe
  `TaxonNodeDto.taxonStatus` und bleibt leer, wo der Baumlauf das Konzept
  nicht erreicht hat; nichts wird synthetisiert. Beide CSVs sind
  RFC-4180-gequotet (`csv`-Reader mit `Comma='|'` nötig, kein `split('|')`).
  Exit-Codes trennen Falsifikator (`3`) von jedem anderen Konvertierungs­
  fehler (`4`). Lizenzlage unverändert:
  **keine Lizenzangabe auffindbar → `redistribution: unknown`, nur lokale
  Auswertung**; committet ist neben den Skripten nur eine
  De-minimis-Testfixture von 32 Zeilen, damit die Task-3-Tests offline laufen
- Messsonde `poc/p08b_cdm_sample/` (`probe.sh` + `cdm_sample.py`, reproduzierbar
  über Seed `20260802` und die committete `sample.tsv`) und der Bericht
  `docs/research/cdm-sample.md`: die Zwei-Hop-Methode aus PoC P8 an einer
  geschichteten Stichprobe von 500 CDM-Konzepten gemessen. Kernbefunde:
  Relationsdichte ≈ 55 % datensatzweit (95-%-Cluster-Bootstrap 48–63 %),
  Relations-UUIDs sind sehr gut gestützt echte Kantenidentitäten (gemessen
  75,9 % Auflösung unter P8s Namensrestriktion, 79,1 % ohne sie; ≈ 100 % bei
  Vollcrawl projiziert, gestützt auf 202/202 anomaliefreie Kanten und mit
  einem Falsifikator für Task 2), drei Relationstypen außerhalb des
  angenommenen SP1-Vokabulars, `sec.`→Klassifikation nur über eine
  handgepflegte Crosswalk-Tabelle, Vollcrawl-Kosten 14–30 h bei 1 req/s

hostus wird vom zustandslosen GBIF-Autosuggest-Proxy zum lokalen
Multi-Backbone-Namens- und Merkmalsservice umgebaut (siehe
`docs/superpowers/specs/2026-07-31-hostus-2.0-architecture.md`). Dieser
Abschnitt sammelt den Abschluss von **SP0 (Harness & Skelett)**, **SP1
(Foundation)**, **SP2 (Suggest + Offline-Bundle)**, **SP3 (Traits +
Fuzzy-Match)** und **SP4 (Xref-Enrichment über eine Wikidata-Brücke)**: SP1
liefert das lokale SQLite/FTS5-Rückgrat selbst — Ingest eines
WCVP/POWO-DwC-A-Manifests
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
- **Deterministische Namensnormalisierung im Trait-Crosswalk**
  (`internal/domain/normalize.go`, `NameCandidates`): ein Trait-Name wird
  jetzt gegen eine geordnete Kandidatenleiter aufgelöst — zuerst
  unverändert `domain.Canonicalize` (identisches Verhalten wie bisher),
  danach je eine Regel für Hybridmarker (`Acer ×coriaceum` →
  `acer × coriaceum`, Marker ergänzen/entfernen, ASCII-`x`),
  Aggregatmarker (`Acer opalus aggr.`, `… s. l.`), infraspezifische
  Autonyme (`Acer obtusatum subsp. obtusatum`) und die
  `-ii`/`-i`-Genitivalternation (ICN Art. 60.8). Kein Fuzzy-Matching: jede
  Regel ist eine endliche, nomenklatorisch begründete Umschreibung, nichts
  wird bewertet. Gemessen an den Volldaten steigt die Taxon-Auflösbarkeit
  von 87,84 / 95,73 / 96,41 % auf **97,96 / 98,82 / 99,11 %** (EIVE /
  Tichý / Midolo), die nicht auflösbaren Trait-Zeilen sinken um
  84 / 72 / 75 % — vollständige Messung inkl. marginalem Zugewinn je Regel
  in `docs/research/reality-check.md`, Abschnitt „Nach Hardening (Task 5)".
  Zwei der Regeln setzen Umgrenzungen gleich, die nicht identisch sind
  (Aggregat → Nominatart, Autonym → Art). Sie sind deshalb **in den Daten**
  gekennzeichnet, nicht nur im Ingest-Report: neue nullable Spalte
  `trait_value.resolution` (NULL = exakter Treffer, sonst der Regelname),
  ausgeliefert als `resolution` auf `GET /v1/concept/{id}/traits`
  (`omitempty` — fehlt das Feld, war es ein exakter Treffer) und in jedes
  Offline-Bundle mitkopiert. Ein Konsument, der die Näherung nicht
  akzeptieren kann, filtert auf `resolution IN
  ('aggregate_to_nominate','autonym')`. Die Mehrdeutigkeitsquote steigt
  durch die Normalisierung (EIVE +231 Taxa) — ausschließlich aus vorher
  unauflösbaren Namen; mehrdeutig heißt weiterhin: es wird nichts
  geschrieben.
- **Exakte Treffer gewinnen den `trait_value`-Platz** (`application.
  selectTraitWinners`). Der Crosswalk ist viele-zu-eins (EIVE führt
  `Acer opalus` UND `Acer opalus aggr.`, beide lösen auf dasselbe Konzept
  auf), der Primärschlüssel von `trait_value` ist aber
  `(concept_id, vocab, vocab_version, dim)` und `AddTraitValue` ein
  `INSERT OR REPLACE` — bis hierher entschied damit die Zeilenreihenfolge
  der CSV, ob ein exakt getroffener Wert oder das Kollektivmittel eines
  Aggregats gespeichert wurde. Jetzt schlägt ein exakter Treffer immer
  einen normalisierten, unter Gleichrangigen gewinnt die erste Zeile.
  Messbare Folge: 646 EIVE- und 65 Tichý-Konzepte behalten ihren exakten
  Wert, den sie zuvor an einen normalisierten verloren hatten; die Zahl der
  exakt aufgelösten Konzepte je Vokabular entspricht damit wieder exakt der
  Baseline (11.000 / 7.072 / 4.963), die Normalisierung ist auf
  Konzeptebene beweisbar rein additiv.
- **`selectTraitWinners` rangiert normalisiert-gegen-normalisiert jetzt
  explizit** (`application.ruleRank`), statt bei einem Slot-Konflikt
  zwischen zwei normalisierten Zeilen einfach die CSV-Zeilenreihenfolge
  entscheiden zu lassen — derselbe Defekt eine Ebene unter dem oben
  beschriebenen exakt-vs-normalisiert-Fix: exakt > jede ungeflaggte
  Regel (reine Schreibweise) > jede geflaggte Regel (Aggregat→Nominatart,
  Autonym→Art), mit der `NameCandidates`-Reihenfolge als Tiebreak.
  Gemessen (`poc/measure/bridge --a1diff` gegen die Task-5-DB): 10 von
  117.153 gespeicherten `trait_value`-Zeilen ändern den Gewinner, alle bei
  EIVE, alle geflaggt→ungeflaggt.
- **Die Behauptung „exakt aufgelöste Konzepte == M2'-Baseline" ist jetzt
  maschinell nachvollziehbar statt nur behauptet**: `poc/measure/stats.sql`
  bekommt eine `resolution`-Aufschlüsselung je Vokabular
  (`GROUP BY vocab, resolution`), bestätigt gegen die echte Datenbank
  11.000/7.072/4.963 exakt.
- **Bewusster Lizenz-Scope-Schnitt (PoC R1)**: Euro+Med PlantBase,
  GermanSL, EuroSL und die FloraVeg.EU-Downloads werden NICHT ingestiert —
  für keine der vier Quellen ließ sich eine belastbare
  Weiterverbreitungslizenz feststellen (`docs/research/quellenregister.md`).
  Der taxonomische Anschluss für EIVE/Tichý/Midolo läuft deshalb direkt
  gegen den WCVP/POWO-Backbone, nicht über eine dieser vier Quellen als
  Vermittlungsschicht — dokumentiert in
  `docs/how-to/trait-ingest.md`, nicht stillschweigend übergangen.

- **Hardening-Zyklus (Tasks 1–6) abgeschlossen**: die in der
  Reality-Check-Volldaten-Messkampagne (siehe „Reality-Check T2–T4" oben)
  gefundenen Defekte sind behoben und am selben unveränderten
  WCVP-Volldatensatz (1.448.984 Zeilen) nachgemessen — konsolidierte
  Vorher/Nachher-Tabelle und aktualisierte Verdikte in
  `docs/research/reality-check.md`, Abschnitt „Task 6: konsolidierte
  Vorher/Nachher-Übersicht". Kernzahlen: Ingest läuft jetzt durch
  (vorher: Absturz nach 5,37 s bzw. quadratischer Abbruch nach 22:48 min)
  in 281,27 s mit linearer Skalierung (6/11/24 s statt 65/293/1.338 s bei
  50k/100k/200k Zeilen); Taxon-Ebenen-Crosswalk-Auflösbarkeit steigt durch
  Namensnormalisierung auf 97,96/98,82/99,11 % (vorher 87,84/95,73/96,41 %);
  Mehrgebiets-Bundle-Export und der Parameterlimit-Bug beim Voll-Export
  sind behoben, das Deutschland-Bundle schrumpft von 103,8 auf 81,05 MiB
  (gzip 19,24 MiB, unter der 20-MB-Spec-Obergrenze). Die Lizenz-Zurückstellungs-
  Empfehlung aus M6 ist nach der Normalisierung STÄRKER geworden, nicht nur
  bestätigt: der real brückbare Gewinn der vier lizenzunklaren Quellen
  schrumpft von 51/3/2 auf **2/1/1 Taxa** (EIVE/Tichý/Midolo), weil
  Normalisierung fast alles bereits einsammelt, was die Brücken sonst
  beigetragen hätten (`poc/measure/bridge --normbridge`).

- **Der letzte offene Punkt des Hardening-Zyklus (Suggest-p95) ist
  nachgemessen und geschlossen — es gab keine Regression** (Task 7,
  `docs/research/reality-check.md`, Abschnitt „Task 7: die offene
  p95-Abweichung — aufgelöst"). M3' hatte gegenüber M4 einen p95-Anstieg
  von 25–27 % berichtet und die Ursache offengelassen. 19
  Wiederholungsläufe desselben Messaufbaus (je 570 Messpunkte, `--reps 15
  --warmup 3`) streuen im p95 über **225,27–316,39 ms**
  (Variationskoeffizient 9,0 %, max/min 1,41) — die berichtete Differenz
  ist kleiner als die Streuung des Aufbaus. Die **Baseline-Konfiguration
  selbst** (Code vor dem Hardening, Commit `53575fe`, gegen die
  Baseline-DB `m2.sqlite`) misst heute 262–310 ms statt der notierten
  220,2 ms, also *schlechter* als die Nach-Hardening-Konfiguration. Beide
  Sachursachen sind belegt ausgeschlossen: die acht FK-Indizes standen
  bereits in der Baseline-DB (`poc/measure/fk_indexes.sql`, dieselben acht
  Spalten) und tauchen in keinem Query-Plan des Suggest-Pfads auf — die
  `EXPLAIN QUERY PLAN`-Ausgaben mit und ohne sie sind zeichengleich; die
  von T1 zugelassenen OTHER-Rang-Zeilen erhöhen die Kandidatenmenge der
  teuersten Präfixe um 0,24–0,49 %. Ein geprüfter Fix (`ANALYZE`) ändert
  den Plan nicht und bringt im verschränkten A/B-Test nichts und wurde
  deshalb **nicht** eingebaut. Kein Code geändert; der p50 (34,5–38,7 ms
  über alle Läufe, Variationskoeffizient 3,3 %) bestätigt das
  ursprüngliche M4-Verdikt **hält**.

- **SP4 (Xref-Enrichment über eine Wikidata-Brücke) abgeschlossen.** hostus
  kennt bislang nur den `powo`-Xref aus dem WCVP-Ingest selbst; SP4 bringt
  echte Cross-References zu sechs weiteren Autoritäten hinzu, ohne
  Wikidata direkt zur Laufzeit abzufragen:
  - `pipelines/wikidata/build.sh` erzeugt einmalig, offline, einen
    kanonischen Bridge-Hub-Export (`join_authority|join_id|authority|
    ext_id|wikidata_qid`, gegen den echten POWO-Xref-Bestand gejoint,
    1.709.127 Zeilen, 0 tote `join_id`s — siehe
    `.superpowers/sdd/2026-08-02-sp4-xref/task-1-report.md`).
  - `internal/adapters/xref` (Reader) und `application.IngestXrefs`
    (zweiphasiger, ID-basierter Join, `xref_sources` im Manifest, über
    `hostus ingest` wie die Trait-Vokabulare berichtet) schreiben die neuen
    Xrefs in einer einzigen Transaktion. Zwei Situationen werden getrennt
    behandelt: dieselbe `(authority, ext_id)`-Kombination, von zwei
    verschiedenen Konzepten beansprucht, wird **übersprungen und
    gemeldet** (kein Raten); ein Konzept mit mehreren `ext_id`s derselben
    Autorität ist **kein** Konflikt und wird vollständig geschrieben.
  - **BREAKING:** `xrefs.<authority>` in `GET /v1/concept/{id}` und
    `GET /v1/xref` ist jetzt ein sortiertes Array (`{"inat": ["160927"]}`)
    statt eines einzelnen Strings (`{"inat": "160927"}`) — jeder Client,
    der den Wert als String liest, muss angepasst werden. Die
    vorherige `map[string]string` hätte bei mehreren `ext_id`s pro
    Autorität stillschweigend nur die zuletzt geschriebene behalten, was
    real bei 63 Konzepten (`inat` allein) aufgetreten wäre.
  - Xref-Provenienz (`xref_source`-Tabelle + `xref.source`-Spalte, wie
    `backbone_version`/`trait_vocabulary`): eine ingestierte Datenbank
    beantwortet jetzt, aus welcher Ernte ihre Xrefs stammen (Version,
    Lizenz, `manifest_sha`), und der Redistributions-Gate von `hostus
    bundle` deckt damit auch Xref-Quellen ab — ein Export mit einer nicht
    freigegebenen Xref-Quelle im Scope wird per Default verweigert,
    `--force-include-restricted` protokolliert sie in
    `bundle_meta.restricted_sources`. Lokaler Ingest bleibt ungegated.
    Bestandsdatenbanken (SP1–SP3) migrieren beim nächsten `Open`
    automatisch (`ALTER TABLE xref ADD COLUMN source`, bestehende Zeilen
    behalten `source = NULL`).
  - Neue deutsche UC2-Anleitung `docs/how-to/inat-uc2.md` (Suggest →
    Concept → `xrefs.inat` → iNaturalist-Beobachtungen) inklusive der
    gemessenen PoC-Einschränkungen (Koordinatenverwischung ~26–28 km,
    32–38 % `obscured`, `quality_grade=research` = zwei
    Community-Zustimmungen statt Fachverifikation).
  - Volldaten-Messung (`docs/research/reality-check.md`, Abschnitte „SP4
    Task 2" und „SP4 Task 4 — Verdikt"): **392.218 / 440.534 Konzepte
    (89,03 %)** gewinnen mindestens einen neuen Xref; pro Autorität
    wikidata 392.218, gbif 383.907, wfo 365.731, colxr 357.878, **inat
    182.821 (41,50 %)**, floraveg 24.274, euromed 95; 16 echte Konflikte
    (8 externe Schlüssel, alle gbif/wfo); 0 von 1.709.127 Zeilen
    unmatched — Letzteres ist durch den joinable-subset-Filter der
    Pipeline garantiert und validiert den Ingest-Join, nicht die
    Abdeckung. **Verdikt: hält mit Auflagen** — der Ingest selbst ist
    vollständig korrekt und end-to-end bewiesen
    (`internal/app/integration_test.go`), aber UC2 erreicht wegen der
    Datenlage bei iNaturalist nur 41,50 % der Konzepte; für die übrigen
    58,5 % muss ein Client "keine iNat-Verknüpfung gefunden" ehrlich
    anzeigen statt zu raten.

Weitere Backbones (COL XR) sowie `translate` und `/openapi` als generierte
Spezifikation folgen in SP5+. `release-please` cuttet daraus das nächste
`2.0.0-alpha.N`-Release; bis dahin akkumulieren PRs hier.

### Fixed
- Xref-Konflikterkennung greift jetzt auch über Läufe hinweg: bisher
  gruppierte `application.IngestXrefs` nur die Zeilen der GERADE
  ingestierten Quelle, sodass eine `(authority, ext_id)`, die bereits in
  der Datenbank auf ein anderes Konzept zeigte (früherer Lauf, ältere CSV,
  zweite Quelle), von `INSERT OR REPLACE` stillschweigend umgehängt wurde —
  ohne Zählung, ohne Stichprobe. Phase 1 löst den bestehenden Eigentümer
  jedes externen Schlüssels jetzt mit auf (weiterhin ein Lesevorgang VOR
  der Transaktion) und behandelt eine Abweichung wie einen quellinternen
  Konflikt: übersprungen, gezählt, gesampelt. Ein unveränderter Re-Ingest
  bleibt idempotent.
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
