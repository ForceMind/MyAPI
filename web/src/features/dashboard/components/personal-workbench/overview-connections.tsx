/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Alert, AlertTitle } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { getChannels } from '@/features/channels/api'
import { CHANNEL_STATUS_CONFIG } from '@/features/channels/constants'
import { toIntlLocale } from '@/i18n/languages'
import { useAuthStore } from '@/stores/auth-store'

export function OverviewConnections(props: {
  canRead: boolean
  showModels: boolean
}) {
  const { t, i18n } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const role = useAuthStore((state) => state.auth.user?.role)
  const sessionId = useAuthStore((state) => state.auth.session?.sid)
  const query = useQuery({
    queryKey: [
      'personal-console',
      'connections',
      userId,
      role,
      sessionId,
      props.canRead,
    ],
    enabled: props.canRead,
    retry: false,
    refetchOnWindowFocus: false,
    queryFn: async () => {
      const response = await getChannels({
        p: 1,
        page_size: 4,
        sort_by: 'id',
        sort_order: 'desc',
      })
      const data = response.data
      if (
        !response.success ||
        !data ||
        !Number.isSafeInteger(data.total) ||
        data.total < 0 ||
        !Array.isArray(data.items) ||
        data.total < data.items.length ||
        data.items.some(
          (item) =>
            !Number.isSafeInteger(item.id) ||
            item.id <= 0 ||
            typeof item.name !== 'string' ||
            !Number.isSafeInteger(item.status)
        )
      ) {
        throw new Error('Connections unavailable')
      }
      // Keep only configuration metadata in the overview cache.
      return {
        total: data.total,
        items: data.items.map((item) => ({
          id: item.id,
          name: item.name,
          status: item.status,
        })),
      }
    },
  })
  if (!props.canRead) return null
  const connections = query.isError ? undefined : query.data
  return (
    <Card className='pc-card min-w-0' data-testid='overview-connections'>
      <CardHeader>
        <CardTitle>
          <h2>{t('Configured connections')}</h2>
        </CardTitle>
        <p className='text-muted-foreground text-sm'>
          {t('API channels and subscription connections')}
        </p>
      </CardHeader>
      <CardContent className='flex min-w-0 flex-col gap-4'>
        {query.isLoading ? <Skeleton className='h-24 w-full' /> : null}
        {query.isError ? (
          <Alert variant='destructive'>
            <AlertTitle>{t('Unable to load connections')}</AlertTitle>
            <Button variant='outline' onClick={() => void query.refetch()}>
              {t('Retry')}
            </Button>
          </Alert>
        ) : null}
        {connections ? (
          <>
            <p className='text-3xl font-semibold tabular-nums'>
              {new Intl.NumberFormat(toIntlLocale(i18n.language)).format(
                connections.total
              )}
            </p>
            {connections.total === 0 ? (
              <p className='text-muted-foreground text-sm'>
                {t('No channels found')}
              </p>
            ) : (
              <>
                <table
                  className='hidden w-full text-left text-sm lg:table'
                  data-testid='connection-desktop-list'
                >
                  <caption className='sr-only'>
                    {t('Recent connections')}
                  </caption>
                  <thead className='text-muted-foreground text-xs'>
                    <tr>
                      <th className='pb-2 font-medium'>{t('Channels')}</th>
                      <th className='pb-2 text-right font-medium'>
                        {t('Status')}
                      </th>
                    </tr>
                  </thead>
                  <tbody className='divide-y'>
                    {connections.items.map((item) => {
                      const status =
                        CHANNEL_STATUS_CONFIG[
                          item.status as keyof typeof CHANNEL_STATUS_CONFIG
                        ] ?? CHANNEL_STATUS_CONFIG[0]
                      return (
                        <tr key={item.id}>
                          <td className='min-w-0 py-2 pr-3 break-all'>
                            <span className='text-muted-foreground mr-2 font-mono text-xs'>
                              #{item.id}
                            </span>
                            {item.name}
                          </td>
                          <td className='py-2 text-right'>
                            <StatusBadge
                              className='h-auto min-h-5 py-0.5 text-left whitespace-normal'
                              variant={status.variant}
                              copyable={false}
                            >
                              {t(status.label)}
                            </StatusBadge>
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
                <ul
                  className='min-w-0 divide-y lg:hidden'
                  aria-label={t('Recent connections')}
                  data-testid='connection-mobile-list'
                >
                  {connections.items.map((item) => {
                    const status =
                      CHANNEL_STATUS_CONFIG[
                        item.status as keyof typeof CHANNEL_STATUS_CONFIG
                      ] ?? CHANNEL_STATUS_CONFIG[0]
                    return (
                      <li
                        key={item.id}
                        className='flex min-w-0 flex-wrap items-center justify-between gap-2 py-2'
                      >
                        <span className='min-w-0 flex-1 break-all'>
                          <span className='text-muted-foreground mr-2 font-mono text-xs'>
                            #{item.id}
                          </span>
                          {item.name}
                        </span>
                        <StatusBadge
                          className='h-auto min-h-5 py-0.5 text-left whitespace-normal'
                          variant={status.variant}
                          copyable={false}
                        >
                          {t(status.label)}
                        </StatusBadge>
                      </li>
                    )
                  })}
                </ul>
              </>
            )}
            <p className='text-muted-foreground text-xs leading-relaxed'>
              {t(
                'Configuration status does not confirm upstream availability.'
              )}
            </p>
          </>
        ) : null}
        <div className='flex flex-wrap gap-2'>
          <Link to='/channels' className={buttonVariants()}>
            {t('Manage connections')}
          </Link>
          {props.showModels ? (
            <Link
              to='/models/$section'
              params={{ section: 'metadata' }}
              className={buttonVariants({ variant: 'outline' })}
            >
              {t('Models')}
            </Link>
          ) : null}
        </div>
      </CardContent>
    </Card>
  )
}
