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

import { patchPlanStatus } from '../../api'
import { useSubscriptions } from '../subscriptions-provider'

export function ToggleStatusDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow, triggerRefresh, returnFocus } = useSubscriptions()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    if (open === 'toggle-status') setError('')
  }, [open, currentRow?.plan.id])

  if (open !== 'toggle-status' || !currentRow) return null

  const isEnabled = currentRow.plan.enabled
  const title = isEnabled ? t('Confirm disable') : t('Confirm enable')
  const description = isEnabled
    ? t(
        'After disabling, it will no longer be shown to users, but historical orders are not affected. Continue?'
      )
    : t('After enabling, the plan will be shown to users. Continue?')

  const handleConfirm = async () => {
    setLoading(true)
    setError('')
    try {
      const res = await patchPlanStatus(currentRow.plan.id, !isEnabled)
      if (res.success) {
        toast.success(
          isEnabled ? t('Has been disabled') : t('Has been enabled')
        )
        triggerRefresh()
        setOpen(null)
      } else {
        setError(res.message || t('Operation failed'))
      }
    } catch (cause: unknown) {
      setError(cause instanceof Error ? cause.message : t('Operation failed'))
      toast.error(t('Operation failed'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <ConfirmDialog
      open
      className='[&_[data-myapi-button]]:min-h-11 [&_[data-slot=select-trigger]]:min-h-11 [&_[role=combobox]]:min-h-11 [&_[data-myapi-button]]:min-w-11'
      returnFocus={returnFocus}
      onOpenChange={(v) => !v && !loading && setOpen(null)}
      title={title}
      desc={<div className='flex min-w-0 flex-col gap-2 [overflow-wrap:anywhere]'><p className='font-medium'>{currentRow.plan.title}</p><p className='font-mono text-xs'>#{currentRow.plan.id}</p><p>{description}</p></div>}
      handleConfirm={handleConfirm}
      isLoading={loading}
      confirmText={isEnabled ? t('Disable') : t('Enable')}
      destructive={isEnabled}
    >
      {error && <p role='alert' className='text-destructive text-sm break-words'>{error}</p>}
    </ConfirmDialog>
  )
}
