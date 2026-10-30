/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Copy01Icon, Tick02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { cn } from '@/lib/utils'

import { previewContent } from '../lib/content-preview'
import { formatLogBody, formatLogBytes } from '../lib/format'

export interface ContentPanelProps {
  body: string
  encoding?: string
  contentType?: string
  emptyText: string
  title: string
  compact?: boolean
  truncated?: boolean
  totalBytes?: number
  wrap?: boolean
  defaultWrap?: boolean
  /** Show and copy the exact saved representation, including JSON spacing. */
  preserveOriginal?: boolean
}

export function ContentPanel(props: ContentPanelProps) {
  const { t } = useTranslation()
  const titleId = useId()
  const { copiedText, copyToClipboard } = useCopyToClipboard()
  const [localWrap, setLocalWrap] = useState(props.defaultWrap ?? true)
  const wrap = props.wrap ?? localWrap
  const preview = useMemo(() => {
    const source = previewContent(props.body)
    if (props.preserveOriginal || source.limited) return source
    const formatted = previewContent(
      formatLogBody(source.preview, props.encoding, props.contentType)
    )
    return { preview: formatted.preview, limited: formatted.limited }
  }, [props.body, props.contentType, props.encoding, props.preserveOriginal])
  const [copiedSource, setCopiedSource] = useState<string | null>(null)

  const copyContent = async () => {
    // Full formatting happens only on an explicit copy, never during preview
    // rendering. Original mode always preserves the saved string exactly.
    const value = props.preserveOriginal
      ? props.body
      : formatLogBody(props.body, props.encoding, props.contentType)
    const success = await copyToClipboard(value)
    setCopiedSource(success ? props.body : null)
  }

  return (
    <section className='min-w-0 space-y-2' aria-labelledby={titleId}>
      <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'>
        <div className='flex min-w-0 flex-wrap items-center gap-1.5'>
          <h3 id={titleId} className='font-medium wrap-anywhere'>
            {props.title}
          </h3>
          <Badge variant='outline'>
            {props.encoding || t('Encoding not recorded')}
          </Badge>
          {props.contentType && (
            <Badge
              variant='secondary'
              className='max-w-full whitespace-normal wrap-anywhere'
            >
              {props.contentType}
            </Badge>
          )}
        </div>
        <div className='flex flex-wrap items-center gap-2'>
          {props.wrap === undefined && (
            <Button
              type='button'
              size='sm'
              variant='outline'
              className='min-h-11'
              aria-pressed={wrap}
              onClick={() => setLocalWrap((value) => !value)}
            >
              {t('Wrap lines')}
            </Button>
          )}
          <Button
            type='button'
            size='sm'
            variant='outline'
            className='min-h-11'
            disabled={!props.body}
            onClick={copyContent}
          >
            <HugeiconsIcon
              icon={
                copiedSource === props.body && copiedText !== null
                  ? Tick02Icon
                  : Copy01Icon
              }
              data-icon='inline-start'
              aria-hidden='true'
            />
            {t('Copy')}
          </Button>
        </div>
      </div>
      <pre
        tabIndex={0}
        className={cn(
          'bg-muted/40 max-w-full overflow-auto rounded-lg border p-3 font-mono text-xs leading-relaxed',
          wrap ? 'break-all whitespace-pre-wrap' : 'whitespace-pre',
          props.compact ? 'max-h-48 min-h-24' : 'max-h-[48vh] min-h-48'
        )}
      >
        {preview.preview || props.emptyText}
      </pre>
      {preview.limited && (
        <p className='text-muted-foreground text-xs'>
          {t('Content preview truncated to 1 MB')}
          {' '}
          {t('Copy includes all loaded content, beyond this display preview.')}
          {!props.truncated && typeof props.totalBytes === 'number' && Number.isFinite(props.totalBytes)
            ? ` (${formatLogBytes(props.totalBytes)})`
            : ''}
        </p>
      )}
      {props.truncated && (
        <p className='text-muted-foreground text-xs'>
          {t('The saved response returned by the server is truncated.')}{' '}
          {t(
            'This is a preview of a large response. Download the raw log file to inspect the complete content.'
          )}
          {typeof props.totalBytes === 'number' && Number.isFinite(props.totalBytes)
            ? ` (${formatLogBytes(props.totalBytes)})`
            : ''}
        </p>
      )}
    </section>
  )
}
