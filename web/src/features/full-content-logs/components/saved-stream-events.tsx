/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import { parseSavedSse } from '../lib/saved-sse'
import type { FullContentLogDetail } from '../types'
import { ContentPanel } from './content-panel'

const EVENT_BATCH_SIZE = 50
const EVENT_TYPE_FIELDS = { event: 'event', payload: 'data.type', default: '' } as const

/** A local inspection of saved events, without a provider call or state change. */
export function SavedStreamEvents(props: { detail: FullContentLogDetail }) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [visibleCount, setVisibleCount] = useState(EVENT_BATCH_SIZE)
  const stream = useMemo(() => {
    const header = Object.entries(props.detail.response_headers || {}).find(
      ([name]) => name.toLowerCase() === 'content-type'
    )?.[1]?.[0]
    return parseSavedSse({
      body: props.detail.response_body,
      encoding: props.detail.response_encoding,
      contentType: props.detail.response_content_type || header,
      truncated: props.detail.response_body_truncated,
    })
  }, [props.detail])
  const matches = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return stream.events
    return stream.events.filter((event) =>
      event.raw.toLowerCase().includes(query)
    )
  }, [search, stream.events])
  const hasCompletionMarker = stream.events.some((event) =>
    event.markers.some((marker) => marker.kind === 'completion')
  )

  let evidenceText = t('No SSE structure was found in the saved preview. Stream status is unknown.')
  if (props.detail.response_encoding === 'base64') {
    evidenceText = t('The saved response is base64 encoded; SSE events are not decoded.')
  } else if (stream.headerDeclaresSse) {
    evidenceText = t('The saved content type identifies an SSE response.')
  } else if (stream.hasSseFields) {
    evidenceText = t('SSE fields were observed in the saved body; the saved content type does not confirm streaming.')
  }

  return (
    <div className='min-w-0 space-y-4'>
      <div className='text-muted-foreground space-y-2 rounded-lg border p-3 text-xs'>
        <p>{evidenceText}</p>
        <p>
          {t('Saved response chunk count')}{': '}
          {Number.isFinite(props.detail.chunk_count) ? props.detail.chunk_count : t('Not recorded')}
        </p>
        <p>{t('Event order comes from saved content. Per-event capture times are not available.')}</p>
        <p>{t('Completion and error markers below describe saved protocol fields, not business success or settlement.')}</p>
        {!hasCompletionMarker && (
          <p>{t('No completion marker was observed in this preview; missing evidence does not establish success or failure.')}</p>
        )}
        {stream.previewLimited && (
          <p>{t('Event parsing and search are limited to the first 1 MB of saved content.')}</p>
        )}
        {stream.sourceTruncated && (
          <p>{t('The saved response returned by the server is truncated.')}</p>
        )}
        {stream.incompleteTail && (
          <p>{t('The last saved event in this preview is incomplete. Its end markers are not confirmed.')}</p>
        )}
      </div>
      <div className='space-y-2'>
        <label className='block text-sm font-medium'>
          <span>{t('Search saved event preview')}</span>
          <Input
            type='search'
            className='mt-2 min-h-11'
            value={search}
            onChange={(event) => {
              setSearch(event.target.value)
              setVisibleCount(EVENT_BATCH_SIZE)
            }}
          />
        </label>
        <p role='status' className='text-muted-foreground text-xs'>
          {t('{{count}} matching events in the saved preview', { count: matches.length })}
        </p>
      </div>
      {matches.length === 0 && (
        <p className='text-muted-foreground text-sm'>{t('No matching saved events')}</p>
      )}
      <ol className='min-w-0 space-y-2' aria-label={t('Saved SSE event preview')}>
        {matches.slice(0, visibleCount).map((event) => (
          <li key={event.index} className='min-w-0 rounded-lg border px-3'>
            <details>
              <summary className='min-h-11 cursor-pointer py-3 text-sm wrap-anywhere focus-visible:outline-2'>
                <span className='mr-2 font-mono'>#{event.index}</span>
                <span className='mr-2'>{event.type}</span>
                {!event.complete && <Badge variant='outline'>{t('Incomplete saved event')}</Badge>}
                {event.markers.map((marker) => (
                  <Badge key={`${marker.kind}:${marker.source}`} variant={marker.kind === 'error' ? 'destructive' : 'outline'} className='m-1 whitespace-normal'>
                    {marker.kind === 'error' ? t('Observed error marker') : t('Observed completion marker')}
                    {': '}{marker.source}
                  </Badge>
                ))}
              </summary>
              <div className='min-w-0 space-y-3 pb-3'>
                <dl className='grid grid-cols-1 gap-2 text-xs sm:grid-cols-2'>
                  <div>
                    <dt className='text-muted-foreground'>{t('Event type source')}</dt>
                    <dd>{event.typeSource === 'default' ? t('Default SSE message type') : `${EVENT_TYPE_FIELDS[event.typeSource]}: ${event.type}`}</dd>
                  </div>
                  {event.id !== undefined && <div><dt className='text-muted-foreground'>id</dt><dd className='wrap-anywhere'>{event.id}</dd></div>}
                  {event.retry !== undefined && <div><dt className='text-muted-foreground'>retry</dt><dd className='wrap-anywhere'>{event.retry}</dd></div>}
                </dl>
                <ContentPanel
                  title={t('Saved event data')}
                  body={event.data}
                  encoding='utf-8'
                  emptyText={t('Empty saved event data')}
                  preserveOriginal
                  compact
                />
              </div>
            </details>
          </li>
        ))}
      </ol>
      {matches.length > visibleCount && (
        <Button
          type='button'
          variant='outline'
          className='min-h-11'
          onClick={() => setVisibleCount((count) => count + EVENT_BATCH_SIZE)}
        >
          {t('Show more saved events')}
        </Button>
      )}
      <ContentPanel
        title={t('Raw response body')}
        body={props.detail.response_body}
        encoding={props.detail.response_encoding}
        contentType={props.detail.response_content_type}
        emptyText={t('Empty response body')}
        truncated={props.detail.response_body_truncated}
        totalBytes={props.detail.response_body_total_bytes}
        preserveOriginal
        defaultWrap={false}
      />
    </div>
  )
}
