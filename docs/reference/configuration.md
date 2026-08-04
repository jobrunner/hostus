# Konfiguration

hostus liest Konfiguration mit folgender Priorität (niedrig → hoch):

1. `config.yaml` (optional, siehe `config.yaml.example`)
2. `HOSTUS_`-Umgebungsvariablen
3. CLI-Flags (z. B. `--port=443`, `--log-level=debug`)

Es gibt keinen Dotenv-Loader im Binary: `.env`/`example.env` sind eine
Convenience für `docker-compose` (`env_file:`), das sie als normale
Prozess-Umgebungsvariablen injiziert — sie bilden keine eigene Prioritätsstufe.

Alle Konfigurationsschlüssel haben eingebaute Defaults (siehe `Defaults()` in
`internal/config/config.go`). Umgebungsvariablen verwenden den Präfix
`HOSTUS_`, wobei `.` durch `_` ersetzt wird, z. B. `server.port` →
`HOSTUS_SERVER_PORT`. Die Regel gilt **ausnahmslos**: Abkürzungen wie
`HOSTUS_PORT` werden stillschweigend ignoriert, der Server startet dann auf
dem Default-Port 8080.

Ein vollständiges Beispiel mit Inline-Kommentaren liegt in
[`config.yaml.example`](https://github.com/jobrunner/hostus/blob/master/config.yaml.example)
und [`example.env`](https://github.com/jobrunner/hostus/blob/master/example.env)
im Repository.

## Wichtige Schlüssel

| Schlüssel / Env-Var                         | Standard    | Beschreibung                       |
|----------------------------------------------|-------------|-------------------------------------|
| `server.port` / `HOSTUS_SERVER_PORT`          | 8080        | Server-Port                        |
| `server.host` / `HOSTUS_SERVER_HOST`          | 0.0.0.0     | Bind-Adresse                       |
| `logging.level` / `HOSTUS_LOGGING_LEVEL`      | info        | Log-Level (debug/info/warn/error)  |
| `metrics.enabled` / `HOSTUS_METRICS_ENABLED`  | true        | Prometheus-Endpunkt aktivieren     |
| `tls.enabled` / `HOSTUS_TLS_ENABLED`          | false       | HTTPS/CertMagic aktivieren         |
| `cors.allowed_origins`                        | []          | Erlaubte CORS-Origins              |
| `ui.enabled` / `HOSTUS_UI_ENABLED`            | true        | Eingebettete Testkonsole unter `/` |

## Testkonsole (`ui.enabled`)

hostus liefert unter `/` eine eingebettete Testkonsole aus, mit der sich die
API von Hand ausprobieren lässt. Sie ist standardmäßig **an** und lässt sich
über alle drei Prioritätsstufen abschalten:

```yaml
# config.yaml
ui:
  enabled: false
```

```bash
HOSTUS_UI_ENABLED=false hostus serve
hostus serve --ui=false      # schlägt die Umgebungsvariable
```

Das CLI-Flag ist bewusst ein Wert-Flag: `--ui=false` muss möglich sein, sonst
könnte die oberste Prioritätsstufe die Konsole nur ein-, aber nie ausschalten.

Ist die Konsole aus, registriert der Router **gar nichts** unter `/`: sowohl
`/` als auch jeder Asset-Pfad antworten mit 404. Die API-Oberfläche (`/v1/*`,
`/health/*`, `/metrics`, `/openapi`) ist in beiden Fällen identisch — die
Konsole spricht dieselbe öffentliche HTTP-API wie ein Browser.

### Was die Konsole ausliefert

Die Konsole ist **ein einziges, in sich geschlossenes HTML-Dokument**: CSS und
JavaScript sind hineingeschrieben, nicht nachgeladen. Grund ist der globale
Token-Bucket (20 rps), den die Konsole sich mit der API teilt — ein Seitenaufruf
mit einem Dutzend Einzel-Requests würde genau das Budget aufbrauchen, dessen
Latenz man beobachten will.

Es gibt **keine externe Referenz**: kein CDN, keine Web-Schrift, kein Bild von
außerhalb. Das ist Bedingung, nicht Stil — hostus ist offline-first (UC1 ist der
Feldeinsatz mit einem Offline-Bundle). Abgesichert wird das durch den Header

```
Content-Security-Policy: default-src 'self'; script-src 'sha256-…';
  style-src 'sha256-…'; connect-src 'self'; base-uri 'none';
  form-action 'none'; frame-ancestors 'none'; object-src 'none'
```

Die beiden eingebetteten Blöcke sind per **SHA-256-Hash** zugelassen, nicht per
`'unsafe-inline'`: die Seite läuft, alles nachträglich Eingeschleuste nicht.

Pfadregeln:

| Pfad | Antwort |
| ---- | ------- |
| `/` | die Konsole |
| `/assets/app.js`, `/assets/style.css` | die Einzel-Assets mit eigenem `Content-Type` und `ETag` (die Seite selbst lädt sie nie) |
| unbekannter Pfad **außerhalb** `/v1`, `/health`, `/metrics`, `/openapi` (GET/HEAD) | die Konsole (SPA-Deep-Link) |
| unbekannter Pfad **unter** diesen Präfixen | 404 wie bisher |
| unbekannter Pfad, andere Methode als GET/HEAD | 404 wie bisher |

Dokument und Assets tragen ein starkes `ETag`; ein Reload kostet einen 304.
API-Antworten werden von der Seite **nie** zwischengespeichert (`no-store`) —
die Konsole soll zeigen, was der Dienst gerade geantwortet hat.
