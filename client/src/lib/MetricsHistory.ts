export type MetricsSnapshot = {
  cpuPercent: number
  memPercent: number
  diskPercent: number
  loadAvg1: number
  numGoroutines: number
  goMemAllocMB: number
}

const CAPACITY = 150

export class MetricsHistory {
  private buf: Array<MetricsSnapshot | undefined>
  private tsBuf: Array<number>
  private head = 0
  private size = 0

  constructor(capacity = CAPACITY) {
    this.buf = new Array<MetricsSnapshot | undefined>(capacity)
    this.tsBuf = new Array<number>(capacity)
  }

  push(snapshot: MetricsSnapshot, timestamp = Date.now()): void {
    this.buf[this.head] = snapshot
    this.tsBuf[this.head] = timestamp
    this.head = (this.head + 1) % this.buf.length
    if (this.size < this.buf.length) this.size++
  }

  toArray(): Array<MetricsSnapshot> {
    if (this.size === 0) return []
    const result: Array<MetricsSnapshot> = []
    const start = this.size < this.buf.length ? 0 : this.head
    for (let i = 0; i < this.size; i++) {
      const idx = (start + i) % this.buf.length
      result.push(this.buf[idx]!)
    }
    return result
  }

  field(key: keyof MetricsSnapshot): Array<number> {
    return this.toArray().map((s) => s[key])
  }

  timestamps(): Array<number> {
    if (this.size === 0) return []
    const result: Array<number> = []
    const start = this.size < this.buf.length ? 0 : this.head
    for (let i = 0; i < this.size; i++) {
      const idx = (start + i) % this.buf.length
      result.push(this.tsBuf[idx])
    }
    return result
  }
}
