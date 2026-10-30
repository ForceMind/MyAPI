/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import type { UsageLog } from '../data/schema'
import { parseLogOther } from '../lib/format'
import { relayAttemptOutcomeLabel } from '../lib/relay-attempts'
import { getRequestFacts } from '../lib/request-facts'

type Tone = 'ok' | 'fail' | 'warn' | 'neutral'

const attemptItemClass =
  'flex min-w-0 flex-col gap-1 rounded-md border px-3 py-2'

const dotTone: Record<Tone, string> = {
  ok: 'bg-success',
  fail: 'bg-destructive',
  warn: 'bg-warning',
  neutral: 'bg-muted-foreground',
}

function attemptTone(outcome: string): Tone {
  if (outcome === 'completed') return 'ok'
  if (outcome === 'failed' || outcome === 'refused') return 'fail'
  if (outcome === 'retryable_refusal') return 'warn'
  return 'neutral'
}

function Step(props: { tone: Tone; title: ReactNode; children?: ReactNode }) {
  return (
    <li className='relative min-w-0 ps-6 pb-4 last:pb-0'>
      <span
        aria-hidden='true'
        className={cn(
          'ring-card absolute start-[-7px] top-1 size-3 rounded-full ring-2',
          dotTone[props.tone]
        )}
      />
      <p className='text-data font-medium wrap-anywhere'>{props.title}</p>
      {props.children != null && (
        <div className='text-caption text-muted-foreground mt-1 flex min-w-0 flex-col gap-1 wrap-anywhere'>
          {props.children}
        </div>
      )}
    </li>
  )
}

function Fact(props: { label: string; value: ReactNode }) {
  return (
    <div className='flex min-w-0 flex-wrap gap-x-2'>
      <dt className='text-muted-foreground'>{props.label}</dt>
      <dd className='text-foreground font-mono wrap-anywhere'>{props.value}</dd>
    </div>
  )
}

/**
 * What happened to one request, in order, from its saved call record:
 * received, rejected before sending or routed (each relay
 * attempt, for administrators), the upstream result and settlement. Steps
 * carry no invented timestamps; only the record time is known.
 */
export function DiagnosisTimeline(props: { log: UsageLog; isAdmin: boolean }) {
  const { t } = useTranslation()
  const log = props.log
  const other = parseLogOther(log.other)
  const facts = getRequestFacts(log)
  const rejected =
    other?.preflight_rejected === true || other?.request_outcome === 'rejected'
  const attempts = props.isAdmin
    ? (other?.admin_info?.relay_attempts ?? [])
    : []

  let settlementTone: Tone = 'neutral'
  if (facts.settlement.variant === 'success') settlementTone = 'ok'
  if (facts.settlement.variant === 'warning') settlementTone = 'warn'

  let routing: ReactNode
  if (attempts.length > 0) {
    routing = (
      <Step tone='neutral' title={t('Routing')}>
        <p>
          {t(
            'Attempts are shown in recorded order. Selected means current at log time, not confirmed completion. Key indexes start at 0 and may change when keys are reordered.'
          )}
        </p>
        <ol
          aria-label={t('Relay attempts')}
          className='mt-1 flex min-w-0 flex-col gap-3'
        >
          {attempts.map((attempt, index) => (
            // Log snapshots are immutable ordered evidence with no attempt ID.
            // oxlint-disable-next-line react/no-array-index-key
            <li key={index} className={attemptItemClass}>
              <p className='text-foreground flex flex-wrap items-center gap-2 font-medium'>
                <span
                  aria-hidden='true'
                  className={cn(
                    'size-2 rounded-full',
                    dotTone[attemptTone(attempt.outcome)]
                  )}
                />
                {t('Attempt {{number}}', { number: index + 1 })}
              </p>
              <dl className='flex min-w-0 flex-col gap-0.5'>
                <Fact
                  label={t('Connection ID')}
                  value={String(attempt.channel_id)}
                />
                <Fact
                  label={t('Key index (0-based)')}
                  value={String(attempt.key_index)}
                />
                <Fact
                  label={t('Upstream model')}
                  value={attempt.upstream_model}
                />
                <Fact
                  label={t('Outcome')}
                  value={t(relayAttemptOutcomeLabel(attempt.outcome))}
                />
                {attempt.status != null && attempt.status > 0 && (
                  <Fact
                    label={t('Failure HTTP status')}
                    value={String(attempt.status)}
                  />
                )}
              </dl>
              {attempt.cooldown_seconds != null &&
                attempt.cooldown_seconds > 0 && (
                  <>
                    <p className='text-foreground'>
                      {t('Future-request account hold: {{seconds}} seconds', {
                        seconds: attempt.cooldown_seconds,
                      })}
                    </p>
                    <p className='text-muted-foreground'>
                      {t(
                        "This temporary hold is recorded for later requests. It does not prove quota exhaustion or extend this request's retry window."
                      )}
                    </p>
                  </>
                )}
            </li>
          ))}
        </ol>
      </Step>
    )
  } else {
    routing = (
      <Step tone='neutral' title={t('Routing')}>
        <p>
          {props.isAdmin
            ? t('No routing attempts were recorded.')
            : t('Routing details are visible to administrators only.')}
        </p>
      </Step>
    )
  }

  const status =
    facts.httpStatus === null
      ? t('No HTTP status recorded')
      : t('HTTP {{status}}', { status: facts.httpStatus })
  const firstByte =
    log.is_stream && other?.frt != null && other.frt > 0
      ? t('First byte {{seconds}} s', {
          seconds: Number((other.frt / 1000).toFixed(2)),
        })
      : null

  return (
    <ol
      aria-label={t('Diagnosis timeline')}
      className='border-border ms-1.5 flex min-w-0 flex-col border-s-2'
    >
      <Step tone='ok' title={t('Request received')} />
      {rejected ? (
        <Step
          tone='warn'
          title={t('Rejected before sending; nothing was sent upstream')}
        >
          {props.isAdmin && other?.reject_reason && (
            <p>{other.reject_reason}</p>
          )}
        </Step>
      ) : (
        <>
          {routing}
          <Step
            tone={facts.completed ? 'ok' : 'fail'}
            title={
              facts.completed ? t('Upstream completed') : t('Request failed')
            }
          >
            <p>
              {status}
              {firstByte && <> · {firstByte}</>}
            </p>
            {other?.upstream_error_code && (
              <p>
                {t('Upstream error code')}:{' '}
                <code>{other.upstream_error_code}</code>
              </p>
            )}
          </Step>
        </>
      )}
      <Step tone={settlementTone} title={t(facts.settlement.label)}>
        <p>{t(facts.usage.label)}</p>
      </Step>
    </ol>
  )
}
