import { spawn } from 'node:child_process'
import { mkdirSync, mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import process from 'node:process'

const disableWebstorageFlag = '--no-experimental-webstorage'
const sandbox = mkdtempSync(join(tmpdir(), 'sentinel-frontend-test-'))
const sandboxDirs = {
  HOME: join(sandbox, 'home'),
  SENTINEL_DATA_DIR: join(sandbox, 'sentinel'),
  TMPDIR: join(sandbox, 'tmp'),
  XDG_CACHE_HOME: join(sandbox, 'xdg', 'cache'),
  XDG_CONFIG_HOME: join(sandbox, 'xdg', 'config'),
  XDG_DATA_HOME: join(sandbox, 'xdg', 'data'),
  XDG_RUNTIME_DIR: join(sandbox, 'xdg', 'runtime'),
  XDG_STATE_HOME: join(sandbox, 'xdg', 'state'),
}

for (const dir of Object.values(sandboxDirs)) {
  mkdirSync(dir, { recursive: true, mode: 0o700 })
}

function buildNodeOptions(existing) {
  const normalized = (existing ?? '').trim()
  if (normalized === '') {
    return disableWebstorageFlag
  }
  const options = normalized.split(/\s+/)
  if (options.includes(disableWebstorageFlag)) {
    return normalized
  }
  return `${disableWebstorageFlag} ${normalized}`
}

const child = spawn(
  process.execPath,
  ['./node_modules/vitest/vitest.mjs', ...process.argv.slice(2)],
  {
    cwd: process.cwd(),
    env: {
      ...process.env,
      ...sandboxDirs,
      ALL_PROXY: 'http://127.0.0.1:1',
      HTTP_PROXY: 'http://127.0.0.1:1',
      HTTPS_PROXY: 'http://127.0.0.1:1',
      NO_PROXY: '127.0.0.1,localhost,::1',
      NODE_OPTIONS: buildNodeOptions(process.env.NODE_OPTIONS),
      SENTINEL_CONFIG: join(sandboxDirs.SENTINEL_DATA_DIR, 'config.toml'),
      SENTINEL_STORAGE_PATH: join(sandboxDirs.SENTINEL_DATA_DIR, 'sentinel.db'),
    },
    stdio: 'inherit',
  },
)

function cleanup() {
  rmSync(sandbox, { recursive: true, force: true })
}

child.on('exit', (code, signal) => {
  cleanup()
  if (signal !== null) {
    process.kill(process.pid, signal)
    return
  }
  process.exit(code ?? 1)
})

child.on('error', (error) => {
  cleanup()
  console.error(error)
  process.exit(1)
})
