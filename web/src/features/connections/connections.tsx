/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { getRouteApi, Link } from '@tanstack/react-router'
import { ArrowLeft, ArrowRight, Search } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { ChannelsProvider } from '@/features/channels/components/channels-provider'
import { ChannelMutateDrawer } from '@/features/channels/components/drawers/channel-mutate-drawer'
import { CHANNEL_STATUS_CONFIG } from '@/features/channels/constants'
import { getChannelTypeLabel } from '@/features/channels/lib/channel-utils'
import { useMediaQuery } from '@/hooks/use-media-query'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'

import {
  CONNECTION_PAGE_SIZE,
  connectionValues,
  type ConnectionSummary,
  type ConnectionsSearch,
} from './connection-state'
import { ConnectionWindows } from './connection-windows'
import { useConnections } from './use-connections'

const route = getRouteApi('/_authenticated/connections/')

export function Connections() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  return (
    <ConnectionsWorkspace
      search={search}
      onSearch={(next) => void navigate({ search: next })}
    />
  )
}

function ConnectionStatus({ status }: { status: number }) {
  const { t } = useTranslation()
  const config =
    CHANNEL_STATUS_CONFIG[status as keyof typeof CHANNEL_STATUS_CONFIG] ??
    CHANNEL_STATUS_CONFIG[0]
  return (
    <StatusBadge
      variant={config.variant}
      copyable={false}
      className='h-auto min-h-5 whitespace-normal'
    >
      {t(config.label)}
    </StatusBadge>
  )
}

