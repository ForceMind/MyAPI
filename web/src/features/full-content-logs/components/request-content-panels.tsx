/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { FileSearchIcon, LockKeyIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { ReactNode } from 'react'
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
import { Skeleton } from '@/components/ui/skeleton'

import { useBodyCaptureStatus } from '../hooks/use-body-capture-status'
import { useFullContentLogDetail } from '../hooks/use-full-content-log-detail'
import { formatLogBytes } from '../lib/format'
import type { FullContentLogDetail } from '../types'
import { SavedContentRenderer } from './saved-content-renderer'

type PanelProps = {
  /** Request ID of the call record; legacy records may have none. */
  requestId: string | null
  /** The panel is visible; nothing is read before that. */
  active: boolean
}

/** Saved body views retain the original administrator-only read boundary. */
export function RequestBodyPanel(props: PanelProps) {
  return (
    <SavedContent {...props}>
      {(detail) => <SavedContentRenderer key={detail.request_id} detail={detail} />}
    </SavedContent>
  )
}

/** Local event inspection reads only the saved stream, never a live relay. */
export function RequestStreamPanel(props: PanelProps) {
  return (
    <SavedContent {...props}>
      {(detail) => <SavedContentRenderer key={detail.request_id} detail={detail} view='stream' />}
    </SavedContent>
  )
}

/** Exact saved request/response representations and expandable saved headers. */
export function RequestOriginalPanel(props: PanelProps) {
  return (
    <SavedContent {...props}>
      {(detail) => <SavedContentRenderer key={detail.request_id} detail={detail} view='original' />}
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

  if (!props.active) return null

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
  return (
    <div className='flex min-w-0 flex-col gap-4'>
      <p className='text-caption text-muted-foreground'>
        {t(
          'Saved, redacted content. Viewing it never resends the request; credentials stay redacted.'
        )}
      </p>
      <dl className='grid grid-cols-2 gap-3 rounded-lg border p-3'>
        <div className='col-span-2 min-w-0'>
          <dt className='text-caption text-muted-foreground'>{t('Saved content timestamp')}</dt>
          <dd className='text-data wrap-anywhere'>{detail.timestamp || t('Not recorded')}</dd>
        </div>
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
          <dd className='text-data tabular-nums'>
            {Number.isFinite(detail.duration_ms) ? `${detail.duration_ms} ms` : t('Not recorded')}
          </dd>
        </div>
        <div className='col-span-2 min-w-0'>
          <dt className='text-caption text-muted-foreground'>{t('Size')}</dt>
          <dd className='text-data tabular-nums'>
            {Number.isFinite(detail.request_bytes) ? formatLogBytes(detail.request_bytes) : t('Not recorded')} →{' '}
            {Number.isFinite(detail.response_bytes) ? formatLogBytes(detail.response_bytes) : t('Not recorded')}
          </dd>
        </div>
      </dl>
      {props.children(detail)}
    </div>
  )
}
