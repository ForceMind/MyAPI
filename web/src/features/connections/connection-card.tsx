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
import { SummaryDisclosure } from '@/components/summary-disclosure'
import { Button, buttonVariants } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { CHANNEL_STATUS } from '@/features/channels/constants'
import { ChannelRoutingFieldEditor } from '@/features/channels/components/channel-routing-field-editor'
import { getChannelTypeLabel } from '@/features/channels/lib/channel-utils'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import { tightestWindowByConnection } from '@/features/dashboard/lib/overview-flow'
import { nativeWindowUsage } from '@/features/personal-upstream/window-state'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

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
import { isHistoricalConnectionWindow } from './window-state'
import { ConnectionEvidenceSummary } from './connection-evidence-summary'

/**
 * One connection in the upstream grid (design v2 upstream): its ring or
 * kind icon, name, kind, priority and models, the saved windows, its state
 * and last test, and the enable switch and View/Test/Edit actions. Explicit
 * links open details; local disclosures and operations never navigate the card.
 */
export function ConnectionCard(props: {
  connection: ConnectionSummary
  windows: ChannelQuotaChangeItem[]
  windowsKnown: boolean
  windowsIncomplete: boolean
  windowsLoading: boolean
  now: number
  recentRequests: number | undefined
  connectionReturn: ConnectionReturn | undefined
  canOperate: boolean
  canEdit?: boolean
  routingReadVersion?: number
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
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  const connection = props.connection
  const name = connection.name || t('Connection #{{id}}', { id: connection.id })
  const kind = connectionKind(connection, props.windows.some((item) => item.metric_type === 'codex_rate_limit' && item.unit === 'percent'))
  const window = tightestWindowByConnection(props.windows).get(connection.id)
  const used = (props.windowsKnown || props.windowsIncomplete) && window ? nativeWindowUsage(window) : null
  const enabled = connection.status === CHANNEL_STATUS.ENABLED
  const inUse = enabled && (props.recentRequests ?? 0) > 0
  const models = connectionValues(connection.models)
  const groups = connectionValues(connection.group)
  const historicalCount = props.windows.filter((item) => isHistoricalConnectionWindow(item, props.windows, props.now)).length
  const hasBalance = props.windows.some((item) => item.metric_type === 'balance')
  let evidenceDetails = t('Window details ({{count}})', { count: props.windows.length })
  if (historicalCount > 0) {
    evidenceDetails = t('Window details ({{count}}), historical records ({{history}})', { count: props.windows.length, history: historicalCount })
  }
  if (hasBalance) evidenceDetails = t('Saved evidence details ({{count}})', { count: props.windows.length })
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
        'bg-card hover:border-input relative flex w-full min-w-0 flex-col gap-3 rounded-xl border p-4 transition-colors',
        'has-[[data-connection-link]:focus-visible]:outline-primary has-[[data-connection-link]:focus-visible]:outline-2 has-[[data-connection-link]:focus-visible]:outline-offset-2',
        inUse && 'border-primary ring-primary ring-1 ring-inset'
      )}
    >
      <div className='flex min-w-0 items-center gap-3'>
        {used !== null && window ? (
          <div className='flex shrink-0 flex-col items-center gap-0.5'>
          <QuotaRing
            used={used}
            label={t('{{window}}: {{used}}% used, {{left}}% left', {
              window: windowName(window, t),
              used: Math.round(used),
              left: Math.round(100 - used),
            })}
          />
          <span className='text-caption text-muted-foreground'>{t('Used')}</span>
          </div>
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
            {/* Keep the detail link separate from local disclosure controls. */}
            <Link
              to='/connections/$connectionId'
              params={{ connectionId: String(connection.id) }}
              search={{ connectionReturn: props.connectionReturn }}
              state={{ inspectorBack: true }}
              data-connection-link={connection.id}
              onClick={props.onOpen}
              className='outline-none underline-offset-4 hover:underline focus-visible:underline'
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
        {props.canOperate && (
          <div className='ml-auto flex min-h-11 items-center gap-2 pointer-fine:min-h-9'>
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
      </div>
      <dl className='flex min-w-0 flex-wrap items-start gap-x-4 gap-y-1 text-caption'>
        <div className='flex flex-wrap gap-x-1'>
          <dt className='text-muted-foreground'>{t('Priority')}</dt>
          <dd className='tabular-nums'><ChannelRoutingFieldEditor channelId={connection.id} field='priority' value={connection.priority} canWrite={props.canEdit === true} readVersion={props.routingReadVersion} /></dd>
        </div>
        <div className='flex flex-wrap gap-x-1'>
          <dt className='text-muted-foreground'>{t('Weight')}</dt>
          <dd className='tabular-nums'><ChannelRoutingFieldEditor channelId={connection.id} field='weight' value={connection.weight} canWrite={props.canEdit === true} readVersion={props.routingReadVersion} /></dd>
        </div>
        <div className='min-w-0 max-w-full has-[[data-slot=collapsible][data-open]]:basis-full'>
          <dt className='sr-only'>{t('Group')}</dt>
          <dd>
            {groups.length > 0 ? <SummaryDisclosure key={`${identity}:${connection.id}:groups`} summary={t('Groups ({{count}})', { count: groups.length })}>
              <p>{groups.join(', ')}</p>
            </SummaryDisclosure> : <p>{t('Group')}: {t('Unknown')}</p>}
          </dd>
        </div>
        <div className='min-w-0 max-w-full has-[[data-slot=collapsible][data-open]]:basis-full'>
          <dt className='sr-only'>{t('Models')}</dt>
          <dd>
            {models.length > 0 ? <SummaryDisclosure key={`${identity}:${connection.id}:models`} summary={t('Models ({{count}})', { count: models.length })}>
              <p>{models.join(', ')}</p>
            </SummaryDisclosure> : <p>{t('Models')}: {t('Unknown')}</p>}
          </dd>
        </div>
        <div className='min-w-0 basis-full'>
          <dt className='sr-only'>{hasBalance ? t('Saved quota evidence') : t('Saved account windows')}</dt>
          <dd className='mt-1 text-muted-foreground'>
            <ConnectionEvidenceSummary
              windows={props.windows}
              windowsKnown={props.windowsKnown}
              windowsIncomplete={props.windowsIncomplete}
              windowsLoading={props.windowsLoading}
              now={props.now}
              compact
              showResetTime
            />
            {(props.windowsKnown || props.windowsIncomplete) && props.windows.length > 0 && <SummaryDisclosure
              key={`${identity}:${connection.id}:windows`}
              summary={evidenceDetails}
            >
              <ConnectionEvidenceSummary windows={props.windows} windowsKnown={props.windowsKnown} windowsIncomplete={props.windowsIncomplete} windowsLoading={props.windowsLoading} now={props.now} showResetTime />
            </SummaryDisclosure>}
          </dd>
        </div>
        <div className='flex min-w-0 basis-full flex-wrap gap-x-2'>
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
