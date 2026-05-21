#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
artifacts_dir="${SENTINEL_SMOKE_ARTIFACTS_DIR:-$(mktemp -d)}"
keep_artifacts="${SENTINEL_SMOKE_KEEP_ARTIFACTS:-0}"
data_dir="$(mktemp -d)"
server_pid=""
session_name="sentinel-smoke-$(date +%s)-$$"
browser_session="sentinel-smoke-${session_name}"
port=""
initial_line_count="${SENTINEL_SMOKE_INITIAL_LINES:-1200}"
live_line_count="${SENTINEL_SMOKE_LIVE_LINES:-3000}"
desktop_viewport_width="${SENTINEL_SMOKE_DESKTOP_WIDTH:-1920}"
desktop_viewport_height="${SENTINEL_SMOKE_DESKTOP_HEIGHT:-1200}"
desktop_device_scale="${SENTINEL_SMOKE_DESKTOP_SCALE:-1}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 127
  fi
}

cleanup() {
  local status=$?

  case "$session_name" in
    sentinel-smoke-*) tmux kill-session -t "$session_name" 2>/dev/null || true ;;
  esac

  agent-browser --session "$browser_session" close --all >/dev/null 2>&1 || true

  if [[ -n "$server_pid" ]]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" >/dev/null 2>&1 || true
  fi

  rm -rf "$data_dir"

  if [[ "$status" -eq 0 && "$keep_artifacts" != "1" ]]; then
    rm -rf "$artifacts_dir"
  else
    echo "smoke artifacts: $artifacts_dir" >&2
  fi
}
trap cleanup EXIT

find_free_port() {
  python3 - <<'PY'
import socket

with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
}

wait_for_server() {
  local url="$1"
  for _ in $(seq 1 80); do
    if curl -fsS "$url/api/meta" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.25
  done
  echo "server did not become ready at $url" >&2
  return 1
}

validate_smoke_config() {
  if ! [[ "$initial_line_count" =~ ^[0-9]+$ && "$live_line_count" =~ ^[0-9]+$ ]]; then
    echo "SENTINEL_SMOKE_INITIAL_LINES and SENTINEL_SMOKE_LIVE_LINES must be positive integers" >&2
    exit 2
  fi
  if ((initial_line_count < 1 || live_line_count <= initial_line_count)); then
    echo "SENTINEL_SMOKE_LIVE_LINES must be greater than SENTINEL_SMOKE_INITIAL_LINES" >&2
    exit 2
  fi
  if ! [[ "$desktop_viewport_width" =~ ^[0-9]+$ && "$desktop_viewport_height" =~ ^[0-9]+$ && "$desktop_device_scale" =~ ^[0-9]+$ ]]; then
    echo "SENTINEL_SMOKE_DESKTOP_WIDTH, SENTINEL_SMOKE_DESKTOP_HEIGHT, and SENTINEL_SMOKE_DESKTOP_SCALE must be positive integers" >&2
    exit 2
  fi
  if ((desktop_viewport_width < 1 || desktop_viewport_height < 1 || desktop_device_scale < 1)); then
    echo "SENTINEL_SMOKE_DESKTOP_WIDTH, SENTINEL_SMOKE_DESKTOP_HEIGHT, and SENTINEL_SMOKE_DESKTOP_SCALE must be positive integers" >&2
    exit 2
  fi
}

create_session_payload() {
  python3 - "$session_name" <<'PY'
import json
import sys

print(json.dumps({"name": sys.argv[1], "cwd": "/tmp"}))
PY
}

browser_storage_script() {
  python3 - "$session_name" <<'PY'
import json
import sys

value = json.dumps({
    "openTabs": [sys.argv[1]],
    "activeSession": sys.argv[1],
    "activeEpoch": 1,
})
print("sessionStorage.setItem('sentinel_tabs', " + json.dumps(value) + "); location.reload();")
PY
}

output_marker() {
  printf '%s_%04d' "$1" "$2"
}

