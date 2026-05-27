export function formatBytes(bytes: number | undefined | null): string {
  if (!Number.isFinite(bytes) || !bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = bytes
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  const precision = size >= 100 || index === 0 ? 0 : 1
  return `${size.toFixed(precision)} ${units[index]}`
}

export function formatPercentValue(value: number | undefined | null, digits = 1): string {
  if (!Number.isFinite(value) || value == null || value < 0) return '-'
  return `${value.toFixed(digits)}%`
}
