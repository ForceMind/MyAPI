/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import type {
  ColumnFiltersState,
  OnChangeFn,
  SortingState,
  Row,
} from '@tanstack/react-table'
import { Eye, EyeOff } from 'lucide-react'
import { useState, useMemo, useEffect, type MouseEvent } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
  DataTableRow,
  useDebouncedColumnFilter,
  useDataTable,
} from '@/components/data-table'
import { LogPagination } from '@/components/log-pagination'
import { ConnectionListContext } from '@/features/connections/connection-list-context'
import { ConnectionViewSwitcher } from '@/features/connections/connection-view-switcher'
import { tableToConnectionsSearch, type ConnectionReturn } from '@/features/connections/connection-state'
import { normalizeLogPagination } from '@/lib/log-pagination'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { hasPermission } from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { ErrorState } from '@/components/error-state'
import {
  useRememberReturnFocus,
  useReturnFocus,
} from '@/components/object-page/return-focus'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import { getChannels, searchChannels, getGroups } from '../api'
import {
  DEFAULT_PAGE_SIZE,
  CHANNEL_STATUS,
  CHANNEL_STATUS_OPTIONS,
} from '../constants'
import {
  channelsQueryKeys,
  aggregateChannelsByTag,
  getChannelTableRowId,
  isTagAggregateRow,
  getChannelTypeIcon,
  getChannelTypeLabel,
} from '../lib'
import { getChannelSortParams } from '../lib/channel-routing'
import type { Channel } from '../types'
import { ChannelCard } from './channel-card'
import { useChannelsColumns } from './channels-columns'
import { useChannels } from './channels-provider'
import { DataTableBulkActions } from './data-table-bulk-actions'

const route = getRouteApi('/_authenticated/channels/')
const CHANNELS_COLUMN_VISIBILITY_STORAGE_KEY = 'channels:column-visibility'
const CHANNELS_COLUMN_SIZING_STORAGE_KEY = 'channels:column-sizing'
const CHANNELS_VIEW_MODE_STORAGE_KEY = 'channels:view-mode'
const CHANNELS_STATUS_FILTER_STORAGE_KEY = 'channel-status-filter'

function isDisabledChannelRow(channel: Channel) {
  return (
    !isTagAggregateRow(channel) && channel.status !== CHANNEL_STATUS.ENABLED
  )
}

