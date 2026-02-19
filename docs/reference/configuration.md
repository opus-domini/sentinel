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
listen = "127.0.0.1:4040"
token = ""
allowed_origins = ""
log_level = "info"

watchtower_enabled = true
watchtower_tick_interval = "1s"
watchtower_capture_lines = 80
watchtower_capture_timeout = "150ms"
watchtower_journal_rows = 5000

recovery_enabled = true
recovery_snapshot_interval = "5s"
recovery_capture_lines = 80
recovery_max_snapshots = 300
```

## Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `SENTINEL_LISTEN` | `127.0.0.1:4040` | HTTP listen address |
| `SENTINEL_TOKEN` | empty | Bearer token for HTTP/WS auth |
| `SENTINEL_ALLOWED_ORIGINS` | empty | Comma-separated allowed origins |
| `SENTINEL_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `SENTINEL_DATA_DIR` | `~/.sentinel` | Data directory |
| `SENTINEL_WATCHTOWER_ENABLED` | `true` | Enable watchtower service |
| `SENTINEL_WATCHTOWER_TICK_INTERVAL` | `1s` | Watchtower collect interval |
| `SENTINEL_WATCHTOWER_CAPTURE_LINES` | `80` | Pane tail capture lines |
| `SENTINEL_WATCHTOWER_CAPTURE_TIMEOUT` | `150ms` | Per-pane capture timeout |
| `SENTINEL_WATCHTOWER_JOURNAL_ROWS` | `5000` | Activity journal retention |
| `SENTINEL_RECOVERY_ENABLED` | `true` | Enable recovery service |
| `SENTINEL_RECOVERY_SNAPSHOT_INTERVAL` | `5s` | Recovery snapshot interval |
| `SENTINEL_RECOVERY_CAPTURE_LINES` | `80` | Recovery pane capture lines |
| `SENTINEL_RECOVERY_MAX_SNAPSHOTS` | `300` | Max snapshots per session |

## Recommended Profiles

### Local-only development

```toml
listen = "127.0.0.1:4040"
token = ""
```

### Remote access (minimum safe baseline)

```toml
listen = "0.0.0.0:4040"
token = "strong-secret"
allowed_origins = "https://sentinel.example.com"
```
