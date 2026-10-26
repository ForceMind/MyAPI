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
import { getRouteApi } from '@tanstack/react-router'
import {
  flexRender,
  type OnChangeFn,
  type SortingState,
  type Table as TanstackTable,
} from '@tanstack/react-table'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
  useDataTable,
} from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import { StatusBadge } from '@/components/status-badge'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { cn } from '@/lib/utils'

import { getUsers, searchUsers } from '../api'
import {
  USER_ROLES,
  USER_STATUS,
  USER_STATUSES,
  getUserStatusOptions,
  getUserRoleOptions,
  isUserDeleted,
} from '../constants'
import { getAccountTierLabel } from '../lib'
import type { User, UserSortBy } from '../types'
import { DataTableBulkActions } from './data-table-bulk-actions'
import { DataTableRowActions } from './data-table-row-actions'
import { useUsersColumns } from './users-columns'
import { useUsers } from './users-provider'

const route = getRouteApi('/_authenticated/users/')
const USERS_MOBILE_SKELETON_IDS = Array.from(
  { length: 5 },
  (_, index) => `user-mobile-skeleton-${index + 1}`
)

const USER_SORTABLE_COLUMNS = new Set<UserSortBy>([
  'id',
  'username',
  'quota',
  'group',
  'created_at',
  'last_login_at',
])

function isDisabledUserRow(user: User) {
  return isUserDeleted(user) || user.status === USER_STATUS.DISABLED
}

/**
 * Phone and tablet records (design direction 3, same form as the API Key
 * list): name and status, then ID, role and account tier, then the usage or
 * quota summary; the row's actions stay on its right.
 */