send_terminal_output() {
  local prefix="$1"
  local start="$2"
  local end="$3"
  local emoji_pack="\\U0001f469\\u200d\\U0001f4bb\\U0001f3f3\\ufe0f\\u200d\\U0001f308\\U0001f1e7\\U0001f1f7\\U0001f1fa\\U0001f1f8\\U0001f680\\U0001f525\\U0001f600\\U0001f602\\U0001f60a\\U0001f970\\U0001f60e\\U0001f92f\\u2728\\u26a1\\U0001f308\\U0001f48e\\U0001f9ea\\U0001f6e0\\ufe0f\\u2705\\u274c"

  tmux send-keys -t "$session_name" \
    "python3 -c 'emoji=\"${emoji_pack}\"; [print(\"${prefix}_%04d \\u2699 \\u03bb \\ue0b6\\ue0b4 \\u250c\\u2500\\u252c\\u2500\\u2510 \\u2502 \\U0001f60a \\u2502 \\u25e2\\u25e3 \\u2591\\u2592\\u2593 \\u2714 \\u2718 \\u2192 \\u2190 EMOJI_STRESS %s\" % (i, emoji)) for i in range(${start}, ${end} + 1)]'" \
    C-m
}

send_pixel_probe() {
  tmux send-keys -t "$session_name" \
    "python3 -c 'print(\"\\033[2J\\033[H\", end=\"\"); colors=[(255,210,40,\"PIXEL_PROBE_YELLOW\"),(40,220,160,\"PIXEL_PROBE_GREEN\"),(245,80,140,\"PIXEL_PROBE_PINK\")]; fill=\"\\u2588\\u2593\\u2592\\u2591\"*4; [print(\"\\033[38;2;%d;%d;%dm%s %s\\033[0m\" % (r,g,b,label,fill)) for r,g,b,label in colors]; print(\"\\033[30mPIXEL_PROBE_LOW_CONTRAST \" + \"\\u2588\"*16 + \"\\033[0m\")'" \
    C-m
}

validate_eval_json() {
  local path="$1"
  shift
  python3 - "$path" "$@" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as fh:
    data = json.load(fh)

failures = []
for key in sys.argv[2:]:
    if data.get(key) is not True:
        value = data.get(key)
        failures.append(f"{key}={value!r}")

if failures:
    print("terminal smoke validation failed: " + ", ".join(failures), file=sys.stderr)
    print(json.dumps(data, indent=2, ensure_ascii=False), file=sys.stderr)
    sys.exit(1)

metrics = data.get("metrics")
if isinstance(metrics, dict):
    metric_failures = []
    for key in ("deltaSyncErrors", "deltaOverflowCount", "fallbackRefreshCount", "wsReconnects"):
        value = metrics.get(key)
        if value != 0:
            metric_failures.append(f"{key}={value!r}")

    max_values = {"wsOpenCount": 3, "wsCloseCount": 2}
    for key, limit in max_values.items():
        value = metrics.get(key)
        if not isinstance(value, int) or value > limit:
            metric_failures.append(f"{key}={value!r} > {limit}")

    min_values = {"wsMessages": 1, "deltaSyncCount": 1}
    for key, minimum in min_values.items():
        value = metrics.get(key)
        if not isinstance(value, int) or value < minimum:
            metric_failures.append(f"{key}={value!r} < {minimum}")

    if metric_failures:
        print("terminal smoke metrics failed: " + ", ".join(metric_failures), file=sys.stderr)
        print(json.dumps(data, indent=2, ensure_ascii=False), file=sys.stderr)
        sys.exit(1)

terminal_metrics = data.get("terminalMetrics")
if isinstance(terminal_metrics, dict):
    terminal_metric_failures = []
    if terminal_metrics.get("renderer") != "dom":
        terminal_metric_failures.append(f"renderer={terminal_metrics.get('renderer')!r} != 'dom'")

    for key in ("writeRecoveries", "writeBacklogRecoveries", "writeStallRecoveries"):
        value = terminal_metrics.get(key)
        if value != 0:
            terminal_metric_failures.append(f"{key}={value!r}")

    min_values = {"writeBatchCount": 1, "writeBytes": 1024, "writeMaxQueueBytes": 1}
    for key, minimum in min_values.items():
        value = terminal_metrics.get(key)
        if not isinstance(value, int) or value < minimum:
            terminal_metric_failures.append(f"{key}={value!r} < {minimum}")

    max_queue_bytes = 16 * 1024 * 1024
    value = terminal_metrics.get("writeMaxQueueBytes")
    if not isinstance(value, int) or value > max_queue_bytes:
        terminal_metric_failures.append(f"writeMaxQueueBytes={value!r} > {max_queue_bytes}")

    if terminal_metric_failures:
        print("terminal smoke write metrics failed: " + ", ".join(terminal_metric_failures), file=sys.stderr)
        print(json.dumps(data, indent=2, ensure_ascii=False), file=sys.stderr)
        sys.exit(1)

print(json.dumps(data, indent=2, ensure_ascii=False))
PY
}

