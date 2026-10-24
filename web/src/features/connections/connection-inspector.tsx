/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Edit02Icon, TestTube01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import type { Ref } from 'react'
import { useTranslation } from 'react-i18next'

import { InspectorHeader } from '@/components/inspector/inspector-layout'
import { StatusBadge } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { CHANNEL_STATUS_CONFIG } from '@/features/channels/constants'
import { getChannelTypeLabel } from '@/features/channels/lib/channel-utils'
import { INSPECTOR_TABS_LIST_CLASS } from '@/lib/console-layout'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  connectionValues,
  type ConnectionInspectorTab,
  type ConnectionSummary,
  type ConnectionsSearch,
} from './connection-state'
import { ConnectionWindows } from './connection-windows'
import type {
  ConnectionTestResult,
  useConnectionTest,
} from './use-connection-test'

export function ConnectionStatus(props: { status: number }) {
  const { t } = useTranslation()
  const config =
    CHANNEL_STATUS_CONFIG[props.status as keyof typeof CHANNEL_STATUS_CONFIG] ??
    CHANNEL_STATUS_CONFIG[0]
  return (
    <StatusBadge
      variant={config.variant}
      showDot
      copyable={false}
      className='h-auto min-h-5 whitespace-normal'
    >
      {t(config.label)}
    </StatusBadge>
  )
}

