export interface ReconnectState {
  delay: number
  reset: () => void
  next: () => number
}

const MIN_DELAY = 1_200
const MAX_DELAY = 30_000
const GROWTH = 1.7

export function createReconnect(): ReconnectState {
  let delay = MIN_DELAY
  return {
    get delay() {
      return delay
    },
    reset() {
      delay = MIN_DELAY
    },
    next() {
      const current = delay
      delay = Math.min(delay * GROWTH, MAX_DELAY)
      return current
    },
  }
}
