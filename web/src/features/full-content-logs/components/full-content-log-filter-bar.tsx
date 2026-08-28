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

import { RotateCcw, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import type { FullContentLogFilters, FullContentLogTokenOption } from '../types'

interface FullContentLogFilterBarProps {
  filters: FullContentLogFilters
  onChange: (filters: FullContentLogFilters) => void
  onApply: () => void
  onReset: () => void
  modelOptions: string[]
  tokenOptions: FullContentLogTokenOption[]
}

export function FullContentLogFilterBar(props: FullContentLogFilterBarProps) {
  const { t } = useTranslation()

  const updateField = (field: keyof FullContentLogFilters, value: string) => {
    props.onChange({ ...props.filters, [field]: value })
  }

  return (
    <form
      className='grid gap-3 rounded-lg border p-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6'
      onSubmit={(event) => {
        event.preventDefault()
        props.onApply()
      }}
    >
      <div className='space-y-1.5'>
        <Label htmlFor='full-log-model'>{t('Model')}</Label>
        <Input
          id='full-log-model'
          list='full-log-model-options'
          value={props.filters.model}
          placeholder={t('All models')}
          onChange={(event) => updateField('model', event.target.value)}
        />
        <datalist id='full-log-model-options'>
          {props.modelOptions.map((model) => (
            <option key={model} value={model} />
          ))}
        </datalist>
      </div>
      <div className='space-y-1.5'>
        <Label htmlFor='full-log-token'>{t('API Key name or ID')}</Label>
        <Input
          id='full-log-token'
          list='full-log-token-options'
          value={props.filters.token}
          placeholder={t('All API keys')}
          onChange={(event) => updateField('token', event.target.value)}
        />
        <datalist id='full-log-token-options'>
          {props.tokenOptions.map((token) => (
            <option
              key={`${token.id}:${token.name}`}
              value={token.id ? String(token.id) : token.name}
            >
              {token.name || `#${token.id}`}
            </option>
          ))}
        </datalist>
      </div>
      <div className='space-y-1.5'>
        <Label htmlFor='full-log-request-id'>{t('Request ID')}</Label>
        <Input
          id='full-log-request-id'
          value={props.filters.requestId}
          className='font-mono'
          onChange={(event) => updateField('requestId', event.target.value)}
        />
      </div>
      <div className='space-y-1.5'>
        <Label htmlFor='full-log-start-time'>{t('Start Time')}</Label>
        <Input
          id='full-log-start-time'
          type='datetime-local'
          value={props.filters.startTime}
          onChange={(event) => updateField('startTime', event.target.value)}
        />
      </div>
      <div className='space-y-1.5'>
        <Label htmlFor='full-log-end-time'>{t('End Time')}</Label>
        <Input
          id='full-log-end-time'
          type='datetime-local'
          value={props.filters.endTime}
          onChange={(event) => updateField('endTime', event.target.value)}
        />
      </div>
      <div className='flex items-end gap-2'>
        <Button type='submit' className='flex-1'>
          <Search />
          {t('Search')}
        </Button>
        <Button
          type='button'
          variant='outline'
          aria-label={t('Reset filters')}
          onClick={props.onReset}
        >
          <RotateCcw />
        </Button>
      </div>
    </form>
  )
}
