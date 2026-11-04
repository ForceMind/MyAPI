/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldLegend, FieldSet } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useSecurityRequestScope } from '../hooks/use-security-request-scope'
import { managementKeyFormSchema, managementKeyScopeLabel, parseManagementResourceIDs, type ManagementKeyForm } from '../lib/management-key-form'
import { managementKeySessionKey } from '../lib/management-key-session'
import { createManagementKey, isManagementKeyRejection, managementKeyScopes } from '../management-key-api'

type ManagementKeyCreateDialogProps = {
  scopeKey: string
  onClose: () => void
  onCreated: () => void
  onUnconfirmed: () => void
  returnFocus: () => HTMLElement | null
}

export function ManagementKeyCreateDialog(props: ManagementKeyCreateDialogProps) {
  const { t } = useTranslation()
  const requestScope = useSecurityRequestScope(props.scopeKey)
  const [review, setReview] = useState<ManagementKeyForm | null>(null)
  const [busy, setBusy] = useState(false)
  const [secret, setSecret] = useState<string | null>(null)
  const [feedback, setFeedback] = useState('')
  const [unknown, setUnknown] = useState(false)
  const currentUser = useAuthStore((state) => state.auth.user)
  const channelPermissions = currentUser?.permissions?.admin_permissions?.channel
  const channelRead = currentUser?.role === ROLE.SUPER_ADMIN || channelPermissions?.read === true
  const channelOperate = currentUser?.role === ROLE.SUPER_ADMIN || channelPermissions?.operate === true
  const form = useForm<ManagementKeyForm>({
    resolver: zodResolver(managementKeyFormSchema),
    defaultValues: {
      name: '', days: '7', channelIDs: '', tokenIDs: '',
      scopes: channelRead ? ['channels.read', 'tokens.read', 'logs.read', 'system.read'] : ['tokens.read', 'logs.read', 'system.read'],
    },
  })
  const errors = form.formState.errors

  async function create() {
    if (!review || busy || unknown || secret || managementKeySessionKey() !== props.scopeKey) return
    setBusy(true)
    setFeedback('')
    try {
      const result = await requestScope.run((signal) => createManagementKey({
        name: review.name,
        scopes: review.scopes,
        channel_ids: parseManagementResourceIDs(review.channelIDs) ?? [],
        token_ids: parseManagementResourceIDs(review.tokenIDs) ?? [],
        expires_at: Math.floor(Date.now() / 1000) + Number(review.days) * 86400,
      }, signal))
      if (!result.current || managementKeySessionKey() !== props.scopeKey) return
      setSecret(result.value.secret)
      setBusy(false)
      props.onCreated()
    } catch (error) {
      setBusy(false)
      const rejected = isManagementKeyRejection(error)
      setUnknown(!rejected)
      if (!rejected) props.onUnconfirmed()
      setFeedback(rejected
        ? t('The management key request was rejected. Check your session and permissions.')
        : t('The result is unknown and the key may already exist. Close this dialog and inspect the key list before creating another key.'))
    }
  }

  async function copySecret() {
    if (!secret || busy || managementKeySessionKey() !== props.scopeKey) return
    setBusy(true)
    try {
      const result = await requestScope.run(() => navigator.clipboard.writeText(secret))
      if (!result.current || managementKeySessionKey() !== props.scopeKey) return
      setFeedback(t('Management key copied. Store it securely.'))
      setBusy(false)
    } catch {
      setFeedback(t('Copy failed. Select and copy the management key manually.'))
      setBusy(false)
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => { if (!open && !busy) props.onClose() }}
      title={secret ? t('Save your management key') : t('Create management key')}
      description={t('Use a trusted tool on your own device. Never paste management keys into a chat.')}
      showCloseButton={!busy}
      returnFocus={props.returnFocus}
      contentClassName='sm:max-w-lg [&_[data-myapi-button]]:min-h-11 [&_input]:min-h-11'
      footer={<>
        <Button variant='outline' disabled={busy} onClick={props.onClose}>{t('Close')}</Button>
        {secret && <Button disabled={busy} onClick={() => void copySecret()}>{t('Copy management key')}</Button>}
        {!secret && review && !unknown && <Button disabled={busy} onClick={() => void create()}>{busy ? t('Creating...') : t('Confirm creation')}</Button>}
      </>}
    >
      {feedback && <p role={unknown ? 'alert' : 'status'} className='mb-4 break-words text-sm'>{feedback}</p>}
      {secret && <FieldGroup>
        <Field>
          <FieldLabel htmlFor='management-secret'>{t('Management key secret')}</FieldLabel>
          <Input id='management-secret' value={secret} readOnly autoComplete='off' spellCheck={false} className='font-mono' />
          <FieldDescription>{t('This secret is shown only now and is erased when this dialog closes. Store it in your local secret manager.')}</FieldDescription>
        </Field>
      </FieldGroup>}
      {!secret && review && <div className='space-y-3 text-sm break-words' aria-busy={busy}>
        <p>{t('Create key named {{name}}?', { name: review.name })}</p>
        <p>{review.scopes.map((scope) => managementKeyScopeLabel(scope, t)).join(', ')}</p>
        <p>{t('Expires in {{days}} days', { days: review.days })}</p>
        <p>{t('Channel IDs')}: {review.channelIDs || t('All authorized resources')}</p>
        <p>{t('API token IDs')}: {review.tokenIDs || t('All authorized resources')}</p>
        <p>{t('An empty ID list allows all resources within your existing authorization. Specific IDs strictly limit access; some aggregate requests may return 403.')}</p>
        <Button variant='outline' disabled={busy || unknown} onClick={() => { setReview(null); setFeedback('') }}>{t('Edit key details')}</Button>
      </div>}
      {!secret && !review && <form onSubmit={form.handleSubmit(setReview)} noValidate>
        <FieldSet disabled={busy}>
          <FieldGroup>
            <Field data-invalid={!!errors.name}>
              <FieldLabel htmlFor='management-name'>{t('Management key name')}</FieldLabel>
              <Input id='management-name' maxLength={100} autoComplete='off' aria-invalid={!!errors.name} {...form.register('name')} />
              {errors.name && <FieldDescription role='alert'>{t('Enter a name between 1 and 100 characters.')}</FieldDescription>}
            </Field>
            <FieldSet>
              <FieldLegend>{t('Management permissions')}</FieldLegend>
              <Controller control={form.control} name='scopes' render={({ field }) => <FieldGroup className='gap-3'>
                {managementKeyScopes.map((scope) => <Field key={scope} orientation='horizontal'>
                  <Checkbox id={`management-${scope}`} checked={field.value.includes(scope)} disabled={(scope === 'channels.read' && !channelRead) || (scope === 'channels.operate' && !channelOperate)} aria-invalid={!!errors.scopes} onCheckedChange={(checked) => field.onChange(checked ? [...field.value, scope] : field.value.filter((value) => value !== scope))} />
                  <FieldLabel htmlFor={`management-${scope}`}>{managementKeyScopeLabel(scope, t)}</FieldLabel>
                </Field>)}
              </FieldGroup>} />
              {errors.scopes && <FieldDescription role='alert'>{t('Select at least one management permission.')}</FieldDescription>}
            </FieldSet>
            <Field data-invalid={!!errors.days}>
              <FieldLabel htmlFor='management-days'>{t('Validity in days')}</FieldLabel>
              <Input id='management-days' type='number' min={1} max={90} aria-invalid={!!errors.days} {...form.register('days')} />
              <FieldDescription>{t('Choose 1 to 90 days. The default is 7 days.')}</FieldDescription>
              {errors.days && <FieldDescription role='alert'>{t('Validity must be a whole number from 1 to 90 days.')}</FieldDescription>}
            </Field>
            <details>
              <summary className='cursor-pointer py-2'>{t('Advanced resource limits')}</summary>
              <FieldGroup className='mt-3'>
                <FieldDescription>{t('An empty ID list allows all resources within your existing authorization. Specific IDs strictly limit access; some aggregate requests may return 403.')}</FieldDescription>
                <Field data-invalid={!!errors.channelIDs}>
                  <FieldLabel htmlFor='management-channels'>{t('Channel IDs')}</FieldLabel>
                  <Input id='management-channels' aria-invalid={!!errors.channelIDs} {...form.register('channelIDs')} />
                  {errors.channelIDs && <FieldDescription role='alert'>{t('Enter comma-separated positive integer IDs. Empty entries and invalid numbers are not allowed.')}</FieldDescription>}
                </Field>
                <Field data-invalid={!!errors.tokenIDs}>
                  <FieldLabel htmlFor='management-tokens'>{t('API token IDs')}</FieldLabel>
                  <Input id='management-tokens' aria-invalid={!!errors.tokenIDs} {...form.register('tokenIDs')} />
                  {errors.tokenIDs && <FieldDescription role='alert'>{t('Enter comma-separated positive integer IDs. Empty entries and invalid numbers are not allowed.')}</FieldDescription>}
                </Field>
              </FieldGroup>
            </details>
            <Button type='submit'>{t('Review management key')}</Button>
          </FieldGroup>
        </FieldSet>
      </form>}
    </Dialog>
  )
}
