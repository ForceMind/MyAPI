/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { toIntlLocale } from '@/i18n/languages'

import type { UsageLog } from '../data/schema'
import { getRequestFacts } from '../lib/request-facts'

/** Request time in the interface language; an invalid timestamp is unknown. */
export function RequestTime(props: { seconds: number; withDate?: boolean }) {
  const { t, i18n } = useTranslation()
  const date = new Date(props.seconds * 1000)
  if (!Number.isFinite(date.getTime()) || props.seconds <= 0) {
    return <span>{t('Unknown')}</span>
  }
  const sameDay = new Date().toDateString() === date.toDateString()
  return (
    <time dateTime={date.toISOString()} className='tabular-nums'>
      {new Intl.DateTimeFormat(toIntlLocale(i18n.language), {
        ...(props.withDate || !sameDay
          ? { month: '2-digit', day: '2-digit' }
          : {}),
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
      }).format(date)}
    </time>
  )
}

/** Business outcome as dot plus text, never colour alone; long labels wrap. */
export function RequestOutcomeBadge(props: { log: UsageLog }) {
  const { t } = useTranslation()
  const facts = getRequestFacts(props.log)
  return (
    <StatusBadge
      variant={facts.outcome.variant}
      showDot
      copyable={false}
      className='h-auto min-h-5 whitespace-normal'
    >
      {t(facts.outcome.label)}
    </StatusBadge>
  )
}

/** Duration in seconds, or "Not provided" when the record has none. */
export function RequestDuration(props: { seconds: number }) {
  const { t } = useTranslation()
  if (!Number.isFinite(props.seconds) || props.seconds <= 0) {
    return <span className='text-muted-foreground'>{t('Not provided')}</span>
  }
  return (
    <span className='tabular-nums'>
      {t('{{count}} s', { count: props.seconds })}
    </span>
  )
}
