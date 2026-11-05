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
import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTableRowActionMenu } from '@/components/data-table/core/row-action-menu'
import { Button } from '@/components/ui/button'
import {
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuShortcut,
} from '@/components/ui/dropdown-menu'
import { useChatPresets } from '@/features/chat/hooks/use-chat-presets'
import { resolveChatUrl, type ChatPreset } from '@/features/chat/lib/chat-links'
import { sendToFluent } from '@/features/chat/lib/send-to-fluent'
import { encodeChannelConnectionInfo } from '@/lib/channel-connection-info'
import { copyToClipboard } from '@/lib/copy-to-clipboard'

import { updateApiKeyStatus } from '../api'
import { API_KEY_STATUS, ERROR_MESSAGES, SUCCESS_MESSAGES } from '../constants'
import { useKeyMaintenanceSession } from '../hooks/use-key-maintenance-session'
import { apiKeySchema, type ApiKey } from '../types'
import { keyClientAddresses } from '../lib/client-configuration'
import { useApiKeys } from './api-keys-provider'

type DataTableRowActionsProps<TData> =
  | {
      row: Row<TData>
      apiKey?: never
      onCredentialMaintenance?: () => void
      /** Only the "more" menu, for narrow rows and the inspector header. */
      menuOnly?: boolean
      /** The detail header already exposes edit, status and copy actions. */
      primaryActionsVisible?: boolean
      hideClientImport?: boolean
    }
  | {
      apiKey: ApiKey
      row?: never
      onCredentialMaintenance?: () => void
      menuOnly?: boolean
      primaryActionsVisible?: boolean
      hideClientImport?: boolean
    }

