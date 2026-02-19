export function isIOSDevice(): boolean {
  if (typeof navigator === 'undefined') return false
  const ua = navigator.userAgent
  // iPad on iPadOS reports as Macintosh but has touch support
  if (/iPad|iPhone|iPod/.test(ua)) return true
  return /Macintosh/.test(ua) && navigator.maxTouchPoints > 1
}

export function isSafari(): boolean {
  if (typeof navigator === 'undefined') return false
  const ua = navigator.userAgent
  return /Safari/.test(ua) && !/Chrome|Chromium|Edg/.test(ua)
}

export function hapticFeedback(duration = 10): void {
  if ('vibrate' in navigator) navigator.vibrate(duration)
}
