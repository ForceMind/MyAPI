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

import { ChatDialog, type ChatEntryData } from './chat-dialog'

type ChatSettingsVisualEditorProps = {
  value: string
  onChange: (value: string) => void
  getCurrentValue?: () => string
  onRequestJSON?: () => void
}

type ChatEntry = ChatEntryData

export function ChatSettingsVisualEditor({
  value,
  onChange,
  getCurrentValue,
  onRequestJSON,
}: ChatSettingsVisualEditorProps) {
  const { t } = useTranslation()
  const [searchText, setSearchText] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editData, setEditData] = useState<ChatEntry | null>(null)

  const [openedSource, setOpenedSource] = useState(value)
  const parsedState = useMemo(() => {
    try {
      const parsed: unknown = JSON.parse(value || '[]')
      if (!Array.isArray(parsed)) return null
      const entries: ChatEntry[] = []
      const names = new Set<string>()
      for (const item of parsed) {
        if (typeof item !== 'object' || item === null || Array.isArray(item)) return null
        const fields = Object.entries(item)
        if (fields.length !== 1) return null
        const [name, url] = fields[0]
        if (typeof url !== 'string' || names.has(name)) return null
        names.add(name)
        entries.push({ name, url })
      }
      return { source: parsed, entries }
    } catch { return null }
  }, [value])
  const chats = parsedState?.entries
  const blocked = parsedState === null || (dialogOpen && openedSource !== value)

  const filteredChats = useMemo(() => {
    if (!chats) return []
    if (!searchText) return chats
    const lowerSearch = searchText.toLowerCase()
    return chats.filter(
      (chat) =>
        chat.name.toLowerCase().includes(lowerSearch) ||
        chat.url.toLowerCase().includes(lowerSearch)
    )
  }, [chats, searchText])

  const handleSave = (data: ChatEntryData) => {
    if (!parsedState || openedSource !== (getCurrentValue?.() ?? value)) return false
    const chatsArray = parsedState.source

    let updatedArray = [...chatsArray]

    if (editData) {
      updatedArray = updatedArray.filter((item) => {
        if (typeof item === 'object' && item !== null && !Array.isArray(item)) {
          return !Object.keys(item).includes(editData.name)
        }
        return true
      })
    }

    updatedArray.push({ [data.name]: data.url })

    onChange(JSON.stringify(updatedArray, null, 2))
  }

  const handleDelete = (name: string) => {
    if (!parsedState || (getCurrentValue?.() ?? value) !== value) return
    const chatsArray = parsedState.source

    const updatedArray = chatsArray.filter((item) => {
      if (typeof item === 'object' && item !== null && !Array.isArray(item)) {
        return !Object.keys(item).includes(name)
      }
      return true
    })

    onChange(JSON.stringify(updatedArray, null, 2))
  }

  const handleEdit = (chat: ChatEntry) => {
    setOpenedSource(value)
    setEditData(chat)
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
            aria-label={t('Search chat presets...')}
            placeholder={t('Search chat presets...')}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            className='pl-9'
          />
        </div>
        <Button onClick={handleAdd}>
          <Plus className='mr-2 h-4 w-4' />
          {t('Add chat preset')}
        </Button>
      </div>

      <StaticDataTable
        data={filteredChats}
        getRowKey={(chat) => chat.name}
        emptyContent={
          searchText
            ? t('No chat presets match your search')
            : t(
                'No chat presets configured. Click "Add chat preset" to get started.'
              )
        }
        columns={[
          {
            id: 'name',
            header: t('Chat Client Name'),
            cellClassName: 'font-medium whitespace-normal break-all',
            cell: (chat) => chat.name,
          },
          {
            id: 'url',
            header: t('URL'),
            cellClassName: 'whitespace-normal break-all font-mono text-sm',
            cell: (chat) => chat.url,
          },
          {
            id: 'actions',
            header: t('Actions'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (chat) => (
              <StaticRowActions
                editLabel={t('Edit')}
                deleteLabel={t('Delete')}
                menuLabel={t('Open menu')}
                onEdit={() => handleEdit(chat)}
                onDelete={() => handleDelete(chat.name)}
              />
            ),
          },
        ]}
      />

      </>}
      <ChatDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSave={handleSave}
        editData={editData}
        blocked={blocked}
      />
    </div>
  )
}
