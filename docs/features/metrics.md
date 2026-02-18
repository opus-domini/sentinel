# Metrics

![Desktop metrics](assets/images/desktop-metrics.png)

The Metrics page (`/metrics`) provides real-time system and runtime metrics for the host machine and the Sentinel process. All data is collected locally by Sentinel with no external monitoring agents required.

## System Metrics

Host-level resource metrics collected from the OS:

- **CPU** — usage percentage across all cores.
- **Memory** — used/total bytes and utilization percentage.
- **Disk** — used/total bytes and utilization percentage.
- **Load averages** — 1-minute, 5-minute, and 15-minute values.

Visual indicators use colored progress bars with green/amber/red thresholds to highlight resource pressure at a glance.

## Runtime Metrics

Go runtime statistics for the Sentinel server process:

- **Goroutine count** — number of active goroutines.
- **Heap memory** — Go heap allocation in MB.
- **PID** — process identifier.
- **Uptime** — time since the Sentinel process started.

## Auto-Refresh

A toggle in the sidebar enables or disables automatic polling:

- When enabled, metrics refresh every **5 seconds**.
- Can be toggled on or off at any time without page reload.
- State is local to the current browser session.

## UI Features

- Dedicated `/metrics` route with full-page metrics dashboard.
- Two tabs: **System** (host resource gauges) and **Runtime** (Go process stats).
- Sidebar displays host info (hostname, OS/arch, CPU count, Go version), Sentinel info (PID, uptime, last collected timestamp), and the auto-refresh toggle.
- Real-time overview updates via WebSocket (`ops.overview.updated`).
- Help dialog (triggered via the `?` button) explaining the metrics system.

## Data Source

- All metrics are collected locally by the Sentinel backend.
- No external monitoring agents or services are required.
- Host resource metrics are served by the `/api/ops/metrics` endpoint.
- Overview data (host identity, Sentinel process info) is served by the `/api/ops/overview` endpoint.

## Realtime Events

Overview state is kept current via the `/ws/events` WebSocket:

- `ops.overview.updated` — updated overview payload including host and Sentinel process info.

## API Endpoints

- `GET /api/ops/metrics` — host resource metrics (CPU, memory, disk, load averages)
- `GET /api/ops/overview` — host + Sentinel + services summary
