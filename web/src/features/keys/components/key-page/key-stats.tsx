/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import z from 'zod'

import { StatTile } from '@/components/stat-tile'
import { quotaWindowLabel } from '@/features/channels/lib/quota-history'
import { getAllLogs, getUserLogs } from '@/features/usage-logs/api'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import { getRequestFacts } from '@/features/usage-logs/lib/request-facts'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import { useKeySubscriptionShare } from '../../hooks/use-key-subscription-share'
import { checkedAddSafeIntegers } from '../../lib/quota-display'
import { heldSharePools, shareUsedPercent } from '../../lib/share-usage'
import type { ApiKey } from '../../types'

const totalSchema = z.number().int().nonnegative()

/**
 * Three figures of one Key from saved records: requests in the last 24
 * hours (exact totals), failures among its latest requests, and its window
 * share, quota or last use. A figure that cannot be read is unknown, never 0.
 */
export function KeyStats(props: {
  apiKey: ApiKey
  isAdmin: boolean
  /** The Key's latest request records, as the requests tab shows them. */
  recent: UsageLog[] | undefined
  recentPending: boolean
  showQuota: boolean
}) {
  const { t } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const keyId = props.apiKey.id
  const [since] = useState(() => Math.floor(Date.now() / 1000) - 24 * 3600)
  const counts = useQuery({
    queryKey: ['logs', 'key-24h', props.isAdmin, userId, sessionId, keyId],
    queryFn: async () => {
      const read = props.isAdmin ? getAllLogs : getUserLogs
      // Consumption (2) and error (5) records are the Key's requests.
      const responses = await Promise.all(
        [2, 5].map((type) =>
          read({
            p: 1,
            page_size: 1,
            type,
            token_id: keyId,
            start_timestamp: since,
          })
        )
      )
      const totals = responses.map((response) =>
        response?.success ? totalSchema.safeParse(response.data?.total) : null
      )
      if (totals.some((total) => !total?.success)) {
        throw new Error('Key request counts unavailable')
      }
      return totals.reduce((sum, total) => sum + (total?.data ?? 0), 0)
    },
    enabled: userId !== null,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const share = useKeySubscriptionShare(keyId)

  const failed = props.recent?.filter(
    (row) => !getRequestFacts(row).completed
  ).length
  const usageMissing = props.recent?.filter((row) => {
    const facts = getRequestFacts(row)
    return facts.completed && !facts.usageKnown
  }).length

  let third: { label: string; value: ReactNode; detail?: ReactNode }
  const pools = heldSharePools(share.data)
  if (share.isPending) {
    third = { label: t('Window share'), value: null }
  } else if (pools.length > 0) {
    // The share closest to its limit; an unknown baseline stays unknown.
    const ranked = pools
      .map((pool) => ({ pool, used: shareUsedPercent(pool) }))
      .sort((a, b) => (b.used ?? -1) - (a.used ?? -1))
    const top = ranked[0]
    third = {
      label: t('{{window}} share used', {
        window: quotaWindowLabel(top.pool.window_type, t),
      }),
      value:
        top.used === null ? t('Unknown') : `${Number(top.used.toFixed(1))}%`,
      detail: t('Approximate, based on {{name}}', {
        name: top.pool.channel_name || t('Native window'),
      }),
    }
  } else if (props.showQuota) {
    const total = checkedAddSafeIntegers(
      props.apiKey.used_quota,
      props.apiKey.remain_quota
    )
    third = props.apiKey.unlimited_quota
      ? { label: t('Quota'), value: t('Unlimited') }
      : {
          label: t('Remaining quota'),
          value: formatQuota(props.apiKey.remain_quota),
          detail: t('of {{total}}', {
            total: total === null ? t('Unavailable') : formatQuota(total),
          }),
        }
  } else {
    third = {
      label: t('Last used'),
      value:
        props.apiKey.accessed_time > 0
          ? formatTimestampToDate(props.apiKey.accessed_time)
          : t('Never'),
    }
  }

  return (
    <dl className='grid min-w-0 grid-cols-1 gap-2 min-[360px]:grid-cols-3 sm:gap-3'>
      <StatTile
        label={t('Requests in 24 hours')}
        value={counts.isError ? t('Unknown') : counts.data}
        loading={counts.isPending}
        detail={t('Successful and failed records')}
      />
      <StatTile
        label={t('Failed')}
        value={failed ?? t('Unknown')}
        loading={props.recentPending}
        danger={Boolean(failed)}
        detail={
          props.recent
            ? t('Among the latest {{count}} requests', {
                count: props.recent.length,
              }) +
              (usageMissing
                ? ` · ${t('Usage not provided: {{count}}', { count: usageMissing })}`
                : '')
            : undefined
        }
      />
      <StatTile
        label={third.label}
        value={third.value}
        loading={share.isPending}
        detail={third.detail}
      />
    </dl>
  )
}
