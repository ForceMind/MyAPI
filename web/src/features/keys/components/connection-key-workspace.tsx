/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button, buttonVariants } from '@/components/ui/button'
import { getConnectionPools } from '@/features/connections/connection-pools-api'
import type { ConnectionKeyContext } from '@/features/connections/key-context'
import { parseFreshSelfUser } from '@/features/dashboard/components/personal-workbench/workbench-state'
import { PoolKeyAllocations } from '@/features/personal-upstream/components/pool-key-allocations'
import { getPoolUsage } from '@/features/personal-upstream/pool-usage-api'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { SubscriptionShareDialog } from './subscription-share-dialog'

export function ConnectionKeyWorkspace(props: {
  context: ConnectionKeyContext
}) {
  const user = useAuthStore((state) => state.auth.user)
  const sid = useAuthStore((state) => state.auth.session?.sid)
  return (
    <ConnectionKeySession
      key={`${user?.id}:${user?.role}:${sid}:${JSON.stringify(props.context)}`}
      context={props.context}
    />
  )
}

function ConnectionKeySession(props: { context: ConnectionKeyContext }) {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const sid = useAuthStore((state) => state.auth.session?.sid)
  const page = props.context.poolPage ?? 1
  const [editing, setEditing] = useState(false)
  const showLogs = useIsSidebarModuleVisible('/usage-logs/common')
  const channelId = props.context.connection.connection
  const identity = [user?.id, user?.role, sid]
  const self = useQuery({
    queryKey: ['connection-keys-self', ...identity],
    queryFn: async ({ signal }) => {
      const response = await api.get('/api/user/self', {
        signal,
        disableDuplicate: true,
        skipErrorHandler: true,
      })
      return parseFreshSelfUser(response.data, user?.id ?? -1)
    },
    enabled: user?.role === ROLE.SUPER_ADMIN && Boolean(sid),
    staleTime: 0,
    refetchOnMount: 'always',
    retry: false,
    refetchOnWindowFocus: false,
  })
  const allowed =
    user?.role === ROLE.SUPER_ADMIN &&
    Boolean(sid) &&
    self.isSuccess &&
    !self.isFetching &&
    !self.isError &&
    self.data.role === ROLE.SUPER_ADMIN
  const pools = useQuery({
    queryKey: ['connection-key-pools', ...identity, channelId, page],
    queryFn: ({ signal }) => getConnectionPools(channelId, page, signal),
    enabled: allowed && !props.context.pool,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const usage = useQuery({
    queryKey: [
      'connection-key-usage',
      ...identity,
      channelId,
      props.context.pool,
    ],
    queryFn: ({ signal }) =>
      getPoolUsage(props.context.pool ?? -1, channelId, signal),
    enabled: allowed && Boolean(props.context.pool),
    retry: false,
    refetchOnWindowFocus: false,
  })
  useEffect(() => {
    if (!allowed || usage.isError) setEditing(false)
  }, [allowed, usage.isError])
  const allocation =
    allowed && !usage.isError
      ? usage.data?.allocations.find(
          (item) => item.token_id === props.context.key
        )
      : undefined
  const canEdit =
    allocation &&
    usage.data?.state !== 'closed' &&
    !['deleted', 'missing', 'unknown'].includes(allocation.token_state)
  return (
    <main
      id='content'
      tabIndex={-1}
      className='pc-page flex min-w-0 flex-col gap-5'
    >
      <h1 className='text-2xl font-semibold'>
        {t('Associated Key shares')} · #{channelId}
      </h1>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Saved allocations only. These relationships do not prove current routing access or exact per-Key upstream usage.'
        )}
      </p>
      <div className='flex flex-wrap gap-2'>
        <Link
          to='/connections'
          search={props.context.connection}
          className={`${buttonVariants({ variant: 'outline' })} h-auto min-h-11 max-w-full whitespace-normal`}
        >
          {t('Back to connections')}
        </Link>
        {props.context.pool && (
          <Link
            to='/keys'
            search={{
              connectionContext: {
                connection: props.context.connection,
                poolPage: props.context.poolPage,
              },
            }}
            className={`${buttonVariants({ variant: 'outline' })} h-auto min-h-11 max-w-full whitespace-normal`}
          >
            {t('All saved share windows')}
          </Link>
        )}
        {showLogs && allowed && (
          <Link
            to='/usage-logs/$section'
            params={{ section: 'common' }}
            search={{
              channel: String(channelId),
              returnToKeys: true,
              connectionContext: props.context,
              page: 1,
              pageSize: 20,
            }}
            className={`${buttonVariants({ variant: 'outline' })} h-auto min-h-11 whitespace-normal`}
          >
            {t('View connection requests')}
          </Link>
        )}
      </div>
      {showLogs && allowed && (
        <p className='text-muted-foreground text-xs'>
          {t(
            'Call records are filtered by connection, not by Key. Key names are not stable identities.'
          )}
        </p>
      )}
      {self.isFetching && <p role='status'>{t('Loading...')}</p>}
      {!self.isFetching && !allowed && (
        <div role='alert'>
          <p>
            {t('Root access is required to match Key shares to this account.')}
          </p>
          <Button
            variant='outline'
            onClick={() => void self.refetch()}
            disabled={user?.role !== ROLE.SUPER_ADMIN || !sid}
          >
            {t('Retry')}
          </Button>
        </div>
      )}
      {allowed && !props.context.pool && (
        <section
          aria-label={t('All saved share windows')}
          className='space-y-3'
        >
          {pools.isPending && <p role='status'>{t('Loading...')}</p>}
          {pools.isError && (
            <div role='alert'>
              <p>{t('Unable to load Key shares')}</p>
              <Button onClick={() => void pools.refetch()}>{t('Retry')}</Button>
            </div>
          )}
          {pools.isSuccess && (
            <>
              {pools.data.items.length === 0 && (
                <p>{t('No saved Key allocations for this connection.')}</p>
              )}
              <ul className='space-y-3'>
                {pools.data.items.map((pool) => (
                  <li
                    key={pool.pool_id}
                    className='min-w-0 rounded-lg border p-4 [overflow-wrap:anywhere]'
                  >
                    <Link
                      to='/keys'
                      search={{
                        connectionContext: {
                          ...props.context,
                          pool: pool.pool_id,
                          key: undefined,
                        },
                      }}
                      className='text-primary underline'
                    >
                      {t('Pool #{{id}}', { id: pool.pool_id })} ·{' '}
                      {pool.window_type}
                    </Link>
                    <p>
                      {pool.enabled
                        ? t('Open share window')
                        : t('Closed share window; historical allocations')}
                    </p>
                  </li>
                ))}
              </ul>
              <nav
                aria-label={t('Pagination')}
                className='flex flex-wrap gap-2'
              >
                <Button
                  variant='outline'
                  disabled={page === 1}
                  nativeButton={false}
                  render={
                    <Link
                      to='/keys'
                      search={{
                        connectionContext: {
                          ...props.context,
                          poolPage: page - 1,
                        },
                      }}
                    />
                  }
                >
                  {t('Previous')}
                </Button>
                <span>{page}</span>
                <Button
                  variant='outline'
                  disabled={!pools.data.has_more}
                  nativeButton={false}
                  render={
                    <Link
                      to='/keys'
                      search={{
                        connectionContext: {
                          ...props.context,
                          poolPage: page + 1,
                        },
                      }}
                    />
                  }
                >
                  {t('Next')}
                </Button>
              </nav>
            </>
          )}
        </section>
      )}
      {allowed && props.context.pool && (
        <section
          className='min-w-0 space-y-4'
          aria-label={t('Key share settings')}
        >
          {usage.isPending && <p role='status'>{t('Loading...')}</p>}
          {usage.isError && (
            <div role='alert'>
              <p>{t('Unable to load window usage')}</p>
              <Button onClick={() => void usage.refetch()}>{t('Retry')}</Button>
            </div>
          )}
          {usage.isSuccess && !usage.isError && (
            <>
              <h2>
                {t('Pool #{{id}}', { id: props.context.pool })} ·{' '}
                {usage.data.state === 'closed'
                  ? t('Closed share window; historical allocations')
                  : t('Open share window')}
              </h2>
              {props.context.key && (
                <div className='space-y-3 rounded-lg border p-4'>
                  <h3>{t('Key #{{id}}', { id: props.context.key })}</h3>
                  {!allocation && (
                    <p role='alert'>
                      {t(
                        'This Key has no saved allocation in the selected window.'
                      )}
                    </p>
                  )}
                  {allocation && (
                    <>
                      <p className='[overflow-wrap:anywhere]'>
                        {allocation.token_name}
                      </p>
                      <Button
                        disabled={!canEdit}
                        onClick={() => setEditing(true)}
                      >
                        {t('Manage Key shares')}
                      </Button>
                    </>
                  )}
                </div>
              )}
              <PoolKeyAllocations
                usage={usage.data}
                connectionContext={props.context}
              />
            </>
          )}
        </section>
      )}
      {allowed &&
        editing &&
        canEdit &&
        props.context.key &&
        props.context.pool && (
          <SubscriptionShareDialog
            tokenId={props.context.key}
            scope={{ poolId: props.context.pool, channelId }}
            onClose={() => {
              setEditing(false)
              void usage.refetch()
            }}
          />
        )}
    </main>
  )
}
