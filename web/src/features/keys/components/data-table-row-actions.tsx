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
import {
  ArrowDataTransferHorizontalIcon,
  Copy01Icon,
  DashboardSpeed01Icon,
  Delete02Icon,
  Edit02Icon,
  FilterHorizontalIcon,
  Link01Icon,
  LinkSquare02Icon,
  Loading03Icon,
  PowerIcon,
  PowerOffIcon,
  SecurityCheckIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { Row } from '@tanstack/react-table'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTableRowActionMenu } from '@/components/data-table/core/row-action-menu'
import { Button } from '@/components/ui/button'
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuShortcut,
} from '@/components/ui/dropdown-menu'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useChatPresets } from '@/features/chat/hooks/use-chat-presets'
import { resolveChatUrl, type ChatPreset } from '@/features/chat/lib/chat-links'
import { sendToFluent } from '@/features/chat/lib/send-to-fluent'
import { encodeChannelConnectionInfo } from '@/lib/channel-connection-info'
import { copyToClipboard } from '@/lib/copy-to-clipboard'

import { updateApiKeyStatus } from '../api'
import { API_KEY_STATUS, ERROR_MESSAGES, SUCCESS_MESSAGES } from '../constants'
import { useKeyMaintenanceSession } from '../hooks/use-key-maintenance-session'
import { apiKeySchema, type ApiKey } from '../types'
import { useApiKeys } from './api-keys-provider'

function getServerAddress(): string {
  try {
    const raw = localStorage.getItem('status')
    if (raw) {
      const status = JSON.parse(raw)
      if (status.server_address) return status.server_address as string
    }
  } catch {
    /* empty */
  }
  return window.location.origin
}

type DataTableRowActionsProps<TData> =
  | {
      row: Row<TData>
      apiKey?: never
      onCredentialMaintenance?: () => void
      /** Only the "more" menu, for narrow rows and the inspector header. */
      menuOnly?: boolean
    }
  | {
      apiKey: ApiKey
      row?: never
      onCredentialMaintenance?: () => void
      menuOnly?: boolean
    }

