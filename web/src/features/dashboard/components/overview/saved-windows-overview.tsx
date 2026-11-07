/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import { ConnectionEvidenceSummary } from '@/features/connections/connection-evidence-summary'
import { windowName } from '@/features/connections/connection-format'
import type { ConnectionSummary } from '@/features/connections/connection-state'
import {
  describeConnectionWindow,
  savedBalanceAmount,
} from '@/features/connections/window-state'
import { nativeWindowUsage } from '@/features/personal-upstream/window-state'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { FLOW_NODE_LIMIT } from '../../lib/overview-flow'

type SavedObservation = {
  item: ChannelQuotaChangeItem
  state: ReturnType<typeof describeConnectionWindow>
}

/** Bound the homepage to read connection groups without merging their series. */
export function SavedWindowsOverview(props: {
  windows: ChannelQuotaChangeItem[]
  windowsKnown: boolean
  incomplete: boolean
  connections: ConnectionSummary[]
  showAccounts: boolean
  now: number
}) {
  const { t } = useTranslation()
  const destination = props.showAccounts ? '/upstream-accounts' : '/connections'
  const groups = new Map<number, ChannelQuotaChangeItem[]>()
  let unassigned = 0
  for (const item of props.windows) {
    if (!Number.isSafeInteger(item.channel_id) || item.channel_id <= 0) {
      unassigned++
      continue
    }
    const group = groups.get(item.channel_id) ?? []
    group.push(item)
    groups.set(item.channel_id, group)
  }
  const names = new Map(props.connections.map((item) => [item.id, item.name]))
  const order = new Map(
    props.connections.map((item, index) => [item.id, index])
  )
  const observations = new Map<number, SavedObservation[]>()
  let failed = 0
  let unsupported = 0
  let uncertain = 0
  const sources = new Set<string>()
  let earliest = Infinity
  let latest = -Infinity
  let invalidTimes = 0
  for (const item of props.windows) {
    const state = describeConnectionWindow(
      item,
      groups.get(item.channel_id) ?? [item],
      props.now,
      t
    )
    if (item.status === 'error') failed++
    else if (item.status === 'unsupported') unsupported++
    else if (state.variant !== 'success') uncertain++
    sources.add(item.source || t('Unknown'))
    const observed = item.observed_at
    if (
      Number.isSafeInteger(observed) &&
      (observed ?? 0) > 0 &&
      (observed ?? 0) <= props.now
    ) {
      earliest = Math.min(earliest, observed as number)
      latest = Math.max(latest, observed as number)
    } else invalidTimes++
    if (groups.has(item.channel_id)) {
      const group = observations.get(item.channel_id) ?? []
      group.push({ item, state })
      observations.set(item.channel_id, group)
    }
  }
  const visible = [...observations]
    .sort(([a], [b]) => {
      const difference = (order.get(a) ?? Infinity) - (order.get(b) ?? Infinity)
      return Number.isNaN(difference) ? a - b : difference || a - b
    })
    .slice(0, FLOW_NODE_LIMIT)

  return (
    <section
      aria-label={t('Saved quota evidence')}
      className='bg-card text-caption flex min-w-0 flex-col gap-3 rounded-lg border p-3'
    >
      <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'>
        <h2 className='text-body font-semibold'>{t('Saved quota evidence')}</h2>
        <Link to={destination} className='underline underline-offset-3'>
          {t('View all saved windows')}
        </Link>
      </div>
      {!props.windowsKnown ? (
        <p>{t('Account windows could not be read')}</p>
      ) : (
        <>
          <p className='text-muted-foreground'>
            {t(
              'Read observations: {{count}}. Only records returned for this page are counted; other records may be absent.',
              { count: props.windows.length }
            )}
          </p>
          {props.incomplete && (
            <p role='status' className='text-warning'>
              {t(
                'Saved quota evidence is incomplete. Missing series and earlier failures may be omitted.'
              )}
            </p>
          )}
          {failed > 0 && (
            <p className='text-destructive'>
              {t('Read observations marked as failed: {{count}}', {
                count: failed,
              })}
            </p>
          )}
          {unsupported > 0 && (
            <p>
              {t('Read observations with unsupported queries: {{count}}', {
                count: unsupported,
              })}
            </p>
          )}
          {uncertain > 0 && (
            <p className='text-warning'>
              {t('Other warnings or unknowns in this read: {{count}}', {
                count: uncertain,
              })}
            </p>
          )}
          {unassigned > 0 && (
            <p className='text-warning'>
              {t(
                'Connection ownership is unknown for {{count}} read observations. Review the complete saved records.',
                { count: unassigned }
              )}
            </p>
          )}
          {props.windows.length === 0 ? (
            <p>
              {t('No saved window sample')} ·{' '}
              {t('Window query capability unknown')}
            </p>
          ) : (
            <div className='text-muted-foreground flex min-w-0 flex-col gap-1 wrap-anywhere'>
              <p>
                {t('Sources in this read: {{sources}}', {
                  sources: [...sources].sort().join(', '),
                })}
              </p>
              <p>
                {Number.isFinite(earliest)
                  ? t(
                      'Saved observation times: {{from}} – {{to}}. Missing or invalid: {{count}}.',
                      {
                        from: formatTimestampToDate(earliest),
                        to: formatTimestampToDate(latest),
                        count: invalidTimes,
                      }
                    )
                  : t('No valid observation timestamp in this read.')}
              </p>
            </div>
          )}
          {visible.length > 0 && (
            <>
              <p>
                {t(
                  'Showing {{shown}} of {{count}} read connections. Unshown connections are not assumed safe.',
                  { shown: visible.length, count: groups.size }
                )}
              </p>
              <div className='grid min-w-0 gap-3 lg:grid-cols-2 xl:grid-cols-3'>
                {visible.map(([id, records]) => (
                  <SavedWindowGroup
                    key={id}
                    connectionId={id}
                    name={names.get(id)}
                    observations={records}
                    now={props.now}
                  />
                ))}
              </div>
            </>
          )}
        </>
      )}
      <p className='text-muted-foreground'>
        {t(
          'Only saved observations are read. Opening a connection does not contact its provider.'
        )}
      </p>
    </section>
  )
}

