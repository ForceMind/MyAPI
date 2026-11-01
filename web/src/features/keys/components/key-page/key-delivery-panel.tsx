/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { SecurityCheckIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { ApiKey } from '../../types'
import { ApiKeyCell } from '../api-keys-cells'
import { CredentialMaintenanceDialog } from '../credential-maintenance-dialog'

/**
 * What a client needs to use this Key: the base URL and the Key itself,
 * which stays masked until the user reveals or copies it. Root also
 * maintains the Key's stored credential here.
 */
export function KeyDeliveryPanel(props: { apiKey: ApiKey }) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const role = useAuthStore((state) => state.auth.user?.role)
  const [maintaining, setMaintaining] = useState(false)
  const serverAddress =
    typeof status?.server_address === 'string'
      ? status.server_address.trim().replace(/\/+$/, '')
      : ''
  const baseUrl = `${serverAddress || window.location.origin}/v1`

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
              {serverAddress
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
              {t('The full Key is read only when you reveal or copy it.')}
            </dd>
          </div>
        </dl>
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
    </div>
  )
}
