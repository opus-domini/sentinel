#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixture_script="$repo_root/scripts/docs-showcase-fixture.js"
check_script="$repo_root/scripts/docs-showcase-check.sh"
output_input="${SENTINEL_DOCS_SHOWCASE_OUTPUT:-}"

expected_files=(
	desktop-now-risk.png
	desktop-services-diagnosis.png
	desktop-metrics-pressure.png
	desktop-runbooks-receipt.png
	desktop-tmux-mission-control.png
	desktop-now-healthy.png
	desktop-settings-operations.png
	mobile-now.png
	mobile-tmux.png
	mobile-settings-experience.png
)

fail() {
	printf 'docs showcase capture failed: %s\n' "$*" >&2
	exit 1
}

require_cmd() {
	command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

[[ -n "$output_input" ]] ||
	fail "SENTINEL_DOCS_SHOWCASE_OUTPUT must name an explicit output directory"

require_cmd agent-browser
require_cmd curl
require_cmd go
require_cmd npm
require_cmd python3
require_cmd tesseract
require_cmd tmux
if ! command -v magick >/dev/null 2>&1 && ! command -v identify >/dev/null 2>&1; then
	fail "missing required command: magick or identify"
fi
[[ -f "$fixture_script" ]] || fail "missing browser fixture: $fixture_script"
[[ -x "$check_script" ]] || fail "showcase check is not executable: $check_script"

output_dir="$(realpath -m "$output_input")"
repo_home="$(realpath -m "${HOME:?}")"
case "$output_dir" in
	/|"$repo_root"|"$repo_home") fail "refusing unsafe output directory: $output_dir" ;;
esac

work_dir="$(mktemp -d)"
staging_dir="$work_dir/staging"
data_dir="$work_dir/data"
tmux_dir="$work_dir/tmux"
decoy_tmux_dir="$work_dir/tmux-decoy"
logs_dir="$work_dir/logs"
mkdir -p "$staging_dir" "$data_dir" "$tmux_dir" "$decoy_tmux_dir" "$logs_dir"

browser_session="sentinel-docs-showcase-$$-$(date +%s)"
server_pid=""
base_url=""
tmux_isolated=0
decoy_tmux_isolated=0
decoy_signature=""
guidance_pane=""

showcase_tmux() {
	env -u TMUX -u TMUX_PANE TMUX_TMPDIR="$tmux_dir" tmux -f /dev/null "$@"
}

decoy_tmux() {
	env -u TMUX -u TMUX_PANE TMUX_TMPDIR="$decoy_tmux_dir" tmux -f /dev/null "$@"
}

browser() {
	agent-browser --session "$browser_session" "$@"
}

cleanup() {
	local status=$?
	browser close >/dev/null 2>&1 || true
	if [[ -n "$server_pid" ]]; then
		kill "$server_pid" >/dev/null 2>&1 || true
		wait "$server_pid" >/dev/null 2>&1 || true
	fi
	if ((tmux_isolated == 1)); then
		for session_name in flight-control telemetry maintenance; do
			if showcase_tmux has-session -t "=$session_name" 2>/dev/null; then
				showcase_tmux kill-session -t "=$session_name" >/dev/null 2>&1 || true
			fi
		done
	fi
	if ((decoy_tmux_isolated == 1)) &&
		decoy_tmux has-session -t '=docs-showcase-decoy' 2>/dev/null; then
		decoy_tmux kill-session -t '=docs-showcase-decoy' >/dev/null 2>&1 || true
	fi
	rm -rf "$work_dir"
	if ((status != 0)); then
		printf 'docs showcase capture failed; temporary browser, daemon and tmux were cleaned up\n' >&2
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
	for _ in $(seq 1 120); do
		if curl -fsS "$url/api/meta" >/dev/null 2>&1; then
			return 0
		fi
		sleep 0.25
	done
	fail "isolated daemon did not become ready"
}

