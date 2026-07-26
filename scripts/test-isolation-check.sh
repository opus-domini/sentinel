#!/usr/bin/env bash
set -euo pipefail

mapfile -t test_dirs < <(
  rg --files -g '*_test.go' |
    xargs -r -n1 dirname |
    sort -u
)

missing=()
for dir in "${test_dirs[@]}"; do
  isolation_file="${dir}/host_isolation_test.go"
  if [[ ! -f "${isolation_file}" ]]; then
    missing+=("${dir}")
    continue
  fi
  if [[ "${dir}" == "internal/testenv" ]]; then
    if ! rg -q 'Run\(m' "${isolation_file}"; then
      missing+=("${dir} (guard not invoked)")
    fi
  elif ! rg -q 'testenv\.Run\(m' "${isolation_file}"; then
    missing+=("${dir} (guard not invoked)")
  fi
done

if ((${#missing[@]} > 0)); then
  printf 'Go test packages without host isolation:\n' >&2
  printf '  %s\n' "${missing[@]}" >&2
  exit 1
fi

for config in frontend/vitest.config.ts frontend/vitest.e2e.config.ts; do
  if ! rg -q "setupFiles: \\['\\./src/test/hostIsolation\\.ts'\\]" "${config}"; then
    printf 'Vitest config does not load the host isolation guard: %s\n' "${config}" >&2
    exit 1
  fi
done

printf 'test isolation: ok (%d Go packages, frontend unit and e2e)\n' "${#test_dirs[@]}"
