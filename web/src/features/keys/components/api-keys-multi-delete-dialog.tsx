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
import type { Table } from '@tanstack/react-table'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'

import { batchDeleteApiKeys } from '../api'
import { ERROR_MESSAGES } from '../constants'
import { useKeyMaintenanceSession } from '../hooks/use-key-maintenance-session'
import type { ApiKey } from '../types'
import { useApiKeys } from './api-keys-provider'

type ApiKeysMultiDeleteDialogProps<TData> = {
  open: boolean
  onOpenChange: (open: boolean) => void
  table: Table<TData>
}

export function ApiKeysMultiDeleteDialog<TData>({
  open,
  onOpenChange,
  table,
}: ApiKeysMultiDeleteDialogProps<TData>) {
  const { t } = useTranslation()
  const isCurrentSession = useKeyMaintenanceSession()
  const { triggerRefresh, onKeysDeleted } = useApiKeys()
  const [isDeleting, setIsDeleting] = useState(false)
  const sending = useRef(false)
  const selectedRows = table.getFilteredSelectedRowModel().rows

  const handleConfirm = async () => {
    if (sending.current || !isCurrentSession()) return
    sending.current = true
    setIsDeleting(true)
    try {
      const ids = selectedRows.map((row) => (row.original as ApiKey).id)
      const result = await batchDeleteApiKeys(ids, {
        skipAuthRefresh: true,
        skipBusinessError: true,
        skipErrorHandler: true,
      })
      if (!isCurrentSession()) return

      if (result.success) {
        const count = result.data || ids.length
        toast.success(t('Successfully deleted {{count}} API key(s)', { count }))
        table.resetRowSelection()
        triggerRefresh()
        onKeysDeleted?.(ids)
        onOpenChange(false)
      } else {
        toast.error(result.message || t(ERROR_MESSAGES.BATCH_DELETE_FAILED))
      }
    } catch {
      if (isCurrentSession()) toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      sending.current = false
      if (isCurrentSession()) setIsDeleting(false)
    }
  }

  return (
    <ConfirmDialog
      destructive
      open={open}
      onOpenChange={(nextOpen) => {
        if (nextOpen || !sending.current) onOpenChange(nextOpen)
      }}
      handleConfirm={handleConfirm}
      isLoading={isDeleting}
      className='max-w-md'
      title={t('Delete {{count}} API key(s)?', { count: selectedRows.length })}
      desc={
        <>
          {t('You are about to delete {{count}} API key(s).', {
            count: selectedRows.length,
          })}{' '}
          <br />
          {t('This action cannot be undone.')}
        </>
      }
      confirmText={t('Delete')}
    />
  )
}
