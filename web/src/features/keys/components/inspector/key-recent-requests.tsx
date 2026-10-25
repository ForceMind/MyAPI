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
import { useState, type CSSProperties } from 'react'
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
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import {
  RequestDuration,
  RequestOutcomeBadge,
  RequestTime,
} from '@/features/usage-logs/components/request-summary'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import { getRequestFacts } from '@/features/usage-logs/lib/request-facts'
import { useMediaQuery } from '@/hooks/use-media-query'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { linkButtonClassName } from '@/lib/link-button'
import { cn } from '@/lib/utils'

import type { useKeyRecentRequests } from '../../hooks/use-key-recent-requests'
import type { KeyRequestContext } from '../../key-request-context'

type Outcome = 'all' | 'failed' | 'usage'
// PC lays each request out in columns; phones use two lines.
const rowGrid =
  'grid grid-cols-[6.5rem_11rem_minmax(0,1fr)_var(--connection-column)_5rem] items-center gap-3'

/**
 * The Key's latest request records: time, result, model with its usage,
 * connection (administrators) and duration. Each row opens the request
 * page. The outcome switch narrows the loaded rows to failures or to
 * completed requests without a usage report; it never invents rows.
 */
export function KeyRecentRequests(props: {
  keyId: number
  query: ReturnType<typeof useKeyRecentRequests>
  /** Shows the connection column (administrators read it). */
  showConnection: boolean
  /** Where "View all" returns to, so the list keeps its place. */
  returnContext: KeyRequestContext
  onOpen: (log: UsageLog) => void
}) {
  const { t } = useTranslation()
  const mobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const [outcome, setOutcome] = useState<Outcome>('all')
  const rows = props.query.isError ? undefined : props.query.data
  const failed = rows?.filter((row) => !getRequestFacts(row).completed) ?? []
  const usageMissing =
    rows?.filter((row) => {
      const facts = getRequestFacts(row)
      return facts.completed && !facts.usageKnown
    }) ?? []
  let shown = rows ?? []
  if (outcome === 'failed') shown = failed
  if (outcome === 'usage') shown = usageMissing

  let body
  if (props.query.isPending) {
    body = (
      <div className='flex flex-col gap-2 p-3' aria-busy='true'>
        <span className='sr-only'>{t('Loading...')}</span>
        {[1, 2, 3, 4].map((item) => (
          <Skeleton key={item} className='h-11 w-full' />
        ))}
      </div>
    )
  } else if (props.query.isError) {
    body = (
      <Alert variant='destructive' className='m-3 w-auto'>
        <AlertTitle>{t('Could not load requests for this Key')}</AlertTitle>
        <AlertDescription className='flex flex-col items-start gap-2'>
          <Button
            variant='outline'
            size='sm'
            disabled={props.query.isFetching}
            onClick={() => void props.query.refetch()}
          >
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else if (!rows?.length) {
    body = (
      <Empty className='m-3 border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={InboxIcon} aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('No requests from this Key yet')}</EmptyTitle>
          <EmptyDescription>
            {t('Requests appear here once a client uses this Key.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  } else if (shown.length === 0) {
    body = (
      <Empty className='m-3 border'>
        <EmptyHeader>
          <EmptyTitle>
            {t('No matching requests among the latest ones')}
          </EmptyTitle>
          <EmptyDescription>
            {t('Older requests are in the call records.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  } else {
    body = (
      <div
        className='min-w-0'
        style={
          {
            '--connection-column': props.showConnection
              ? 'minmax(0,12rem)'
              : '0px',
          } as CSSProperties
        }
      >
        {/* Column names for sighted users; each row's button names itself. */}
        {!mobile && (
          <div
            aria-hidden='true'
            className={cn(
              'text-caption text-muted-foreground border-b px-4 py-2',
              rowGrid
            )}
          >
            <span>{t('Time')}</span>
            <span>{t('Result')}</span>
            <span>{t('Model')}</span>
            <span>{props.showConnection ? t('Connection') : ''}</span>
            <span className='text-right'>{t('Duration')}</span>
          </div>
        )}
        <ul className='flex flex-col divide-y' role='list'>
          {shown.map((row) => {
            const usage = getRequestFacts(row).usageKnown
              ? t('{{count}} Tokens', {
                  count: row.prompt_tokens + row.completion_tokens,
                })
              : t('Usage not provided')
            return (
              <li key={row.id}>
                <button
                  type='button'
                  data-request-row={row.id}
                  onClick={() => props.onOpen(row)}
                  className={cn(
                    'hover:bg-muted flex min-h-12 w-full min-w-0 items-center gap-3 px-4 py-2 text-left focus-visible:outline-2 focus-visible:-outline-offset-2',
                    !mobile && rowGrid
                  )}
                >
                  {mobile ? (
                    <>
                      <span className='flex min-w-0 flex-1 flex-col gap-1'>
                        <span className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1'>
                          <span className='text-data text-muted-foreground font-mono'>
                            <RequestTime seconds={row.created_at} />
                          </span>
                          <RequestOutcomeBadge log={row} />
                        </span>
                        <span className='text-data min-w-0 font-mono wrap-anywhere'>
                          {row.model_name || t('Unknown model')}
                          <span className='text-muted-foreground'>
                            {' · '}
                            <span>{usage}</span>
                            {' · '}
                            <span className='whitespace-nowrap'>
                              <RequestDuration seconds={row.use_time} />
                            </span>
                          </span>
                        </span>
                      </span>
                      <HugeiconsIcon
                        icon={ArrowRight01Icon}
                        className='text-muted-foreground size-4 shrink-0'
                        aria-hidden='true'
                      />
                    </>
                  ) : (
                    <>
                      <span className='text-data text-muted-foreground font-mono'>
                        <RequestTime seconds={row.created_at} />
                      </span>
                      <span>
                        <RequestOutcomeBadge log={row} />
                      </span>
                      <span className='text-data min-w-0 truncate font-mono'>
                        {row.model_name || t('Unknown model')}
                        <span className='text-muted-foreground'>
                          {' · '}
                          <span>{usage}</span>
                        </span>
                      </span>
                      <span className='text-data min-w-0 truncate'>
                        {props.showConnection &&
                          (row.channel_name ||
                            (row.channel ? `#${row.channel}` : t('Unknown')))}
                      </span>
                      <span className='text-data text-right'>
                        <RequestDuration seconds={row.use_time} />
                      </span>
                    </>
                  )}
                </button>
              </li>
            )
          })}
        </ul>
      </div>
    )
  }

  return (
    <section
      aria-labelledby='key-requests-title'
      className='bg-card flex min-w-0 flex-col overflow-hidden rounded-xl border'
    >
      <div className='flex min-w-0 flex-wrap items-center gap-2 border-b px-4 py-3'>
        <h2 id='key-requests-title' className='sr-only'>
          {t('Recent requests')}
        </h2>
        <ToggleGroup
          aria-label={t('Request outcome')}
          variant='outline'
          size='sm'
          value={[outcome]}
          onValueChange={(value) => {
            const next = value[0] as Outcome | undefined
            if (next) setOutcome(next)
          }}
          className='max-w-full flex-wrap'
        >
          <ToggleGroupItem value='all' className={outcomeItemClassName}>
            {rows ? t('All {{count}}', { count: rows.length }) : t('All')}
          </ToggleGroupItem>
          <ToggleGroupItem value='failed' className={outcomeItemClassName}>
            {rows
              ? t('Failed {{count}}', { count: failed.length })
              : t('Failed')}
          </ToggleGroupItem>
          <ToggleGroupItem value='usage' className={outcomeItemClassName}>
            {rows
              ? t('Usage not provided {{count}}', {
                  count: usageMissing.length,
                })
              : t('Usage not provided')}
          </ToggleGroupItem>
        </ToggleGroup>
        <span className='text-caption text-muted-foreground ms-auto'>
          {t('Latest requests of this Key')}
        </span>
      </div>
      {body}
      <div className='border-t px-2 py-2'>
        <Link
          to='/usage-logs/$section'
          params={{ section: 'common' }}
          search={{
            tokenId: props.keyId,
            page: 1,
            pageSize: 20,
            keyReturn: props.returnContext,
          }}
          data-myapi-button=''
          className={linkButtonClassName({ variant: 'ghost' })}
        >
          {t('View all in call records')}
        </Link>
      </div>
    </section>
  )
}

const outcomeItemClassName =
  'h-auto min-h-7 min-w-0 shrink whitespace-normal max-lg:min-h-11'
