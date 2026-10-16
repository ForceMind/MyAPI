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
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { getUserLogs } from '@/features/usage-logs/api'
import { usageLogSchema } from '@/features/usage-logs/data/schema'
import { getPersonalCallEvidence } from '@/features/usage-logs/lib/personal-call-evidence'
import { toIntlLocale } from '@/i18n/languages'
import { useAuthStore } from '@/stores/auth-store'

export function OverviewRecent() {
  const { t, i18n } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const sessionId = useAuthStore((state) => state.auth.session?.sid)
  const query = useQuery({
    queryKey: ['personal-console', 'recent', userId, sessionId],
    queryFn: async () => {
      const response = await getUserLogs({ p: 1, page_size: 10 })
      if (
        !response.success ||
        !response.data ||
        !Array.isArray(response.data.items)
      ) {
        throw new Error('Recent calls unavailable')
      }
      return usageLogSchema
        .array()
        .parse(response.data.items)
        .filter((log) => log.type === 2 || log.type === 5)
        .slice(0, 5)
    },
    retry: false,
    refetchOnWindowFocus: false,
  })
  const rows = query.isError ? undefined : query.data
  return (
    <Card className='pc-card min-w-0' data-testid='recent-records-section'>
      <CardHeader>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <CardTitle>
            <h2>{t('Recent calls')}</h2>
          </CardTitle>
          <Button
            variant='ghost'
            size='sm'
            nativeButton={false}
            render={
              <Link to='/usage-logs/$section' params={{ section: 'common' }} />
            }
          >
            {t('View call logs')}
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {query.isLoading ? <Skeleton className='h-40 w-full' /> : null}
        {query.isError ? (
          <Alert variant='destructive' data-testid='recent-error-state'>
            <AlertTitle>{t('Unable to load your recent records')}</AlertTitle>
            <Button variant='outline' onClick={() => void query.refetch()}>
              {t('Retry')}
            </Button>
          </Alert>
        ) : null}
        {rows?.length === 0 ? (
          <p
            className='text-muted-foreground py-8 text-center'
            data-testid='recent-empty-state'
          >
            {t('No recent records')}
          </p>
        ) : null}
        {rows && rows.length > 0 ? (
          <ul className='divide-y'>
            {rows.map((row) => {
              const evidence = getPersonalCallEvidence(row)
              const createdAt = new Date(row.created_at * 1000)
              const validDate = Number.isFinite(createdAt.getTime())
              return (
                <li
                  key={row.id}
                  className='grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-2 py-4 sm:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,1fr)_auto]'
                >
                  <div className='min-w-0'>
                    <p className='font-medium break-words'>
                      {row.token_name || row.model_name || t('Unknown')}
                    </p>
                    <p className='text-muted-foreground mt-1 text-xs break-words'>
                      {row.model_name}
                    </p>
                  </div>
                  <span className='col-start-1 row-start-2 sm:col-start-auto sm:row-start-auto'>
                    <StatusBadge
                      variant={evidence.completed ? 'success' : 'danger'}
                      copyable={false}
                    >
                      {evidence.completed
                        ? t('Request successful')
                        : t('Request failed')}
                    </StatusBadge>
                  </span>
                  <span className='text-muted-foreground col-start-2 row-start-2 text-right text-xs sm:col-start-auto sm:row-start-auto sm:text-left'>
                    {evidence.usageKnown
                      ? t('{{count}} Tokens', {
                          count: row.prompt_tokens + row.completion_tokens,
                        })
                      : t('Usage not provided')}
                  </span>
                  <div className='col-start-2 row-start-1 text-right text-xs sm:col-start-auto sm:row-start-auto'>
                    {validDate ? (
                      <time dateTime={createdAt.toISOString()}>
                        {new Intl.DateTimeFormat(toIntlLocale(i18n.language), {
                          hour: '2-digit',
                          minute: '2-digit',
                        }).format(createdAt)}
                      </time>
                    ) : (
                      t('Unknown')
                    )}
                    <p className='text-muted-foreground mt-1'>
                      {Number.isFinite(row.use_time) && row.use_time > 0
                        ? t('{{count}} seconds', { count: row.use_time })
                        : t('Unknown')}
                    </p>
                  </div>
                </li>
              )
            })}
          </ul>
        ) : null}
      </CardContent>
    </Card>
  )
}