send_tmux_command() {
	local target="$1"
	local command="$2"
	showcase_tmux send-keys -t "$target" -l -- "$command"
	showcase_tmux send-keys -t "$target" Enter
}

send_tmux_scene() {
	local target="$1"
	shift
	local command="clear; printf '%s\\n'"
	local line quoted
	for line in "$@"; do
		printf -v quoted '%q' "$line"
		command+=" $quoted"
	done
	send_tmux_command "$target" "$command"
}

create_decoy_scenario() {
	local socket_path expected_socket decoy_ready
	decoy_tmux new-session -d -s docs-showcase-decoy -n hold -c /tmp -x 80 -y 24 \
		"printf '%s\\n' 'DECOY // KEEP THIS SESSION'; exec sleep 3600"
	socket_path="$(decoy_tmux display-message -p '#{socket_path}')"
	expected_socket="$decoy_tmux_dir/tmux-$(id -u)/default"
	if [[ "$socket_path" != "$expected_socket" ]]; then
		fail "decoy tmux isolation check failed before any cleanup was armed"
	fi
	decoy_tmux_isolated=1
	decoy_signature="$(
		decoy_tmux display-message -p -t 'docs-showcase-decoy:hold' \
			'#{session_name}|#{session_id}|#{pane_pid}'
	)"
	decoy_ready=0
	for _ in $(seq 1 40); do
		if decoy_tmux capture-pane -p -t 'docs-showcase-decoy:hold' |
			grep -Fq 'DECOY // KEEP THIS SESSION'; then
			decoy_ready=1
			break
		fi
		sleep 0.05
	done
	((decoy_ready == 1)) || fail "decoy tmux content was not initialized"
	printf 'docs showcase: decoy socket isolated at %s\n' "$socket_path"
}

