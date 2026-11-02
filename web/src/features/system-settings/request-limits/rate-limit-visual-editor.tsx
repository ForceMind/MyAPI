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
import { Plus, Search } from 'lucide-react'
import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import { RateLimitDialog, type RateLimitEntryData } from './rate-limit-dialog'

type RateLimitVisualEditorProps = {
  value: string
  onChange: (value: string) => void
  getCurrentValue?: () => string | undefined
  onRequestJSON?: () => void
}

type RateLimitEntry = RateLimitEntryData

export function RateLimitVisualEditor({
  value,
  onChange,
  getCurrentValue,
  onRequestJSON,
}: RateLimitVisualEditorProps) {
  const { t } = useTranslation()
  const [searchText, setSearchText] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editData, setEditData] = useState<RateLimitEntry | null>(null)

  const [openedSource, setOpenedSource] = useState(value)
  const parsedState = useMemo(() => {
    try {
      const parsed: unknown = JSON.parse(value || '{}')
      if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) return null
      const entries: RateLimitEntry[] = []
      for (const [groupName, limits] of Object.entries(parsed)) {
        if (!Array.isArray(limits) || limits.length !== 2 || !limits.every((limit) => typeof limit === 'number' && Number.isFinite(limit))) return null
        entries.push({ groupName, maxRequests: limits[0], maxSuccess: limits[1] })
      }
      return { source: parsed as Record<string, unknown>, entries }
    } catch { return null }
  }, [value])
  const rateLimits = parsedState?.entries
  const blocked = parsedState === null || (dialogOpen && openedSource !== value)

  const filteredRateLimits = useMemo(() => {
    if (!rateLimits) return []
    if (!searchText) return rateLimits
    const lowerSearch = searchText.toLowerCase()
    return rateLimits.filter((limit) =>
      limit.groupName.toLowerCase().includes(lowerSearch)
    )
  }, [rateLimits, searchText])

  const handleSave = (data: RateLimitEntryData) => {
    const currentValue = getCurrentValue ? getCurrentValue() : value
    if (!parsedState || openedSource !== currentValue) return false
    const parsed = JSON.parse(value || '{}') as Record<string, unknown>

    if (editData && editData.groupName !== data.groupName) {
      delete parsed[editData.groupName]
    }

    parsed[data.groupName] = [data.maxRequests, data.maxSuccess]

    onChange(JSON.stringify(parsed, null, 2))
  }

  const handleDelete = (groupName: string) => {
    const currentValue = getCurrentValue ? getCurrentValue() : value
    if (!parsedState || currentValue !== value) return
    const parsed = JSON.parse(value || '{}') as Record<string, unknown>

    delete parsed[groupName]

    onChange(JSON.stringify(parsed, null, 2))
  }

  const handleEdit = (limit: RateLimitEntry) => {
    setOpenedSource(value)
    setEditData(limit)
    setDialogOpen(true)
  }

  const handleAdd = () => {
    setOpenedSource(value)
    setEditData(null)
    setDialogOpen(true)
  }

  return (
    <div className='space-y-4 pointer-coarse:[&_[data-slot=input]]:min-h-11'>
      {!parsedState && <div role='alert' className='space-y-3 rounded-lg border p-4 text-sm'>
        <p>{t('This configuration cannot be edited safely as a list. The original JSON has been preserved.')}</p>
        <pre aria-label={t('Original JSON')} className='whitespace-pre-wrap break-all'>{value}</pre>
        {onRequestJSON && <Button type='button' variant='outline' onClick={onRequestJSON}>{t('Repair JSON')}</Button>}
      </div>}
      {parsedState && <>
      <div className='flex items-center gap-4'>
        <div className='relative flex-1'>
          <Search className='text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4' />
          <Input
            aria-label={t('Search group names...')}
            placeholder={t('Search group names...')}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            className='pl-9'
          />
        </div>
        <Button onClick={handleAdd}>
          <Plus className='mr-2 h-4 w-4' />
          {t('Add group')}
        </Button>
      </div>

      <StaticDataTable
        data={filteredRateLimits}
        getRowKey={(limit) => limit.groupName}
        emptyContent={
          searchText
            ? t('No groups match your search')
            : t(
                'No group-based rate limits configured. Click "Add group" to get started.'
              )
        }
        columns={[
          {
            id: 'group',
            header: t('Group Name'),
            cellClassName: 'font-medium whitespace-normal break-all',
            cell: (limit) => limit.groupName,
          },
          {
            id: 'max-requests',
            header: t('Max Requests (incl. failures)'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (limit) => (
              <span className='font-mono'>
                {limit.maxRequests === 0
                  ? t('Unlimited')
                  : limit.maxRequests.toLocaleString()}
              </span>
            ),
          },
          {
            id: 'max-success',
            header: t('Max Success'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (limit) => (
              <span className='font-mono'>
                {limit.maxSuccess.toLocaleString()}
              </span>
            ),
          },
          {
            id: 'actions',
            header: t('Actions'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (limit) => (
              <StaticRowActions
                editLabel={t('Edit')}
                deleteLabel={t('Delete')}
                menuLabel={t('Open menu')}
                onEdit={() => handleEdit(limit)}
                onDelete={() => handleDelete(limit.groupName)}
              />
            ),
          },
        ]}
      />

      </>}
      <RateLimitDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSave={handleSave}
        editData={editData}
        blocked={blocked}
      />
    </div>
  )
}
