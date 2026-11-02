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
import { getRouteApi } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
  useDataTable,
} from '@/components/data-table'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { ErrorState } from '@/components/error-state'
import { LogPagination } from '@/components/log-pagination'
import { getLogPageCount, normalizeLogPagination } from '@/lib/log-pagination'
import { useAuthStore } from '@/stores/auth-store'

import { getRedemptions, searchRedemptions } from '../api'
import {
  ERROR_MESSAGES,
  REDEMPTION_STATUS,
  getRedemptionStatusOptions,
} from '../constants'
import { isRedemptionExpired } from '../lib'
import { type Redemption, redemptionSchema } from '../types'
import { DataTableBulkActions } from './data-table-bulk-actions'
import { useRedemptionsColumns } from './redemptions-columns'
import { RedemptionsMobileList } from './redemptions-mobile-list'
import { useRedemptions } from './redemptions-provider'

const route = getRouteApi('/_authenticated/redemption-codes/')

function isDisabledRedemptionRow(redemption: Redemption) {
  return (
    redemption.status !== REDEMPTION_STATUS.ENABLED ||
    isRedemptionExpired(redemption.expired_time, redemption.status)
  )
}

export function RedemptionsTable() {
  const { t } = useTranslation()
  const columns = useRedemptionsColumns()
  const { refreshTrigger } = useRedemptions()
  const isMobile = useMediaQuery('(max-width: 640px)')

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
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [{ columnId: 'status', searchKey: 'status', type: 'array' }],
  })
  const pagination = normalizeLogPagination(urlPagination.pageIndex, urlPagination.pageSize)
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  const statusFilter =
    (columnFilters.find((filter) => filter.id === 'status')?.value as
      | string[]
      | undefined) ?? []
  const statusFilterValue = statusFilter[0] ?? ''

  // Fetch data with React Query
  const { data, isLoading, isFetching, isError, error, refetch } = useQuery({
    queryKey: [
      'redemptions',
      identity,
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      statusFilterValue,
      refreshTrigger,
    ],
    queryFn: async () => {
      const hasFilter = globalFilter?.trim()
      const hasStatusFilter = statusFilterValue !== ''
      const params = {
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
      }

      const result =
        hasFilter || hasStatusFilter
          ? await searchRedemptions({
              ...params,
              keyword: globalFilter,
              status: statusFilterValue,
            })
          : await getRedemptions(params)

      if (!result.success) throw new Error(result.message || t(hasFilter || hasStatusFilter ? ERROR_MESSAGES.SEARCH_FAILED : ERROR_MESSAGES.LOAD_FAILED))
      const items = result.data?.items
      if (items !== null && !Array.isArray(items)) throw new Error(t('Invalid redemption list response'))
      const rows = items ?? []
      if (rows.some((row) => !redemptionSchema.safeParse(row).success)) throw new Error(t('Invalid redemption list response'))
      return { items: rows, total: result.data?.total }
    },
  })

  const redemptions = isError ? [] : data?.items ?? []
  const returnedTotal = data?.total
  const totalCount = !isError && typeof returnedTotal === 'number' && Number.isSafeInteger(returnedTotal) && returnedTotal >= redemptions.length && returnedTotal >= 0 ? returnedTotal : undefined
  const pageCount = getLogPageCount(totalCount, pagination.pageSize)

  const { table } = useDataTable({
    data: redemptions,
    columns,
    getRowId: (row) => String(row.id),
    pageCount: pageCount ?? -1,
    enableRowSelection: true,
    columnFilters,
    globalFilter,
    pagination,
    globalFilterFn: (row, _columnId, filterValue) => {
      const name = String(row.getValue('name')).toLowerCase()
      const id = String(row.getValue('id'))
      const searchValue = String(filterValue).toLowerCase()

      return name.includes(searchValue) || id.includes(searchValue)
    },
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount,
    ensurePageInRange: totalCount === undefined ? undefined : ensurePageInRange,
  })

  const redemptionStatusOptions = useMemo(
    () => getRedemptionStatusOptions(t),
    [t]
  )

  let emptyTitle = t('No Redemption Codes Found')
  let emptyDescription = t('No redemption codes available. Create your first redemption code to get started.')
  if (totalCount === undefined) {
    emptyTitle = t('No redemption codes on this page')
    emptyDescription = t('The returned page is empty. The record total is unavailable.')
  } else if (globalFilter || statusFilterValue) {
    emptyTitle = t('No matches found')
    emptyDescription = t('Try adjusting your search or filters.')
  }

  return (
    <>
    <h2 data-redemption-list-heading tabIndex={-1} className='sr-only'>{t('Redemption code records')}</h2>
    <DataTablePage
      table={table}
      fixedHeight={!isMobile}
      mobileQuery={CONSOLE_MOBILE_QUERY}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      errorState={isError ? <ErrorState title={t('Failed to load redemption codes')} description={error.message} onRetry={() => { if (!isFetching) void refetch() }} /> : undefined}
      pagination={<LogPagination label={t('Redemption codes pagination')} page={pagination.pageIndex + 1} pageSize={pagination.pageSize} total={totalCount} busy={isLoading || isFetching} onPageChange={(page) => onPaginationChange({ pageIndex: page - 1, pageSize: pagination.pageSize })} onPageSizeChange={(pageSize) => onPaginationChange({ pageIndex: 0, pageSize })} />}
      emptyTitle={emptyTitle}
      emptyDescription={emptyDescription}
      skeletonKeyPrefix='redemptions-skeleton'
      applyHeaderSize
      toolbarProps={{
        searchPlaceholder: t('Filter by name or ID...'),
        searchDebounceMs: 500,
        filters: [
          {
            columnId: 'status',
            title: t('Status'),
            options: redemptionStatusOptions,
            singleSelect: true,
          },
        ],
      }}
      mobile={<RedemptionsMobileList table={table} isLoading={isLoading} emptyTitle={emptyTitle} emptyDescription={emptyDescription} />}
      getRowClassName={(row, { isMobile }) => {
        if (!isDisabledRedemptionRow(row.original)) return undefined
        return isMobile ? DISABLED_ROW_MOBILE : DISABLED_ROW_DESKTOP
      }}
      bulkActions={<DataTableBulkActions table={table} />}
    />
    </>
  )
}
