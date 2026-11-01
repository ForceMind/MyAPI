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
import { Database01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi, useLocation } from '@tanstack/react-router'
import { flexRender, type Row, type RowSelectionState, type Table as TanstackTable } from '@tanstack/react-table'
import { useEffect, useState, type MouseEvent } from 'react'
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
import { getLogPageCount, normalizeLogPagination } from '@/lib/log-pagination'
import { useAuthStore } from '@/stores/auth-store'
import { ErrorState } from '@/components/error-state'
import { useReturnFocus } from '@/components/object-page/return-focus'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { useMediaQuery } from '@/hooks/use-media-query'
import { usePersonalWorkbenchMode } from '@/hooks/use-personal-workbench-mode'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { cn } from '@/lib/utils'

import { getApiKeys, searchApiKeys } from '../api'
import {
  API_KEY_STATUS,
  API_KEY_STATUS_OPTIONS,
  ERROR_MESSAGES,
} from '../constants'
import { readKeyListSelection, rememberKeyListSelection } from '../lib/key-list-selection'
import type { ApiKey } from '../types'
import { useApiKeysColumns } from './api-keys-columns'
import { useApiKeys } from './api-keys-provider'
import { DataTableBulkActions } from './data-table-bulk-actions'

const route = getRouteApi('/_authenticated/keys/')
const API_KEYS_COLUMN_VISIBILITY_STORAGE_KEY = 'api-keys:column-visibility'
const EMPTY_KEY_SELECTION: RowSelectionState = {}
// Long names wrap and short columns keep
// their content readable; a narrow window scrolls the table sideways.
const KEY_COLUMN_WIDTH: Record<string, string | undefined> = {
  name: 'max-w-60',
  status: 'min-w-24',
  model_limits: 'min-w-36',
  accessed_time: 'min-w-28',
}
// Name, status, Key, quota, model scope (a blocked scope must stay visible)
// and last use by default (design direction 3); the other columns stay
// available from the view options and on the Key's page.
const API_KEYS_DEFAULT_HIDDEN_COLUMNS = {
  group: false,
  allow_ips: false,
  created_time: false,
  expired_time: false,
}
const API_KEYS_MOBILE_SKELETON_IDS = Array.from(
  { length: 5 },
  (_, index) => `api-key-mobile-skeleton-${index + 1}`
)

function isDisabledApiKeyRow(apiKey: ApiKey) {
  return apiKey.status !== API_KEY_STATUS.ENABLED
}

function ApiKeysMobileSkeleton() {
  const { t } = useTranslation()
  return (
    <div className='divide-border overflow-hidden rounded-lg border' aria-busy='true'>
      <span className='sr-only' role='status'>{t('Loading...')}</span>
      {API_KEYS_MOBILE_SKELETON_IDS.map((id) => (
        <div
          key={id}
          className='space-y-2 border-b px-3 py-2.5 last:border-b-0'
        >
          <div className='flex items-center justify-between'>
            <Skeleton className='h-4 w-32' />
            <Skeleton className='h-5 w-16 rounded-md' />
          </div>
          <div className='flex items-center justify-between gap-3'>
            <Skeleton className='h-7 w-44' />
            <Skeleton className='h-8 w-16' />
          </div>
          <Skeleton className='h-3 w-28' />
        </div>
      ))}
    </div>
  )
}