create_tmux_scenario() {
	local downlink_pane telemetry_pane socket_path expected_socket
	showcase_tmux new-session -d -s flight-control -n mission-control -c /tmp -x 160 -y 44 \
		"env PS1='ORBITAL> ' bash --noprofile --norc"
	socket_path="$(showcase_tmux display-message -p '#{socket_path}')"
	expected_socket="$tmux_dir/tmux-$(id -u)/default"
	if [[ "$socket_path" != "$expected_socket" ]]; then
		fail "tmux isolation check failed before any cleanup was armed"
	fi
	tmux_isolated=1
	downlink_pane="$(showcase_tmux display-message -p -t flight-control:mission-control '#{pane_id}')"
	guidance_pane="$(
		showcase_tmux split-window -h -t flight-control:mission-control -c /tmp -P -F '#{pane_id}' \
			"env PS1='ORBITAL> ' bash --noprofile --norc"
	)"
	showcase_tmux select-layout -t flight-control:mission-control even-horizontal
	showcase_tmux select-pane -t "$downlink_pane" -T downlink
	showcase_tmux select-pane -t "$guidance_pane" -T guidance

	send_tmux_scene "$downlink_pane" \
		$'\033[1;36mDEEP SPACE NETWORK // RELAY 07\033[0m' \
		$'\033[2;34m----------------------------------------\033[0m' \
		$'\033[2;34m            .              *\033[0m' \
		$'\033[2;34m     *             \033[1;35m__|__\033[2;34m         .\033[0m' \
		$'\033[1;35m          .-------/ 07 \\-------.\033[1;33m   ))) )))\033[0m' \
		$'\033[1;35m                  \\___/\033[1;33m             GS-7\033[0m' \
		$'\033[1;35m                    |\033[0m' \
		$'\033[1;36m            .-~~~~~~~~~~~-.      \033[1;33mAOS 18:32\033[0m' \
		$'\033[1;36m          .\'    ORBITAL    \'.\033[0m' \
		$'\033[1;36m         /      STATION      \\\033[0m' \
		$'\033[1;36m         \'._               _.\'\033[0m' \
		$'\033[1;36m            \'-------------\'\033[0m' \
		'' \
		$'\033[2;37mCARRIER  \033[1;32mLOCKED\033[2;37m      SNR      \033[1;33m+18.4 dB\033[0m' \
		$'\033[2;37mFRAMES   \033[1;32m1842/1842\033[2;37m   LATENCY  \033[1;33m42 ms\033[0m' \
		$'\033[2;37mRELAY    \033[1;32mRECOVERED\033[2;37m   VECTOR   \033[1;36mSTABLE\033[0m' \
		$'\033[2;34m----------------------------------------\033[0m' \
		$'\033[1;32m[OK] RECOVERY RECEIPT VERIFIED\033[0m' \
		$'\033[1;36mORBITAL>\033[0m monitoring deep-space downlink'
	send_tmux_scene "$guidance_pane" \
		$'\033[1;35mGROUND ARRAY // GS-7 TRACKING\033[0m' \
		$'\033[2;34m--------------------------------\033[0m' \
		$'\033[2;34m          .              *\033[0m' \
		$'\033[1;33m      ))) )))\033[2;37m       signal acquired\033[0m' \
		$'\033[1;33m             \\\033[0m' \
		$'\033[1;33m              \\\033[1;36m    .-.\033[0m' \
		$'\033[1;33m               \\\033[1;36m .\'   \'.\033[0m' \
		$'\033[1;36m                /       \\\033[0m' \
		$'\033[1;36m               /_________\\\033[0m' \
		$'\033[1;36m                    ||\033[0m' \
		$'\033[1;36m             _______||_______\033[0m' \
		'' \
		$'\033[2;37mAZIMUTH    \033[1;36m214.8 deg\033[0m' \
		$'\033[2;37mELEVATION  \033[1;36m36.2 deg\033[0m' \
		$'\033[2;37mDOPPLER    \033[1;33m-2.1 kHz\033[0m' \
		$'\033[2;37mUPLINK     \033[1;33mSTANDBY\033[0m' \
		$'\033[2;37mDOWNLINK   \033[1;32mACQUIRED\033[0m' \
		$'\033[2;37mBEACON     \033[1;32m8/8\033[0m' \
		'' \
		$'\033[1;32m[##################\033[2;37m..\033[1;32m] 92%\033[0m' \
		$'\033[1;35mGUIDANCE>\033[0m handshake confirmed'

	showcase_tmux new-session -d -s telemetry -n stream -c /tmp -x 120 -y 36 \
		"env PS1='ORBITAL> ' bash --noprofile --norc"
	telemetry_pane="$(showcase_tmux display-message -p -t telemetry:stream '#{pane_id}')"
	showcase_tmux select-pane -t "$telemetry_pane" -T telemetry
	send_tmux_command telemetry:stream \
		"clear; printf '%s\\n' 'ORBITAL TELEMETRY STREAM' '------------------------' 'carrier         LOCKED' 'frames verified 1842' 'signal quality  NOMINAL' 'relay state     RECOVERED' '' 'next pass       18:32 UTC' '' 'ORBITAL> downlink current'"

	showcase_tmux new-session -d -s maintenance -n checklist -c /tmp -x 120 -y 36 \
		"env PS1='ORBITAL> ' bash --noprofile --norc"
	send_tmux_command maintenance:checklist \
		"clear; printf '%s\\n' 'ORBITAL MAINTENANCE' '[ok] antenna path' '[ok] telemetry relay' '[ok] payload link'"

	showcase_tmux select-pane -t "$downlink_pane"
	printf 'docs showcase: showcase socket isolated at %s\n' "$socket_path"
}

