#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output_dir="${1:-${SENTINEL_DOCS_SHOWCASE_OUTPUT:-${repo_root}/.artifacts/docs-showcase}}"
manifest_file="${output_dir}/showcase-manifest.tsv"

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
	printf 'docs showcase check failed: %s\n' "$*" >&2
	exit 1
}

require_cmd() {
	command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

identify_image() {
	if command -v magick >/dev/null 2>&1; then
		magick identify "$@"
	else
		identify "$@"
	fi
}

ocr_contains() {
	local ocr_file="$1"
	local pattern="$2"
	grep -Eiq "$pattern" "$ocr_file"
}

[ -d "$output_dir" ] || fail "missing output directory: $output_dir"
[ -f "$manifest_file" ] || fail "missing showcase manifest: $manifest_file"
require_cmd tesseract
if ! command -v magick >/dev/null 2>&1 && ! command -v identify >/dev/null 2>&1; then
	fail "missing required command: magick or identify"
fi

actual_files=()
while IFS= read -r filename; do
	actual_files+=("$filename")
done < <(
	find "$output_dir" -maxdepth 1 -type f -name '*.png' ! -name 'logo.png' -printf '%f\n' |
		LC_ALL=C sort
)

expected_sorted="$(printf '%s\n' "${expected_files[@]}" | LC_ALL=C sort)"
actual_sorted="$(printf '%s\n' "${actual_files[@]}" | LC_ALL=C sort)"
[[ "$actual_sorted" == "$expected_sorted" ]] ||
	fail "expected exactly the ten canonical PNG filenames"

ocr_dir="$(mktemp -d)"
trap 'rm -rf "$ocr_dir"' EXIT
combined_ocr="$ocr_dir/all.txt"
: >"$combined_ocr"

for filename in "${expected_files[@]}"; do
	image_path="$output_dir/$filename"
	dimensions="$(identify_image -quiet -format '%m %w %h' "$image_path" 2>/dev/null)" ||
		fail "invalid or corrupt PNG: $filename"
	read -r format width height <<<"$dimensions"
	[[ "$format" == "PNG" ]] || fail "$filename is not a PNG"
	if [[ "$filename" == desktop-* ]]; then
		[[ "$width" == "1440" && "$height" == "900" ]] ||
			fail "$filename must be 1440x900, got ${width}x${height}"
	else
		[[ "$width" == "780" && "$height" == "1688" ]] ||
			fail "$filename must be 780x1688, got ${width}x${height}"
	fi

	ocr_file="$ocr_dir/${filename%.png}.txt"
	tesseract "$image_path" stdout --psm 11 2>/dev/null |
		tr '[:upper:]' '[:lower:]' >"$ocr_file" ||
		fail "OCR failed for $filename"
	cat "$ocr_file" >>"$combined_ocr"
	printf '\n' >>"$combined_ocr"
done

while IFS=';' read -r filename pattern label; do
	ocr_file="$ocr_dir/${filename%.png}.txt"
	ocr_contains "$ocr_file" "$pattern" ||
		fail "$filename does not contain expected Orbital Station evidence: $label"
done <<'EOF'
desktop-now-risk.png;needs attention;Now decision queue
desktop-services-diagnosis.png;telemetry;telemetry service
desktop-services-diagnosis.png;current condition;service condition
desktop-metrics-pressure.png;saturation;metrics saturation
desktop-metrics-pressure.png;pressure;pressure evidence
desktop-runbooks-receipt.png;(schema|receipt|execution|succeeded);execution receipt marker
desktop-runbooks-receipt.png;recover telemetry;recovery procedure
desktop-tmux-mission-control.png;orbital;orbital terminal
desktop-tmux-mission-control.png;telemetry;terminal telemetry
desktop-tmux-mission-control.png;deep space network;deep-space network
desktop-now-healthy.png;healthy;healthy posture
desktop-settings-operations.png;operations;Settings Operations
desktop-settings-operations.png;watchtower;Watchtower controls
mobile-now.png;needs attention;mobile Now
mobile-tmux.png;orbital;mobile terminal
mobile-tmux.png;deep space network;mobile deep-space network
mobile-settings-experience.png;experience;mobile Settings Experience
mobile-settings-experience.png;terminal theme;browser theme controls
EOF

deny_patterns=(
	'(^|[^[:alnum:]_])/home/'
	'(^|[^[:alnum:]_])/users/'
	'(^|[^[:alnum:]_])(token|cookie|authorization)([^[:alnum:]_]|$)'
	'(^|[^[:xdigit:]])([[:xdigit:]]{2}:){5}[[:xdigit:]]{2}([^[:xdigit:]]|$)'
	'(^|[^[:alnum:]_])(ssid|bssid|wlp[[:alnum:]_-]*|enp[[:alnum:]_-]*|eth[0-9]+)([^[:alnum:]_]|$)'
	'(^|[^[:alnum:]_])pid[[:space:]:=#-]*[0-9]+([^0-9]|$)'
)
for pattern in "${deny_patterns[@]}"; do
	if ocr_contains "$combined_ocr" "$pattern"; then
		fail "OCR matched a prohibited host or credential pattern"
	fi
done

runtime_user="$(id -un | tr '[:upper:]' '[:lower:]')"
runtime_hostname="$(hostname | tr '[:upper:]' '[:lower:]')"
runtime_workspace="$(printf '%s' "$repo_root" | tr '[:upper:]' '[:lower:]')"
if ((${#runtime_user} >= 4)) &&
	grep -Eiq "(^|[^[:alnum:]_])${runtime_user//./\\.}([^[:alnum:]_]|$)" "$combined_ocr"; then
	fail "OCR matched the runtime user"
fi
if ((${#runtime_hostname} >= 4)) &&
	grep -Eiq "(^|[^[:alnum:]_])${runtime_hostname//./\\.}([^[:alnum:]_]|$)" "$combined_ocr"; then
	fail "OCR matched the runtime hostname"
fi
if grep -Fiq "$runtime_workspace" "$combined_ocr"; then
	fail "OCR matched the runtime workspace"
fi

if perl -ne '
	while (/\b((?:\d{1,3}\.){3}\d{1,3})\b/g) {
		$ip = $1;
		next if $ip =~ /^(?:127\.|192\.0\.2\.|198\.51\.100\.|203\.0\.113\.)/;
		exit 1;
	}
' "$combined_ocr"; then
	:
else
	fail "OCR matched a non-reserved IP address"
fi

manifest_names="$(awk -F '\t' '
	NR == 1 && $1 == "filename" { next }
	/^[[:space:]]*#/ || /^[[:space:]]*$/ { next }
	{ print $1 }
' "$manifest_file" | LC_ALL=C sort)"
[[ "$manifest_names" == "$expected_sorted" ]] ||
	fail "showcase manifest must contain exactly the ten canonical PNG filenames"

printf 'docs showcase check passed: 10 PNGs, dimensions, OCR and provenance verified\n'
