/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { getChannelQuotaChanges } from '@/features/channels/api'
import { quotaSeriesKey } from '@/features/channels/lib/quota-history'
import { WindowCard } from '@/features/personal-upstream/components/window-card'

export function ConnectionWindows({
  channelId,
  identity,
}: {
  channelId: number
  identity: string
}) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['connections-windows', identity, channelId],
    queryFn: async ({ signal }) => {
      const response = await getChannelQuotaChanges(
        {
          channel_ids: String(channelId),
          range: '24h',
          limit: 100,
          sort: 'observed_desc',
        },
        { signal, disableDuplicate: true }
      )
      if (
        !response.success ||
        !response.data ||
        !Array.isArray(response.data.items) ||
        response.data.items.some(
          (item) =>
            item.channel_id !== channelId || typeof item.name !== 'string'
        )
      ) {
        throw new Error('Connection windows unavailable')
      }
      return response.data.items.filter(
        (item) =>
          item.metric_type === 'codex_rate_limit' && item.unit === 'percent'
      )
    },
    retry: false,
    refetchOnWindowFocus: false,
  })
  return (
    <section aria-label={t('Saved account windows')} className='space-y-3'>
      <h3 className='font-semibold'>{t('Saved account windows')}</h3>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Only saved observations are read. Opening a connection does not contact its provider.'
        )}
      </p>
      {query.isPending && <p role='status'>{t('Loading...')}</p>}
      {query.isError && (
        <div role='alert'>
          <p>{t('Unable to load account windows')}</p>
          <Button variant='outline' onClick={() => void query.refetch()}>
            {t('Retry')}
          </Button>
        </div>
      )}
      {query.isSuccess && query.data.length === 0 && (
        <p className='text-muted-foreground text-sm'>
          {t(
            'No saved subscription windows for this connection. This does not mean the connection is unavailable.'
          )}
        </p>
      )}
      {query.isSuccess &&
        query.data.map((item) => (
          <WindowCard key={quotaSeriesKey(item)} item={item} showHistory />
        ))}
    </section>
  )
}
