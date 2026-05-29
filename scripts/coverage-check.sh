#!/usr/bin/env sh
set -eu

# Enforce a minimum total percentage for a Go coverage profile.
#
# Usage:
#   COVERAGE_MIN=80 ./scripts/coverage-check.sh [coverage-profile]
#
# The Makefile passes the profile path explicitly. For manual use, the script
# falls back to $COVERAGE_PROFILE and then to coverage.txt.

# Force decimal parsing/formatting to be identical on every machine.
export LC_ALL=C

profile="${1:-${COVERAGE_PROFILE:-coverage.txt}}"
min="${COVERAGE_MIN:-80}"

fail() {
	printf 'coverage check failed: %s\n' "$*" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

need awk
need go

[ -f "$profile" ] || fail "coverage profile not found: $profile"

total="$(go tool cover -func="$profile" | awk '/^total:/ { sub(/%$/, "", $3); print $3; found = 1 } END { if (!found) exit 1 }')" \
	|| fail "unable to read total coverage from $profile"
[ -n "$total" ] || fail "unable to read total coverage from $profile"

awk -v total="$total" -v min="$min" -v profile="$profile" 'BEGIN {
	if ((total + 0) < (min + 0)) {
		printf "coverage check failed: %.1f%% < %.1f%% (%s)\n", total, min, profile > "/dev/stderr"
		exit 1
	}
	printf "coverage check passed: %.1f%% >= %.1f%% (%s)\n", total, min, profile
}'
