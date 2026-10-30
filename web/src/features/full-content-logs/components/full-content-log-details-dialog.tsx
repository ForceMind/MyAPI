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
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useMediaQuery } from '@/hooks/use-media-query'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'

import { useFullContentLogDetail } from '../hooks/use-full-content-log-detail'
import { formatLogBytes } from '../lib/format'
import { SavedContentRenderer } from './saved-content-renderer'

// Retain the established import path for request-page and legacy consumers.
export { ContentPanel } from './content-panel'

interface FullContentLogDetailsDialogProps {
  requestId: string | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function FullContentLogDetailsDialog(
  props: FullContentLogDetailsDialogProps
) {
  const { t } = useTranslation()
  const isMobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const { detailQuery, detail, contentError, canRead } =
    useFullContentLogDetail(props.requestId, props.open)

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
      {!props.requestId && (
        <p>{t('No Request ID on this record')}</p>
      )}
      {props.requestId && !canRead && (
        <p role='alert'>
          {t('Request content is available to administrators only.')}
        </p>
      )}
      {props.requestId && canRead && detailQuery.isLoading && (
        <div className='space-y-3' aria-label={t('Loading')}>
          <Skeleton className='h-16 w-full' />
          <Skeleton className='h-80 w-full' />
        </div>
      )}

      {props.requestId && canRead && detailQuery.isError && (
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

      {props.requestId && canRead && detail && !detailQuery.isError && (
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
              <div className='mt-1 font-medium'>
                {Number.isFinite(detail.duration_ms) ? `${detail.duration_ms} ms` : t('Not recorded')}
              </div>
            </div>
            <div>
              <span className='text-muted-foreground'>{t('Size')}</span>
              <div className='mt-1 font-medium'>
                {Number.isFinite(detail.request_bytes) ? formatLogBytes(detail.request_bytes) : t('Not recorded')} →{' '}
                {Number.isFinite(detail.response_bytes) ? formatLogBytes(detail.response_bytes) : t('Not recorded')}
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

          <SavedContentRenderer key={detail.request_id} detail={detail} tabbed />
        </div>
      )}
    </Dialog>
  )
}
