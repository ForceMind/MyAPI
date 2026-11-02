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
import { Loading03Icon, Refresh01Icon, Wallet01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import { getCodexUsage, updateChannelBalance } from '../../api'
import { channelsQueryKeys } from '../../lib'
import { useChannels } from '../channels-provider'
import { ChannelQuotaHistory } from './channel-quota-history'
import {
  CodexUsageDialog,
  type CodexUsageDialogData,
} from './codex-usage-dialog'
import { useChannelTask } from '../../hooks/use-channel-task'

type BalanceQueryDialogProps = {
  initialRawResponse?: string
  open: boolean
  onOpenChange: (open: boolean) => void
  returnFocus?: () => HTMLElement | null
}

export function BalanceQueryDialog(props: BalanceQueryDialogProps) {
  const { currentRow } = useChannels()
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  return <BalanceQueryTask key={JSON.stringify([identity, currentRow?.id])} {...props} />
}

function BalanceQueryTask(props: BalanceQueryDialogProps) {
  const { t } = useTranslation()
  const { currentRow, setCurrentRow } = useChannels()
  const queryClient = useQueryClient()
  const task = useChannelTask(props.open, currentRow?.id)
  const readGeneration = useRef(0)
  const [actionError, setActionError] = useState<string | null>(null)
  const [isQuerying, setIsQuerying] = useState(false)
  const [balance, setBalance] = useState<number | null>(null)
  const [balanceUpdatedTime, setBalanceUpdatedTime] = useState<number | null>(
    null
  )
  const [rawResponse, setRawResponse] = useState<string | null>(
    props.initialRawResponse ?? null
  )
  const [codexUsageResponse, setCodexUsageResponse] =
    useState<CodexUsageDialogData | null>(null)

  const isCodex = currentRow?.type === 57

  const handleQueryCodexUsage = async () => {
    const row = currentRow
    if (!row) return
    const operation = task.begin()
    if (!operation) return
    const request = ++readGeneration.current
    setActionError(null)
    setIsQuerying(true)
    try {
      const res = await getCodexUsage(row.id)
      if (!operation.isCurrent() || request !== readGeneration.current) return
      if (!res.success) {
        throw new Error(res.message || t('Failed to fetch usage'))
      }
      setCodexUsageResponse(res)
    } catch (error: unknown) {
      if (!operation.isCurrent() || request !== readGeneration.current) return
      const message =
        error instanceof Error ? error.message : t('Failed to fetch usage')
      setCodexUsageResponse({ success: false, message })
      toast.error(message)
    } finally {
      if (operation.isCurrent() && request === readGeneration.current) setIsQuerying(false)
      operation.finish()
    }
  }

  useEffect(() => {
    if (!isCodex) return
    if (!props.open) return
    handleQueryCodexUsage()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.open, isCodex, currentRow?.id, task.identity])

  if (!currentRow) return null

  const handleQueryBalance = async () => {
    const operation = task.begin()
    if (!operation) return
    const request = ++readGeneration.current
    setActionError(null)
    setIsQuerying(true)
    try {
      const response = await updateChannelBalance(currentRow.id)
      if (!operation.isCurrent() || request !== readGeneration.current) return
      if (response.success && typeof response.balance === 'number' && Number.isFinite(response.balance)) {
        const newBalance = response.balance
        const now = Math.floor(Date.now() / 1000)

        setBalance(newBalance)
        setBalanceUpdatedTime(now)
        toast.success(t('Balance updated successfully'))

        // Update currentRow immediately with new balance and timestamp
        setCurrentRow({
          ...currentRow,
          balance: newBalance,
          balance_updated_time: now,
        })

        // Invalidate queries to refresh the table
        await queryClient.invalidateQueries({
          queryKey: channelsQueryKeys.lists(),
        })
        await queryClient.invalidateQueries({
          queryKey: ['channel-quota-history', currentRow.id],
        })
        setRawResponse(null)
      } else if (response.success && typeof response.raw_response === 'string') {
        setRawResponse(response.raw_response)
      } else {
        setActionError(response.message || t('Failed to query balance'))
        toast.error(response.message || t('Failed to query balance'))
      }
    } catch (error: unknown) {
      if (!operation.isCurrent() || request !== readGeneration.current) return
      setActionError(error instanceof Error ? error.message : t('Failed to query balance'))
      toast.error(
        error instanceof Error ? error.message : t('Failed to query balance')
      )
    } finally {
      if (operation.isCurrent() && request === readGeneration.current) setIsQuerying(false)
      operation.finish()
    }
  }

  const handleClose = () => {
    readGeneration.current += 1
    setIsQuerying(false)
    setBalance(null)
    setBalanceUpdatedTime(null)
    setRawResponse(null)
    setCodexUsageResponse(null)
    props.onOpenChange(false)
  }

  const formatBalance = (bal: number) =>
    formatCurrencyFromUSD(bal, {
      digitsLarge: 2,
      digitsSmall: 4,
      abbreviate: false,
    })

  const formatDate = (timestamp: number) => {
    if (timestamp === 0) return t('Never')
    if (!Number.isFinite(timestamp)) return t('Unknown')
    return formatTimestampToDate(timestamp)
  }

  if (isCodex) {
    return (
      <CodexUsageDialog
        open={props.open}
        onOpenChange={(v) => {
          if (!v) handleClose()
        }}
        channelName={currentRow.name}
        channelId={currentRow.id}
        response={codexUsageResponse}
        onRefresh={handleQueryCodexUsage}
        isRefreshing={isQuerying}
      />
    )
  }

  let balanceDisplay = t('Unknown')
  if (balance !== null) {
    balanceDisplay = formatBalance(balance)
  } else if (typeof currentRow.balance === 'number' && Number.isFinite(currentRow.balance)) {
    balanceDisplay = formatBalance(currentRow.balance)
  }

  return (
    <Dialog
      open={props.open}
      returnFocus={props.returnFocus}
      onOpenChange={handleClose}
      title={t('Query Balance')}
      description={
        <>
          {t('Update balance for:')}
          <strong>{currentRow.name}</strong>
        </>
      }
      contentHeight='auto'
      contentClassName='h-dvh max-h-dvh w-screen max-w-none sm:h-auto sm:max-h-[calc(100dvh-2rem)] [&_button]:min-h-11 [&_button]:min-w-11'
      bodyClassName='space-y-4'
      footer={
        <Button variant='outline' onClick={handleClose} disabled={isQuerying}>
          {t('Close')}
        </Button>
      }
    >
      <div className='space-y-4 py-4'>
        {actionError ? <div role='alert' className='text-destructive text-sm break-words'>{actionError}</div> : null}
        {rawResponse !== null ? (
          <>
            <Alert>
              <AlertTitle>{t('Balance response not recognized')}</AlertTitle>
              <AlertDescription>
                {t(
                  'The upstream response is valid JSON, but it does not match the OpenAI credit_summary format. The channel balance was not updated.'
                )}
              </AlertDescription>
            </Alert>
            <CodeBlock
              code={rawResponse}
              language='json'
              maxExpandedLines={24}
              showLineNumbers
              title={t('Upstream JSON response')}
            >
              <CodeBlockCopyButton />
            </CodeBlock>
          </>
        ) : (
          <>
            {/* Current Balance Display */}
            <div className='bg-muted/50 rounded-lg border p-4'>
              <div className='text-muted-foreground mb-2 flex items-center gap-2 text-sm'>
                <IconBadge tone='success' size='xs'>
                  <HugeiconsIcon icon={Wallet01Icon} aria-hidden='true' />
                </IconBadge>
                <span>{t('Current Balance')}</span>
              </div>
              <div className='text-2xl font-bold'>
                {balanceDisplay}
              </div>
              <div className='text-muted-foreground mt-2 text-xs'>
                {t('Last updated:')}{' '}
                {formatDate(
                  balanceUpdatedTime ?? currentRow.balance_updated_time
                )}
              </div>
            </div>
            <ChannelQuotaHistory channel={currentRow} open={props.open} />
          </>
        )}

        {/* Balance Update Button */}
        <Button
          className='w-full'
          onClick={handleQueryBalance}
          disabled={isQuerying}
        >
          {isQuerying && <HugeiconsIcon icon={Loading03Icon} aria-hidden='true' className='mr-2 h-4 w-4 animate-spin' />}
          {!isQuerying && <HugeiconsIcon icon={Refresh01Icon} aria-hidden='true' className='mr-2 h-4 w-4' />}
          {isQuerying ? t('Querying...') : t('Update Balance')}
        </Button>
      </div>
    </Dialog>
  )
}
