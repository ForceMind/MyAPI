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

import { Eye } from 'lucide-react'
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
      <TableCell className='whitespace-nowrap'>
        {new Date(props.log.timestamp).toLocaleString()}
      </TableCell>
      <TableCell>
        <div className='font-medium'>{props.log.model || '-'}</div>
        <div className='text-muted-foreground mt-0.5 font-mono text-xs'>
          {props.log.method} {props.log.path}
        </div>
      </TableCell>
      <TableCell>
        <div>{props.log.token_name || '-'}</div>
        <div className='text-muted-foreground text-xs'>
          {props.log.token_id ? `#${props.log.token_id}` : '-'}
        </div>
      </TableCell>
      <TableCell>
        <Badge variant={statusVariant(props.log.status)}>
          {props.log.status || t('Pending')}
        </Badge>
      </TableCell>
      <TableCell className='font-mono text-xs whitespace-nowrap'>
        {formatLogBytes(props.log.request_bytes)} →{' '}
        {formatLogBytes(props.log.response_bytes)}
        <div className='text-muted-foreground mt-0.5'>
          {t('{{count}} chunks', { count: props.log.chunk_count })}
        </div>
      </TableCell>
      <TableCell className='whitespace-nowrap'>
        {props.log.duration_ms} ms
      </TableCell>
      <TableCell className='max-w-64 truncate font-mono text-xs'>
        {props.log.request_id}
      </TableCell>
      <TableCell className='bg-background sticky right-0 text-right group-hover:bg-[color-mix(in_oklch,var(--muted)_50%,var(--background))]'>
        <Button
          size='sm'
          variant='outline'
          onClick={(event) => {
            event.stopPropagation()
            props.onView(props.log.request_id)
          }}
        >
          <Eye />
          {t('View content')}
        </Button>
      </TableCell>
    </TableRow>
  )
}
