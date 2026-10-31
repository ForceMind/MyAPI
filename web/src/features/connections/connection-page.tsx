/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import {
  Edit02Icon,
  MoreHorizontalIcon,
  TestTube01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { getRouteApi, Link, useNavigate } from '@tanstack/react-router'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useInspectorBack } from '@/components/inspector/inspector-history'
import { ObjectPage } from '@/components/object-page/object-page'
import {
  useRememberReturnFocus,
  useReturnFocus,
} from '@/components/object-page/return-focus'
import { StatTile } from '@/components/stat-tile'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ChannelsProvider } from '@/features/channels/components/channels-provider'
import { ChannelMutateDrawer } from '@/features/channels/components/drawers/channel-mutate-drawer'
import { CHANNEL_STATUS } from '@/features/channels/constants'
import { getChannelTypeLabel } from '@/features/channels/lib/channel-utils'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import { tightestWindowByConnection } from '@/features/dashboard/lib/overview-flow'
import { WindowShareObservation } from '@/features/personal-upstream/components/window-share-observation'
import { nativeWindowUsage } from '@/features/personal-upstream/window-state'
import { RecentRequestList } from '@/features/usage-logs/components/recent-request-list'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { OBJECT_PAGE_TABS_LIST_CLASS } from '@/lib/console-layout'
import { linkButtonClassName } from '@/lib/link-button'

import { windowName } from './connection-format'
import {
  ConnectionHistory,
  ConnectionKindBadge,
  ConnectionRouting,
  ConnectionStatusBadge,
  ConnectionTestOutcome,
  LastTest,
} from './connection-sections'
import {
  connectionKind,
  connectionsToTableSearch,
  type ConnectionPageTab,
  type ConnectionReturn,
  type ConnectionSummary,
} from './connection-state'
import { ConnectionWindows } from './connection-windows'
import { useConnectionSwitch } from './use-connection-switch'
import { useConnectionTest } from './use-connection-test'
import {
  useConnectionAccess,
  useConnectionDetail,
  useConnectionRequests,
  useConnectionStats,
  useConnectionTraffic,
  useConnectionWindows,
  type ConnectionAccess,
} from './use-connections'

const route = getRouteApi('/_authenticated/connections/$connectionId/')

/**
 * One connection as a page (design v2 upstream): name, state and kind, the
 * switch, Test, Edit and more, three figures, then its windows (with Key
 * shares), routing and models, requests and history as tabs. The tab is in
 * the URL; Back returns to the grid with its filters, page and focus on the
 * card. Opening it reads saved configuration, observations and call records;
 * only Test contacts the provider.
 */
export function ConnectionPage() {
  const params = route.useParams()
  const connectionId = Number(params.connectionId)
  const search = route.useSearch()
  const navigate = useNavigate()
  const goBack = useInspectorBack()
  const access = useConnectionAccess()
  const connectionReturn = search.connectionReturn
  const backToList = useCallback(
    () =>
      goBack(() => {
        if (connectionReturn?.view === 'table') {
          void navigate({
            to: '/channels',
            search: {
              ...connectionsToTableSearch(connectionReturn),
              focusConnectionId: connectionId,
            },
            replace: true,
          })
          return
        }
        void navigate({
          to: '/connections',
          search: { ...connectionReturn, focusConnectionId: connectionId },
          replace: true,
        })
      }),
    [goBack, navigate, connectionReturn, connectionId]
  )
  return (
    <ConnectionPageContent
      key={`${access.identity}:${connectionId}`}
      access={access}
      connectionId={connectionId}
      tab={search.tab ?? 'windows'}
      connectionReturn={connectionReturn}
      onBack={backToList}
    />
  )
}

