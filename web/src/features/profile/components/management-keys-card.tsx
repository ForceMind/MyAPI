/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { SummaryDisclosure } from '@/components/summary-disclosure'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useAuthStore } from '@/stores/auth-store'

import { useSecurityRequestScope } from '../hooks/use-security-request-scope'
import { managementKeyScopeLabel } from '../lib/management-key-form'
import { managementKeySessionKey } from '../lib/management-key-session'
import { isManagementKeyRejection, listManagementKeys, revokeManagementKey, type ManagementKeyMetadata } from '../management-key-api'
import { ManagementKeyCreateDialog } from './management-key-create-dialog'

export function ManagementKeysCard() {
  const scopeKey = useAuthStore((state) => managementKeySessionKey(state.auth))
  if (!scopeKey) return null
  return <ManagementKeysPanel key={scopeKey} scopeKey={scopeKey} />
}

function ManagementKeysPanel(props: { scopeKey: string }) {
  const { t, i18n } = useTranslation()
  const scope = useSecurityRequestScope(props.scopeKey)
  const createButton = useRef<HTMLButtonElement>(null)
  const revokeButton = useRef<HTMLElement | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [target, setTarget] = useState<ManagementKeyMetadata | null>(null)
  const [busy, setBusy] = useState(false)
  const [feedback, setFeedback] = useState('')
  const [unknownTarget, setUnknownTarget] = useState<number | null>(null)
  const [verificationRequired, setVerificationRequired] = useState(false)
  const keys = useQuery({
    queryKey: ['profile', 'management-keys', props.scopeKey],
    queryFn: ({ signal }) => listManagementKeys(signal),
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  })

  async function refreshKeys() {
    setVerificationRequired(true)
    const result = await keys.refetch()
    if (managementKeySessionKey() === props.scopeKey && result.isSuccess) {
      setVerificationRequired(false)
    }
  }

  async function revoke() {
    if (!target || busy || unknownTarget === target.id || managementKeySessionKey() !== props.scopeKey) return
    setBusy(true)
    setFeedback('')
    try {
      const result = await scope.run((signal) => revokeManagementKey(target.id, signal))
      if (!result.current || managementKeySessionKey() !== props.scopeKey) return
      setBusy(false)
      revokeButton.current = createButton.current
      setTarget(null)
      void refreshKeys()
    } catch (error) {
      setBusy(false)
      setTarget(null)
      if (isManagementKeyRejection(error)) {
        setFeedback(t('The management key request was rejected. Check your session and permissions.'))
      } else {
        revokeButton.current = createButton.current
        setUnknownTarget(target.id)
        setFeedback(t('The revocation result is unknown and may already have taken effect. Refresh the list to inspect this key; this action will not be replayed.'))
      }
    }
  }

  return <Card data-card-hover='false'>
    <CardHeader>
      <CardTitle><h2>{t('Management keys')}</h2></CardTitle>
      <CardDescription>{t('Issue limited management access for trusted local tools, separately from API tokens and your account access token.')}</CardDescription>
    </CardHeader>
    <CardContent className='min-w-0 space-y-4'>
      <p className='text-muted-foreground text-sm'>{t('Use a trusted tool on your own device. Never paste management keys into a chat.')}{' '}
        <a href='https://github.com/ForceMind/MyAPI/blob/dev/docs/REMOTE_MANAGEMENT.md' target='_blank' rel='noreferrer' className='underline underline-offset-4'>{t('Secure tool setup guide')}</a>
      </p>
      <div className='flex flex-wrap gap-2'>
        <Button ref={createButton} onClick={() => setCreateOpen(true)} disabled={busy || keys.isPending || verificationRequired}>{t('Create management key')}</Button>
        <Button variant='outline' onClick={() => void refreshKeys()} disabled={keys.isFetching || busy}>{t('Refresh key list')}</Button>
      </div>
      {feedback && <p role='alert' className='text-sm break-words'>{feedback}</p>}
      {(keys.isPending || (verificationRequired && keys.isFetching)) && <p role='status'>{t('Loading management keys...')}</p>}
      {keys.isError && <p role='alert'>{t('Management keys could not be loaded. Check your session and management permissions, then refresh the list.')}</p>}
      {keys.isSuccess && !verificationRequired && keys.data.length === 0 && <p className='text-muted-foreground text-sm'>{t('No management keys yet.')}</p>}
      {keys.isSuccess && !verificationRequired && <ul className='space-y-3' aria-label={t('Management key list')}>
        {keys.data.map((key) => {
          let status = t('Active')
          if (key.revoked_at > 0) status = t('Revoked')
          else if (key.expires_at <= Date.now() / 1000) status = t('Expired')
          return <li key={key.id} className='min-w-0 space-y-2 rounded-lg border p-3 text-sm break-words'>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <span className='font-medium'>{key.name}</span><Badge variant='outline'>{status}</Badge>
            </div>
            <p className='font-mono'>{key.prefix}</p>
            <p>{key.scopes.map((value) => managementKeyScopeLabel(value, t)).join(', ')}</p>
            {key.channel_ids.length > 3 ? <SummaryDisclosure summary={t('Channel IDs ({{count}})', { count: key.channel_ids.length })}><p>{key.channel_ids.join(', ')}</p></SummaryDisclosure> : <p>{t('Channel IDs')}: {key.channel_ids.join(', ') || t('All authorized resources')}</p>}
            {key.token_ids.length > 3 ? <SummaryDisclosure summary={t('API token IDs ({{count}})', { count: key.token_ids.length })}><p>{key.token_ids.join(', ')}</p></SummaryDisclosure> : <p>{t('API token IDs')}: {key.token_ids.join(', ') || t('All authorized resources')}</p>}
            <p>{t('Created')}: {new Date(key.created_at * 1000).toLocaleString(i18n.language)}</p>
            <p>{t('Expires')}: {new Date(key.expires_at * 1000).toLocaleString(i18n.language)}</p>
            {key.revoked_at > 0 && <p>{t('Revoked')}: {new Date(key.revoked_at * 1000).toLocaleString(i18n.language)}</p>}
            <Button variant='outline' className='min-h-11' disabled={busy || key.revoked_at > 0 || unknownTarget === key.id} aria-label={t('Revoke management key {{name}}', { name: key.name })} onClick={(event) => { revokeButton.current = event.currentTarget; setTarget(key) }}>{t('Revoke')}</Button>
          </li>
        })}
      </ul>}
      {createOpen && <ManagementKeyCreateDialog scopeKey={props.scopeKey} onClose={() => setCreateOpen(false)} onCreated={() => { void refreshKeys() }} onUnconfirmed={() => { void refreshKeys() }} returnFocus={() => createButton.current} />}
      <ConfirmDialog open={target !== null} onOpenChange={(open) => { if (!open && !busy) setTarget(null) }} title={t('Revoke management key?')} desc={t('Revoke {{name}} ({{prefix}}) immediately? Tools using this key will lose access.', { name: target?.name ?? '', prefix: target?.prefix ?? '' })} confirmText={busy ? t('Revoking...') : t('Confirm revocation')} destructive isLoading={busy} handleConfirm={() => void revoke()} returnFocus={() => revokeButton.current} />
    </CardContent>
  </Card>
}
