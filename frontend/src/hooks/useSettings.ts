import { useCallback } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { getSettings, patchSettings } from '@/api/settings'
import type { SettingsPatch, SettingsSnapshot } from '@/api/settings'
import { ApiError } from '@/hooks/useTmuxApi'

export const SETTINGS_QUERY_KEY = ['settings'] as const

type UseSettingsOptions = {
  enabled?: boolean
}

export function useSettings(options: UseSettingsOptions = {}) {
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: SETTINGS_QUERY_KEY,
    queryFn: getSettings,
    enabled: options.enabled ?? true,
  })
  const mutation = useMutation({
    mutationFn: async (patch: SettingsPatch) => {
      const snapshot = queryClient.getQueryData<SettingsSnapshot>(SETTINGS_QUERY_KEY)
      if (snapshot == null) {
        throw new ApiError('Settings must finish loading before they can be changed', 409)
      }
      return patchSettings(snapshot.etag, patch)
    },
    onSuccess: (snapshot) => {
      queryClient.setQueryData(SETTINGS_QUERY_KEY, snapshot)
    },
  })

  const save = useCallback(
    async (patch: SettingsPatch) => {
      try {
        return await mutation.mutateAsync(patch)
      } catch (error) {
        if (error instanceof ApiError && error.code === 'CONFIG_CONFLICT') {
          await queryClient.refetchQueries({
            queryKey: SETTINGS_QUERY_KEY,
            exact: true,
          })
        }
        throw error
      }
    },
    [mutation, queryClient],
  )

  return {
    ...query,
    snapshot: query.data ?? null,
    settings: query.data?.settings ?? null,
    save,
    isSaving: mutation.isPending,
    saveError: mutation.error,
  }
}
