/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useAuthStore } from '@/stores/auth-store'

import { getChannelModelDiscovery, refreshChannelModelDiscovery } from '../api'

export function ChannelModelDiscovery(props: {
  channelId: number | null
  disabled?: boolean
}) {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const sessionId = useAuthStore((state) => state.auth.session?.sid)
  const queryKey = [
    'channels',
    'model-discovery',
    userId,
    sessionId,
    props.channelId,
  ]
  const discovery = useQuery({
    queryKey,
    queryFn: () => {
      if (props.channelId === null) throw new Error('Channel ID is required')
      return getChannelModelDiscovery(props.channelId)
    },
    enabled: props.channelId !== null && userId !== undefined,
    retry: false,
  })
  const refresh = useMutation({
    mutationFn: (scope: {
      channelId: number
      userId: number
      sessionId: string | undefined
    }) => refreshChannelModelDiscovery(scope.channelId),
    onSuccess: (response, scope) => {
      client.setQueryData(
        [
          'channels',
          'model-discovery',
          scope.userId,
          scope.sessionId,
          scope.channelId,
        ],
        response
      )
    },
  })
  const resetRefresh = refresh.reset
  useEffect(() => {
    resetRefresh()
  }, [userId, sessionId, props.channelId, resetRefresh])
  const error = refresh.error || discovery.error
  const permissionDenied =
    axios.isAxiosError(error) &&
    (error.response?.status === 401 || error.response?.status === 403)
  const evidence =
    !permissionDenied && userId !== undefined ? discovery.data?.data : undefined
  let status = t('Discovery not checked')
  if (evidence?.status === 'manual_unverified') {
    status = t('Manually configured models are unverified')
  }
  if (evidence?.status === 'success') status = t('Model discovery succeeded')
  if (evidence?.status === 'empty') status = t('Upstream returned no models')
  if (evidence?.status === 'refreshing' || refresh.isPending) {
    status = t('Refreshing model discovery')
  }
  if (
    evidence?.status === 'failed' ||
    discovery.isError ||
    refresh.isError ||
    discovery.data?.success === false
  ) {
    status = t('Model discovery failed; previous evidence is retained')
  }
  if (evidence?.status === 'configuration_changed') {
    status = t('Channel configuration changed; refresh discovery')
  }
  if (discovery.isLoading) status = t('Loading...')
  if (permissionDenied || userId === undefined) {
    status = t("You don't have necessary permission")
  }
  return (
    <section
      className='space-y-3 rounded-lg border p-4'
      aria-label={t('Model discovery evidence')}
    >
      <h3 className='font-medium'>{t('Model discovery evidence')}</h3>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Discovery uses saved channel settings. It does not enable models, grant access, or prove request success.'
        )}
      </p>
      {props.channelId === null ? (
        <p className='text-sm'>
          {t(
            'Save the channel to discover models. Manually entered models are unverified.'
          )}
        </p>
      ) : (
        <>
          <p role='status' className='text-sm'>
            {status}
          </p>
          {evidence?.stale && (
            <p className='text-sm text-amber-600'>
              {t('Stale discovery evidence')}
            </p>
          )}
          {evidence && (
            <dl className='grid gap-1 text-sm break-all'>
              <div>
                <dt className='inline font-medium'>
                  {t('Discovery source')}:{' '}
                </dt>
                <dd className='inline'>{evidence.source}</dd>
              </div>
              <div>
                <dt className='inline font-medium'>
                  {t('Last successful discovery')}:{' '}
                </dt>
                <dd className='inline'>
                  {evidence.fetched_at
                    ? new Date(evidence.fetched_at * 1000).toLocaleString(
                        i18n.language
                      )
                    : t('Never')}
                </dd>
              </div>
              <div>
                <dt className='inline font-medium'>
                  {t('Last discovery check')}:{' '}
                </dt>
                <dd className='inline'>
                  {evidence.checked_at
                    ? new Date(evidence.checked_at * 1000).toLocaleString(
                        i18n.language
                      )
                    : t('Never')}
                </dd>
              </div>
            </dl>
          )}
          <Button
            type='button'
            variant='outline'
            disabled={
              props.disabled ||
              userId === undefined ||
              permissionDenied ||
              discovery.isLoading ||
              refresh.isPending
            }
            onClick={() => {
              if (props.channelId !== null && userId !== undefined) {
                refresh.mutate({
                  channelId: props.channelId,
                  userId,
                  sessionId,
                })
              }
            }}
          >
            {t('Refresh model discovery')}
          </Button>
          {!!evidence?.models.length && (
            <ul
              aria-label={t('Discovered upstream models')}
              className='max-h-40 space-y-1 overflow-auto font-mono text-xs break-all'
            >
              {evidence.models.map((model) => (
                <li key={model}>{model}</li>
              ))}
            </ul>
          )}
        </>
      )}
    </section>
  )
}