function formatDuration(milliseconds: number) {
  return milliseconds >= 1000
    ? `${(milliseconds / 1000).toFixed(2)} s`
    : `${Math.max(1, Math.round(milliseconds))} ms`
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
        ? ` · ${formatDuration(props.item.response_time)}`
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
    <div aria-live='polite' className='min-w-0'>
      {result?.success && (
        <Alert>
          <AlertTitle>{t('Test succeeded')}</AlertTitle>
          {result.responseTime !== undefined && (
            <AlertDescription>
              {t('Response time: {{duration}}', {
                duration: formatDuration(result.responseTime),
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

function Routing(props: { item: ConnectionSummary }) {
  const { t } = useTranslation()
  const models = connectionValues(props.item.models)
  const groups = connectionValues(props.item.group)
  return (
    <section
      aria-label={t('Routing configuration')}
      className='flex flex-col gap-3'
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
                    className='bg-muted text-data max-w-full rounded-md px-2 py-1 wrap-anywhere'
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
    </section>
  )
}

/**
 * One connection: status and the last saved test in the header, the
 * explicit Test and Edit actions, then overview, account windows and
 * routing tabs. Opening it reads saved configuration only.
 */
export function ConnectionInspector(props: {
  connectionId: number
  search: ConnectionsSearch
  detail: {
    isPending: boolean
    isError: boolean
    isSuccess: boolean
    refetch: () => unknown
  }
  item: ConnectionSummary | undefined
  identity: string
  canWrite: boolean
  canOperate: boolean
  showKeys: boolean
  showLogs: boolean
  isRoot: boolean
  test: ReturnType<typeof useConnectionTest>
  editButtonRef: Ref<HTMLButtonElement>
  onEdit: () => void
  onTabChange: (tab: ConnectionInspectorTab) => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const item = props.detail.isSuccess ? props.item : undefined
  const tab = props.search.tab ?? 'overview'
  const testing = props.test.running === props.connectionId

  return (
    <div className='flex min-w-0 flex-col' data-testid='connection-detail'>
      <InspectorHeader
        focusKey={`connection:${props.connectionId}`}
        onClose={props.onClose}
        title={
          item?.name || t('Connection #{{id}}', { id: props.connectionId })
        }
        subtitle={
          <span className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1'>
            <span className='font-mono'>#{props.connectionId}</span>
            {item && <span>{t(getChannelTypeLabel(item.type))}</span>}
            {item && <ConnectionStatus status={item.status} />}
          </span>
        }
        actions={
          item ? (
            <>
              {props.canOperate && (
                <Button
                  disabled={props.test.running !== null}
                  onClick={() => void props.test.run(item)}
                >
                  {testing ? (
                    <Spinner data-icon='inline-start' />
                  ) : (
                    <HugeiconsIcon
                      icon={TestTube01Icon}
                      data-icon='inline-start'
                      aria-hidden='true'
                    />
                  )}
                  {testing ? t('Testing...') : t('Test')}
                </Button>
              )}
              {props.canWrite && (
                <Button
                  ref={props.editButtonRef}
                  variant='outline'
                  onClick={props.onEdit}
                >
                  <HugeiconsIcon
                    icon={Edit02Icon}
                    data-icon='inline-start'
                    aria-hidden='true'
                  />
                  {t('Edit connection')}
                </Button>
              )}
            </>
          ) : undefined
        }
      />
      <div className='p-card flex min-w-0 flex-col gap-4'>
        {props.detail.isPending && (
          <div className='flex flex-col gap-2' aria-busy='true'>
            <span className='sr-only'>{t('Loading...')}</span>
            <Skeleton className='h-8 w-full' />
            <Skeleton className='h-32 w-full' />
          </div>
        )}
        {props.detail.isError && (
          <Alert variant='destructive'>
            <AlertTitle>
              {t(
                'Unable to load this connection. It may have been removed or access may have changed.'
              )}
            </AlertTitle>
            <AlertDescription>
              <Button
                variant='outline'
                size='sm'
                onClick={() => void props.detail.refetch()}
              >
                {t('Retry')}
              </Button>
            </AlertDescription>
          </Alert>
        )}
        {item && (
          <>
            <ConnectionTestOutcome
              result={props.test.result}
              connectionId={item.id}
            />
            <Tabs
              value={tab}
              onValueChange={(value) =>
                props.onTabChange(value as ConnectionInspectorTab)
              }
            >
              <TabsList className={INSPECTOR_TABS_LIST_CLASS}>
                <TabsTrigger value='overview'>{t('Overview')}</TabsTrigger>
                <TabsTrigger value='windows'>
                  {t('Account windows')}
                </TabsTrigger>
                <TabsTrigger value='routing'>{t('Routing')}</TabsTrigger>
              </TabsList>
              <TabsContent
                value='overview'
                className='flex min-w-0 flex-col gap-4 pt-2'
              >
                <dl className='grid min-w-0 grid-cols-2 gap-3'>
                  <div className='min-w-0'>
                    <dt className='text-caption text-muted-foreground'>
                      {t('Last test')}
                    </dt>
                    <dd className='text-data'>
                      <LastTest item={item} />
                    </dd>
                  </div>
                  <div className='min-w-0'>
                    <dt className='text-caption text-muted-foreground'>
                      {t('Type')}
                    </dt>
                    <dd className='text-data wrap-anywhere'>
                      {t(getChannelTypeLabel(item.type))}
                    </dd>
                  </div>
                </dl>
                <p className='text-caption text-muted-foreground'>
                  {t(
                    'Configuration status does not confirm upstream availability.'
                  )}
                </p>
                <div className='flex min-w-0 flex-wrap gap-2'>
                  {props.showLogs && (
                    <Link
                      to='/usage-logs/$section'
                      params={{ section: 'common' }}
                      search={{
                        channel: String(item.id),
                        connectionContext: {
                          connection: { ...props.search, connection: item.id },
                        },
                        page: 1,
                        pageSize: 20,
                      }}
                      data-myapi-button=''
                      className={cn(
                        buttonVariants({ variant: 'outline' }),
                        'h-auto min-h-9 whitespace-normal'
                      )}
                    >
                      {t('View connection requests')}
                    </Link>
                  )}
                  {props.showKeys && props.isRoot && (
                    <Link
                      to='/keys'
                      search={{
                        connectionContext: {
                          connection: { ...props.search, connection: item.id },
                        },
                      }}
                      data-myapi-button=''
                      className={cn(
                        buttonVariants({ variant: 'ghost' }),
                        'h-auto min-h-9 whitespace-normal'
                      )}
                    >
                      {t('Associated Key shares')}
                    </Link>
                  )}
                </div>
              </TabsContent>
              <TabsContent value='windows' className='min-w-0 pt-2'>
                <ConnectionWindows
                  key={`${props.identity}:${item.id}`}
                  channelId={item.id}
                  identity={props.identity}
                />
              </TabsContent>
              <TabsContent value='routing' className='min-w-0 pt-2'>
                <Routing item={item} />
              </TabsContent>
            </Tabs>
          </>
        )}
      </div>
    </div>
  )
}