export function DataTableRowActions<TData = ApiKey>(
  props: DataTableRowActionsProps<TData>
) {
  const { t } = useTranslation()
  const isCurrentSession = useKeyMaintenanceSession()
  const apiKey = apiKeySchema.parse(props.apiKey ?? props.row?.original)
  const {
    viewRequests,
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
  const clientAddress = useRef(keyClientAddresses(serverAddress, window.location.origin))
  clientAddress.current = keyClientAddresses(serverAddress, window.location.origin)
  const [isTogglingStatus, setIsTogglingStatus] = useState(false)
  const resolvedRealKey = resolvedKeys[apiKey.id]
  const isRealKeyLoading = Boolean(loadingKeys[apiKey.id])

  const hasChatPresets = chatPresets.length > 0
  const toggleLabel = isEnabled ? t('Pause Key') : t('Resume Key')

  const copy = async (connectionInfo = false) => {
    if (isRealKeyLoading || !isCurrentSession()) return
    const realKey = resolvedRealKey || (await resolveRealKey(apiKey.id))
    if (!realKey || !isCurrentSession()) return
    const value = connectionInfo
      ? encodeChannelConnectionInfo(realKey, clientAddress.current.root)
      : realKey
    const copied = await copyToClipboard(value)
    if (!isCurrentSession()) return
    if (copied) toast.success(t('Copied successfully'))
    else toast.error(t('Failed to copy'))
  }

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
    if (isTogglingStatus || !isCurrentSession()) return
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

  let statusIcon = (
    <HugeiconsIcon icon={PowerIcon} className='size-4' aria-hidden='true' />
  )
  if (isTogglingStatus) {
    statusIcon = (
      <HugeiconsIcon
        icon={Loading03Icon}
        className='size-4 animate-spin'
        aria-hidden='true'
      />
    )
  } else if (isEnabled) {
    statusIcon = (
      <HugeiconsIcon icon={PowerOffIcon} className='size-4' aria-hidden='true' />
    )
  }

  const menuItems = (
    <DropdownMenuGroup>
      {props.menuOnly && !props.primaryActionsVisible && (
        <>
          {viewRequests && (
            <DropdownMenuItem
              onClick={() => {
                if (isCurrentSession()) viewRequests(apiKey)
              }}
            >
              {t('View Key requests')}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem
            onClick={() => {
              setCurrentRow(apiKey)
              setOpen('update')
            }}
          >
            {t('Edit')}
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isTogglingStatus}
            onClick={() => void handleToggleStatus()}
          >
            {toggleLabel}
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={isRealKeyLoading}
            onClick={() => void copy()}
          >
            {t('Copy Key')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
        </>
      )}
      <DropdownMenuItem
        onClick={() => {
          setCurrentRow(apiKey)
          setOpen('budget')
        }}
      >
        {t('Usage budgets')}
        <DropdownMenuShortcut>
          <HugeiconsIcon icon={DashboardSpeed01Icon} />
        </DropdownMenuShortcut>
      </DropdownMenuItem>
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
        disabled={isRealKeyLoading}
        onClick={() => void copy(true)}
      >
        {t('Copy Connection Info')}
        <DropdownMenuShortcut>
          <HugeiconsIcon icon={Link01Icon} />
        </DropdownMenuShortcut>
      </DropdownMenuItem>
      {(!props.hideClientImport || hasChatPresets) && <DropdownMenuSeparator />}
      {!props.hideClientImport && <DropdownMenuItem
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
      </DropdownMenuItem>}
      {hasChatPresets && (
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>{t('Chat')}</DropdownMenuSubTrigger>
          <DropdownMenuSubContent className='max-w-[calc(100vw-2rem)] [&_[role=menuitem]]:whitespace-normal max-sm:[&_[role=menuitem]]:min-h-11 pointer-coarse:[&_[role=menuitem]]:min-h-11'>
            <DropdownMenuGroup>
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
            </DropdownMenuGroup>
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
    </DropdownMenuGroup>
  )
  const menu = (
    <DataTableRowActionMenu
      ariaLabel={t('Open menu')}
      contentClassName='w-60 max-w-[calc(100vw-2rem)] [&_[role=menuitem]]:whitespace-normal max-sm:[&_[role=menuitem]]:min-h-11 pointer-coarse:[&_[role=menuitem]]:min-h-11'
      modal={false}
    >
      {menuItems}
    </DataTableRowActionMenu>
  )
  if (props.menuOnly) {
    return (
      <span
        data-row-actions=''
        onClick={(event) => event.stopPropagation()}
        className='flex max-sm:[&_button]:min-h-11 max-sm:[&_button]:min-w-11 pointer-coarse:[&_button]:min-h-11 pointer-coarse:[&_button]:min-w-11'
      >
        {menu}
      </span>
    )
  }

  return (
    <div
      data-row-actions=''
      onClick={(event) => event.stopPropagation()}
      className='flex max-w-full min-w-0 flex-wrap items-center gap-2 max-sm:[&_button]:min-h-11 max-sm:[&_button]:min-w-11 pointer-coarse:[&_button]:min-h-11 pointer-coarse:[&_button]:min-w-11'
    >
      <Button
        variant='ghost'
        size='sm'
        onClick={handleToggleStatus}
        disabled={isTogglingStatus}
        aria-busy={isTogglingStatus}
        className='h-auto min-h-8 whitespace-normal'
      >
        {statusIcon}
        {toggleLabel}
      </Button>
      <Button
        variant='ghost'
        size='sm'
        className='h-auto min-h-8 whitespace-normal'
        onClick={() => {
          setCurrentRow(apiKey)
          setOpen('update')
        }}
      >
        <HugeiconsIcon
          icon={Edit02Icon}
          data-icon='inline-start'
          aria-hidden='true'
        />
        {t('Edit')}
      </Button>
      <Button
        variant='ghost'
        size='sm'
        disabled={isRealKeyLoading}
        aria-busy={isRealKeyLoading}
        className='h-auto min-h-8 whitespace-normal'
        onClick={() => void copy()}
      >
        <HugeiconsIcon
          icon={isRealKeyLoading ? Loading03Icon : Copy01Icon}
          className={isRealKeyLoading ? 'animate-spin' : undefined}
          data-icon='inline-start'
          aria-hidden='true'
        />
        {t('Copy Key')}
      </Button>

      {viewRequests && (
        <Button
          variant='outline'
          size='sm'
          className='h-auto min-h-9 max-w-full wrap-break-word break-keep whitespace-normal pointer-coarse:min-h-11'
          data-key-requests={apiKey.id}
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
