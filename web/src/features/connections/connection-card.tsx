/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import {
  ComputerIcon,
  PlugSocketIcon,
  TestTube01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { QuotaRing } from '@/components/quota-ring'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { CHANNEL_STATUS } from '@/features/channels/constants'
import { getChannelTypeLabel } from '@/features/channels/lib/channel-utils'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import { tightestWindowByConnection } from '@/features/dashboard/lib/overview-flow'
import {
  describeNativeWindowState,
  nativeWindowState,
  nativeWindowUsage,
} from '@/features/personal-upstream/window-state'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { windowName } from './connection-format'
import {
  ConnectionKindBadge,
  ConnectionStatusBadge,
  ConnectionTestOutcome,
  LastTest,
} from './connection-sections'
import {
  connectionKind,
  connectionValues,
  type ConnectionReturn,
  type ConnectionSummary,
} from './connection-state'
import type { ConnectionTestResult } from './use-connection-test'

/** Every saved window of the connection in one line, or why there is none. */
function WindowsLine(props: {
  connection: ConnectionSummary
  windows: ChannelQuotaChangeItem[]
  /** False when the saved windows could not be read. */
  windowsKnown: boolean
}) {
  const { t } = useTranslation()
  const kind = connectionKind(props.connection, props.windows.length > 0)
  if (!props.windowsKnown) {
    return <span>{t('Account windows could not be read')}</span>
  }
  if (props.windows.length === 0) {
    if (kind === 'local') {
      return <span>{t('Local server, no quota window')}</span>
    }
    if (kind === 'api') {
      return <span>{t('Metered, no subscription window')}</span>
    }
    return <span>{t('No saved window sample')}</span>
  }
  const now = Date.now() / 1000
  const parts = props.windows.map((item) => {
    const used = nativeWindowUsage(item)
    return used === null
      ? t('{{window}}: {{state}}', {
          window: windowName(item, t),
          state: describeNativeWindowState(nativeWindowState(item, now), t)
            .label,
        })
      : t('{{window}} · {{value}}% left', {
          window: windowName(item, t),
          value: Math.round(100 - used),
        })
  })
  const observed = Math.max(
    ...props.windows.map((item) => item.observed_at ?? 0)
  )
  if (observed > 0) {
    parts.push(
      t('Observed {{time}}', { time: formatTimestampToDate(observed) })
    )
  }
  return <span>{parts.join(' · ')}</span>
}

/**
 * One connection in the upstream grid (design v2 upstream): its ring or
 * kind icon, name, kind, priority and models, the saved windows, its state
 * and last test, and the enable switch and Test on the card. The name is the
 * card's link (stretched over the card); the switch and Test sit above it,
 * so using them never opens the page.
 */
export function ConnectionCard(props: {
  connection: ConnectionSummary
  windows: ChannelQuotaChangeItem[]
  windowsKnown: boolean
  recentRequests: number | undefined
  connectionReturn: ConnectionReturn | undefined
  canOperate: boolean
  switching: boolean
  testing: boolean
  testBusy: boolean
  testResult: ConnectionTestResult | null
  onOpen: () => void
  onToggle: (enabled: boolean) => void
  onTest: () => void
}) {
  const { t } = useTranslation()
  const connection = props.connection
  const name = connection.name || t('Connection #{{id}}', { id: connection.id })
  const kind = connectionKind(connection, props.windows.length > 0)
  const window = tightestWindowByConnection(props.windows).get(connection.id)
  const used = window ? nativeWindowUsage(window) : null
  const enabled = connection.status === CHANNEL_STATUS.ENABLED
  const inUse = enabled && (props.recentRequests ?? 0) > 0
  const models = connectionValues(connection.models)
  // The provider comes last: a long provider name is what gets cut off.
  const subtitle = [
    connection.priority == null
      ? t('Priority unknown')
      : t('Priority {{value}}', { value: connection.priority }),
    t('{{count}} models', { count: models.length }),
    t(getChannelTypeLabel(connection.type)),
  ].join(' · ')
  let icon = PlugSocketIcon
  if (kind === 'local') icon = ComputerIcon

  return (
    <article
      data-testid='connection-card'
      data-connection-card={connection.id}
      aria-labelledby={`connection-card-${connection.id}`}
      className={cn(
        'bg-background hover:border-input relative flex w-full min-w-0 flex-col gap-3 rounded-xl border p-4 transition-colors',
        'has-[[data-connection-link]:focus-visible]:outline-primary has-[[data-connection-link]:focus-visible]:outline-2 has-[[data-connection-link]:focus-visible]:outline-offset-2',
        inUse && 'border-primary ring-primary ring-1 ring-inset'
      )}
    >
      <div className='flex min-w-0 items-center gap-3'>
        {used !== null && window ? (
          <QuotaRing
            used={used}
            label={t('{{window}}: {{used}}% used, {{left}}% left', {
              window: windowName(window, t),
              used: Math.round(used),
              left: Math.round(100 - used),
            })}
          />
        ) : (
          <span
            aria-hidden='true'
            className='bg-muted text-muted-foreground grid size-11 shrink-0 place-items-center rounded-full border'
          >
            <HugeiconsIcon icon={icon} className='size-5' />
          </span>
        )}
        <div className='flex min-w-0 flex-1 flex-col'>
          <h2
            id={`connection-card-${connection.id}`}
            className='text-body min-w-0 truncate font-semibold'
            title={name}
          >
            {/* The stretched link makes the whole card open the page. */}
            <Link
              to='/connections/$connectionId'
              params={{ connectionId: String(connection.id) }}
              search={{ connectionReturn: props.connectionReturn }}
              state={{ inspectorBack: true }}
              data-connection-link={connection.id}
              onClick={props.onOpen}
              className='outline-none after:absolute after:inset-0 after:rounded-xl after:content-[""]'
            >
              {name}
            </Link>
          </h2>
          <p
            className='text-caption text-muted-foreground min-w-0 truncate'
            title={subtitle}
          >
            {subtitle}
          </p>
        </div>
      </div>
      <p className='text-caption text-muted-foreground min-w-0 wrap-anywhere'>
        <WindowsLine
          connection={connection}
          windows={props.windows}
          windowsKnown={props.windowsKnown}
        />
      </p>
      <div className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1'>
        <ConnectionStatusBadge
          status={connection.status}
          recentRequests={props.recentRequests}
        />
        <ConnectionKindBadge kind={kind} />
        <span className='text-caption text-muted-foreground'>
          {t('Last test')}: <LastTest item={connection} />
        </span>
      </div>
      {props.testResult?.connectionId === connection.id && (
        <div className='relative z-10'>
          <ConnectionTestOutcome
            result={props.testResult}
            connectionId={connection.id}
          />
        </div>
      )}
      {props.canOperate && (
        <div className='relative z-10 mt-auto flex min-w-0 items-center gap-2 border-t pt-3'>
          <div className='flex min-h-11 items-center gap-2 pointer-fine:min-h-9'>
            {/* The hit area reaches 44px on touch screens. */}
            <Switch
              size='lg'
              checked={enabled}
              disabled={props.switching}
              onCheckedChange={props.onToggle}
              aria-label={t('Enable {{name}}', { name })}
              className='after:-inset-y-2.5'
            />
            <span
              aria-hidden='true'
              className='text-caption text-muted-foreground'
            >
              {enabled
                ? t('Enabled')
                : t('Disabled', { context: 'connection' })}
            </span>
          </div>
          <span className='flex-1' />
          <Button
            variant='outline'
            disabled={props.testBusy}
            onClick={props.onTest}
            className='pointer-coarse:min-h-11'
          >
            {props.testing ? (
              <Spinner data-icon='inline-start' />
            ) : (
              <HugeiconsIcon
                icon={TestTube01Icon}
                data-icon='inline-start'
                aria-hidden='true'
              />
            )}
            {props.testing ? t('Testing...') : t('Test')}
          </Button>
        </div>
      )}
    </article>
  )
}
