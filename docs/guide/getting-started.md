# Getting Started

This guide covers installation, first run, and baseline production-safe setup.

## Requirements

- Linux or macOS
- `tmux` installed (required for tmux workspace features)
- Browser with WebSocket support

## Install

### Recommended

```bash
curl -fsSL https://raw.githubusercontent.com/opus-domini/sentinel/main/install.sh | bash
```

Installer behavior:

- Linux user install: `~/.local/bin/sentinel` + user `systemd` service.
- Linux root install: `/usr/local/bin/sentinel` + system `systemd` service.
- macOS user install: `~/.local/bin/sentinel` + `LaunchAgents` plist.
- macOS root install: `/usr/local/bin/sentinel` + `LaunchDaemons` plist.

### Alternative (Go)

```bash
go install github.com/opus-domini/sentinel/cmd/sentinel@latest
```

## First Run Checklist

1. Verify runtime:

```bash
sentinel doctor
```

2. Check service:

```bash
sentinel service status
```

3. Open UI:

- `http://127.0.0.1:4040`

4. Open Settings and verify:

- Sentinel version
- Token/auth state
- Storage and guardrail tabs

![Desktop settings token](assets/images/desktop-settings-token.png)

## Configure Listen and Token

Initialize and edit the config file:

```bash
sentinel config init
sentinel config edit
```

`sentinel config edit` opens the canonical file from `sentinel config path`.
Set `EDITOR="code --wait"` or another blocking editor command when you want
automatic validation after saving.

```toml
[server]
host = "127.0.0.1"
port = 4040
token = ""
allowed_origins = []
trusted_proxies = []

[log]
level = "info"
```

Remote access baseline:

```toml
[server]
host = "0.0.0.0"
port = 4040
token = "replace-with-strong-token"
allowed_origins = ["https://sentinel.example.com"]
# Optional: only when a reverse proxy terminates TLS for Sentinel.
trusted_proxies = ["127.0.0.1"]
```

Validate before restarting the service:

```bash
sentinel config validate
```

## Enable Daily Autoupdate

```bash
sentinel service autoupdate install
sentinel service autoupdate status
```

Linux root/system scope:

```bash
sentinel service autoupdate install --scope system
```

## Useful First Commands

```bash
sentinel --help
sentinel --version
sentinel update check
sentinel recovery list
```
