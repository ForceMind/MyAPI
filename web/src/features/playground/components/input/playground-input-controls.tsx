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
import { SendIcon, SquareIcon } from 'lucide-react'
import { useId, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { PromptInputButton } from '@/components/ai-elements/prompt-input'
import { ModelSelector } from '@/components/model-group-selector'
import { Field } from '@/components/ui/field'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'

import { isPlaygroundKeyAvailable } from '../../api'
import type { ModelOption, PlaygroundKey } from '../../types'

interface PlaygroundInputControlsProps {
  disabled?: boolean
  canSend: boolean
  hasContent: boolean
  isGenerating?: boolean
  isModelLoading?: boolean
  models: ModelOption[]
  modelValue: string
  keys: PlaygroundKey[]
  keyId?: number | null
  isLoadingKeys?: boolean
  onKeyChange: (keyId: number | null) => void
  onModelChange: (value: string) => void
  onStop?: () => void
  tools: ReactNode
}

export function PlaygroundInputControls(props: PlaygroundInputControlsProps) {
  const { t } = useTranslation()
  const identityId = useId()
  const keyOptions = props.keys.map((key) => ({
    value: key.id,
    label: `${key.name || `#${key.id}`} · ${key.group}${!isPlaygroundKeyAvailable(key) ? ` (${t('Unavailable')})` : ''}`,
    disabled: !isPlaygroundKeyAvailable(key),
  }))
  const selectedKey = keyOptions.find((key) => key.value === props.keyId)
  return (
    <div className='flex w-full flex-wrap items-center gap-2'>
      {props.tools}
      <Field
        aria-label={t('API key')}
        className='w-full min-w-0 basis-full gap-1 sm:max-w-56 sm:min-w-48 sm:flex-1 sm:basis-auto'
      >
        <NativeSelect
          aria-label={t('API key')}
          aria-describedby={props.keyId ? identityId : undefined}
          className='w-full min-w-0 [&_select]:h-11 sm:[&_select]:h-8'
          value={props.keyId ?? ''}
          disabled={props.disabled || props.isLoadingKeys}
          onChange={(event) =>
            props.onKeyChange(
              event.target.value ? Number(event.target.value) : null
            )
          }
        >
          <NativeSelectOption value=''>
            {t('Select API key')}
          </NativeSelectOption>
          {props.keyId && !selectedKey && (
            <NativeSelectOption value={props.keyId} disabled>
              {t('Unavailable API key')}
            </NativeSelectOption>
          )}
          {keyOptions.map((key) => (
            <NativeSelectOption
              key={key.value}
              value={key.value}
              disabled={key.disabled}
            >
              {key.label}
            </NativeSelectOption>
          ))}
        </NativeSelect>
        {props.keyId && (
          <p
            id={identityId}
            className='text-sm leading-snug [overflow-wrap:anywhere] whitespace-normal'
          >
            {selectedKey?.label ||
              `${t('Unavailable API key')} · #${props.keyId}`}
          </p>
        )}
      </Field>
      <ModelSelector
        selectedModel={props.modelValue}
        models={props.models}
        onModelChange={props.onModelChange}
        disabled={
          props.disabled || props.isModelLoading || props.models.length === 0
        }
      />
      {props.isGenerating && props.onStop ? (
        <PromptInputButton
          aria-label={t('Stop')}
          onClick={props.onStop}
          variant='secondary'
        >
          <SquareIcon aria-hidden='true' size={16} />
          {t('Stop')}
        </PromptInputButton>
      ) : (
        <PromptInputButton
          aria-label={t('Send')}
          disabled={props.disabled || !props.canSend || !props.hasContent}
          type='submit'
          variant='default'
        >
          <SendIcon aria-hidden='true' size={16} />
          {t('Send')}
        </PromptInputButton>
      )}
    </div>
  )
}
