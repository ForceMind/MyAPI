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

import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'

type DescriptionDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  modelName: string
  description: string
  returnFocus?: () => HTMLElement | null
}

export function DescriptionDialog({ open, onOpenChange, modelName, description, returnFocus }: DescriptionDialogProps) {
  const { t } = useTranslation()
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent finalFocus={returnFocus} className='flex h-dvh max-h-dvh w-full max-w-none flex-col gap-0 overflow-hidden rounded-none p-0 sm:h-auto sm:max-h-[calc(100dvh-3rem)] sm:max-w-2xl sm:rounded-xl [&_[data-slot=dialog-close]]:min-h-11 [&_[data-slot=dialog-close]]:min-w-11'>
        <DialogHeader className='shrink-0 border-b px-4 py-4 pr-14 text-left sm:px-6'>
          <DialogTitle className='wrap-anywhere'>{modelName}</DialogTitle>
          <DialogDescription>{t('Model Description')}</DialogDescription>
        </DialogHeader>
        <div className='min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-5 sm:px-6'>
          <p className='text-sm leading-relaxed wrap-anywhere whitespace-pre-wrap'>{description}</p>
        </div>
        <DialogFooter className='shrink-0 border-t px-4 py-3 sm:px-6'><DialogClose render={<Button variant='outline' className='min-h-11' />}>{t('Close')}</DialogClose></DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