function SavedWindowGroup(props: {
  connectionId: number
  name: string | undefined
  observations: SavedObservation[]
  now: number
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const windows = props.observations.map(({ item }) => item)
  // An error remains an error even when a later observation exists for a
  // different identity. Unsupported is a capability result, never an error.
  const ranked = props.observations
    .map((record, index) => {
      let priority = 3
      if (record.item.status === 'error') {
        priority = 0
      } else if (
        record.item.status !== 'unsupported' &&
        record.state.variant !== 'success'
      ) {
        priority = 1
      } else if (record.item.status === 'unsupported') {
        priority = 2
      }
      return { ...record, priority, index }
    })
    .sort((a, b) => a.priority - b.priority || a.index - b.index)
  const failed = windows.filter((item) => item.status === 'error').length
  const unsupported = windows.filter(
    (item) => item.status === 'unsupported'
  ).length
  const uncertain = props.observations.filter(
    ({ item, state }) =>
      item.status !== 'error' &&
      item.status !== 'unsupported' &&
      state.variant !== 'success'
  ).length
  const name = props.name?.trim()
  const title = name
    ? t('{{name}} (#{{id}})', { name, id: props.connectionId })
    : t('Connection #{{id}}', { id: props.connectionId })

  return (
    <section
      aria-labelledby={`overview-saved-connection-${props.connectionId}`}
      className='flex min-w-0 flex-col gap-2 rounded-md border p-3'
    >
      <h3
        id={`overview-saved-connection-${props.connectionId}`}
        className='text-body font-semibold wrap-anywhere'
      >
        {title}
      </h3>
      {failed > 0 && (
        <p className='text-destructive'>
          {t('Read observations marked as failed: {{count}}', {
            count: failed,
          })}
        </p>
      )}
      {unsupported > 0 && (
        <p>
          {t('Read observations with unsupported queries: {{count}}', {
            count: unsupported,
          })}
        </p>
      )}
      {uncertain > 0 && (
        <p className='text-warning'>
          {t('Other warnings or unknowns in this read: {{count}}', {
            count: uncertain,
          })}
        </p>
      )}
      <ul className='flex min-w-0 flex-col gap-3'>
        {ranked.slice(0, 2).map(({ item, state, index }) => {
          const used = nativeWindowUsage(item)
          const balance = savedBalanceAmount(item)
          const label = windowName(item, t)
          let value = t('{{window}}: {{state}}', {
            window: label,
            state: t('Unknown'),
          })
          if (used !== null) {
            value = t('{{window}} · {{value}}% left', {
              window: label,
              value: Math.round(100 - used),
            })
          } else if (balance !== null) {
            value = t('{{window}}: {{value}} {{unit}}', {
              window: label,
              value: balance,
              unit: item.currency || item.unit || t('Unknown'),
            })
          }
          const observed = item.observed_at
          const time =
            Number.isSafeInteger(observed) &&
            (observed ?? 0) > 0 &&
            (observed ?? 0) <= props.now
              ? t('Observed {{time}}', {
                  time: formatTimestampToDate(observed as number),
                })
              : t('No timestamp')
          return (
            <li
              key={`${item.series_id ?? ''}:${index}`}
              className='flex min-w-0 flex-col gap-1 wrap-anywhere'
            >
              <div className='flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1'>
                <p className='font-medium'>
                  {item.account_label || t('Account identity unknown')}
                </p>
                <p>{value}</p>
                <p
                  className={cn(
                    item.status !== 'unsupported' &&
                      state.variant === 'danger' &&
                      'text-destructive',
                    item.status !== 'unsupported' &&
                      state.variant === 'warning' &&
                      'text-warning'
                  )}
                >
                  {state.label}
                </p>
              </div>
              <div className='text-muted-foreground flex min-w-0 flex-wrap gap-x-2 gap-y-1'>
                <span>
                  {t('Unit')}: {item.currency || item.unit || t('Unknown')}
                </span>
                <span>
                  {t('Source')}: {item.source || t('Unknown')}
                </span>
                <span>{time}</span>
              </div>
            </li>
          )
        })}
      </ul>
      <p className='text-muted-foreground'>
        {t(
          'Showing {{shown}} of {{count}} read observations for this connection.',
          { shown: Math.min(2, windows.length), count: windows.length }
        )}
      </p>
      <Collapsible open={expanded} onOpenChange={setExpanded}>
        <CollapsibleTrigger
          render={<Button variant='outline' size='sm' />}
          className='h-auto min-h-11 max-w-full whitespace-normal'
        >
          {t('Saved observations ({{count}})', { count: windows.length })}
        </CollapsibleTrigger>
        <CollapsibleContent>
          {expanded && (
            <div className='mt-3'>
              <ConnectionEvidenceSummary
                windows={windows}
                windowsKnown
                windowsLoading={false}
                now={props.now}
              />
            </div>
          )}
        </CollapsibleContent>
      </Collapsible>
    </section>
  )
}
