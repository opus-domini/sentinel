import type { SettingsPatch, SettingsResponse } from '@/api/settings'
import type { ApiError } from '@/hooks/useTmuxApi'

export type AccountsDraft = {
  allowedUsers: Array<string>
  allowRootTarget: boolean
  userSwitchMethod: string
}

export type AccountsDraftKey = keyof AccountsDraft

export type AccountsDraftChange = {
  key: AccountsDraftKey
  configKey: string
  label: string
  before: string
  after: string
}

export type AccountsDraftErrors = Partial<Record<AccountsDraftKey, string>>

const fieldDetails: Record<AccountsDraftKey, { configKey: string; label: string }> = {
  allowedUsers: {
    configKey: 'multi_user.allowed_users',
    label: 'Allowed accounts',
  },
  allowRootTarget: {
    configKey: 'multi_user.allow_root_target',
    label: 'Root targeting',
  },
  userSwitchMethod: {
    configKey: 'multi_user.user_switch_method',
    label: 'Switch method',
  },
}

export function accountsDraftFromSettings(accounts: SettingsResponse['accounts']): AccountsDraft {
  return {
    allowedUsers: [...accounts.allowedUsers.effectiveValue],
    allowRootTarget: accounts.allowRootTarget.effectiveValue,
    userSwitchMethod: accounts.userSwitchMethod.effectiveValue,
  }
}

export function diffAccountsDraft(
  base: AccountsDraft,
  draft: AccountsDraft,
): Array<AccountsDraftChange> {
  return (Object.keys(fieldDetails) as Array<AccountsDraftKey>)
    .filter((key) => !equalDraftValue(base[key], draft[key]))
    .map((key) => ({
      key,
      configKey: fieldDetails[key].configKey,
      label: fieldDetails[key].label,
      before: formatDraftValue(base[key]),
      after: formatDraftValue(draft[key]),
    }))
}

export function validateAccountsDraft(
  draft: AccountsDraft,
  accounts: SettingsResponse['accounts'],
): AccountsDraftErrors {
  const errors: AccountsDraftErrors = {}
  const knownUsers = new Set(accounts.allowedUsers.validation.options.map((option) => option.value))
  const seen = new Set<string>()
  for (const name of draft.allowedUsers) {
    if (seen.has(name)) {
      errors.allowedUsers = `Account ${name} is selected more than once.`
      break
    }
    if (!knownUsers.has(name)) {
      errors.allowedUsers = `Account ${name} is not in the detected OS inventory.`
      break
    }
    seen.add(name)
  }
  if (!draft.allowRootTarget && draft.allowedUsers.includes('root')) {
    errors.allowedUsers = 'Remove root from the allowlist or explicitly enable root targeting.'
  }
  if (
    !accounts.userSwitchMethod.validation.options.some(
      (option) => option.value === draft.userSwitchMethod,
    )
  ) {
    errors.userSwitchMethod = 'Choose sudo or systemd-run as provided by Sentinel.'
  }
  return errors
}

export function accountsPatchFromChanges(
  draft: AccountsDraft,
  changes: Array<AccountsDraftChange>,
): SettingsPatch {
  const changed = new Set(changes.map((change) => change.key))
  return {
    accounts: {
      ...(changed.has('allowedUsers') ? { allowedUsers: [...draft.allowedUsers].sort() } : {}),
      ...(changed.has('allowRootTarget') ? { allowRootTarget: draft.allowRootTarget } : {}),
      ...(changed.has('userSwitchMethod') ? { userSwitchMethod: draft.userSwitchMethod } : {}),
    },
  }
}

export function accountsErrorsFromAPI(error: ApiError): AccountsDraftErrors {
  const details = error.details as { issues?: unknown } | undefined
  const issues = Array.isArray(details?.issues)
    ? details.issues.filter((issue): issue is string => typeof issue === 'string')
    : []
  const joined = issues.join(' ')
  return {
    ...(joined.includes('multi_user.allowed_users') ? { allowedUsers: joined } : {}),
    ...(joined.includes('multi_user.allow_root_target') ? { allowRootTarget: joined } : {}),
    ...(joined.includes('multi_user.user_switch_method') ? { userSwitchMethod: joined } : {}),
  }
}

function equalDraftValue(
  left: AccountsDraft[AccountsDraftKey],
  right: AccountsDraft[AccountsDraftKey],
): boolean {
  if (Array.isArray(left) && Array.isArray(right)) {
    return [...left].sort().join('\u0000') === [...right].sort().join('\u0000')
  }
  return left === right
}

function formatDraftValue(value: AccountsDraft[AccountsDraftKey]): string {
  if (Array.isArray(value)) {
    return value.length === 0 ? 'All detected accounts' : [...value].sort().join(', ')
  }
  if (typeof value === 'boolean') return value ? 'Enabled' : 'Disabled'
  return value
}
