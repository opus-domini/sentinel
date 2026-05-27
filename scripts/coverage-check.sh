#!/usr/bin/env sh
set -eu

profile="${1:-coverage.txt}"
min="${COVERAGE_MIN:-80}"

if [ ! -f "$profile" ]; then
	echo "coverage profile not found: $profile" >&2
	exit 1
fi

total="$(go tool cover -func="$profile" | awk '/^total:/ { sub(/%$/, "", $3); print $3 }')"
if [ -z "$total" ]; then
	echo "unable to read total coverage from $profile" >&2
	exit 1
fi

awk -v total="$total" -v min="$min" 'BEGIN {
	if (total + 0 < min + 0) {
		printf "coverage %.1f%% is below required %.1f%%\n", total, min
		exit 1
	}
	printf "coverage %.1f%% meets required %.1f%%\n", total, min
}'
