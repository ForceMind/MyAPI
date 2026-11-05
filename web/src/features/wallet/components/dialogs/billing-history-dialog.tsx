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
import { ArrowLeft01Icon, ArrowRight01Icon, Copy01Icon, Search01Icon, Tick02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState, useRef, useLayoutEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { useWalletSession } from '../../hooks/use-wallet-session'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { StatusBadge } from '@/components/status-badge'
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
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatNumber } from '@/lib/format'

import { useBillingHistory } from '../../hooks/use-billing-history'
import {
  getStatusConfig,
  getPaymentMethodName,
  formatTimestamp,
} from '../../lib/billing'

interface BillingHistoryDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const BILLING_HISTORY_SKELETON_IDS = Array.from(
  { length: 5 },
  (_, index) => `billing-history-skeleton-${index + 1}`
)

export function BillingHistoryDialog(props: BillingHistoryDialogProps) {
  const isCurrent = useWalletSession()
  const identity = useAuthStore((state) => [state.auth.user?.id, state.auth.user?.role, state.auth.session?.sid ?? null].join(':'))
  if (!isCurrent()) return null
  return <BillingHistoryTask key={identity} {...props} />
}

function BillingHistoryTask({
  open,
  onOpenChange,
}: BillingHistoryDialogProps) {
  const { t } = useTranslation()
  const isCurrent = useWalletSession()
  const opening = useRef<object>({})
  const {
    records,
    total,
    page,
    pageSize,
    keyword,
    loading,
    error,
    refresh,
    completing,
    completionUnknown,
    completionFailure,
    isAdmin,
    handlePageChange,
    handlePageSizeChange,
    handleSearch,
    handleCompleteOrder,
  } = useBillingHistory({ enabled: open })

  const [confirmTradeNo, setConfirmTradeNo] = useState<string | null>(null)
  const [completionError, setCompletionError] = useState('')
  useLayoutEffect(() => {
    opening.current = {}
    if (!open) { setConfirmTradeNo(null); setCompletionError('') }
  }, [open])
  const { copyToClipboard, copiedText } = useCopyToClipboard({ notify: false })

  const totalPages = total === undefined ? undefined : Math.ceil(total / pageSize)

  const handleConfirmComplete = async () => {
    if (confirmTradeNo && isCurrent()) {
      const operation = opening.current
      setCompletionError('')
      const success = await handleCompleteOrder(confirmTradeNo)
      if (!isCurrent() || operation !== opening.current) return
      if (success) {
        setConfirmTradeNo(null)
      } else {
        setCompletionError(t('Failed to complete order'))
      }
    }
  }

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className='flex h-dvh max-h-dvh w-full max-w-none flex-col gap-0 overflow-hidden rounded-none p-0 sm:h-[min(85dvh,800px)] sm:max-h-[calc(100dvh-3rem)] sm:max-w-4xl sm:rounded-xl [&_[data-slot=button]]:min-h-11 [&_[data-slot=button]]:min-w-11 [&_[data-slot=input]]:min-h-11 [&_[data-slot=select-trigger]]:min-h-11 [&_[data-slot=dialog-close]]:min-h-11 [&_[data-slot=dialog-close]]:min-w-11'>
          {completionUnknown && <p role='alert' className='text-warning px-4 pt-4 text-sm'>{t('Payment request outcome is unknown. Check billing history before trying again.')}</p>}
          <DialogHeader className='shrink-0 border-b px-4 py-4 pr-16 sm:px-6 sm:pr-16'><DialogTitle>{t('Billing History')}</DialogTitle><DialogDescription>{t('View your topup transaction records and payment history')}</DialogDescription></DialogHeader>
          <section role='region' tabIndex={0} aria-label={t('Billing history results')} className='min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain px-4 py-4 [overflow-wrap:anywhere] sm:px-6'>
        <div className='min-h-0 space-y-3'>
          {error && <Alert variant='destructive'><AlertTitle>{t('Unable to load billing history')}</AlertTitle><AlertDescription>{error}</AlertDescription><Button className='mt-3 min-h-11 w-fit' variant='outline' disabled={loading} onClick={() => { void refresh() }}>{t('Retry')}</Button></Alert>}
          {/* Search and Filter Bar */}
          <div className='flex items-center gap-2'>
            <div className='relative flex-1'>
              <HugeiconsIcon icon={Search01Icon} className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2' />
              <Input
                aria-label={t('Search billing history')}
                placeholder={t('Search by order number...')}
                value={keyword}
                onChange={(e) => handleSearch(e.target.value)}
                className='h-9 pl-10'
              />
            </div>
            <Select
              items={[
                { value: '10', label: t('10 / page') },
                { value: '20', label: t('20 / page') },
                { value: '50', label: t('50 / page') },
                { value: '100', label: t('100 / page') },
              ]}
              value={pageSize.toString()}
              onValueChange={(value) =>
                value !== null && handlePageSizeChange(Number.parseInt(value))
              }
            >
              <SelectTrigger aria-label={t('Rows per page')} className='min-h-11 w-[92px] sm:w-32'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false} className='[&_[data-slot=select-item]]:min-h-11'>
                <SelectGroup>
                  <SelectItem value='10'>{t('10 / page')}</SelectItem>
                  <SelectItem value='20'>{t('20 / page')}</SelectItem>
                  <SelectItem value='50'>{t('50 / page')}</SelectItem>
                  <SelectItem value='100'>{t('100 / page')}</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>

          {/* Records List */}
          <div className='min-w-0'>
            {loading && (
              <div className='space-y-3'>
                {BILLING_HISTORY_SKELETON_IDS.map((id) => (
                  <div key={id} className='rounded-lg border p-3 sm:p-4'>
                    <div className='flex items-start justify-between'>
                      <div className='flex-1 space-y-2'>
                        <Skeleton className='h-4 w-48' />
                        <Skeleton className='h-3 w-32' />
                      </div>
                      <Skeleton className='h-5 w-16' />
                    </div>
                    <div className='mt-3 grid grid-cols-2 gap-3 sm:grid-cols-3 sm:gap-4'>
                      <Skeleton className='h-3 w-full' />
                      <Skeleton className='h-3 w-full' />
                      <Skeleton className='h-3 w-full' />
                    </div>
                  </div>
                ))}
              </div>
            )}
            {!loading && !error && total !== undefined && records.length === 0 && (
              <div className='text-muted-foreground flex min-h-40 flex-col items-center justify-center py-10 text-center'>
                <p className='text-sm font-medium'>
                  {t('No billing records found')}
                </p>
                <p className='mt-1 text-xs'>
                  {keyword
                    ? t('Try adjusting your search')
                    : t('Your transaction history will appear here')}
                </p>
              </div>
            )}
            {!loading && !error && records.length > 0 && (
              <ul className='divide-border divide-y overflow-hidden rounded-lg border'>
                {records.map((record) => {
                  const statusConfig = getStatusConfig(record.status)
                  return (
                    <li
                      key={record.id}
                      className='bg-card flex min-w-0 flex-col gap-2 px-3 py-2.5 sm:px-4 sm:py-3'
                    >
                      {/* Order number, time and status */}
                      <div className='flex items-start justify-between gap-2'>
                        <div className='flex min-w-0 flex-1 flex-col gap-0.5'>
                          <div className='flex min-w-0 items-center gap-2'>
                            <code className='text-foreground min-w-0 font-mono text-sm break-words [overflow-wrap:anywhere]'>
                              {record.trade_no}
                            </code>
                            <Button
                              type='button'
                              variant='ghost'
                              size='sm'
                              className='min-h-11 min-w-11 shrink-0 p-0'
                              aria-label={t('Copy')}
                              onClick={() => copyToClipboard(record.trade_no)}
                            >
                              {copiedText === record.trade_no ? (
                                <HugeiconsIcon icon={Tick02Icon} className='h-3 w-3' />
                              ) : (
                                <HugeiconsIcon icon={Copy01Icon} className='h-3 w-3' />
                              )}
                            </Button>
                            {isAdmin && record.user_id != null && (
                              <StatusBadge
                                label={`${t('User ID')}: ${record.user_id}`}
                                variant='neutral'
                                size='sm'
                                copyText={String(record.user_id)}
                              />
                            )}
                          </div>
                          <div className='text-muted-foreground text-xs'>
                            {formatTimestamp(record.create_time)}
                          </div>
                        </div>
                        <StatusBadge
                          label={statusConfig.label}
                          variant={statusConfig.variant}
                          showDot
                          copyable={false}
                        />
                      </div>

                      {/* Payment method, credited amount and paid money */}
                      <dl className='grid grid-cols-2 gap-x-3 gap-y-1 text-xs sm:grid-cols-3'>
                        <div className='flex min-w-0 flex-col'>
                          <dt className='text-muted-foreground'>
                            {t('Payment Method')}
                          </dt>
                          <dd className='text-sm font-medium wrap-anywhere'>
                            {getPaymentMethodName(record.payment_method, t)}
                          </dd>
                        </div>
                        <div className='flex min-w-0 flex-col'>
                          <dt className='text-muted-foreground'>
                            {t('Amount')}
                          </dt>
                          <dd className='text-sm font-semibold tabular-nums'>
                            {formatCurrencyFromUSD(record.amount, {
                              digitsLarge: 2,
                              digitsSmall: 2,
                              abbreviate: false,
                            })}
                          </dd>
                        </div>
                        <div className='flex min-w-0 flex-col'>
                          <dt className='text-muted-foreground'>
                            {t('Payment')}
                          </dt>
                          <dd className='text-sm font-semibold tabular-nums'>
                            {formatNumber(record.money)}
                          </dd>
                        </div>
                      </dl>

                      {/* Admin Actions */}
                      {isAdmin && record.status === 'pending' && (
                        <div className='flex justify-end'>
                          <Button
                            type='button'
                            size='sm'
                            variant='outline'
                            onClick={() => { setCompletionError(''); setConfirmTradeNo(record.trade_no) }}
                            disabled={completing}
                          >
                            {t('Complete Order')}
                          </Button>
                        </div>
                      )}
                    </li>
                  )
                })}
              </ul>
            )}
          </div>

          {!loading && !error && total === undefined && records.length === 0 && <p role='status' className='text-muted-foreground py-6 text-sm'>{t('Record total unavailable')}</p>}
        </div>
          </section>
          <DialogFooter role='group' aria-label={t('Actions')} className='m-0 shrink-0 flex-col gap-3 rounded-none px-4 py-4 sm:flex-col sm:px-6'>
            {!loading && !error && records.length > 0 && <nav aria-label={t('Billing history pagination')} className='flex w-full flex-wrap items-center justify-between gap-3'>
              <span className='text-muted-foreground text-sm'>{total === undefined ? t('Record total unavailable') : <>{t('Showing')} {(page - 1) * pageSize + 1}-{Math.min(page * pageSize, total)} {t('of')} {total}</>}</span>
              <div className='flex items-center gap-2'>
                <Button type='button' variant='outline' size='icon' className='min-h-11 min-w-11' onClick={() => handlePageChange(page - 1)} disabled={total === undefined || page <= 1} aria-label={t('Previous page')}><HugeiconsIcon icon={ArrowLeft01Icon} aria-hidden='true' className='size-4' /></Button>
                <span className='text-muted-foreground text-sm'>{page} / {totalPages ?? t('Unknown')}</span>
                <Button type='button' variant='outline' size='icon' className='min-h-11 min-w-11' onClick={() => handlePageChange(page + 1)} disabled={totalPages === undefined || page >= totalPages} aria-label={t('Next page')}><HugeiconsIcon icon={ArrowRight01Icon} aria-hidden='true' className='size-4' /></Button>
              </div>
            </nav>}
            <div className='flex w-full justify-end'><Button className='min-h-11' variant='outline' onClick={() => onOpenChange(false)}>{t('Close')}</Button></div>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Confirm Complete Order Dialog */}
      <AlertDialog
        open={!!confirmTradeNo}
        onOpenChange={(open) => !open && !completing && setConfirmTradeNo(null)}
      >
        <AlertDialogContent className='flex max-h-[calc(100dvh-2rem)] min-w-0 flex-col gap-0 overflow-hidden p-0 [&_[data-myapi-button]]:min-h-11 [&_[data-slot=select-trigger]]:min-h-11 [&_[role=combobox]]:min-h-11 [&_[data-myapi-button]]:min-w-11'>
          <AlertDialogHeader className='shrink-0 border-b px-4 py-4 sm:px-6'>
            <AlertDialogTitle>{t('Complete Order')}</AlertDialogTitle>
          </AlertDialogHeader>
          <section role='region' tabIndex={0} aria-label={t('Confirmation details')} className='min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain px-4 py-4 [overflow-wrap:anywhere] sm:px-6'>
            <AlertDialogDescription>
              {t(
                'Are you sure you want to manually complete this order? The user will be credited with the corresponding quota.'
              )}
              <code className='mt-3 block font-mono text-xs break-words [overflow-wrap:anywhere]'>{confirmTradeNo}</code>
            </AlertDialogDescription>
            {completionError && <p role='alert' className='text-destructive mt-3 text-sm'>{completionFailure || completionError}</p>}
          </section>
          <AlertDialogFooter className='m-0 shrink-0 border-t px-4 py-4 sm:px-6'>
            <AlertDialogCancel disabled={completing}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleConfirmComplete}
              disabled={completing}
            >
              {completing ? t('Processing...') : t('Confirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
