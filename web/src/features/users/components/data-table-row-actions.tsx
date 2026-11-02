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
import type { Row } from '@tanstack/react-table'
import {
  Pencil,
  Trash2,
  Power,
  PowerOff,
  ArrowUp,
  ArrowDown,
  KeyRound,
  ShieldAlert,
  Link2,
  CreditCard,
} from 'lucide-react'
import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTableRowActionMenu } from '@/components/data-table/core/row-action-menu'
import { Button } from '@/components/ui/button'
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
} from '@/components/ui/dropdown-menu'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { UserSubscriptionsDialog } from '@/features/subscriptions/components/dialogs/user-subscriptions-dialog'
import { useAuthStore } from '@/stores/auth-store'

import { manageUser, resetUserPasskey, resetUserTwoFA } from '../api'
import {
  USER_STATUS,
  USER_ROLE,
  ERROR_MESSAGES,
  isUserDeleted,
} from '../constants'
import { getUserActionMessage } from '../lib'
import type { User, ManageUserAction } from '../types'
import { UserBindingDialog } from './dialogs/user-binding-dialog'
import { useUsers } from './users-provider'
import { useUserTask } from '../hooks/use-user-task'

interface DataTableRowActionsProps {
  row: Row<User>
}

export function DataTableRowActions({ row }: DataTableRowActionsProps) {
  const { t } = useTranslation()
  const user = row.original
  const { setOpen, setCurrentRow, triggerRefresh } = useUsers()
  const [resetPasskeyOpen, setResetPasskeyOpen] = useState(false)
  const [resetTwoFAOpen, setResetTwoFAOpen] = useState(false)
  const [bindingDialogOpen, setBindingDialogOpen] = useState(false)
  const [subscriptionsDialogOpen, setSubscriptionsDialogOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)
  const task = useUserTask(true, user.id)
  const actions = useRef<HTMLDivElement>(null)
  const returnFocus = useCallback(() => actions.current?.querySelector<HTMLElement>('button[aria-haspopup]') ?? null, [])
  const canEditAssignedAccess = useAuthStore(
    (state) => (state.auth.user?.role ?? 0) >= 10
  )
  const canEditUsagePolicy = useAuthStore(
    (state) => state.auth.user?.role === 100
  )

  const handleEdit = () => {
    setCurrentRow(user)
    setOpen('update')
  }

  const handleDelete = () => {
    setCurrentRow(user)
    setOpen('delete')
  }

  const handleManage = async (action: Exclude<ManageUserAction, 'delete'>) => {
    const operation = task.begin()
    if (!operation) return
    setBusy(true)
    try {
      const result = await manageUser(user.id, action)
      if (!operation.isCurrent()) return
      if (result.success) {
        toast.success(t(getUserActionMessage(action)))
        triggerRefresh()
      } else {
        toast.error(
          result.message || t('Failed to {{action}} user', { action })
        )
      }
    } catch {
      if (!operation.isCurrent()) return
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      if (operation.isCurrent()) setBusy(false)
      operation.finish()
    }
  }

  const handleResetPasskey = async () => {
    if (user.role === USER_ROLE.ROOT) return
    const operation = task.begin()
    if (!operation) return
    setBusy(true)
    setActionError(null)
    try {
      const result = await resetUserPasskey(user.id)
      if (!operation.isCurrent()) return
      if (result.success) {
        toast.success(t('Passkey reset successfully'))
        triggerRefresh()
        setResetPasskeyOpen(false)
      } else {
        setActionError(result.message || t('Failed to reset Passkey'))
        toast.error(result.message || t('Failed to reset Passkey'))
      }
    } catch {
      if (!operation.isCurrent()) return
      setActionError(t(ERROR_MESSAGES.UNEXPECTED))
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      if (operation.isCurrent()) setBusy(false)
      operation.finish()
    }
  }

  const handleResetTwoFA = async () => {
    if (user.role === USER_ROLE.ROOT) return
    const operation = task.begin()
    if (!operation) return
    setBusy(true)
    setActionError(null)
    try {
      const result = await resetUserTwoFA(user.id)
      if (!operation.isCurrent()) return
      if (result.success) {
        toast.success(t('Two-factor authentication reset'))
        triggerRefresh()
        setResetTwoFAOpen(false)
      } else {
        setActionError(result.message || t('Failed to reset 2FA'))
        toast.error(result.message || t('Failed to reset 2FA'))
      }
    } catch {
      if (!operation.isCurrent()) return
      setActionError(t(ERROR_MESSAGES.UNEXPECTED))
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      if (operation.isCurrent()) setBusy(false)
      operation.finish()
    }
  }

  const isDisabled = user.status === USER_STATUS.DISABLED
  const isAdmin = user.role >= USER_ROLE.ADMIN
  const isRoot = user.role === USER_ROLE.ROOT

  if (isUserDeleted(user)) {
    return null
  }

  return (
    <div ref={actions} data-user-actions={String(user.id)} aria-busy={busy} className='flex items-center gap-1 [&_[data-myapi-button]]:min-h-11 [&_[data-slot=select-trigger]]:min-h-11 [&_[role=combobox]]:min-h-11 [&_[data-myapi-button]]:min-w-11'>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={handleEdit}
              disabled={busy}
              data-user-edit={String(user.id)}
              aria-label={t('Edit')}
            />
          }
        >
          <Pencil />
        </TooltipTrigger>
        <TooltipContent>{t('Edit')}</TooltipContent>
      </Tooltip>

      <DataTableRowActionMenu
        ariaLabel={t('Open menu')}
        contentClassName='w-48'
      >
        {canEditAssignedAccess && (
          <DropdownMenuItem
            onSelect={(event) => {
              event.preventDefault()
              setCurrentRow(user)
              setOpen('access-policy')
            }}
          >
            {t('Assigned access')}
          </DropdownMenuItem>
        )}
        {canEditUsagePolicy && (
          <DropdownMenuItem
            onSelect={(event) => {
              event.preventDefault()
              setCurrentRow(user)
              setOpen('usage-policy')
            }}
          >
            {t('User usage policy')}
          </DropdownMenuItem>
        )}
        {isDisabled ? (
          <DropdownMenuItem onClick={() => handleManage('enable')}>
            {t('Enable')}
            <DropdownMenuShortcut>
              <Power size={16} />
            </DropdownMenuShortcut>
          </DropdownMenuItem>
        ) : (
          <DropdownMenuItem
            onClick={() => handleManage('disable')}
            disabled={isRoot}
          >
            {t('Disable')}
            <DropdownMenuShortcut>
              <PowerOff size={16} />
            </DropdownMenuShortcut>
          </DropdownMenuItem>
        )}

        {isAdmin && !isRoot && (
          <DropdownMenuItem onClick={() => handleManage('demote')}>
            {t('Demote')}
            <DropdownMenuShortcut>
              <ArrowDown size={16} />
            </DropdownMenuShortcut>
          </DropdownMenuItem>
        )}

        {!isAdmin && (
          <DropdownMenuItem onClick={() => handleManage('promote')}>
            {t('Promote')}
            <DropdownMenuShortcut>
              <ArrowUp size={16} />
            </DropdownMenuShortcut>
          </DropdownMenuItem>
        )}

        <DropdownMenuItem
          onSelect={(event) => {
            event.preventDefault()
            setBindingDialogOpen(true)
          }}
        >
          {t('Manage Bindings')}
          <DropdownMenuShortcut>
            <Link2 size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>

        <DropdownMenuItem
          onSelect={(event) => {
            event.preventDefault()
            setSubscriptionsDialogOpen(true)
          }}
        >
          {t('Manage Subscriptions')}
          <DropdownMenuShortcut>
            <CreditCard size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>

        <DropdownMenuSeparator />

        <DropdownMenuItem
          onSelect={(event) => {
            event.preventDefault()
            setActionError(null)
            setResetPasskeyOpen(true)
          }}
          disabled={isRoot}
        >
          {t('Reset Passkey')}
          <DropdownMenuShortcut>
            <KeyRound size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>

        <DropdownMenuItem
          onSelect={(event) => {
            event.preventDefault()
            setActionError(null)
            setResetTwoFAOpen(true)
          }}
          disabled={isRoot}
        >
          {t('Reset 2FA')}
          <DropdownMenuShortcut>
            <ShieldAlert size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>

        <DropdownMenuSeparator />

        <DropdownMenuItem
          onClick={handleDelete}
          className='text-destructive focus:text-destructive'
          disabled={isRoot}
        >
          {t('Delete')}
          <DropdownMenuShortcut>
            <Trash2 size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
      </DataTableRowActionMenu>

      <ConfirmDialog
        open={resetPasskeyOpen}
        onOpenChange={(next) => { if (!task.isPending()) setResetPasskeyOpen(next) }}
        returnFocus={returnFocus}
        title={t('Reset Passkey')}
        desc={t(
          'Reset Passkey for {{username}}? The user will need to register a new Passkey before using passwordless login.',
          { username: user.username }
        )}
        confirmText={t('Reset Passkey')}
        handleConfirm={handleResetPasskey}
        isLoading={busy}
      >{actionError ? <div role='alert' className='text-destructive text-sm break-words'>{actionError}</div> : null}</ConfirmDialog>

      <ConfirmDialog
        open={resetTwoFAOpen}
        onOpenChange={(next) => { if (!task.isPending()) setResetTwoFAOpen(next) }}
        returnFocus={returnFocus}
        title={t('Reset Two-Factor Authentication')}
        desc={t(
          'Reset 2FA for {{username}}? The user must set up 2FA again to continue using it.',
          { username: user.username }
        )}
        confirmText={t('Reset 2FA')}
        handleConfirm={handleResetTwoFA}
        isLoading={busy}
      >{actionError ? <div role='alert' className='text-destructive text-sm break-words'>{actionError}</div> : null}</ConfirmDialog>

      <UserBindingDialog
        open={bindingDialogOpen}
        onOpenChange={setBindingDialogOpen}
        userId={user.id}
        onUnbindSuccess={triggerRefresh}
        returnFocus={returnFocus}
      />

      <UserSubscriptionsDialog
        open={subscriptionsDialogOpen}
        onOpenChange={setSubscriptionsDialogOpen}
        user={{ id: user.id, username: user.username }}
        onSuccess={triggerRefresh}
        returnFocus={returnFocus}
      />
    </div>
  )
}
