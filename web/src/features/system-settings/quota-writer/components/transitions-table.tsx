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
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatTimestampToDate } from '@/lib/format'

import {
  QUOTA_WRITER_TRANSITION_STATUS_LABEL_KEYS,
  QUOTA_WRITER_TRANSITION_STATUS_VARIANTS,
  translateQuotaWriterFailureReason,
} from '../constants'
import { useQuotaWriterTransitions } from '../hooks/use-quota-writer'
import type { QuotaWriterTransition } from '../types'

const PAGE_SIZE = 10

export function TransitionsTable() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [expandedId, setExpandedId] = useState<number | null>(null)
  const query = useQuotaWriterTransitions(page, PAGE_SIZE)

  const items = query.data?.items ?? []
  const total = query.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const toggleExpanded = (id: number) => {
    setExpandedId((current) => (current === id ? null : id))
  }

  const renderDetails = (item: QuotaWriterTransition) => (
    <div className='flex flex-col gap-2 text-sm'>
      {item.failure_reason && (
        <div>
          <span className='text-muted-foreground'>{t('Failure reason')}: </span>
          <span className='text-destructive'>
            {translateQuotaWriterFailureReason(item.failure_reason)}
          </span>
        </div>
      )}
      {item.ack_note && (
        <div>
          <span className='text-muted-foreground'>
            {t('Acknowledgement note')}:{' '}
          </span>
          <span>{item.ack_note}</span>
        </div>
      )}
    </div>
  )

  let body
  if (query.isLoading) {
    body = <LoadingState message={t('Loading transitions...')} />
  } else if (query.isError) {
    body = (
      <ErrorState
        description={t('Failed to load transitions')}
        onRetry={() => query.refetch()}
      />
    )
  } else if (items.length === 0) {
    body = (
      <EmptyState
        title={t('No transitions recorded yet')}
        description={t(
          'Every mode switch attempt is persisted here as immutable evidence.'
        )}
      />
    )
  } else {
    body = (
      <>
        <div className='overflow-x-auto rounded-lg border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className='w-10' />
                <TableHead>{t('ID')}</TableHead>
                <TableHead>{t('Operator')}</TableHead>
                <TableHead>{t('Transition')}</TableHead>
                <TableHead>{t('Epoch')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead>{t('Created')}</TableHead>
                <TableHead>{t('Finished')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => {
                const hasDetails = Boolean(item.failure_reason || item.ack_note)
                const expanded = expandedId === item.id
                return [
                  <TableRow key={item.id}>
                    <TableCell>
                      {hasDetails && (
                        <Button
                          variant='ghost'
                          size='icon'
                          className='size-6'
                          aria-expanded={expanded}
                          aria-label={
                            expanded ? t('Hide details') : t('Show details')
                          }
                          onClick={() => toggleExpanded(item.id)}
                        >
                          {expanded ? (
                            <ChevronDown
                              className='size-4'
                              aria-hidden='true'
                            />
                          ) : (
                            <ChevronRight
                              className='size-4'
                              aria-hidden='true'
                            />
                          )}
                        </Button>
                      )}
                    </TableCell>
                    <TableCell>{item.id}</TableCell>
                    <TableCell>{item.operator_user_id}</TableCell>
                    <TableCell className='whitespace-nowrap'>
                      {item.from_mode} → {item.to_mode}
                    </TableCell>
                    <TableCell className='whitespace-nowrap'>
                      {item.from_epoch} → {item.to_epoch}
                    </TableCell>
                    <TableCell>
                      <StatusBadge
                        label={t(
                          QUOTA_WRITER_TRANSITION_STATUS_LABEL_KEYS[item.status]
                        )}
                        variant={
                          QUOTA_WRITER_TRANSITION_STATUS_VARIANTS[item.status]
                        }
                        pulse={item.status === 'applying'}
                        copyable={false}
                      />
                    </TableCell>
                    <TableCell className='whitespace-nowrap'>
                      {formatTimestampToDate(item.created_at)}
                    </TableCell>
                    <TableCell className='whitespace-nowrap'>
                      {formatTimestampToDate(item.finished_at)}
                    </TableCell>
                  </TableRow>,
                  expanded && hasDetails ? (
                    <TableRow key={`${item.id}-details`}>
                      <TableCell colSpan={8} className='bg-muted/30'>
                        {renderDetails(item)}
                      </TableCell>
                    </TableRow>
                  ) : null,
                ]
              })}
            </TableBody>
          </Table>
        </div>
        <div className='flex items-center justify-between gap-2'>
          <span className='text-muted-foreground text-sm'>
            {t('Page {{page}} of {{total}}', {
              page,
              total: totalPages,
            })}
          </span>
          <div className='flex gap-2'>
            <Button
              variant='outline'
              size='sm'
              disabled={page <= 1}
              onClick={() => setPage((current) => Math.max(1, current - 1))}
            >
              {t('Previous')}
            </Button>
            <Button
              variant='outline'
              size='sm'
              disabled={page >= totalPages}
              onClick={() =>
                setPage((current) => Math.min(totalPages, current + 1))
              }
            >
              {t('Next')}
            </Button>
          </div>
        </div>
      </>
    )
  }

  return (
    <div className='flex flex-col gap-4 rounded-lg border p-4'>
      <div className='flex flex-col gap-1'>
        <h4 className='text-sm font-semibold'>{t('Transition history')}</h4>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Immutable evidence of every mode switch attempt, including blocked and failed applies.'
          )}
        </p>
      </div>
      {body}
    </div>
  )
}
