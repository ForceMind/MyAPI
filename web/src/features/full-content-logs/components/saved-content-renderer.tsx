/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { previewContent } from '../lib/content-preview'
import {
  extractLogRequestText,
  extractLogResponseErrors,
  extractLogResponseText,
} from '../lib/format'
import type { FullContentLogDetail } from '../types'
import { ContentPanel } from './content-panel'
import { SavedStreamEvents } from './saved-stream-events'

/** Only serialize and mount a saved header/parameter record when expanded. */
function SavedRecord(props: {
  title: string
  values: Record<string, string[]>
  emptyText: string
}) {
  const [open, setOpen] = useState(false)
  return (
    <details
      className='min-w-0 rounded-lg border px-3'
      onToggle={(event) => setOpen(event.currentTarget.open)}
    >
      <summary className='min-h-11 cursor-pointer py-3 text-sm font-medium focus-visible:outline-2'>
        {props.title}
      </summary>
      {open && (
        <div className='pb-3'>
          <ContentPanel
            title={props.title}
            body={
              Object.keys(props.values || {}).length
                ? JSON.stringify(props.values, null, 2)
                : ''
            }
            encoding='json'
            contentType='application/json'
            emptyText={props.emptyText}
            preserveOriginal
            compact
          />
        </div>
      )}
    </details>
  )
}

/**
 * Pure saved-detail presentation shared by the request page and legacy dialog.
 * Fetching and authorization stay with their existing entrypoint and hook.
 */
export function SavedContentRenderer(props: {
  detail: FullContentLogDetail
  view?: 'body' | 'original' | 'stream'
  /** Keep the legacy dialog's Request and Response tabs. */
  tabbed?: boolean
}) {
  const { t } = useTranslation()
  const [showRaw, setShowRaw] = useState(false)
  const raw = props.view === 'original' || showRaw
  const content = useMemo(() => {
    if (props.view === 'stream') return null
    const requestPreview = previewContent(props.detail.request_body)
    const responsePreview = previewContent(props.detail.response_body)
    return {
      requestText: extractLogRequestText(
        requestPreview.preview,
        props.detail.request_encoding,
        props.detail.request_content_type
      ),
      responseText: extractLogResponseText(
        responsePreview.preview,
        props.detail.response_encoding,
        props.detail.response_content_type
      ),
      errors: [
        ...new Set([
          ...(props.detail.error ? [props.detail.error] : []),
          ...extractLogResponseErrors(
            responsePreview.preview,
            props.detail.response_encoding,
            props.detail.response_content_type
          ),
        ]),
      ],
      limited: requestPreview.limited || responsePreview.limited,
    }
  }, [props.detail, props.view])

  const request = (
    <div className='min-w-0 space-y-3'>
      <SavedRecord
        title={t('Request headers')}
        values={props.detail.request_headers}
        emptyText={t('No request headers were recorded')}
      />
      {raw && (
        <>
          <div className='space-y-1 rounded-lg border p-3 text-xs'>
            <div className='text-muted-foreground'>{t('Request URL')}</div>
            <div className='font-mono wrap-anywhere'>
              {props.detail.method} {props.detail.path}
            </div>
          </div>
          <SavedRecord
            title={t('Query parameters')}
            values={props.detail.query}
            emptyText={t('No query parameters')}
          />
        </>
      )}
      <ContentPanel
        title={raw ? t('Raw request body') : t('Request text')}
        body={raw ? props.detail.request_body : content?.requestText || ''}
        encoding={raw ? props.detail.request_encoding : 'utf-8'}
        contentType={raw ? props.detail.request_content_type : 'text/plain'}
        emptyText={raw ? t('Empty request body') : t('No readable request text found')}
        preserveOriginal={raw}
        defaultWrap={!raw}
      />
    </div>
  )
  const response = (
    <div className='min-w-0 space-y-3'>
      <SavedRecord
        title={t('Response headers')}
        values={props.detail.response_headers}
        emptyText={t('No response headers were recorded')}
      />
      <ContentPanel
        title={raw ? t('Raw response body') : t('Model response text')}
        body={raw ? props.detail.response_body : content?.responseText || ''}
        encoding={raw ? props.detail.response_encoding : 'utf-8'}
        contentType={raw ? props.detail.response_content_type : 'text/plain'}
        emptyText={raw ? t('Empty response body') : t('No readable response text found')}
        truncated={props.detail.response_body_truncated}
        totalBytes={props.detail.response_body_total_bytes}
        preserveOriginal={raw}
        defaultWrap={!raw}
      />
    </div>
  )

  if (props.view === 'stream') {
    return (
      <div className='min-w-0 space-y-3'>
        {props.detail.error && (
          <Alert variant='destructive'>
            <AlertTitle>{t('Error')}</AlertTitle>
            <AlertDescription className='wrap-anywhere'>
              {props.detail.error}
            </AlertDescription>
          </Alert>
        )}
        <SavedStreamEvents key={props.detail.request_id} detail={props.detail} />
        <SavedRecord
          title={t('Response headers')}
          values={props.detail.response_headers}
          emptyText={t('No response headers were recorded')}
        />
      </div>
    )
  }

  return (
    <div className='min-w-0 space-y-4'>
      {content && content.errors.length > 0 && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Error')}</AlertTitle>
          <AlertDescription className='space-y-1 wrap-anywhere'>
            {content.errors.map((message) => (
              <p key={message}>{message}</p>
            ))}
          </AlertDescription>
        </Alert>
      )}
      {props.view !== 'original' && (
        <div className='bg-muted/30 flex min-w-0 flex-col gap-3 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between'>
          <p className='text-muted-foreground text-xs'>
            {raw
              ? t(
                  'Raw mode shows headers, parameters and protocol envelopes. Sensitive credentials remain redacted.'
                )
              : t(
                  'Clean mode shows only conversational text and is enabled by default.'
                )}
          </p>
          <div className='flex shrink-0 flex-wrap gap-2'>
            <Button
              type='button'
              className='min-h-11'
              variant={showRaw ? 'outline' : 'secondary'}
              aria-pressed={!showRaw}
              onClick={() => setShowRaw(false)}
            >
              {t('Clean text')}
            </Button>
            <Button
              type='button'
              className='min-h-11'
              variant={showRaw ? 'secondary' : 'outline'}
              aria-pressed={showRaw}
              onClick={() => setShowRaw(true)}
            >
              {t('Raw data')}
            </Button>
          </div>
        </div>
      )}
      {!raw && content?.limited && (
        <p className='text-muted-foreground text-xs'>
          {t(
            'Readable text is extracted from the first 1 MB. Switch to raw data to copy all loaded content.'
          )}
        </p>
      )}
      {props.tabbed ? (
        <Tabs defaultValue='request'>
          <TabsList className='h-auto min-h-11 max-w-full overflow-x-auto'>
            <TabsTrigger value='request' className='min-h-11'>
              {t('Request')}
            </TabsTrigger>
            <TabsTrigger value='response' className='min-h-11'>
              {t('Response')} (
              {Number.isFinite(props.detail.chunk_count)
                ? props.detail.chunk_count
                : t('Not recorded')}
              )
            </TabsTrigger>
          </TabsList>
          <TabsContent value='request' className='mt-3'>
            {request}
          </TabsContent>
          <TabsContent value='response' className='mt-3'>
            {response}
          </TabsContent>
        </Tabs>
      ) : (
        <>
          {request}
          {response}
        </>
      )}
    </div>
  )
}
