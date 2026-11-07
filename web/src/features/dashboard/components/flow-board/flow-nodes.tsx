/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { QuotaRing } from '@/components/quota-ring'
import { SummaryDisclosure } from '@/components/summary-disclosure'
import { CHANNEL_STATUS } from '@/features/channels/constants'
import { getChannelTypeLabel } from '@/features/channels/lib/channel-utils'
import {
  formatQuotaDuration,
  quotaWindowLabel,
} from '@/features/channels/lib/quota-history'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import {
  connectionKind,
  connectionValues,
  type ConnectionSummary,
} from '@/features/connections/connection-state'
import { API_KEY_STATUS, API_KEY_STATUSES } from '@/features/keys/constants'
import {
  describeNativeWindowState,
  nativeWindowState,
  nativeWindowUsage,
} from '@/features/personal-upstream/window-state'
import { linkButtonClassName } from '@/lib/link-button'
import { cn } from '@/lib/utils'

import type { FlowKeyNode, FlowTraffic } from '../../lib/overview-flow'

const nodeClassName =
  'bg-background hover:border-input relative grid w-full min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 rounded-lg border px-3.5 py-3 text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary aria-pressed:border-primary aria-pressed:ring-1 aria-pressed:ring-primary aria-pressed:ring-inset'
// Desktop connectors: a 48px line across the column gap to the core card.
const keyLine =
  "lg:after:border-line lg:after:pointer-events-none lg:after:absolute lg:after:top-1/2 lg:after:left-full lg:after:ml-px lg:after:w-12 lg:after:border-t-2 lg:after:content-['']"
const connectionLine =
  "lg:before:border-line lg:before:pointer-events-none lg:before:absolute lg:before:top-1/2 lg:before:right-full lg:before:mr-px lg:before:w-12 lg:before:border-t-2 lg:before:content-['']"

function StatusDot(props: { tone: 'success' | 'warning' | 'danger' | 'off' }) {
  return (
    <span
      aria-hidden='true'
      className={cn(
        'size-2 shrink-0 rounded-full',
        props.tone === 'success' && 'bg-success',
        props.tone === 'warning' && 'bg-warning',
        props.tone === 'danger' && 'bg-destructive',
        props.tone === 'off' && 'bg-muted-foreground'
      )}
    />
  )
}

export function KeyNode(props: {
  node: FlowKeyNode
  selected: boolean
  onToggle: () => void
}) {
  const { t } = useTranslation()
  const node = props.node
  const status =
    node.status === null ? undefined : API_KEY_STATUSES[node.status]
  let tone: 'success' | 'warning' | 'danger' | 'off' = 'off'
  if (node.status === API_KEY_STATUS.ENABLED) tone = 'success'
  else if (node.status === API_KEY_STATUS.EXPIRED) tone = 'warning'
  else if (node.status === API_KEY_STATUS.EXHAUSTED) tone = 'danger'
  let state = <span>{t('Status not provided')}</span>
  if (node.failures !== null && node.failures > 0) {
    state = (
      <span className='text-destructive'>
        {t('Failed {{count}}', { count: node.failures })}
      </span>
    )
  } else if (!node.own) {
    state = <span>{t('Other user')}</span>
  } else if (status) {
    state = <span>{t(status.label)}</span>
  }
  return (
    <button
      type='button'
      aria-pressed={props.selected}
      data-flow-node={`key-${node.id}`}
      onClick={props.onToggle}
      className={cn(nodeClassName, keyLine)}
    >
      <span className='text-body flex min-w-0 items-center gap-2 font-semibold'>
        <StatusDot tone={tone} />
        <span className='truncate' title={node.name}>
          {node.name || t('Unknown Key')}
        </span>
      </span>
      <span className='text-caption text-muted-foreground text-right tabular-nums'>
        {node.requests === null
          ? t('Unknown')
          : t('{{count}} requests', { count: node.requests })}
      </span>
      <span className='text-caption text-muted-foreground font-mono'>
        #{node.id}
      </span>
      <span className='text-caption text-muted-foreground text-right'>
        {state}
      </span>
    </button>
  )
}

