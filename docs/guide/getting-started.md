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

## Configure Listen and Token

Edit `~/.sentinel/config.toml` (or root data dir when running as root):

```toml
listen = "127.0.0.1:4040"
token = ""
allowed_origins = ""
log_level = "info"
```

Remote access baseline:

```toml
listen = "0.0.0.0:4040"
token = "replace-with-strong-token"
allowed_origins = "https://sentinel.example.com"
```

Restart service after config changes.

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
