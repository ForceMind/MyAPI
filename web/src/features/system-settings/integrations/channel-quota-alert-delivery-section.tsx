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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCcw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  getChannelQuotaAlertDeliveryEvents,
  getChannelQuotaAlertDeliveryStatus,
  runChannelQuotaAlertDelivery,
  updateChannelQuotaAlertDeliverySettings,
} from '@/features/channels/api'
import type { ChannelQuotaAlertDeliveryStatus } from '@/features/channels/types'
import { formatTimestampToDate } from '@/lib/format'

const DELIVERY_QUERY_KEY = ['channel-quota-alert-delivery'] as const
const DELIVERY_PAGE_SIZE = 10

function isValidDeliveryConfiguration(url: string, secret: string): boolean {
  if (secret.length < 16) {
    return false
  }
  try {
    return new URL(url).protocol === 'https:'
  } catch {
    return false
  }
}

function getDeliveryStatusBadge(
  status: ChannelQuotaAlertDeliveryStatus,
  t: (key: string) => string
) {
  if (!status.policy_enabled) {
    return { label: t('Policy disabled'), variant: 'warning' as const }
  }
  if (!status.configured) {
    return { label: t('Not configured'), variant: 'neutral' as const }
  }
  return { label: t('Configured'), variant: 'success' as const }
}

function getEventStateVariant(state: string) {
  if (state === 'delivered') {
    return 'success' as const
  }
  if (state === 'quarantined') {
    return 'danger' as const
  }
  return 'warning' as const
}

