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
import { Delete02Icon, PlusSignIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { useAuthStore } from '@/stores/auth-store'

import { deleteInvalidRedemptions } from '../api'
import { ERROR_MESSAGES } from '../constants'
import { useRedemptions } from './redemptions-provider'

export function RedemptionsPrimaryButtons() {
  const { t } = useTranslation()
  const { setOpen, triggerRefresh } = useRedemptions()
  const [showDeleteInvalidConfirm, setShowDeleteInvalidConfirm] =
    useState(false)
  const [isDeleting, setIsDeleting] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)
  const pending = useRef(false)
  const deleteTrigger = useRef<HTMLButtonElement>(null)
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  const isCurrentIdentity = () => {
    const { auth } = useAuthStore.getState()
    return identity === `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`
  }

  const handleDeleteInvalid = async () => {
    if (pending.current || !isCurrentIdentity()) return
    pending.current = true
    setActionError(null)
    setIsDeleting(true)
    try {
      const result = await deleteInvalidRedemptions()
      if (!isCurrentIdentity()) return
      if (result.success) {
        const count = result.data
        if (typeof count === 'number' && Number.isSafeInteger(count) && count >= 0) toast.success(t('Successfully deleted {{count}} invalid redemption codes', { count }))
        else toast.success(t('Deleted successfully'))
        triggerRefresh()
        setShowDeleteInvalidConfirm(false)
      } else {
        const message = result.message || t(ERROR_MESSAGES.DELETE_INVALID_FAILED)
        setActionError(message)
        toast.error(message)
      }
    } catch (error: unknown) {
      if (!isCurrentIdentity()) return
      const message = error instanceof Error ? error.message : t(ERROR_MESSAGES.DELETE_INVALID_FAILED)
      setActionError(message)
      toast.error(message)
    } finally {
      pending.current = false
      if (isCurrentIdentity()) setIsDeleting(false)
    }
  }

  return (
    <>
      <div className='flex flex-wrap gap-2'>
        <Button
          size='sm'
          variant='outline'
          className='min-h-11'
          ref={deleteTrigger}
          onClick={() => { setActionError(null); setShowDeleteInvalidConfirm(true) }}
        >
          <HugeiconsIcon icon={Delete02Icon} aria-hidden='true' className='text-destructive h-4 w-4' />
          {t('Delete Invalid')}
        </Button>
        <Button size='sm' className='min-h-11' data-redemption-create onClick={() => setOpen('create')}>
          <HugeiconsIcon icon={PlusSignIcon} aria-hidden='true' className='h-4 w-4' />
          {t('Create Code')}
        </Button>
      </div>

      <ConfirmDialog
        destructive
        open={showDeleteInvalidConfirm}
        onOpenChange={(open) => { if (!pending.current) setShowDeleteInvalidConfirm(open) }}
        returnFocus={() => deleteTrigger.current}
        handleConfirm={handleDeleteInvalid}
        isLoading={isDeleting}
        className='max-w-md'
        title={t('Delete Invalid Redemption Codes?')}
        desc={
          <>
            {t('This will delete all')} <strong>{t('used')}</strong>,{' '}
            <strong>{t('disabled')}</strong>
            {t(', and')} <strong>{t('expired')}</strong>{' '}
            {t('redemption codes.')}
            <br />
            {t('This action cannot be undone.')}
          </>
        }
        confirmText={t('Delete Invalid')}
      >
        {actionError && <p role='alert' className='text-destructive text-sm break-words'>{actionError}</p>}
      </ConfirmDialog>
    </>
  )
}
