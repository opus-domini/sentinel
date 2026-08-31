// The settings API reports validation failures as a string list under
// `details.issues`. Every settings draft maps those strings onto its own field
// errors, so the parsing lives here once.
export function readSettingsIssues(details: unknown): Array<string> {
  if (details == null || typeof details !== 'object' || !('issues' in details)) return []
  const issues = (details as { issues?: unknown }).issues
  return Array.isArray(issues)
    ? issues.filter((issue): issue is string => typeof issue === 'string')
    : []
}
