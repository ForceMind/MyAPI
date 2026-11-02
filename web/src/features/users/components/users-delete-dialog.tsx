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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'

import { deleteUser } from '../api'
import { ERROR_MESSAGES } from '../constants'
import { getUserActionMessage } from '../lib'
import { useUsers } from './users-provider'
import { useUserTask } from '../hooks/use-user-task'

export function UsersDeleteDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow, triggerRefresh, returnFocus } = useUsers()
  const [isDeleting, setIsDeleting] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)
  const task = useUserTask(open === 'delete', currentRow?.id)
  useEffect(() => {
    if (open === 'delete') { setIsDeleting(false); setActionError(null) }
  }, [open, currentRow?.id, task])

  const handleDelete = async () => {
    if (!currentRow) return
    const operation = task.begin()
    if (!operation) return

    setIsDeleting(true)
    setActionError(null)
    try {
      const result = await deleteUser(currentRow.id)
      if (!operation.isCurrent()) return
      if (result.success) {
        toast.success(t(getUserActionMessage('delete')))
        setOpen(null)
        triggerRefresh()
      } else {
        setActionError(result.message || t(ERROR_MESSAGES.DELETE_FAILED))
        toast.error(result.message || t(ERROR_MESSAGES.DELETE_FAILED))
      }
    } catch {
      if (!operation.isCurrent()) return
      setActionError(t(ERROR_MESSAGES.UNEXPECTED))
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      if (operation.isCurrent()) setIsDeleting(false)
      operation.finish()
    }
  }

  return (
    <ConfirmDialog
      open={open === 'delete'}
      onOpenChange={(next) => { if (!next && !task.isPending()) setOpen(null) }}
      returnFocus={returnFocus}
      title={t('Are you sure?')}
      desc={
        <>
          {t('This will permanently delete user')}{' '}
          <span className='font-semibold'>{currentRow?.username}</span>
          {t('. This action cannot be undone.')}
        </>
      }
      confirmText={isDeleting ? t('Deleting...') : t('Delete')}
      destructive
      isLoading={isDeleting}
      handleConfirm={handleDelete}
    >
      <p className='text-muted-foreground text-sm'>{t('User ID')}: {currentRow?.id}</p>
      {actionError ? <div role='alert' className='text-destructive text-sm break-words'>{actionError}</div> : null}
    </ConfirmDialog>
  )
}
