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
import { useQueryClient } from '@tanstack/react-query'
import type { Table } from '@tanstack/react-table'
import { Cancel01Icon, Copy01Icon, Delete02Icon, Tick02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { copyToClipboard } from '@/lib/copy-to-clipboard'

import {
  handleBatchEnableModels,
  handleBatchDisableModels,
  handleBatchDeleteModels,
} from '../lib'
import type { Model } from '../types'

interface DataTableBulkActionsProps<TData> {
  table: Table<TData>
}

export function DataTableBulkActions<TData>({
  table,
}: DataTableBulkActionsProps<TData>) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)

  const selectedRows = table.getFilteredSelectedRowModel().rows
  const selectedIds = selectedRows.reduce<number[]>((ids, row) => {
    const id = (row.original as Model).id

    if (typeof id === 'number') {
      ids.push(id)
    }

    return ids
  }, [])

  const selectedModels = selectedRows.map((row) => row.original as Model)

  const handleClearSelection = () => {
    table.resetRowSelection()
  }

  const handleEnableAll = () => {
    handleBatchEnableModels(selectedIds, queryClient, handleClearSelection)
  }

  const handleDisableAll = () => {
    handleBatchDisableModels(selectedIds, queryClient, handleClearSelection)
  }

  const handleDeleteAll = () => {
    handleBatchDeleteModels(selectedIds, queryClient, () => {
      setShowDeleteConfirm(false)
      handleClearSelection()
    })
  }

  const handleCopyNames = async () => {
    const names = selectedModels.map((m) => m.model_name).join(',')
    const success = await copyToClipboard(names)
    if (success) {
      toast.success(t('Model names copied to clipboard'))
    } else {
      toast.error(t('Failed to copy model names'))
    }
  }

  return (
    <>
      <div className='[&_[role=toolbar]]:right-3 [&_[role=toolbar]]:left-3 [&_[role=toolbar]]:translate-x-0 [&_[role=toolbar]>div]:flex-wrap [&_[role=toolbar]>div]:gap-2 [&_[role=toolbar]]:hover:scale-100 [&_[role=toolbar]_[data-slot=button]]:min-h-11 [&_[role=toolbar]_[data-slot=button]]:min-w-11'>
      <BulkActionsToolbar table={table} entityName='model'>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                onClick={handleEnableAll}
                className='min-h-11 min-w-11'
                aria-label={t('Enable selected models')}
                title={t('Enable selected models')}
              />
            }
          >
            <HugeiconsIcon icon={Tick02Icon} aria-hidden='true' />
            <span className='sr-only'>{t('Enable selected models')}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Enable selected models')}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                onClick={handleDisableAll}
                className='min-h-11 min-w-11'
                aria-label={t('Disable selected models')}
                title={t('Disable selected models')}
              />
            }
          >
            <HugeiconsIcon icon={Cancel01Icon} aria-hidden='true' />
            <span className='sr-only'>{t('Disable selected models')}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Disable selected models')}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                onClick={handleCopyNames}
                className='min-h-11 min-w-11'
                aria-label={t('Copy model names')}
                title={t('Copy model names')}
              />
            }
          >
            <HugeiconsIcon icon={Copy01Icon} aria-hidden='true' />
            <span className='sr-only'>{t('Copy model names')}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Copy model names')}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='destructive'
                size='icon'
                onClick={() => setShowDeleteConfirm(true)}
                className='min-h-11 min-w-11'
                aria-label={t('Delete selected models')}
                title={t('Delete selected models')}
              />
            }
          >
            <HugeiconsIcon icon={Delete02Icon} aria-hidden='true' />
            <span className='sr-only'>{t('Delete selected models')}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Delete selected models')}</p>
          </TooltipContent>
        </Tooltip>
      </BulkActionsToolbar>
      </div>

      {/* Delete Confirmation Dialog */}
      <Dialog
        open={showDeleteConfirm}
        onOpenChange={setShowDeleteConfirm}
        title={t('Delete Models?')}
        description={t(
          'Are you sure you want to delete {{count}} model(s)? This action cannot be undone.',
          { count: selectedIds.length }
        )}
        contentHeight='auto'
        footer={
          <>
            <Button
              variant='outline'
              onClick={() => setShowDeleteConfirm(false)}
            >
              {t('Cancel')}
            </Button>
            <Button variant='destructive' onClick={handleDeleteAll}>
              {t('Delete')}
            </Button>
          </>
        }
      >
        <div className='max-h-48 space-y-2 overflow-y-auto text-sm'>{selectedModels.map((model) => <p key={model.id} className='wrap-anywhere'><span className='font-mono'>#{model.id}</span> {model.model_name}</p>)}</div>
      </Dialog>
    </>
  )
}
