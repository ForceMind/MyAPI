/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { FileSearchIcon, LockKeyIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'

import { useBodyCaptureStatus } from '../hooks/use-body-capture-status'
import { useFullContentLogDetail } from '../hooks/use-full-content-log-detail'
import {
  extractLogRequestText,
  extractLogResponseErrors,
  extractLogResponseText,
  formatLogBytes,
} from '../lib/format'
import type { FullContentLogDetail } from '../types'
import { ContentPanel } from './full-content-log-details-dialog'

type PanelProps = {
  /** Request ID of the call record; legacy records may have none. */
  requestId: string | null
  /** The panel is visible; nothing is read before that. */
  active: boolean
}

/**
 * Saved request and response text (clean mode). Reading a saved body never
 * resends the request; credentials stay redacted by the server.
 */
export function RequestBodyPanel(props: PanelProps) {
  return (
    <SavedContent {...props}>
      {(detail) => <ConversationText detail={detail} />}
    </SavedContent>
  )
}

function ConversationText(props: { detail: FullContentLogDetail }) {
  const { t } = useTranslation()
  const requestText = useMemo(
    () =>
      extractLogRequestText(
        props.detail.request_body,
        props.detail.request_encoding,
        props.detail.request_content_type
      ),
    [props.detail]
  )
  const responseText = useMemo(
    () =>
      extractLogResponseText(
        props.detail.response_body,
        props.detail.response_encoding,
        props.detail.response_content_type
      ),
    [props.detail]
  )
  return (
    <>
      <ContentPanel
        title={t('Request text')}
        body={requestText}
        encoding='utf-8'
        contentType='text/plain'
        emptyText={t('No readable request text found')}
        compact
      />
      <ContentPanel
        title={t('Model response text')}
        body={responseText}
        encoding='utf-8'
        contentType='text/plain'
        emptyText={t('No readable response text found')}
        truncated={props.detail.response_body_truncated}
        totalBytes={props.detail.response_body_total_bytes}
      />
    </>
  )
}

/**
 * The raw response as saved, which for a streamed request is the SSE event
 * stream. Long lines scroll inside the code area unless wrapping is on.
 */
export function RequestStreamPanel(props: PanelProps) {
  const { t } = useTranslation()
  const [wrap, setWrap] = useState(false)
  return (
    <SavedContent {...props}>
      {(detail) => (
        <>
          <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'>
            <p className='text-caption text-muted-foreground'>
              {detail.response_content_type?.includes('event-stream')
                ? t('Streamed response: {{count}} events saved', {
                    count: detail.chunk_count,
                  })
                : t('This response was not streamed; the raw body is shown.')}
            </p>
            <div className='flex items-center gap-2'>
              <Switch
                id='request-stream-wrap'
                checked={wrap}
                onCheckedChange={setWrap}
              />
              <Label htmlFor='request-stream-wrap'>{t('Wrap lines')}</Label>
            </div>
          </div>
          <ContentPanel
            title={t('Raw response body')}
            body={detail.response_body}
            encoding={detail.response_encoding}
            contentType={detail.response_content_type}
            emptyText={t('Empty response body')}
            truncated={detail.response_body_truncated}
            totalBytes={detail.response_body_total_bytes}
            wrap={wrap}
          />
          <ContentPanel
            title={t('Response headers')}
            body={
              Object.keys(detail.response_headers || {}).length
                ? JSON.stringify(detail.response_headers, null, 2)
                : ''
            }
            encoding='json'
            contentType='application/json'
            emptyText={t('No response headers were recorded')}
            compact
            wrap={wrap}
          />
        </>
      )}
    </SavedContent>
  )
}

function SavedContent(
  props: PanelProps & {
    children: (detail: FullContentLogDetail) => ReactNode
  }
) {
  const { t } = useTranslation()
  const content = useFullContentLogDetail(props.requestId, props.active)
  const capture = useBodyCaptureStatus(props.active && content.missing)

  if (!props.requestId) {
    return (
      <Empty className='border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={FileSearchIcon} aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('No Request ID on this record')}</EmptyTitle>
          <EmptyDescription>
            {t('Saved bodies are matched by Request ID, so none can be shown.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }
  if (!content.canRead) {
    return (
      <Empty className='border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={LockKeyIcon} aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('Administrator access required')}</EmptyTitle>
          <EmptyDescription>
            {t('Request content is available to administrators only.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }
  if (content.detailQuery.isPending) {
    return (
      <div className='flex flex-col gap-3' aria-busy='true'>
        <span className='sr-only'>{t('Loading...')}</span>
        <Skeleton className='h-16 w-full' />
        <Skeleton className='h-48 w-full' />
      </div>
    )
  }
  if (content.missing) {
    let captureText = t('Checking whether body capture is on...')
    if (capture.capturing === true) {
      captureText = t(
        'Body capture is on. This request may predate it, or its saved file may have expired.'
      )
    } else if (capture.capturing === false) {
      captureText = t(
        'Body capture is off (FULL_CONTENT_LOG_ENABLED). Existing files remain available.'
      )
    } else if (capture.query.isError) {
      captureText = t('Body capture status is unknown.')
    }
    return (
      <Empty className='border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={FileSearchIcon} aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('No saved body for this request')}</EmptyTitle>
          <EmptyDescription>{captureText}</EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }
  if (content.detailQuery.isError || !content.detail) {
    return (
      <Alert variant='destructive'>
        <AlertTitle>{t('Could not load the saved body')}</AlertTitle>
        <AlertDescription className='flex flex-col items-start gap-2'>
          <span>{content.contentError}</span>
          <Button
            variant='outline'
            size='sm'
            disabled={content.detailQuery.isFetching}
            onClick={() => void content.detailQuery.refetch()}
          >
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }

  const detail = content.detail
  const errors = [
    ...new Set([
      ...(detail.error ? [detail.error] : []),
      ...extractLogResponseErrors(
        detail.response_body,
        detail.response_encoding,
        detail.response_content_type
      ),
    ]),
  ]
  return (
    <div className='flex min-w-0 flex-col gap-4'>
      <p className='text-caption text-muted-foreground'>
        {t(
          'Saved, redacted content. Viewing it never resends the request; credentials stay redacted.'
        )}
      </p>
      <dl className='grid grid-cols-2 gap-3 rounded-lg border p-3'>
        <div className='min-w-0'>
          <dt className='text-caption text-muted-foreground'>
            {t('HTTP status')}
          </dt>
          <dd className='text-data tabular-nums'>
            {detail.status || t('Not recorded')}
          </dd>
        </div>
        <div className='min-w-0'>
          <dt className='text-caption text-muted-foreground'>
            {t('Duration')}
          </dt>
          <dd className='text-data tabular-nums'>{detail.duration_ms} ms</dd>
        </div>
        <div className='col-span-2 min-w-0'>
          <dt className='text-caption text-muted-foreground'>{t('Size')}</dt>
          <dd className='text-data tabular-nums'>
            {formatLogBytes(detail.request_bytes)} →{' '}
            {formatLogBytes(detail.response_bytes)}
          </dd>
        </div>
      </dl>
      {errors.length > 0 && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Error')}</AlertTitle>
          <AlertDescription className='wrap-anywhere'>
            {errors.map((message) => (
              <p key={message}>{message}</p>
            ))}
          </AlertDescription>
        </Alert>
      )}
      {props.children(detail)}
    </div>
  )
}
