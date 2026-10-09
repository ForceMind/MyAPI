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
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PromptInputFooter } from '@/components/ai-elements/prompt-input'
import { Button } from '@/components/ui/button'
import { InputGroup, InputGroupTextarea } from '@/components/ui/input-group'

import {
  ATTACHMENT_ACCEPT,
  readChatAttachment,
  validateAttachmentFiles,
} from '../../lib/input/chat-attachments'
import type {
  ChatAttachment,
  PlaygroundDraftIdentity,
  ModelOption,
  ParameterEnabled,
  PlaygroundParameterKey,
  PlaygroundConfig,
  PlaygroundKey,
} from '../../types'
import { PlaygroundAttachmentPreviews } from './playground-attachment-previews'
import { PlaygroundInputControls } from './playground-input-controls'
import { PlaygroundInputTools } from './playground-input-tools'

interface PlaygroundInputProps {
  completedRetryDraft?: PlaygroundDraftIdentity | null
  config: PlaygroundConfig
  onSubmit: (text: string, attachments?: ChatAttachment[]) => Promise<boolean>
  onStop?: () => void
  disabled?: boolean
  canSend: boolean
  keyNotice?: string | null
  keys: PlaygroundKey[]
  isLoadingKeys?: boolean
  onKeyChange: (keyId: number | null) => void
  onRefreshKeys: () => void
  isGenerating?: boolean
  models: ModelOption[]
  modelValue: string
  onModelChange: (value: string) => void
  isModelLoading?: boolean
  hasMessages?: boolean
  onConfigChange: <K extends keyof PlaygroundConfig>(
    key: K,
    value: PlaygroundConfig[K]
  ) => void
  onClearMessages?: () => void
  onParameterEnabledChange: (
    key: keyof ParameterEnabled,
    value: boolean
  ) => void
  parameterEnabled: ParameterEnabled
  unsupportedParameters?: PlaygroundParameterKey[]
  unsupportedProvider?: string
}