export function ChannelsTable() {
  const { t } = useTranslation()
  const {
    enableTagMode,
    idSort,
    batchMode,
    sensitiveVisible,
    setSensitiveVisible,
  } = useChannels()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const navigate = useNavigate()
  const search = route.useSearch()
  const showCards = useIsSidebarModuleVisible('/connections')
  const currentUser = useAuthStore((state) => state.auth.user)
  const actor = useAuthStore((state) => [state.auth.user?.id, state.auth.session?.sid, state.auth.user?.role, hasPermission(state.auth.user, 'channel', 'read')].join(':'))
  const canRead = Boolean(currentUser && currentUser.role >= ROLE.ADMIN && hasPermission(currentUser, 'channel', 'read'))
  const rememberFocus = useRememberReturnFocus()

  // Table state
  const [sorting, setSorting] = useState<SortingState>([])

  // URL state management
  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination: urlPagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search,
    navigate: route.useNavigate(),
    pagination: {
      defaultPage: 1,
      defaultPageSize: DEFAULT_PAGE_SIZE,
    },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      {
        columnId: 'status',
        searchKey: 'status',
        type: 'array',
        deserialize: (value) => {
          if (value !== undefined) return value
          const stored = localStorage.getItem(
            CHANNELS_STATUS_FILTER_STORAGE_KEY
          )
          return stored === 'enabled' || stored === 'disabled' ? [stored] : []
        },
      },
      { columnId: 'type', searchKey: 'type', type: 'array' },
      { columnId: 'group', searchKey: 'group', type: 'array' },
      { columnId: 'model', searchKey: 'model', type: 'string' },
    ],
  })

  const pagination = normalizeLogPagination(urlPagination.pageIndex, urlPagination.pageSize)

  const handleColumnFiltersChange: OnChangeFn<ColumnFiltersState> = (
    updater
  ) => {
    onColumnFiltersChange((previous) => {
      const next = typeof updater === 'function' ? updater(previous) : updater
      const status = next.find((f) => f.id === 'status')?.value as
        | string[]
        | undefined
      localStorage.setItem(
        CHANNELS_STATUS_FILTER_STORAGE_KEY,
        status?.[0] ?? 'all'
      )
      return next
    })
  }

  // Extract filters from column filters
  const statusFilter =
    (columnFilters.find((f) => f.id === 'status')?.value as string[]) || []
  const typeFilter = useMemo(
    () => (columnFilters.find((f) => f.id === 'type')?.value as string[]) || [],
    [columnFilters]
  )
  const groupFilter =
    (columnFilters.find((f) => f.id === 'group')?.value as string[]) || []
  const {
    value: modelFilter,
    inputValue: modelFilterInput,
    onChange: onModelFilterInputChange,
    onCompositionStart: onModelFilterCompositionStart,
    onCompositionEnd: onModelFilterCompositionEnd,
    resetInput: resetModelFilterInput,
  } = useDebouncedColumnFilter({
    columnFilters,
    columnId: 'model',
    onColumnFiltersChange,
  })

  // Determine whether to use search or regular list API
  const shouldSearch = Boolean(globalFilter?.trim() || modelFilter.trim())

  const effectiveSorting = useMemo(
    () =>
      enableTagMode
        ? sorting.filter(
            (sort) => sort.id !== 'priority' && sort.id !== 'weight'
          )
        : sorting,
    [enableTagMode, sorting]
  )
  const sortParams = useMemo(
    () => getChannelSortParams(effectiveSorting, enableTagMode),
    [effectiveSorting, enableTagMode]
  )

  const handleSortingChange: OnChangeFn<SortingState> = (updater) => {
    setSorting((previous) => {
      const next = typeof updater === 'function' ? updater(previous) : updater
      if (pagination.pageIndex > 0) {
        onPaginationChange({ ...pagination, pageIndex: 0 })
      }
      return next
    })
  }

  // Fetch groups for filter
  const { data: groupsData } = useQuery({
    queryKey: ['groups', actor],
    queryFn: getGroups,
    enabled: canRead,
  })

  const groupOptions = useMemo(
    () =>
      (groupsData?.data || []).map((g) => ({
        label: g,
        value: g,
      })),
    [groupsData]
  )

  // Fetch channels data
  // eslint-disable-next-line @tanstack/query/exhaustive-deps
  const { data, isError, isLoading, isFetching, refetch } = useQuery({
    queryKey: [...channelsQueryKeys.list({
      keyword: globalFilter,
      model: modelFilter,
      group:
        groupFilter.length > 0 && !groupFilter.includes('all')
          ? groupFilter[0]
          : undefined,
      status:
        statusFilter.length > 0 && !statusFilter.includes('all')
          ? statusFilter[0]
          : undefined,
      type:
        typeFilter.length > 0 && !typeFilter.includes('all')
          ? Number(typeFilter[0])
          : undefined,
      tag_mode: enableTagMode,
      id_sort: idSort,
      ...sortParams,
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
    }), actor],
    enabled: canRead,
    queryFn: async () => {
      const result = shouldSearch
        ? await searchChannels({
            keyword: globalFilter,
            model: modelFilter,
            group:
              groupFilter.length > 0 && !groupFilter.includes('all')
                ? groupFilter[0]
                : undefined,
            status:
              statusFilter.length > 0 && !statusFilter.includes('all')
                ? statusFilter[0]
                : undefined,
            type:
              typeFilter.length > 0 && !typeFilter.includes('all')
                ? Number(typeFilter[0])
                : undefined,
            tag_mode: enableTagMode,
            id_sort: idSort,
            ...sortParams,
            p: pagination.pageIndex + 1,
            page_size: pagination.pageSize,
          })
        : await getChannels({
            group:
              groupFilter.length > 0 && !groupFilter.includes('all')
                ? groupFilter[0]
                : undefined,
            status:
              statusFilter.length > 0 && !statusFilter.includes('all')
                ? statusFilter[0]
                : undefined,
            type:
              typeFilter.length > 0 && !typeFilter.includes('all')
                ? Number(typeFilter[0])
                : undefined,
            tag_mode: enableTagMode,
            id_sort: idSort,
            ...sortParams,
            p: pagination.pageIndex + 1,
            page_size: pagination.pageSize,
          })

      if (!result.success || !result.data || !Array.isArray(result.data.items) || !Number.isSafeInteger(result.data.total) || result.data.total < 0 || (!enableTagMode && result.data.total < result.data.items.length)) {
        throw new Error('Failed to load channels')
      }

      return result
    },
  })

  // Apply tag aggregation if tag mode is enabled
  const channels = useMemo(() => {
    const rawChannels = isError || !canRead ? [] : data?.data?.items || []

    if (enableTagMode && rawChannels.length > 0) {
      return aggregateChannelsByTag(rawChannels)
    }

    return rawChannels
  }, [data, enableTagMode, isError, canRead])

  const knownTotal = isError || isFetching || !canRead ? undefined : data?.data?.total
  const totalCount = knownTotal ?? 0
  const connectionReturn: ConnectionReturn = {
    ...tableToConnectionsSearch({
      ...search,
      page: pagination.pageIndex + 1,
      pageSize: pagination.pageSize,
      filter: globalFilter,
      model: modelFilter,
      status: statusFilter,
      type: typeFilter,
      group: groupFilter,
    }),
    view: 'table',
    quotaChannelId: search.quotaChannelId,
  }
  const typeCounts = isError ? undefined : data?.data?.type_counts

  // Columns configuration
  const columns = useChannelsColumns({
    enableSelection: batchMode,
    tagMode: enableTagMode,
  })

  // React Table instance
  const { table } = useDataTable({
    data: channels,
    columns,
    totalCount,
    sorting: effectiveSorting,
    initialColumnVisibility: {
      models: false,
      tag: false,
    },
    columnVisibilityStorageKey: CHANNELS_COLUMN_VISIBILITY_STORAGE_KEY,
    columnSizingStorageKey: isMobile
      ? false
      : CHANNELS_COLUMN_SIZING_STORAGE_KEY,
    columnFilters,
    pagination,
    globalFilter,
    enableRowSelection: batchMode
      ? (row: Row<Channel>) => !isTagAggregateRow(row.original)
      : false,
    onSortingChange: handleSortingChange,
    onColumnFiltersChange: handleColumnFiltersChange,
    onPaginationChange,
    onGlobalFilterChange,
    getRowId: getChannelTableRowId,
    getSubRows: (row: Channel & { children?: Channel[] }) => row.children,
    manualPagination: true,
    manualSorting: true,
    manualFiltering: true,
    withExpandedRowModel: true,
    enableColumnResizing: !isMobile,
    ensurePageInRange: knownTotal === undefined ? undefined : ensurePageInRange,
  })

  useEffect(() => {
    if (!batchMode) {
      table.resetRowSelection()
    }
  }, [batchMode, table])

  // Prepare filter options from existing channel types only.
  const typeFilterOptions = useMemo(() => {
    const counts = typeCounts || {}
    const typeIds = Object.entries(counts)
      .map(([type, count]) => ({
        type: Number(type),
        count: Number(count) || 0,
      }))
      .filter((item) => item.type > 0 && item.count > 0)
      .sort((a, b) => {
        const labelA = t(getChannelTypeLabel(a.type))
        const labelB = t(getChannelTypeLabel(b.type))
        return labelA.localeCompare(labelB)
      })

    const selectedType = typeFilter.find((value) => value !== 'all')
    if (selectedType) {
      const selectedTypeId = Number(selectedType)
      const alreadyIncluded = typeIds.some(
        (item) => item.type === selectedTypeId
      )
      if (selectedTypeId > 0 && !alreadyIncluded) {
        typeIds.push({
          type: selectedTypeId,
          count: Number(counts[selectedType]) || 0,
        })
      }
    }

    const totalTypes = Object.values(counts).reduce(
      (sum, count) => sum + (Number(count) || 0),
      0
    )

    return [
      {
        label: 'All Types',
        value: 'all',
        count: totalTypes,
      },
      ...typeIds.map((item) => {
        const iconName = getChannelTypeIcon(item.type)
        return {
          label: getChannelTypeLabel(item.type),
          value: String(item.type),
          count: item.count,
          iconNode: getLobeIcon(`${iconName}.Color`, 16),
        }
      }),
    ]
  }, [t, typeCounts, typeFilter])

  const groupFilterOptions = [
    { label: t('All Groups'), value: 'all' },
    ...groupOptions.map((option) => ({
      ...option,
      label: sensitiveVisible ? option.label : '••••',
    })),
  ]

  // Back from a connection's page puts focus on the name that opened it.
  useReturnFocus({
    attribute: 'data-connection-link',
    requestedId: search.focusConnectionId,
    settled: !isLoading && !isFetching,
    rows: data,
  })
  // A pointer anywhere on a channel row opens its connection page, except on
  // the row's own controls; tag rows only expand.
  const openFromRow = (event: MouseEvent<HTMLElement>, channel: Channel) => {
    const target = event.target as HTMLElement
    if (isTagAggregateRow(channel) || !event.currentTarget.contains(target)) {
      return
    }
    if (
      target.closest('button,a,input,label,[data-row-actions],[role="checkbox"],[role="switch"]')
    ) {
      return
    }
    rememberFocus('data-connection-link', channel.id)
    void navigate({
      to: '/connections/$connectionId',
      params: { connectionId: String(channel.id) },
      state: { inspectorBack: true },
      search: { connectionReturn },
    })
  }

  if (!canRead) {
    return <Alert variant='destructive'><AlertDescription>{t('Connection access could not be verified.')}</AlertDescription></Alert>
  }

  return (
    <ConnectionListContext value={connectionReturn}>
      <div className='mb-4 flex min-w-0 flex-wrap items-center justify-between gap-3'>
        <ConnectionViewSwitcher view='table' search={connectionReturn} showOther={showCards} />
        <p className='text-caption text-muted-foreground'>{t('Advanced connection controls')}</p>
      </div>
      {enableTagMode ? (
        <Alert className='mb-3'>
          <AlertDescription>
            {t(
              'Priority and weight sorting are unavailable in tag aggregation mode because each row represents multiple channels.'
            )}
          </AlertDescription>
        </Alert>
      ) : null}
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        errorState={
          isError ? (
            <ErrorState
              title={t('Failed to load channels')}
              description={t('Please try again later.')}
              onRetry={() => {
                if (!isFetching) void refetch()
              }}
            />
          ) : undefined
        }
        emptyTitle={t('No Channels Found')}
        emptyDescription={t(
          'No channels available. Create your first channel to get started.'
        )}
        skeletonKeyPrefix='channel-skeleton'
        enableCardView
        viewModeStorageKey={CHANNELS_VIEW_MODE_STORAGE_KEY}
        renderCard={(row, { isSelected }) => (
          <ChannelCard row={row} isSelected={isSelected} />
        )}
        cardGridClassName='grid grid-cols-1 gap-3 sm:gap-4 lg:grid-cols-3'
        applyHeaderSize
        fixedHeight={false}
        pagination={!isError ? (
          <LogPagination
            label={t('Connections')}
            page={pagination.pageIndex + 1}
            pageSize={pagination.pageSize}
            total={knownTotal}
            busy={isLoading || isFetching}
            onPageChange={(page) => onPaginationChange({ ...pagination, pageIndex: page - 1 })}
            onPageSizeChange={(pageSize) => onPaginationChange({ pageIndex: 0, pageSize })}
          />
        ) : null}
        toolbarProps={{
          searchPlaceholder: t('Filter by name, ID, or key...'),
          searchDebounceMs: 500,
          onReset: () => {
            resetModelFilterInput()
          },
          additionalSearch: (
            <Input
              aria-label={t('Model')}
              placeholder={t('Filter by model...')}
              value={modelFilterInput}
              onChange={onModelFilterInputChange}
              onCompositionStart={onModelFilterCompositionStart}
              onCompositionEnd={onModelFilterCompositionEnd}
              className='w-full sm:w-[150px] lg:w-[180px]'
            />
          ),
          filters: [
            {
              columnId: 'status',
              title: t('Status'),
              options: [...CHANNEL_STATUS_OPTIONS],
              singleSelect: true,
            },
            {
              columnId: 'type',
              title: t('Type'),
              options: typeFilterOptions,
              singleSelect: true,
            },
            {
              columnId: 'group',
              title: t('Group'),
              options: groupFilterOptions,
              singleSelect: true,
            },
          ],
          preActions: (
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant='ghost'
                    size='icon'
                    onClick={() => setSensitiveVisible(!sensitiveVisible)}
                    aria-label={sensitiveVisible ? t('Hide') : t('Show')}
                    className='text-muted-foreground hover:text-foreground size-8'
                  />
                }
              >
                {sensitiveVisible ? <Eye /> : <EyeOff />}
              </TooltipTrigger>
              <TooltipContent>
                {sensitiveVisible ? t('Hide') : t('Show')}
              </TooltipContent>
            </Tooltip>
          ),
        }}
        renderRow={(row, helpers) => (
          <DataTableRow
            key={row.id}
            row={row}
            className={cn(
              !isTagAggregateRow(row.original) && 'cursor-pointer',
              isDisabledChannelRow(row.original) && DISABLED_ROW_DESKTOP
            )}
            getColumnClassName={(columnId) =>
              helpers.getCellClassName(columnId)
            }
            cellRenderColumns={table.options.columns}
            onClick={(event) => openFromRow(event, row.original)}
          />
        )}
        getRowClassName={(row, { isMobile }) => {
          if (!isDisabledChannelRow(row.original)) {
            return undefined
          }
          if (isMobile) {
            return DISABLED_ROW_MOBILE
          }
          return DISABLED_ROW_DESKTOP
        }}
        bulkActions={batchMode ? <DataTableBulkActions table={table} /> : null}
      />
    </ConnectionListContext>
  )
}
