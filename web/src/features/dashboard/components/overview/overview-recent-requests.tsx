/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import {
  ArrowRight01Icon,
  Cancel01Icon,
  InboxIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { useState, type ReactNode, type Ref } from 'react'
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
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
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

import type { FlowSelection } from '../../lib/overview-flow'
import type { OverviewData } from './use-overview-data'

const DESKTOP_ROWS = 8
const MOBILE_ROWS = 5
type Outcome = 'all' | 'failed' | 'usage'
const outcomeItemClassName =
  'h-auto min-h-7 min-w-0 shrink whitespace-normal max-lg:min-h-11'

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
      to='/usage-logs/request/$requestId'
      params={{ requestId: props.log.request_id }}
      className={props.className}
    >
      {props.children}
    </Link>
  )
}

/** Tokens when usage was reported; a completed request without it says so. */
function UsageNote(props: { log: UsageLog }) {
  const { t } = useTranslation()
  const facts = getRequestFacts(props.log)
  if (facts.usageKnown) {
    const tokens = props.log.prompt_tokens + props.log.completion_tokens
    if (tokens <= 0) return null
    return <> · {t('{{count}} Tokens', { count: tokens })}</>
  }
  if (!facts.completed) return null
  return <> · {t('Usage not provided')}</>
}

/**
 * Latest requests under the flow board: time (opens the request), result,
 * Key, model with usage, connection (administrators) and duration. Content
 * is never shown here. A selected flow node filters the rows and shows a
 * clearable chip; the outcome switch narrows to failures or missing usage.
 */
export function OverviewRecentRequests(props: {
  data: OverviewData
  selection: FlowSelection | null
  selectionLabel: string
  onClearSelection: () => void
  ref?: Ref<HTMLElement>
}) {
  const { t } = useTranslation()
  const mobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const showLogs = useIsSidebarModuleVisible('/usage-logs/common')
  const [outcome, setOutcome] = useState<Outcome>('all')
  const query = props.data.sample
  const selection = props.selection
  const rows = (query.isError ? undefined : query.data?.rows)?.filter((row) => {
    if (selection?.kind === 'key' && row.token_id !== selection.id) return false
    if (selection?.kind === 'connection' && row.channel !== selection.id) {
      return false
    }
    const facts = getRequestFacts(row)
    if (outcome === 'failed') return !facts.completed
    if (outcome === 'usage') return facts.completed && !facts.usageKnown
    return true
  })
  const shown = rows?.slice(0, mobile ? MOBILE_ROWS : DESKTOP_ROWS) ?? []
  const showConnection = props.data.isAdmin
  const filtered = selection !== null || outcome !== 'all'
  let recordsSearch = {}
  if (selection?.kind === 'key') recordsSearch = { tokenId: selection.id }
  if (selection?.kind === 'connection') {
    recordsSearch = { channel: String(selection.id) }
  }

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
          <EmptyTitle>
            {filtered
              ? t('No matching requests among the latest ones')
              : t('No requests in the last 24 hours')}
          </EmptyTitle>
          <EmptyDescription>
            {filtered
              ? t('Older requests are in the call records.')
              : t('Requests appear here once a client uses one of your Keys.')}
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
                  {row.model_name || t('Unknown model')}
                  <UsageNote log={row} /> ·{' '}
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
                <TableCell className='max-w-72 truncate' title={row.model_name}>
                  {row.model_name || t('Unknown model')}
                  <span className='text-muted-foreground'>
                    <UsageNote log={row} />
                  </span>
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
      ref={props.ref}
      aria-labelledby='overview-recent-title'
      className='bg-card p-card flex min-w-0 scroll-mt-20 flex-col gap-3 rounded-xl border'
    >
      <div className='flex min-w-0 flex-wrap items-center gap-2'>
        <h2 id='overview-recent-title' className='text-title me-auto'>
          {t('Recent requests')}
        </h2>
        {selection && (
          <span
            data-testid='flow-filter-chip'
            className='bg-primary text-primary-foreground text-data inline-flex h-8 max-w-full min-w-0 items-center gap-1 rounded-full ps-3'
          >
            <span className='truncate'>{props.selectionLabel}</span>
            <Button
              variant='ghost'
              size='icon-sm'
              aria-label={t('Clear filter')}
              onClick={props.onClearSelection}
              className='text-primary-foreground hover:bg-primary-foreground/15 hover:text-primary-foreground rounded-full'
            >
              <HugeiconsIcon icon={Cancel01Icon} aria-hidden='true' />
            </Button>
          </span>
        )}
        <ToggleGroup
          aria-label={t('Request outcome')}
          variant='outline'
          size='sm'
          value={[outcome]}
          onValueChange={(value) => {
            const next = value[0] as Outcome | undefined
            if (next) setOutcome(next)
          }}
          className='max-w-full'
        >
          {/* Long labels wrap inside their segment instead of widening the
              card; phones get 44px targets. */}
          <ToggleGroupItem value='all' className={outcomeItemClassName}>
            {t('All')}
          </ToggleGroupItem>
          <ToggleGroupItem value='failed' className={outcomeItemClassName}>
            {t('Failed')}
          </ToggleGroupItem>
          <ToggleGroupItem value='usage' className={outcomeItemClassName}>
            {t('Usage not provided')}
          </ToggleGroupItem>
        </ToggleGroup>
        {showLogs && (
          <Link
            to='/usage-logs/$section'
            params={{ section: 'common' }}
            search={recordsSearch}
            data-myapi-button=''
            className={linkButtonClassName({ variant: 'ghost' })}
          >
            {t('View call records')}
          </Link>
        )}
      </div>
      {body}
    </section>
  )
}