capture_output_eval() {
  local path="$1"
  local marker="$2"

  agent-browser --session "$browser_session" eval \
    "(() => { const screenText = document.querySelector('.xterm-screen')?.innerText || ''; const gear = String.fromCodePoint(0x2699); const powerline = String.fromCodePoint(0xe0b6); const emoji = String.fromCodePoint(0x1f60a); const rocket = String.fromCodePoint(0x1f680); const fire = String.fromCodePoint(0x1f525); const brazil = String.fromCodePoint(0x1f1e7, 0x1f1f7); const technologist = String.fromCodePoint(0x1f469) + String.fromCodePoint(0x200d) + String.fromCodePoint(0x1f4bb); const marker = '${marker}'; const terminalMetrics = window.__SENTINEL_TERMINAL_METRICS || {}; const terminalCanvasCount = document.querySelectorAll('.xterm canvas').length; return { terminalPresent: Boolean(document.querySelector('.xterm')), screenPresent: Boolean(document.querySelector('.xterm-screen')), canvasCount: document.querySelectorAll('canvas').length, terminalCanvasCount, usesDomRenderer: terminalMetrics.renderer === 'dom' && terminalCanvasCount === 0, hasOutputMarker: screenText.includes(marker), hasGear: screenText.includes(gear), hasPowerline: screenText.includes(powerline), hasEmoji: screenText.includes(emoji), hasEmojiStress: screenText.includes('EMOJI_STRESS') && screenText.includes(rocket) && screenText.includes(fire) && screenText.includes(brazil) && screenText.includes(technologist), metrics: window.__SENTINEL_TMUX_METRICS || {}, terminalMetrics, bodyTail: screenText.slice(-600) }; })()" \
    >"$path"
}

wait_for_output_eval() {
  local path="$1"
  local marker="$2"

  for _ in $(seq 1 80); do
    capture_output_eval "$path" "$marker"
    if python3 - "$path" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as fh:
    data = json.load(fh)

required = (
    "terminalPresent",
    "screenPresent",
    "hasOutputMarker",
    "hasGear",
    "hasPowerline",
    "hasEmoji",
    "hasEmojiStress",
)
raise SystemExit(0 if all(data.get(key) is True for key in required) else 1)
PY
    then
      return 0
    fi
    sleep 0.25
  done

  return 1
}

wait_for_pixel_probe_eval() {
  local path="$1"
  local eval_script="(() => { const body = document.body.innerText; const terminalMetrics = window.__SENTINEL_TERMINAL_METRICS || {}; const terminalCanvasCount = document.querySelectorAll('.xterm canvas').length; return { terminalPresent: Boolean(document.querySelector('.xterm')), screenPresent: Boolean(document.querySelector('.xterm-screen')), usesDomRenderer: terminalMetrics.renderer === 'dom' && terminalCanvasCount === 0, terminalMetrics, terminalCanvasCount, hasPixelProbeYellow: body.includes('PIXEL_PROBE_YELLOW'), hasPixelProbeGreen: body.includes('PIXEL_PROBE_GREEN'), hasPixelProbePink: body.includes('PIXEL_PROBE_PINK'), hasLowContrastProbe: body.includes('PIXEL_PROBE_LOW_CONTRAST') }; })()"

  for _ in $(seq 1 40); do
    agent-browser --session "$browser_session" eval "$eval_script" >"$path"
    if python3 - "$path" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as fh:
    data = json.load(fh)

required = (
    "terminalPresent",
    "screenPresent",
    "hasPixelProbePink",
    "hasLowContrastProbe",
)
raise SystemExit(0 if all(data.get(key) is True for key in required) else 1)
PY
    then
      return 0
    fi
    sleep 0.25
  done

  return 1
}

