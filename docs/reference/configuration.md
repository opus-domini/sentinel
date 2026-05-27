# Configuration Reference

Configuration precedence:

1. Environment variables
2. `config.toml`
3. Built-in defaults

## Files and Directories

- Config file: `~/.sentinel/config.toml`
- Database: `~/.sentinel/sentinel.db`
- Daemon log: `~/.sentinel/logs/sentinel.log`

`SENTINEL_DATA_DIR` changes the default data root. `SENTINEL_CONFIG` or
`sentinel --config <path>` changes only the config file path.

Use the CLI as the source of truth:

```bash
sentinel config path
sentinel config init
sentinel config edit
sentinel config validate
sentinel config show
sentinel db init
```

`sentinel config init --force` overwrites an existing config file.
`sentinel config show` prints the effective runtime configuration as JSON with
secret values redacted.

## Config File

```toml
version = 1

[server]
host = "127.0.0.1"
port = 4040
token = ""
allowed_origins = []
cookie_secure = "auto"
allow_insecure_cookie = false
timezone = "America/Sao_Paulo"
locale = "pt-BR"

[storage]
path = "~/.sentinel/sentinel.db"

[log]
level = "info"
path = "~/.sentinel/logs/sentinel.log"

[alerts]
cpu_percent = 90.0
mem_percent = 90.0
disk_percent = 95.0
webhook_url = ""
webhook_events = ["alert.created", "alert.resolved", "alert.acked"]

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

[multi_user]
allowed_users = []
allow_root_target = false
user_switch_method = "systemd-run"
```

## Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `SENTINEL_CONFIG` | `<data-dir>/config.toml` | Config file path |
| `SENTINEL_DATA_DIR` | `~/.sentinel` | Default data root |
| `SENTINEL_SERVER_HOST` | `127.0.0.1` | HTTP listen host |
| `SENTINEL_SERVER_PORT` | `4040` | HTTP listen port |
| `SENTINEL_SERVER_TOKEN` | empty | Auth token |
| `SENTINEL_SERVER_ALLOWED_ORIGINS` | empty | Comma-separated allowed origins |
| `SENTINEL_SERVER_COOKIE_SECURE` | `auto` | Cookie secure flag: `auto`, `always`, `never` |
| `SENTINEL_SERVER_ALLOW_INSECURE_COOKIE` | `false` | Allow auth cookie over plain HTTP |
| `SENTINEL_SERVER_TIMEZONE` | system timezone | IANA timezone for displayed timestamps |
| `SENTINEL_SERVER_LOCALE` | empty | BCP 47 locale for date/number formatting |
| `SENTINEL_STORAGE_PATH` | `~/.sentinel/sentinel.db` | SQLite database path |
| `SENTINEL_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `SENTINEL_LOG_PATH` | `~/.sentinel/logs/sentinel.log` | Daemon log file path |
| `SENTINEL_ALERT_CPU_PERCENT` | `90` | CPU usage alert threshold |
| `SENTINEL_ALERT_MEM_PERCENT` | `90` | Memory usage alert threshold |
| `SENTINEL_ALERT_DISK_PERCENT` | `95` | Disk usage alert threshold |
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
| `SENTINEL_ALLOWED_USERS` | empty | Comma-separated OS users allowed as session targets |
| `SENTINEL_ALLOW_ROOT_TARGET` | `false` | Whether to allow targeting root |
| `SENTINEL_USER_SWITCH_METHOD` | `systemd-run` on Linux, `sudo` elsewhere | User switch method |

## Recommended Profiles

### Local-only development

```toml
[server]
host = "127.0.0.1"
port = 4040
token = ""
```

### Remote access

```toml
[server]
host = "0.0.0.0"
port = 4040
token = "strong-secret"
allowed_origins = ["https://sentinel.example.com"]
```
