/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { RecentRequestList } from '@/features/usage-logs/components/recent-request-list'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import { linkButtonClassName } from '@/lib/link-button'

import type { useKeyRecentRequests } from '../../hooks/use-key-recent-requests'
import type { KeyRequestContext } from '../../key-request-context'

/**
 * The Key's latest request records, with the connection column for
 * administrators; each row opens the request page.
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
  return (
    <RecentRequestList
      query={props.query}
      sideColumn={
        props.showConnection
          ? {
              label: t('Connection'),
              value: (row) =>
                row.channel_name ||
                (row.channel ? `#${row.channel}` : t('Unknown')),
            }
          : undefined
      }
      labels={{
        error: t('Could not load requests for this Key'),
        emptyTitle: t('No requests from this Key yet'),
        emptyDescription: t(
          'Requests appear here once a client uses this Key.'
        ),
        caption: t('Latest requests of this Key'),
      }}
      footer={
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
      }
      onOpen={props.onOpen}
    />
  )
}
