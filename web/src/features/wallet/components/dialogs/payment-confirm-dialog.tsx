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
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { useTranslation } from 'react-i18next'

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
import { Skeleton } from '@/components/ui/skeleton'
import { formatLocalCurrencyAmount } from '@/lib/currency'

import { DEFAULT_DISCOUNT_RATE } from '../../constants'
import { formatCurrency, getPaymentIcon } from '../../lib'
import type { PaymentMethod } from '../../types'

interface PaymentConfirmDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
  onOpenHistory?: () => void
  topupAmount: number
  paymentAmount: number
  quoteKnown?: boolean
  paymentMethod: PaymentMethod | undefined
  calculating: boolean
  processing: boolean
  outcomeUnknown?: boolean
  discountRate?: number
  usdExchangeRate?: number
}

export function PaymentConfirmDialog({
  open,
  onOpenChange,
  onConfirm,
  onOpenHistory,
  topupAmount,
  paymentAmount,
  quoteKnown = true,
  paymentMethod,
  calculating,
  processing,
  outcomeUnknown = false,
  discountRate = DEFAULT_DISCOUNT_RATE,
  usdExchangeRate = 1,
}: PaymentConfirmDialogProps) {
  const { t } = useTranslation()
  const hasDiscount = quoteKnown && discountRate > 0 && discountRate < 1 && paymentAmount > 0
  const originalAmount = hasDiscount ? paymentAmount / discountRate : 0
  const discountAmount = hasDiscount ? originalAmount - paymentAmount : 0

  return (
    <AlertDialog open={open} onOpenChange={(nextOpen) => (nextOpen || !processing) && onOpenChange(nextOpen)}>
      <AlertDialogContent className='flex max-h-[calc(100dvh-2rem)] min-w-0 flex-col gap-0 overflow-hidden p-0 max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md [&_[data-myapi-button]]:min-h-11 [&_[data-slot=select-trigger]]:min-h-11 [&_[role=combobox]]:min-h-11 [&_[data-myapi-button]]:min-w-11'>
        <AlertDialogHeader className='shrink-0 border-b px-4 py-4 sm:px-6'>
          <AlertDialogTitle className='text-xl font-semibold'>
            {t('Confirm Payment')}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t('Review your payment details')}
          </AlertDialogDescription>
        </AlertDialogHeader>

        <section role='region' tabIndex={0} aria-label={t('Payment details')} className='min-h-0 min-w-0 flex-1 space-y-3 overflow-y-auto overscroll-contain px-4 py-4 [overflow-wrap:anywhere] sm:space-y-4 sm:px-6'>
          <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
            <span className='text-muted-foreground text-sm'>
              {t('Topup Amount')}
            </span>
            <span className='text-lg font-semibold'>
              {formatLocalCurrencyAmount(topupAmount * usdExchangeRate, {
                digitsLarge: 2,
                digitsSmall: 2,
                abbreviate: false,
              })}
            </span>
          </div>

          <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
            <span className='text-muted-foreground text-sm'>
              {t('You Pay')}
            </span>
            {calculating ? (
              <Skeleton className='h-6 w-24' />
            ) : (
              <div className='flex items-baseline gap-2'>
                <span className='text-2xl font-semibold'>
                  {quoteKnown ? formatCurrency(paymentAmount) : t('Unknown')}
                </span>
                {hasDiscount && (
                  <span className='text-muted-foreground text-sm line-through'>
                    {formatCurrency(originalAmount)}
                  </span>
                )}
              </div>
            )}
          </div>

          {hasDiscount && !calculating && (
            <div className='bg-muted/50 rounded-lg p-3'>
              <div className='flex items-center justify-between text-sm'>
                <span className='text-muted-foreground'>{t('You save')}</span>
                <span className='text-success font-semibold'>
                  {formatCurrency(discountAmount)}
                </span>
              </div>
            </div>
          )}

          <div className='border-t pt-4'>
            <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
              <span className='text-muted-foreground text-sm'>
                {t('Payment Method')}
              </span>
              <div className='flex min-w-0 flex-wrap items-center justify-end gap-2'>
                {getPaymentIcon(
                  paymentMethod?.type,
                  'h-4 w-4',
                  paymentMethod?.icon,
                  paymentMethod?.name
                )}
                <span className='font-medium'>{paymentMethod?.name}</span>
              </div>
            </div>
          </div>
          {outcomeUnknown && <p role='alert' className='text-warning text-sm'>{t('Payment outcome is unconfirmed. New payments are locked; review billing history.')}</p>}
          {outcomeUnknown && onOpenHistory && <Button variant='outline' className='min-h-11' onClick={onOpenHistory}>{t('Billing History')}</Button>}
          <p className='text-muted-foreground text-xs'>{t('Closing a window does not cancel a submitted payment. Check billing history if the outcome is unknown.')}</p>
        </section>

        <AlertDialogFooter className='m-0 grid shrink-0 grid-cols-2 gap-2 border-t px-4 py-4 sm:flex sm:px-6'>
          <AlertDialogCancel disabled={processing}>
            {t('Cancel')}
          </AlertDialogCancel>
          <AlertDialogAction onClick={onConfirm} disabled={processing || outcomeUnknown || calculating || !quoteKnown}>
            {processing && <Spinner aria-hidden='true' data-icon='inline-start' />}
            {t('Confirm Payment')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
