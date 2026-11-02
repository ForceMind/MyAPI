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

import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

type ConfirmDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: React.ReactNode
  disabled?: boolean
  desc: React.JSX.Element | string
  cancelBtnText?: string
  confirmText?: React.ReactNode
  destructive?: boolean
  handleConfirm: () => void
  isLoading?: boolean
  className?: string
  children?: React.ReactNode
  returnFocus?: () => HTMLElement | null
}

export function ConfirmDialog(props: ConfirmDialogProps) {
  const { t } = useTranslation()
  const {
    title,
    desc,
    children,
    className,
    confirmText,
    cancelBtnText,
    destructive,
    isLoading,
    disabled = false,
    handleConfirm,
    returnFocus,
    ...actions
  } = props
  return (
    <AlertDialog {...actions}>
      <AlertDialogContent finalFocus={returnFocus} className={cn('flex max-h-[calc(100dvh-2rem)] flex-col gap-0 p-0 [&_[data-myapi-button]]:min-h-11 [&_[data-slot=select-trigger]]:min-h-11 [&_[role=combobox]]:min-h-11 [&_[data-myapi-button]]:min-w-11', className, 'overflow-hidden')}>
        <AlertDialogHeader className='shrink-0 border-b px-4 py-4 text-start sm:px-6'>
          <AlertDialogTitle>{title}</AlertDialogTitle>
        </AlertDialogHeader>
        <section aria-label={t('Confirmation details')} tabIndex={0} className='min-h-0 min-w-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4 [overflow-wrap:anywhere] sm:px-6'>
          <AlertDialogDescription render={<div />}>
            {desc}
          </AlertDialogDescription>
          {children}
        </section>
        <AlertDialogFooter className='m-0 shrink-0 rounded-none border-t px-4 py-3 sm:px-6'>
          <AlertDialogCancel className='min-h-11 min-w-11' disabled={isLoading}>
            {cancelBtnText ?? t('Cancel')}
          </AlertDialogCancel>
          <Button
            className='min-h-11 min-w-11'
            variant={destructive ? 'destructive' : 'default'}
            onClick={handleConfirm}
            disabled={disabled || isLoading}
          >
            {confirmText ?? t('Continue')}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