export function ChannelQuotaAlertDeliverySection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [webhookURL, setWebhookURL] = useState('')
  const [webhookSecret, setWebhookSecret] = useState('')
  const [page, setPage] = useState(1)
  const [stateFilter, setStateFilter] = useState('')

  const statusQuery = useQuery({
    queryKey: [...DELIVERY_QUERY_KEY, 'status'],
    queryFn: async () => {
      const response = await getChannelQuotaAlertDeliveryStatus()
      if (!response.success || !response.data) {
        throw new Error(
          response.message || 'Unable to load quota alert delivery status.'
        )
      }
      return response.data
    },
  })

  const historyQuery = useQuery({
    queryKey: [...DELIVERY_QUERY_KEY, 'events', page, stateFilter],
    queryFn: async () => {
      const response = await getChannelQuotaAlertDeliveryEvents({
        p: page,
        page_size: DELIVERY_PAGE_SIZE,
        state: stateFilter || undefined,
      })
      if (!response.success || !response.data) {
        throw new Error(
          response.message || 'Unable to load quota alert delivery history.'
        )
      }
      return response.data
    },
  })

  const refreshDelivery = async () => {
    await Promise.all([statusQuery.refetch(), historyQuery.refetch()])
  }

  const updateMutation = useMutation({
    mutationFn: () =>
      updateChannelQuotaAlertDeliverySettings({
        webhook_url: webhookURL.trim(),
        webhook_secret: webhookSecret,
      }),
    onSuccess: async (response) => {
      if (!response.success || !response.data) {
        toast.error(
          response.message || t('Unable to save quota alert delivery settings.')
        )
        return
      }
      setWebhookURL('')
      setWebhookSecret('')
      toast.success(t('Quota alert delivery settings saved.'))
      await refreshDelivery()
    },
    onError: () => {
      toast.error(t('Unable to save quota alert delivery settings.'))
    },
  })

  const runMutation = useMutation({
    mutationFn: runChannelQuotaAlertDelivery,
    onSuccess: async (response) => {
      if (!response.success || !response.data) {
        toast.error(
          response.message || t('Unable to run quota alert delivery.')
        )
        return
      }
      toast.success(
        t(
          'Quota alert delivery run completed: {{delivered}} delivered, {{retryable}} retryable, {{quarantined}} quarantined.',
          {
            delivered: response.data.delivered,
            retryable: response.data.retryable,
            quarantined: response.data.quarantined,
          }
        )
      )
      await queryClient.invalidateQueries({ queryKey: DELIVERY_QUERY_KEY })
    },
    onError: () => {
      toast.error(t('Unable to run quota alert delivery.'))
    },
  })

  if (statusQuery.isLoading) {
    return <LoadingState message={t('Loading quota alert delivery...')} />
  }

  if (statusQuery.isError || !statusQuery.data) {
    return (
      <ErrorState
        title={t('Unable to load quota alert delivery status.')}
        onRetry={() => void statusQuery.refetch()}
      />
    )
  }

  const status = statusQuery.data
  const statusBadge = getDeliveryStatusBadge(status, t)
  const isSaving = updateMutation.isPending
  const canSave = isValidDeliveryConfiguration(webhookURL.trim(), webhookSecret)
  const canRun =
    status.policy_enabled && status.configured && !runMutation.isPending
  const events = historyQuery.data?.items ?? []
  const totalPages = Math.max(
    1,
    Math.ceil((historyQuery.data?.total ?? 0) / DELIVERY_PAGE_SIZE)
  )

  return (
    <div className='bg-muted/20 grid gap-4 rounded-lg border p-4'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div>
          <h4 className='font-medium'>{t('Quota alert delivery')}</h4>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t(
              'Deliver trusted provider quota alert events to an HTTPS webhook.'
            )}
          </p>
        </div>
        <div className='flex items-center gap-2'>
          <StatusBadge
            label={statusBadge.label}
            variant={statusBadge.variant}
            copyable={false}
          />
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => void refreshDelivery()}
            disabled={statusQuery.isFetching || historyQuery.isFetching}
          >
            <RefreshCcw
              className={
                statusQuery.isFetching || historyQuery.isFetching
                  ? 'animate-spin'
                  : undefined
              }
              aria-hidden='true'
            />
            {t('Refresh')}
          </Button>
        </div>
      </div>

      {!status.policy_enabled && (
        <Alert>
          <AlertTitle>{t('Policy disabled')}</AlertTitle>
          <AlertDescription>
            {t(
              'Enable provider quota alerts before alert events can be delivered.'
            )}
          </AlertDescription>
        </Alert>
      )}

      <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-5'>
        <StatusMetric
          label={t('Endpoint host')}
          value={status.endpoint_host || '—'}
        />
        <StatusMetric
          label={t('HTTPS only')}
          value={status.https_only ? t('Yes') : t('No')}
        />
        <StatusMetric
          label={t('Redirects')}
          value={status.redirects_allowed ? t('Allowed') : t('Blocked')}
        />
        <StatusMetric label={t('Timeout')} value={`${status.timeout_ms} ms`} />
        <StatusMetric
          label={t('Maximum attempts')}
          value={String(status.max_attempts)}
        />
      </div>

      <div className='grid gap-4 sm:grid-cols-2'>
        <div className='space-y-2'>
          <Label htmlFor='quota-alert-webhook-url'>{t('Webhook URL')}</Label>
          <Input
            id='quota-alert-webhook-url'
            type='url'
            value={webhookURL}
            onChange={(event) => setWebhookURL(event.target.value)}
            placeholder='https://alerts.example.com/quota'
            disabled={isSaving}
          />
        </div>
        <div className='space-y-2'>
          <Label htmlFor='quota-alert-webhook-secret'>
            {t('Webhook signing secret')}
          </Label>
          <Input
            id='quota-alert-webhook-secret'
            type='password'
            value={webhookSecret}
            onChange={(event) => setWebhookSecret(event.target.value)}
            autoComplete='new-password'
            disabled={isSaving}
          />
        </div>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Replace both values to change the delivery endpoint. Secrets are never displayed after saving.'
        )}
      </p>
      <div className='flex flex-wrap gap-2'>
        <Button
          type='button'
          onClick={() => updateMutation.mutate()}
          disabled={!canSave || isSaving}
        >
          {isSaving ? t('Saving...') : t('Save delivery configuration')}
        </Button>
        <Button
          type='button'
          variant='outline'
          onClick={() => runMutation.mutate()}
          disabled={!canRun}
        >
          {runMutation.isPending ? t('Running...') : t('Run delivery now')}
        </Button>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Manual delivery runs only when provider quota alerts and delivery are configured.'
        )}
      </p>

      <div className='space-y-3 border-t pt-4'>
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <h5 className='text-sm font-semibold'>{t('Delivery history')}</h5>
          <Select
            items={[
              { value: '', label: t('All states') },
              { value: 'pending', label: 'pending' },
              { value: 'retryable', label: 'retryable' },
              { value: 'delivered', label: 'delivered' },
              { value: 'quarantined', label: 'quarantined' },
            ]}
            value={stateFilter}
            onValueChange={(value) => {
              setStateFilter(value ?? '')
              setPage(1)
            }}
            disabled={historyQuery.isFetching}
          >
            <SelectTrigger className='w-40'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectItem value=''>{t('All states')}</SelectItem>
                <SelectItem value='pending'>pending</SelectItem>
                <SelectItem value='retryable'>retryable</SelectItem>
                <SelectItem value='delivered'>delivered</SelectItem>
                <SelectItem value='quarantined'>quarantined</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>

        {historyQuery.isLoading && (
          <LoadingState
            inline
            message={t('Loading quota alert delivery history...')}
          />
        )}
        {!historyQuery.isLoading && historyQuery.isError && (
          <ErrorState
            className='min-h-40'
            title={t('Unable to load quota alert delivery history.')}
            onRetry={() => void historyQuery.refetch()}
          />
        )}
        {!historyQuery.isLoading &&
          !historyQuery.isError &&
          events.length === 0 && (
            <p className='text-muted-foreground rounded-md border border-dashed px-4 py-6 text-center text-sm'>
              {t('No quota alert delivery events yet.')}
            </p>
          )}
        {!historyQuery.isLoading &&
          !historyQuery.isError &&
          events.length > 0 && (
            <div className='overflow-x-auto rounded-md border'>
              <table className='w-full min-w-[760px] text-left text-sm'>
                <thead className='bg-muted/50 text-muted-foreground text-xs'>
                  <tr>
                    <th className='px-3 py-2'>{t('Event')}</th>
                    <th className='px-3 py-2'>{t('Channel')}</th>
                    <th className='px-3 py-2'>{t('Status')}</th>
                    <th className='px-3 py-2'>{t('Attempts')}</th>
                    <th className='px-3 py-2'>{t('Observed')}</th>
                    <th className='px-3 py-2'>{t('Next attempt')}</th>
                    <th className='px-3 py-2'>{t('Error')}</th>
                  </tr>
                </thead>
                <tbody>
                  {events.map((event) => (
                    <tr key={event.id} className='border-t'>
                      <td className='px-3 py-2 font-mono text-xs'>
                        {event.event_key}
                      </td>
                      <td className='px-3 py-2'>{event.channel_id}</td>
                      <td className='px-3 py-2'>
                        <StatusBadge
                          label={event.state}
                          variant={getEventStateVariant(event.state)}
                          copyable={false}
                        />
                      </td>
                      <td className='px-3 py-2'>{event.attempt_count}</td>
                      <td className='px-3 py-2'>
                        {formatTimestampToDate(event.observed_at)}
                      </td>
                      <td className='px-3 py-2'>
                        {event.next_attempt_at
                          ? formatTimestampToDate(event.next_attempt_at)
                          : '—'}
                      </td>
                      <td className='px-3 py-2 font-mono text-xs'>
                        {event.last_error_code || '—'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

        {totalPages > 1 && (
          <div className='flex items-center justify-end gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => setPage((current) => Math.max(1, current - 1))}
              disabled={page === 1 || historyQuery.isFetching}
            >
              {t('Previous')}
            </Button>
            <span className='text-muted-foreground text-sm'>
              {t('Page {{current}} of {{total}}', {
                current: page,
                total: totalPages,
              })}
            </span>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() =>
                setPage((current) => Math.min(totalPages, current + 1))
              }
              disabled={page >= totalPages || historyQuery.isFetching}
            >
              {t('Next')}
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}

function StatusMetric(props: { label: string; value: string }) {
  return (
    <div className='bg-muted/40 rounded-md p-3'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='mt-1 text-sm font-medium break-all'>{props.value}</div>
    </div>
  )
}
