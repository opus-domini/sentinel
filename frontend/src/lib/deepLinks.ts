export type ServicesSearch = {
  service?: string
  panel?: 'status' | 'logs'
}

export type RunbooksSearch = {
  runbook?: string
  job?: string
}

export type TmuxSearch = {
  session?: string
}

function optionalSearchText(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const trimmed = value.trim()
  return trimmed === '' ? undefined : trimmed
}

export function parseServicesSearch(search: Record<string, unknown>): ServicesSearch {
  const service = optionalSearchText(search.service)
  const panel = search.panel === 'status' || search.panel === 'logs' ? search.panel : undefined
  if (service == null || panel == null) return {}
  return { service, panel }
}

export function parseRunbooksSearch(search: Record<string, unknown>): RunbooksSearch {
  const runbook = optionalSearchText(search.runbook)
  if (runbook == null) return {}
  return {
    runbook,
    job: optionalSearchText(search.job),
  }
}

export function parseTmuxSearch(search: Record<string, unknown>): TmuxSearch {
  const session = optionalSearchText(search.session)
  return session == null ? {} : { session }
}
