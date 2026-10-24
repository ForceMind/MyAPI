/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import {
  Alert02Icon,
  ArrowRight01Icon,
  CheckmarkCircle02Icon,
  FileSearchIcon,
  Plug01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { PendingUsageReviews } from '@/features/usage-logs/components/pending-usage-reviews'
import { getRequestFacts } from '@/features/usage-logs/lib/request-facts'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'

import type { OverviewData } from './use-overview-data'

function AttentionLink(props: {
  icon: typeof Alert02Icon
  children: ReactNode
  render: (className: string, content: ReactNode) => ReactNode
}) {
  const content = (
    <>
      <HugeiconsIcon
        icon={props.icon}
        className='text-warning size-4 shrink-0'
        aria-hidden='true'
      />
      <span className='min-w-0 flex-1'>{props.children}</span>
      <HugeiconsIcon
        icon={ArrowRight01Icon}
        className='text-muted-foreground size-4 shrink-0'
        aria-hidden='true'
      />
    </>
  )
  return (
    <li>
      {props.render(
        'hover:bg-muted text-body flex min-h-11 items-center gap-3 rounded-lg px-2 py-2 focus-visible:outline-2',
        content
      )}
    </li>
  )
}

/**
 * Needs attention (design 8.1), grouped by object: disabled connections,
 * failed requests and requests without a usage report among the latest
 * ones, failing account samples and body capture being off. Each item links
 * to the filtered place where it can be handled.
 */
export function OverviewAttention(props: { data: OverviewData }) {
  const { t } = useTranslation()
  const showLogs = useIsSidebarModuleVisible('/usage-logs/common')
  const showConnections = useIsSidebarModuleVisible('/connections')
  const showAccounts = useIsSidebarModuleVisible('/upstream-accounts')
  const showBodies = useIsSidebarModuleVisible('/full-content-logs')
  const rows = props.data.sample.isError ? undefined : props.data.sample.data
  const facts = rows?.map(getRequestFacts) ?? []
  const failed = facts.filter((fact) => !fact.completed).length
  const usageMissing = facts.filter(
    (fact) => fact.completed && !fact.usageKnown
  ).length
  const connections = props.data.connections.data
  const disabled = connections ? connections.total - connections.enabled : 0
  const windows = props.data.upstream.accountsQuery.isError
    ? []
    : (props.data.upstream.accountsQuery.data?.items ?? [])
  const failingAccounts = new Set(
    windows
      .filter((item) => item.status === 'error')
      .map((item) => item.channel_id)
  ).size
  const errorSearch = {
    type: ['5' as const],
    startTime: props.data.since * 1000,
    endTime: Date.now() + 3600 * 1000,
  }

  const items: ReactNode[] = []
  if (props.data.canReadChannels && showConnections && disabled > 0) {
    items.push(
      <AttentionLink
        key='connections'
        icon={Plug01Icon}
        render={(className, content) => (
          <Link
            to='/connections'
            search={{ status: 'disabled' }}
            className={className}
          >
            {content}
          </Link>
        )}
      >
        {t('{{count}} connections are disabled', { count: disabled })}
      </AttentionLink>
    )
  }
  if (showLogs && failed > 0) {
    items.push(
      <AttentionLink
        key='failed'
        icon={Alert02Icon}
        render={(className, content) => (
          <Link
            to='/usage-logs/$section'
            params={{ section: 'common' }}
            search={errorSearch}
            className={className}
          >
            {content}
          </Link>
        )}
      >
        {t('{{count}} of the latest requests failed', { count: failed })}
      </AttentionLink>
    )
  }
  if (showLogs && usageMissing > 0) {
    items.push(
      <AttentionLink
        key='usage'
        icon={Alert02Icon}
        render={(className, content) => (
          <Link
            to='/usage-logs/$section'
            params={{ section: 'common' }}
            search={errorSearch}
            className={className}
          >
            {content}
          </Link>
        )}
      >
        {t('{{count}} requests completed without a usage report', {
          count: usageMissing,
        })}
      </AttentionLink>
    )
  }
  if (showAccounts && failingAccounts > 0) {
    items.push(
      <AttentionLink
        key='accounts'
        icon={Alert02Icon}
        render={(className, content) => (
          <Link to='/upstream-accounts' className={className}>
            {content}
          </Link>
        )}
      >
        {t('Accounts with failed samples: {{count}}', {
          count: failingAccounts,
        })}
      </AttentionLink>
    )
  }
  if (showBodies && props.data.capture.capturing === false) {
    items.push(
      <AttentionLink
        key='capture'
        icon={FileSearchIcon}
        render={(className, content) => (
          <Link to='/full-content-logs' className={className}>
            {content}
          </Link>
        )}
      >
        {t(
          'Body capture is off (FULL_CONTENT_LOG_ENABLED). New requests save no body.'
        )}
      </AttentionLink>
    )
  }

  let body: ReactNode
  if (props.data.sample.isPending) {
    body = (
      <div className='flex flex-col gap-2' aria-busy='true'>
        <span className='sr-only'>{t('Loading...')}</span>
        <Skeleton className='h-9 w-full' />
        <Skeleton className='h-9 w-full' />
      </div>
    )
  } else if (items.length > 0) {
    body = <ul className='flex flex-col gap-1'>{items}</ul>
  } else if (props.data.sample.isError) {
    body = (
      <p className='text-body text-muted-foreground flex items-center gap-2'>
        <HugeiconsIcon
          icon={Alert02Icon}
          className='text-warning size-4 shrink-0'
          aria-hidden='true'
        />
        {t('Recent requests could not be checked.')}
      </p>
    )
  } else {
    body = (
      <p className='text-body text-muted-foreground flex items-center gap-2'>
        <HugeiconsIcon
          icon={CheckmarkCircle02Icon}
          className='text-success size-4 shrink-0'
          aria-hidden='true'
        />
        {t('Nothing needs attention right now.')}
      </p>
    )
  }

  return (
    <section
      aria-labelledby='overview-attention-title'
      className='bg-card p-card flex min-w-0 flex-col gap-3 rounded-xl border'
    >
      <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'>
        <h2 id='overview-attention-title' className='text-title'>
          {t('Needs attention')}
        </h2>
        {showLogs && <PendingUsageReviews />}
      </div>
      {body}
    </section>
  )
}
