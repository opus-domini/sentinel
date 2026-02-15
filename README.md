<div align="center">
    <img src="docs/assets/images/logo.png" alt="Sentinel logo" width="500"/>
    <hr />
    <p><strong>Your terminal watchtower</strong></p>
    <p>
        <a href="https://goreportcard.com/report/opus-domini/sentinel"><img src="https://goreportcard.com/badge/opus-domini/sentinel" alt="Go Report Badge"></a>
        <a href="https://pkg.go.dev/github.com/opus-domini/sentinel"><img src="https://pkg.go.dev/badge/github.com/opus-domini/sentinel.svg" alt="Go Package Docs Badge"></a>
        <a href="https://github.com/opus-domini/sentinel/actions/workflows/ci.yml"><img src="https://github.com/opus-domini/sentinel/actions/workflows/ci.yml/badge.svg" alt="Coverage Actions Badge"></a>
        <a href="https://github.com/opus-domini/sentinel/blob/main/LICENSE"><img src="https://img.shields.io/github/license/opus-domini/sentinel.svg" alt="License Badge"></a>
    </p>
</div>

Sentinel is a terminal-first workspace delivered as a single binary.
It gives you a realtime browser interface to operate tmux sessions, standalone terminals, and recovery workflows on your own host.

No Electron. No cloud relay. Just your machine and your shell.

<p align="center">
  <a href="https://opus-domini.github.io/sentinel/">Documentation</a> •
  <a href="https://github.com/opus-domini/sentinel/releases">Releases</a> •
  <a href="#quick-start">Quick Start</a>
</p>

## Why Sentinel

- One binary, fast setup, low operational overhead.
- Realtime tmux control with session, window, and pane visibility.
- Optimistic and responsive UI tuned for desktop and mobile.
- Built-in watchtower activity tracking and timeline.
- Built-in recovery snapshots and restore jobs.
- Guardrails for safer destructive terminal actions.

## Core Capabilities

- Interactive PTY terminal in the browser.
- Tmux workspace management (`Session > Window > Pane`).
- Standalone terminal tabs not tied to tmux.
- Event-driven updates over WebSocket (`/ws/events`).
- Service mode and daily autoupdate (Linux/macOS).
- Optional token auth and origin allowlist.

## Requirements

- Linux or macOS host.
- `tmux` installed for tmux workspace features.

## Quick Start

### Install

```bash
curl -fsSL https://raw.githubusercontent.com/opus-domini/sentinel/main/install.sh | bash
```

### Open Sentinel

- `http://127.0.0.1:4040`

### Validate Runtime

```bash
sentinel doctor
sentinel service status
```

### Security Baseline for Remote Access

If you expose Sentinel outside localhost (`0.0.0.0`), always configure:

- `token`
- `allowed_origins`

## Documentation

- [Getting Started](https://opus-domini.github.io/sentinel/#/guide/getting-started)
- [Architecture](https://opus-domini.github.io/sentinel/#/guide/architecture)
- [Security](https://opus-domini.github.io/sentinel/#/guide/security)
- [CLI Reference](https://opus-domini.github.io/sentinel/#/reference/cli)
- [HTTP API](https://opus-domini.github.io/sentinel/#/reference/http-api)
- [WebSocket and Events](https://opus-domini.github.io/sentinel/#/reference/websockets-events)
- [Troubleshooting](https://opus-domini.github.io/sentinel/#/troubleshooting/common-issues)

## Screenshots

![Desktop tmux sessions](docs/assets/images/desktop-tmux-sessions.png)

![Desktop tmux fullscreen](docs/assets/images/desktop-tmux-fullscreen.png)

<p align="center">
  <img src="docs/assets/images/mobile-tmux.png" alt="Mobile tmux view" width="320" />
</p>

## Stargazers over time ⭐

[![Stargazers over time](https://starchart.cc/opus-domini/sentinel.svg?variant=adaptive)](https://starchart.cc/opus-domini/sentinel)
