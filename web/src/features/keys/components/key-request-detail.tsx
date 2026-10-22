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
import { useAuthStore } from '@/stores/auth-store'

import { getApiKey } from '../api'
import type { ApiKey } from '../types'

export function KeyRequestDetail(props: {
  keyId: number
  onBack: () => void
  onViewRequests: (key: ApiKey) => void
}) {
  const { t } = useTranslation()
  const userId = useAuthStore((s) => s.auth.user?.id)
  const sid = useAuthStore((s) => s.auth.session?.sid)
  const query = useQuery({
    queryKey: ['key-request-detail', userId, sid, props.keyId],
    enabled: Boolean(userId && sid),
    retry: false,
    queryFn: async ({ signal }) => {
      const response = await getApiKey(props.keyId, signal)
      if (
        !response.success ||
        !response.data ||
        response.data.id !== props.keyId
      ) {
        throw new Error(
          t('This Key is unavailable or belongs to another account.')
        )
      }
      return response.data
    },
  })
  const key = query.isError ? undefined : query.data
  return (
    <section
      className='mx-auto w-full max-w-3xl space-y-4 p-4 md:p-6'
      aria-label={t('API Key')}
    >
      <Button variant='outline' onClick={props.onBack}>
        {t('Back to Keys')}
      </Button>
      <h1 className='text-xl font-semibold'>
        {t('API Key')} #{props.keyId}
      </h1>
      {query.isLoading && <p>{t('Loading...')}</p>}
      {query.isError && (
        <div role='alert' className='space-y-3 rounded-lg border p-4'>
          <p>{t('This Key is unavailable or belongs to another account.')}</p>
          <Button variant='outline' onClick={() => void query.refetch()}>
            {t('Retry')}
          </Button>
        </div>
      )}
      {key && (
        <div className='space-y-4 rounded-lg border p-4'>
          <h2 className='font-medium break-words'>{key.name}</h2>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Requests are matched by the saved Key ID, including before a rename.'
            )}
          </p>
          <Button
            className='min-h-11 whitespace-normal'
            onClick={() => props.onViewRequests(key)}
          >
            {t('View Key requests')}
          </Button>
        </div>
      )}
    </section>
  )
}