function Configuration({ item }: { item: ConnectionSummary }) {
  const { t } = useTranslation()
  const models = connectionValues(item.models)
  const groups = connectionValues(item.group)
  return (
    <section aria-label={t('Routing configuration')} className='space-y-3'>
      <h3 className='font-semibold'>{t('Routing configuration')}</h3>
      <dl className='grid min-w-0 grid-cols-2 gap-3 text-sm'>
        <div>
          <dt className='text-muted-foreground'>{t('Priority')}</dt>
          <dd>{item.priority ?? t('Unknown')}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Weight')}</dt>
          <dd>{item.weight ?? t('Unknown')}</dd>
        </div>
        <div className='col-span-2 min-w-0'>
          <dt className='text-muted-foreground'>{t('Group')}</dt>
          <dd className='[overflow-wrap:anywhere]'>
            {groups.join(', ') || t('Unknown')}
          </dd>
        </div>
        <div className='col-span-2 min-w-0'>
          <dt className='text-muted-foreground'>{t('Models')}</dt>
          <dd>
            {models.length ? (
              <ul className='mt-1 flex min-w-0 flex-wrap gap-2'>
                {models.map((model) => (
                  <li
                    key={model}
                    className='bg-muted max-w-full rounded px-2 py-1 [overflow-wrap:anywhere]'
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

export function ConnectionsWorkspace({
  search,
  onSearch,
}: {
  search: ConnectionsSearch
  onSearch: (next: ConnectionsSearch) => void
}) {
  const { t } = useTranslation()
  const data = useConnections(search)
  const mobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const showKeys = useIsSidebarModuleVisible('/keys')
  const showLogs = useIsSidebarModuleVisible('/usage-logs/common')
  const [filter, setFilter] = useState(search.filter ?? '')
  const [editor, setEditor] = useState<string | null>(null)
  const detailHeading = useRef<HTMLHeadingElement>(null)
  const editButton = useRef<HTMLButtonElement>(null)
  const lastSelection = useRef(search.connection)
  const pendingFocusReturn = useRef<number | undefined>(undefined)
  const inventoryHeading = useRef<HTMLHeadingElement>(null)
  const inventoryContext = JSON.stringify([
    data.identity,
    data.canRead,
    search.filter,
    search.status,
    search.page ?? 1,
  ])
  const lastInventoryContext = useRef(inventoryContext)
  const listRef = useRef<HTMLDivElement>(null)
  const selectedId = search.connection
  const editorIdentity = `${data.identity}:${selectedId}`
  const item = data.detail.isError ? undefined : data.detail.data
  useEffect(() => setFilter(search.filter ?? ''), [search.filter])
  useEffect(() => setEditor(null), [editorIdentity, data.canWrite])
  useEffect(() => {
    if (selectedId !== undefined) detailHeading.current?.focus()
    if (
      selectedId !== undefined ||
      lastInventoryContext.current !== inventoryContext
    ) {
      pendingFocusReturn.current = undefined
    } else if (lastSelection.current !== undefined) {
      pendingFocusReturn.current = lastSelection.current
    }
    lastSelection.current = selectedId
    lastInventoryContext.current = inventoryContext
  }, [selectedId, mobile, data.canRead, inventoryContext])
  useEffect(() => {
    const returnId = pendingFocusReturn.current
    if (returnId === undefined) return
    const row = listRef.current?.querySelector<HTMLButtonElement>(
      `[data-connection-id="${returnId}"]`
    )
    if (row || data.list.isSuccess || data.list.isError) {
      // A removed/filtered row or failed inventory has a stable, visible fallback.
      pendingFocusReturn.current = undefined
      const target = row ?? inventoryHeading.current
      target?.focus()
      return
    }
    // Keep the target while loading, but never override a newer user action.
    const cancelFocusReturn = () => {
      pendingFocusReturn.current = undefined
    }
    document.addEventListener('pointerdown', cancelFocusReturn, true)
    document.addEventListener('keydown', cancelFocusReturn, true)
    document.addEventListener('focusin', cancelFocusReturn, true)
    return () => {
      document.removeEventListener('pointerdown', cancelFocusReturn, true)
      document.removeEventListener('keydown', cancelFocusReturn, true)
      document.removeEventListener('focusin', cancelFocusReturn, true)
    }
  }, [
    selectedId,
    mobile,
    inventoryContext,
    data.list.data,
    data.list.isSuccess,
    data.list.isError,
  ])
  // Selection/session changes cannot reopen an editor with the previous target.
  const editing =
    data.canWrite && editor === editorIdentity && item?.id === selectedId
  const page = search.page ?? 1
  return (
    <main
      id='content'
      tabIndex={-1}
      className='pc-page flex min-w-0 flex-col gap-5'
      data-testid='connections-workspace'
    >
      <header className='flex min-w-0 flex-wrap items-start justify-between gap-3'>
        <div className='min-w-0'>
          <h1 className='text-2xl font-semibold tracking-tight'>
            {t('Connections')}
          </h1>
          <p className='text-muted-foreground mt-2 text-sm'>
            {t(
              'Inspect a connection, its routing and recorded requests without leaving your place.'
            )}
          </p>
        </div>
        {data.canRead ? (
          <Link
            to='/channels'
            className={`${buttonVariants({ variant: 'outline' })} h-auto min-h-11 max-w-full py-2 whitespace-normal`}
          >
            {t('Advanced channel management')}
          </Link>
        ) : null}
      </header>
      {data.self.isFetching && <p role='status'>{t('Loading...')}</p>}
      {!data.self.isFetching && !data.canRead && (
        <div role='alert' className='space-y-3'>
          <p>{t('Connection access could not be verified.')}</p>
          <Button variant='outline' onClick={() => void data.self.refetch()}>
            {t('Retry')}
          </Button>
        </div>
      )}
      {data.canRead && (
        <div className='grid min-w-0 items-start gap-5 lg:grid-cols-[minmax(260px,0.85fr)_minmax(0,1.4fr)]'>
          {!mobile || selectedId === undefined ? (
            <Card
              className='pc-card min-w-0'
              data-testid='connections-inventory'
            >
              <CardHeader>
                <CardTitle>
                  <h2 ref={inventoryHeading} tabIndex={-1}>
                    {t('Configured connections')}
                  </h2>
                </CardTitle>
                <form
                  className='mt-3 flex min-w-0 flex-wrap gap-2'
                  onSubmit={(event) => {
                    event.preventDefault()
                    onSearch({
                      ...search,
                      filter: filter.trim() || undefined,
                      page: 1,
                      connection: undefined,
                    })
                  }}
                >
                  <label className='min-w-0 flex-1 basis-40'>
                    <span className='sr-only'>{t('Search connections')}</span>
                    <Input
                      className='min-h-11'
                      value={filter}
                      onChange={(event) => setFilter(event.target.value)}
                      maxLength={200}
                      placeholder={t('Search connections')}
                    />
                  </label>
                  <Button
                    type='submit'
                    className='min-h-11'
                    aria-label={t('Search')}
                  >
                    <Search size={16} />
                    <span>{t('Search')}</span>
                  </Button>
                  <label className='flex min-w-0 basis-full flex-wrap items-center gap-2 text-sm'>
                    <span>{t('Status')}</span>
                    <select
                      aria-label={t('Status')}
                      className='border-input bg-background min-h-11 max-w-full flex-1 rounded-md border px-3'
                      value={search.status ?? ''}
                      onChange={(event) =>
                        onSearch({
                          ...search,
                          status:
                            (event.target
                              .value as ConnectionsSearch['status']) ||
                            undefined,
                          page: 1,
                          connection: undefined,
                        })
                      }
                    >
                      <option value=''>{t('All Status')}</option>
                      <option value='enabled'>{t('Enabled')}</option>
                      <option value='disabled'>{t('Disabled')}</option>
                    </select>
                  </label>
                </form>
              </CardHeader>
              <CardContent className='space-y-4'>
                {data.list.isPending && <p role='status'>{t('Loading...')}</p>}
                {data.list.isError && (
                  <div role='alert' className='space-y-3'>
                    <p>{t('Unable to load connections')}</p>
                    <Button
                      variant='outline'
                      onClick={() => void data.list.refetch()}
                    >
                      {t('Retry')}
                    </Button>
                  </div>
                )}
                {data.list.isSuccess && (
                  <>
                    <p
                      className='text-muted-foreground text-sm'
                      aria-live='polite'
                    >
                      {t('Connections: {{count}}', {
                        count: data.list.data.total,
                      })}
                    </p>
                    {data.list.data.items.length === 0 ? (
                      <p>{t('No channels found')}</p>
                    ) : (
                      <div ref={listRef} className='space-y-2'>
                        {data.list.data.items.map((connection) => (
                          <button
                            key={connection.id}
                            type='button'
                            data-connection-id={connection.id}
                            aria-pressed={selectedId === connection.id}
                            onClick={() =>
                              onSearch({ ...search, connection: connection.id })
                            }
                            className='border-border hover:bg-muted focus-visible:ring-ring aria-pressed:border-primary aria-pressed:bg-primary/5 flex min-h-20 w-full min-w-0 flex-col gap-2 rounded-lg border p-3 text-left focus-visible:ring-2 focus-visible:outline-none'
                          >
                            <span className='flex min-w-0 items-start justify-between gap-2'>
                              <span className='min-w-0 font-medium [overflow-wrap:anywhere]'>
                                {connection.name || `#${connection.id}`}
                              </span>
                              <ArrowRight
                                className='mt-1 size-4 shrink-0'
                                aria-hidden
                              />
                            </span>
                            <span className='flex flex-wrap items-center gap-2 text-xs'>
                              <span className='text-muted-foreground'>
                                #{connection.id} ·{' '}
                                {t(getChannelTypeLabel(connection.type))}
                              </span>
                              <ConnectionStatus status={connection.status} />
                            </span>
                          </button>
                        ))}
                      </div>
                    )}
                    <nav
                      aria-label={t('Pagination')}
                      className='flex flex-wrap items-center justify-between gap-2'
                    >
                      <Button
                        variant='outline'
                        className='min-h-11'
                        disabled={page <= 1}
                        onClick={() =>
                          onSearch({
                            ...search,
                            page: page - 1,
                            connection: undefined,
                          })
                        }
                      >
                        {t('Previous')}
                      </Button>
                      <span className='text-muted-foreground text-sm'>
                        {t('Page {{page}}', { page })}
                      </span>
                      <Button
                        variant='outline'
                        className='min-h-11'
                        disabled={
                          page * CONNECTION_PAGE_SIZE >= data.list.data.total
                        }
                        onClick={() =>
                          onSearch({
                            ...search,
                            page: page + 1,
                            connection: undefined,
                          })
                        }
                      >
                        {t('Next')}
                      </Button>
                    </nav>
                  </>
                )}
              </CardContent>
            </Card>
          ) : null}
          {!mobile || selectedId !== undefined ? (
            <Card className='pc-card min-w-0' data-testid='connection-detail'>
              <CardHeader className='space-y-3'>
                {selectedId !== undefined ? (
                  <Button
                    variant='ghost'
                    className='h-auto min-h-11 max-w-full self-start py-2 whitespace-normal'
                    onClick={() => {
                      setEditor(null)
                      onSearch({ ...search, connection: undefined })
                    }}
                  >
                    <ArrowLeft size={16} />
                    {t('Back to connections')}
                  </Button>
                ) : null}
                <CardTitle>
                  <h2
                    ref={detailHeading}
                    tabIndex={-1}
                    className='min-w-0 [overflow-wrap:anywhere]'
                  >
                    {t('Connection details')}
                    {selectedId ? ` · #${selectedId}` : ''}
                  </h2>
                </CardTitle>
              </CardHeader>
              <CardContent className='flex min-w-0 flex-col gap-6'>
                {selectedId === undefined && (
                  <p className='text-muted-foreground'>
                    {t(
                      'Select a connection to inspect its configuration and requests.'
                    )}
                  </p>
                )}
                {selectedId !== undefined && data.detail.isPending && (
                  <p role='status'>{t('Loading...')}</p>
                )}
                {selectedId !== undefined && data.detail.isError && (
                  <div role='alert' className='space-y-3'>
                    <p>
                      {t(
                        'Unable to load this connection. It may have been removed or access may have changed.'
                      )}
                    </p>
                    <Button
                      variant='outline'
                      onClick={() => void data.detail.refetch()}
                    >
                      {t('Retry')}
                    </Button>
                  </div>
                )}
                {selectedId !== undefined && data.detail.isSuccess && item && (
                  <>
                    <section className='min-w-0 space-y-3'>
                      <h3 className='text-xl font-semibold [overflow-wrap:anywhere]'>
                        {item.name || `#${item.id}`}
                      </h3>
                      <div className='flex flex-wrap items-center gap-2 text-sm'>
                        <span>{t(getChannelTypeLabel(item.type))}</span>
                        <ConnectionStatus status={item.status} />
                      </div>
                      <p className='text-muted-foreground text-sm'>
                        {t(
                          'Configuration status does not confirm upstream availability.'
                        )}
                      </p>
                      <div className='flex flex-wrap gap-2'>
                        {data.canWrite ? (
                          <Button
                            ref={editButton}
                            className='h-auto min-h-11 max-w-full py-2 whitespace-normal'
                            onClick={() => setEditor(editorIdentity)}
                          >
                            {t('Edit connection')}
                          </Button>
                        ) : null}
                        {showKeys && data.self.data?.role === 100 ? (
                          <Link
                            to='/keys'
                            search={{
                              connectionContext: {
                                connection: { ...search, connection: item.id },
                              },
                            }}
                            className={`${buttonVariants({ variant: 'outline' })} h-auto min-h-11 max-w-full py-2 whitespace-normal`}
                          >
                            {t('Associated Key shares')}
                          </Link>
                        ) : null}
                        {showLogs ? (
                          <Link
                            to='/usage-logs/$section'
                            params={{ section: 'common' }}
                            search={{
                              channel: String(item.id),
                              connectionContext: {
                                connection: { ...search, connection: item.id },
                              },
                              page: 1,
                              pageSize: 20,
                            }}
                            className={`${buttonVariants({ variant: 'outline' })} h-auto min-h-11 max-w-full py-2 whitespace-normal`}
                          >
                            {t('View connection requests')}
                          </Link>
                        ) : null}
                      </div>
                    </section>
                    <Configuration item={item} />
                    <ConnectionWindows
                      key={`${data.identity}:${item.id}`}
                      channelId={item.id}
                      identity={data.identity}
                    />
                  </>
                )}
              </CardContent>
            </Card>
          ) : null}
        </div>
      )}
      {editing && selectedId !== undefined ? (
        <ChannelsProvider key={editorIdentity}>
          <ChannelMutateDrawer
            open
            currentRow={{ id: selectedId }}
            onOpenChange={(open) => {
              if (!open) {
                setEditor(null)
                editButton.current?.focus()
              }
            }}
          />
        </ChannelsProvider>
      ) : null}
    </main>
  )
}
