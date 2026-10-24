/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { ArrowRight01Icon, InboxIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  RequestDuration,
  RequestOutcomeBadge,
  RequestTime,
} from '@/features/usage-logs/components/request-summary'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import { getRequestFacts } from '@/features/usage-logs/lib/request-facts'
import { useMediaQuery } from '@/hooks/use-media-query'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { linkButtonClassName } from '@/lib/link-button'

import type { OverviewData } from './use-overview-data'

const DESKTOP_ROWS = 8
const MOBILE_ROWS = 5

/** Opens the request's details directly (same contract as command search). */
function RequestLink(props: {
  log: UsageLog
  className?: string
  children: ReactNode
}) {
  if (!props.log.request_id) {
    return <span className={props.className}>{props.children}</span>
  }
  return (
    <Link
      to='/usage-logs/$section'
      params={{ section: 'common' }}
      search={{ requestId: props.log.request_id, inspect: 'request' }}
      className={props.className}
    >
      {props.children}
    </Link>
  )
}

/**
 * Latest requests (design 8.1): time, result, Key, model, connection
 * (administrators) and duration. Content is never shown here. The "failed"
 * link opens the call records filtered to errors of the last 24 hours.
 */
export function OverviewRecentRequests(props: { data: OverviewData }) {
  const { t } = useTranslation()
  const mobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const showLogs = useIsSidebarModuleVisible('/usage-logs/common')
  const query = props.data.sample
  const rows = query.isError ? undefined : query.data
  const failed =
    rows?.filter((row) => !getRequestFacts(row).completed).length ?? 0
  const shown = rows?.slice(0, mobile ? MOBILE_ROWS : DESKTOP_ROWS) ?? []
  const showConnection = props.data.isAdmin

  let body
  if (query.isPending) {
    body = (
      <div className='flex flex-col gap-2' aria-busy='true'>
        <span className='sr-only'>{t('Loading...')}</span>
        {[1, 2, 3].map((row) => (
          <Skeleton key={row} className='h-10 w-full' />
        ))}
      </div>
    )
  } else if (query.isError) {
    body = (
      <Alert variant='destructive'>
        <AlertTitle>{t('Unable to load recent requests')}</AlertTitle>
        <AlertDescription>
          <Button
            variant='outline'
            size='sm'
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else if (shown.length === 0) {
    body = (
      <Empty className='border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={InboxIcon} aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('No requests in the last 24 hours')}</EmptyTitle>
          <EmptyDescription>
            {t('Requests appear here once a client uses one of your Keys.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  } else if (mobile) {
    body = (
      <ul className='flex flex-col divide-y rounded-lg border'>
        {shown.map((row) => (
          <li key={row.id}>
            <RequestLink
              log={row}
              className='hover:bg-muted flex min-h-14 min-w-0 items-center gap-3 px-3 py-2'
            >
              <span className='flex min-w-0 flex-1 flex-col gap-1'>
                <span className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1'>
                  <RequestOutcomeBadge log={row} />
                  <span className='text-data text-muted-foreground'>
                    <RequestTime seconds={row.created_at} />
                  </span>
                </span>
                <span className='text-data min-w-0 wrap-anywhere'>
                  {row.token_name || t('Unknown Key')} ·{' '}
                  {row.model_name || t('Unknown model')} ·{' '}
                  <RequestDuration seconds={row.use_time} />
                </span>
              </span>
              {row.request_id && (
                <HugeiconsIcon
                  icon={ArrowRight01Icon}
                  className='text-muted-foreground size-4 shrink-0'
                  aria-hidden='true'
                />
              )}
            </RequestLink>
          </li>
        ))}
      </ul>
    )
  } else {
    body = (
      <div className='min-w-0 overflow-x-auto rounded-lg border'>
        <Table className='text-data'>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Time')}</TableHead>
              <TableHead>{t('Result')}</TableHead>
              <TableHead>{t('API Key')}</TableHead>
              <TableHead>{t('Model')}</TableHead>
              {showConnection && <TableHead>{t('Connection')}</TableHead>}
              <TableHead className='text-right'>{t('Duration')}</TableHead>
            </TableRow>
          </TableHeader>
          {/* Single-line rows: the overview's first screen at 1280x800
              holds several latest requests below health and windows. */}
          <TableBody className='[&>tr]:h-11'>
            {shown.map((row) => (
              <TableRow key={row.id}>
                <TableCell>
                  <RequestLink
                    log={row}
                    className='text-primary underline-offset-4 hover:underline'
                  >
                    <RequestTime seconds={row.created_at} />
                  </RequestLink>
                </TableCell>
                <TableCell>
                  <RequestOutcomeBadge log={row} />
                </TableCell>
                <TableCell className='max-w-48 truncate' title={row.token_name}>
                  {row.token_name || t('Unknown Key')}
                </TableCell>
                <TableCell className='max-w-48 truncate' title={row.model_name}>
                  {row.model_name || t('Unknown model')}
                </TableCell>
                {showConnection && (
                  <TableCell
                    className='max-w-48 truncate'
                    title={row.channel_name ?? ''}
                  >
                    {row.channel_name ||
                      (row.channel ? `#${row.channel}` : t('Unknown'))}
                  </TableCell>
                )}
                <TableCell className='text-right'>
                  <RequestDuration seconds={row.use_time} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    )
  }

  return (
    <section
      aria-labelledby='overview-recent-title'
      className='bg-card p-card flex min-w-0 flex-col gap-3 rounded-xl border'
    >
      <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'>
        <h2 id='overview-recent-title' className='text-title'>
          {t('Recent requests')}
        </h2>
        {showLogs && (
          <div className='flex flex-wrap items-center gap-1'>
            {failed > 0 && (
              <Link
                to='/usage-logs/$section'
                params={{ section: 'common' }}
                search={{
                  type: ['5'],
                  startTime: props.data.since * 1000,
                  endTime: Date.now() + 3600 * 1000,
                }}
                data-myapi-button=''
                className={linkButtonClassName({ variant: 'ghost' })}
              >
                {t('Failed {{count}}', { count: failed })}
                <HugeiconsIcon
                  icon={ArrowRight01Icon}
                  data-icon='inline-end'
                  aria-hidden='true'
                />
              </Link>
            )}
            <Link
              to='/usage-logs/$section'
              params={{ section: 'common' }}
              data-myapi-button=''
              className={linkButtonClassName({ variant: 'ghost' })}
            >
              {t('View call records')}
            </Link>
          </div>
        )}
      </div>
      {body}
    </section>
  )
}
