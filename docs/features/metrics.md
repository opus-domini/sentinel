# Metrics

![Desktop metrics](assets/images/desktop-metrics.png)

The Metrics page (`/metrics`) provides real-time system and runtime metrics for the host machine and the Sentinel process. All data is collected locally by Sentinel with no external monitoring agents required.

## System Metrics

Host-level resource metrics collected from the OS:

- **CPU** — usage percentage across all cores, core count, load averages, and load-per-core.
- **Memory** — used, available, total, and utilization percentage.
- **Swap** — used/total bytes and utilization percentage when swap is configured.
- **Disk** — used/free/total bytes, utilization percentage, and inode utilization for the root filesystem.
- **Network** — total RX/TX bytes and live RX/TX rates across non-loopback interfaces.
- **Processes** — process and thread counts.
- **Host uptime** — uptime and boot time.
- **Pressure stall information** — CPU, memory, and I/O PSI `avg10` values on Linux.

Visual indicators use green/amber/red thresholds to highlight resource pressure at a glance.

## Runtime Metrics

Go runtime statistics for the Sentinel server process:

- **Goroutine count** — number of active goroutines.
- **Heap memory** — Go heap allocation and runtime memory in MB.
- **GC** — garbage collection count and latest GC pause duration.
- **PID** — process identifier.
- **Uptime** — time since the Sentinel process started.

## UI Features

- Dedicated `/metrics` route with full-page metrics dashboard.
- Unified command-center dashboard with an always-visible host posture overview.
- Context tabs for saturation, network, and Sentinel runtime metrics, so dense widgets have enough room for labels, details, and trends.
- Metrics uses the full available panel width and keeps help, token, refresh, and connection controls in the page header.
- Metrics are pushed from the server every **2 seconds** over WebSocket.
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
- `ops.metrics.updated` — updated host and runtime metrics.

## API Endpoints

- `GET /api/ops/metrics` — host and Sentinel runtime metrics
- `GET /api/ops/overview` — host + Sentinel + services summary
