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
import { flexRender, type Row, type Table as TanstackTable } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { DISABLED_ROW_MOBILE } from '@/components/data-table'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

import { REDEMPTION_STATUS } from '../constants'
import { isRedemptionExpired } from '../lib'
import type { Redemption } from '../types'
import { getCellLabel } from '@/components/data-table/layout/card-cell-utils'

const MOBILE_SKELETON_KEYS = [
  'redemption-mobile-skeleton-1',
  'redemption-mobile-skeleton-2',
  'redemption-mobile-skeleton-3',
  'redemption-mobile-skeleton-4',
  'redemption-mobile-skeleton-5',
]

function RedemptionsMobileSkeleton() {
  return (
    <div className='divide-border overflow-hidden rounded-lg border'>
      {MOBILE_SKELETON_KEYS.map((key) => (
        <div
          key={key}
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

interface RedemptionsMobileListProps {
  table: TanstackTable<Redemption>
  isLoading: boolean
  emptyTitle?: string
  emptyDescription?: string
}

export function RedemptionsMobileList(props: RedemptionsMobileListProps) {
  const { t } = useTranslation()
  const rows = props.table.getRowModel().rows

  if (props.isLoading) return <RedemptionsMobileSkeleton />

  if (!rows.length) {
    return (
      <div className='rounded-lg border p-8'>
        <Empty className='border-none p-0'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <HugeiconsIcon icon={Database01Icon} className='size-6' />
            </EmptyMedia>
            <EmptyTitle>{props.emptyTitle ?? t('No Redemption Codes Found')}</EmptyTitle>
            <EmptyDescription>
              {props.emptyDescription ?? t('No redemption codes available. Create your first redemption code to get started.')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      </div>
    )
  }

  return <div className='grid min-w-0 gap-4'>{rows.map((row) => <RedemptionMobileCard key={row.id} row={row} />)}</div>
}

function RedemptionMobileCard({ row }: { row: Row<Redemption> }) {
  const { t } = useTranslation()
  const redemption = row.original
  const cells = row.getAllCells()
  const renderCell = (id: string) => {
    const cell = cells.find((candidate) => candidate.column.id === id)
    return cell ? flexRender(cell.column.columnDef.cell, cell.getContext()) : null
  }
  const expired = isRedemptionExpired(redemption.expired_time, redemption.status)
  return <article aria-label={redemption.name} className={cn('bg-card min-w-0 space-y-4 rounded-xl border p-4', (expired || redemption.status !== REDEMPTION_STATUS.ENABLED) && DISABLED_ROW_MOBILE)}>
    <div className='flex min-w-0 items-start gap-3'><span className='inline-flex min-h-11 min-w-11 items-center justify-center [&_[data-slot=checkbox]]:after:-inset-3.5'>{renderCell('select')}</span><div className='min-w-0 flex-1 wrap-anywhere'>{renderCell('name')}</div>{renderCell('status')}</div>
    <div className='min-w-0 [&_[data-myapi-button]]:min-h-11 [&_[data-slot=select-trigger]]:min-h-11 [&_[role=combobox]]:min-h-11 [&_[data-myapi-button]]:min-w-11'>{renderCell('code')}</div>
    <dl className='grid min-w-0 gap-3 sm:grid-cols-2'>{cells.filter((cell) => ['id', 'quota', 'expired_time'].includes(cell.column.id)).map((cell) => <div key={cell.id} className='min-w-0'><dt className='text-muted-foreground text-xs'>{getCellLabel(cell)}</dt><dd className='mt-1 wrap-anywhere'>{flexRender(cell.column.columnDef.cell, cell.getContext())}</dd></div>)}</dl>
    <details className='min-w-0 rounded-lg border p-3'><summary className='min-h-11 cursor-pointer py-3 font-medium'>{t('More details')}</summary><dl className='mt-3 grid min-w-0 gap-3'>{cells.filter((cell) => !['select', 'name', 'status', 'code', 'id', 'quota', 'expired_time', 'actions'].includes(cell.column.id)).map((cell) => <div key={cell.id} className='min-w-0'><dt className='text-muted-foreground text-xs'>{getCellLabel(cell)}</dt><dd className='mt-1 wrap-anywhere'>{flexRender(cell.column.columnDef.cell, cell.getContext())}</dd></div>)}</dl></details>
    <div className='border-t pt-3'>{renderCell('actions')}</div>
  </article>
}
