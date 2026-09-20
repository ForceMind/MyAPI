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
import { useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { resolveChannelOperationAttempt, handleCopyChannel } from '../../lib'
import { useChannels } from '../channels-provider'

type CopyChannelDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CopyChannelDialog({
  open,
  onOpenChange,
}: CopyChannelDialogProps) {
  const { t } = useTranslation()
  const { currentRow } = useChannels()
  const queryClient = useQueryClient()
  const [suffix, setSuffix] = useState('_copy')
  const [resetBalance, setResetBalance] = useState(true)
  const [isCopying, setIsCopying] = useState(false)
  const operationRef = useRef<{
    fingerprint: string
    operationKey: string
  } | null>(null)

  if (!currentRow) return null

  const handleCopy = async () => {
    const fingerprint = JSON.stringify({
      channelId: currentRow.id,
      suffix,
      resetBalance,
    })
    const operation = resolveChannelOperationAttempt(
      operationRef.current,
      fingerprint
    )
    operationRef.current = operation
    setIsCopying(true)

    await handleCopyChannel(
      currentRow.id,
      {
        suffix,
        reset_balance: resetBalance,
        operation_key: operation.operationKey,
      },
      queryClient,
      () => {
        onOpenChange(false)
        operationRef.current = null
        setSuffix('_copy')
        setResetBalance(true)
      }
    )

    setIsCopying(false)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) {
          operationRef.current = null
        }
        onOpenChange(nextOpen)
      }}
      title={t('Copy Channel')}
      description={
        <>
          {t('Create a copy of:')}
          <strong>{currentRow.name}</strong>
        </>
      }
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={isCopying}
          >
            {t('Cancel')}
          </Button>
          <Button onClick={handleCopy} disabled={isCopying}>
            {isCopying && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {isCopying ? t('Copying...') : t('Copy Channel')}
          </Button>
        </>
      }
    >
      <div className='space-y-4 py-4'>
        <div className='space-y-2'>
          <Label htmlFor='suffix'>{t('Name Suffix')}</Label>
          <Input
            id='suffix'
            placeholder={t('_copy')}
            value={suffix}
            onChange={(e) => setSuffix(e.target.value)}
            disabled={isCopying}
          />
          <p className='text-muted-foreground text-xs'>
            {t('New name will be:')} {currentRow.name}
            {suffix}
          </p>
        </div>

        <div className='flex items-center space-x-2'>
          <Checkbox
            id='reset-balance'
            checked={resetBalance}
            onCheckedChange={(checked) => setResetBalance(!!checked)}
            disabled={isCopying}
          />
          <Label htmlFor='reset-balance' className='text-sm font-normal'>
            {t('Reset balance and used quota')}
          </Label>
        </div>
      </div>
    </Dialog>
  )
}
