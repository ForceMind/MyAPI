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
import { Edit02Icon, PowerIcon, PowerOffIcon, Refresh01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import type { PlanRecord } from '../types'
import { useSubscriptions } from './subscriptions-provider'

interface DataTableRowActionsProps {
  row: Row<PlanRecord>
}

export function DataTableRowActions({ row }: DataTableRowActionsProps) {
  const { t } = useTranslation()
  const { setOpen, setCurrentRow, complianceConfirmed } = useSubscriptions()
  const isEnabled = row.original.plan.enabled
  const toggleLabel = isEnabled ? t('Disable') : t('Enable')

  const handleEdit = () => {
    setCurrentRow(row.original)
    setOpen('update')
  }

  const handleToggleStatus = () => {
    setCurrentRow(row.original)
    setOpen('toggle-status')
  }

  const handleResetSubscriptions = () => {
    setCurrentRow(row.original)
    setOpen('reset-subscriptions')
  }

  return (
    <div data-plan-actions={String(row.original.plan.id)} className='flex flex-wrap items-center gap-1 [&_[data-myapi-button]]:min-h-11 [&_[data-slot=select-trigger]]:min-h-11 [&_[role=combobox]]:min-h-11 [&_[data-myapi-button]]:min-w-11'>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              disabled={!complianceConfirmed}
              onClick={handleEdit}
              aria-label={t('Edit')}
                data-plan-action='edit'
            />
          }
        >
          <HugeiconsIcon icon={Edit02Icon} aria-hidden='true' />
        </TooltipTrigger>
        <TooltipContent>{t('Edit')}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              disabled={!complianceConfirmed}
              onClick={handleResetSubscriptions}
              aria-label={t('Reset subscription quota')}
                data-plan-action='reset'
            />
          }
        >
          <HugeiconsIcon icon={Refresh01Icon} aria-hidden='true' />
        </TooltipTrigger>
        <TooltipContent>{t('Reset subscription quota')}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              disabled={!complianceConfirmed}
              onClick={handleToggleStatus}
              aria-label={toggleLabel}
              data-plan-action='status'
              className={
                isEnabled
                  ? 'text-destructive hover:text-destructive'
                  : 'text-success hover:text-success'
              }
            />
          }
        >
          <HugeiconsIcon icon={isEnabled ? PowerOffIcon : PowerIcon} aria-hidden='true' />
        </TooltipTrigger>
        <TooltipContent>{toggleLabel}</TooltipContent>
      </Tooltip>
    </div>
  )
}
