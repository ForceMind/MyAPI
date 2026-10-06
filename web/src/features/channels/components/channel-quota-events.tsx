/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { formatTimestampToDate } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import { getChannelQuotaEvents } from '../api'
import { quotaWindowLabel } from '../lib/quota-history'
import type { ChannelQuotaAlertDeliveryEvent } from '../types'

function QuotaEvent(props: { event: ChannelQuotaAlertDeliveryEvent }) {
  const { t } = useTranslation()
  const event = props.event
  const evidence = event.evidence
  const statusLabels: Record<string, string> = {
    warning: t('Low quota'),
    critical: t('Critical quota'),
    exhausted: t('Quota exhausted'),
    healthy: t('Quota recovered'),
  }
  const kindLabels: Record<string, string> = {
    threshold: t('Threshold crossed'),
    reminder: t('Quota reminder'),
    recovery: t('Recovery observation'),
  }
  const deliveryLabels: Record<string, string> = {
    pending: t('External delivery pending'),
    claimed: t('External delivery in progress'),
    retryable: t('External delivery retry scheduled'),
    delivered: t('External delivery completed'),
    quarantined: t('External delivery quarantined'),
  }
  return (
    <li className='min-w-0 space-y-2 rounded-lg border p-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <strong className='text-sm'>
          {statusLabels[event.status] ?? t('Unknown')}
        </strong>
        <span className='text-muted-foreground text-xs'>
          {formatTimestampToDate(event.observed_at)}
        </span>
      </div>
      <p className='text-sm'>{kindLabels[event.kind] ?? t('Unknown')}</p>
      <p className='text-muted-foreground text-xs wrap-break-word'>
        {t('Channel {{id}} · Account window {{series}}', {
          id: event.channel_id,
          series: event.series_ref?.slice(0, 12) || t('Unknown'),
        })}
      </p>
      {evidence ? (
        <div className='space-y-1 text-sm'>
          <p>
            {quotaWindowLabel(evidence.window_type, t)} · {evidence.metric_type}
          </p>
          <p>
            {t('Observed remaining: {{remaining}} / {{total}} {{unit}}', {
              remaining: evidence.available,
              total: evidence.total,
              unit: evidence.currency || evidence.unit,
            })}
          </p>
          {evidence.reset_at ? (
            <p>
              {t('Window resets: {{time}}', {
                time: formatTimestampToDate(evidence.reset_at),
              })}
            </p>
          ) : null}
        </div>
      ) : (
        <p className='text-muted-foreground text-sm'>
          {t('Original observation unavailable')}
        </p>
      )}
      <p className='text-muted-foreground text-xs'>
        {deliveryLabels[event.state] ?? t('Unknown')}
      </p>
      {event.last_error_code ? (
        <p className='text-muted-foreground text-xs wrap-break-word'>
          {t('Delivery error: {{code}}', { code: event.last_error_code })}
        </p>
      ) : null}
    </li>
  )
}

export function ChannelQuotaEvents() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const session = useAuthStore((state) => state.auth.session?.sid)
  const [open, setOpen] = useState(false)
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState('')
  const canRead = hasPermission(
    user,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.READ
  )
  const query = useQuery({
    queryKey: [
      'channel-quota-events',
      user?.id,
      session,
      canRead,
      page,
      status,
    ],
    queryFn: async ({ signal }) => {
      const response = await getChannelQuotaEvents(
        { p: page, page_size: 10, status: status || undefined },
        signal
      )
      if (!response.success || !response.data) {
        throw new Error('quota events unavailable')
      }
      return response.data
    },
    enabled: canRead && open,
    retry: false,
  })
  if (!canRead) return null
  const pages = Math.max(1, Math.ceil((query.data?.total ?? 0) / 10))
  return (
    <Card className='mb-4 min-w-0'>
      <CardHeader>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <CardTitle>{t('Quota events')}</CardTitle>
          <Button
            variant='outline'
            onClick={() => setOpen(!open)}
            aria-expanded={open}
            aria-controls='quota-events-panel'
          >
            {open ? t('Hide events') : t('Show events')}
          </Button>
        </div>
        <CardDescription>
          {t(
            'In-app history works without a webhook. Each account window stays separate; events are observations, not Key consumption.'
          )}
        </CardDescription>
      </CardHeader>
      {open ? (
        <CardContent id='quota-events-panel' className='min-w-0 space-y-3'>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Events are recorded only while provider quota alerts are enabled. Recovery follows the configured preference; no event does not prove healthy quota.'
            )}
          </p>
          <div className='flex flex-wrap items-end gap-3'>
            <label className='grid gap-1 text-sm'>
              {t('Quota event status')}
              <select
                className='bg-background max-w-full rounded-md border p-2'
                value={status}
                onChange={(event) => {
                  setStatus(event.target.value)
                  setPage(1)
                }}
              >
                <option value=''>{t('All states')}</option>
                <option value='warning'>{t('Low quota')}</option>
                <option value='critical'>{t('Critical quota')}</option>
                <option value='exhausted'>{t('Quota exhausted')}</option>
                <option value='healthy'>{t('Quota recovered')}</option>
              </select>
            </label>
            <Button
              variant='outline'
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              {t('Refresh')}
            </Button>
          </div>
          {query.isLoading ? (
            <p role='status'>{t('Loading quota events...')}</p>
          ) : null}
          {query.isError ? (
            <p role='alert'>
              {t('Unable to load quota events. Refresh to try again.')}
            </p>
          ) : null}
          {!query.isLoading &&
          !query.isError &&
          query.data?.items.length === 0 ? (
            <p>{t('No recorded quota events')}</p>
          ) : null}
          {!query.isLoading && !query.isError && query.data ? (
            <ul className='grid min-w-0 gap-3 md:grid-cols-2'>
              {query.data.items.map((event) => (
                <QuotaEvent key={event.id} event={event} />
              ))}
            </ul>
          ) : null}
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <Button
              variant='outline'
              disabled={page <= 1 || query.isFetching}
              onClick={() => setPage(page - 1)}
            >
              {t('Previous')}
            </Button>
            <span className='text-muted-foreground text-xs'>
              {t('Page {{page}} of {{total}}', { page, total: pages })}
            </span>
            <Button
              variant='outline'
              disabled={page >= pages || query.isFetching}
              onClick={() => setPage(page + 1)}
            >
              {t('Next')}
            </Button>
          </div>
        </CardContent>
      ) : null}
    </Card>
  )
}
