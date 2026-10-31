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
import { Button, buttonVariants } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { CHANNEL_STATUS } from '@/features/channels/constants'
import { getChannelTypeLabel } from '@/features/channels/lib/channel-utils'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import { tightestWindowByConnection } from '@/features/dashboard/lib/overview-flow'
import { nativeWindowUsage } from '@/features/personal-upstream/window-state'
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
import { describeConnectionWindow } from './window-state'

/** Every saved series keeps its own state and observation time. */
function WindowsLine(props: {
  connection: ConnectionSummary
  windows: ChannelQuotaChangeItem[]
  /** False when the saved windows could not be read. */
  windowsKnown: boolean
  windowsLoading: boolean
  now: number
}) {
  const { t } = useTranslation()
  const kind = connectionKind(props.connection, props.windows.length > 0)
  if (props.windowsLoading) {
    return <span>{t('Loading...')}</span>
  }
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
  return (
    <ul aria-label={t('Saved account windows')} className='flex min-w-0 flex-col gap-2'>
      {props.windows.map((item) => {
        const used = nativeWindowUsage(item)
        const state = describeConnectionWindow(item, props.windows, props.now, t)
        const summary =
          used === null
            ? t('{{window}}: {{state}}', {
                window: windowName(item, t),
                state: state.label,
              })
            : t('{{window}} · {{value}}% left', {
                window: windowName(item, t),
                value: Math.round(100 - used),
              })
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
            className='min-w-0 wrap-anywhere'
          >
            <span>
              {[summary, used === null ? null : state.label, time]
                .filter(Boolean)
                .join(' · ')}
            </span>
            {item.status === 'error' && (
              <p className='mt-1'>{state.description}</p>
            )}
          </li>
        )
      })}
    </ul>
  )
}

/**
 * One connection in the upstream grid (design v2 upstream): its ring or
 * kind icon, name, kind, priority and models, the saved windows, its state
 * and last test, and the enable switch and View/Test/Edit actions. The name is the
 * card's link (stretched over the card); the switch and Test sit above it,
 * so using them never opens the page.
 */
export function ConnectionCard(props: {
  connection: ConnectionSummary
  windows: ChannelQuotaChangeItem[]
  windowsKnown: boolean
  windowsLoading: boolean
  now: number
  recentRequests: number | undefined
  connectionReturn: ConnectionReturn | undefined
  canOperate: boolean
  canEdit?: boolean
  switching: boolean
  testing: boolean
  testBusy: boolean
  testResult: ConnectionTestResult | null
  onOpen: () => void
  onToggle: (enabled: boolean) => void
  onTest: () => void
  onEdit?: () => void
}) {
  const { t } = useTranslation()
  const connection = props.connection
  const name = connection.name || t('Connection #{{id}}', { id: connection.id })
  const kind = connectionKind(connection, props.windows.length > 0)
  const window = tightestWindowByConnection(props.windows).get(connection.id)
  const used = props.windowsKnown && window ? nativeWindowUsage(window) : null
  const enabled = connection.status === CHANNEL_STATUS.ENABLED
  const inUse = enabled && (props.recentRequests ?? 0) > 0
  const models = connectionValues(connection.models)
  const groups = connectionValues(connection.group)
  const subtitle = [
    `#${connection.id}`,
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
        'bg-card hover:border-input relative flex w-full min-w-0 flex-col gap-4 rounded-xl border p-4 transition-colors sm:p-5',
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
            className='text-body min-w-0 wrap-anywhere font-semibold'
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
            className='text-caption text-muted-foreground min-w-0 wrap-anywhere'
            title={subtitle}
          >
            {subtitle}
          </p>
        </div>
      </div>
      <div className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1'>
        <ConnectionStatusBadge
          status={connection.status}
          recentRequests={props.recentRequests}
        />
        <ConnectionKindBadge kind={kind} />
      </div>
      <dl className='grid min-w-0 grid-cols-2 gap-x-4 gap-y-3 text-caption'>
        <div>
          <dt className='text-muted-foreground'>{t('Priority')}</dt>
          <dd className='tabular-nums'>{connection.priority ?? t('Unknown')}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Weight')}</dt>
          <dd className='tabular-nums'>{connection.weight ?? t('Unknown')}</dd>
        </div>
        <div className='col-span-2 min-w-0'>
          <dt className='text-muted-foreground'>{t('Group')}</dt>
          <dd className='wrap-anywhere'>{groups.join(', ') || t('Unknown')}</dd>
        </div>
        <div className='col-span-2 min-w-0'>
          <dt className='text-muted-foreground'>{t('Models')}</dt>
          <dd className='wrap-anywhere'>{models.join(', ') || t('Unknown')}</dd>
        </div>
        <div className='col-span-2 min-w-0'>
          <dt className='text-muted-foreground'>{t('Saved account windows')}</dt>
          <dd className='mt-1 text-muted-foreground'>
            <WindowsLine
              connection={connection}
              windows={props.windows}
              windowsKnown={props.windowsKnown}
              windowsLoading={props.windowsLoading}
              now={props.now}
            />
          </dd>
        </div>
        <div className='col-span-2 min-w-0'>
          <dt className='text-muted-foreground'>{t('Last test')}</dt>
          <dd className='wrap-anywhere'>
            <LastTest item={connection} />
          </dd>
        </div>
      </dl>
      {props.testResult?.connectionId === connection.id && (
        <div className='relative z-10'>
          <ConnectionTestOutcome
            result={props.testResult}
            connectionId={connection.id}
          />
        </div>
      )}
      <div className='relative z-10 mt-auto flex min-w-0 flex-col gap-3'>
        <Separator />
        {props.canOperate && (
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
        )}
        <div className='flex min-w-0 flex-wrap items-center gap-2'>
          <Link
            to='/connections/$connectionId'
            params={{ connectionId: String(connection.id) }}
            search={{ connectionReturn: props.connectionReturn }}
            state={{ inspectorBack: true }}
            onClick={props.onOpen}
            data-myapi-button=''
            className={cn(
              buttonVariants({ variant: 'outline' }),
              'min-h-11 flex-1 sm:min-h-9'
            )}
          >
            {t('View')}
          </Link>
          {props.canOperate && (
            <Button
              variant='outline'
              disabled={props.testBusy}
              onClick={props.onTest}
              className='min-h-11 flex-1 sm:min-h-9'
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
          )}
          {props.canEdit && props.onEdit && (
            <Button
              variant='ghost'
              onClick={props.onEdit}
              data-connection-edit={connection.id}
              className='min-h-11 flex-1 sm:min-h-9'
            >
              {t('Edit')}
            </Button>
          )}
        </div>
      </div>
    </article>
  )
}
