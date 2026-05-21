import { formatInstalledAppName } from './appBrand'

export function formatPageTitle(hostname?: string | null): string {
  return formatInstalledAppName(hostname)
}
