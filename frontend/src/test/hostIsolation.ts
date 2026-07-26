import { beforeEach } from 'vitest'

const blockedFetch: typeof fetch = async (input) => {
  throw new Error(`Unexpected network access in test: ${String(input)}`)
}

class BlockedWebSocket {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSING = 2
  static readonly CLOSED = 3

  constructor(url: string | URL) {
    throw new Error(`Unexpected WebSocket access in test: ${String(url)}`)
  }
}

function installNetworkGuards() {
  globalThis.fetch = blockedFetch
  globalThis.WebSocket = BlockedWebSocket as unknown as typeof WebSocket
}

installNetworkGuards()
beforeEach(installNetworkGuards)
