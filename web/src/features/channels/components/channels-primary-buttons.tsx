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
import {
  Plus,
  MoreHorizontal,
  Settings2,
  Trash2,
  Tags,
  TestTube,
  DollarSign,
  ListChecks,
  SortAsc,
  RefreshCw,
  ArrowUpFromLine,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuCheckboxItem,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuGroup,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import {
  handleDeleteAllDisabled,
  handleFixAbilities,
  handleTestAllChannels,
  handleUpdateAllBalances,
} from '../lib'
import { useChannels } from './channels-provider'

export function ChannelsPrimaryButtons() {
  const { t } = useTranslation()
  const {
    setOpen,
    setCurrentRow,
    enableTagMode,
    setEnableTagMode,
    idSort,
    setIdSort,
    batchMode,
    setBatchMode,
    upstream,
  } = useChannels()
  const queryClient = useQueryClient()
  const [showDeleteDialog, setShowDeleteDialog] = useState(false)
  const [showConsistencyDialog, setShowConsistencyDialog] = useState(false)
  const [isRepairingConsistency, setIsRepairingConsistency] = useState(false)
  const currentUser = useAuthStore((s) => s.auth.user)
  const canEditSensitive = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )

  const canOperate = hasPermission(currentUser, 'channel', 'operate')
  const canWrite = hasPermission(currentUser, 'channel', 'write')

  const handleTagModeToggle = (checked: boolean) => {
    localStorage.setItem('enable-tag-mode', String(checked))
    setEnableTagMode(checked)
  }

  const handleIdSortToggle = (checked: boolean) => {
    localStorage.setItem('channels-id-sort', String(checked))
    setIdSort(checked)
  }

  const handleBatchModeToggle = (checked: boolean) => {
    setBatchMode(checked)
  }

  return (
    <>
      <div className='flex min-w-0 flex-wrap items-center gap-2 [&_[data-myapi-button]]:min-h-11 [&_[data-myapi-button]]:min-w-11'>
        <Button type='button' onClick={() => {
          if (!canEditSensitive) return
          setCurrentRow(null)
          setOpen('create-channel')
        }} disabled={!canEditSensitive}>
          <Plus aria-hidden='true' />{t('Create Channel')}
        </Button>
        <Button type='button' variant={batchMode ? 'secondary' : 'outline'} aria-pressed={batchMode} onClick={() => handleBatchModeToggle(!batchMode)}>
          <ListChecks aria-hidden='true' />{batchMode ? t('Exit channel selection') : t('Select channels')}
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger render={<Button type='button' variant='outline' />}>
            <Settings2 aria-hidden='true' />{t('Display settings')}
          </DropdownMenuTrigger>
          <DropdownMenuContent align='end' className='w-64 [&_[role=menuitemcheckbox]]:min-h-11'>
            <DropdownMenuGroup>
            <DropdownMenuLabel>{t('Inventory presentation')}</DropdownMenuLabel>
            <DropdownMenuCheckboxItem checked={enableTagMode} onCheckedChange={handleTagModeToggle}>
              <Tags aria-hidden='true' />{t('Tag Mode')}
            </DropdownMenuCheckboxItem>
            <DropdownMenuCheckboxItem checked={idSort} onCheckedChange={handleIdSortToggle}>
              <SortAsc aria-hidden='true' />{t('Sort by ID')}
            </DropdownMenuCheckboxItem>
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
        <DropdownMenu>
          <DropdownMenuTrigger render={<Button type='button' variant='outline' />}>
            <MoreHorizontal aria-hidden='true' />{t('Global maintenance')}
          </DropdownMenuTrigger>
          <DropdownMenuContent align='end' className='w-72 [&_[role=menuitem]]:min-h-11'>
            <DropdownMenuGroup>
            <DropdownMenuLabel>{t('All channels')}</DropdownMenuLabel>
            <DropdownMenuItem disabled={!canOperate} onClick={() => handleTestAllChannels(queryClient)}>
              <TestTube aria-hidden='true' />{t('Test All Channels')}
            </DropdownMenuItem>
            <DropdownMenuItem disabled={!canOperate} onClick={() => handleUpdateAllBalances(queryClient)}>
              <DollarSign aria-hidden='true' />{t('Update All Balances')}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem disabled={!canOperate} onClick={() => upstream.detectAllUpdates()}>
              <RefreshCw aria-hidden='true' />{t('Detect All Upstream Updates')}
            </DropdownMenuItem>
            <DropdownMenuItem disabled={!canWrite} onClick={() => upstream.applyAllUpdates()}>
              <ArrowUpFromLine aria-hidden='true' />{t('Apply All Upstream Updates')}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            </DropdownMenuGroup>
            <DropdownMenuGroup>
            <DropdownMenuLabel>{t('Dangerous operations')}</DropdownMenuLabel>
            <DropdownMenuItem disabled={!canOperate} onSelect={(event) => { event.preventDefault(); setShowConsistencyDialog(true) }}>
              <Settings2 aria-hidden='true' />{t('Repair Channel Consistency')}
            </DropdownMenuItem>
            <DropdownMenuItem disabled={!canEditSensitive} className='text-destructive' onSelect={(event) => {
              event.preventDefault()
              if (canEditSensitive) setShowDeleteDialog(true)
            }}>
              <Trash2 aria-hidden='true' />{t('Delete All Disabled')}
            </DropdownMenuItem>
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
        {!canEditSensitive && <p className='text-muted-foreground text-xs'>{t('You do not have permission to create channels.')}</p>}
      </div>

      <ConfirmDialog
        open={showDeleteDialog}
        onOpenChange={setShowDeleteDialog}
        title={t('Delete All Disabled Channels?')}
        desc={t(
          'This will permanently delete all manually and automatically disabled channels. This action cannot be undone.'
        )}
        destructive
        handleConfirm={() => {
          if (!canEditSensitive) return
          handleDeleteAllDisabled(queryClient, (_count) => {
            // eslint-disable-next-line no-console
            console.log(`Deleted ${_count} channels`)
          })
          setShowDeleteDialog(false)
        }}
      />

      <ConfirmDialog
        open={showConsistencyDialog}
        onOpenChange={setShowConsistencyDialog}
        title={t('Repair channel consistency?')}
        desc={t(
          'This will rebuild the channel routing index from every channel configuration, including supported models, groups, priorities, and weights. Routing may be briefly incomplete while the rebuild is running. Continue?'
        )}
        confirmText={t('Repair')}
        isLoading={isRepairingConsistency}
        handleConfirm={async () => {
          setIsRepairingConsistency(true)
          try {
            await handleFixAbilities(queryClient, (_result) => {
              // eslint-disable-next-line no-console
              console.log('Repair channel consistency result:', _result)
            })
            setShowConsistencyDialog(false)
          } finally {
            setIsRepairingConsistency(false)
          }
        }}
      />
    </>
  )
}