function UsersMobileList(props: {
  table: TanstackTable<User>
  isLoading: boolean
}) {
  const { t } = useTranslation()
  const rows = props.table.getRowModel().rows

  if (props.isLoading) {
    return (
      <div className='divide-border overflow-hidden rounded-lg border'>
        {USERS_MOBILE_SKELETON_IDS.map((id) => (
          <div
            key={id}
            className='flex flex-col gap-2 border-b px-3 py-2.5 last:border-b-0'
          >
            <div className='flex items-center justify-between'>
              <Skeleton className='h-4 w-32' />
              <Skeleton className='h-5 w-16 rounded-md' />
            </div>
            <Skeleton className='h-3 w-40' />
            <Skeleton className='h-12 w-full' />
          </div>
        ))}
      </div>
    )
  }

  if (!rows.length) {
    return (
      <div className='rounded-lg border p-8'>
        <Empty className='border-none p-0'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <HugeiconsIcon icon={Database01Icon} className='size-6' />
            </EmptyMedia>
            <EmptyTitle>{t('No Users Found')}</EmptyTitle>
            <EmptyDescription>
              {t('No users available. Try adjusting your search or filters.')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      </div>
    )
  }

  return (
    <div className='divide-border overflow-hidden rounded-lg border'>
      {rows.map((row) => {
        const user = row.original
        const statusConfig = isUserDeleted(user)
          ? USER_STATUSES[USER_STATUS.DELETED]
          : USER_STATUSES[user.status as keyof typeof USER_STATUSES]
        const roleConfig = USER_ROLES[user.role as keyof typeof USER_ROLES]
        // The usage cell carries the funding and usage-policy rules of the
        // table column, so the record renders that same cell.
        const quotaCell = row
          .getVisibleCells()
          .find((cell) => cell.column.id === 'quota')

        return (
          <div
            key={row.id}
            className={cn(
              'bg-card flex min-w-0 items-start gap-2 border-b px-3 py-2.5 last:border-b-0',
              isDisabledUserRow(user) && DISABLED_ROW_MOBILE
            )}
          >
            <div className='flex min-w-0 flex-1 flex-col gap-1.5'>
              <div className='flex min-w-0 items-start justify-between gap-2'>
                <div className='min-w-0'>
                  <div
                    className='truncate text-sm font-semibold'
                    title={user.username}
                  >
                    {user.username}
                  </div>
                  {user.display_name &&
                    user.display_name !== user.username && (
                      <div
                        className='text-caption text-muted-foreground truncate'
                        title={user.display_name}
                      >
                        {user.display_name}
                      </div>
                    )}
                </div>
                {statusConfig && (
                  <StatusBadge
                    label={t(statusConfig.labelKey)}
                    variant={statusConfig.variant}
                    copyable={false}
                    className='shrink-0'
                  />
                )}
              </div>
              <div className='text-caption text-muted-foreground flex min-w-0 flex-wrap gap-x-2'>
                <span className='font-mono'>#{user.id}</span>
                {roleConfig && <span>{t(roleConfig.labelKey)}</span>}
                <span>{getAccountTierLabel(user.group, t)}</span>
                {user.remark && (
                  <span className='max-w-full truncate' title={user.remark}>
                    {user.remark}
                  </span>
                )}
              </div>
              {quotaCell && (
                <div className='min-w-0 text-xs'>
                  {flexRender(
                    quotaCell.column.columnDef.cell,
                    quotaCell.getContext()
                  )}
                </div>
              )}
            </div>
            <DataTableRowActions row={row} />
          </div>
        )
      })}
    </div>
  )
}

export function UsersTable() {
  const { t } = useTranslation()
  const columns = useUsersColumns()
  const { refreshTrigger } = useUsers()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [sorting, setSorting] = useState<SortingState>([])

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'status', searchKey: 'status', type: 'array' },
      { columnId: 'role', searchKey: 'role', type: 'array' },
      { columnId: 'group', searchKey: 'group', type: 'string' },
    ],
  })
  const statusFilter =
    (columnFilters.find((filter) => filter.id === 'status')?.value as
      | string[]
      | undefined) ?? []
  const roleFilter =
    (columnFilters.find((filter) => filter.id === 'role')?.value as
      | string[]
      | undefined) ?? []
  const groupFilter =
    (columnFilters.find((filter) => filter.id === 'group')?.value as string) ??
    ''

  const sortParams = useMemo(() => {
    const activeSort = sorting[0]
    if (
      !activeSort ||
      !USER_SORTABLE_COLUMNS.has(activeSort.id as UserSortBy)
    ) {
      return {}
    }

    return {
      sort_by: activeSort.id as UserSortBy,
      sort_order: activeSort.desc ? 'desc' : 'asc',
    } as const
  }, [sorting])

  const handleSortingChange: OnChangeFn<SortingState> = (updater) => {
    setSorting(updater)
    if (pagination.pageIndex > 0) {
      onPaginationChange({ ...pagination, pageIndex: 0 })
    }
  }

  // Fetch data with React Query
  const { data, isError, isLoading, isFetching, refetch } = useQuery({
    queryKey: [
      'users',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      statusFilter,
      roleFilter,
      groupFilter,
      sortParams,
      refreshTrigger,
    ],
    queryFn: async () => {
      const hasFilter = globalFilter?.trim()
      const hasColumnFilter =
        statusFilter.length > 0 || roleFilter.length > 0 || Boolean(groupFilter)
      const params = {
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        ...sortParams,
      }

      const result =
        hasFilter || hasColumnFilter
          ? await searchUsers({
              ...params,
              keyword: globalFilter,
              status: statusFilter[0] ?? '',
              role: roleFilter[0] ?? '',
              group: groupFilter,
            })
          : await getUsers(params)

      if (!result.success) {
        throw new Error('Failed to load users')
      }

      return {
        items: result.data?.items || [],
        total: result.data?.total || 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const users = isError ? [] : data?.items || []

  const { table } = useDataTable({
    data: users,
    columns,
    enableRowSelection: true,
    columnFilters,
    globalFilter,
    pagination,
    sorting,
    globalFilterFn: (row, _columnId, filterValue) => {
      const searchValue = String(filterValue).toLowerCase()
      const fields = [
        row.getValue('username'),
        row.original.display_name,
        row.original.email,
      ]
      return fields.some((field) =>
        String(field || '')
          .toLowerCase()
          .includes(searchValue)
      )
    },
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    onSortingChange: handleSortingChange,
    manualPagination: true,
    manualFiltering: true,
    manualSorting: true,
    totalCount: isError ? 0 : data?.total || 0,
    ensurePageInRange: isError ? undefined : ensurePageInRange,
  })

  return (
    <DataTablePage
      table={table}
      mobileQuery={CONSOLE_MOBILE_QUERY}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      errorState={
        isError ? (
          <ErrorState
            title={t('Failed to load users')}
            description={t('Please try again later.')}
            onRetry={() => {
              if (!isFetching) void refetch()
            }}
          />
        ) : undefined
      }
      emptyTitle={t('No Users Found')}
      emptyDescription={t(
        'No users available. Try adjusting your search or filters.'
      )}
      skeletonKeyPrefix='users-skeleton'
      applyHeaderSize
      toolbarProps={{
        searchPlaceholder: t('Filter by username, name or email...'),
        searchDebounceMs: 500,
        filters: [
          {
            columnId: 'status',
            title: t('Status'),
            options: getUserStatusOptions(t),
            singleSelect: true,
          },
          {
            columnId: 'role',
            title: t('Role'),
            options: getUserRoleOptions(t),
            singleSelect: true,
          },
        ],
      }}
      mobile={<UsersMobileList table={table} isLoading={isLoading} />}
      getRowClassName={(row) =>
        isDisabledUserRow(row.original) ? DISABLED_ROW_DESKTOP : undefined
      }
      bulkActions={<DataTableBulkActions table={table} />}
    />
  )
}
