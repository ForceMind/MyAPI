import {
  Activity02Icon,
  LinkSquare02Icon,
  Refresh01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
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
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { getChannelQuotaChanges } from '@/features/channels/api'
import { hasPermission } from '@/lib/admin-permissions'
import { getSelf } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { PanelWrapper } from '../ui/panel-wrapper'
import { CodexAccountQuotaChart } from './codex-account-quota-chart'

const OVERVIEW_LIMIT = 4
const OVERVIEW_RANGE = '24h' as const
const OVERVIEW_RATE_WINDOW_SECONDS = 60 * 60
const OVERVIEW_EWMA_HALF_LIFE_SECONDS = 30 * 60
const OVERVIEW_POINT_LIMIT = 48

function getHttpStatus(error: unknown): number | undefined {
  if (!error || typeof error !== 'object') return undefined
  const response = (error as { response?: unknown }).response
  if (!response || typeof response !== 'object') return undefined
  const status = (response as { status?: unknown }).status
  return typeof status === 'number' ? status : undefined
}

export function AccountQuotaChangesPanel() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const setUser = useAuthStore((state) => state.auth.setUser)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const capabilityRefreshKey = useRef<string | null>(null)

  useEffect(() => {
    if (
      !user ||
      !sessionId ||
      user.role < ROLE.ADMIN ||
      user.role >= ROLE.SUPER_ADMIN
    ) {
      return
    }
    const refreshKey = `${user.id}:${sessionId}`
    if (capabilityRefreshKey.current === refreshKey) return
    capabilityRefreshKey.current = refreshKey
    let active = true
    void getSelf()
      .then((response) => {
        const candidate = response?.data
        if (
          !active ||
          !response?.success ||
          !candidate ||
          typeof candidate !== 'object' ||
          candidate.id !== user.id ||
          typeof candidate.role !== 'number'
        ) {
          return
        }
        setUser(candidate as typeof user)
      })
      .catch(() => {
        // The quota query remains authoritative if this best-effort capability
        // refresh fails.
      })
    return () => {
      active = false
    }
  }, [sessionId, setUser, user])

  const canReadChannels = hasPermission(user, 'channel', 'read')
  const query = useQuery({
    queryKey: [
      'dashboard',
      'account-quota-overview',
      user?.id ?? null,
      sessionId,
      canReadChannels,
      OVERVIEW_RANGE,
      OVERVIEW_RATE_WINDOW_SECONDS,
      OVERVIEW_EWMA_HALF_LIFE_SECONDS,
      OVERVIEW_POINT_LIMIT,
      OVERVIEW_LIMIT,
    ],
    queryFn: () =>
      getChannelQuotaChanges({
        range: OVERVIEW_RANGE,
        rate_window: OVERVIEW_RATE_WINDOW_SECONDS,
        ewma_half_life: OVERVIEW_EWMA_HALF_LIFE_SECONDS,
        overview_points: OVERVIEW_POINT_LIMIT,
        limit: OVERVIEW_LIMIT,
        sort: 'observed_desc',
      }),
    enabled: canReadChannels,
    staleTime: 60 * 1000,
    refetchInterval: 60 * 1000,
    retry: false,
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[2] === (user?.id ?? null) &&
      previousQuery?.queryKey[3] === sessionId
        ? previous
        : undefined,
  })

  if (!canReadChannels) {
    if (!user || user.role < ROLE.ADMIN) return null
    return (
      <PanelWrapper
        title={
          <span className='flex items-center gap-2'>
            <IconBadge tone='warning' size='sm'>
              <HugeiconsIcon icon={Activity02Icon} strokeWidth={2} />
            </IconBadge>
            {t('Quota consumption')}
          </span>
        }
        description={t(
          'Observed provider account consumption and latest quota status'
        )}
        empty
        emptyMessage={t(
          'Administrator permission required to view account quota changes'
        )}
      />
    )
  }

  const queryFailed = query.isError || query.data?.success === false
  const status = getHttpStatus(query.error)
  let errorMessage = t('Unable to load account quota changes')
  if (status === 401) {
    errorMessage = t(
      'Your session is missing or expired. Sign in again to load provider account quota.'
    )
  } else if (status === 403) {
    errorMessage = t('Your account does not have permission to read channels.')
  }
  const items = (query.data?.data?.items ?? []).slice(0, OVERVIEW_LIMIT)
  const incomplete = query.data?.data?.source_complete === false

  return (
    <PanelWrapper
      title={
        <span className='flex items-center gap-2'>
          <IconBadge tone='info' size='sm'>
            <HugeiconsIcon icon={Activity02Icon} strokeWidth={2} />
          </IconBadge>
          {t('Quota consumption')}
        </span>
      }
      description={t(
        'Observed provider account consumption and latest quota status'
      )}
      loading={query.isLoading}
      height='h-64'
      contentClassName='grid gap-3'
      headerActions={
        <div className='flex items-center gap-1'>
          <Button
            variant='ghost'
            size='icon-sm'
            onClick={() => void query.refetch()}
            disabled={query.isFetching}
            aria-label={t('Refresh')}
          >
            <HugeiconsIcon
              icon={Refresh01Icon}
              strokeWidth={2}
              className={cn('size-3.5', query.isFetching && 'animate-spin')}
              aria-hidden='true'
            />
          </Button>
          <Button
            variant='ghost'
            size='sm'
            render={<Link to='/channels' />}
            nativeButton={false}
          >
            {t('Channels')}
            <HugeiconsIcon
              icon={LinkSquare02Icon}
              strokeWidth={2}
              data-icon='inline-end'
              aria-hidden='true'
            />
          </Button>
        </div>
      }
    >
      {queryFailed ? (
        <Alert variant='destructive'>
          <AlertTitle>{errorMessage}</AlertTitle>
          <AlertDescription className='grid gap-2'>
            <p>{t('Please try again later.')}</p>
            {status === 401 ? (
              <Button
                variant='outline'
                size='sm'
                render={<Link to='/sign-in' />}
                nativeButton={false}
              >
                {t('Sign in again')}
              </Button>
            ) : null}
          </AlertDescription>
        </Alert>
      ) : null}
      {incomplete ? (
        <Alert>
          <AlertTitle>{t('Incomplete quota history')}</AlertTitle>
          <AlertDescription>
            {t(
              'Some quota series are missing. Narrow the time range to load complete history.'
            )}
          </AlertDescription>
        </Alert>
      ) : null}
      {query.isPlaceholderData ? (
        <p className='text-muted-foreground text-xs' role='status'>
          {t('Loading')}
        </p>
      ) : null}
      {!queryFailed && !query.isLoading && items.length === 0 ? (
        <div className='text-muted-foreground rounded-xl border border-dashed p-4 text-center text-sm'>
          {t(
            'No account quota changes recorded yet. Enable quota sampling or query a provider account to start history.'
          )}
        </div>
      ) : null}
      {!queryFailed && items.length > 0 ? (
        <CodexAccountQuotaChart items={items} />
      ) : null}
    </PanelWrapper>
  )
}
