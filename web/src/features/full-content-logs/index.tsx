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
import { useState, type ReactNode } from 'react'
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
import {
  FullContentLogMobileCard,
  FullContentLogRow,
} from './components/full-content-log-row'
import { formatLogBytes } from './lib/format'
import { getFullContentLogsQueryKey } from './lib/query-key'
import type { FullContentLogFilters } from './types'

const PAGE_SIZE = 20
const SKELETON_ROWS = [
  'skeleton-1',
  'skeleton-2',
  'skeleton-3',
  'skeleton-4',
  'skeleton-5',
  'skeleton-6',
] as const
const EMPTY_FILTERS: FullContentLogFilters = {
  model: '',
  token: '',
  requestId: '',
  startTime: '',
  endTime: '',
}
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

export function FullContentLogs() {
  const { t } = useTranslation()
  const isMobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const role = useAuthStore((state) => state.auth.user?.role ?? 0)
  const canRead = userId !== null && role >= ROLE.ADMIN
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const [page, setPage] = useState(1)
  const [draftFilters, setDraftFilters] =
    useState<FullContentLogFilters>(EMPTY_FILTERS)
  const [appliedFilters, setAppliedFilters] =
    useState<FullContentLogFilters>(EMPTY_FILTERS)
  const [detailRequestId, setDetailRequestId] = useState<string | null>(null)
  const [filesOpen, setFilesOpen] = useState(false)

  const logsQuery = useQuery({
    queryKey: getFullContentLogsQueryKey(
      page,
      appliedFilters,
      userId,
      sessionId
    ),
    enabled: canRead,
    queryFn: async ({ signal }) => {
      const response = await getFullContentLogs(
        {
          page,
          pageSize: PAGE_SIZE,
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
  const totalPages = Math.max(1, Math.ceil((data?.total ?? 0) / PAGE_SIZE))
  const errorMessage =
    logsQuery.error instanceof Error
      ? logsQuery.error.message
      : t('Failed to load full content logs')
  const isEmpty =
    !logsQuery.isLoading && !logsQuery.isError && (data?.items.length ?? 0) === 0

  const applyFilters = () => {
    setPage(1)
    setAppliedFilters({ ...draftFilters })
  }

  const resetFilters = () => {
    setPage(1)
    setDraftFilters(EMPTY_FILTERS)
    setAppliedFilters(EMPTY_FILTERS)
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
                value={data?.total ?? '—'}
                loading={canRead && logsQuery.isLoading}
              />
              <StatTile
                label={t('Log Files')}
                value={data?.files.count ?? '—'}
                loading={canRead && logsQuery.isLoading}
              />
              <StatTile
                label={t('Disk Usage')}
                value={data ? formatLogBytes(data.files.total_size) : '—'}
                loading={canRead && logsQuery.isLoading}
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

            <div className='flex flex-col gap-2 text-sm sm:flex-row sm:items-center sm:justify-between'>
              <span className='text-muted-foreground'>
                {t('Page {{page}} of {{totalPages}} · {{total}} requests', {
                  page,
                  totalPages,
                  total: data?.total ?? 0,
                })}
              </span>
              <div className='flex gap-2'>
                <Button
                  variant='outline'
                  disabled={page <= 1 || logsQuery.isFetching}
                  onClick={() => setPage((current) => Math.max(1, current - 1))}
                >
                  {t('Previous')}
                </Button>
                <Button
                  variant='outline'
                  disabled={page >= totalPages || logsQuery.isFetching}
                  onClick={() => setPage((current) => current + 1)}
                >
                  {t('Next')}
                </Button>
              </div>
            </div>
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
