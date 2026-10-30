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
import type { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  DataTablePage,
  DataTableRow,
  useDataTable,
} from '@/components/data-table'
import { LogPagination } from '@/components/log-pagination'
import { getLogPageCount, normalizeLogPagination } from '@/lib/log-pagination'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import {
  LOG_TYPE_ALL_VALUE,
  LOG_TYPE_ENUM,
} from '../constants'
import { useColumnsByCategory } from '../lib/columns'
import { parseLogOther } from '../lib/format'
import { fetchLogsByCategory, isCompletedUsageReview } from '../lib/utils'
import type { LogCategory } from '../types'
import { CommonLogsFilterBar } from './common-logs-filter-bar'
import { TaskLogsFilterBar } from './task-logs-filter-bar'
import { UsageLogsMobileList } from './usage-logs-mobile-card'
import { useLogsViewScope } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

const logTypeRowTint: Record<number, string> = {
  [LOG_TYPE_ENUM.ERROR]: 'bg-destructive-muted/50',
  [LOG_TYPE_ENUM.REFUND]: 'bg-info-muted/50',
}
const usageReviewRowTint = 'bg-warning-muted/50'

// Warning tint for logs where a quota conversion saturated (admin-only marker).
// Takes precedence over the per-type tint since it flags a billing anomaly.
const quotaSaturationRowTint = 'bg-warning-muted'

function getColumnVisibilityStorageKey(
  logCategory: LogCategory,
  isAdmin: boolean
): string {
  return `usage-logs:${logCategory}:${isAdmin ? 'admin' : 'user'}:column-visibility`
}

function deserializeLogTypeFilter(value: unknown): unknown[] {
  let values: unknown[] = []
  if (Array.isArray(value)) {
    values = value
  } else if (value) {
    values = [value]
  }
  return values.filter((item) => String(item) !== LOG_TYPE_ALL_VALUE)
}

interface UsageLogsTableProps {
  logCategory: LogCategory
}

export function UsageLogsTable(props: UsageLogsTableProps) {
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const { isAdminView } = useLogsViewScope()
  return <UsageLogsTableContent key={JSON.stringify([props.logCategory, userId, sessionId, isAdminView])} {...props} />
}