verify_decoy_unchanged() {
	local current_signature
	decoy_tmux has-session -t '=docs-showcase-decoy' 2>/dev/null ||
		fail "decoy tmux session disappeared during capture"
	current_signature="$(
		decoy_tmux display-message -p -t 'docs-showcase-decoy:hold' \
			'#{session_name}|#{session_id}|#{pane_pid}'
	)"
	[[ "$current_signature" == "$decoy_signature" ]] ||
		fail "decoy tmux identity changed during capture"
	decoy_tmux capture-pane -p -t 'docs-showcase-decoy:hold' |
		grep -Fq 'DECOY // KEEP THIS SESSION' ||
		fail "decoy tmux content changed during capture"
	if showcase_tmux has-session -t '=docs-showcase-decoy' 2>/dev/null; then
		fail "showcase socket unexpectedly contains the decoy session"
	fi
	for session_name in flight-control telemetry maintenance; do
		if decoy_tmux has-session -t "=$session_name" 2>/dev/null; then
			fail "decoy socket unexpectedly contains showcase session $session_name"
		fi
	done
	printf 'docs showcase: decoy name, ID, pane PID and content remained unchanged\n'
}

wait_for_page() {
	local text="$1"
	if ! browser wait --text "$text" >/dev/null; then
		printf 'docs showcase: expected text did not render: %s\n' "$text" >&2
		browser get url >&2 || true
		browser snapshot -c -d 4 >&2 || true
		browser errors >&2 || true
		fail "browser route did not reach its expected state"
	fi
	browser wait --fn \
		"!document.body.innerText.includes('Now is unavailable') && !document.body.innerText.includes('failed to load')" \
		>/dev/null
}

open_page() {
	local route="$1"
	local text="$2"
	browser open "${base_url}${route}" >/dev/null
	wait_for_page "$text"
}

wait_for_condition() {
	local description="$1"
	local condition="$2"
	if ! browser wait --fn "$condition" >/dev/null; then
		printf 'docs showcase: browser condition timed out: %s\n' "$description" >&2
		browser get url >&2 || true
		browser snapshot -c -d 5 >&2 || true
		browser errors >&2 || true
		fail "browser route did not reach its expected condition"
	fi
}

capture_page() {
	local route="$1"
	local text="$2"
	local filename="$3"
	open_page "$route" "$text"
	browser screenshot "$staging_dir/$filename" >/dev/null
}

validate_browser_diagnostics() {
	local errors_file="$logs_dir/browser-errors.json"
	local console_file="$logs_dir/browser-console.json"
	browser --json errors >"$errors_file"
	browser --json console >"$console_file"
	python3 - "$errors_file" "$console_file" <<'PY'
import json
import sys


def payload(path):
    with open(path, encoding="utf-8") as handle:
        value = json.load(handle)
    if isinstance(value, dict) and "data" in value:
        return value["data"]
    return value


errors = payload(sys.argv[1])
if isinstance(errors, dict):
    errors = errors.get("errors", [])
if errors:
    raise SystemExit("browser page errors are not empty")

console = payload(sys.argv[2])
if isinstance(console, dict):
    console = console.get("messages", console.get("console", []))
if not isinstance(console, list):
    console = []
bad = [
    entry
    for entry in console
    if isinstance(entry, dict)
    and str(entry.get("type", entry.get("level", ""))).lower() in {"error", "assert"}
]
if bad:
    raise SystemExit("browser console contains error entries")
PY
}

