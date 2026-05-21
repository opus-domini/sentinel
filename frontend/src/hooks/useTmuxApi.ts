import { useCallback } from 'react'
import type { GuardrailRule } from '@/types'

type GuardrailDecision = {
  mode: string
  allowed: boolean
  requireConfirm: boolean
  message: string
  matchedRuleId: string
  matchedRules: Array<GuardrailRule>
}

export class GuardrailConfirmError extends Error {
  readonly decision: GuardrailDecision
  readonly path: string
  readonly init: RequestInit | undefined

  constructor(
    message: string,
    decision: GuardrailDecision,
    path: string,
    init: RequestInit | undefined,
  ) {
    super(message)
    this.name = 'GuardrailConfirmError'
    this.decision = decision
    this.path = path
    this.init = init
  }
}

export function useTmuxApi() {
  return useCallback(
    async <T>(path: string, init?: RequestInit): Promise<T> => {
      const headers: Record<string, string> = {
        'Content-Type': 'application/json',
      }

      if (init?.headers) {
        Object.assign(headers, init.headers as Record<string, string>)
      }

      const response = await fetch(path, {
        ...init,
        credentials: 'same-origin',
        headers,
      })

      let payload: unknown = {}
      try {
        payload = await response.json()
      } catch {
        payload = {}
      }

      if (!response.ok) {
        const errorObj =
          typeof payload === 'object' && payload !== null && 'error' in payload
            ? (payload as { error: Record<string, unknown> }).error
            : null

        if (
          response.status === 428 &&
          errorObj?.code === 'GUARDRAIL_CONFIRM_REQUIRED'
        ) {
          const details = errorObj.details as
            | { decision?: GuardrailDecision }
            | undefined
          const decision = details?.decision
          if (decision) {
            throw new GuardrailConfirmError(
              decision.message ||
                (errorObj.message as string) ||
                'Confirmation required',
              decision,
              path,
              init,
            )
          }
        }

        const message =
          errorObj?.message != null && typeof errorObj.message === 'string'
            ? errorObj.message
            : `HTTP ${response.status}`
        throw new Error(message)
      }

      if (
        typeof payload === 'object' &&
        payload !== null &&
        'data' in payload
      ) {
        return (payload as { data: T }).data
      }

      return payload as T
    },
    [],
  )
}
