/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useTranslation } from 'react-i18next'

import type { StatusVariant } from '@/components/status-badge'
import { cn } from '@/lib/utils'

import type { UsageLog } from '../data/schema'
import { getRequestFacts } from '../lib/request-facts'

const valueTone: Partial<Record<StatusVariant, string>> = {
  success: 'text-success',
  danger: 'text-destructive',
  warning: 'text-warning',
}

/**
 * The four independent results of one request (design 8.2): business
 * outcome, HTTP status, how far the usage can be trusted, and settlement.
 * A missing fact reads as not recorded or not provided, never as success or
 * zero.
 */
export function ResultGrid(props: { log: UsageLog }) {
  const { t } = useTranslation()
  const facts = getRequestFacts(props.log)
  let usage: { label: string; variant: StatusVariant } = facts.usage
  if (facts.usageKnown && facts.accuracy === 'estimated') {
    usage = { label: 'Estimated usage', variant: 'warning' }
  } else if (facts.usageKnown && facts.accuracy === 'reported') {
    usage = { label: 'Reported usage', variant: 'success' }
  }
  const cells: {
    key: string
    label: string
    value: string | number
    variant?: StatusVariant
  }[] = [
    {
      key: 'outcome',
      label: t('Business result'),
      value: t(facts.outcome.label),
      variant: facts.outcome.variant,
    },
    {
      key: 'http',
      label: t('HTTP status'),
      value: facts.httpStatus ?? t('Not recorded'),
      variant: facts.httpStatus === null ? 'neutral' : undefined,
    },
    {
      key: 'usage',
      label: t('Usage reliability'),
      value: t(usage.label),
      variant: usage.variant,
    },
    {
      key: 'settlement',
      label: t('Settlement'),
      value: t(facts.settlement.label),
      variant: facts.settlement.variant,
    },
  ]
  return (
    <dl
      className='grid min-w-0 grid-cols-2 gap-2 sm:gap-3 lg:grid-cols-4'
      data-testid='request-result-grid'
    >
      {cells.map((cell) => (
        <div
          key={cell.key}
          className='bg-card flex min-w-0 flex-col gap-0.5 rounded-lg border px-3 py-2.5'
        >
          <dt className='text-caption text-muted-foreground'>{cell.label}</dt>
          <dd
            className={cn(
              'text-sm font-semibold tabular-nums wrap-anywhere',
              cell.variant === 'neutral'
                ? 'text-muted-foreground'
                : cell.variant && valueTone[cell.variant]
            )}
          >
            {cell.value}
          </dd>
        </div>
      ))}
    </dl>
  )
}
