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

import { useQuery } from '@tanstack/react-query'
import { Database, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
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

export function FullContentLogs() {
  const { t } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
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
    queryFn: async () => {
      const response = await getFullContentLogs({
        page,
        pageSize: PAGE_SIZE,
        filters: appliedFilters,
      })
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Failed to load full content logs')
        )
      }
      return response.data
    },
    placeholderData: (previousData) => previousData,
    retry: false,
  })

  const data = logsQuery.data
  const totalPages = Math.max(1, Math.ceil((data?.total ?? 0) / PAGE_SIZE))

  const applyFilters = () => {
    setPage(1)
    setAppliedFilters({ ...draftFilters })
  }

  const resetFilters = () => {
    setPage(1)
    setDraftFilters(EMPTY_FILTERS)
    setAppliedFilters(EMPTY_FILTERS)
  }

  return (
    <>
      <SectionPageLayout fixedContent={!isMobile}>
        <SectionPageLayout.Title>
          {t('API Request Logs')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button variant='outline' onClick={() => setFilesOpen(true)}>
            <Database />
            {t('Raw log storage')}
          </Button>
          <Button
            variant='outline'
            disabled={logsQuery.isFetching}
            onClick={() => void logsQuery.refetch()}
          >
            <RefreshCw className={logsQuery.isFetching ? 'animate-spin' : ''} />
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
            <div className='bg-muted/30 rounded-lg border p-3 text-sm'>
              <div className='font-medium'>{t('Online request explorer')}</div>
              <div className='text-muted-foreground mt-1'>
                {t(
                  'Filter by API key, time, model or request ID, then click any row to inspect the complete request and response online.'
                )}
              </div>
            </div>

            {data && !data.enabled && (
              <div className='border-warning/40 bg-warning/10 text-warning rounded-lg border p-3 text-sm'>
                {t(
                  'Full content logging is disabled. Existing files remain available.'
                )}
              </div>
            )}

            <div className='grid gap-3 sm:grid-cols-3'>
              <div className='rounded-lg border p-3'>
                <div className='text-muted-foreground text-xs'>
                  {t('Matching Requests')}
                </div>
                <div className='mt-1 text-xl font-semibold tabular-nums'>
                  {data?.total ?? 0}
                </div>
              </div>
              <div className='rounded-lg border p-3'>
                <div className='text-muted-foreground text-xs'>
                  {t('Log Files')}
                </div>
                <div className='mt-1 text-xl font-semibold tabular-nums'>
                  {data?.files.count ?? 0}
                </div>
              </div>
              <div className='rounded-lg border p-3'>
                <div className='text-muted-foreground text-xs'>
                  {t('Disk Usage')}
                </div>
                <div className='mt-1 text-xl font-semibold tabular-nums'>
                  {formatLogBytes(data?.files.total_size ?? 0)}
                </div>
              </div>
            </div>

            <FullContentLogFilterBar
              filters={draftFilters}
              onChange={setDraftFilters}
              onApply={applyFilters}
              onReset={resetFilters}
              modelOptions={data?.facets.models ?? []}
              tokenOptions={data?.facets.tokens ?? []}
            />

            {isMobile ? (
              <div className='space-y-3'>
                {logsQuery.isLoading &&
                  SKELETON_ROWS.slice(0, 3).map((row) => (
                    <div
                      key={row}
                      className='rounded-lg border p-3'
                      aria-label={t('Loading')}
                    >
                      <Skeleton className='h-24 w-full' />
                    </div>
                  ))}
                {logsQuery.isError && (
                  <div className='text-destructive rounded-lg border p-6 text-center text-sm'>
                    {logsQuery.error instanceof Error
                      ? logsQuery.error.message
                      : t('Failed to load full content logs')}
                  </div>
                )}
                {!logsQuery.isLoading &&
                  !logsQuery.isError &&
                  (data?.items.length ?? 0) === 0 && (
                    <div className='text-muted-foreground rounded-lg border p-8 text-center text-sm'>
                      {t('No matching full content logs')}
                    </div>
                  )}
                {!logsQuery.isError &&
                  data?.items.map((log) => (
                    <FullContentLogMobileCard
                      key={log.request_id}
                      log={log}
                      onView={setDetailRequestId}
                    />
                  ))}
              </div>
            ) : (
              <div className='min-h-0 flex-1 overflow-auto rounded-lg border'>
                <Table className='min-w-[1120px]'>
                  <TableHeader className='bg-muted/40 sticky top-0 z-10'>
                    <TableRow>
                      <TableHead>{t('Time')}</TableHead>
                      <TableHead>{t('Model / Endpoint')}</TableHead>
                      <TableHead>{t('API Key')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead>{t('Request / Response')}</TableHead>
                      <TableHead>{t('Duration')}</TableHead>
                      <TableHead>{t('Request ID')}</TableHead>
                      <TableHead className='text-right'>
                        {t('Actions')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {logsQuery.isLoading &&
                      SKELETON_ROWS.map((row) => (
                        <TableRow key={row}>
                          <TableCell colSpan={8}>
                            <Skeleton className='h-8 w-full' />
                          </TableCell>
                        </TableRow>
                      ))}

                    {logsQuery.isError && (
                      <TableRow>
                        <TableCell
                          colSpan={8}
                          className='text-destructive h-32 text-center'
                        >
                          {logsQuery.error instanceof Error
                            ? logsQuery.error.message
                            : t('Failed to load full content logs')}
                        </TableCell>
                      </TableRow>
                    )}

                    {!logsQuery.isLoading &&
                      !logsQuery.isError &&
                      (data?.items.length ?? 0) === 0 && (
                        <TableRow>
                          <TableCell
                            colSpan={8}
                            className='text-muted-foreground h-32 text-center'
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
