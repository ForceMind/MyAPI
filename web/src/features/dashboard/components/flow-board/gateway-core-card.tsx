/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Skeleton } from '@/components/ui/skeleton'
import { toIntlLocale } from '@/i18n/languages'
import { cn } from '@/lib/utils'

import type { HourBucket } from '../../lib/overview-flow'

export type GatewayAddress = {
  baseUrl: string
  /** `server`: the configured server address; `current`: this browser's origin. */
  source: 'server' | 'current'
}

function Kpi(props: { label: string; loading: boolean; value: string }) {
  return (
    <div className='flex min-w-0 flex-col'>
      <dt className='text-caption opacity-90'>{props.label}</dt>
      <dd className='text-title font-semibold tabular-nums'>
        {props.loading ? (
          <Skeleton className='bg-core-foreground/20 mt-1 h-5 w-12' />
        ) : (
          props.value
        )}
      </dd>
    </div>
  )
}

/**
 * Gateway core card of the flow board: the access address with copy, the
 * 24-hour request count, failure rate and median duration, requests per hour,
 * and the edition, version and access summary. A value the server did not
 * provide shows "—", never 0.
 */
export function GatewayCoreCard(props: {
  address: GatewayAddress
  /** Exact 24-hour request count; null when unavailable. */
  requests: number | null
  requestsLoading: boolean
  /** Failure share of the sampled requests; null when nothing was sampled. */
  failureRate: number | null
  medianSeconds: number | undefined
  sampleLoading: boolean
  /** Null while loading or when the call records could not be read. */
  buckets: HourBucket[] | null
  meta: string[]
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.language)
  const number = new Intl.NumberFormat(locale)
  const percent = new Intl.NumberFormat(locale, {
    style: 'percent',
    maximumFractionDigits: 1,
  })
  const hour = new Intl.DateTimeFormat(locale, {
    hour: '2-digit',
    minute: '2-digit',
  })
  const peak = Math.max(1, ...(props.buckets ?? []).map((item) => item.count))
  const uncovered = props.buckets?.some((bucket) => !bucket.covered) ?? false

  return (
    <section
      aria-labelledby='overview-gateway-title'
      data-testid='flow-core'
      className='bg-core text-core-foreground rounded-card flex min-w-0 flex-col gap-3 p-5'
    >
      <h3
        id='overview-gateway-title'
        className='text-title flex items-center gap-2.5 font-semibold'
      >
        <img
          src='/myapi-logo-v1.png'
          alt=''
          className='size-8 shrink-0 rounded-md'
        />
        {t('MyAPI gateway')}
      </h3>
      <div className='flex min-w-0 flex-col gap-1'>
        <div className='bg-core-foreground/10 flex min-w-0 items-center gap-1 rounded-md py-0.5 ps-2.5'>
          <code
            className='text-data min-w-0 flex-1 truncate font-mono'
            title={props.address.baseUrl}
          >
            {props.address.baseUrl}
          </code>
          <CopyButton
            value={props.address.baseUrl}
            tooltip={t('Copy access address')}
            aria-label={t('Copy access address')}
            className='hover:bg-core-foreground/15 text-core-foreground hover:text-core-foreground'
          />
        </div>
        <p className='text-caption opacity-90'>
          {props.address.source === 'server'
            ? t('Configured server address')
            : t('Current access address')}
        </p>
      </div>
      <dl className='grid grid-cols-3 gap-2'>
        <Kpi
          label={t('Last 24 h')}
          loading={props.requestsLoading}
          value={props.requests === null ? '—' : number.format(props.requests)}
        />
        <Kpi
          label={t('Failure rate')}
          loading={props.sampleLoading}
          value={
            props.failureRate === null ? '—' : percent.format(props.failureRate)
          }
        />
        <Kpi
          label={t('Median duration')}
          loading={props.sampleLoading}
          value={
            props.medianSeconds === undefined
              ? '—'
              : t('{{count}} s', {
                  count: Number(props.medianSeconds.toFixed(2)),
                })
          }
        />
      </dl>
      {props.buckets && (
        <figure className='flex min-w-0 flex-col gap-1'>
          {/* Single series: no legend; every value is also in the list text. */}
          <ol
            aria-label={t('Requests per hour')}
            className='flex h-8 items-end gap-0.5'
          >
            {props.buckets.map((bucket) => {
              const label = bucket.covered
                ? t('{{time}}: {{count}} requests', {
                    time: hour.format(new Date(bucket.start * 1000)),
                    count: bucket.count,
                  })
                : t('{{time}}: not covered by the sample', {
                    time: hour.format(new Date(bucket.start * 1000)),
                  })
              return (
                <li
                  key={bucket.start}
                  title={label}
                  className={cn(
                    'flex h-full min-w-0 flex-1 items-end',
                    !bucket.covered &&
                      'border-core-foreground/40 border-b border-dashed'
                  )}
                >
                  <span className='sr-only'>{label}</span>
                  {bucket.covered && bucket.count > 0 && (
                    <span
                      aria-hidden='true'
                      className='bg-telemetry block w-full rounded-t-[1px]'
                      style={{
                        height: `${Math.max(8, (bucket.count / peak) * 100)}%`,
                      }}
                    />
                  )}
                  {/* A measured zero keeps a baseline; it is not a gap. */}
                  {bucket.covered && bucket.count === 0 && (
                    <span
                      aria-hidden='true'
                      className='bg-core-foreground/30 block h-0.5 w-full'
                    />
                  )}
                </li>
              )
            })}
          </ol>
          <figcaption className='text-caption opacity-90'>
            {uncovered
              ? t('Requests per hour, latest sampled requests only')
              : t('Requests per hour, last 24 hours')}
          </figcaption>
        </figure>
      )}
      {props.meta.length > 0 && (
        <p className='text-caption opacity-90'>{props.meta.join(' · ')}</p>
      )}
    </section>
  )
}
