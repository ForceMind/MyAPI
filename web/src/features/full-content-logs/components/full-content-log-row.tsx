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

import { ArrowRight01Icon, FileSearchIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { TableCell, TableRow } from '@/components/ui/table'

import { formatLogBytes } from '../lib/format'
import type { FullContentLogSummary } from '../types'

interface FullContentLogRowProps {
  log: FullContentLogSummary
  onView: (requestId: string) => void
}

function statusVariant(
  status: number
): 'secondary' | 'destructive' | 'outline' {
  if (status >= 200 && status < 400) return 'secondary'
  if (status >= 400) return 'destructive'
  return 'outline'
}

export function FullContentLogRow(props: FullContentLogRowProps) {
  const { t } = useTranslation()

  return (
    <TableRow
      className='cursor-pointer'
      onClick={() => props.onView(props.log.request_id)}
    >
      <TableCell>
        <div className='truncate tabular-nums'>
          {new Date(props.log.timestamp).toLocaleString()}
        </div>
        <div
          className='text-muted-foreground mt-0.5 truncate font-mono'
          title={props.log.request_id}
        >
          {props.log.request_id}
        </div>
      </TableCell>
      <TableCell>
        <div className='truncate font-medium'>{props.log.model || '-'}</div>
        <div className='text-muted-foreground mt-0.5 truncate font-mono'>
          {props.log.method} {props.log.path}
        </div>
      </TableCell>
      <TableCell>
        <div className='truncate'>{props.log.token_name || '-'}</div>
        <div className='text-muted-foreground'>
          {props.log.token_id ? `#${props.log.token_id}` : '-'}
        </div>
      </TableCell>
      <TableCell>
        <Badge variant={statusVariant(props.log.status)}>
          {props.log.status || t('Pending')}
        </Badge>
      </TableCell>
      <TableCell className='font-mono whitespace-nowrap'>
        {formatLogBytes(props.log.request_bytes)} →{' '}
        {formatLogBytes(props.log.response_bytes)}
        <div className='text-muted-foreground mt-0.5'>
          {t('{{count}} chunks', { count: props.log.chunk_count })}
        </div>
      </TableCell>
      <TableCell className='whitespace-nowrap tabular-nums'>
        {props.log.duration_ms} ms
      </TableCell>
      <TableCell className='text-right'>
        <Button
          size='sm'
          variant='outline'
          onClick={(event) => {
            event.stopPropagation()
            props.onView(props.log.request_id)
          }}
        >
          <HugeiconsIcon
            icon={FileSearchIcon}
            data-icon='inline-start'
            aria-hidden='true'
          />
          {t('View content')}
        </Button>
      </TableCell>
    </TableRow>
  )
}

/**
 * A saved request on a phone, in the same two lines as a call record: time,
 * status and Request ID; then model, endpoint, sizes and duration. The whole
 * row opens the saved content.
 */
export function FullContentLogMobileCard(props: FullContentLogRowProps) {
  const { t } = useTranslation()
  const summaryId = useId()
  const log = props.log

  return (
    <button
      type='button'
      aria-label={t('View content')}
      aria-describedby={summaryId}
      onClick={() => props.onView(log.request_id)}
      className='hover:bg-muted flex min-h-12 w-full min-w-0 items-center gap-3 px-4 py-2.5 text-left focus-visible:outline-2 focus-visible:-outline-offset-2'
    >
      <span id={summaryId} className='flex min-w-0 flex-1 flex-col gap-1'>
        <span className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5'>
          <span className='text-muted-foreground shrink-0 font-mono text-xs tabular-nums'>
            {new Date(log.timestamp).toLocaleString()}
          </span>
          <Badge className='shrink-0' variant={statusVariant(log.status)}>
            {log.status || t('Pending')}
          </Badge>
          <span className='text-muted-foreground min-w-0 truncate font-mono text-xs'>
            {log.request_id}
          </span>
        </span>
        <span className='text-muted-foreground flex min-w-0 flex-wrap items-baseline gap-x-1.5 text-xs'>
          <span className='text-foreground min-w-0 font-mono wrap-anywhere'>
            {log.model || '-'}
          </span>
          <span aria-hidden='true'>·</span>
          <span className='min-w-0 font-mono wrap-anywhere'>
            {log.method} {log.path}
          </span>
          <span aria-hidden='true'>·</span>
          <span className='font-mono whitespace-nowrap'>
            {formatLogBytes(log.request_bytes)} →{' '}
            {formatLogBytes(log.response_bytes)}
          </span>
          <span aria-hidden='true'>·</span>
          <span className='whitespace-nowrap tabular-nums'>
            {log.duration_ms} ms
          </span>
          {log.token_name || log.token_id ? (
            <>
              <span aria-hidden='true'>·</span>
              <span className='min-w-0 wrap-anywhere'>
                {log.token_name || `#${log.token_id}`}
              </span>
            </>
          ) : null}
        </span>
      </span>
      <HugeiconsIcon
        icon={ArrowRight01Icon}
        className='text-muted-foreground size-4 shrink-0'
        aria-hidden='true'
      />
    </button>
  )
}
