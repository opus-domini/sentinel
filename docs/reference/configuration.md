# Configuration Reference

Configuration precedence:

1. Environment variables
2. `config.toml`
3. Built-in defaults

Sentinel resolves this precedence through one configuration boundary shared by
daemon startup, `sentinel config`, and the Settings API. Each field retains its
source (`environment`, `file`, or `default`) independently of its final value.
An environment-owned field is effective but read-only to file-based updates;
remove the corresponding `SENTINEL_*` variable before changing it in the file.

## Files and Directories

Managed deployments use one layout selected by installation scope:

| Scope | Config | Database and mutable state | Daemon log |
| --- | --- | --- | --- |
| Linux system | `/etc/sentinel/config.toml` | `/var/lib/sentinel` | `/var/log/sentinel/sentinel.log` |
| Linux user | `~/.sentinel/config.toml` | `~/.sentinel` | `~/.sentinel/logs/sentinel.log` |
| macOS system | `/Library/Preferences/io.opusdomini.sentinel.toml` | `/Library/Application Support/Sentinel` | `/Library/Logs/Sentinel/sentinel.log` |
| macOS user | `~/.sentinel/config.toml` | `~/.sentinel` | `~/.sentinel/logs/sentinel.log` |

`/root/.sentinel` and `/var/root/.sentinel` are legacy system layouts. They are
never selected for a fresh managed installation.

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

Managed config and database commands automatically target the installed
deployment. Use `--scope user|system` when both intent and privileges must be
explicit. `sentinel config init --force` overwrites the selected config file.
`sentinel config show` prints the effective runtime configuration as JSON with
secret values redacted.

Managed writes render the complete canonical TOML document. They do not
preserve comments or custom ordering. A save:

- locks the adjacent `<configPath>.lock` file to reject concurrent writers;
- creates the config on first use with mode `0600`;
- keeps the previous document as `<configPath>.bak`, also with mode `0600`;
- writes through a same-directory temporary file, syncs it, and atomically
  renames it into place;
- restores the previous bytes if a subsequent live-application step fails.

The revision used for optimistic concurrency is derived from the raw config
bytes and whether the file exists, so a missing file and an empty file are
different states.

Validation is strict:

- every `allowed_origins` entry must be a canonical `http://` or `https://`
  origin without a path, query, fragment, or credentials;
- every `trusted_proxies` entry must be an IP address or CIDR;
- IPv4 and IPv6 loopback are trusted proxy peers by default; other TLS
  terminator addresses must be listed in `trusted_proxies`.
- `locale` must be empty or one of the locales supported by the application;
- `timezone` must be `Local` or a valid IANA timezone;
- watchtower durations and limits, Runbook concurrency, and health-report cron
  schedules must remain inside their documented runtime limits;
- webhook URLs must be empty or absolute HTTP(S) URLs with a host and without
  userinfo or fragments;
- `user_switch_method` accepts only `sudo` or `systemd-run`;
- invalid non-empty cookie security or user-switch values are rejected rather
  than normalized to a fallback.

## Config File

```toml
version = 1

[server]
host = "127.0.0.1"
port = 4040
token = ""
allowed_origins = []
trusted_proxies = []
cookie_secure = "auto"
allow_insecure_cookie = false
timezone = "America/Sao_Paulo"
locale = "pt-BR"

[storage]
path = "~/.sentinel/sentinel.db"

[log]
level = "info"
path = "~/.sentinel/logs/sentinel.log"

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

[mcp]
enabled = false

[multi_user]
allowed_users = []
allow_root_target = false
user_switch_method = "systemd-run"
```

## Environment Variables