export function DataTableRowActions<TData = ApiKey>(
  props: DataTableRowActionsProps<TData>
) {
  const { t } = useTranslation()
  const isCurrentSession = useKeyMaintenanceSession()
  const apiKey = apiKeySchema.parse(props.apiKey ?? props.row?.original)
  const {
    viewRequests,
    requestFocusId,
    setOpen,
    setCurrentRow,
    triggerRefresh,
    setResolvedKey,
    resolveRealKey,
    resolvedKeys,
    loadingKeys,
  } = useApiKeys()
  const isEnabled = apiKey.status === API_KEY_STATUS.ENABLED
  const { chatPresets, serverAddress } = useChatPresets()
  const [isTogglingStatus, setIsTogglingStatus] = useState(false)
  const resolvedRealKey = resolvedKeys[apiKey.id]
  const isRealKeyLoading = Boolean(loadingKeys[apiKey.id])

  const hasChatPresets = chatPresets.length > 0
  const toggleLabel = isEnabled ? t('Disable') : t('Enable')

  const handleMenuOpenChange = useCallback(
    (open: boolean) => {
      if (open && isCurrentSession() && !resolvedRealKey && !isRealKeyLoading) {
        void resolveRealKey(apiKey.id)
      }
    },
    [
      apiKey.id,
      isCurrentSession,
      isRealKeyLoading,
      resolvedRealKey,
      resolveRealKey,
    ]
  )

  const getCachedRealKey = useCallback(() => {
    if (!isCurrentSession()) return null
    if (resolvedRealKey) return resolvedRealKey
    void resolveRealKey(apiKey.id)
    toast.info(t('API key is loading, please try again in a moment'))
    return null
  }, [apiKey.id, isCurrentSession, resolvedRealKey, resolveRealKey, t])

  const handleOpenChatPreset = useCallback(
    async (preset: ChatPreset) => {
      if (!isCurrentSession()) return
      const realKey = await resolveRealKey(apiKey.id)
      if (!realKey || !isCurrentSession()) return

      if (preset.type === 'fluent') {
        const success = sendToFluent(realKey, serverAddress)
        if (success) {
          toast.success(t('Sent the API key to FluentRead.'))
        } else {
          toast.info(
            t(
              'FluentRead extension not detected. Please ensure it is installed and active.'
            )
          )
        }
        return
      }

      const resolvedUrl = resolveChatUrl({
        template: preset.url,
        apiKey: realKey,
        serverAddress,
      })

      if (!resolvedUrl) {
        toast.error(t('Invalid chat link. Please contact your administrator.'))
        return
      }

      if (typeof window === 'undefined') return

      try {
        window.open(resolvedUrl, '_blank', 'noopener')
      } catch {
        window.location.href = resolvedUrl
      }
    },
    [resolveRealKey, apiKey.id, isCurrentSession, serverAddress, t]
  )

  const handleToggleStatus = async (
    e?: React.MouseEvent<HTMLButtonElement>
  ) => {
    e?.stopPropagation()
    if (!isCurrentSession()) return
    const newStatus = isEnabled
      ? API_KEY_STATUS.DISABLED
      : API_KEY_STATUS.ENABLED

    setIsTogglingStatus(true)
    try {
      const result = await updateApiKeyStatus(apiKey.id, newStatus, {
        skipErrorHandler: true,
        skipBusinessError: true,
        skipAuthRefresh: true,
      })
      if (!isCurrentSession()) return
      if (result.success) {
        const message = isEnabled
          ? t(SUCCESS_MESSAGES.API_KEY_DISABLED)
          : t(SUCCESS_MESSAGES.API_KEY_ENABLED)
        toast.success(message)
        triggerRefresh()
      } else {
        toast.error(result.message || t(ERROR_MESSAGES.STATUS_UPDATE_FAILED))
      }
    } catch {
      if (isCurrentSession()) toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      if (isCurrentSession()) setIsTogglingStatus(false)
    }
  }

  let statusIcon = <HugeiconsIcon icon={PowerIcon} className='size-4' />
  if (isTogglingStatus) {
    statusIcon = (
      <HugeiconsIcon icon={Loading03Icon} className='size-4 animate-spin' />
    )
  } else if (isEnabled) {
    statusIcon = <HugeiconsIcon icon={PowerOffIcon} className='size-4' />
  }

  const menuItems = (
    <>
      <DropdownMenuItem
        onClick={() => {
          setCurrentRow(apiKey)
          setOpen('access-policy')
        }}
      >
        {t('Assigned access')}
      </DropdownMenuItem>
      <DropdownMenuItem
        onClick={() => {
          setCurrentRow(apiKey)
          setOpen('model-scope')
        }}
      >
        {t('Model scope')}
        <DropdownMenuShortcut>
          <HugeiconsIcon icon={FilterHorizontalIcon} />
        </DropdownMenuShortcut>
      </DropdownMenuItem>
      {props.onCredentialMaintenance && (
        <DropdownMenuItem onClick={props.onCredentialMaintenance}>
          {t('Credential maintenance')}
          <DropdownMenuShortcut>
            <HugeiconsIcon icon={SecurityCheckIcon} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
      )}
      <DropdownMenuItem
        onClick={() => {
          setCurrentRow(apiKey)
          setOpen('subscription-share')
        }}
      >
        {t('Subscription shares')}
      </DropdownMenuItem>
      <DropdownMenuItem
        onClick={async () => {
          const realKey = getCachedRealKey()
          if (!realKey) return
          const ok = await copyToClipboard(realKey)
          if (ok && isCurrentSession()) toast.success(t('Copied'))
        }}
      >
        {t('Copy Key')}
        <DropdownMenuShortcut>
          <HugeiconsIcon icon={Copy01Icon} />
        </DropdownMenuShortcut>
      </DropdownMenuItem>
      <DropdownMenuItem
        onClick={async () => {
          const realKey = getCachedRealKey()
          if (!realKey) return
          const connStr = encodeChannelConnectionInfo(
            realKey,
            getServerAddress()
          )
          const ok = await copyToClipboard(connStr)
          if (ok && isCurrentSession()) toast.success(t('Copied'))
        }}
      >
        {t('Copy Connection Info')}
        <DropdownMenuShortcut>
          <HugeiconsIcon icon={Link01Icon} />
        </DropdownMenuShortcut>
      </DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem
        onClick={async () => {
          if (!isCurrentSession()) return
          const realKey = await resolveRealKey(apiKey.id)
          if (!realKey || !isCurrentSession()) return
          setResolvedKey(realKey)
          setCurrentRow(apiKey)
          setOpen('cc-switch')
        }}
      >
        {t('CC Switch')}
        <DropdownMenuShortcut>
          <HugeiconsIcon icon={ArrowDataTransferHorizontalIcon} />
        </DropdownMenuShortcut>
      </DropdownMenuItem>
      {hasChatPresets && (
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>{t('Chat')}</DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            {chatPresets.map((preset) => (
              <DropdownMenuItem
                key={preset.id}
                onClick={() => handleOpenChatPreset(preset)}
              >
                {preset.name}
                {preset.type !== 'web' && (
                  <DropdownMenuShortcut>
                    <HugeiconsIcon icon={LinkSquare02Icon} />
                  </DropdownMenuShortcut>
                )}
              </DropdownMenuItem>
            ))}
          </DropdownMenuSubContent>
        </DropdownMenuSub>
      )}
      <DropdownMenuSeparator />
      <DropdownMenuItem
        onClick={() => {
          setCurrentRow(apiKey)
          setOpen('delete')
        }}
        className='text-destructive focus:text-destructive'
      >
        {t('Delete')}
        <DropdownMenuShortcut>
          <HugeiconsIcon icon={Delete02Icon} />
        </DropdownMenuShortcut>
      </DropdownMenuItem>
    </>
  )
  const menu = (
    <DataTableRowActionMenu
      ariaLabel={t('Open menu')}
      contentClassName='w-[200px]'
      modal={false}
      onOpenChange={handleMenuOpenChange}
    >
      {menuItems}
    </DataTableRowActionMenu>
  )
  if (props.menuOnly) return menu

  return (
    <div className='-ml-1.5 flex max-w-full min-w-0 flex-wrap items-center gap-1 md:min-w-min'>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={handleToggleStatus}
              disabled={isTogglingStatus}
              aria-label={toggleLabel}
              className={
                isEnabled
                  ? 'text-destructive hover:text-destructive'
                  : 'text-success hover:text-success'
              }
            />
          }
        >
          {statusIcon}
        </TooltipTrigger>
        <TooltipContent>{toggleLabel}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={() => {
                setCurrentRow(apiKey)
                setOpen('update')
              }}
              aria-label={t('Edit')}
            />
          }
        >
          <HugeiconsIcon icon={Edit02Icon} />
        </TooltipTrigger>
        <TooltipContent>{t('Edit')}</TooltipContent>
      </Tooltip>

      <Button
        variant='outline'
        size='sm'
        className='h-auto min-h-8 max-w-full min-w-0 px-2 whitespace-normal md:min-w-min md:flex-col md:gap-1 pointer-coarse:min-h-11'
        aria-label={t('API Key usage budgets')}
        onClick={() => {
          setCurrentRow(apiKey)
          setOpen('budget')
        }}
      >
        <HugeiconsIcon
          icon={DashboardSpeed01Icon}
          className='size-4 shrink-0'
          aria-hidden='true'
        />
        <span className='max-w-full min-w-0 break-words whitespace-normal md:min-w-min md:break-normal'>
          {t('Usage budgets')}
        </span>
      </Button>

      {viewRequests && (
        <Button
          variant='outline'
          size='sm'
          className='min-h-9 max-w-full whitespace-normal pointer-coarse:min-h-11'
          autoFocus={requestFocusId === apiKey.id}
          onClick={() => {
            if (isCurrentSession()) viewRequests(apiKey)
          }}
        >
          {t('View Key requests')}
        </Button>
      )}

      {menu}
    </div>
  )
}