write_manifest() {
	local source_commit captured_at
	source_commit="$(git -C "$repo_root" rev-parse HEAD)"
	captured_at="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
	{
		printf 'filename\troute_state\tviewport\ttheme\tscenario\tsource_commit\tcaptured_at\n'
		printf 'desktop-now-risk.png\t/ (at risk)\t1440x900@1x\tdark\tOrbital Station\t%s\t%s\n' "$source_commit" "$captured_at"
		printf 'desktop-services-diagnosis.png\t/services?service=telemetry-relay&panel=status\t1440x900@1x\tdark\tOrbital Station\t%s\t%s\n' "$source_commit" "$captured_at"
		printf 'desktop-metrics-pressure.png\t/metrics?signal=cpuPressure\t1440x900@1x\tdark\tOrbital Station\t%s\t%s\n' "$source_commit" "$captured_at"
		printf 'desktop-runbooks-receipt.png\t/runbooks?job=job-orbital-042\t1440x900@1x\tdark\tOrbital Station\t%s\t%s\n' "$source_commit" "$captured_at"
		printf 'desktop-tmux-mission-control.png\t/tmux?session=flight-control\t1440x900@1x\tdark\tOrbital Station\t%s\t%s\n' "$source_commit" "$captured_at"
		printf 'desktop-now-healthy.png\t/ (healthy)\t1440x900@1x\tdark\tOrbital Station\t%s\t%s\n' "$source_commit" "$captured_at"
		printf 'desktop-settings-operations.png\t/settings/operations\t1440x900@1x\tdark\tOrbital Station\t%s\t%s\n' "$source_commit" "$captured_at"
		printf 'mobile-now.png\t/ (at risk)\t390x844@2x\tdark\tOrbital Station\t%s\t%s\n' "$source_commit" "$captured_at"
		printf 'mobile-tmux.png\t/tmux?session=flight-control\t390x844@2x\tdark\tOrbital Station\t%s\t%s\n' "$source_commit" "$captured_at"
		printf 'mobile-settings-experience.png\t/settings/experience\t390x844@2x\tdark\tOrbital Station\t%s\t%s\n' "$source_commit" "$captured_at"
	} >"$staging_dir/showcase-manifest.tsv"
}

publish_staging() {
	mkdir -p "$output_dir"
	for filename in "${expected_files[@]}"; do
		rm -f "$output_dir/$filename"
	done
	rm -f "$output_dir/showcase-manifest.tsv"
	for filename in "${expected_files[@]}"; do
		cp "$staging_dir/$filename" "$output_dir/$filename"
	done
	cp "$staging_dir/showcase-manifest.tsv" "$output_dir/showcase-manifest.tsv"
}

printf 'docs showcase: building real frontend\n'
npm --prefix "$repo_root/frontend" run build

printf 'docs showcase: creating independent decoy tmux\n'
create_decoy_scenario

printf 'docs showcase: creating isolated Orbital Station tmux\n'
create_tmux_scenario

port="$(find_free_port)"
base_url="http://127.0.0.1:${port}"
printf 'docs showcase: starting isolated daemon at loopback port %s\n' "$port"
(
	cd "$repo_root"
	env -u TMUX -u TMUX_PANE \
		TMUX_TMPDIR="$tmux_dir" \
		SENTINEL_SERVER_HOST=127.0.0.1 \
		SENTINEL_SERVER_PORT="$port" \
		SENTINEL_CONFIG="$data_dir/config.toml" \
		SENTINEL_DATA_DIR="$data_dir" \
		SENTINEL_STORAGE_PATH="$data_dir/sentinel.db" \
		SENTINEL_LOG_PATH="$logs_dir/sentinel.log" \
		go run ./cmd/sentinel daemon
) >"$logs_dir/daemon.stdout.log" 2>&1 &
server_pid=$!
wait_for_server "$base_url"

tmux_sessions_file="$logs_dir/tmux-sessions.json"
curl -fsS "$base_url/api/tmux/sessions" >"$tmux_sessions_file"
python3 - "$tmux_sessions_file" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    payload = json.load(handle)
data = payload.get("data", payload)
names = sorted(item.get("name") for item in data.get("sessions", []))
expected = ["flight-control", "maintenance", "telemetry"]
if names != expected:
    raise SystemExit("isolated daemon did not expose exactly the synthetic tmux sessions")
PY

printf 'docs showcase: capturing desktop workflow\n'
agent-browser --session "$browser_session" --init-script "$fixture_script" open "$base_url/" \
	>/dev/null
browser set media dark reduced-motion >/dev/null
browser set viewport 1440 900 1 >/dev/null
browser eval "localStorage.setItem('sentinel_docs_showcase_scene', 'risk')" >/dev/null
capture_page / 'Needs attention' desktop-now-risk.png
capture_page '/services?service=telemetry-relay&panel=status' 'Current condition' \
	desktop-services-diagnosis.png
open_page '/metrics?signal=cpuPressure&focusAt=2026-07-27T18%3A00%3A00Z' 'SATURATION'
wait_for_condition 'Metrics contains at least four populated sparklines' \
	"document.querySelectorAll('svg[aria-label=\"Metric trend\"]').length >= 4"
