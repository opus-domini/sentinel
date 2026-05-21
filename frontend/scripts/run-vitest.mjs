import { spawn } from 'node:child_process'
import process from 'node:process'

const disableWebstorageFlag = '--no-experimental-webstorage'

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
      NODE_OPTIONS: buildNodeOptions(process.env.NODE_OPTIONS),
    },
    stdio: 'inherit',
  },
)

child.on('exit', (code, signal) => {
  if (signal !== null) {
    process.kill(process.pid, signal)
    return
  }
  process.exit(code ?? 1)
})
