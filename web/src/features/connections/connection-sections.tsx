/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { CHANNEL_STATUS } from '@/features/channels/constants'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import {
  describeNativeWindowState,
  nativeWindowState,
  nativeWindowUsage,
} from '@/features/personal-upstream/window-state'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { formatTestDuration, windowName } from './connection-format'
import {
  connectionValues,
  type ConnectionKind,
  type ConnectionSummary,
} from './connection-state'
import type { ConnectionTestResult } from './use-connection-test'

/**
 * The connection's state in four words (design v2 upstream): in use (enabled
 * and carrying recent requests), available (enabled), auto disabled and
 * disabled. A status the server did not send is not guessed.
 */
export function ConnectionStatusBadge(props: {
  status: number
  /** Requests among the latest records; undefined when unknown. */
  recentRequests?: number
}) {
  const { t } = useTranslation()
  let variant: StatusVariant = 'neutral'
  let label = t('Status not provided')
  if (props.status === CHANNEL_STATUS.ENABLED) {
    const inUse = (props.recentRequests ?? 0) > 0
    variant = inUse ? 'info' : 'success'
    label = inUse ? t('In use') : t('Available')
  } else if (props.status === CHANNEL_STATUS.AUTO_DISABLED) {
    variant = 'warning'
    label = t('Auto Disabled')
  } else if (props.status === CHANNEL_STATUS.MANUAL_DISABLED) {
    label = t('Disabled', { context: 'connection' })
  }
  return (
    <StatusBadge
      variant={variant}
      label={label}
      showDot
      copyable={false}
      className='h-auto min-h-5 whitespace-normal'
    />
  )
}

export function ConnectionKindBadge(props: { kind: ConnectionKind }) {
  const { t } = useTranslation()
  let label = t('API Key')
  if (props.kind === 'subscription') {
    label = t('Subscription')
  } else if (props.kind === 'local') {
    label = t('Local')
  }
  return (
    <StatusBadge
      variant='neutral'
      label={label}
      copyable={false}
      className='h-auto min-h-5 whitespace-normal'
    />
  )
}

/** Last saved test from the connection record; 0 means never tested. */
export function LastTest(props: { item: ConnectionSummary }) {
  const { t } = useTranslation()
  if (!props.item.test_time) {
    return <span className='text-muted-foreground'>{t('Not tested')}</span>
  }
  return (
    <span className='tabular-nums'>
      {formatTimestampToDate(props.item.test_time)}
      {props.item.response_time
        ? ` · ${formatTestDuration(props.item.response_time)}`
        : ''}
    </span>
  )
}