capture_terminal_pixels() {
  local label="$1"
  local screenshot_path="$artifacts_dir/${label}-terminal.png"
  local rect_path="$artifacts_dir/${label}-terminal-rect.json"

  agent-browser --session "$browser_session" eval \
    "(() => { const screen = document.querySelector('.xterm-screen'); if (!screen) return { present: false }; const rect = screen.getBoundingClientRect(); return { present: true, left: rect.left, top: rect.top, width: rect.width, height: rect.height, viewportWidth: window.innerWidth, viewportHeight: window.innerHeight, devicePixelRatio: window.devicePixelRatio || 1 }; })()" \
    >"$rect_path"
  agent-browser --session "$browser_session" screenshot "$screenshot_path" >/dev/null
  validate_terminal_pixels "$screenshot_path" "$rect_path" "$label"
}

validate_terminal_pixels() {
  local screenshot_path="$1"
  local rect_path="$2"
  local label="$3"

  python3 - "$screenshot_path" "$rect_path" "$label" <<'PY'
import collections
import json
import math
import struct
import sys
import zlib


def paeth(a, b, c):
    p = a + b - c
    pa = abs(p - a)
    pb = abs(p - b)
    pc = abs(p - c)
    if pa <= pb and pa <= pc:
        return a
    if pb <= pc:
        return b
    return c


def parse_png(path):
    with open(path, "rb") as fh:
        data = fh.read()
    if not data.startswith(b"\x89PNG\r\n\x1a\n"):
        raise ValueError("not a PNG")

    pos = 8
    width = height = bit_depth = color_type = None
    palette = None
    idat = bytearray()
    while pos < len(data):
        if pos + 8 > len(data):
            raise ValueError("truncated PNG chunk")
        length = struct.unpack(">I", data[pos : pos + 4])[0]
        chunk_type = data[pos + 4 : pos + 8]
        chunk_data = data[pos + 8 : pos + 8 + length]
        pos += 12 + length
        if chunk_type == b"IHDR":
            width, height, bit_depth, color_type, compression, flt, interlace = struct.unpack(
                ">IIBBBBB", chunk_data
            )
            if bit_depth != 8 or compression != 0 or flt != 0 or interlace != 0:
                raise ValueError("unsupported PNG encoding")
        elif chunk_type == b"PLTE":
            palette = [
                tuple(chunk_data[i : i + 3]) + (255,)
                for i in range(0, len(chunk_data), 3)
            ]
        elif chunk_type == b"IDAT":
            idat.extend(chunk_data)
        elif chunk_type == b"IEND":
            break

    channels_by_type = {0: 1, 2: 3, 3: 1, 4: 2, 6: 4}
    if width is None or height is None or color_type not in channels_by_type:
        raise ValueError("unsupported PNG color type")

    channels = channels_by_type[color_type]
    stride = width * channels
    raw = zlib.decompress(bytes(idat))
    rows = []
    src = 0
    prev = bytearray(stride)
    for _ in range(height):
        filter_type = raw[src]
        src += 1
        row = bytearray(raw[src : src + stride])
        src += stride
        for i in range(stride):
            left = row[i - channels] if i >= channels else 0
            up = prev[i]
            up_left = prev[i - channels] if i >= channels else 0
            if filter_type == 1:
                row[i] = (row[i] + left) & 0xFF
            elif filter_type == 2:
                row[i] = (row[i] + up) & 0xFF
            elif filter_type == 3:
                row[i] = (row[i] + ((left + up) // 2)) & 0xFF
            elif filter_type == 4:
                row[i] = (row[i] + paeth(left, up, up_left)) & 0xFF
            elif filter_type != 0:
                raise ValueError("unsupported PNG filter")
        rows.append(row)
        prev = row

    def pixel_at(x, y):
        row = rows[y]
        i = x * channels
        if color_type == 0:
            v = row[i]
            return v, v, v, 255
        if color_type == 2:
            return row[i], row[i + 1], row[i + 2], 255
        if color_type == 3:
            if palette is None:
                raise ValueError("indexed PNG without palette")
            return palette[row[i]]
        if color_type == 4:
            v = row[i]
            return v, v, v, row[i + 1]
        return row[i], row[i + 1], row[i + 2], row[i + 3]

    return width, height, pixel_at


image_path, rect_path, label = sys.argv[1:4]
with open(rect_path, "r", encoding="utf-8") as fh:
    rect = json.load(fh)
if rect.get("present") is not True:
    raise SystemExit("terminal visual validation failed: xterm screen missing")

width, height, pixel_at = parse_png(image_path)
viewport_width = max(float(rect.get("viewportWidth") or width), 1.0)
viewport_height = max(float(rect.get("viewportHeight") or height), 1.0)
scale_x = width / viewport_width
scale_y = height / viewport_height
left = max(0, min(width - 1, int(math.floor(float(rect.get("left", 0)) * scale_x))))
top = max(0, min(height - 1, int(math.floor(float(rect.get("top", 0)) * scale_y))))
right = max(left + 1, min(width, int(math.ceil((float(rect.get("left", 0)) + float(rect.get("width", 0))) * scale_x))))
bottom = max(top + 1, min(height, int(math.ceil((float(rect.get("top", 0)) + float(rect.get("height", 0))) * scale_y))))

total = (right - left) * (bottom - top)
if total < 10_000:
    raise SystemExit(f"terminal visual validation failed: crop too small for {label}: {total}")

hist = collections.Counter()
sampled = []
for y in range(top, bottom):
    for x in range(left, right):
        r, g, b, a = pixel_at(x, y)
        if a < 16:
            continue
        hist[(r // 8, g // 8, b // 8)] += 1
        sampled.append((r, g, b))

if not sampled:
    raise SystemExit(f"terminal visual validation failed: no opaque pixels for {label}")

dominant_bucket, _ = hist.most_common(1)[0]
dominant = tuple(channel * 8 + 4 for channel in dominant_bucket)
foreground = 0
bright = 0
colorful = 0
expected_color_hits = {"yellow": 0, "green": 0, "pink": 0}
expected_colors = {
    "yellow": (255, 210, 40),
    "green": (40, 220, 160),
    "pink": (245, 80, 140),
}
for r, g, b in sampled:
    distance = abs(r - dominant[0]) + abs(g - dominant[1]) + abs(b - dominant[2])
    if distance > 36:
        foreground += 1
    if max(r, g, b) > 150 and distance > 36:
        bright += 1
    if max(r, g, b) - min(r, g, b) > 45 and max(r, g, b) > 100 and distance > 36:
        colorful += 1
    for name, (tr, tg, tb) in expected_colors.items():
        if abs(r - tr) + abs(g - tg) + abs(b - tb) < 110:
            expected_color_hits[name] += 1

foreground_ratio = foreground / max(len(sampled), 1)
stats = {
    "label": label,
    "image": [width, height],
    "crop": [left, top, right, bottom],
    "foreground": foreground,
    "foregroundRatio": round(foreground_ratio, 5),
    "bright": bright,
    "colorful": colorful,
    "expectedColors": expected_color_hits,
    "uniqueBuckets": len(hist),
}

missing_expected_colors = {
    name: count for name, count in expected_color_hits.items() if count < 200
}
if (
    foreground < 900
    or foreground_ratio < 0.002
    or bright < 200
    or colorful < 40
    or len(hist) < 8
    or missing_expected_colors
):
    print("terminal visual validation failed:", json.dumps(stats, sort_keys=True), file=sys.stderr)
    raise SystemExit(1)

print("terminal visual validation:", json.dumps(stats, sort_keys=True))
PY
}

require_cmd agent-browser
require_cmd curl
require_cmd go
require_cmd npm
require_cmd python3
require_cmd tmux

validate_smoke_config
mkdir -p "$artifacts_dir"
port="$(find_free_port)"
base_url="http://127.0.0.1:${port}"

echo "building frontend assets"
npm --prefix "$root_dir/frontend" run build

echo "starting sentinel at $base_url"
(
  cd "$root_dir"
  SENTINEL_LISTEN="127.0.0.1:${port}" \
    SENTINEL_DATA_DIR="$data_dir" \
    go run ./cmd/sentinel serve
) >"$artifacts_dir/server.log" 2>&1 &
server_pid=$!
wait_for_server "$base_url"

echo "creating tmux session $session_name"
curl -fsS -X POST "$base_url/api/tmux/sessions" \
  -H 'Content-Type: application/json' \
  --data "$(create_session_payload)" \
  >"$artifacts_dir/create-session.json"

send_terminal_output "SOAK" 1 "$initial_line_count"

echo "opening browser"
agent-browser --session "$browser_session" open "$base_url/tmux" >/dev/null
agent-browser --session "$browser_session" set viewport "$desktop_viewport_width" "$desktop_viewport_height" "$desktop_device_scale" >/dev/null
agent-browser --session "$browser_session" wait 1000 >/dev/null
agent-browser --session "$browser_session" eval "$(browser_storage_script)" >/dev/null
agent-browser --session "$browser_session" wait .xterm >/dev/null

wait_for_output_eval "$artifacts_dir/initial-eval.json" "$(output_marker SOAK "$initial_line_count")" || true
validate_eval_json "$artifacts_dir/initial-eval.json" \
  terminalPresent screenPresent usesDomRenderer hasOutputMarker hasGear hasPowerline hasEmoji hasEmojiStress

send_terminal_output "LIVE" "$((initial_line_count + 1))" "$live_line_count"
wait_for_output_eval "$artifacts_dir/live-eval.json" "$(output_marker LIVE "$live_line_count")" || true
validate_eval_json "$artifacts_dir/live-eval.json" \
  terminalPresent screenPresent usesDomRenderer hasOutputMarker hasGear hasPowerline hasEmoji hasEmojiStress

send_pixel_probe
wait_for_pixel_probe_eval "$artifacts_dir/pixel-probe-eval.json" || true
validate_eval_json "$artifacts_dir/pixel-probe-eval.json" \
  terminalPresent screenPresent usesDomRenderer hasPixelProbePink hasLowContrastProbe
capture_terminal_pixels desktop

agent-browser --session "$browser_session" set viewport 390 844 2 >/dev/null
agent-browser --session "$browser_session" wait 500 >/dev/null
agent-browser --session "$browser_session" eval \
  "(() => { const body = document.body.innerText; const terminalMetrics = window.__SENTINEL_TERMINAL_METRICS || {}; const terminalCanvasCount = document.querySelectorAll('.xterm canvas').length; return { terminalPresent: Boolean(document.querySelector('.xterm')), screenPresent: Boolean(document.querySelector('.xterm-screen')), usesDomRenderer: terminalMetrics.renderer === 'dom' && terminalCanvasCount === 0, hasMobileControls: Boolean(document.querySelector('[aria-label=\"Toggle keyboard\"]')), hasFooterSize: body.includes('cols ') && body.includes(' rows '), terminalMetrics, terminalCanvasCount, viewportWidth: window.innerWidth, viewportHeight: window.innerHeight }; })()" \
  >"$artifacts_dir/mobile-eval.json"
validate_eval_json "$artifacts_dir/mobile-eval.json" \
  terminalPresent screenPresent usesDomRenderer hasMobileControls hasFooterSize
capture_terminal_pixels mobile

agent-browser --session "$browser_session" set viewport "$desktop_viewport_width" "$desktop_viewport_height" "$desktop_device_scale" >/dev/null
agent-browser --session "$browser_session" wait 500 >/dev/null
agent-browser --session "$browser_session" eval \
  "(() => { const terminalMetrics = window.__SENTINEL_TERMINAL_METRICS || {}; const terminalCanvasCount = document.querySelectorAll('.xterm canvas').length; return { terminalPresent: Boolean(document.querySelector('.xterm')), screenPresent: Boolean(document.querySelector('.xterm-screen')), usesDomRenderer: terminalMetrics.renderer === 'dom' && terminalCanvasCount === 0, terminalMetrics, terminalCanvasCount, viewportWidth: window.innerWidth, viewportHeight: window.innerHeight }; })()" \
  >"$artifacts_dir/desktop-restored-eval.json"
validate_eval_json "$artifacts_dir/desktop-restored-eval.json" \
  terminalPresent screenPresent usesDomRenderer
capture_terminal_pixels desktop-restored

agent-browser --session "$browser_session" errors >"$artifacts_dir/page-errors.txt"
if [[ -s "$artifacts_dir/page-errors.txt" ]]; then
  echo "browser page errors detected" >&2
  cat "$artifacts_dir/page-errors.txt" >&2
  exit 1
fi

agent-browser --session "$browser_session" console >"$artifacts_dir/console.log"

echo "terminal smoke passed"