export function PlaygroundInput(props: PlaygroundInputProps) {
  const { t } = useTranslation()
  const [text, setText] = useState('')
  const [attachments, setAttachments] = useState<ChatAttachment[]>([])
  const [attachmentError, setAttachmentError] = useState<string | null>(null)
  const [reading, setReading] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const readingRef = useRef(false)
  const submittingRef = useRef(false)
  const isComposingRef = useRef(false)
  const mountedRef = useRef(true)
  const handledRetryDraftRef = useRef<PlaygroundDraftIdentity | null>(null)
  const disabled = props.disabled || reading

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  useEffect(() => {
    const completedDraft = props.completedRetryDraft
    if (!completedDraft || handledRetryDraftRef.current === completedDraft) {
      return
    }
    handledRetryDraftRef.current = completedDraft
    const originalAttachments = completedDraft.attachmentIds
    if (
      text !== completedDraft.text ||
      attachments.length !== originalAttachments.length ||
      attachments.some(
        (attachment, index) => attachment.id !== originalAttachments[index]
      )
    ) {
      return
    }
    setText('')
    setAttachments([])
    setAttachmentError(null)
  }, [props.completedRetryDraft, text, attachments])

  const addAttachments = async (files: File[]) => {
    if (
      props.disabled ||
      readingRef.current ||
      submittingRef.current ||
      files.length === 0
    ) {
      return
    }
    readingRef.current = true
    setReading(true)
    setAttachmentError(null)
    try {
      validateAttachmentFiles(files, attachments)
      const nextAttachments = await Promise.all(files.map(readChatAttachment))
      if (mountedRef.current) {
        setAttachments((current) => [...current, ...nextAttachments])
      }
    } catch (error) {
      if (mountedRef.current) {
        setAttachmentError(
          error instanceof Error
            ? error.message
            : 'Failed to read file. Please try attaching it again.'
        )
      }
    } finally {
      readingRef.current = false
      if (mountedRef.current) setReading(false)
    }
  }

  const submit = async () => {
    if (
      disabled ||
      !props.canSend ||
      readingRef.current ||
      submittingRef.current ||
      (!text.trim() && attachments.length === 0)
    ) {
      return
    }
    submittingRef.current = true
    try {
      const succeeded = await props.onSubmit(text, attachments)
      if (succeeded && mountedRef.current) {
        setText('')
        setAttachments([])
        setAttachmentError(null)
      }
    } catch {
      if (mountedRef.current) {
        setAttachmentError(
          'The request failed. Your draft and attachments are still here.'
        )
      }
    } finally {
      submittingRef.current = false
    }
  }

  return (
    <div className='grid shrink-0 gap-2 px-1 md:pb-4'>
      <p className='text-muted-foreground px-2 text-xs'>
        {t(
          'Uses the selected API key’s permissions, budget and usage. Images and PDFs require compatible models and providers; strict token budgets reject attachments.'
        )}
      </p>
      {props.keyNotice && (
        <div className='flex items-center gap-2 px-2 text-sm' role='status'>
          {props.keyNotice}
          <Button
            type='button'
            variant='link'
            size='sm'
            disabled={props.disabled}
            onClick={props.onRefreshKeys}
          >
            {t('Refresh')}
          </Button>
        </div>
      )}
      <form
        onSubmit={(event) => {
          event.preventDefault()
          void submit()
        }}
        onDragOver={(event) => {
          if (event.dataTransfer.types.includes('Files')) event.preventDefault()
        }}
        onDrop={(event) => {
          event.preventDefault()
          void addAttachments([...event.dataTransfer.files])
        }}
      >
        <InputGroup className='bg-background/95 border-border/70 overflow-hidden rounded-xl shadow-lg'>
          <input
            type='file'
            ref={fileInputRef}
            className='hidden'
            multiple
            accept={ATTACHMENT_ACCEPT}
            aria-label={t('Upload attachments')}
            disabled={disabled}
            onChange={(event) => {
              const files = [...(event.target.files ?? [])]
              event.target.value = ''
              void addAttachments(files)
            }}
          />
          {attachments.length > 0 && (
            <PlaygroundAttachmentPreviews
              attachments={attachments}
              disabled={disabled}
              onRemove={(id) =>
                setAttachments((current) =>
                  current.filter((image) => image.id !== id)
                )
              }
            />
          )}
          <InputGroupTextarea
            autoComplete='off'
            autoCorrect='off'
            autoCapitalize='off'
            spellCheck={false}
            aria-label={t('Message')}
            className='field-sizing-content max-h-48 min-h-20 px-5 pt-4 pb-3 leading-7 md:text-base'
            disabled={disabled}
            onChange={(event) => setText(event.target.value)}
            placeholder={t('Ask anything')}
            value={text}
            onCompositionStart={() => {
              isComposingRef.current = true
            }}
            onCompositionEnd={() => {
              isComposingRef.current = false
            }}
            onKeyDown={(event) => {
              if (
                event.key === 'Enter' &&
                !event.shiftKey &&
                !event.nativeEvent.isComposing &&
                !isComposingRef.current
              ) {
                event.preventDefault()
                void submit()
              }
            }}
            onPaste={(event) => {
              const files = [...event.clipboardData.items].flatMap((item) => {
                const file = item.kind === 'file' ? item.getAsFile() : null
                return file ? [file] : []
              })
              if (files.length > 0) {
                event.preventDefault()
                void addAttachments(files)
              }
            }}
          />
          <PromptInputFooter className='border-border/60 bg-muted/20 border-t px-3 py-2.5'>
            <PlaygroundInputControls
              disabled={disabled}
              canSend={props.canSend}
              hasContent={Boolean(text.trim() || attachments.length)}
              keys={props.keys}
              keyId={props.config.keyId}
              isLoadingKeys={props.isLoadingKeys}
              onKeyChange={props.onKeyChange}
              isGenerating={props.isGenerating}
              isModelLoading={props.isModelLoading}
              models={props.models}
              modelValue={props.modelValue}
              onModelChange={props.onModelChange}
              onStop={props.onStop}
              tools={
                <PlaygroundInputTools
                  config={props.config}
                  disabled={disabled}
                  hasMessages={props.hasMessages}
                  onAttachImages={() => fileInputRef.current?.click()}
                  onConfigChange={props.onConfigChange}
                  onClearMessages={props.onClearMessages}
                  onParameterEnabledChange={props.onParameterEnabledChange}
                  parameterEnabled={props.parameterEnabled}
                  unsupportedParameters={props.unsupportedParameters}
                  unsupportedProvider={props.unsupportedProvider}
                />
              }
            />
          </PromptInputFooter>
        </InputGroup>
      </form>
      {reading && (
        <p role='status' className='px-2 text-xs'>
          {t('Reading attachments...')}
        </p>
      )}
      {attachmentError && (
        <p role='alert' className='text-destructive px-2 text-sm'>
          {t(attachmentError)}
        </p>
      )}
      <p className='text-muted-foreground px-2 text-xs'>
        {t(
          'PNG, JPEG, WEBP, GIF or PDF. Up to 4 files, 10 MiB each and 20 MiB total per conversation. Provider and policy limits may be lower. Attachments stay in memory and are not saved after leaving or reloading.'
        )}
      </p>
    </div>
  )
}