function ConnectionPageContent(props: {
  access: ConnectionAccess
  connectionId: number
  tab: ConnectionPageTab
  connectionReturn: ConnectionReturn | undefined
  onBack: () => void
}) {
  const { t } = useTranslation()
  const navigate = route.useNavigate()
  const access = props.access
  const detail = useConnectionDetail(access, props.connectionId)
  const windows = useConnectionWindows(access, [props.connectionId])
  const stats = useConnectionStats(access, props.connectionId)
  const requests = useConnectionRequests(access, props.connectionId)
  const traffic = useConnectionTraffic(access)
  const test = useConnectionTest(access.identity)
  const power = useConnectionSwitch()
  const rememberFocus = useRememberReturnFocus()
  const showLogs = useIsSidebarModuleVisible('/usage-logs/common')
  const showTable = useIsSidebarModuleVisible('/channels')
  const showKeys = useIsSidebarModuleVisible('/keys')
  const [editing, setEditing] = useState(false)
  const editButton = useRef<HTMLButtonElement>(null)
  // Back from a request puts focus on its row.
  useReturnFocus({
    attribute: 'data-request-row',
    settled: !requests.isFetching && !detail.isFetching,
    rows: detail.data ? requests.data : undefined,
  })
  // A lost grant closes the editor.
  useEffect(() => {
    if (!access.canWrite) setEditing(false)
  }, [access.canWrite])

  const item = detail.isError ? undefined : detail.data
  const title =
    item?.name || t('Connection #{{id}}', { id: props.connectionId })
  const savedWindows = windows.isError ? undefined : windows.data
  const testing = test.running === props.connectionId
  const listContext = {
    connection: { ...props.connectionReturn, connection: props.connectionId },
  }

  const openRequest = (log: UsageLog) => {
    if (!log.request_id) return
    rememberFocus('data-request-row', log.id)
    void navigate({
      to: '/usage-logs/request/$requestId',
      params: { requestId: log.request_id },
      state: { inspectorBack: true },
    })
  }

  let body
  if (access.self.isFetching && !access.canRead) {
    body = (
      <p role='status' className='text-body text-muted-foreground'>
        {t('Loading...')}
      </p>
    )
  } else if (!access.canRead) {
    body = (
      <Alert variant='destructive'>
        <AlertTitle>{t('Connection access could not be verified.')}</AlertTitle>
        <AlertDescription>
          <Button
            variant='outline'
            size='sm'
            onClick={() => void access.self.refetch()}
          >
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else if (detail.isPending) {
    body = (
      <div className='flex flex-col gap-3' aria-busy='true'>
        <span className='sr-only'>{t('Loading...')}</span>
        <Skeleton className='h-24 w-full' />
        <Skeleton className='h-64 w-full' />
      </div>
    )
  } else if (!item) {
    body = (
      <Alert variant='destructive'>
        <AlertTitle>
          {t(
            'Unable to load this connection. It may have been removed or access may have changed.'
          )}
        </AlertTitle>
        <AlertDescription className='flex flex-wrap gap-2'>
          <Button
            variant='outline'
            size='sm'
            disabled={detail.isFetching}
            onClick={() => void detail.refetch()}
          >
            {t('Retry')}
          </Button>
          {props.connectionReturn?.view === 'table' ? (
            <Link
              to='/channels'
              search={connectionsToTableSearch(props.connectionReturn)}
              data-myapi-button=''
              className={linkButtonClassName({ variant: 'ghost', size: 'sm' })}
            >
              {t('Table view')}
            </Link>
          ) : (
            <Link
              to='/connections'
              search={props.connectionReturn ?? {}}
              data-myapi-button=''
              className={linkButtonClassName({ variant: 'ghost', size: 'sm' })}
            >
              {t('Back to connections')}
            </Link>
          )}
        </AlertDescription>
      </Alert>
    )
  } else {
    body = (
      <>
        <ConnectionTestOutcome result={test.result} connectionId={item.id} />
        <ConnectionStats
          item={item}
          windows={savedWindows}
          windowsPending={windows.isPending}
          stats={stats}
          showLogs={showLogs}
          listContext={listContext}
        />
        <Tabs
          value={props.tab}
          onValueChange={(value) =>
            void navigate({
              search: (previous) => ({
                ...previous,
                tab:
                  value === 'windows'
                    ? undefined
                    : (value as Exclude<ConnectionPageTab, 'windows'>),
              }),
              replace: true,
              state: (previous) => previous,
            })
          }
          className='min-w-0 gap-4'
        >
          <TabsList variant='line' className={OBJECT_PAGE_TABS_LIST_CLASS}>
            <TabsTrigger value='windows'>{t('Windows')}</TabsTrigger>
            <TabsTrigger value='routing'>{t('Routing and models')}</TabsTrigger>
            <TabsTrigger value='requests'>
              {t('Requests', { context: 'section' })}
            </TabsTrigger>
            <TabsTrigger value='history'>{t('History')}</TabsTrigger>
          </TabsList>
          <TabsContent value='windows' className='flex min-w-0 flex-col gap-4'>
            <ConnectionWindows query={windows} now={windows.now} />
            <WindowShareObservation
              channelId={item.id}
              canReadPoolUsage={access.isRoot}
            />
            {showKeys && access.isRoot && (
              <Link
                to='/keys'
                search={{ connectionContext: listContext }}
                data-myapi-button=''
                className={linkButtonClassName({ variant: 'outline' })}
              >
                {t('Associated Key shares')}
              </Link>
            )}
          </TabsContent>
          <TabsContent value='routing' className='min-w-0'>
            <ConnectionRouting item={item} showPreview={showTable} />
          </TabsContent>
          <TabsContent value='requests' className='min-w-0'>
            <RecentRequestList
              query={requests}
              sideColumn={{
                label: t('API Key'),
                value: (row) =>
                  row.token_name ||
                  (row.token_id ? `#${row.token_id}` : t('Unknown Key')),
              }}
              labels={{
                error: t('Could not load requests for this connection'),
                emptyTitle: t('No requests through this connection yet'),
                emptyDescription: t(
                  'Requests appear here once a Key is routed to this connection.'
                ),
                caption: t('Latest requests of this connection'),
              }}
              footer={
                showLogs && (
                  <Link
                    to='/usage-logs/$section'
                    params={{ section: 'common' }}
                    search={{
                      channel: String(item.id),
                      connectionContext: listContext,
                      page: 1,
                      pageSize: 20,
                    }}
                    data-myapi-button=''
                    className={linkButtonClassName({ variant: 'ghost' })}
                  >
                    {t('View connection requests')}
                  </Link>
                )
              }
              onOpen={openRequest}
            />
          </TabsContent>
          <TabsContent value='history' className='min-w-0'>
            <ConnectionHistory
              item={item}
              windows={savedWindows}
              windowsError={windows.isError}
              showFullHistory={showTable}
            />
          </TabsContent>
        </Tabs>
      </>
    )
  }

  return (
    <ObjectPage
      data-testid='connection-page'
      title={title}
      label={title}
      meta={
        item ? (
          <>
            <ConnectionStatusBadge
              status={item.status}
              recentRequests={
                traffic.data && !traffic.isError
                  ? (traffic.data.traffic.get(item.id)?.requests ?? 0)
                  : undefined
              }
            />
            <ConnectionKindBadge
              kind={connectionKind(item, (savedWindows?.length ?? 0) > 0)}
            />
            <span className='font-mono'>#{item.id}</span>
            <span>{t(getChannelTypeLabel(item.type))}</span>
          </>
        ) : undefined
      }
      actions={
        item ? (
          <>
            {access.canOperate && (
              <Switch
                size='lg'
                checked={item.status === CHANNEL_STATUS.ENABLED}
                disabled={power.switching !== null}
                onCheckedChange={(enabled) =>
                  void power.toggle(item.id, enabled)
                }
                aria-label={t('Enable {{name}}', { name: title })}
                className='me-1 after:-inset-y-2.5'
              />
            )}
            {access.canOperate && (
              <Button
                disabled={test.running !== null}
                onClick={() => void test.run(item)}
                title={t('Quick test')}
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
            {access.canWrite && (
              <Button
                ref={editButton}
                variant='outline'
                onClick={() => setEditing(true)}
              >
                <HugeiconsIcon
                  icon={Edit02Icon}
                  data-icon='inline-start'
                  aria-hidden='true'
                />
                {t('Edit connection')}
              </Button>
            )}
            {(showLogs || showTable) && (
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <Button
                      variant='ghost'
                      size='icon'
                      aria-label={t('More actions')}
                      className='pointer-coarse:min-w-11!'
                    />
                  }
                >
                  <HugeiconsIcon icon={MoreHorizontalIcon} aria-hidden='true' />
                </DropdownMenuTrigger>
                <DropdownMenuContent align='end' className='w-60'>
                  {showLogs && (
                    <DropdownMenuItem
                      render={
                        <Link
                          to='/usage-logs/$section'
                          params={{ section: 'common' }}
                          search={{
                            channel: String(item.id),
                            connectionContext: listContext,
                            page: 1,
                            pageSize: 20,
                          }}
                        />
                      }
                    >
                      {t('View connection requests')}
                    </DropdownMenuItem>
                  )}
                  {showTable && (
                    <DropdownMenuItem
                      render={
                        <Link
                          to='/channels'
                          search={{ filter: String(item.id) }}
                        />
                      }
                    >
                      {t('Open in table view')}
                    </DropdownMenuItem>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </>
        ) : undefined
      }
      navigation={{ close: props.onBack }}
    >
      <div className='flex min-w-0 flex-col gap-5'>{body}</div>
      {editing && access.canWrite && item ? (
        <ChannelsProvider key={`${access.identity}:${item.id}`}>
          <ChannelMutateDrawer
            open
            currentRow={{ id: item.id }}
            onOpenChange={(open) => {
              if (!open) {
                setEditing(false)
                editButton.current?.focus()
              }
            }}
          />
        </ChannelsProvider>
      ) : null}
    </ObjectPage>
  )
}

/**
 * Three figures from saved records: requests and request failures in the
 * last 24 hours (exact totals), and the account window closest to its limit
 * or, for a connection without windows, its last saved test. Unknown is
 * never written as 0.
 */
function ConnectionStats(props: {
  item: ConnectionSummary
  windows: ChannelQuotaChangeItem[] | undefined
  windowsPending: boolean
  stats: ReturnType<typeof useConnectionStats>
  showLogs: boolean
  listContext: { connection: ConnectionReturn & { connection: number } }
}) {
  const { t } = useTranslation()
  const figures = props.stats.isError ? undefined : props.stats.data
  const failed = figures?.failed ?? null
  const window = tightestWindowByConnection(props.windows ?? []).get(
    props.item.id
  )
  const used = window ? nativeWindowUsage(window) : null

  let third = (
    <StatTile
      label={t('Last test')}
      value={<LastTest item={props.item} />}
      detail={
        connectionKind(props.item, false) === 'local'
          ? t('Local server, no quota window')
          : t('Metered, no subscription window')
      }
      loading={props.windowsPending}
    />
  )
  if (window) {
    third = (
      <StatTile
        label={t('{{window}} left', { window: windowName(window, t) })}
        value={used === null ? t('Unknown') : `${Math.round(100 - used)}%`}
        detail={t('Closest to its limit among saved windows')}
      />
    )
  } else if (props.windows === undefined && !props.windowsPending) {
    third = (
      <StatTile
        label={t('Account window')}
        value={t('Unknown')}
        detail={t('Account windows could not be read')}
      />
    )
  }

  return (
    <dl className='grid min-w-0 grid-cols-1 gap-2 min-[360px]:grid-cols-3 sm:gap-3'>
      <StatTile
        label={t('Requests in 24 hours')}
        value={figures?.requests ?? t('Unknown')}
        loading={props.stats.isPending}
        detail={t('Successful and failed records')}
      />
      <StatTile
        label={t('Failed in 24 hours')}
        value={
          failed !== null && failed > 0 && props.showLogs ? (
            <Link
              to='/usage-logs/$section'
              params={{ section: 'common' }}
              search={{
                channel: String(props.item.id),
                type: ['5'],
                result: 'failed',
                startTime: (figures?.since ?? 0) * 1000,
                endTime: Date.now() + 3600 * 1000,
                connectionContext: props.listContext,
              }}
              className='inline-flex min-w-11 items-center underline-offset-4 hover:underline pointer-coarse:min-h-11'
            >
              {failed}
            </Link>
          ) : (
            (failed ?? t('Unknown'))
          )
        }
        loading={props.stats.isPending}
        danger={failed !== null && failed > 0}
        detail={t('Request failures only, without pending reviews')}
      />
      {third}
    </dl>
  )
}
