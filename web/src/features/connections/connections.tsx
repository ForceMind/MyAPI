/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import {
  ArrowRight01Icon,
  PlusSignIcon,
  Search01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { getRouteApi, Link } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useInspectorBack } from '@/components/inspector/inspector-history'
import { InspectorLayout } from '@/components/inspector/inspector-layout'
import { useInspectorFocusReturn } from '@/components/inspector/use-inspector-focus-return'
import { SectionPageLayout } from '@/components/layout'
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
import { getChannelTypeLabel } from '@/features/channels/lib/channel-utils'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'

import {
  ConnectionInspector,
  ConnectionStatus,
  LastTest,
} from './connection-inspector'
import {
  CONNECTION_PAGE_SIZE,
  type ConnectionInspectorTab,
  type ConnectionSummary,
  type ConnectionsSearch,
} from './connection-state'
import { useConnectionTest } from './use-connection-test'
import { useConnections } from './use-connections'

const route = getRouteApi('/_authenticated/connections/')

export type ConnectionsNavigation = {
  replace?: boolean
  /** The new entry was opened from the list; closing goes back to it. */
  fromList?: boolean
}

export function Connections() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const goBack = useInspectorBack()
  return (
    <ConnectionsWorkspace
      search={search}
      onSearch={(next, options) =>
        void navigate({
          search: next,
          replace: options?.replace,
          state: options?.fromList ? { inspectorBack: true } : undefined,
        })
      }
      onCloseInspector={() =>
        goBack(
          () =>
            void navigate({
              search: { ...search, connection: undefined, tab: undefined },
              replace: true,
            })
        )
      }
    />
  )
}

/**
 * Upstream connections: the list with an inspector (status, last test, Test
 * and Edit, saved account windows, routing), plus "Add connection" that saves
 * through the existing connection editor and then runs the existing test.
 * Every view is in the URL; opening the page or a connection reads saved
 * configuration only and never contacts a provider.
 */
