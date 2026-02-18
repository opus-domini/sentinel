#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
sidebar_file="${repo_root}/docs/_sidebar.md"

if [[ ! -f "${sidebar_file}" ]]; then
  echo "docs check failed: missing ${sidebar_file}" >&2
  exit 1
fi

missing=0

while IFS= read -r path; do
  trimmed="${path#/}"
  if [[ -z "${trimmed}" ]]; then
    continue
  fi
  target="${repo_root}/docs/${trimmed}"
  if [[ ! -f "${target}" ]]; then
    echo "docs check failed: sidebar link points to missing file ${target}" >&2
    missing=1
  fi
done < <(
  sed -n 's/.*](\([^)]*\)).*/\1/p' "${sidebar_file}" \
    | grep '^/' \
    | grep -v '^/$'
)

if [[ "${missing}" -ne 0 ]]; then
  exit 1
fi

echo "docs check passed"
