export type MetricsSnapshot = {
  cpuPercent: number
  memPercent: number
  diskPercent: number
  diskInodesPercent: number
  swapPercent: number
  loadAvg1: number
  loadPerCPU: number
  netRxBytes: number
  netTxBytes: number
  processCount: number
  threadCount: number
  cpuPressureAvg10: number
  memPressureAvg10: number
  ioPressureAvg10: number
  numGoroutines: number
  goMemAllocMB: number
  goMemSysMB: number
  goLastGcPauseMs: number
  sensorTemperatures: Record<string, number>
  sensorFanRPM: Record<string, number>
  sensorPowerWatts: Record<string, number>
}

export type NumericMetricKey = {
  [Key in keyof MetricsSnapshot]: MetricsSnapshot[Key] extends number ? Key : never
}[keyof MetricsSnapshot]
