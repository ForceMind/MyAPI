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
import { Copy01Icon, Tick02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { useMediaQuery } from '@/hooks/use-media-query'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { cn } from '@/lib/utils'

import { useFullContentLogDetail } from '../hooks/use-full-content-log-detail'
import {
  extractLogRequestText,
  extractLogResponseText,
  extractLogResponseErrors,
  formatLogBody,
  formatLogBytes,
} from '../lib/format'
import { FullContentLogViewModeToggle } from './full-content-log-view-mode-toggle'

interface FullContentLogDetailsDialogProps {
  requestId: string | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

const MAX_DISPLAY_LENGTH = 1024 * 1024

export function ContentPanel(props: {
  body: string
  encoding?: string
  contentType?: string
  emptyText: string
  title: string
  compact?: boolean
  truncated?: boolean
  totalBytes?: number
  /** `false` keeps long lines intact and scrolls the code area sideways. */
  wrap?: boolean
}) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard()
  const formattedBody = useMemo(
    () => formatLogBody(props.body, props.encoding, props.contentType),
    [props.body, props.contentType, props.encoding]
  )
  const displayBody = useMemo(
    () =>
      formattedBody.length > MAX_DISPLAY_LENGTH
        ? `${formattedBody.slice(0, MAX_DISPLAY_LENGTH)}\n\n… ${t('Content preview truncated to 1 MB')}`
        : formattedBody,
    [formattedBody, t]
  )

  return (
    <div className='space-y-2'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='flex flex-wrap items-center gap-1.5'>
          <span className='font-medium'>{props.title}</span>
          <Badge variant='outline'>{props.encoding || 'utf-8'}</Badge>
          {props.contentType && (
            <Badge variant='secondary'>{props.contentType}</Badge>
          )}
        </div>
        <Button
          size='sm'
          variant='outline'
          disabled={!formattedBody}
          onClick={() => void copyToClipboard(formattedBody)}
        >
          <HugeiconsIcon
            icon={copiedText === formattedBody ? Tick02Icon : Copy01Icon}
            data-icon='inline-start'
            aria-hidden='true'
          />
          {t('Copy')}
        </Button>
      </div>
      <pre
        className={cn(
          'bg-muted/40 overflow-auto rounded-lg border p-3 font-mono text-xs leading-relaxed',
          props.wrap === false
            ? 'whitespace-pre'
            : 'break-all whitespace-pre-wrap',
          props.compact ? 'max-h-48 min-h-24' : 'max-h-[48vh] min-h-48'
        )}
      >
        {displayBody || props.emptyText}
      </pre>
      {(props.truncated || formattedBody.length > MAX_DISPLAY_LENGTH) && (
        <div className='text-muted-foreground text-xs'>
          {t(
            'This is a preview of a large response. Download the raw log file to inspect the complete content.'
          )}
          {props.totalBytes ? ` (${formatLogBytes(props.totalBytes)})` : ''}
        </div>
      )}
    </div>
  )
}

export function FullContentLogDetailsDialog(
  props: FullContentLogDetailsDialogProps
) {
  const { t } = useTranslation()
  const isMobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const [showRaw, setShowRaw] = useState(false)
  const { detailQuery, detail, contentError, canRead } =
    useFullContentLogDetail(props.requestId, props.open)
  const requestText = useMemo(
    () =>
      detail
        ? extractLogRequestText(
            detail.request_body,
            detail.request_encoding,
            detail.request_content_type
          )
        : '',
    [detail]
  )
  const responseText = useMemo(
    () =>
      detail
        ? extractLogResponseText(
            detail.response_body,
            detail.response_encoding,
            detail.response_content_type
          )
        : '',
    [detail]
  )
  const responseErrors = useMemo(
    () =>
      detail
        ? [
            ...new Set([
              ...(detail.error ? [detail.error] : []),
              ...extractLogResponseErrors(
                detail.response_body,
                detail.response_encoding,
                detail.response_content_type
              ),
            ]),
          ]
        : [],
    [detail]
  )

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Full Content Log Detail')}
      description={props.requestId || undefined}
      contentClassName='w-[calc(100vw-1rem)] max-w-none sm:max-w-6xl'
      contentHeight={isMobile ? 'min(88dvh, 900px)' : 'min(82vh, 900px)'}
      bodyClassName='pb-[env(safe-area-inset-bottom,0px)]'
    >
      {!canRead && (
        <p role='alert'>
          {t('Request content is available to administrators only.')}
        </p>
      )}
      {detailQuery.isLoading && (
        <div className='space-y-3' aria-label={t('Loading')}>
          <Skeleton className='h-16 w-full' />
          <Skeleton className='h-80 w-full' />
        </div>
      )}

      {detailQuery.isError && (
        <div className='border-destructive/30 bg-destructive/5 rounded-lg border p-4 text-sm'>
          <div className='text-destructive'>{contentError}</div>
          <Button
            className='mt-3'
            size='sm'
            variant='outline'
            onClick={() => void detailQuery.refetch()}
          >
            {t('Retry')}
          </Button>
        </div>
      )}

      {detail && !detailQuery.isError && (
        <div className='space-y-4'>
          <p className='text-muted-foreground rounded-lg border p-3 text-sm'>
            {t(
              'HTTP status describes the transport. Streaming failures may appear inside the response body.'
            )}
          </p>
          <div className='grid gap-2 rounded-lg border p-3 text-xs sm:grid-cols-2 lg:grid-cols-4'>
            <div>
              <span className='text-muted-foreground'>{t('Model')}</span>
              <div className='mt-1 font-medium'>{detail.model || '-'}</div>
            </div>
            <div>
              <span className='text-muted-foreground'>{t('HTTP status')}</span>
              <div className='mt-1 font-medium'>{detail.status || '-'}</div>
            </div>
            <div>
              <span className='text-muted-foreground'>{t('Duration')}</span>
              <div className='mt-1 font-medium'>{detail.duration_ms} ms</div>
            </div>
            <div>
              <span className='text-muted-foreground'>{t('Size')}</span>
              <div className='mt-1 font-medium'>
                {formatLogBytes(detail.request_bytes)} →{' '}
                {formatLogBytes(detail.response_bytes)}
              </div>
            </div>
            <div className='sm:col-span-2'>
              <span className='text-muted-foreground'>{t('Endpoint')}</span>
              <div className='mt-1 font-mono break-all'>
                {detail.method} {detail.path}
              </div>
            </div>
            <div>
              <span className='text-muted-foreground'>{t('API Key')}</span>
              <div className='mt-1 break-all'>
                {detail.token_name || '-'}
                {detail.token_id ? ` (#${detail.token_id})` : ''}
              </div>
            </div>
            <div>
              <span className='text-muted-foreground'>{t('Client IP')}</span>
              <div className='mt-1 font-mono'>{detail.client_ip || '-'}</div>
            </div>
          </div>

          {responseErrors.length > 0 && (
            <div
              role='alert'
              className='border-destructive/30 bg-destructive/5 text-destructive space-y-1 rounded-lg border p-3 text-sm break-words'
            >
              <p className='font-medium'>{t('Error')}</p>
              {responseErrors.map((message) => (
                <p key={message}>{message}</p>
              ))}
            </div>
          )}

          <div className='bg-muted/30 flex flex-col gap-3 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between'>
            <div className='text-muted-foreground text-xs'>
              {showRaw
                ? t(
                    'Raw mode shows headers, parameters and protocol envelopes. Sensitive credentials remain redacted.'
                  )
                : t(
                    'Clean mode shows only conversational text and is enabled by default.'
                  )}
            </div>
            <FullContentLogViewModeToggle
              raw={showRaw}
              onRawChange={setShowRaw}
            />
          </div>

          <Tabs defaultValue='request'>
            <TabsList className='max-w-full overflow-x-auto'>
              <TabsTrigger value='request'>{t('Request')}</TabsTrigger>
              <TabsTrigger value='response'>
                {t('Response')} ({detail.chunk_count})
              </TabsTrigger>
            </TabsList>
            <TabsContent value='request' className='mt-3 space-y-4'>
              {showRaw ? (
                <>
                  <div className='space-y-1 rounded-lg border p-3 text-xs'>
                    <div className='text-muted-foreground'>
                      {t('Request URL')}
                    </div>
                    <div className='font-mono break-all'>
                      {detail.method} {detail.path}
                    </div>
                  </div>
                  <ContentPanel
                    title={t('Query parameters')}
                    body={
                      Object.keys(detail.query || {}).length
                        ? JSON.stringify(detail.query, null, 2)
                        : ''
                    }
                    encoding='json'
                    contentType='application/json'
                    emptyText={t('No query parameters')}
                    compact
                  />
                  <ContentPanel
                    title={t('Request headers')}
                    body={
                      Object.keys(detail.request_headers || {}).length
                        ? JSON.stringify(detail.request_headers, null, 2)
                        : ''
                    }
                    encoding='json'
                    contentType='application/json'
                    emptyText={t('No request headers were recorded')}
                    compact
                  />
                  <ContentPanel
                    title={t('Raw request body')}
                    body={detail.request_body}
                    encoding={detail.request_encoding}
                    contentType={detail.request_content_type}
                    emptyText={t('Empty request body')}
                  />
                </>
              ) : (
                <ContentPanel
                  title={t('Request text')}
                  body={requestText}
                  encoding='utf-8'
                  contentType='text/plain'
                  emptyText={t('No readable request text found')}
                />
              )}
            </TabsContent>
            <TabsContent value='response' className='mt-3 space-y-4'>
              {showRaw ? (
                <>
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
                  />
                  <ContentPanel
                    title={t('Raw response body')}
                    body={detail.response_body}
                    encoding={detail.response_encoding}
                    contentType={detail.response_content_type}
                    emptyText={t('Empty response body')}
                    truncated={detail.response_body_truncated}
                    totalBytes={detail.response_body_total_bytes}
                  />
                </>
              ) : (
                <ContentPanel
                  title={t('Model response text')}
                  body={responseText}
                  encoding='utf-8'
                  contentType='text/plain'
                  emptyText={t('No readable response text found')}
                  truncated={detail.response_body_truncated}
                  totalBytes={detail.response_body_total_bytes}
                />
              )}
            </TabsContent>
          </Tabs>
        </div>
      )}
    </Dialog>
  )
}
