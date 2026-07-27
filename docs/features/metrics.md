# Metrics

<img src="assets/images/desktop-metrics-pressure.png" alt="Metrics focused on critical CPU pressure in the fictitious Orbital Station workload, with live saturation samples and no causal attribution" />

The Orbital Station values are fictitious showcase samples captured from the
real Metrics UI; Sentinel has no satellite-specific behavior.

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
The backend also evaluates one canonical host posture, shared by Metrics and
Now:

- `normal/ok` when every available key signal is below its warning threshold;
- `pressure/warning` or `pressure/critical` when at least one key signal crosses
  a threshold;
- `unavailable/unknown` when no key signal can be evaluated.

The posture evaluates CPU (80/90%), memory (80/90%), root disk (85/95%),
inodes (80/90%), swap when configured (20/60%), and CPU/memory/I/O PSI avg10
(2/10). Missing individual signals are ignored; an entirely unevaluable sample
is never reported as nominal.

## Runtime Metrics

Go runtime statistics for the Sentinel server process:

- **Goroutine count** — number of active goroutines.
- **Heap memory** — Go heap allocation and runtime memory in MB.
- **GC** — garbage collection count and latest GC pause duration.
- **PID** — process identifier.
- **Uptime** — time since the Sentinel process started.

## UI Features

- Dedicated `/metrics` route with full-page metrics dashboard.
- Typed handoffs use
  `/metrics?signal=<canonical-signal>&focusAt=<RFC3339>`. The route selects
  Saturation, scrolls to, focuses, and highlights the requested card. Canonical
  signals are `cpu`, `memory`, `rootDisk`, `inodes`, `swap`, `cpuPressure`,
  `memoryPressure`, and `ioPressure`.
- `focusAt` identifies when the owner handoff evidence was observed. The page
  displays it as context while explicitly treating charts as current/live
  samples, not as a persisted historical sample for that instant.
- Owner-focused dashboard with an always-visible host posture overview.
- Context tabs for saturation, network, and Sentinel runtime metrics, so dense widgets have enough room for labels, details, and trends.
- Metrics uses the full available panel width and keeps help, token, refresh, and connection controls in the page header.
- Metrics are pushed from the server every **2 seconds** over WebSocket.
- Real-time overview updates via WebSocket (`ops.overview.updated`).
- Help dialog (triggered via the `?` button) explaining the metrics system.

Metrics owns pressure diagnosis in the operational loop. A Now handoff carries
the canonical signal name and the attention item's `observedAt`; Metrics focuses
that live card while clearly stating that it does not retain a historical
sample for the handoff instant. It never infers a responsible Service or
process from host-level pressure.

## In-Memory Chart History

The trend charts use a browser-side ring buffer with capacity for 150 metric
samples. The page seeds it from the initial API response and appends subsequent
`ops.metrics.updated` events; older entries are overwritten when the buffer is
full.

This buffer belongs to the mounted Metrics page. Reloading the browser or
remounting the route starts the charts again from the next current sample.
Sentinel does not persist this visual history, query older chart samples, or
turn correlation between live signals into a causal diagnosis.

## Data Source

- All metrics are collected locally by the Sentinel backend.
- No external monitoring agents or services are required.
- Host resource metrics and their canonical posture are served together by the
  `/api/ops/metrics` endpoint as `{ metrics, posture }`.
- Overview data (host identity, Sentinel process info) is served by the `/api/ops/overview` endpoint.

## Temporal Posture

Raw metric values remain samples for visual history. The posture is stateful:

- root disk and inode capacity enter warning at 85%/80% and critical at
  95%/90% immediately; they recover below 83%/78%;
- CPU and memory enter warning at 80% and critical at 90%; swap enters at
  20%/60%. These volatile signals must remain above the threshold for ten
  seconds, including warning-to-critical escalation;
- CPU, memory, and swap recover after ten seconds below five percentage points
  under their warning threshold;
- CPU, memory, and I/O PSI `avg10` enter immediately at 2/10 and recover after
  ten seconds below 1.5.

Every active signal includes `since`, the start of the sustained threshold
condition. The aggregate posture includes `observedAt`. A short CPU spike
therefore remains visible in the raw sample without immediately becoming an
operational pressure signal.

## Realtime Events

Overview state is kept current via the `/ws/events` WebSocket:

- `ops.overview.updated` — updated overview payload including host and Sentinel process info.
- `ops.metrics.updated` — every raw host/runtime sample with its canonical
  posture, used by the live Metrics view.
- `ops.posture.updated` — emitted only when aggregate state/severity or the
  active `name+severity` signal set changes.

## API Endpoints

- `GET /api/ops/metrics` — `{ metrics, posture }` for host and Sentinel runtime
- `GET /api/ops/overview` — host + Sentinel + services summary