export function ConnectionsWorkspace({
  search,
  onSearch,
  onCloseInspector,
}: {
  search: ConnectionsSearch
  onSearch: (next: ConnectionsSearch, options?: ConnectionsNavigation) => void
  onCloseInspector?: () => void
}) {
  const { t } = useTranslation()
  const data = useConnections(search)
  const showKeys = useIsSidebarModuleVisible('/keys')
  const showLogs = useIsSidebarModuleVisible('/usage-logs/common')
  const showTable = useIsSidebarModuleVisible('/channels')
  const [filter, setFilter] = useState(search.filter ?? '')
  const [editor, setEditor] = useState<string | null>(null)
  const [adding, setAdding] = useState(false)
  const editButton = useRef<HTMLButtonElement>(null)
  const addButton = useRef<HTMLButtonElement>(null)
  const inventoryHeading = useRef<HTMLHeadingElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const test = useConnectionTest(data.identity)
  const selectedId = search.connection
  const editorIdentity = `${data.identity}:${selectedId}`
  const item = data.detail.isError ? undefined : data.detail.data
  useEffect(() => setFilter(search.filter ?? ''), [search.filter])
  useEffect(() => setEditor(null), [editorIdentity, data.canWrite])
  useEffect(() => setAdding(false), [data.identity, data.canWrite])
  useInspectorFocusReturn({
    selectedId: selectedId === undefined ? undefined : String(selectedId),
    listRef,
    fallbackRef: inventoryHeading,
    context: JSON.stringify([
      data.identity,
      data.canRead,
      search.filter,
      search.status,
      search.page ?? 1,
    ]),
    settled: data.list.isSuccess || data.list.isError,
    rows: data.list.data,
  })
  // Selection/session changes cannot reopen an editor with the previous target.
  const editing =
    data.canWrite && editor === editorIdentity && item?.id === selectedId
  const page = search.page ?? 1
  const closeInspector =
    onCloseInspector ??
    (() =>
      onSearch(
        { ...search, connection: undefined, tab: undefined },
        { replace: true }
      ))
  const openConnection = (connection: ConnectionSummary) =>
    onSearch(
      { ...search, connection: connection.id, tab: undefined },
      selectedId === undefined ? { fromList: true } : { replace: true }
    )

  const inventory = (
    <section
      className='bg-card p-card flex min-w-0 flex-col gap-4 rounded-xl border'
      aria-labelledby='connections-inventory-title'
      data-testid='connections-inventory'
    >
      <div className='flex min-w-0 flex-col gap-3'>
        <h2
          id='connections-inventory-title'
          ref={inventoryHeading}
          tabIndex={-1}
          className='text-title outline-none'
        >
          {t('Configured connections')}
        </h2>
        <form
          className='flex min-w-0 flex-wrap gap-2'
          onSubmit={(event) => {
            event.preventDefault()
            onSearch({
              ...search,
              filter: filter.trim() || undefined,
              page: 1,
              connection: undefined,
              tab: undefined,
            })
          }}
        >
          <label className='min-w-0 flex-1 basis-40'>
            <span className='sr-only'>{t('Search connections')}</span>
            <Input
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
              maxLength={200}
              placeholder={t('Search connections')}
            />
          </label>
          <Button type='submit' variant='outline' aria-label={t('Search')}>
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
            onSearch({
              ...search,
              status:
                next === 'all'
                  ? undefined
                  : (next as ConnectionsSearch['status']),
              page: 1,
              connection: undefined,
              tab: undefined,
            })
          }}
        >
          <ToggleGroupItem value='all'>{t('All Status')}</ToggleGroupItem>
          <ToggleGroupItem value='enabled'>{t('Enabled')}</ToggleGroupItem>
          <ToggleGroupItem value='disabled'>{t('Disabled')}</ToggleGroupItem>
        </ToggleGroup>
      </div>
      {data.list.isPending && (
        <div className='flex flex-col gap-2' aria-busy='true'>
          <span className='sr-only' role='status'>
            {t('Loading...')}
          </span>
          {[1, 2, 3].map((row) => (
            <Skeleton key={row} className='h-16 w-full' />
          ))}
        </div>
      )}
      {data.list.isError && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Unable to load connections')}</AlertTitle>
          <AlertDescription>
            <Button
              variant='outline'
              size='sm'
              onClick={() => void data.list.refetch()}
            >
              {t('Retry')}
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {data.list.isSuccess && (
        <>
          <p className='text-caption text-muted-foreground' aria-live='polite'>
            {t('Connections: {{count}}', { count: data.list.data.total })}
          </p>
          {data.list.data.items.length === 0 ? (
            <Empty className='border'>
              <EmptyHeader>
                <EmptyTitle>{t('No connections yet')}</EmptyTitle>
                <EmptyDescription>
                  {search.filter || search.status
                    ? t('No connection matches these filters.')
                    : t(
                        'Add an upstream connection to start routing requests.'
                      )}
                </EmptyDescription>
              </EmptyHeader>
              {data.canWrite && !search.filter && !search.status && (
                <EmptyContent>
                  <Button onClick={() => setAdding(true)}>
                    {t('Add connection')}
                  </Button>
                </EmptyContent>
              )}
            </Empty>
          ) : (
            <div
              ref={listRef}
              className='flex flex-col divide-y rounded-lg border'
            >
              {data.list.data.items.map((connection) => (
                <button
                  key={connection.id}
                  type='button'
                  data-connection-id={connection.id}
                  data-inspect-id={connection.id}
                  aria-expanded={selectedId === connection.id}
                  onClick={() => openConnection(connection)}
                  className={cn(
                    'hover:bg-muted flex min-h-16 w-full min-w-0 items-center gap-3 px-3 py-2 text-left focus-visible:outline-2 focus-visible:-outline-offset-2',
                    selectedId === connection.id && 'bg-accent'
                  )}
                >
                  <span className='flex min-w-0 flex-1 flex-col gap-1'>
                    <span className='text-body min-w-0 font-medium wrap-anywhere'>
                      {connection.name || `#${connection.id}`}
                    </span>
                    <span className='text-caption text-muted-foreground flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1'>
                      <span>
                        #{connection.id} ·{' '}
                        {t(getChannelTypeLabel(connection.type))}
                      </span>
                      <ConnectionStatus status={connection.status} />
                      <span>
                        {t('Last test')}: <LastTest item={connection} />
                      </span>
                    </span>
                  </span>
                  <HugeiconsIcon
                    icon={ArrowRight01Icon}
                    className='text-muted-foreground size-4 shrink-0'
                    aria-hidden='true'
                  />
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
              disabled={page <= 1}
              onClick={() =>
                onSearch({
                  ...search,
                  page: page - 1,
                  connection: undefined,
                  tab: undefined,
                })
              }
            >
              {t('Previous page')}
            </Button>
            <span className='text-caption text-muted-foreground'>
              {t('Page {{page}}', { page })}
            </span>
            <Button
              variant='outline'
              disabled={page * CONNECTION_PAGE_SIZE >= data.list.data.total}
              onClick={() =>
                onSearch({
                  ...search,
                  page: page + 1,
                  connection: undefined,
                  tab: undefined,
                })
              }
            >
              {t('Next page')}
            </Button>
          </nav>
        </>
      )}
    </section>
  )

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Connections')}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t(
            'Select a connection to test it, edit it or read its saved account windows.'
          )}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          {data.canRead && showTable && (
            <nav aria-label={t('Connection views')} className='flex gap-1'>
              <span
                aria-current='page'
                className={cn(
                  buttonVariants({ variant: 'secondary', size: 'sm' }),
                  'h-auto min-h-8'
                )}
              >
                {t('List view')}
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
          {data.canWrite && (
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
            {data.self.isFetching && (
              <p role='status' className='text-body text-muted-foreground'>
                {t('Loading...')}
              </p>
            )}
            {!data.self.isFetching && !data.canRead && (
              <Alert variant='destructive'>
                <AlertTitle>
                  {t('Connection access could not be verified.')}
                </AlertTitle>
                <AlertDescription>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => void data.self.refetch()}
                  >
                    {t('Retry')}
                  </Button>
                </AlertDescription>
              </Alert>
            )}
            {data.canRead && (
              <InspectorLayout
                open={selectedId !== undefined}
                label={
                  item?.id === selectedId && item?.name
                    ? item.name
                    : t('Connection #{{id}}', { id: selectedId ?? '' })
                }
                onClose={closeInspector}
                list={inventory}
                inspector={
                  selectedId !== undefined && (
                    <ConnectionInspector
                      connectionId={selectedId}
                      search={search}
                      detail={data.detail}
                      item={item}
                      identity={data.identity}
                      canWrite={data.canWrite}
                      canOperate={data.canOperate}
                      showKeys={showKeys}
                      showLogs={showLogs}
                      isRoot={data.self.data?.role === ROLE.SUPER_ADMIN}
                      test={test}
                      editButtonRef={editButton}
                      onEdit={() => setEditor(editorIdentity)}
                      onTabChange={(tab: ConnectionInspectorTab) =>
                        onSearch(
                          {
                            ...search,
                            tab: tab === 'overview' ? undefined : tab,
                          },
                          { replace: true }
                        )
                      }
                      onClose={closeInspector}
                    />
                  )
                }
              />
            )}
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>
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
      {adding && data.canWrite ? (
        <ChannelsProvider key={`${data.identity}:add`}>
          <ChannelMutateDrawer
            open
            currentRow={null}
            title={t('Add connection')}
            submitLabel={data.canOperate ? t('Save and test') : undefined}
            onSaved={(ids) => {
              const created = ids[0]
              if (created === undefined) return
              onSearch(
                { ...search, connection: created, tab: undefined },
                { fromList: true }
              )
              if (data.canOperate) {
                void test.run({ id: created, name: '' })
              }
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
