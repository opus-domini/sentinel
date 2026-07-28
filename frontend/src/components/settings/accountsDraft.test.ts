import { describe, expect, it } from 'vitest'

import {
  accountsDraftFromSettings,
  accountsErrorsFromAPI,
  accountsPatchFromChanges,
  diffAccountsDraft,
  validateAccountsDraft,
} from './accountsDraft'
import { ApiError } from '@/hooks/useTmuxApi'
import { createSettingsSnapshot } from '@/test/settings'

describe('accounts draft', () => {
  it('builds the smallest typed patch with stable account order', () => {
    const accounts = createSettingsSnapshot().settings.accounts
    const base = accountsDraftFromSettings(accounts)
    const draft = {
      allowedUsers: ['hugo', 'deploy'],
      allowRootTarget: false,
      userSwitchMethod: 'sudo',
    }
    const changes = diffAccountsDraft(base, draft)

    expect(changes.map((change) => change.configKey)).toEqual([
      'multi_user.allowed_users',
      'multi_user.user_switch_method',
    ])
    expect(accountsPatchFromChanges(draft, changes)).toEqual({
      accounts: {
        allowedUsers: ['deploy', 'hugo'],
        userSwitchMethod: 'sudo',
      },
    })
  })

  it('rejects duplicate, unknown, root-gate, and invalid method drafts', () => {
    const accounts = createSettingsSnapshot().settings.accounts
    expect(
      validateAccountsDraft(
        {
          allowedUsers: ['deploy', 'deploy'],
          allowRootTarget: false,
          userSwitchMethod: 'systemd-run',
        },
        accounts,
      ).allowedUsers,
    ).toContain('more than once')
    expect(
      validateAccountsDraft(
        {
          allowedUsers: ['ghost'],
          allowRootTarget: false,
          userSwitchMethod: 'systemd-run',
        },
        accounts,
      ).allowedUsers,
    ).toContain('not in the detected')
    expect(
      validateAccountsDraft(
        {
          allowedUsers: ['root'],
          allowRootTarget: false,
          userSwitchMethod: 'invalid',
        },
        accounts,
      ),
    ).toEqual({
      allowedUsers: 'Remove root from the allowlist or explicitly enable root targeting.',
      userSwitchMethod: 'Choose sudo or systemd-run as provided by Sentinel.',
    })
  })

  it('maps canonical backend issues to account fields', () => {
    const error = new ApiError('invalid', 422, 'CONFIG_INVALID', {
      issues: ['multi_user.allowed_users contains unknown account ghost'],
    })
    expect(accountsErrorsFromAPI(error)).toEqual({
      allowedUsers: 'multi_user.allowed_users contains unknown account ghost',
    })
  })
})
