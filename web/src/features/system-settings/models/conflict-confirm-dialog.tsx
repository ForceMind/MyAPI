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
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
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

export type ConflictItem = {
  channel: string
  model: string
  current: string
  newVal: string
}

type ConflictConfirmDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  conflicts: ConflictItem[]
  onConfirm: () => void
  isLoading?: boolean
}

export function ConflictConfirmDialog({
  open,
  onOpenChange,
  conflicts,
  onConfirm,
  isLoading = false,
}: ConflictConfirmDialogProps) {
  const { t } = useTranslation()
  return (
    <AlertDialog open={open} onOpenChange={(nextOpen) => { if (nextOpen || !isLoading) onOpenChange(nextOpen) }}>
      <AlertDialogContent className='flex max-h-[calc(100dvh-2rem)] min-w-0 flex-col gap-0 overflow-hidden p-0 sm:max-w-4xl [&_[data-myapi-button]]:min-h-11 [&_[data-myapi-button]]:min-w-11'>
        <AlertDialogHeader className='shrink-0 border-b px-4 py-4 sm:px-6'>
          <AlertDialogTitle>{t('Confirm Billing Conflicts')}</AlertDialogTitle>
          <AlertDialogDescription>
            {t(
              'The following models have billing type conflicts (fixed price vs ratio billing). Confirm to proceed with the changes.'
            )}
          </AlertDialogDescription>
        </AlertDialogHeader>

        <section role='region' tabIndex={0} aria-label={t('Billing conflict details')} className='min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain px-4 py-4 [overflow-wrap:anywhere] sm:px-6'>
        <StaticDataTable
          className='min-w-0'
          data={conflicts}
          columns={[
            {
              id: 'channel',
              header: t('Channel'),
              cellClassName: 'font-medium whitespace-normal break-words [overflow-wrap:anywhere]',
              cell: (conflict) => conflict.channel,
            },
            {
              id: 'model',
              header: t('Model'),
              cellClassName: 'font-mono text-sm whitespace-normal break-words [overflow-wrap:anywhere]',
              cell: (conflict) => conflict.model,
            },
            {
              id: 'current',
              header: t('Current Billing'),
              cell: (conflict) => (
                <pre className='text-sm whitespace-pre-wrap'>
                  {conflict.current}
                </pre>
              ),
            },
            {
              id: 'new',
              header: t('Change To'),
              cell: (conflict) => (
                <pre className='text-sm whitespace-pre-wrap'>
                  {conflict.newVal}
                </pre>
              ),
            },
          ]}
        />

        </section>
        <AlertDialogFooter className='m-0 shrink-0 border-t px-4 py-4 sm:px-6'>
          <AlertDialogCancel disabled={isLoading}>
            {t('Cancel')}
          </AlertDialogCancel>
          <AlertDialogAction onClick={onConfirm} disabled={isLoading}>
            {isLoading ? t('Applying...') : t('Confirm Changes')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
