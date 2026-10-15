/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Link, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { getPlaygroundKeys } from '@/features/playground/api'
import { createPlaygroundKeyHandoff } from '@/features/playground/lib/playground-key-handoff'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { useAuthStore } from '@/stores/auth-store'

import { getPlaygroundKeyCondition } from './workbench-state'

export function OverviewKeys() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const showPlayground = useIsSidebarModuleVisible('/playground')
  const [selected, setSelected] = useState('')
  const keysQuery = useQuery({ queryKey: ['personal-console', 'keys', user?.id, sessionId], queryFn: getPlaygroundKeys, retry: false, refetchOnWindowFocus: false })
  const keys = keysQuery.isError ? undefined : keysQuery.data
  const selectedKey = keys?.find((key) => String(key.id) === selected)
  const available = Boolean(selectedKey && getPlaygroundKeyCondition(selectedKey) === 'available')
  const openPlayground = () => {
    // A fresh list and the current user/session must still confirm the explicit choice.
    if (!available || !selectedKey || !user || !sessionId) return
    createPlaygroundKeyHandoff({ userId: user.id, sessionId, keyId: selectedKey.id })
    void navigate({ to: '/playground' })
  }
  return <Card className='pc-card min-w-0' data-testid='overview-keys'>
    <CardHeader><CardTitle><h2>{t('Existing API Keys')}</h2></CardTitle></CardHeader>
    <CardContent className='flex flex-col gap-4'>
      {keysQuery.isLoading ? <Skeleton className='h-16 w-full' /> : null}
      {keysQuery.isError ? <Alert variant='destructive' data-testid='keys-error-state'><AlertTitle>{t('Unable to load Keys')}</AlertTitle><Button variant='outline' onClick={() => void keysQuery.refetch()}>{t('Retry')}</Button></Alert> : null}
      {keys ? <><p className='text-5xl font-semibold tabular-nums'>{keys.length}</p><p className='text-muted-foreground'>{t('Keep using your existing Keys')}</p>{keys.length === 0 ? <p data-testid='keys-empty-state'>{t('No API Keys yet')}</p> : null}</> : null}
      <Button nativeButton={false} render={<Link to='/keys' />}>{t('Manage API Keys')}</Button>
      {showPlayground ? <details className='min-w-0 border-t pt-4'><summary className='cursor-pointer text-sm font-medium'>{t('Test with an existing Key')}</summary><div className='mt-4 flex min-w-0 flex-col gap-3'>
        <label htmlFor='personal-test-key' className='text-sm'>{t('Select an API Key')}</label>
        <select id='personal-test-key' className='bg-background h-10 w-full min-w-0 rounded-md border px-2' value={selected} onChange={(event) => setSelected(event.target.value)} disabled={!keys?.length}>
          <option value=''>{t('Choose a Key')}</option>{keys?.map((key) => <option key={key.id} value={key.id}>{key.name}</option>)}
        </select>
        <p className='text-muted-foreground text-xs break-words' data-testid='request-flow-selected-key'>{selectedKey?.name || t('No Key selected')}</p>
        <Button variant='outline' disabled={!available || !sessionId} onClick={openPlayground}>{t('Open Playground')}</Button>
      </div></details> : null}
    </CardContent>
  </Card>
}
