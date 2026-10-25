/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Alert02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { PendingUsageReviews } from '@/features/usage-logs/components/pending-usage-reviews'
import { getRequestFacts } from '@/features/usage-logs/lib/request-facts'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'

import type { OverviewData } from './use-overview-data'

const linkClassName =
  'underline underline-offset-3 hover:no-underline focus-visible:outline-2'

/**
 * Needs attention (design v2 home): one amber strip listing what to handle,
 * each item linking to the filtered place where it can be handled: disabled
 * connections, failed requests and requests without a usage report among
 * the latest ones, failing account samples and body capture being off. It
 * takes no space when nothing needs attention, except for the root operator,
 * who always reaches the pending settlement reviews here.
 */
export function OverviewAttention(props: { data: OverviewData }) {
  const { t } = useTranslation()
  const showLogs = useIsSidebarModuleVisible('/usage-logs/common')
  const showConnections = useIsSidebarModuleVisible('/connections')
  const showAccounts = useIsSidebarModuleVisible('/upstream-accounts')
  const showBodies = useIsSidebarModuleVisible('/full-content-logs')
  const rows = props.data.sample.isError
    ? undefined
    : props.data.sample.data?.rows
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
  const root = props.data.user?.role === ROLE.SUPER_ADMIN

  const items: { id: string; link: ReactNode }[] = []
  if (props.data.canReadChannels && showConnections && disabled > 0) {
    items.push({
      id: 'connections',
      link: (
        <Link
          to='/connections'
          search={{ status: 'disabled' }}
          className={linkClassName}
        >
          {t('{{count}} connections are disabled', { count: disabled })}
        </Link>
      ),
    })
  }
  if (showLogs && failed > 0) {
    items.push({
      id: 'failed',
      link: (
        <Link
          to='/usage-logs/$section'
          params={{ section: 'common' }}
          search={errorSearch}
          className={linkClassName}
        >
          {t('{{count}} of the latest requests failed', { count: failed })}
        </Link>
      ),
    })
  }
  if (showLogs && usageMissing > 0) {
    items.push({
      id: 'usage',
      link: (
        <Link
          to='/usage-logs/$section'
          params={{ section: 'common' }}
          search={errorSearch}
          className={linkClassName}
        >
          {t('{{count}} requests completed without a usage report', {
            count: usageMissing,
          })}
        </Link>
      ),
    })
  }
  if (showAccounts && failingAccounts > 0) {
    items.push({
      id: 'accounts',
      link: (
        <Link to='/upstream-accounts' className={linkClassName}>
          {t('Accounts with failed samples: {{count}}', {
            count: failingAccounts,
          })}
        </Link>
      ),
    })
  }
  if (showBodies && props.data.capture.capturing === false) {
    items.push({
      id: 'capture',
      link: (
        <Link to='/full-content-logs' className={linkClassName}>
          {t(
            'Body capture is off (FULL_CONTENT_LOG_ENABLED). New requests save no body.'
          )}
        </Link>
      ),
    })
  }

  let summary: ReactNode = null
  if (items.length > 0) {
    summary = (
      <strong className='font-semibold'>
        {t('{{count}} items need attention:', { count: items.length })}
      </strong>
    )
  } else if (props.data.sample.isError) {
    summary = t('Recent requests could not be checked.')
  } else if (root && props.data.sample.isPending) {
    summary = t('Loading...')
  } else if (root) {
    summary = t('Nothing needs attention right now.')
  }
  const reviews = showLogs && root
  if (summary === null && !reviews) return null
  const warning = items.length > 0 || props.data.sample.isError

  return (
    <section
      aria-label={t('Needs attention')}
      className={cn(
        'text-body flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2 rounded-lg px-3.5 py-2.5',
        warning ? 'bg-warning-muted text-warning' : 'bg-muted text-foreground'
      )}
    >
      {warning && (
        <HugeiconsIcon
          icon={Alert02Icon}
          className='size-4 shrink-0'
          aria-hidden='true'
        />
      )}
      <p className='min-w-0 flex-1 basis-64 wrap-anywhere'>
        {summary}
        {items.map((item, index) => (
          <span key={item.id}>
            {index === 0 ? ' ' : ' · '}
            {item.link}
          </span>
        ))}
      </p>
      {reviews && <PendingUsageReviews />}
    </section>
  )
}
