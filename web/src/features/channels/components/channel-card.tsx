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
import { flexRender, type Row } from '@tanstack/react-table'
import { memo } from 'react'
import { useTranslation } from 'react-i18next'

import { SummaryDisclosure } from '@/components/summary-disclosure'
import { isTagAggregateRow } from '../lib'
import type { ChannelInventoryProfile } from '../lib/channel-inventory-profile'
import type { Channel } from '../types'
import { ChannelRowActionsLayoutContext } from './channel-row-actions-context'
import { useChannels } from './channels-provider'

function ChannelCardComponent(props: {
  row: Row<Channel>
  isSelected: boolean
  profile?: ChannelInventoryProfile
}) {
  const { t } = useTranslation()
  const { sensitiveVisible } = useChannels()
  const profile = props.profile ?? 'custom'
  const isTag = isTagAggregateRow(props.row.original)
  const cells = props.row.getAllCells()
  const renderCell = (id: string) => {
    const cell = cells.find((item) => item.column.id === id)
    return cell?.column.columnDef.cell ? flexRender(cell.column.columnDef.cell, cell.getContext()) : null
  }
  const taskFields = profile === 'routing' ? ['priority', 'weight', 'group', 'models', 'tag'] : ['balance', 'response_time']
  const primary = profile === 'custom' ? taskFields.filter((id) => props.row.getVisibleCells().some((cell) => cell.column.id === id)) : taskFields
  const extra = ['id', 'priority', 'weight', 'group', 'models', 'tag', 'balance', 'response_time', 'test_time'].filter((id) => !(isTag && id === 'id') && !primary.includes(id) && !(profile === 'monitoring' && id === 'test_time'))
  const labels: Record<string, string> = { id: t('ID'), priority: t('Priority'), weight: t('Weight'), group: t('Groups'), models: t('Models'), tag: t('Tag'), balance: t('Used / Remaining'), response_time: profile === 'monitoring' ? t('Last saved test') : t('Response'), test_time: t('Last Tested') }
  return (
    <ChannelRowActionsLayoutContext.Provider value='card'>
      <article data-state={props.isSelected ? 'selected' : undefined} className='flex min-w-0 flex-col gap-3'>
        <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'>
          <div className='flex min-w-0 items-center gap-2'>{!isTag && renderCell('select')}{renderCell('type')}</div>
          {renderCell('status')}
        </div>
        <div className='min-w-0'>{renderCell('name')}</div>
        {profile === 'custom' && !isTag && <p className='text-muted-foreground font-mono text-xs'>#{sensitiveVisible ? props.row.original.id : '••••'}</p>}
        <dl className='grid min-w-0 grid-cols-1 gap-3 min-[360px]:grid-cols-2'>
          {primary.map((id) => <div key={id} className='min-w-0'><dt className='text-muted-foreground text-xs'>{labels[id]}</dt><dd className='min-w-0 text-sm'>{renderCell(id)}</dd></div>)}
        </dl>
        {renderCell('actions')}
        <SummaryDisclosure summary={t('More channel fields')}>
          <dl className='grid min-w-0 grid-cols-1 gap-3 min-[360px]:grid-cols-2'>
            {extra.map((id) => <div key={id} className='min-w-0'><dt className='text-muted-foreground text-xs'>{labels[id]}</dt><dd className='min-w-0 text-sm'>{renderCell(id)}</dd></div>)}
          </dl>
        </SummaryDisclosure>
      </article>
    </ChannelRowActionsLayoutContext.Provider>
  )
}

export const ChannelCard = memo(ChannelCardComponent)