/** Reuse the table cells so responsive views retain the same facts and actions. */
function ApiKeyMobileCard(props: { row: Row<ApiKey>; showQuota: boolean }) {
  const { t } = useTranslation()
  const { openKey } = useApiKeys()
  const cells = props.row.getAllCells()
  const renderCell = (id: string) => {
    const cell = cells.find((item) => item.column.id === id)
    return cell ? flexRender(cell.column.columnDef.cell, cell.getContext()) : null
  }
  return (
    <article className={cn('bg-card flex min-w-0 flex-col gap-4 rounded-xl border p-4', isDisabledApiKeyRow(props.row.original) && DISABLED_ROW_MOBILE)}>
      <div className='flex min-w-0 items-start gap-3'>
        <span className='inline-flex min-h-11 min-w-11 items-center justify-center [&_[data-slot=checkbox]]:after:-inset-3.5'>{renderCell('select')}</span>
        <div className='min-w-0 flex-1'>
          <h3 className='min-w-0'>{renderCell('name')}</h3>
          {!openKey && <span className='text-caption text-muted-foreground font-mono'>#{props.row.original.id}</span>}
        </div>
        <div className='min-w-0'>{renderCell('status')}</div>
      </div>
      <dl className='grid min-w-0 gap-3 sm:grid-cols-2'>
        <div className='min-w-0'><dt className='text-caption text-muted-foreground'>{t('API Key')}</dt><dd className='min-w-0'>{renderCell('key')}</dd></div>
        <div className='min-w-0'><dt className='text-caption text-muted-foreground'>{t('Models')}</dt><dd className='min-w-0'>{renderCell('model_limits')}</dd></div>
        {props.showQuota && <div className='min-w-0'><dt className='text-caption text-muted-foreground'>{t('Quota')}</dt><dd className='min-w-0'>{renderCell('quota')}</dd></div>}
        <div className='min-w-0'><dt className='text-caption text-muted-foreground'>{t('Last used')}</dt><dd className='min-w-0'>{renderCell('accessed_time')}</dd></div>
      </dl>
      <details className='min-w-0 rounded-lg border p-3'>
        <summary className='text-body flex min-h-11 cursor-pointer items-center font-medium'>{t('More details')}</summary>
        <dl className='mt-3 grid min-w-0 gap-3 sm:grid-cols-2'>
          <div className='min-w-0'><dt className='text-caption text-muted-foreground'>{t('Group')}</dt><dd className='min-w-0'>{renderCell('group')}</dd></div>
          <div className='min-w-0'><dt className='text-caption text-muted-foreground'>{t('IP Restriction')}</dt><dd className='min-w-0'>{renderCell('allow_ips')}</dd></div>
          <div className='min-w-0'><dt className='text-caption text-muted-foreground'>{t('Created')}</dt><dd className='min-w-0'>{renderCell('created_time')}</dd></div>
          <div className='min-w-0'><dt className='text-caption text-muted-foreground'>{t('Expires')}</dt><dd className='min-w-0'>{renderCell('expired_time')}</dd></div>
        </dl>
      </details>
      <div className='min-w-0 border-t pt-3'>{renderCell('actions')}</div>
    </article>
  )
}

function ApiKeysMobileList(props: { table: TanstackTable<ApiKey>; isLoading: boolean; showQuota: boolean }) {
  const { t } = useTranslation()
  if (props.isLoading) return <ApiKeysMobileSkeleton />
  const rows = props.table.getRowModel().rows
  if (!rows.length) {
    return (
      <Empty className='rounded-xl border'>
        <EmptyHeader><EmptyMedia variant='icon'><HugeiconsIcon icon={Database01Icon} aria-hidden='true' /></EmptyMedia><EmptyTitle>{t('No API Keys Found')}</EmptyTitle><EmptyDescription>{t('No API keys available. Create your first API key to get started.')}</EmptyDescription></EmptyHeader>
      </Empty>
    )
  }
  return <div className='grid min-w-0 grid-cols-1 gap-4'>{rows.map((row) => <ApiKeyMobileCard key={row.id} row={row} showQuota={props.showQuota} />)}</div>
}

export function ApiKeysTable() {
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  return <ApiKeysTableSession key={identity} />
}

function ApiKeysTableSession() {
  const { t } = useTranslation()
  const mobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const { refreshTrigger, openKey, requestFocusId } = useApiKeys()
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const role = useAuthStore((state) => state.auth.user?.role ?? 0)
  const sid = useAuthStore((state) => state.auth.session?.sid ?? null)
  const identity = `${userId}:${role}:${sid}`
  const entry = useLocation({ select: (location) => location.state.__TSR_key ?? location.state.key })
  const [selection, setSelection] = useState<{ identity: string; rows: RowSelectionState }>(() => ({ identity, rows: readKeyListSelection(identity, entry) }))
  const selectedRows = selection.identity === identity ? selection.rows : EMPTY_KEY_SELECTION
  useEffect(() => rememberKeyListSelection(identity, entry, selectedRows), [identity, entry, selectedRows])
  const [now, setNow] = useState(() => Date.now())
  // Personal workbench Keys carry no financial quota (funding disabled).
  const showQuota = !usePersonalWorkbenchMode()
  const columns = useApiKeysColumns(now, { showQuota })

  useEffect(() => {
    const intervalId = window.setInterval(() => {
      setNow(Date.now())
    }, 30_000)

    return () => window.clearInterval(intervalId)
  }, [])

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination: urlPagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'status', searchKey: 'status', type: 'array' },
      { columnId: '_tokenSearch', searchKey: 'token', type: 'string' },
    ],
  })

  const pagination = normalizeLogPagination(urlPagination.pageIndex, urlPagination.pageSize)

  const {
    value: tokenFilter,
    inputValue: tokenFilterInput,
    setInputValue: setTokenFilterInput,
  } = useDebouncedColumnFilter({
    columnFilters,
    columnId: '_tokenSearch',
    onColumnFiltersChange,
  })
  const shouldSearch = Boolean(globalFilter?.trim() || tokenFilter.trim())

  // Fetch data with React Query
  // eslint-disable-next-line @tanstack/query/exhaustive-deps
  const { data, isError, isLoading, isFetching, refetch } = useQuery({
    queryKey: [
      'keys',
      userId, role, sid, showQuota,
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      tokenFilter,
      refreshTrigger,
    ],
    queryFn: async () => {
      const result = shouldSearch
        ? await searchApiKeys({
            keyword: globalFilter,
            token: tokenFilter,
            p: pagination.pageIndex + 1,
            size: pagination.pageSize,
          })
        : await getApiKeys({
            p: pagination.pageIndex + 1,
            size: pagination.pageSize,
          })

      if (!result.success || !result.data || !Array.isArray(result.data.items)) {
        throw new Error(ERROR_MESSAGES.LOAD_FAILED)
      }

      return {
        items: result.data.items,
        total: Number.isSafeInteger(result.data.total) && result.data.total >= result.data.items.length ? result.data.total : undefined,
      }
    },
    enabled: userId !== null,
    retry: false,
  })

  const apiKeys = isError || userId === null ? [] : data?.items || []
  const knownTotal = isError || isFetching || userId === null ? undefined : data?.total

  const { table } = useDataTable({
    data: apiKeys,
    columns,
    enableRowSelection: true,
    getRowId: (row) => String(row.id),
    rowSelection: selectedRows,
    onRowSelectionChange: (updater) => {
      const rows = typeof updater === 'function' ? updater(selectedRows) : updater
      rememberKeyListSelection(identity, entry, rows)
      setSelection({ identity, rows })
    },
    columnFilters,
    columnVisibilityStorageKey: API_KEYS_COLUMN_VISIBILITY_STORAGE_KEY,
    initialColumnVisibility: API_KEYS_DEFAULT_HIDDEN_COLUMNS,
    globalFilter,
    pagination,
    globalFilterFn: () => true,
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    manualPagination: true,
    totalCount: knownTotal ?? 0,
    pageCount: getLogPageCount(knownTotal, pagination.pageSize) ?? undefined,
    ensurePageInRange: knownTotal === undefined ? undefined : ensurePageInRange,
  })

  // Back from a Key's page puts focus on the row that opened it.
  useReturnFocus({
    attribute: 'data-inspect-id',
    requestedId: requestFocusId,
    requestedAttribute: mobile ? 'data-inspect-id' : 'data-key-requests',
    settled: !isLoading && !isFetching,
    rows: data,
  })
  // A pointer anywhere on a row opens the Key, except on the row's own
  // controls (and portalled popovers, whose clicks bubble through React).
  const openFromRow = (event: MouseEvent<HTMLElement>, apiKey: ApiKey) => {
    const target = event.target as HTMLElement
    if (!openKey || !event.currentTarget.contains(target)) return
    if (target.closest('button,a,input,label,summary,[data-row-actions],[role="checkbox"]')) return
    openKey(apiKey)
  }

  return (
    <div className='flex min-w-0 flex-col gap-4'>
      <p className='text-caption text-muted-foreground'>{t('Status filters apply to the current page. Name and credential searches cover your accessible Keys.')}</p>
      <DataTablePage
        table={table}
        fixedHeight={false}
        mobileQuery={CONSOLE_MOBILE_QUERY}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        errorState={
          isError ? (
            <ErrorState
              title={t('Failed to load API keys')}
              description={t('Please try again later.')}
              onRetry={() => {
                if (!isFetching) void refetch()
              }}
            />
          ) : undefined
        }
        emptyTitle={t('No API Keys Found')}
        emptyDescription={t(
          'No API keys available. Create your first API key to get started.'
        )}
        skeletonKeyPrefix='api-keys-skeleton'
        applyHeaderSize
        getColumnClassName={(columnId) => KEY_COLUMN_WIDTH[columnId]}
        pagination={!isError ? <LogPagination label={t('API Keys')} page={pagination.pageIndex + 1} pageSize={pagination.pageSize} total={knownTotal} busy={isLoading || isFetching} onPageChange={(page) => onPaginationChange({ ...pagination, pageIndex: page - 1 })} onPageSizeChange={(pageSize) => onPaginationChange({ pageIndex: 0, pageSize })} /> : null}
        toolbarProps={{
          searchPlaceholder: t('Filter by name...'),
          searchDebounceMs: 500,
          additionalSearch: (
            <Input
              placeholder={t('Filter by API key...')}
              aria-label={t('Filter by API key...')}
              value={tokenFilterInput}
              onChange={(e) => setTokenFilterInput(e.target.value)}
              className='w-full sm:w-50 lg:w-60'
            />
          ),
          filters: [
            {
              columnId: 'status',
              title: t('Status'),
              options: API_KEY_STATUS_OPTIONS,
              singleSelect: true,
            },
          ],
        }}
        mobile={
          <ApiKeysMobileList
            table={table}
            isLoading={isLoading}
            showQuota={showQuota}
          />
        }
        renderRow={(row, helpers) => (
          <DataTableRow
            key={row.id}
            row={row}
            className={cn(
              openKey && 'cursor-pointer',
              isDisabledApiKeyRow(row.original) && DISABLED_ROW_DESKTOP
            )}
            getColumnClassName={(columnId) =>
              helpers.getCellClassName(columnId)
            }
            cellRenderColumns={table.options.columns}
            onClick={(event) => openFromRow(event, row.original)}
          />
        )}
        afterTable={mobile ? <div className='max-sm:[&_button]:min-h-11 max-sm:[&_button]:min-w-11 pointer-coarse:[&_button]:min-h-11 pointer-coarse:[&_button]:min-w-11'><DataTableBulkActions table={table} /></div> : undefined}
        bulkActions={<DataTableBulkActions table={table} />}
      />
    </div>
  )
}
