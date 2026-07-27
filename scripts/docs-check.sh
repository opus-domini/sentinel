#!/usr/bin/env bash
set -euo pipefail

# Validate every local link and image in the public Markdown surface.
#
# Usage:
#   ./scripts/docs-check.sh
#   ./scripts/docs-check.sh --self-test
#
# Environment variables:
#   DOCS_REPO_ROOT  Override the repository root (default: parent of scripts/)
#   DOCS_DIR        Override the docs directory (default: <repo>/docs)
#   DOCS_SIDEBAR    Override the sidebar file (default: $DOCS_DIR/_sidebar.md)

script_path="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"
repo_root="${DOCS_REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
docs_dir="${DOCS_DIR:-${repo_root}/docs}"
sidebar_file="${DOCS_SIDEBAR:-${docs_dir}/_sidebar.md}"
images_dir="${docs_dir}/assets/images"
manifest_file="${images_dir}/showcase-manifest.tsv"

fail() {
	printf 'docs check failed: %s\n' "$*" >&2
	exit 1
}

extract_references() {
	awk '
		function emit_markdown(line, token) {
			while (match(line, /!?\[[^][]*\]\([^()]*\)/)) {
				token = substr(line, RSTART, RLENGTH)
				sub(/^.*\]\(/, "", token)
				sub(/\)$/, "", token)
				sub(/[[:space:]]+["'\''][^"'\'']*["'\''][[:space:]]*$/, "", token)
				print token
				line = substr(line, RSTART + RLENGTH)
			}
		}
		function emit_html(line, token) {
			while (match(line, /<[iI][mM][gG][^>]*[sS][rR][cC][[:space:]]*=[[:space:]]*"[^"]+"/)) {
				token = substr(line, RSTART, RLENGTH)
				sub(/^.*=[[:space:]]*"/, "", token)
				sub(/"$/, "", token)
				print token
				line = substr(line, RSTART + RLENGTH)
			}
			while (match(line, /<[iI][mM][gG][^>]*[sS][rR][cC][[:space:]]*=[[:space:]]*'\''[^'\'']+'\''/)) {
				token = substr(line, RSTART, RLENGTH)
				sub(/^.*=[[:space:]]*'\''/, "", token)
				sub(/'\''$/, "", token)
				print token
				line = substr(line, RSTART + RLENGTH)
			}
		}
		{
			emit_markdown($0)
			emit_html($0)
		}
	' "$1"
}

canonical_target() {
	local source_file="$1"
	local reference="$2"
	local relative_path candidate

	if [[ "$reference" == /* ]]; then
		relative_path="${reference#/}"
		candidate="${docs_dir}/${relative_path}"
	elif [[ "$source_file" == "$repo_root/README.md" ]]; then
		candidate="${repo_root}/${reference}"
	else
		candidate="$(dirname "$source_file")/${reference}"
		if [[ ! -e "$candidate" ]]; then
			# Docsify defaults to root-relative resolution when relativePath is false.
			candidate="${docs_dir}/${reference}"
		fi
	fi

	if [[ "$candidate" == */ ]]; then
		candidate="${candidate}README.md"
	elif [[ -d "$candidate" ]]; then
		candidate="${candidate}/README.md"
	fi

	printf '%s\n' "$candidate"
}

run_check() {
	local checked=0
	local missing=0
	local source_file reference target resolved_file references_file markdown_files_file

	[ -d "$docs_dir" ] || fail "missing docs directory: $docs_dir"
	[ -f "$sidebar_file" ] || fail "missing sidebar file: $sidebar_file"
	[ -f "$repo_root/README.md" ] || fail "missing repository README: $repo_root/README.md"

	references_file="$(mktemp)"
	markdown_files_file="$(mktemp)"
	trap 'rm -f "$references_file" "$markdown_files_file"' RETURN

	printf '%s\n' "$repo_root/README.md" >"$markdown_files_file"
	find "$docs_dir" -type f -name '*.md' -print | LC_ALL=C sort >>"$markdown_files_file"

	while IFS= read -r source_file; do
		while IFS= read -r reference; do
			reference="${reference#<}"
			reference="${reference%>}"
			case "$reference" in
				''|'#'*|http://*|https://*|mailto:*|tel:*|data:*|javascript:*|'/') continue ;;
			esac

			reference="${reference%%#*}"
			reference="${reference%%\?*}"
			[ -n "$reference" ] || continue

			target="$(canonical_target "$source_file" "$reference")"
			checked=$((checked + 1))
			if [[ ! -e "$target" ]]; then
				printf 'docs check failed: %s references missing local path %s\n' \
					"${source_file#"$repo_root"/}" "$reference" >&2
				missing=$((missing + 1))
				continue
			fi

			resolved_file="$(cd "$(dirname "$target")" && pwd)/$(basename "$target")"
			printf '%s\n' "$resolved_file" >>"$references_file"
		done < <(extract_references "$source_file")
	done <"$markdown_files_file"

	if ((missing > 0)); then
		fail "$missing local reference(s) are broken"
	fi

	if [[ -f "$manifest_file" ]]; then
		local manifest_entries_file filename png
		manifest_entries_file="$(mktemp)"
		trap 'rm -f "$references_file" "$markdown_files_file" "$manifest_entries_file"' RETURN

		awk -F '\t' '
			NR == 1 && $1 == "filename" { next }
			/^[[:space:]]*#/ || /^[[:space:]]*$/ { next }
			{ print $1 }
		' "$manifest_file" | LC_ALL=C sort -u >"$manifest_entries_file"

		while IFS= read -r filename; do
			[[ "$filename" != */* && "$filename" == *.png ]] ||
				fail "invalid showcase manifest filename: $filename"
			[[ -f "$images_dir/$filename" ]] ||
				fail "showcase manifest entry is missing: $filename"
		done <"$manifest_entries_file"

		while IFS= read -r png; do
			filename="$(basename "$png")"
			if [[ "$filename" == "logo.png" ]]; then
				continue
			fi
			if grep -Fqx "$png" "$references_file" ||
				grep -Fqx "$filename" "$manifest_entries_file"; then
				continue
			fi
			fail "PNG is neither referenced nor declared in showcase manifest: $filename"
		done < <(find "$images_dir" -maxdepth 1 -type f -name '*.png' -print | LC_ALL=C sort)
	fi

	printf 'docs check passed: %d local reference(s) resolved\n' "$checked"
}

self_test() {
	local test_root output
	test_root="$(mktemp -d)"
	trap 'rm -rf "$test_root"' RETURN

	mkdir -p "$test_root/docs/assets/images" "$test_root/docs/guide"
	printf '# Root\n\n[Docs](docs/README.md)\n' >"$test_root/README.md"
	printf '# Home\n\n[Guide](/guide/start.md)\n![Markdown](assets/images/markdown.png)\n<img src="assets/images/html.png" alt="HTML">\n[External](https://example.com)\n[Anchor](#local)\n' >"$test_root/docs/README.md"
	printf -- '- [Home](/)\n- [Guide](/guide/start.md?mode=docs#first)\n' >"$test_root/docs/_sidebar.md"
	printf '# Start\n\n[Home](../README.md)\n' >"$test_root/docs/guide/start.md"
	: >"$test_root/docs/assets/images/markdown.png"
	: >"$test_root/docs/assets/images/html.png"

	DOCS_REPO_ROOT="$test_root" DOCS_DIR="$test_root/docs" \
		DOCS_SIDEBAR="$test_root/docs/_sidebar.md" "$script_path" --run >/dev/null

	printf '\n[Missing](/guide/missing.md)\n' >>"$test_root/docs/README.md"
	if output="$(DOCS_REPO_ROOT="$test_root" DOCS_DIR="$test_root/docs" \
		DOCS_SIDEBAR="$test_root/docs/_sidebar.md" "$script_path" --run 2>&1)"; then
		fail "self-test expected a missing link failure"
	fi
	grep -Fq 'references missing local path /guide/missing.md' <<<"$output" ||
		fail "self-test missing link failed for the wrong reason"
	sed -i '$d' "$test_root/docs/README.md"

	printf '\n![Missing image](assets/images/missing.png)\n' >>"$test_root/docs/README.md"
	if output="$(DOCS_REPO_ROOT="$test_root" DOCS_DIR="$test_root/docs" \
		DOCS_SIDEBAR="$test_root/docs/_sidebar.md" "$script_path" --run 2>&1)"; then
		fail "self-test expected a missing image failure"
	fi
	grep -Fq 'references missing local path assets/images/missing.png' <<<"$output" ||
		fail "self-test missing image failed for the wrong reason"
	sed -i '$d' "$test_root/docs/README.md"

	printf 'filename\troute_state\tviewport\ttheme\tscenario\tsource_commit\tcaptured_at\nmissing.png\t/\t1x1\tdark\ttest\tdeadbeef\t2026-07-27T00:00:00Z\n' \
		>"$test_root/docs/assets/images/showcase-manifest.tsv"
	if output="$(DOCS_REPO_ROOT="$test_root" DOCS_DIR="$test_root/docs" \
		DOCS_SIDEBAR="$test_root/docs/_sidebar.md" "$script_path" --run 2>&1)"; then
		fail "self-test expected a missing manifest entry failure"
	fi
	grep -Fq 'showcase manifest entry is missing: missing.png' <<<"$output" ||
		fail "self-test missing manifest entry failed for the wrong reason"

	printf 'filename\troute_state\tviewport\ttheme\tscenario\tsource_commit\tcaptured_at\n' \
		>"$test_root/docs/assets/images/showcase-manifest.tsv"
	: >"$test_root/docs/assets/images/orphan.png"
	if output="$(DOCS_REPO_ROOT="$test_root" DOCS_DIR="$test_root/docs" \
		DOCS_SIDEBAR="$test_root/docs/_sidebar.md" "$script_path" --run 2>&1)"; then
		fail "self-test expected an orphan PNG failure"
	fi
	grep -Fq 'PNG is neither referenced nor declared in showcase manifest: orphan.png' <<<"$output" ||
		fail "self-test orphan PNG failed for the wrong reason"

	printf 'docs check self-test passed\n'
}

case "${1:-}" in
	'') run_check ;;
	--run) run_check ;;
	--self-test) self_test ;;
	*) fail "unknown argument: $1" ;;
esac
