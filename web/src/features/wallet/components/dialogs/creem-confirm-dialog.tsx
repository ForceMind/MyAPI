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
import { Spinner } from '@/components/ui/spinner'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { formatNumber } from '@/lib/format'

import { formatCreemPrice } from '../../lib/format'
import type { CreemProduct } from '../../types'

interface CreemConfirmDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
  onOpenHistory?: () => void
  product: CreemProduct | null
  processing: boolean
  outcomeUnknown?: boolean
}

export function CreemConfirmDialog({
  open,
  onOpenChange,
  onConfirm,
  onOpenHistory,
  product,
  processing,
  outcomeUnknown = false,
}: CreemConfirmDialogProps) {
  const { t } = useTranslation()

  if (!product) return null

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => (nextOpen || !processing) && onOpenChange(nextOpen)}
      showCloseButton={!processing}
      title={t('Confirm Creem Purchase')}
      description={t('Review your purchase details before proceeding.')}
      contentClassName='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-[425px]'
      footerClassName='grid grid-cols-2 gap-2 sm:flex [&_[data-myapi-button]]:min-h-11 [&_[data-slot=select-trigger]]:min-h-11 [&_[role=combobox]]:min-h-11 [&_[data-myapi-button]]:min-w-11'
      contentHeight='auto'
      bodyClassName='space-y-4 [overflow-wrap:anywhere]'
      footer={
        <>
          <Button
            className='min-h-11'
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={processing}
          >
            {t('Cancel')}
          </Button>
          <Button className='min-h-11' onClick={onConfirm} disabled={processing || outcomeUnknown}>
            {processing && <Spinner aria-hidden='true' data-icon='inline-start' />}
            {t('Confirm Payment')}
          </Button>
        </>
      }
    >
      {outcomeUnknown && <p role='alert' className='text-warning text-sm'>{t('Payment outcome is unconfirmed. New payments are locked; review billing history.')}</p>}
      {outcomeUnknown && onOpenHistory && <Button variant='outline' className='min-h-11' onClick={onOpenHistory}>{t('Billing History')}</Button>}
      <div role='region' tabIndex={0} aria-label={t('Payment details')} className='space-y-3 py-3 sm:space-y-4 sm:py-4'>
        <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
          <span className='text-muted-foreground'>{t('Product')}</span>
          <span className='min-w-0 text-right font-medium break-words [overflow-wrap:anywhere]'>{product.name}</span>
        </div>
        <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
          <span className='text-muted-foreground'>{t('Price')}</span>
          <span className='text-primary font-medium'>
            {formatCreemPrice(product.price, product.currency)}
          </span>
        </div>
        <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
          <span className='text-muted-foreground'>{t('Quota')}</span>
          <span className='font-medium'>{formatNumber(product.quota)}</span>
        </div>
      </div>
    </Dialog>
  )
}
