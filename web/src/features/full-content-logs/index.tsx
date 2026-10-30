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
import {
  Database01Icon,
  InboxIcon,
  Refresh01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { StatTile } from '@/components/stat-tile'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useMediaQuery } from '@/hooks'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { getFullContentLogs } from './api'
import { FullContentLogDetailsDialog } from './components/full-content-log-details-dialog'
import { FullContentLogFilesDialog } from './components/full-content-log-files-dialog'
import { FullContentLogFilterBar } from './components/full-content-log-filter-bar'
import { FullContentLogPagination } from './components/full-content-log-pagination'
import {
  FullContentLogMobileCard,
  FullContentLogRow,
} from './components/full-content-log-row'
import { formatLogBytes } from './lib/format'
import {
  createFullContentLogListState,
  EMPTY_FULL_CONTENT_FILTERS,
  getFullContentPageCount,
  type FullContentLogListState,
} from './lib/list-state'
import { getFullContentLogsQueryKey } from './lib/query-key'
import type { FullContentLogFilters } from './types'

const SKELETON_ROWS = [
  'skeleton-1',
  'skeleton-2',
  'skeleton-3',
  'skeleton-4',
  'skeleton-5',
  'skeleton-6',
] as const
// Fixed columns; the model and endpoint take the rest, so the table fits the
// content column from 1024px without scrolling sideways.
const COLUMN_WIDTH = {
  time: 'w-48',
  key: 'w-36',
  status: 'w-20',
  bytes: 'w-40',
  duration: 'w-24',
  actions: 'w-36',
} as const

interface FullContentLogsProps {
  initialState?: FullContentLogListState
  onStateChange?: (state: FullContentLogListState) => void
}

export function FullContentLogs(props: FullContentLogsProps) {
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const canRead = useAuthStore((state) =>
    state.auth.user !== null && state.auth.user.role >= ROLE.ADMIN
  )
  return (
    <FullContentLogsContent
      key={JSON.stringify([userId, sessionId, canRead])}
      initialState={props.initialState}
      onStateChange={props.onStateChange}
    />
  )
}

