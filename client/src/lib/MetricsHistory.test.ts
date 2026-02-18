import { describe, expect, it } from 'vitest'

import { MetricsHistory } from './MetricsHistory'
import type { MetricsSnapshot } from './MetricsHistory'

function snap(cpu: number): MetricsSnapshot {
  return {
    cpuPercent: cpu,
    memPercent: 50,
    diskPercent: 30,
    loadAvg1: 1.0,
    numGoroutines: 10,
    goMemAllocMB: 8,
  }
}

describe('MetricsHistory', () => {
  it('returns empty array when no data pushed', () => {
    const h = new MetricsHistory()
    expect(h.toArray()).toEqual([])
    expect(h.field('cpuPercent')).toEqual([])
    expect(h.timestamps()).toEqual([])
  })

  it('preserves push order (oldest first)', () => {
    const h = new MetricsHistory()
    h.push(snap(10))
    h.push(snap(20))
    h.push(snap(30))
    expect(h.field('cpuPercent')).toEqual([10, 20, 30])
  })

  it('wraps around when exceeding capacity', () => {
    const h = new MetricsHistory(3)
    h.push(snap(1))
    h.push(snap(2))
    h.push(snap(3))
    h.push(snap(4))
    expect(h.field('cpuPercent')).toEqual([2, 3, 4])
  })

  it('wraps around multiple times', () => {
    const h = new MetricsHistory(2)
    h.push(snap(1))
    h.push(snap(2))
    h.push(snap(3))
    h.push(snap(4))
    h.push(snap(5))
    expect(h.field('cpuPercent')).toEqual([4, 5])
  })

  it('extracts field correctly', () => {
    const h = new MetricsHistory()
    h.push({ ...snap(10), memPercent: 60, goMemAllocMB: 4.2 })
    h.push({ ...snap(20), memPercent: 70, goMemAllocMB: 5.1 })
    expect(h.field('memPercent')).toEqual([60, 70])
    expect(h.field('goMemAllocMB')).toEqual([4.2, 5.1])
  })

  it('toArray returns full snapshot objects', () => {
    const h = new MetricsHistory()
    const s = snap(42)
    h.push(s)
    const arr = h.toArray()
    expect(arr).toHaveLength(1)
    expect(arr[0]).toEqual(s)
  })

  it('handles single element', () => {
    const h = new MetricsHistory(1)
    h.push(snap(99))
    expect(h.field('cpuPercent')).toEqual([99])
    h.push(snap(100))
    expect(h.field('cpuPercent')).toEqual([100])
  })

  describe('timestamps', () => {
    it('stores auto-generated timestamps', () => {
      const h = new MetricsHistory()
      const before = Date.now()
      h.push(snap(10))
      const after = Date.now()
      const ts = h.timestamps()
      expect(ts).toHaveLength(1)
      expect(ts[0]).toBeGreaterThanOrEqual(before)
      expect(ts[0]).toBeLessThanOrEqual(after)
    })

    it('stores explicit timestamps', () => {
      const h = new MetricsHistory()
      h.push(snap(10), 1000)
      h.push(snap(20), 2000)
      h.push(snap(30), 3000)
      expect(h.timestamps()).toEqual([1000, 2000, 3000])
    })

    it('preserves order after wrap', () => {
      const h = new MetricsHistory(2)
      h.push(snap(1), 100)
      h.push(snap(2), 200)
      h.push(snap(3), 300)
      expect(h.timestamps()).toEqual([200, 300])
      expect(h.field('cpuPercent')).toEqual([2, 3])
    })

    it('returns empty for no data', () => {
      const h = new MetricsHistory()
      expect(h.timestamps()).toEqual([])
    })
  })
})