export function ConnectionTestOutcome(props: {
  result: ConnectionTestResult | null
  connectionId: number
}) {
  const { t } = useTranslation()
  const result =
    props.result?.connectionId === props.connectionId ? props.result : null
  return (
    <div aria-live='polite' className='min-w-0 empty:hidden'>
      {result?.success && (
        <Alert>
          <AlertTitle>{t('Test succeeded')}</AlertTitle>
          {result.responseTime !== undefined && (
            <AlertDescription>
              {t('Response time: {{duration}}', {
                duration: formatTestDuration(result.responseTime),
              })}
            </AlertDescription>
          )}
        </Alert>
      )}
      {result && !result.success && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Test failed')}</AlertTitle>
          <AlertDescription className='wrap-anywhere'>
            {result.error ?? t('Unknown error')}
            {result.errorCode ? ` (${result.errorCode})` : ''}
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}

/** Priority, weight, groups and models from the saved configuration. */
export function ConnectionRouting(props: {
  item: ConnectionSummary
  /** Links to the routing preview of the table view. */
  showPreview: boolean
}) {
  const { t } = useTranslation()
  const models = connectionValues(props.item.models)
  const groups = connectionValues(props.item.group)
  return (
    <section
      aria-label={t('Routing configuration')}
      className='bg-card p-card flex min-w-0 flex-col gap-4 rounded-xl border'
    >
      <dl className='grid min-w-0 grid-cols-2 gap-3'>
        <div>
          <dt className='text-caption text-muted-foreground'>
            {t('Priority')}
          </dt>
          <dd className='text-data tabular-nums'>
            {props.item.priority ?? t('Unknown')}
          </dd>
        </div>
        <div>
          <dt className='text-caption text-muted-foreground'>{t('Weight')}</dt>
          <dd className='text-data tabular-nums'>
            {props.item.weight ?? t('Unknown')}
          </dd>
        </div>
        <div className='col-span-2 min-w-0'>
          <dt className='text-caption text-muted-foreground'>{t('Group')}</dt>
          <dd className='text-data wrap-anywhere'>
            {groups.join(', ') || t('Unknown')}
          </dd>
        </div>
        <div className='col-span-2 min-w-0'>
          <dt className='text-caption text-muted-foreground'>{t('Models')}</dt>
          <dd>
            {models.length ? (
              <ul className='mt-1 flex min-w-0 flex-wrap gap-2'>
                {models.map((model) => (
                  <li
                    key={model}
                    className='bg-muted text-data max-w-full rounded-md px-2 py-1 font-mono wrap-anywhere'
                  >
                    {model}
                  </li>
                ))}
              </ul>
            ) : (
              t('Unknown')
            )}
          </dd>
        </div>
      </dl>
      <p className='text-caption text-muted-foreground'>
        {t('Configuration status does not confirm upstream availability.')}
      </p>
      {props.showPreview && (
        <Link
          to='/channels'
          hash='channel-routing'
          className='text-primary text-body inline-flex items-center self-start underline-offset-4 hover:underline pointer-coarse:min-h-11'
        >
          {t('Preview routing for a group and model')}
        </Link>
      )}
    </section>
  )
}

type HistoryEvent = {
  key: string
  time: number
  text: string
  tone: 'success' | 'warning' | 'neutral'
}
const HISTORY_DOT: Record<HistoryEvent['tone'], string> = {
  success: 'bg-success',
  warning: 'bg-warning',
  neutral: 'bg-muted-foreground',
}

/**
 * What the saved records say happened to the connection, newest first: each
 * window's latest and previous samples and the last saved connectivity
 * test. Nothing is inferred between samples.
 */
export function ConnectionHistory(props: {
  item: ConnectionSummary
  windows: ChannelQuotaChangeItem[] | undefined
  windowsError: boolean
  /** Links to the full window history of the table view. */
  showFullHistory: boolean
}) {
  const { t } = useTranslation()
  const now = Date.now() / 1000
  const events: HistoryEvent[] = []
  for (const item of props.windows ?? []) {
    const name = windowName(item, t)
    const used = nativeWindowUsage(item)
    const observed = item.observed_at
    if (Number.isSafeInteger(observed) && (observed ?? 0) > 0) {
      const state = describeNativeWindowState(nativeWindowState(item, now), t)
      events.push({
        key: `${item.series_id ?? item.window_type}:latest`,
        time: observed as number,
        text:
          used === null
            ? t('{{window}} sample: {{state}}', {
                window: name,
                state: state.label,
              })
            : t('{{window}} sample: {{value}}% left', {
                window: name,
                value: Math.round(100 - used),
              }),
        tone: used === null ? 'warning' : 'success',
      })
    }
    const previous = item.previous_available
    const previousAt = item.previous_observed_at
    if (
      typeof previous === 'number' &&
      Number.isFinite(previous) &&
      previous >= 0 &&
      previous <= 100 &&
      Number.isSafeInteger(previousAt) &&
      (previousAt ?? 0) > 0
    ) {
      events.push({
        key: `${item.series_id ?? item.window_type}:previous`,
        time: previousAt as number,
        text: t('{{window}} sample: {{value}}% left', {
          window: name,
          value: Math.round(previous),
        }),
        tone: 'neutral',
      })
    }
  }
  if (props.item.test_time) {
    events.push({
      key: 'test',
      time: props.item.test_time,
      text: props.item.response_time
        ? t('Connectivity test saved · {{duration}}', {
            duration: formatTestDuration(props.item.response_time),
          })
        : t('Connectivity test saved'),
      tone: 'neutral',
    })
  }
  events.sort((a, b) => b.time - a.time)

  return (
    <section
      aria-label={t('History')}
      className='bg-card p-card flex min-w-0 flex-col gap-4 rounded-xl border'
    >
      {props.windowsError && (
        <p role='alert' className='text-body text-destructive'>
          {t('Unable to load account windows')}
        </p>
      )}
      {events.length === 0 ? (
        <p className='text-body text-muted-foreground'>
          {t('No saved samples or tests for this connection yet.')}
        </p>
      ) : (
        <ol className='border-border ms-2 flex min-w-0 flex-col gap-3.5 border-s-2 ps-4'>
          {events.map((event) => (
            <li key={event.key} className='relative min-w-0'>
              <span
                aria-hidden='true'
                className={cn(
                  'border-card absolute -start-[1.4rem] top-1 size-3 rounded-full border-2',
                  HISTORY_DOT[event.tone]
                )}
              />
              <span className='text-caption text-muted-foreground block font-mono tabular-nums'>
                {formatTimestampToDate(event.time)}
              </span>
              <span className='text-body wrap-anywhere'>{event.text}</span>
            </li>
          ))}
        </ol>
      )}
      <p className='text-caption text-muted-foreground'>
        {t(
          'Only saved samples and the last saved test are listed. Opening this page does not contact the provider.'
        )}
      </p>
      {props.showFullHistory && (props.windows?.length ?? 0) > 0 && (
        <Link
          to='/channels'
          search={{ quotaChannelId: props.item.id }}
          className='text-primary text-body inline-flex items-center self-start underline-offset-4 hover:underline pointer-coarse:min-h-11'
        >
          {t('View channel history')}
        </Link>
      )}
    </section>
  )
}
