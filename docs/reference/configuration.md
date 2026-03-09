# Configuration Reference

Configuration precedence:

1. Environment variables
2. `config.toml`
3. Built-in defaults

## Files and Directories

- Data dir default: `~/.sentinel`
- Config file: `<data-dir>/config.toml`
- Database: `<data-dir>/sentinel.db`

When running as root, defaults resolve under root home (for example `/root/.sentinel`).

## Core Keys

```toml
[server]
listen = "127.0.0.1:4040"
token = ""
allowed_origins = ""
log_level = "info"
timezone = "America/Sao_Paulo"
locale = "pt-BR"
cookie_secure = "auto"
allow_insecure_cookie = false

[alerts]
cpu_percent = 90.0
mem_percent = 90.0
disk_percent = 95.0
webhook_url = ""
webhook_events = "alert.created,alert.resolved"

[health_report]
webhook_url = ""
schedule = ""

[watchtower]
enabled = true
tick_interval = "1s"
capture_lines = 80
capture_timeout = "150ms"
journal_rows = 5000

[runbooks]
max_concurrent = 5
```

## Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `SENTINEL_LISTEN` | `127.0.0.1:4040` | HTTP listen address |
| `SENTINEL_TOKEN` | empty | Auth token (cookie-based) |
| `SENTINEL_ALLOWED_ORIGINS` | empty | Comma-separated allowed origins |
| `SENTINEL_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `SENTINEL_DATA_DIR` | `~/.sentinel` | Data directory |
| `SENTINEL_TIMEZONE` | system timezone | IANA timezone for displayed timestamps |
| `SENTINEL_LOCALE` | empty (browser default) | BCP 47 locale for date/number formatting |
| `SENTINEL_COOKIE_SECURE` | `auto` | Cookie secure flag: `auto`, `always`, `never` |
| `SENTINEL_ALLOW_INSECURE_COOKIE` | `false` | Allow auth cookie over plain HTTP |
| `SENTINEL_ALERT_CPU_PERCENT` | `90` | CPU usage alert threshold (percent) |
| `SENTINEL_ALERT_MEM_PERCENT` | `90` | Memory usage alert threshold (percent) |
| `SENTINEL_ALERT_DISK_PERCENT` | `95` | Disk usage alert threshold (percent) |
| `SENTINEL_ALERT_WEBHOOK_URL` | empty | Webhook URL for alert notifications |
| `SENTINEL_ALERT_WEBHOOK_EVENTS` | `alert.created,alert.resolved` | Comma-separated alert webhook events |
| `SENTINEL_HEALTH_REPORT_WEBHOOK_URL` | empty | Webhook URL for health report delivery |
| `SENTINEL_HEALTH_REPORT_SCHEDULE` | empty | Cron schedule for health reports |
| `SENTINEL_WATCHTOWER_ENABLED` | `true` | Enable watchtower service |
| `SENTINEL_WATCHTOWER_TICK_INTERVAL` | `1s` | Watchtower collect interval |
| `SENTINEL_WATCHTOWER_CAPTURE_LINES` | `80` | Pane tail capture lines |
| `SENTINEL_WATCHTOWER_CAPTURE_TIMEOUT` | `150ms` | Per-pane capture timeout |
| `SENTINEL_WATCHTOWER_JOURNAL_ROWS` | `5000` | Activity journal retention |
| `SENTINEL_RUNBOOK_MAX_CONCURRENT` | `5` | Max concurrent manual runbook executions |

## Recommended Profiles

### Local-only development

```toml
[server]
listen = "127.0.0.1:4040"
token = ""
```

### Remote access (minimum safe baseline)

```toml
[server]
listen = "0.0.0.0:4040"
token = "strong-secret"
allowed_origins = "https://sentinel.example.com"
```
