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

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

import { deleteRedemption } from '../api'
import { SUCCESS_MESSAGES } from '../constants'
import { useRedemptions } from './redemptions-provider'

export function RedemptionsDeleteDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow, triggerRefresh, returnFocus } = useRedemptions()
  const [isDeleting, setIsDeleting] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    if (open === 'delete') setError('')
  }, [open, currentRow?.id])

  const handleDelete = async () => {
    if (!currentRow) return

    setIsDeleting(true)
    setError('')
    try {
      const result = await deleteRedemption(currentRow.id)
      if (result.success) {
        toast.success(t(SUCCESS_MESSAGES.REDEMPTION_DELETED))
        setOpen(null)
        triggerRefresh()
      } else {
        setError(result.message || t('Operation failed'))
      }
    } catch (cause: unknown) {
      setError(cause instanceof Error ? cause.message : t('Operation failed'))
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <AlertDialog
      open={open === 'delete'}
      onOpenChange={(open) => !open && !isDeleting && setOpen(null)}
    >
      <AlertDialogContent finalFocus={returnFocus} className='flex max-h-[calc(100dvh-2rem)] min-w-0 flex-col gap-0 overflow-hidden p-0 [&_[data-myapi-button]]:min-h-11 [&_[data-slot=select-trigger]]:min-h-11 [&_[role=combobox]]:min-h-11 [&_[data-myapi-button]]:min-w-11'>
        <AlertDialogHeader className='shrink-0 border-b px-4 py-4 sm:px-6'>
          <AlertDialogTitle>{t('Are you sure?')}</AlertDialogTitle>
        </AlertDialogHeader>
        <section role='region' tabIndex={0} aria-label={t('Confirmation details')} className='min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain px-4 py-4 [overflow-wrap:anywhere] sm:px-6'>
          <AlertDialogDescription>
            {t('This will permanently delete redemption code')}{' '}
            <span className='mt-2 block font-semibold break-words [overflow-wrap:anywhere]'>{currentRow?.name}</span>
            {currentRow && <span className='mt-1 block font-mono text-xs'>#{currentRow.id}</span>}
            {t('. This action cannot be undone.')}
          </AlertDialogDescription>
          {error && <p role='alert' className='text-destructive mt-3 text-sm'>{error}</p>}
        </section>
        <AlertDialogFooter className='m-0 shrink-0 border-t px-4 py-4 sm:px-6'>
          <AlertDialogCancel disabled={isDeleting}>
            {t('Cancel')}
          </AlertDialogCancel>
          <AlertDialogAction
            onClick={handleDelete}
            disabled={isDeleting}
            variant='destructive'
          >
            {isDeleting ? t('Deleting...') : t('Delete')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
