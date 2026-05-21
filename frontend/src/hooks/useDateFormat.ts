import { useMemo } from 'react'
import { useMetaContext } from '@/contexts/MetaContext'
import {
  formatDateTime as fmtDateTime,
  formatDateTimeShort as fmtDateTimeShort,
  formatRelativeTime as fmtRelativeTime,
  formatTimestamp as fmtTimestamp,
} from '@/lib/dateFormat'

export function useDateFormat() {
  const { timezone, locale } = useMetaContext()
  return useMemo(
    () => ({
      formatDateTime: (value: string) => fmtDateTime(value, timezone, locale),
      formatDateTimeShort: (value: string) =>
        fmtDateTimeShort(value, timezone, locale),
      formatRelativeTime: (value: string) =>
        fmtRelativeTime(value, timezone, locale),
      formatTimestamp: (value: string) => fmtTimestamp(value, timezone, locale),
    }),
    [timezone, locale],
  )
}
