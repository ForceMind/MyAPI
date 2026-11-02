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
import { useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Switch } from '@/components/ui/switch'

import { resetPlanSubscriptions } from '../../api'
import { useSubscriptions } from '../subscriptions-provider'

export function ResetSubscriptionsDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow, triggerRefresh, returnFocus } = useSubscriptions()
  const [advanceResetTime, setAdvanceResetTime] = useState(true)
  const [resetting, setResetting] = useState(false)
  const [error, setError] = useState('')
  const advanceId = useId()
  const isOpen = open === 'reset-subscriptions'
  const plan = currentRow?.plan
  const planLabel = plan?.title || (plan?.id ? `#${plan.id}` : '-')

  useEffect(() => {
    if (isOpen) {
      setAdvanceResetTime(true)
      setError('')
    }
  }, [isOpen])

  const handleConfirm = async () => {
    if (!plan?.id) return
    setResetting(true)
    setError('')
    try {
      const res = await resetPlanSubscriptions(plan.id, {
        advance_reset_time: advanceResetTime,
      })
      if (res.success) {
        toast.success(
          t('Reset {{count}} active subscriptions', {
            count: res.data?.reset_count || 0,
          })
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
      setResetting(false)
    }
  }

  return (
    <ConfirmDialog
      open={isOpen}
      className='[&_[data-myapi-button]]:min-h-11 [&_[data-slot=select-trigger]]:min-h-11 [&_[role=combobox]]:min-h-11 [&_[data-myapi-button]]:min-w-11'
      returnFocus={returnFocus}
      onOpenChange={(nextOpen) => !nextOpen && !resetting && setOpen(null)}
      title={t('Reset subscription quota')}
      desc={<div className='flex min-w-0 flex-col gap-2 [overflow-wrap:anywhere]'><p>{t('Reset all active subscriptions under {{plan}}?', { plan: planLabel, interpolation: { escapeValue: false } })}</p>{Boolean(plan?.id) && <p className='font-mono text-xs'>#{plan?.id}</p>}</div>}
      confirmText={t('Reset quota')}
      handleConfirm={handleConfirm}
      disabled={!plan?.id}
      isLoading={resetting}
    >
      {error && <p role='alert' className='text-destructive text-sm break-words'>{error}</p>}
      <div className='flex min-h-11 items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm'>
        <label id={`${advanceId}-label`} htmlFor={advanceId} className='flex min-h-11 flex-1 items-center'>{t('Advance next reset time')}</label>
        <Switch
          id={advanceId}
          aria-labelledby={`${advanceId}-label`}
          checked={advanceResetTime}
          onCheckedChange={(checked) => setAdvanceResetTime(!!checked)}
        />
      </div>
    </ConfirmDialog>
  )
}
