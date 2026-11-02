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
import { flexRender, type Row, type Table as TanstackTable, type ColumnFiltersState, type PaginationState } from '@tanstack/react-table'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import { getCellLabel } from '@/components/data-table/layout/card-cell-utils'
import { LogPagination } from '@/components/log-pagination'
import { useAuthStore } from '@/stores/auth-store'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { useMediaQuery } from '@/hooks/use-media-query'

import { getAdminPlans } from '../api'
import type { PlanRecord } from '../types'
import { useSubscriptionsColumns } from './subscriptions-columns'
import { useSubscriptions } from './subscriptions-provider'

const PLANS_MOBILE_SKELETON_IDS = Array.from(
  { length: 4 },
  (_, index) => `plan-mobile-skeleton-${index + 1}`
)

/**
 * Phone and tablet records (design direction 3, as the users list): plan
 * name and status, then ID, price, validity, reset period and quota; the
 * row's actions follow on the right.
 */
function SubscriptionsMobileList(props: {
  table: TanstackTable<PlanRecord>
  isLoading: boolean
  emptyTitle?: string
  emptyDescription?: string
}) {
  const { t } = useTranslation()
  const rows = props.table.getRowModel().rows

  if (props.isLoading) {
    return (
      <div className='divide-border overflow-hidden rounded-lg border'>
        {PLANS_MOBILE_SKELETON_IDS.map((id) => (
          <div
            key={id}
            className='flex flex-col gap-2 border-b px-3 py-2.5 last:border-b-0'
          >
            <div className='flex items-center justify-between'>
              <Skeleton className='h-4 w-32' />
              <Skeleton className='h-5 w-16 rounded-md' />
            </div>
            <Skeleton className='h-3 w-48' />
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
            <EmptyTitle>{props.emptyTitle ?? t('No subscription plans yet')}</EmptyTitle>
            <EmptyDescription>
              {props.emptyDescription ?? t('Click "Create Plan" to create your first subscription plan')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      </div>
    )
  }

  return (
    <div className='divide-border overflow-hidden rounded-lg border'>
      {rows.map((row) => <PlanMobileCard key={row.id} row={row} />)}
    </div>
  )
}

function PlanMobileCard({ row }: { row: Row<PlanRecord> }) {
  const { t } = useTranslation()
  const cells = row.getAllCells()
  const primary = ['id', 'price', 'duration', 'reset', 'total_amount', 'upgrade_group']
  const renderCell = (id: string) => {
    const cell = cells.find((candidate) => candidate.column.id === id)
    return cell ? flexRender(cell.column.columnDef.cell, cell.getContext()) : null
  }
  return <article aria-label={row.original.plan.title} className='bg-card min-w-0 space-y-4 rounded-xl border p-4'>
    <div className='flex min-w-0 items-start justify-between gap-3'><div className='min-w-0 flex-1'>{renderCell('title')}</div>{renderCell('enabled')}</div>
    <dl className='grid min-w-0 gap-3 sm:grid-cols-2'>{cells.filter((cell) => primary.includes(cell.column.id)).map((cell) => <div key={cell.id} className='min-w-0'><dt className='text-muted-foreground text-xs'>{getCellLabel(cell)}</dt><dd className='mt-1 min-w-0 wrap-anywhere'>{flexRender(cell.column.columnDef.cell, cell.getContext())}</dd></div>)}</dl>
    <details className='min-w-0 rounded-lg border p-3'><summary className='min-h-11 cursor-pointer py-3 font-medium'>{t('More details')}</summary><dl className='mt-3 grid min-w-0 gap-3'>{cells.filter((cell) => ![...primary, 'title', 'enabled', 'actions'].includes(cell.column.id)).map((cell) => <div key={cell.id} className='min-w-0'><dt className='text-muted-foreground text-xs'>{getCellLabel(cell)}</dt><dd className='mt-1 min-w-0 wrap-anywhere'>{flexRender(cell.column.columnDef.cell, cell.getContext())}</dd></div>)}</dl></details>
    <div className='border-t pt-3'>{renderCell('actions')}</div>
  </article>
}

export function SubscriptionsTable() {
  const { t } = useTranslation()
  const columns = useSubscriptionsColumns()
  const isMobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const { refreshTrigger } = useSubscriptions()
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  const [pagination, setPagination] = useState<PaginationState>({ pageIndex: 0, pageSize: 20 })
  const [search, setSearch] = useState('')
  const [filters, setFilters] = useState<ColumnFiltersState>([])
  const ensurePageInRange = useCallback((count: number) => {
    const lastIndex = Math.max(0, count - 1)
    setPagination((current) => current.pageIndex > lastIndex ? { ...current, pageIndex: lastIndex } : current)
  }, [])
  const query = useQuery({
    queryKey: ['admin-subscription-plans', refreshTrigger, identity],
    queryFn: async () => {
      const result = await getAdminPlans()
      if (!result.success || result.data === undefined || (result.data !== null && !Array.isArray(result.data))) throw new Error(result.message || t('Failed to load subscription plans'))
      const records = result.data ?? []
      if (records.some((record) => !record?.plan || !Number.isSafeInteger(record.plan.id) || record.plan.id <= 0 || typeof record.plan.title !== 'string' || typeof record.plan.enabled !== 'boolean')) throw new Error(t('Invalid subscription plan response'))
      return records
    },
  })
  const plans = useMemo(() => query.isError ? [] : query.data ?? [], [query.data, query.isError])
  const { table } = useDataTable({
    data: plans,
    columns,
    getRowId: (record) => String(record.plan.id),
    pagination,
    onPaginationChange: setPagination,
    globalFilter: search,
    onGlobalFilterChange: (updater) => {
      setSearch((current) => {
        const next = typeof updater === 'function' ? updater(current) : updater
        return typeof next === 'string' ? next : ''
      })
      setPagination((current) => ({ ...current, pageIndex: 0 }))
    },
    columnFilters: filters,
    onColumnFiltersChange: (updater) => {
      setFilters((current) => typeof updater === 'function' ? updater(current) : updater)
      setPagination((current) => ({ ...current, pageIndex: 0 }))
    },
    globalFilterFn: (row, _columnId, value) => `${row.original.plan.id} ${row.original.plan.title} ${row.original.plan.subtitle ?? ''}`.toLowerCase().includes(String(value).toLowerCase()),
    autoResetPageIndex: false,
    ensurePageInRange: query.isSuccess && !query.isFetching ? ensurePageInRange : undefined,
    withFacetedRowModel: false,
  })
  const total = query.isSuccess ? table.getFilteredRowModel().rows.length : undefined
  const groups = [...new Set(plans.map((record) => record.plan.upgrade_group ?? ''))].map((value) => ({ value, label: value || t('No Upgrade') }))
  const emptyTitle = search || filters.length > 0 ? t('No matches found') : t('No subscription plans yet')
  const emptyDescription = search || filters.length > 0 ? t('Try adjusting your search or filters.') : t('Click "Create Plan" to create your first subscription plan')
  return <><h2 data-plan-list-heading tabIndex={-1} className='sr-only'>{t('Subscription plan records')}</h2><DataTablePage
    table={table}
    fixedHeight={!isMobile}
    mobileQuery={CONSOLE_MOBILE_QUERY}
    columns={columns}
    isLoading={query.isLoading}
    isFetching={query.isFetching}
    errorState={query.isError ? <ErrorState title={t('Failed to load subscription plans')} description={query.error.message} onRetry={() => { if (!query.isFetching) void query.refetch() }} /> : undefined}
    emptyTitle={emptyTitle}
    emptyDescription={emptyDescription}
    skeletonKeyPrefix='subscriptions-skeleton'
    applyHeaderSize
    toolbarProps={{ searchDebounceMs: 0, searchPlaceholder: t('Search plans by name or ID...'), filters: [{ columnId: 'enabled', title: t('Status'), options: [{ value: 'true', label: t('Enable') }, { value: 'false', label: t('Disable') }] }, { columnId: 'upgrade_group', title: t('Upgrade Group'), options: groups }] }}
    pagination={<LogPagination label={t('Subscription plans pagination')} page={pagination.pageIndex + 1} pageSize={pagination.pageSize} total={total} busy={query.isLoading || query.isFetching} onPageChange={(page) => setPagination((current) => ({ ...current, pageIndex: page - 1 }))} onPageSizeChange={(pageSize) => setPagination({ pageIndex: 0, pageSize })} />}
    mobile={<SubscriptionsMobileList table={table} isLoading={query.isLoading} emptyTitle={emptyTitle} emptyDescription={emptyDescription} />}
  /></>
}