wait_for_condition 'CPU pressure card received deep-link focus' \
	"document.activeElement?.id === 'metric-signal-cpuPressure'"
browser eval \
	"(() => { const viewport = document.querySelector('[data-slot=\"scroll-area-viewport\"]'); if (viewport instanceof HTMLElement) viewport.scrollTop = Math.max(0, viewport.scrollTop - 16) })()" \
	>/dev/null
wait_for_condition 'Metrics context tabs are fully framed below the header' \
	"(() => { const header = document.querySelector('main > header'); const tabs = document.querySelector('[role=\"tablist\"][aria-label=\"Metric contexts\"]'); return header instanceof HTMLElement && tabs instanceof HTMLElement && tabs.getBoundingClientRect().top >= header.getBoundingClientRect().bottom + 8 })()"
browser screenshot "$staging_dir/desktop-metrics-pressure.png" >/dev/null
browser open "${base_url}/runbooks?job=job-orbital-042" >/dev/null
wait_for_condition 'focused immutable execution receipt exists' \
	"document.querySelector('[aria-label^=\"Execution receipt\"]') !== null"
browser eval \
	"document.querySelector('[aria-label^=\"Execution receipt\"]')?.scrollIntoView({ block: 'start' })" \
	>/dev/null
wait_for_condition 'immutable execution receipt copy is rendered' \
	"document.querySelector('[aria-label^=\"Execution receipt\"]')?.textContent?.includes('Immutable execution receipt') === true"
browser mouse move 1400 880 >/dev/null
browser eval "document.activeElement instanceof HTMLElement && document.activeElement.blur()" \
	>/dev/null
browser press Escape >/dev/null
browser wait 500 >/dev/null
browser screenshot "$staging_dir/desktop-runbooks-receipt.png" >/dev/null
open_page '/tmux?session=flight-control' 'flight-control'
browser wait '.xterm' >/dev/null
wait_for_condition 'desktop terminal contains deep-space network output' \
	"document.querySelector('.xterm-rows')?.textContent?.includes('DEEP SPACE NETWORK') === true"
browser screenshot "$staging_dir/desktop-tmux-mission-control.png" >/dev/null

capture_page '/settings/operations' 'Control collection cadence' \
	desktop-settings-operations.png

browser eval "localStorage.setItem('sentinel_docs_showcase_scene', 'healthy')" >/dev/null
capture_page / 'Healthy' desktop-now-healthy.png

printf 'docs showcase: capturing mobile workflow\n'
[[ -n "$guidance_pane" ]] || fail "isolated guidance pane was not recorded"
showcase_tmux kill-pane -t "$guidance_pane"
browser set viewport 390 844 2 >/dev/null
browser eval "localStorage.setItem('sentinel_docs_showcase_scene', 'risk')" >/dev/null
capture_page / 'Needs attention' mobile-now.png
open_page '/tmux?session=flight-control' 'flight-control'
wait_for_condition 'mobile terminal attached to flight-control' \
	"document.querySelector('.xterm') !== null"
wait_for_condition 'mobile terminal contains rendered output' \
	"document.querySelector('.xterm-rows')?.textContent?.includes('DEEP SPACE NETWORK') === true"
browser screenshot "$staging_dir/mobile-tmux.png" >/dev/null
capture_page '/settings/experience' 'Terminal theme' mobile-settings-experience.png

browser network requests --filter /api/tmux >"$logs_dir/tmux-network.txt"
grep -Fq '/api/tmux/' "$logs_dir/tmux-network.txt" ||
	fail "Tmux did not use the isolated daemon API"
validate_browser_diagnostics
verify_decoy_unchanged
write_manifest

printf 'docs showcase: validating staging before publication\n'
"$check_script" "$staging_dir"
publish_staging
printf 'docs showcase capture passed: published 10 validated PNGs to %s\n' "$output_dir"
