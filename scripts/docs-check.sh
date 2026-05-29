#!/usr/bin/env bash
set -euo pipefail

# Validate the docsify sidebar against files in docs/.
#
# Usage:
#   ./scripts/docs-check.sh
#
# Environment variables:
#   DOCS_DIR      Override the docs directory (default: <repo>/docs)
#   DOCS_SIDEBAR  Override the sidebar file (default: $DOCS_DIR/_sidebar.md)

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
docs_dir="${DOCS_DIR:-${repo_root}/docs}"
sidebar_file="${DOCS_SIDEBAR:-${docs_dir}/_sidebar.md}"

fail() {
	printf 'docs check failed: %s\n' "$*" >&2
	exit 1
}

[ -d "$docs_dir" ] || fail "missing docs directory: $docs_dir"
[ -f "$sidebar_file" ] || fail "missing sidebar file: $sidebar_file"

checked=0
missing=0

while IFS= read -r link; do
	# Skip external links, pure anchors and docsify's root link.
	case "$link" in
		''|'#'*|http://*|https://*|mailto:*|tel:*|'/') continue ;;
	esac

	# Remove optional query strings and anchors before resolving the file path.
	link="${link%%#*}"
	link="${link%%\?*}"
	[ -n "$link" ] || continue
	[ "$link" = "/" ] && continue

	if [[ "$link" == /* ]]; then
		relative_path="${link#/}"
	else
		relative_path="$link"
	fi

	if [[ "$relative_path" == */ ]]; then
		target="${docs_dir}/${relative_path}README.md"
	else
		target="${docs_dir}/${relative_path}"
	fi

	checked=$((checked + 1))
	if [ ! -f "$target" ]; then
		printf 'docs check failed: sidebar link %s points to missing file %s\n' "$link" "$target" >&2
		missing=$((missing + 1))
	fi
done < <(
	awk '
		{
			line = $0
			while (match(line, /\[[^][]+\]\([^()]+\)/)) {
				link = substr(line, RSTART, RLENGTH)
				sub(/^.*\]\(/, "", link)
				sub(/\)$/, "", link)
				print link
				line = substr(line, RSTART + RLENGTH)
			}
		}
	' "$sidebar_file"
)

if [ "$missing" -ne 0 ]; then
	fail "$missing sidebar link(s) are broken"
fi

printf 'docs check passed: %d sidebar link(s) resolved\n' "$checked"
