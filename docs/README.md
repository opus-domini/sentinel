<div align="center">
  <img src="assets/images/logo.png" alt="Sentinel logo" width="560" />
  <hr />
  <p><strong>Your terminal watchtower</strong></p>
  <p>
      <a href="https://goreportcard.com/report/opus-domini/sentinel"><img src="https://goreportcard.com/badge/opus-domini/sentinel" alt="Go Report Badge"></a>
      <a href="https://godoc.org/github.com/opus-domini/sentinel"><img src="https://godoc.org/github.com/opus-domini/sentinel?status.svg" alt="Go Doc Badge"></a>    
      <a href="https://github.com/opus-domini/sentinel/actions/workflows/ci.yml"><img src="https://github.com/opus-domini/sentinel/actions/workflows/ci.yml/badge.svg" alt="Converage Actions Badge"></a>
      <a href="https://github.com/opus-domini/sentinel/blob/main/LICENSE"><img src="https://img.shields.io/github/license/opus-domini/sentinel.svg" alt="License Badge"></a>      
  </p>
</div>

Sentinel is a terminal-first workspace delivered as a single binary.
It gives you an interactive browser UI to manage tmux sessions, run standalone terminals, and inspect active host terminals in one place.

No Electron. No cloud relay. Just your machine and your shell.

## Why Sentinel

Sentinel is for people who want terminal power with less friction.

- Real PTY terminals in the browser.
- tmux session, window, and pane control with live attach.
- Standalone shell tabs that are not tied to tmux.
- Optional token auth and origin validation.
- One binary and simple operations.

## Screenshots

Tip: click any image to zoom.

### Desktop

Session and pane management with full tmux visibility.

![Desktop tmux sessions](assets/images/desktop-tmux-sessions.png)

Interactive terminal focused on the active workspace.

![Desktop tmux fullscreen](assets/images/desktop-tmux-fullscreen.png)

### Mobile

Responsive terminal workflow with touch-first controls.

<p align="center">
  <img src="assets/images/mobile-tmux.png" alt="Mobile tmux view" width="320" />
</p>

### Settings

Theme customization and terminal identity tuning.

![Theme settings](assets/images/settings-theme.png)

Token authentication setup for protected access.

![Token settings](assets/images/settings-token.png)

## Installation

### 1) Recommended: GitHub release installer

```bash
curl -fsSL https://raw.githubusercontent.com/opus-domini/sentinel/main/install.sh | bash
```

What `install.sh` does on Linux:

- regular user:
  - installs binary to `~/.local/bin/sentinel`
  - installs `~/.config/systemd/user/sentinel.service`
  - starts the service (`systemctl --user start sentinel`)
- root:
  - installs binary to `/usr/local/bin/sentinel`
  - installs `/etc/systemd/system/sentinel@.service`
  - starts `sentinel@root` (or `sentinel@$SYSTEMD_TARGET_USER`)
- optional persistence:
  - `systemctl --user enable sentinel` (regular user)
  - `systemctl enable sentinel@root` (root)

### 2) Go install

Requirements:

- `Go 1.25+`
- Linux or macOS

From GitHub:

```bash
go install github.com/opus-domini/sentinel/cmd/sentinel@latest
```

From local checkout:

```bash
go install ./cmd/sentinel
```

Go puts the binary in `GOBIN` (or `$(go env GOPATH)/bin` when `GOBIN` is empty).
If needed:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

For development from source, see `CONTRIBUTING.md`.

## After Installation (User Journey)

### 1) Confirm service status

`install.sh` already starts the service for you.

If installed as regular user:

```bash
systemctl --user status sentinel
journalctl --user -u sentinel -f
```

If installed as root:

```bash
systemctl status sentinel@root
journalctl -u sentinel@root -f
```

If you used `SYSTEMD_TARGET_USER`, replace `root` with that user in the unit name.

Optional: persist across reboot/login.

```bash
# regular user
systemctl --user enable sentinel

# root/system install
systemctl enable sentinel@root
```

If you used `SYSTEMD_TARGET_USER`, enable `sentinel@your-user` instead.

### 2) Open Sentinel

Open `http://127.0.0.1:4040`.

Default binding is local-only (`127.0.0.1:4040`).

### 3) Edit config file (recommended)

Sentinel uses:

- config file: `~/.sentinel/config.toml`
- data dir: `~/.sentinel`

The config file is created on first startup.

## CLI Subcommands

- `sentinel` or `sentinel serve`: start server.
- `sentinel service install`: install systemd user service (Linux).
- `sentinel service uninstall`: remove systemd user service (Linux).
- `sentinel service status`: show service state.
- `sentinel doctor`: print environment diagnostics.
- `sentinel -h` / `sentinel --help`: help.
- `sentinel -v` / `sentinel --version`: version.

Examples:

```bash
sentinel serve
sentinel service install
sentinel service status
sentinel doctor
sentinel --help
sentinel --version
```

## Configuration File (Recommended)

By default Sentinel listens on:

```toml
listen = "127.0.0.1:4040"
```

For remote access, update `~/.sentinel/config.toml`:

```toml
listen = "0.0.0.0:4040"
token = "strong-token"
allowed_origins = ["https://sentinel.example.com"]
log_level = "info"
```

After editing config, restart the service.

Regular user service:

```bash
systemctl --user restart sentinel
```

Root/system service:

```bash
systemctl restart sentinel@root
```

If you used `SYSTEMD_TARGET_USER`, restart `sentinel@your-user` instead.

Security recommendations:

- never expose Sentinel without token auth;
- prefer private networking (Tailscale) or authenticated Cloudflare Tunnel;
- set `allowed_origins` explicitly when using reverse proxies.

## Advanced: Environment Variables

Environment variables override config file values and are useful for technical/automation scenarios.

| Environment variable | Config key | Default | Description |
| --- | --- | --- | --- |
| `SENTINEL_LISTEN` | `listen` | `127.0.0.1:4040` | Listen address |
| `SENTINEL_TOKEN` | `token` | disabled | Bearer token for API and WS auth |
| `SENTINEL_ALLOWED_ORIGINS` | `allowed_origins` | auto | Comma-separated allowlist |
| `SENTINEL_LOG_LEVEL` | `log_level` | `info` | `debug`, `info`, `warn`, `error` |
| `SENTINEL_DATA_DIR` | n/a | `~/.sentinel` | Data directory |

## Current Limitations

- Host support: Linux and macOS only.
- Windows is not supported yet.
- tmux workflows require `tmux` installed on the host.
- No multi-tenant RBAC yet.

## Development

```bash
make dev
make build
make test
make test-client
make lint
make lint-client
make ci
```

## Contributing

Pull requests are welcome.

1. Fork repository
2. Create a feature branch
3. Run `make ci`
4. Open a pull request
