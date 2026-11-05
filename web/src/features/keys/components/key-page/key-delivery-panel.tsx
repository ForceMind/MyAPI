/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { SecurityCheckIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ComboboxInput } from '@/components/ui/combobox-input'
import { Label } from '@/components/ui/label'
import { useStatus } from '@/hooks/use-status'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { ApiKey } from '../../types'
import { useKeyClientModels } from '../../hooks/use-key-client-models'
import { useKeyMaintenanceSession } from '../../hooks/use-key-maintenance-session'
import { keyClientAddresses, keyClientModelAllowed, keyClientModels } from '../../lib/client-configuration'
import { ApiKeyCell } from '../api-keys-cells'
import { useApiKeys } from '../api-keys-provider'
import { CredentialMaintenanceDialog } from '../credential-maintenance-dialog'
import { CCSwitchDialog } from '../dialogs/cc-switch-dialog'

/**
 * What a client needs to use this Key: the base URL and the Key itself,
 * which stays masked until the user reveals or copies it. Root also
 * maintains the Key's stored credential here.
 */
export function KeyDeliveryPanel(props: { apiKey: ApiKey; onViewScope: () => void }) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const modelId = useId()
  const isCurrentSession = useKeyMaintenanceSession()
  const { resolveRealKey } = useApiKeys()
  const role = useAuthStore((state) => state.auth.user?.role)
  const [maintaining, setMaintaining] = useState(false)
  const [model, setModel] = useState('')
  const [importKey, setImportKey] = useState('')
  const [importPending, setImportPending] = useState(false)
  const directory = useKeyClientModels(isCurrentSession())
  const candidates = directory.isError ? [] : keyClientModels(directory.data ?? [], props.apiKey)
  const modelAllowed = keyClientModelAllowed(model, props.apiKey)
  const selectedModel = modelAllowed ? model : ''
  const address = keyClientAddresses(status?.server_address, window.location.origin)
  const baseUrl = address.openAI
  const limitedModelCount = new Set((props.apiKey.model_limits ?? '').split(',').filter(Boolean)).size
  let modelRestriction = t('No Key-level model restriction')
  if (props.apiKey.model_limits_enabled) {
    modelRestriction = limitedModelCount > 0
      ? t('Model restriction enabled: {{count}} models', { count: limitedModelCount })
      : t('Model restriction enabled: no models allowed')
  }

  const openImport = async () => {
    if (!selectedModel || importPending || !isCurrentSession()) return
    setImportPending(true)
    try {
      const key = await resolveRealKey(props.apiKey.id)
      if (key && isCurrentSession()) setImportKey(key)
    } finally {
      if (isCurrentSession()) setImportPending(false)
    }
  }

  if (!isCurrentSession()) return null

  return (
    <div className='flex min-w-0 flex-col gap-4'>
      <section
        aria-labelledby='key-connection-title'
        className='bg-card p-card flex min-w-0 flex-col gap-4 rounded-xl border'
      >
        <h2 id='key-connection-title' className='text-title'>
          {t('Connection details')}
        </h2>
        <dl className='grid min-w-0 grid-cols-1 gap-5 lg:grid-cols-2'>
          <div className='flex min-w-0 flex-col gap-1'>
            <dt className='text-caption text-muted-foreground'>
              {t('Base URL')}
            </dt>
            <dd className='flex min-w-0 items-start gap-2'>
              <code className='text-data min-w-0 flex-1 font-mono wrap-anywhere'>
                {baseUrl}
              </code>
              <CopyButton
                value={baseUrl}
                size='icon'
                variant='ghost'
                className='shrink-0 pointer-coarse:min-h-11 pointer-coarse:min-w-11'
                tooltip={t('Copy Base URL')}
                aria-label={t('Copy Base URL')}
              />
            </dd>
            <dd className='text-caption text-muted-foreground wrap-anywhere'>
              {address.source === 'setting'
                ? t('From the server address setting')
                : t('Current access address')}
            </dd>
          </div>
          <div className='flex min-w-0 flex-col gap-1'>
            <dt className='text-caption text-muted-foreground'>{t('API Key')}</dt>
            <dd className='min-w-0 pointer-coarse:[&_button]:min-h-11 pointer-coarse:[&_button]:min-w-11'>
              <ApiKeyCell apiKey={props.apiKey} />
            </dd>
            <dd className='text-caption text-muted-foreground wrap-anywhere'>
              {t('The full Key is read only when you reveal, copy, or import it.')}
            </dd>
          </div>
        </dl>
        <p className='text-caption text-muted-foreground wrap-anywhere'>
          {t('OpenAI-compatible clients use this /v1 Base URL. CC Switch keeps the selected application’s endpoint format.')}
        </p>
        <div className='flex min-w-0 flex-col gap-2 border-t pt-4 [&_[role=option]]:min-h-11 [&_[role=option]]:items-start [&_[role=option]_span]:overflow-visible [&_[role=option]_span]:text-clip [&_[role=option]_span]:whitespace-normal [&_[role=option]_span]:wrap-anywhere'>
          <Label htmlFor={modelId}>{t('Candidate model')}</Label>
          <ComboboxInput
            id={modelId}
            className='min-h-11'
            options={candidates.map((candidate) => ({ value: candidate, label: candidate }))}
            value={model}
            onValueChange={setModel}
            placeholder={t('Select a model')}
            emptyText={t('No candidate models for this Key')}
            allowCustomValue
            aria-invalid={model && !modelAllowed ? true : undefined}
          />
          {model && !modelAllowed && <p role='alert' className='text-destructive text-caption'>{t('One or more model IDs are outside this Key’s restrictions.')}</p>}
          {directory.isFetching && <p role='status' className='text-caption text-muted-foreground'>{t('Loading...')}</p>}
          {directory.isError && (
            <Alert variant='destructive'>
              <AlertTitle>{t('Unable to load available models')}</AlertTitle>
              <AlertDescription>{directory.error.message}</AlertDescription>
              <Button variant='outline' className='mt-3 min-h-11' disabled={directory.isFetching} onClick={() => void directory.refetch()}>{t('Retry')}</Button>
            </Alert>
          )}
          {!directory.isPending && !directory.isError && candidates.length === 0 && (
            <p className='text-caption text-muted-foreground'>{t('No candidate models for this Key')}</p>
          )}
          <p className='text-caption text-muted-foreground'>
            {t('Candidates follow this Key’s restrictions. Protocol support and upstream availability have not been tested.')}
          </p>
          <p className='text-caption text-muted-foreground'>{t('You can enter a model ID manually. Its protocol and availability remain unverified.')}</p>
          <div className='flex min-w-0 flex-wrap gap-2 pointer-coarse:[&_button]:min-h-11'>
            {selectedModel && <CopyButton value={selectedModel} variant='outline' size='sm' tooltip={t('Copy model name')} aria-label={t('Copy model name')}>{t('Copy model name')}</CopyButton>}
            <Button variant='outline' disabled={!selectedModel || importPending} aria-busy={importPending} onClick={() => void openImport()}>{t('Import to CC Switch')}</Button>
          </div>
        </div>
        <div className='flex min-w-0 flex-col items-start gap-2 border-t pt-4'>
          <h3 className='text-data font-medium'>{t('Key restrictions')}</h3>
          <p className='text-caption text-muted-foreground wrap-anywhere'>
            {modelRestriction}
          </p>
          <p className='text-caption text-muted-foreground wrap-anywhere'>{t('Access profile')}: {props.apiKey.access_profile?.label || props.apiKey.group || t('Default')}{props.apiKey.access_profile?.enabled === false && ` · ${t('Disabled')}`}</p>
          <p className='text-caption text-muted-foreground wrap-anywhere'>{t('IP Restriction')}: {props.apiKey.allow_ips || t('None')}</p>
          <Button variant='outline' className='pointer-coarse:min-h-11' onClick={props.onViewScope}>{t('View Key restrictions')}</Button>
        </div>
      </section>
      {role === ROLE.SUPER_ADMIN && (
        <section
          aria-labelledby='key-credential-title'
          className='bg-card p-card flex min-w-0 flex-col items-start gap-3 rounded-xl border'
        >
          <h2 id='key-credential-title' className='text-title'>
            {t('Credential maintenance')}
          </h2>
          <Button
            variant='outline'
            className='pointer-coarse:min-h-11'
            onClick={() => setMaintaining(true)}
          >
            <HugeiconsIcon
              icon={SecurityCheckIcon}
              data-icon='inline-start'
              aria-hidden='true'
            />
            {t('Credential maintenance')}
          </Button>
        </section>
      )}
      {maintaining && (
        <CredentialMaintenanceDialog
          tokenId={props.apiKey.id}
          onClose={() => setMaintaining(false)}
        />
      )}
      {importKey && (
        <CCSwitchDialog open tokenKey={importKey} apiKey={props.apiKey} selectedModel={selectedModel} address={address} onOpenChange={(open) => { if (!open) setImportKey('') }} />
      )}
    </div>
  )
}