function UsageLogsTableContent({ logCategory }: UsageLogsTableProps) {
  const { t } = useTranslation()
  const { isAdminView: isAdmin } = useLogsViewScope()
  const isMobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const searchParams = route.useSearch()
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)

  const {
    columnFilters,
    onColumnFiltersChange,
    pagination: urlPagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 20 : 100 },
    globalFilter: { enabled: false },
    columnFilters: [
      {
        columnId: 'created_at',
        searchKey: 'type',
        type: 'array' as const,
        deserialize: deserializeLogTypeFilter,
      },
      { columnId: 'model_name', searchKey: 'model', type: 'string' as const },
      { columnId: 'token_name', searchKey: 'token', type: 'string' as const },
      { columnId: 'group', searchKey: 'group', type: 'string' as const },
      ...(isAdmin
        ? [
            {
              columnId: 'channel',
              searchKey: 'channel',
              type: 'string' as const,
            },
            {
              columnId: 'username',
              searchKey: 'username',
              type: 'string' as const,
            },
          ]
        : []),
    ],
  })

  const pagination = normalizeLogPagination(urlPagination.pageIndex, urlPagination.pageSize)

  const { data, error, isError, isLoading, isFetching, isPlaceholderData, refetch } = useQuery({
    queryKey: [
      'logs',
      logCategory,
      isAdmin,
      userId,
      sessionId,
      pagination.pageIndex + 1,
      pagination.pageSize,
      columnFilters,
      searchParams,
      t,
    ],
    queryFn: async () => {
      const result = await fetchLogsByCategory({
        logCategory,
        isAdmin,
        page: pagination.pageIndex + 1,
        pageSize: pagination.pageSize,
        searchParams,
        columnFilters,
      })

      if (!result?.success) {
        const message = result?.message || t('Failed to load logs')
        toast.error(message)
        throw new Error(message)
      }

      if (!result.data) throw new Error(t('Failed to load logs'))
      return result.data
    },
    placeholderData: (previousData, previousQuery) => {
      const previousSearch = previousQuery?.queryKey[8]
      const previousKeyId =
        previousSearch &&
        typeof previousSearch === 'object' &&
        'tokenId' in previousSearch
          ? previousSearch.tokenId
          : undefined
      if (
        previousQuery?.queryKey[1] === logCategory &&
        previousQuery.queryKey[2] === isAdmin &&
        previousQuery.queryKey[3] === userId &&
        previousQuery.queryKey[4] === sessionId &&
        previousKeyId === searchParams.tokenId
      ) {
        return previousData
      }
      return undefined
    },
  })

  // TanStack Query keeps the last successful data when a refetch fails. Do
  // not render that stale, potentially privileged log data alongside an auth
  // or permission error; the mobile slot already renders ErrorState directly.
  const currentData = isError || isPlaceholderData ? undefined : data
  const logs = currentData?.items || []
  const totalPages = getLogPageCount(currentData?.total, pagination.pageSize)
  const total = totalPages === null ? undefined : currentData?.total
  const columns = useColumnsByCategory(logCategory, isAdmin)
  const isLoadingData = isLoading || isPlaceholderData || (isFetching && !data)
  const errorMessage =
    error instanceof Error ? error.message : t('Failed to load logs')

  const { table } = useDataTable({
    data: logs as Record<string, unknown>[],
    columns: columns as ColumnDef<Record<string, unknown>>[],
    columnFilters,
    columnVisibilityStorageKey: getColumnVisibilityStorageKey(
      logCategory,
      isAdmin
    ),
    pagination,
    enableRowSelection: false,
    onPaginationChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: total,
    pageCount: totalPages ?? -1,
    ensurePageInRange: totalPages !== null && !isFetching
      ? (count) => ensurePageInRange(count, { resetTo: 'last' })
      : undefined,
  })

  const isCommon = logCategory === 'common'

  return (
    <DataTablePage
      table={table}
      pagination={
        <LogPagination
          label={t('Log pagination')}
          page={pagination.pageIndex + 1}
          pageSize={pagination.pageSize}
          total={total}
          busy={isFetching || isLoading}
          onPageChange={(page) => {
            if (totalPages === null || !Number.isSafeInteger(page)) return
            onPaginationChange({ pageIndex: Math.min(totalPages, Math.max(1, page)) - 1, pageSize: pagination.pageSize })
          }}
          onPageSizeChange={(pageSize) => onPaginationChange({ pageIndex: 0, pageSize })}
        />
      }
      mobileQuery={CONSOLE_MOBILE_QUERY}
      columns={columns as ColumnDef<Record<string, unknown>>[]}
      isLoading={isLoadingData}
      isFetching={isFetching}
      // The mobile list is content-sized and must not inherit the desktop
      // h-full flex contract. On a phone that contract can resolve to zero
      // height inside the scrollable page, leaving only the filter toolbar
      // visible. Keep the bounded flex viewport for desktop only.
      fixedHeight={!isMobile}
      emptyTitle={isError ? t('Failed to load logs') : t('No Logs Found')}
      emptyDescription={
        isError
          ? errorMessage
          : t(
              'No usage logs available. Logs will appear here once API calls are made.'
            )
      }
      emptyAction={
        isError ? (
          <Button variant='outline' size='sm' onClick={() => void refetch()}>
            {t('Retry')}
          </Button>
        ) : undefined
      }
      skeletonKeyPrefix='usage-log-skeleton'
      applyHeaderSize
      mobile={
        isError ? (
          <ErrorState
            title={t('Failed to load logs')}
            description={errorMessage}
            onRetry={() => void refetch()}
          />
        ) : (
          <UsageLogsMobileList
            table={table}
            isLoading={isLoadingData}
            logCategory={logCategory}
          />
        )
      }
      toolbar={
        isCommon ? (
          <CommonLogsFilterBar table={table} />
        ) : (
          <TaskLogsFilterBar table={table} logCategory={logCategory} />
        )
      }
      renderRow={(row) => {
        const logType = (row.original as Record<string, unknown>).type as
          | number
          | undefined
        let tintClass =
          isCommon && logType != null ? (logTypeRowTint[logType] ?? '') : ''
        if (
          isCommon &&
          isCompletedUsageReview(
            logType ?? LOG_TYPE_ENUM.UNKNOWN,
            parseLogOther(
              ((row.original as Record<string, unknown>).other as string) ?? ''
            )
          )
        ) {
          tintClass = usageReviewRowTint
        }
        const outcome = parseLogOther(
          ((row.original as Record<string, unknown>).other as string) ?? ''
        )
        if (
          isCommon &&
          logType === 5 &&
          outcome?.request_outcome === 'completed' &&
          !isCompletedUsageReview(logType, outcome)
        ) {
          tintClass = ''
        }
        if (isCommon && isAdmin) {
          const other = parseLogOther(
            ((row.original as Record<string, unknown>).other as string) ?? ''
          )
          if (other?.admin_info?.quota_saturation) {
            tintClass = quotaSaturationRowTint
          }
        }

        return (
          <DataTableRow
            key={row.id}
            row={row}
            className={cn('transition-colors', tintClass)}
          />
        )
      }}
    />
  )
}
