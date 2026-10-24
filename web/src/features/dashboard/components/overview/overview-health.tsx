/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { getRequestFacts } from '@/features/usage-logs/lib/request-facts'
import { toIntlLocale } from '@/i18n/languages'

import type { OverviewData } from './use-overview-data'

function Stat(props: {
  label: string
  value: ReactNode
  caption: ReactNode
  loading: boolean
}) {
  return (
    <div className='bg-card flex min-w-0 flex-col gap-0.5 rounded-xl border px-4 py-3'>
      <dt className='text-caption text-muted-foreground'>{props.label}</dt>
      <dd className='text-metric min-w-0 tabular-nums'>
        {props.loading ? <Skeleton className='h-9 w-20' /> : props.value}
      </dd>
      <dd className='text-caption text-muted-foreground min-w-0'>
        {props.caption}
      </dd>
    </div>
  )
}

/** Median of positive durations; undefined when no row reports one. */
function medianSeconds(values: number[]) {
  const known = values
    .filter((value) => Number.isFinite(value) && value > 0)
    .sort((a, b) => a - b)
  if (known.length === 0) return undefined
  const middle = Math.floor(known.length / 2)
  return known.length % 2
    ? known[middle]
    : (known[middle - 1] + known[middle]) / 2
}

/**
 * Health strip (design 8.1): requests in the last 24 hours, failure rate and
 * median duration of the latest requests, and enabled connections. A value
 * the server did not provide shows "—" with the reason, never 0.
 */
export function OverviewHealth(props: { data: OverviewData }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.language)
  const rows = props.data.sample.isError ? undefined : props.data.sample.data
  const failures = rows?.filter((row) => !getRequestFacts(row).completed).length
  const median = rows
    ? medianSeconds(rows.map((row) => row.use_time))
    : undefined
  const sampleCaption =
    rows && rows.length > 0
      ? t('Latest {{count}} requests in 24 hours', { count: rows.length })
      : t('No requests in the last 24 hours')
  const unavailable = t('Not provided')
  const number = new Intl.NumberFormat(locale)
  const percent = new Intl.NumberFormat(locale, {
    style: 'percent',
    maximumFractionDigits: 1,
  })
  const connections = props.data.connections
  let medianCaption = sampleCaption
  if (median === undefined) {
    medianCaption =
      props.data.sample.isError || (rows && rows.length > 0)
        ? unavailable
        : t('No requests in the last 24 hours')
  }

  return (
    <section aria-labelledby='overview-health-title' className='min-w-0'>
      <h2 id='overview-health-title' className='sr-only'>
        {t('Health')}
      </h2>
      <dl className='grid min-w-0 grid-cols-2 gap-3 lg:grid-cols-4'>
        <Stat
          label={t('Requests (24 h)')}
          loading={props.data.counts.isPending}
          value={
            props.data.counts.isError
              ? '—'
              : number.format(props.data.counts.data ?? 0)
          }
          caption={
            props.data.counts.isError ? unavailable : t('Successful and failed')
          }
        />
        <Stat
          label={t('Failure rate')}
          loading={props.data.sample.isPending}
          value={
            rows && rows.length > 0 && failures !== undefined
              ? percent.format(failures / rows.length)
              : '—'
          }
          caption={props.data.sample.isError ? unavailable : sampleCaption}
        />
        <Stat
          label={t('Median duration')}
          loading={props.data.sample.isPending}
          value={
            median === undefined
              ? '—'
              : t('{{count}} s', { count: Number(median.toFixed(2)) })
          }
          caption={medianCaption}
        />
        {props.data.canReadChannels && (
          <Stat
            label={t('Enabled connections')}
            loading={connections.isPending}
            value={
              connections.data
                ? `${number.format(connections.data.enabled)} / ${number.format(connections.data.total)}`
                : '—'
            }
            caption={
              connections.isError ? unavailable : t('Enabled of configured')
            }
          />
        )}
      </dl>
    </section>
  )
}
