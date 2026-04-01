export function formatPageTitle(hostname?: string | null): string {
  return hostname ? `${hostname} - Sentinel` : 'Sentinel'
}
