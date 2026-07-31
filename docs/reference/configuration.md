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
`HOSTUS_SERVER_PORT`.

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
