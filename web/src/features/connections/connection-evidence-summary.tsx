/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */
import { useContext } from 'react'
import { useTranslation } from 'react-i18next'

import { SummaryDisclosure } from '@/components/summary-disclosure'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import { nativeWindowResetLabel, nativeWindowUsage } from '@/features/personal-upstream/window-state'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { windowName } from './connection-format'
import { ConnectionEvidenceContext } from './connection-evidence-context'
import { describeConnectionWindow, isHistoricalConnectionWindow, savedBalanceAmount } from './window-state'

/** Counts describe this saved series and do not claim old failures recovered. */
export function ConnectionEvidenceQuality(props: { item: ChannelQuotaChangeItem }) {
  const { t } = useTranslation()
  const errors = props.item.data_quality?.error_count
  const unsupported = props.item.data_quality?.unsupported_count
  const hasErrors = Number.isSafeInteger(errors) && (errors ?? 0) > 0
  const hasUnsupported = Number.isSafeInteger(unsupported) && (unsupported ?? 0) > 0
  if (!hasErrors && !hasUnsupported) return null
  return <div className='text-caption text-muted-foreground min-w-0 wrap-anywhere'>
    {hasErrors && <p>{t('Failed samples in this series: {{count}}', { count: errors })}</p>}
    {hasUnsupported && <p>{t('Unsupported samples in this series: {{count}}', { count: unsupported })}</p>}
  </div>
}

/** Every saved series keeps its own state and observation time. */
export function ConnectionEvidenceSummary(props: {
  windows: ChannelQuotaChangeItem[]
  /** False when the saved windows could not be read. */
  windowsKnown: boolean
  windowsIncomplete?: boolean
  windowsLoading: boolean
  now: number
  compact?: boolean
}) {
  const { t } = useTranslation()
  if (props.windowsLoading) {
    return <span>{t('Loading...')}</span>
  }
  if (!props.windowsKnown && !props.windowsIncomplete) {
    return <span>{t('Account windows could not be read')}</span>
  }
  if (props.windows.length === 0 && !props.windowsIncomplete) {
    return <div><p>{t('No saved window sample')}</p><p>{t('Window query capability unknown')}</p></div>
  }
  const hasBalance = props.windows.some((item) => item.metric_type === 'balance')
  let label = props.compact ? t('Current account windows') : t('Saved account windows')
  if (hasBalance) label = t('Saved quota evidence')
  return (
    <div className='flex min-w-0 flex-col gap-1'>
    {props.windowsIncomplete && <p role='status' className='text-warning'>{t('Saved quota evidence is incomplete. Missing series and earlier failures may be omitted.')}</p>}
    <ul aria-label={label} className={cn('flex min-w-0', props.compact ? 'flex-wrap gap-x-3 gap-y-1' : 'flex-col gap-2')}>
      {props.windows.map((item) => {
        const used = nativeWindowUsage(item)
        const reset = nativeWindowResetLabel(item, props.now, t)
        const balance = savedBalanceAmount(item)
        const state = describeConnectionWindow(item, props.windows, props.now, t)
        const stateLabel = props.compact && isHistoricalConnectionWindow(item, props.windows, props.now)
          ? t('Saved sample error; recovery unconfirmed')
          : state.label
        let summary = used === null
          ? t('{{window}}: {{state}}', { window: windowName(item, t), state: stateLabel })
          : t('{{window}} · {{value}}% left', { window: windowName(item, t), value: Math.round(100 - used) })
        if (balance !== null) {
          summary = t('{{window}}: {{value}} {{unit}}', {
            window: windowName(item, t),
            value: balance,
            unit: item.currency || item.unit || t('Unknown'),
          })
        }
        const observed = item.observed_at
        const time =
          Number.isSafeInteger(observed) && (observed ?? 0) > 0
            ? t('Observed {{time}}', {
                time: formatTimestampToDate(observed as number),
              })
            : t('No timestamp')
        return (
          <li
            key={item.series_id ?? `${item.channel_id}:${item.source}:${item.account_label ?? ''}:${item.window_type}:${item.window_seconds ?? ''}:${item.observed_at ?? ''}`}
            className={cn('min-w-0 wrap-anywhere', props.compact && state.variant === 'danger' && 'text-destructive', props.compact && state.variant === 'warning' && 'text-warning')}
          >
            <span>
              {[summary, used !== null || balance !== null ? stateLabel : null, props.compact ? null : time]
                .filter(Boolean)
                .join(' · ')}
            </span>
            {reset && <>{' · '}<span className='text-muted-foreground'>{reset}</span></>}
            {!props.compact && (
              <dl className='mt-1 flex min-w-0 flex-col gap-1'>
                <div>
                  <dt className='sr-only'>{t('Account')}</dt>
                  <dd>{item.account_label || t('Account identity unknown')}</dd>
                </div>
                <div className='flex min-w-0 flex-wrap gap-x-1'>
                  <dt>{t('Source')}:</dt>
                  <dd className='min-w-0 font-mono wrap-anywhere'>{item.source || t('Unknown')}</dd>
                </div>
              </dl>
            )}
            {(item.status === 'error' || item.status === 'unsupported') && !props.compact && (
              <p className='mt-1'>{state.description}</p>
            )}
            {!props.compact && <ConnectionEvidenceQuality item={item} />}
          </li>
        )
      })}
    </ul>
    {props.compact && hasBalance && !props.windows.some((item) => item.metric_type === 'codex_rate_limit') && (
      <p>{t('Window query capability unknown')}</p>
    )}
    </div>
  )
}


export function ChannelSavedEvidence(props: { channelId: number }) {
  const { t } = useTranslation()
  const evidence = useContext(ConnectionEvidenceContext)
  if (!evidence) return null
  const windows = evidence.data?.filter((item) => item.channel_id === props.channelId) ?? []
  return <div className='flex min-w-0 flex-col gap-1'>
    <ConnectionEvidenceSummary windows={windows} windowsKnown={evidence.known} windowsIncomplete={evidence.incomplete} windowsLoading={evidence.loading} now={evidence.now} compact />
    {(evidence.known || evidence.incomplete) && windows.length > 0 && <SummaryDisclosure key={`${evidence.identity}:${props.channelId}:evidence`} summary={t('Saved evidence details ({{count}})', { count: windows.length })}>
      <ConnectionEvidenceSummary windows={windows} windowsKnown={evidence.known} windowsIncomplete={evidence.incomplete} windowsLoading={evidence.loading} now={evidence.now} />
    </SummaryDisclosure>}
  </div>
}