function FullContentLogsContent(props: FullContentLogsProps) {
  const { t } = useTranslation()
  const isMobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const role = useAuthStore((state) => state.auth.user?.role ?? 0)
  const canRead = userId !== null && role >= ROLE.ADMIN
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const [listState, setListState] = useState(
    () => props.initialState ?? createFullContentLogListState()
  )
  const { page, pageSize, filters: appliedFilters } = listState
  const [draftFilters, setDraftFilters] = useState<FullContentLogFilters>(
    () => ({ ...listState.filters })
  )
  const [detailRequestId, setDetailRequestId] = useState<string | null>(null)
  const [filesOpen, setFilesOpen] = useState(false)

  const logsQuery = useQuery({
    queryKey: getFullContentLogsQueryKey(
      page,
      appliedFilters,
      userId,
      sessionId,
      pageSize
    ),
    enabled: canRead,
    queryFn: async ({ signal }) => {
      const response = await getFullContentLogs(
        {
          page,
          pageSize,
          filters: appliedFilters,
        },
        signal
      )
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Failed to load full content logs')
        )
      }
      return response.data
    },
    retry: false,
  })

  const data = canRead && !logsQuery.isError ? logsQuery.data : undefined
  const totalPages = getFullContentPageCount(data?.total, pageSize)
  const matchingTotal = totalPages === null ? undefined : data?.total
  const { onStateChange } = props
  useEffect(() => {
    onStateChange?.(listState)
  }, [listState, onStateChange])
  useEffect(() => {
    if (totalPages !== null && page > totalPages) {
      setListState((current) => ({ ...current, page: totalPages }))
    }
  }, [page, totalPages])
  const errorMessage =
    logsQuery.error instanceof Error
      ? logsQuery.error.message
      : t('Failed to load full content logs')
  const isEmpty =
    data !== undefined && !logsQuery.isLoading && data.items.length === 0

  const applyFilters = () => {
    setListState((current) => ({
      ...current,
      page: 1,
      filters: { ...draftFilters },
    }))
  }

  const resetFilters = () => {
    setDraftFilters({ ...EMPTY_FULL_CONTENT_FILTERS })
    setListState((current) => ({
      ...current,
      page: 1,
      filters: { ...EMPTY_FULL_CONTENT_FILTERS },
    }))
  }

  let mobileList: ReactNode = null
  if (logsQuery.isLoading) {
    mobileList = (
      <div className='bg-card flex flex-col gap-2 rounded-xl border p-3'>
        {SKELETON_ROWS.slice(0, 3).map((row) => (
          <Skeleton key={row} className='h-14 w-full' />
        ))}
      </div>
    )
  } else if (logsQuery.isError) {
    mobileList = (
      <Alert variant='destructive'>
        <AlertDescription>{errorMessage}</AlertDescription>
      </Alert>
    )
  } else if (isEmpty) {
    mobileList = (
      <Empty className='bg-card rounded-xl border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={InboxIcon} aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('No matching full content logs')}</EmptyTitle>
        </EmptyHeader>
      </Empty>
    )
  } else if (data) {
    mobileList = (
      <ul
        role='list'
        className='bg-card flex min-w-0 flex-col divide-y overflow-hidden rounded-xl border'
      >
        {data.items.map((log) => (
          <li key={log.request_id}>
            <FullContentLogMobileCard log={log} onView={setDetailRequestId} />
          </li>
        ))}
      </ul>
    )
  }

  return (
    <>
      <SectionPageLayout fixedContent={!isMobile}>
        <SectionPageLayout.Title>
          {t('Request bodies & SSE')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button variant='outline' onClick={() => setFilesOpen(true)}>
            <HugeiconsIcon
              icon={Database01Icon}
              data-icon='inline-start'
              aria-hidden='true'
            />
            {t('Raw log storage')}
          </Button>
          <Button
            variant='outline'
            disabled={logsQuery.isFetching}
            onClick={() => void logsQuery.refetch()}
          >
            <HugeiconsIcon
              icon={Refresh01Icon}
              data-icon='inline-start'
              aria-hidden='true'
              className={logsQuery.isFetching ? 'animate-spin' : undefined}
            />
            {t('Refresh')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div
            className={
              isMobile
                ? 'flex min-h-full flex-col gap-3'
                : 'flex h-full min-h-0 flex-col gap-4'
            }
          >
            <p className='text-muted-foreground text-sm'>
              {t(
                'Saved request and response bodies, including streaming events. Opening a record never resends it.'
              )}
            </p>

            {!canRead && (
              <Alert>
                <AlertDescription>
                  {t('Request content is available to administrators only.')}
                </AlertDescription>
              </Alert>
            )}
            {data?.enabled && (
              <Alert role='status'>
                <AlertDescription>
                  {t(
                    'Body capture is enabled for new requests. Earlier content cannot be recovered.'
                  )}
                </AlertDescription>
              </Alert>
            )}
            {data && !data.enabled && (
              <Alert
                role='status'
                className='border-warning/40 bg-warning-muted text-foreground'
              >
                <AlertDescription className='text-foreground'>
                  {t(
                    'Full content logging is disabled. Existing files remain available.'
                  )}
                </AlertDescription>
              </Alert>
            )}

            <dl className='grid min-w-0 grid-cols-1 gap-2 min-[360px]:grid-cols-3 sm:gap-3'>
              <StatTile
                label={t('Matching Requests')}
                value={matchingTotal ?? '—'}
                loading={canRead && (logsQuery.isLoading || logsQuery.isFetching)}
              />
              <StatTile
                label={t('Log Files')}
                value={data?.files.count ?? '—'}
                loading={canRead && (logsQuery.isLoading || logsQuery.isFetching)}
              />
              <StatTile
                label={t('Disk Usage')}
                value={data ? formatLogBytes(data.files.total_size) : '—'}
                loading={canRead && (logsQuery.isLoading || logsQuery.isFetching)}
              />
            </dl>

            <FullContentLogFilterBar
              filters={draftFilters}
              onChange={setDraftFilters}
              onApply={applyFilters}
              onReset={resetFilters}
              modelOptions={data?.facets.models ?? []}
              tokenOptions={data?.facets.tokens ?? []}
            />

            {isMobile ? (
              mobileList
            ) : (
              <div className='bg-card min-h-0 flex-1 overflow-auto rounded-xl border'>
                <Table className='table-fixed'>
                  <TableHeader className='bg-muted sticky top-0 z-10'>
                    <TableRow>
                      <TableHead className={COLUMN_WIDTH.time}>
                        {t('Time')}
                      </TableHead>
                      <TableHead>{t('Model / Endpoint')}</TableHead>
                      <TableHead className={COLUMN_WIDTH.key}>
                        {t('API Key')}
                      </TableHead>
                      <TableHead className={COLUMN_WIDTH.status}>
                        {t('Status')}
                      </TableHead>
                      <TableHead className={COLUMN_WIDTH.bytes}>
                        {t('Request / Response')}
                      </TableHead>
                      <TableHead className={COLUMN_WIDTH.duration}>
                        {t('Duration')}
                      </TableHead>
                      <TableHead
                        className={cn(COLUMN_WIDTH.actions, 'text-right')}
                      >
                        {t('Actions')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {logsQuery.isLoading &&
                      SKELETON_ROWS.map((row) => (
                        <TableRow key={row}>
                          <TableCell colSpan={7}>
                            <Skeleton className='h-8 w-full' />
                          </TableCell>
                        </TableRow>
                      ))}

                    {logsQuery.isError && (
                      <TableRow>
                        <TableCell
                          colSpan={7}
                          className='text-destructive h-32 text-center whitespace-normal'
                        >
                          {errorMessage}
                        </TableCell>
                      </TableRow>
                    )}

                    {isEmpty && (
                      <TableRow>
                        <TableCell
                          colSpan={7}
                          className='text-muted-foreground h-32 text-center whitespace-normal'
                        >
                          {t('No matching full content logs')}
                        </TableCell>
                      </TableRow>
                    )}

                    {!logsQuery.isError &&
                      data?.items.map((log) => (
                        <FullContentLogRow
                          key={log.request_id}
                          log={log}
                          onView={setDetailRequestId}
                        />
                      ))}
                  </TableBody>
                </Table>
              </div>
            )}

            <FullContentLogPagination
              key={`${page}:${pageSize}`}
              page={page}
              pageSize={pageSize}
              total={data?.total}
              busy={logsQuery.isFetching || logsQuery.isLoading}
              onPageChange={(nextPage) => {
                if (totalPages === null || !Number.isSafeInteger(nextPage)) return
                setListState((current) => ({
                  ...current,
                  page: Math.min(totalPages, Math.max(1, nextPage)),
                }))
              }}
              onPageSizeChange={(nextPageSize) =>
                setListState((current) => ({
                  ...current,
                  page: 1,
                  pageSize: nextPageSize,
                }))
              }
            />
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <FullContentLogDetailsDialog
        key={detailRequestId ?? 'closed'}
        requestId={detailRequestId}
        open={Boolean(detailRequestId)}
        onOpenChange={(open) => !open && setDetailRequestId(null)}
      />
      <FullContentLogFilesDialog open={filesOpen} onOpenChange={setFilesOpen} />
    </>
  )
}
