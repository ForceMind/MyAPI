/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useTranslation } from 'react-i18next'

import { Alert, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'

import type { useUpstreamAccounts } from '../use-upstream-accounts'

export function AccountsState(props: { data: ReturnType<typeof useUpstreamAccounts> }) {
  const { t } = useTranslation()
  const data = props.data
  if (data.selfQuery.isLoading || data.accountsQuery.isLoading) return <div role='status' aria-label={t('Loading upstream accounts')}><Skeleton className='h-64 w-full' /></div>
  if (data.selfQuery.isError) return <Alert variant='destructive'><AlertTitle>{t('Unable to confirm channel permissions')}</AlertTitle><Button variant='outline' onClick={() => void data.selfQuery.refetch()}>{t('Retry')}</Button></Alert>
  if (!data.canRead) return <Alert><AlertTitle>{t('Channel read permission is required to view upstream account windows.')}</AlertTitle></Alert>
  if (data.accountsQuery.isError) return <Alert variant='destructive' data-testid='accounts-error-state'><AlertTitle>{t('Unable to load upstream account windows')}</AlertTitle><Button variant='outline' onClick={() => void data.accountsQuery.refetch()}>{t('Retry')}</Button></Alert>
  return <Empty className='rounded-xl border bg-card' data-testid='accounts-empty-state'><EmptyHeader><EmptyTitle>{t('No upstream account windows were reported')}</EmptyTitle><EmptyDescription>{t('Existing channel logins are kept. Open Channels to inspect the connection.')}</EmptyDescription></EmptyHeader></Empty>
}
