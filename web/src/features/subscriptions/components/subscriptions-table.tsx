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
import type { Table as TanstackTable } from '@tanstack/react-table'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { formatQuota } from '@/lib/format'

import { getAdminPlans } from '../api'
import { formatDuration, formatResetPeriod } from '../lib'
import type { PlanRecord } from '../types'
import { DataTableRowActions } from './data-table-row-actions'
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
            <EmptyTitle>{t('No subscription plans yet')}</EmptyTitle>
            <EmptyDescription>
              {t('Click "Create Plan" to create your first subscription plan')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      </div>
    )
  }

  return (
    <div className='divide-border overflow-hidden rounded-lg border'>
      {rows.map((row) => {
        const plan = row.original.plan
        const total = Number(plan.total_amount || 0)

        return (
          <div
            key={row.id}
            className='bg-card flex min-w-0 flex-col gap-1.5 border-b px-3 py-2.5 last:border-b-0'
          >
            <div className='flex min-w-0 items-start justify-between gap-2'>
              <div className='min-w-0'>
                <div
                  className='truncate text-sm font-semibold'
                  title={plan.title}
                >
                  {plan.title}
                </div>
                {plan.subtitle && (
                  <div
                    className='text-caption text-muted-foreground truncate'
                    title={plan.subtitle}
                  >
                    {plan.subtitle}
                  </div>
                )}
              </div>
              <StatusBadge
                label={plan.enabled ? t('Enable') : t('Disable')}
                variant={plan.enabled ? 'success' : 'neutral'}
                copyable={false}
                className='shrink-0'
              />
            </div>
            <div className='text-caption text-muted-foreground flex min-w-0 flex-wrap gap-x-2'>
              <span className='font-mono'>#{plan.id}</span>
              <span className='text-foreground font-semibold tabular-nums'>
                ${Number(plan.price_amount || 0).toFixed(2)}
              </span>
              <span>{formatDuration(plan, t)}</span>
              <span>{formatResetPeriod(plan, t)}</span>
              <span>{total > 0 ? formatQuota(total) : t('Unlimited')}</span>
            </div>
            <div className='flex justify-end'>
              <DataTableRowActions row={row} />
            </div>
          </div>
        )
      })}
    </div>
  )
}

export function SubscriptionsTable() {
  const { t } = useTranslation()
  const columns = useSubscriptionsColumns()
  const { refreshTrigger } = useSubscriptions()

  const { data, isLoading } = useQuery({
    queryKey: ['admin-subscription-plans', refreshTrigger],
    queryFn: async () => {
      const result = await getAdminPlans()
      return result.data || []
    },
    placeholderData: (prev) => prev,
  })

  const plans = useMemo(() => data || [], [data])

  const { table } = useDataTable({
    data: plans,
    columns,
    withFilteredRowModel: false,
    withFacetedRowModel: false,
  })

  return (
    <DataTablePage
      table={table}
      mobileQuery={CONSOLE_MOBILE_QUERY}
      columns={columns}
      isLoading={isLoading}
      emptyTitle={t('No subscription plans yet')}
      emptyDescription={t(
        'Click "Create Plan" to create your first subscription plan'
      )}
      skeletonKeyPrefix='subscriptions-skeleton'
      applyHeaderSize
      mobile={<SubscriptionsMobileList table={table} isLoading={isLoading} />}
    />
  )
}