| Variable                                | Default                                  | Description                                                     |
| --------------------------------------- | ---------------------------------------- | --------------------------------------------------------------- |
| `SENTINEL_CONFIG`                       | `<data-dir>/config.toml`                 | Config file path                                                |
| `SENTINEL_DATA_DIR`                     | `~/.sentinel`                            | Default data root                                               |
| `SENTINEL_SERVER_HOST`                  | `127.0.0.1`                              | HTTP listen host                                                |
| `SENTINEL_SERVER_PORT`                  | `4040`                                   | HTTP listen port                                                |
| `SENTINEL_SERVER_TOKEN`                 | empty                                    | Auth token                                                      |
| `SENTINEL_SERVER_ALLOWED_ORIGINS`       | empty                                    | Comma-separated allowed origins                                 |
| `SENTINEL_SERVER_TRUSTED_PROXIES`       | empty                                    | Comma-separated proxy IPs/CIDRs trusted for `X-Forwarded-Proto` |
| `SENTINEL_SERVER_COOKIE_SECURE`         | `auto`                                   | Cookie secure flag: `auto`, `always`, `never`                   |
| `SENTINEL_SERVER_ALLOW_INSECURE_COOKIE` | `false`                                  | Allow auth cookie over plain HTTP                               |
| `SENTINEL_SERVER_TIMEZONE`              | system timezone                          | IANA timezone for displayed timestamps                          |
| `SENTINEL_SERVER_LOCALE`                | empty                                    | BCP 47 locale for date/number formatting                        |
| `SENTINEL_STORAGE_PATH`                 | `~/.sentinel/sentinel.db`                | SQLite database path                                            |
| `SENTINEL_LOG_LEVEL`                    | `info`                                   | `debug`, `info`, `warn`, `error`                                |
| `SENTINEL_LOG_PATH`                     | `~/.sentinel/logs/sentinel.log`          | Daemon log file path                                            |
| `SENTINEL_HEALTH_REPORT_WEBHOOK_URL`    | empty                                    | Webhook URL for health report delivery                          |
| `SENTINEL_HEALTH_REPORT_SCHEDULE`       | empty                                    | Cron schedule for health reports                                |
| `SENTINEL_WATCHTOWER_ENABLED`           | `true`                                   | Enable watchtower service                                       |
| `SENTINEL_WATCHTOWER_TICK_INTERVAL`     | `1s`                                     | Watchtower collect interval                                     |
| `SENTINEL_WATCHTOWER_CAPTURE_LINES`     | `80`                                     | Pane tail capture lines                                         |
| `SENTINEL_WATCHTOWER_CAPTURE_TIMEOUT`   | `150ms`                                  | Per-pane capture timeout                                        |
| `SENTINEL_WATCHTOWER_JOURNAL_ROWS`      | `5000`                                   | Tmux activity retention                                         |
| `SENTINEL_RUNBOOK_MAX_CONCURRENT`       | `5`                                      | Max concurrent manual runbook executions                        |
| `SENTINEL_MCP_ENABLED`                  | `false`                                  | Expose the Streamable HTTP MCP endpoint at `/mcp`                |
| `SENTINEL_ALLOWED_USERS`                | empty                                    | Comma-separated OS users allowed as session targets             |
| `SENTINEL_ALLOW_ROOT_TARGET`            | `false`                                  | Whether to allow targeting root                                 |
| `SENTINEL_USER_SWITCH_METHOD`           | `systemd-run` on Linux, `sudo` elsewhere | User switch method                                              |

Invalid environment overrides fail configuration loading. They are never
silently ignored in favor of the file or a default.

## Settings validation limits

Settings exposes the same constraints enforced by startup, CLI validation, and
the typed PATCH endpoint:

| Field | Accepted value |
| --- | --- |
| `watchtower.enabled` | `true` or `false` |
| `watchtower.tick_interval` | `100ms` through `1m` |
| `watchtower.capture_lines` | `1` through `2000` |
| `watchtower.capture_timeout` | `10ms` through `10s` |
| `watchtower.journal_rows` | `100` through `1,000,000` |
| `runbooks.max_concurrent` | `1` through `64` |
| `log.level` | `debug`, `info`, `warn`, or `error` |
| `health_report.schedule` | empty, a valid five-field cron, or supported `@descriptor` |
| `health_report.webhook_url` | empty or absolute HTTP(S), with host and no userinfo or fragment |

These values are restart-based. Saving updates `config.toml` atomically and
marks the changed keys as restart pending; it does not partially reload
Watchtower, the Runbook manager, the logger, or the health-report scheduler.
Environment-owned fields remain read-only and a forged PATCH is rejected.

Managed systemd and launchd definitions do not set `SENTINEL_LOG_LEVEL`.
Therefore an explicit `[log].level` value is no longer shadowed by the service
template.

## Write-only secrets and integrations

Settings represents `server.token` and `health_report.webhook_url` with
non-secret metadata only: configured state, source, editability, apply mode,
and restart state. GET responses never include persisted, effective, or
default secret values.

PATCH uses an explicit action object:

```json
{ "action": "keep" }
{ "action": "replace", "value": "<new value>" }
{ "action": "clear" }
```

`keep` and `clear` reject an accompanying value. `replace` requires a non-empty
value. Environment-owned secrets reject all file mutations.

The health-report cron and webhook are restart-based. Sentinel calculates the
next activation on the server using the same parser used at daemon startup and
returns it only when both a valid schedule and webhook are configured.

MCP uses `server.token`; there is no separate credential. Disabling MCP applies
live. Enabling applies live only when the process started with a token. A token
saved through Settings is persisted write-only, and a simultaneous MCP enable
is marked restart pending until that token becomes part of the startup
baseline.

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
# Set only when the direct reverse proxy peer is not on loopback.
trusted_proxies = []

[mcp]
enabled = true
```

Use the address Sentinel sees as the direct peer. Local Tailscale Serve, nginx,
and Caddy proxies commonly use loopback and need no entry. `sentinel doctor` reports
the exact invalid field when this configuration is incoherent.

MCP uses `server.token`; there is no separate MCP secret. Configuration
validation rejects `mcp.enabled = true` when the shared token is empty.
