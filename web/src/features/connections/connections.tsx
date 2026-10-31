/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { PlusSignIcon, Search01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { LogPagination } from '@/components/log-pagination'
import {
  useRememberReturnFocus,
  useReturnFocus,
} from '@/components/object-page/return-focus'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { ChannelsProvider } from '@/features/channels/components/channels-provider'
import { ChannelMutateDrawer } from '@/features/channels/components/drawers/channel-mutate-drawer'
import {
  CHANNEL_STATUS,
  CHANNEL_TYPE_OPTIONS,
} from '@/features/channels/constants'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'

import { ConnectionCard } from './connection-card'
import { ConnectionViewSwitcher } from './connection-view-switcher'
import {
  CONNECTION_PAGE_SIZE,
  connectionFilterValue,
  connectionListContext,
  connectionValues,
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
  const [model, setModel] = useState(search.model ?? '')
  const [types, setTypes] = useState(search.type ?? [])
  const [groups, setGroups] = useState(search.group ?? [])
  const [adding, setAdding] = useState(false)
  const [editing, setEditing] = useState<number | null>(null)
  const addButton = useRef<HTMLButtonElement>(null)
  const inventoryHeading = useRef<HTMLHeadingElement>(null)
  const connectionReturn = connectionListContext(search)
  const page = search.page ?? 1
  const pageSize = search.pageSize ?? CONNECTION_PAGE_SIZE
  const status = connectionFilterValue(search.status)
  const selectedType = connectionFilterValue(types) ?? 'all'
  const selectedGroup = connectionFilterValue(groups) ?? ''
  const hasMultipleFilterValues =
    types.length > 1 || groups.length > 1 ||
    (Array.isArray(search.status) && search.status.length > 1)
  useEffect(() => setFilter(search.filter ?? ''), [search.filter])
  useEffect(() => setModel(search.model ?? ''), [search.model])
  useEffect(() => setTypes(search.type ?? []), [search.type])
  useEffect(() => setGroups(search.group ?? []), [search.group])
  useEffect(() => {
    setAdding(false)
    setEditing(null)
  }, [access.identity, access.canWrite])
  // Back from a connection's page puts focus on the card that opened it.
  useReturnFocus({
    attribute: 'data-connection-link',
    requestedId: search.focusConnectionId,
    settled: list.isSuccess || list.isError,
    rows: list.data,
    // A card that is gone (or a list that could not be read) returns focus
    // to the inventory instead.
    fallback: inventoryHeading,
  })

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
        className='grid min-w-0 grid-cols-1 gap-4 lg:grid-cols-2'
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
    const filtered = Boolean(
      search.filter || search.model || status || search.type?.length || search.group?.length
    )
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
        className='grid min-w-0 grid-cols-1 gap-4 lg:grid-cols-2'
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
              windowsKnown={windows.isSuccess}
              windowsLoading={windows.isPending}
              now={windows.now}
              recentRequests={recentRequests(connection.id)}
              connectionReturn={connectionReturn}
              canOperate={access.canOperate}
              canEdit={access.canWrite}
              switching={power.switching === connection.id}
              testing={test.running === connection.id}
              testBusy={test.running !== null}
              testResult={test.result}
              onOpen={() =>
                rememberFocus('data-connection-link', connection.id)
              }
              onToggle={(enabled) => void power.toggle(connection.id, enabled)}
              onTest={() => void test.run(connection)}
              onEdit={() => setEditing(connection.id)}
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
          {access.canRead && (
            <ConnectionViewSwitcher view='cards' search={{ ...search, pageSize }} showOther={showTable} />
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
                <div className='bg-card flex min-w-0 flex-col gap-4 rounded-xl border p-4'>
                  <h3 className='text-body font-medium'>{t('Filters')}</h3>
                  <form
                    role='search'
                    className='grid min-w-0 grid-cols-1 items-end gap-3 sm:grid-cols-2 xl:grid-cols-4'
                    onSubmit={(event) => {
                      event.preventDefault()
                      applySearch({
                        ...search,
                        filter: filter.trim() || undefined,
                        model: model.trim() || undefined,
                        type: types.length ? types : undefined,
                        group: groups.length ? groups : undefined,
                        page: 1,
                      })
                    }}
                  >
                    <label className='flex min-w-0 flex-col gap-2'>
                      <span className='text-caption text-muted-foreground'>
                        {t('Search connections')}
                      </span>
                      <Input
                        value={filter}
                        onChange={(event) => setFilter(event.target.value)}
                        maxLength={200}
                        placeholder={t('Search connections')}
                        className='min-h-11'
                      />
                    </label>
                    <label className='flex min-w-0 flex-col gap-2'>
                      <span className='text-caption text-muted-foreground'>
                        {t('Model')}
                      </span>
                      <Input
                        value={model}
                        onChange={(event) => setModel(event.target.value)}
                        maxLength={200}
                        placeholder={t('Filter by model name...')}
                        className='min-h-11'
                      />
                    </label>
                    <div className='flex min-w-0 flex-col gap-2'>
                      <label htmlFor='connection-types' className='text-caption text-muted-foreground'>
                        {t('Provider')}
                      </label>
                      <NativeSelect
                        id='connection-types'
                        value={selectedType}
                        onChange={(event) =>
                          setTypes(event.target.value === 'all' ? [] : [event.target.value])
                        }
                        className='w-full [&_select]:min-h-11'
                      >
                        <NativeSelectOption value='all'>{t('All Types')}</NativeSelectOption>
                        {selectedType !== 'all' && !CHANNEL_TYPE_OPTIONS.some((item) => String(item.value) === selectedType) && (
                          <NativeSelectOption value={selectedType}>{selectedType}</NativeSelectOption>
                        )}
                        {CHANNEL_TYPE_OPTIONS.map((item) => (
                          <NativeSelectOption key={item.value} value={String(item.value)}>
                            {t(item.label)}
                          </NativeSelectOption>
                        ))}
                      </NativeSelect>
                    </div>
                    <div className='flex min-w-0 flex-col gap-2'>
                      <label htmlFor='connection-groups' className='text-caption text-muted-foreground'>
                        {t('Group')}
                      </label>
                      <Input
                        id='connection-groups'
                        value={selectedGroup}
                        onChange={(event) =>
                          setGroups(event.target.value ? [event.target.value] : [])
                        }
                        list='connection-group-options'
                        maxLength={200}
                        placeholder={t('All Groups')}
                        className='min-h-11'
                      />
                      <datalist id='connection-group-options'>
                        {[
                          ...new Set([
                            ...groups,
                            ...items.flatMap((item) => connectionValues(item.group)),
                          ]),
                        ].map((value) => <option key={value} value={value} />)}
                      </datalist>
                    </div>
                    <Button
                      type='submit'
                      variant='outline'
                      className='min-h-11 sm:justify-self-start'
                    >
                      <HugeiconsIcon
                        icon={Search01Icon}
                        data-icon='inline-start'
                        aria-hidden='true'
                      />
                      {t('Search')}
                    </Button>
                  </form>
                  {hasMultipleFilterValues && (
                    <p className='text-caption text-muted-foreground'>
                      {t('Only the first value of each filter is applied.')}
                    </p>
                  )}
                  <ToggleGroup
                    aria-label={t('Status')}
                    variant='outline'
                    size='sm'
                    value={[status ?? 'all']}
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
                <LogPagination
                  label={t('Pagination')}
                  page={page}
                  pageSize={pageSize}
                  total={list.isSuccess ? list.data.total : undefined}
                  busy={list.isFetching}
                  onPageChange={(nextPage) => applySearch({ ...search, page: nextPage })}
                  onPageSizeChange={(nextSize) =>
                    applySearch({ ...search, pageSize: nextSize, page: 1 })
                  }
                />
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
      {editing !== null && access.canWrite ? (
        <ChannelsProvider key={`${access.identity}:edit:${editing}`}>
          <ChannelMutateDrawer
            open
            currentRow={{ id: editing }}
            title={t('Edit connection')}
            onOpenChange={(open) => {
              if (!open) {
                const previous = editing
                setEditing(null)
                document.querySelector<HTMLButtonElement>(`[data-connection-edit="${previous}"]`)?.focus()
              }
            }}
          />
        </ChannelsProvider>
      ) : null}
    </>
  )
}
