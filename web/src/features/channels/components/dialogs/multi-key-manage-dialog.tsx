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
import { Delete02Icon, Loading03Icon, PowerIcon, PowerOffIcon, Refresh01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import {
  getMultiKeyStatus,
  enableMultiKey,
  disableMultiKey,
  deleteMultiKey,
  enableAllMultiKeys,
  disableAllMultiKeys,
  deleteDisabledMultiKeys,
} from '../../api'
import { MULTI_KEY_FILTER_OPTIONS } from '../../constants'
import {
  channelsQueryKeys,
  formatTimestamp,
  getMultiKeyStatusConfig,
  getMultiKeyConfirmMessage,
  isDestructiveAction,
  notifyChannelMutationSuccess,
} from '../../lib'
import type { KeyStatus, MultiKeyConfirmAction } from '../../types'
import { useChannels } from '../channels-provider'
import { StatisticsCard } from './multi-key-statistics-card'
import { MultiKeyTableRowActions } from './multi-key-table-row-actions'
import { useChannelTask } from '../../hooks/use-channel-task'

type MultiKeyManageDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  returnFocus?: () => HTMLElement | null
}

export function MultiKeyManageDialog({
  open,
  onOpenChange,
  returnFocus,
}: MultiKeyManageDialogProps) {
  const { t } = useTranslation()
  const { currentRow } = useChannels()
  const queryClient = useQueryClient()
  const task = useChannelTask(open, currentRow?.id)
  const readGeneration = useRef(0)
  const refreshButton = useRef<HTMLButtonElement>(null)
  const confirmationOpener = useRef<HTMLElement | null>(null)
  const [readError, setReadError] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const currentUser = useAuthStore((s) => s.auth.user)
  const canEditSensitive = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )

  // Data state
  const [isLoading, setIsLoading] = useState(false)
  const [keys, setKeys] = useState<KeyStatus[]>([])
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [total, setTotal] = useState<number | undefined>()
  const [totalPages, setTotalPages] = useState<number | undefined>()
  const [enabledCount, setEnabledCount] = useState<number | undefined>()
  const [manualDisabledCount, setManualDisabledCount] = useState<number | undefined>()
  const [autoDisabledCount, setAutoDisabledCount] = useState<number | undefined>()

  // UI state
  const [statusFilter, setStatusFilter] = useState<number | null>(null)
  const [confirmAction, setConfirmAction] =
    useState<MultiKeyConfirmAction | null>(null)
  const [isPerformingAction, setIsPerformingAction] = useState(false)
  const openConfirmation = (action: MultiKeyConfirmAction) => {
    confirmationOpener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    setActionError(null)
    setConfirmAction(action)
  }

  // Reset and load data when dialog opens
  useEffect(() => {
    if (open && currentRow) {
      setCurrentPage(1)
      setStatusFilter(null)
      setConfirmAction(null)
      setActionError(null)
      setIsPerformingAction(false)
      loadKeyStatus(1, pageSize, null)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, currentRow?.id, task.identity])

  const loadKeyStatus = async (
    page: number = currentPage,
    size: number = pageSize,
    status: number | null = statusFilter
  ) => {
    if (!currentRow) return
    const snapshot = task.capture()
    const request = ++readGeneration.current
    if (!snapshot.isCurrent()) return
    setIsLoading(true)
    setReadError(null)
    setKeys([])
    setTotal(undefined)
    setTotalPages(undefined)
    setEnabledCount(undefined)
    setManualDisabledCount(undefined)
    setAutoDisabledCount(undefined)
    try {
      const response = await getMultiKeyStatus(
        currentRow.id,
        page,
        size,
        status === null ? undefined : status
      )
      if (!snapshot.isCurrent() || request !== readGeneration.current) return

      if (response.success && response.data) {
        setKeys(response.data.keys || [])
        const counts = [response.data.total, response.data.total_pages, response.data.enabled_count, response.data.manual_disabled_count, response.data.auto_disabled_count].map((value) => typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 ? value : undefined)
        setTotal(counts[0])
        setCurrentPage(response.data.page || 1)
        setPageSize(response.data.page_size || 10)
        setTotalPages(counts[1])
        setEnabledCount(counts[2])
        setManualDisabledCount(counts[3])
        setAutoDisabledCount(counts[4])
      } else {
        setReadError(response.message || t('Failed to load key status'))
        toast.error(response.message || t('Failed to load key status'))
      }
    } catch (error: unknown) {
      if (!snapshot.isCurrent() || request !== readGeneration.current) return
      setReadError(error instanceof Error ? error.message : t('Failed to load key status'))
      toast.error(
        error instanceof Error ? error.message : t('Failed to load key status')
      )
    } finally {
      if (snapshot.isCurrent() && request === readGeneration.current) setIsLoading(false)
    }
  }

  const handleStatusFilterChange = (value: string) => {
    const newFilter = value === 'all' ? null : Number.parseInt(value)
    setStatusFilter(newFilter)
    setCurrentPage(1)
    loadKeyStatus(1, pageSize, newFilter)
  }

  const handlePageChange = (newPage: number) => {
    setCurrentPage(newPage)
    loadKeyStatus(newPage, pageSize)
  }

  const performAction = async () => {
    if (!confirmAction || !currentRow) return
    if (
      !canEditSensitive &&
      (confirmAction.type === 'delete' ||
        confirmAction.type === 'delete-disabled')
    ) {
      setConfirmAction(null)
      return
    }

    const operation = task.begin()
    if (!operation) return

    setIsPerformingAction(true)
    setActionError(null)
    try {
      const { type, keyIndex } = confirmAction
      let response

      // Execute the appropriate action
      if (type === 'enable' && keyIndex !== undefined) {
        response = await enableMultiKey(currentRow.id, keyIndex)
      } else if (type === 'disable' && keyIndex !== undefined) {
        response = await disableMultiKey(currentRow.id, keyIndex)
      } else if (type === 'delete' && keyIndex !== undefined) {
        response = await deleteMultiKey(currentRow.id, keyIndex)
      } else if (type === 'enable-all') {
        response = await enableAllMultiKeys(currentRow.id)
      } else if (type === 'disable-all') {
        response = await disableAllMultiKeys(currentRow.id)
      } else if (type === 'delete-disabled') {
        response = await deleteDisabledMultiKeys(currentRow.id)
      }
      if (!operation.isCurrent()) return

      if (response?.success) {
        notifyChannelMutationSuccess(
          response,
          response.message || t('Operation successful')
        )
        queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
        setConfirmAction(null)

        // Reload data - reset to page 1 for bulk actions
        const isBulkAction = type.includes('all') || type === 'delete-disabled'
        if (isBulkAction) {
          setCurrentPage(1)
          loadKeyStatus(1, pageSize)
        } else {
          loadKeyStatus(currentPage, pageSize)
        }
      } else {
        setActionError(response?.message || t('Operation failed'))
        toast.error(response?.message || t('Operation failed'))
      }
    } catch (error: unknown) {
      if (!operation.isCurrent()) return
      setActionError(error instanceof Error ? error.message : t('Operation failed'))
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      if (operation.isCurrent()) setIsPerformingAction(false)
      operation.finish()
    }
  }

  const renderStatusBadge = (status: number) => {
    const config = getMultiKeyStatusConfig(status)
    return (
      <StatusBadge
        label={t(config.label)}
        variant={config.variant}
        showDot
        copyable={false}
      />
    )
  }

  const formatKeyTimestamp = (timestamp?: number) => {
    if (!timestamp) return '-'
    return formatTimestamp(timestamp)
  }

  if (!currentRow) return null

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(next) => { if (!task.isPending()) onOpenChange(next) }}
        returnFocus={returnFocus}
        showCloseButton={!isPerformingAction}
        title={
          <>
            {t('Multi-Key Management')}
            <StatusBadge
              label={currentRow.name}
              variant='neutral'
              copyable={false}
            />
            {currentRow.channel_info?.multi_key_mode && (
              <StatusBadge
                label={
                  currentRow.channel_info.multi_key_mode === 'random'
                    ? t('Random')
                    : t('Polling')
                }
                variant='neutral'
                copyable={false}
              />
            )}
          </>
        }
        description={t(
          'Manage multi-key status and configuration for this channel'
        )}
        contentClassName='h-dvh max-h-dvh w-screen max-w-none sm:h-auto sm:max-h-[calc(100dvh-2rem)] sm:max-w-5xl [&_button]:min-h-11 [&_button]:min-w-11'
        titleClassName='flex flex-wrap items-center gap-2'
        contentHeight='min(72vh, 720px)'
        bodyClassName='space-y-4'
      >
        <div className='flex min-h-0 flex-1 flex-col space-y-4'>
          {readError ? <div role='alert' className='text-destructive text-sm break-words'>{readError}</div> : null}
          {/* Statistics */}
          <div className='grid shrink-0 grid-cols-1 gap-3 sm:grid-cols-3'>
            <StatisticsCard
              label={t('Enabled')}
              count={enabledCount}
              total={total}
            />
            <StatisticsCard
              label={t('Manual Disabled')}
              count={manualDisabledCount}
              total={total}
            />
            <StatisticsCard
              label={t('Auto Disabled')}
              count={autoDisabledCount}
              total={total}
            />
          </div>

          <Separator className='shrink-0' />

          {/* Toolbar */}
          <div className='flex shrink-0 flex-wrap items-center justify-between gap-3'>
            <Select
              items={MULTI_KEY_FILTER_OPTIONS.map((option) => ({
                value: option.value,
                label: t(option.label),
              }))}
              value={statusFilter === null ? 'all' : statusFilter.toString()}
              onValueChange={(v) => v !== null && handleStatusFilterChange(v)}
            >
              <SelectTrigger aria-label={t('Status')} className='min-h-11 w-40'>
                <SelectValue placeholder={t('All Status')} />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {MULTI_KEY_FILTER_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {t(option.label)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>

            <div className='flex flex-wrap items-center gap-2'>
              <Button
                variant='outline'
                size='sm'
                ref={refreshButton}
                aria-label={t('Refresh')}
                onClick={() => loadKeyStatus()}
                disabled={isLoading}
              >
                <HugeiconsIcon icon={Refresh01Icon} aria-hidden='true' className='h-4 w-4' />
              </Button>

              {((manualDisabledCount ?? 0) + (autoDisabledCount ?? 0)) > 0 && (
                <Button
                  variant='default'
                  size='sm'
                  onClick={() => openConfirmation({ type: 'enable-all' })}
                >
                  <HugeiconsIcon icon={PowerIcon} aria-hidden='true' className='mr-2 h-4 w-4' />
                  {t('Enable All')}
                </Button>
              )}

              {enabledCount !== undefined && enabledCount > 0 && (
                <Button
                  variant='destructive'
                  size='sm'
                  onClick={() => openConfirmation({ type: 'disable-all' })}
                >
                  <HugeiconsIcon icon={PowerOffIcon} aria-hidden='true' className='mr-2 h-4 w-4' />
                  {t('Disable All')}
                </Button>
              )}

              {autoDisabledCount !== undefined && autoDisabledCount > 0 && (
                <Button
                  variant='destructive'
                  size='sm'
                  onClick={() => {
                    if (!canEditSensitive) return
                    openConfirmation({ type: 'delete-disabled' })
                  }}
                  disabled={!canEditSensitive}
                  title={
                    canEditSensitive
                      ? undefined
                      : t('No permission to perform this action')
                  }
                >
                  <HugeiconsIcon icon={Delete02Icon} aria-hidden='true' className='mr-2 h-4 w-4' />
                  {t('Delete Auto-Disabled')}
                </Button>
              )}
            </div>
          </div>
          {!canEditSensitive && (
            <p className='text-muted-foreground text-xs'>
              {t('No permission to perform this action')}
            </p>
          )}

          {/* Table */}
          <div className='min-w-0 overflow-x-auto rounded-md border'>
            {isLoading ? (
              <div className='flex items-center justify-center py-12'>
                <HugeiconsIcon icon={Loading03Icon} aria-hidden='true' className='text-muted-foreground h-8 w-8 animate-spin' />
              </div>
            ) : null}
            {!isLoading && !readError && keys.length === 0 ? (
              <div className='text-muted-foreground py-12 text-center'>
                {t('No keys found')}
              </div>
            ) : null}
            {!isLoading && keys.length > 0 ? (
              <StaticDataTable
                className='rounded-none border-0'
                tableClassName='min-w-[800px]'
                data={keys}
                getRowKey={(key) => key.index}
                columns={[
                  {
                    id: 'index',
                    header: t('Index'),
                    className: 'w-20',
                    cellClassName: 'font-mono text-sm',
                    cell: (key) => `#${key.index + 1}`,
                  },
                  {
                    id: 'status',
                    header: t('Status'),
                    className: 'w-32',
                    cell: (key) => renderStatusBadge(key.status),
                  },
                  {
                    id: 'reason',
                    header: t('Disabled Reason'),
                    className: 'min-w-[200px]',
                    cellClassName: 'max-w-xs truncate text-sm',
                    cell: (key) => key.reason || '-',
                  },
                  {
                    id: 'disabled-time',
                    header: t('Disabled Time'),
                    className: 'w-44',
                    cellClassName: 'text-muted-foreground text-sm',
                    cell: (key) => formatKeyTimestamp(key.disabled_time),
                  },
                  {
                    id: 'actions',
                    header: t('Actions'),
                    className: 'text-right',
                    cell: (key) => (
                      <MultiKeyTableRowActions
                        keyIndex={key.index}
                        status={key.status}
                        canDelete={canEditSensitive}
                        onAction={openConfirmation}
                      />
                    ),
                  },
                ]}
              />
            ) : null}
          </div>

          {/* Pagination */}
          {totalPages !== undefined && totalPages > 1 && (
            <div className='flex shrink-0 items-center justify-between'>
              <div className='text-muted-foreground text-sm'>
                {t('Page {{current}} of {{total}}', {
                  current: currentPage,
                  total: totalPages,
                })}
              </div>
              <div className='flex gap-2'>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => handlePageChange(currentPage - 1)}
                  disabled={currentPage === 1 || isLoading}
                >
                  {t('Previous')}
                </Button>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => handlePageChange(currentPage + 1)}
                  disabled={currentPage >= totalPages || isLoading}
                >
                  {t('Next')}
                </Button>
              </div>
            </div>
          )}
        </div>
      </Dialog>

      {/* Confirmation Dialog */}
      <AlertDialog
        open={confirmAction !== null}
        onOpenChange={(next) => { if (!next && !task.isPending()) { setConfirmAction(null); setActionError(null) } }}
      >
        <AlertDialogContent finalFocus={() => {
          const original = confirmationOpener.current
          if (original?.isConnected) return original
          const refresh = refreshButton.current
          return refresh?.disabled ? refresh.closest<HTMLElement>('[data-slot=dialog-content]') : refresh
        }} className='[&_button]:min-h-11 [&_button]:min-w-11'>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Confirm Action')}</AlertDialogTitle>
            <AlertDialogDescription>{t(getMultiKeyConfirmMessage(confirmAction))}</AlertDialogDescription>
          </AlertDialogHeader>
          <p className='text-sm break-words'>{currentRow.name} · {t('ID')}: {currentRow.id}{confirmAction?.keyIndex !== undefined ? ` · #${confirmAction.keyIndex + 1}` : ''}</p>
          {actionError ? <div role='alert' className='text-destructive text-sm break-words'>{actionError}</div> : null}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isPerformingAction}>{t('Cancel')}</AlertDialogCancel>
            <Button onClick={performAction} disabled={isPerformingAction} variant={isDestructiveAction(confirmAction) ? 'destructive' : 'default'}>{t('Continue')}</Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
