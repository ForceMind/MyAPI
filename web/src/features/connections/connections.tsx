/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { PlusSignIcon, Search01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { getRouteApi, Link, useNavigate } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import {
  useRememberReturnFocus,
  useReturnFocus,
} from '@/components/object-page/return-focus'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { ChannelsProvider } from '@/features/channels/components/channels-provider'
import { ChannelMutateDrawer } from '@/features/channels/components/drawers/channel-mutate-drawer'
import { CHANNEL_STATUS } from '@/features/channels/constants'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { cn } from '@/lib/utils'

import { ConnectionCard } from './connection-card'
import {
  CONNECTION_PAGE_SIZE,
  connectionListContext,
  type ConnectionsSearch,
} from './connection-state'
import { useConnectionSwitch } from './use-connection-switch'
import { useConnectionTest } from './use-connection-test'
import {
  useConnectionAccess,
  useConnectionList,
  useConnectionTraffic,
  useConnectionWindows,
} from './use-connections'

const route = getRouteApi('/_authenticated/connections/')

/**
 * Upstream connections (design v2 upstream): a card grid with each
 * connection's saved window ring, state, enable switch and Test, plus "Add
 * connection" that saves through the existing editor and tests the new
 * connection. A card opens the connection's page; Back returns here with the
 * same filters and page and focus on that card. Opening the page reads saved
 * configuration, observations and call records only; only Test contacts a
 * provider.
 */
export function Connections() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const navigate = useNavigate()
  const access = useConnectionAccess()
  const list = useConnectionList(access, search)
  const items = list.isError ? [] : (list.data?.items ?? [])
  const windows = useConnectionWindows(
    access,
    items.map((item) => item.id)
  )
  const traffic = useConnectionTraffic(access)
  const test = useConnectionTest(access.identity)
  const power = useConnectionSwitch()
  const rememberFocus = useRememberReturnFocus()
  const showTable = useIsSidebarModuleVisible('/channels')
  const [filter, setFilter] = useState(search.filter ?? '')
  const [adding, setAdding] = useState(false)
  const addButton = useRef<HTMLButtonElement>(null)
  const inventoryHeading = useRef<HTMLHeadingElement>(null)
  const connectionReturn = connectionListContext(search)
  const page = search.page ?? 1
  useEffect(() => setFilter(search.filter ?? ''), [search.filter])
  useEffect(() => setAdding(false), [access.identity, access.canWrite])
  // Back from a connection's page puts focus on the card that opened it.
  useReturnFocus({
    attribute: 'data-connection-link',
    settled: list.isSuccess || list.isError,
    rows: list.data,
    // A card that is gone (or a list that could not be read) returns focus
    // to the inventory instead.
    fallback: inventoryHeading,
  })

  // Back from a page that was opened by a link lands here with the card to
  // focus (there is no history entry to return to).
  const focusId = search.focusConnectionId
  useEffect(() => {
    if (focusId === undefined || !list.isSuccess) return
    document
      .querySelector<HTMLElement>(`[data-connection-link="${focusId}"]`)
      ?.focus()
  }, [focusId, list.isSuccess])

  const applySearch = (next: ConnectionsSearch) =>
    void navigate({
      to: '/connections',
      search: { ...next, focusConnectionId: undefined },
    })
  const trafficMap = traffic.isError ? undefined : traffic.data?.traffic
  const inUse = items.filter(
    (item) =>
      item.status === CHANNEL_STATUS.ENABLED &&
      (trafficMap?.get(item.id)?.requests ?? 0) > 0
  )
  // Unknown traffic stays unknown; a connection absent from known traffic
  // carried none of the latest requests.
  const recentRequests = (id: number) =>
    trafficMap ? (trafficMap.get(id)?.requests ?? 0) : undefined

  let summary = t('Saved upstream connections')
  if (list.isSuccess) {
    summary = t('{{count}} connections', { count: list.data.total })
    if (inUse.length > 0) {
      summary += ` · ${t('In use: {{names}}', {
        names: inUse.map((item) => item.name || `#${item.id}`).join(', '),
      })}`
    }
  }

  let content
  if (list.isPending) {
    content = (
      <div
        className='grid min-w-0 grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3'
        aria-busy='true'
      >
        <span className='sr-only' role='status'>
          {t('Loading...')}
        </span>
        {[1, 2, 3].map((item) => (
          <Skeleton key={item} className='h-48 w-full rounded-xl' />
        ))}
      </div>
    )
  } else if (list.isError) {
    content = (
      <Alert variant='destructive'>
        <AlertTitle>{t('Unable to load connections')}</AlertTitle>
        <AlertDescription>
          <Button
            variant='outline'
            size='sm'
            disabled={list.isFetching}
            onClick={() => void list.refetch()}
          >
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else if (items.length === 0) {
    const filtered = Boolean(search.filter || search.status)
    content = (
      <Empty className='border'>
        <EmptyHeader>
          <EmptyTitle>{t('No connections yet')}</EmptyTitle>
          <EmptyDescription>
            {filtered
              ? t('No connection matches these filters.')
              : t('Add an upstream connection to start routing requests.')}
          </EmptyDescription>
        </EmptyHeader>
        {access.canWrite && !filtered && (
          <EmptyContent>
            <Button onClick={() => setAdding(true)}>
              {t('Add connection')}
            </Button>
          </EmptyContent>
        )}
      </Empty>
    )
  } else {
    content = (
      <ul
        data-testid='connection-grid'
        className='grid min-w-0 grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3'
      >
        {items.map((connection) => (
          <li key={connection.id} className='flex min-w-0'>
            <ConnectionCard
              connection={connection}
              windows={
                windows.data?.filter(
                  (item) => item.channel_id === connection.id
                ) ?? []
              }
              windowsKnown={!windows.isError}
              now={windows.now}
              recentRequests={recentRequests(connection.id)}
              connectionReturn={connectionReturn}
              canOperate={access.canOperate}
              switching={power.switching === connection.id}
              testing={test.running === connection.id}
              testBusy={test.running !== null}
              testResult={test.result}
              onOpen={() =>
                rememberFocus('data-connection-link', connection.id)
              }
              onToggle={(enabled) => void power.toggle(connection.id, enabled)}
              onTest={() => void test.run(connection)}
            />
          </li>
        ))}
      </ul>
    )
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Connections')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>{summary}</SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          {access.canRead && items.length > 0 && (
            <Button
              variant='outline'
              disabled={windows.isFetching}
              onClick={() => void windows.refetch()}
            >
              {t('Refresh saved windows')}
            </Button>
          )}
          {access.canRead && showTable && (
            <nav aria-label={t('Connection views')} className='flex gap-1'>
              <span
                aria-current='page'
                className={cn(
                  buttonVariants({ variant: 'secondary', size: 'sm' }),
                  'h-auto min-h-8'
                )}
              >
                {t('Card view')}
              </span>
              <Link
                to='/channels'
                data-myapi-button=''
                className={cn(
                  buttonVariants({ variant: 'ghost', size: 'sm' }),
                  'h-auto min-h-8'
                )}
              >
                {t('Table view')}
              </Link>
            </nav>
          )}
          {access.canWrite && (
            <Button ref={addButton} onClick={() => setAdding(true)}>
              <HugeiconsIcon
                icon={PlusSignIcon}
                data-icon='inline-start'
                aria-hidden='true'
              />
              {t('Add connection')}
            </Button>
          )}
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div
            className='flex min-w-0 flex-col gap-4'
            data-testid='connections-workspace'
          >
            {access.self.isFetching && (
              <p role='status' className='text-body text-muted-foreground'>
                {t('Loading...')}
              </p>
            )}
            {!access.self.isFetching && !access.canRead && (
              <Alert variant='destructive'>
                <AlertTitle>
                  {t('Connection access could not be verified.')}
                </AlertTitle>
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
            )}
            {access.canRead && (
              <section
                aria-labelledby='connections-inventory-title'
                className='flex min-w-0 flex-col gap-4'
                data-testid='connections-inventory'
              >
                <h2
                  id='connections-inventory-title'
                  ref={inventoryHeading}
                  tabIndex={-1}
                  className='sr-only'
                >
                  {t('Configured connections')}
                </h2>
                <div className='flex min-w-0 flex-wrap items-center gap-2'>
                  <form
                    role='search'
                    className='flex min-w-0 flex-1 basis-64 gap-2'
                    onSubmit={(event) => {
                      event.preventDefault()
                      applySearch({
                        ...search,
                        filter: filter.trim() || undefined,
                        page: 1,
                      })
                    }}
                  >
                    <label className='min-w-0 flex-1'>
                      <span className='sr-only'>{t('Search connections')}</span>
                      <Input
                        value={filter}
                        onChange={(event) => setFilter(event.target.value)}
                        maxLength={200}
                        placeholder={t('Search connections')}
                      />
                    </label>
                    <Button type='submit' variant='outline'>
                      <HugeiconsIcon
                        icon={Search01Icon}
                        data-icon='inline-start'
                        aria-hidden='true'
                      />
                      {t('Search')}
                    </Button>
                  </form>
                  <ToggleGroup
                    aria-label={t('Status')}
                    variant='outline'
                    size='sm'
                    value={[search.status ?? 'all']}
                    onValueChange={(value) => {
                      const next = value[0]
                      if (!next) return
                      applySearch({
                        ...search,
                        status:
                          next === 'all'
                            ? undefined
                            : (next as ConnectionsSearch['status']),
                        page: 1,
                      })
                    }}
                    className='max-w-full'
                  >
                    <ToggleGroupItem value='all' className='max-lg:min-h-11'>
                      {t('All Status')}
                    </ToggleGroupItem>
                    <ToggleGroupItem
                      value='enabled'
                      className='max-lg:min-h-11'
                    >
                      {t('Enabled')}
                    </ToggleGroupItem>
                    <ToggleGroupItem
                      value='disabled'
                      className='max-lg:min-h-11'
                    >
                      {t('Disabled')}
                    </ToggleGroupItem>
                  </ToggleGroup>
                </div>
                {content}
                {list.isSuccess && list.data.total > CONNECTION_PAGE_SIZE && (
                  <nav
                    aria-label={t('Pagination')}
                    className='flex flex-wrap items-center justify-between gap-2'
                  >
                    <Button
                      variant='outline'
                      disabled={page <= 1}
                      onClick={() => applySearch({ ...search, page: page - 1 })}
                    >
                      {t('Previous page')}
                    </Button>
                    <span className='text-caption text-muted-foreground'>
                      {t('Page {{page}}', { page })}
                    </span>
                    <Button
                      variant='outline'
                      disabled={page * CONNECTION_PAGE_SIZE >= list.data.total}
                      onClick={() => applySearch({ ...search, page: page + 1 })}
                    >
                      {t('Next page')}
                    </Button>
                  </nav>
                )}
              </section>
            )}
            <p className='text-caption text-muted-foreground'>
              {t(
                'Opening this page does not send requests to any upstream; only Test contacts the provider.'
              )}
            </p>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>
      {adding && access.canWrite ? (
        <ChannelsProvider key={`${access.identity}:add`}>
          <ChannelMutateDrawer
            open
            currentRow={null}
            title={t('Add connection')}
            submitLabel={access.canOperate ? t('Save and test') : undefined}
            onSaved={(ids) => {
              const created = ids[0]
              if (created === undefined) return
              if (access.canOperate) void test.run({ id: created, name: '' })
              void navigate({
                to: '/connections/$connectionId',
                params: { connectionId: String(created) },
                search: { connectionReturn },
                state: { inspectorBack: true },
              })
            }}
            onOpenChange={(open) => {
              if (!open) {
                setAdding(false)
                addButton.current?.focus()
              }
            }}
          />
        </ChannelsProvider>
      ) : null}
    </>
  )
}
