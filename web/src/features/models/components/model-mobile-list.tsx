import { flexRender, type Row, type Table } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { getCellLabel } from '@/components/data-table/layout/card-cell-utils'
import { Skeleton } from '@/components/ui/skeleton'

import type { Model } from '../types'

/** Use all original cells, including columns hidden in the desktop view. */
function ModelMobileCard({ row }: { row: Row<Model> }) {
  const { t } = useTranslation()
  const cells = row.getAllCells()
  const renderCell = (id: string) => {
    const cell = cells.find((candidate) => candidate.column.id === id)
    return cell ? flexRender(cell.column.columnDef.cell, cell.getContext()) : null
  }
  return (
    <article className='bg-card min-w-0 space-y-4 rounded-xl border p-4'>
      <div className='flex min-w-0 items-start gap-3'>
        <span className='inline-flex min-h-11 min-w-11 items-center justify-center [&_[data-slot=checkbox]]:after:-inset-3.5'>{renderCell('select')}</span>
        <div className='min-w-0 flex-1'><h3>{renderCell('model_name')}</h3>{renderCell('vendor_id')}</div>
        <div>{renderCell('status')}</div>
      </div>
      <div>{renderCell('name_rule')}</div>
      <details className='min-w-0 rounded-lg border p-3'>
        <summary className='min-h-11 cursor-pointer py-3 font-medium'>{t('More details')}</summary>
        <dl className='mt-3 grid min-w-0 gap-4'>
          {cells.filter((cell) => !['select', 'model_name', 'vendor_id', 'status', 'name_rule', 'actions'].includes(cell.column.id)).map((cell) => (
            <div key={cell.id} className='min-w-0'><dt className='text-muted-foreground mb-1 text-xs'>{getCellLabel(cell)}</dt><dd className='min-w-0 wrap-anywhere'>{flexRender(cell.column.columnDef.cell, cell.getContext())}</dd></div>
          ))}
        </dl>
      </details>
      <div className='border-t pt-3'>{renderCell('actions')}</div>
    </article>
  )
}

export function ModelMobileList({ table, loading }: { table: Table<Model>; loading: boolean }) {
  const { t } = useTranslation()
  if (loading) return <div role='status' aria-label={t('Loading...')}><Skeleton className='h-64 w-full' /></div>
  const rows = table.getRowModel().rows
  if (rows.length === 0) return <div className='rounded-xl border p-6 text-center'><h3>{t('No Models Found')}</h3><p className='text-muted-foreground mt-2 text-sm'>{t('No models available. Create your first model to get started.')}</p></div>
  return <div className='grid min-w-0 gap-4'>{rows.map((row) => <ModelMobileCard key={row.id} row={row} />)}</div>
}