export function ConnectionNode(props: {
  connection: ConnectionSummary
  window: ChannelQuotaChangeItem | undefined
  traffic: FlowTraffic | undefined
  /** False when the call records could not be read. */
  trafficKnown: boolean
  selected: boolean
  onToggle: () => void
  /** Present only when the parent grants the Full saved-settings tools. */
  onOpenRouting?: (id: number) => void
}) {
  const { t } = useTranslation()
  const connection = props.connection
  const window = props.window
  const used = window ? nativeWindowUsage(window) : null
  const enabled = connection.status === CHANNEL_STATUS.ENABLED
  let tone: 'success' | 'warning' | 'danger' | 'off' = 'off'
  if (enabled) tone = 'success'
  else if (connection.status === CHANNEL_STATUS.AUTO_DISABLED) tone = 'warning'
  let windowLabel = ''
  if (window) {
    windowLabel =
      window.window_seconds && window.window_seconds > 0
        ? formatQuotaDuration(window.window_seconds, t)
        : quotaWindowLabel(window.window_type, t)
  }

  let kind = t('Metered')
  const connectionType = connectionKind(connection, Boolean(window))
  if (connectionType === 'subscription') {
    kind = t('Subscription')
  } else if (connectionType === 'local') {
    kind = t('Local')
  }
  let aside = <span className='text-caption text-muted-foreground'>{kind}</span>
  if (used !== null) {
    aside = (
      <QuotaRing
        used={used}
        label={t('{{window}}: {{used}}% used, {{left}}% left', {
          window: windowLabel,
          used: Math.round(used),
          left: Math.round(100 - used),
        })}
      />
    )
  } else if (window) {
    aside = (
      <span className='text-caption text-warning'>
        {
          describeNativeWindowState(
            nativeWindowState(window, Date.now() / 1000),
            t
          ).label
        }
      </span>
    )
  }

  let state = <span>{t('Enabled')}</span>
  if (connection.status === CHANNEL_STATUS.MANUAL_DISABLED) {
    state = <span>{t('Disabled')}</span>
  } else if (connection.status === CHANNEL_STATUS.AUTO_DISABLED) {
    state = <span className='text-warning'>{t('Auto Disabled')}</span>
  } else if (!enabled) {
    state = <span>{t('Status not provided')}</span>
  } else if (used !== null) {
    state = (
      <span>
        {t('{{window}} · {{value}}% left', {
          window: windowLabel,
          value: Math.round(100 - used),
        })}
      </span>
    )
  } else if (props.trafficKnown && (props.traffic?.requests ?? 0) > 0) {
    state = (
      <span>
        {t('In use · {{count}} recent', {
          count: props.traffic?.requests ?? 0,
        })}
      </span>
    )
  }

  const filterButton = (
    <button
      type='button'
      aria-pressed={props.selected}
      data-flow-node={`connection-${connection.id}`}
      onClick={props.onToggle}
      className={cn(
        nodeClassName,
        !props.onOpenRouting && connectionLine,
        !props.onOpenRouting && !enabled && 'lg:before:border-dashed',
        props.onOpenRouting && 'rounded-b-none border-b-0'
      )}
    >
      <span className='text-body flex min-w-0 items-center gap-2 font-semibold'>
        <StatusDot tone={tone} />
        <span className='truncate' title={connection.name}>
          {connection.name || `#${connection.id}`}
        </span>
      </span>
      <span className='flex justify-end'>{aside}</span>
      <span className='text-caption text-muted-foreground flex min-w-0 flex-wrap gap-x-1'>
        <span className='truncate'>
          {t(getChannelTypeLabel(connection.type))}
        </span>
        {!props.onOpenRouting && connection.priority != null && (
          <span>
            · {t('Priority {{value}}', { value: connection.priority })}
          </span>
        )}
      </span>
      <span className='text-caption text-muted-foreground text-right'>
        {state}
      </span>
    </button>
  )
  if (!props.onOpenRouting) {
    return filterButton
  }

  const models = connectionValues(connection.models)
  let modelSummary = (
    <p className='text-caption text-muted-foreground'>
      {t('Models')}: {t('Unknown')}
    </p>
  )
  if (connection.models != null) {
    if (models.length === 0) {
      modelSummary = (
        <div className='text-caption text-muted-foreground flex min-w-0 flex-col gap-1'>
          <p>{t('Models ({{count}})', { count: 0 })}</p>
          <p>{t('No model IDs are saved for this connection.')}</p>
        </div>
      )
    } else {
      modelSummary = (
        <SummaryDisclosure
          summary={t('Models ({{count}})', { count: models.length })}
        >
          <ul className='text-caption min-w-0 space-y-1 font-mono wrap-anywhere'>
            {models.map((model) => (
              <li key={model}>{model}</li>
            ))}
          </ul>
        </SummaryDisclosure>
      )
    }
  }
  return (
    <article
      aria-label={t('Connection #{{id}}', { id: connection.id })}
      className={cn(
        'relative min-w-0',
        connectionLine,
        !enabled && 'lg:before:border-dashed'
      )}
    >
      {filterButton}
      <div className='bg-background flex min-w-0 flex-col gap-3 rounded-b-lg border border-t-0 px-3.5 pt-1 pb-3'>
        <dl className='text-caption flex min-w-0 flex-wrap gap-x-4 gap-y-1'>
          <div className='flex min-w-0 flex-wrap gap-x-1'>
            <dt className='text-muted-foreground'>{t('Priority')}</dt>
            <dd className='tabular-nums'>
              {connection.priority ?? t('Unknown')}
            </dd>
          </div>
          <div className='flex min-w-0 flex-wrap gap-x-1'>
            <dt className='text-muted-foreground'>{t('Weight')}</dt>
            <dd className='tabular-nums'>
              {connection.weight ?? t('Unknown')}
            </dd>
          </div>
        </dl>
        <div className='flex min-w-0 flex-wrap items-start gap-x-2 gap-y-1'>
          <div className='max-w-full min-w-0 has-[[data-slot=collapsible][data-open]]:basis-full'>
            {modelSummary}
          </div>
          <Link
            to='/connections/$connectionId'
            params={{ connectionId: String(connection.id) }}
            search={{ tab: 'routing' }}
            state={{ inspectorBack: true }}
            data-overview-routing-link={connection.id}
            className={linkButtonClassName({
              variant: 'outline',
              size: 'sm',
              className:
                'h-auto min-h-11 max-w-full self-start whitespace-normal text-left',
            })}
            onClick={(event) => {
              if (
                event.defaultPrevented ||
                event.button !== 0 ||
                event.metaKey ||
                event.ctrlKey ||
                event.shiftKey ||
                event.altKey ||
                (event.currentTarget.target !== '' &&
                  event.currentTarget.target !== '_self')
              ) {
                return
              }
              props.onOpenRouting?.(connection.id)
            }}
          >
            {t('Routing and models')}
          </Link>
        </div>
      </div>
    </article>
  )
}
