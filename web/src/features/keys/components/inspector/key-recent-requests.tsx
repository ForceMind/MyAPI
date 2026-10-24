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
  RequestDuration,
  RequestOutcomeBadge,
  RequestTime,
} from '@/features/usage-logs/components/request-summary'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import { getRequestFacts } from '@/features/usage-logs/lib/request-facts'
import { linkButtonClassName } from '@/lib/link-button'

import type { useKeyRecentRequests } from '../../hooks/use-key-recent-requests'
import type { KeyRequestContext } from '../../key-request-context'

export function KeyRecentRequests(props: {
  keyId: number
  query: ReturnType<typeof useKeyRecentRequests>
  /** Where "View all" returns to, so the list keeps its place. */
  returnContext: KeyRequestContext
  onOpen: (log: UsageLog) => void
}) {
  const { t } = useTranslation()
  const rows = props.query.isError ? undefined : props.query.data

  let body
  if (props.query.isPending) {
    body = (
      <div className='flex flex-col gap-2' aria-busy='true'>
        <span className='sr-only'>{t('Loading...')}</span>
        {[1, 2, 3, 4].map((item) => (
          <Skeleton key={item} className='h-14 w-full' />
        ))}
      </div>
    )
  } else if (props.query.isError) {
    body = (
      <Alert variant='destructive'>
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
      <Empty className='border'>
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
  } else {
    body = (
      <ul className='flex flex-col divide-y rounded-lg border' role='list'>
        {rows.map((row) => {
          const facts = getRequestFacts(row)
          return (
            <li key={row.id}>
              <button
                type='button'
                data-request-row={row.id}
                onClick={() => props.onOpen(row)}
                className='hover:bg-muted flex min-h-14 w-full min-w-0 items-center gap-3 px-3 py-2 text-left focus-visible:outline-2 focus-visible:-outline-offset-2'
              >
                <span className='flex min-w-0 flex-1 flex-col gap-1'>
                  <span className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1'>
                    <RequestOutcomeBadge log={row} />
                    <span className='text-data text-muted-foreground'>
                      <RequestTime seconds={row.created_at} />
                    </span>
                  </span>
                  <span className='text-data flex min-w-0 flex-wrap gap-x-2 gap-y-0.5'>
                    <span className='min-w-0 truncate'>
                      {row.model_name || t('Unknown model')}
                    </span>
                    <span className='text-muted-foreground'>
                      <RequestDuration seconds={row.use_time} />
                    </span>
                    <span className='text-muted-foreground'>
                      {facts.usageKnown
                        ? t('{{count}} Tokens', {
                            count: row.prompt_tokens + row.completion_tokens,
                          })
                        : t('Usage not provided')}
                    </span>
                  </span>
                </span>
                <HugeiconsIcon
                  icon={ArrowRight01Icon}
                  className='text-muted-foreground size-4 shrink-0'
                  aria-hidden='true'
                />
              </button>
            </li>
          )
        })}
      </ul>
    )
  }

  return (
    <div className='flex min-w-0 flex-col gap-3'>
      {body}
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
        className={linkButtonClassName({
          variant: 'ghost',
          className: 'self-start',
        })}
      >
        {t('View all in call records')}
      </Link>
    </div>
  )
}
