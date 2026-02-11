<div align="center">
    <img src="docs/assets/images/logo.png" alt="Sentinel logo" width="500"/>
    <hr />
    <p>Your terminal watchtower.</p>
</div>

Sentinel is a terminal-first workspace delivered as a single binary.
It gives you an interactive browser UI to manage tmux sessions, run standalone terminals, and inspect active host terminals in one place.

No Electron. No cloud relay. Just your machine and your shell.

## Why Sentinel?

- Real PTY terminals in the browser.
- tmux session, window, and pane control with live attach.
- Standalone shell tabs that are not tied to tmux.
- Optional token auth and origin validation.
- One binary and simple operations.

## Screenshots

### Desktop

Session and pane management with full tmux visibility.

![Desktop tmux sessions](docs/assets/images/desktop-tmux-sessions.png)

Interactive terminal focused on the active workspace.

![Desktop tmux fullscreen](docs/assets/images/desktop-tmux-fullscreen.png)

### Mobile

Responsive terminal workflow with touch-first controls.

<p align="center">
  <img src="docs/assets/images/mobile-tmux.png" alt="Mobile tmux view" width="320" />
</p>

### Settings

Theme customization and terminal identity tuning.

![Theme settings](docs/assets/images/settings-theme.png)

Token authentication setup for protected access.

![Token settings](docs/assets/images/settings-token.png)

## Quick Start

### 1) Recommended: GitHub release installer

```bash
curl -fsSL https://raw.githubusercontent.com/opus-domini/sentinel/main/install.sh | bash
```

Optional overrides:

- `INSTALL_DIR=/usr/local/bin`
- `VERSION=vX.Y.Z`

### 2) Go install

```bash
go install github.com/opus-domini/sentinel/cmd/sentinel@latest
```

Or from local checkout:

```bash
go install ./cmd/sentinel
```

Go puts the binary in `GOBIN` (or `$(go env GOPATH)/bin` when `GOBIN` is empty).
If needed:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

### 3) Run from source

Requirements:

- `Go 1.25+`
- `Node.js 20+`
- `npm`
- Linux or macOS
- `tmux` (required only for tmux workflows)

```bash
git clone https://github.com/opus-domini/sentinel.git
cd sentinel
make run
```

Open `http://127.0.0.1:4040`.

## After Installation

Start Sentinel in foreground:

```bash
sentinel serve
```

Run Sentinel as a Linux user daemon:

```bash
sentinel service install
sentinel service status
```

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

## Running as a Service

### Linux user service (recommended)

```bash
sentinel service install
sentinel service status
journalctl --user -u sentinel -f
```

Optional boot start without interactive login:

```bash
sudo loginctl enable-linger "$USER"
```

### Linux user service (manual)

```bash
systemctl --user enable --now sentinel
journalctl --user -u sentinel -f
```

### Linux system-level template service

```bash
sudo cp contrib/sentinel@.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now sentinel@your-user
```

### macOS

`install.sh` installs the binary and prints launchd guidance.

## Configuration

Environment variables override config file values.

| Environment variable | Config key | Default | Description |
| --- | --- | --- | --- |
| `SENTINEL_LISTEN` | `listen` | `127.0.0.1:4040` | Listen address |
| `SENTINEL_TOKEN` | `token` | disabled | Bearer token for API and WS auth |
| `SENTINEL_ALLOWED_ORIGINS` | `allowed_origins` | auto | Comma-separated allowlist |
| `SENTINEL_LOG_LEVEL` | `log_level` | `info` | `debug`, `info`, `warn`, `error` |
| `SENTINEL_DATA_DIR` | n/a | `~/.sentinel` | Data directory |

Example:

```bash
SENTINEL_TOKEN='replace-this' \
SENTINEL_LOG_LEVEL=debug \
./build/sentinel
```

## Remote Access

For local-only usage keep `127.0.0.1:4040`.
For remote usage, bind to `0.0.0.0` and place Sentinel behind private networking or an authenticated tunnel.

```bash
SENTINEL_LISTEN=0.0.0.0:4040 \
SENTINEL_TOKEN='strong-token' \
SENTINEL_ALLOWED_ORIGINS='https://sentinel.example.com' \
./build/sentinel
```

Recommended:

- never expose without token auth;
- prefer Tailscale or authenticated Cloudflare Tunnel;
- set `SENTINEL_ALLOWED_ORIGINS` explicitly when behind reverse proxy.

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
