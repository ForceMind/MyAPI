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
    <div className='bg-card p-card flex min-w-0 flex-col gap-4 rounded-xl border'>
      <dl className='grid min-w-0 gap-4 sm:grid-cols-2'>
        <div className='flex min-w-0 flex-col gap-1'>
          <dt className='text-caption text-muted-foreground'>
            {t('Base URL')}
          </dt>
          <dd className='flex min-w-0 items-center gap-1'>
            <code className='text-data min-w-0 font-mono wrap-anywhere'>
              {baseUrl}
            </code>
            <CopyButton
              value={baseUrl}
              size='icon'
              variant='ghost'
              tooltip={t('Copy Base URL')}
              aria-label={t('Copy Base URL')}
            />
          </dd>
          <dd className='text-caption text-muted-foreground'>
            {serverAddress
              ? t('From the server address setting')
              : t('Current access address')}
          </dd>
        </div>
        <div className='flex min-w-0 flex-col gap-1'>
          <dt className='text-caption text-muted-foreground'>{t('API Key')}</dt>
          <dd className='min-w-0'>
            <ApiKeyCell apiKey={props.apiKey} />
          </dd>
          <dd className='text-caption text-muted-foreground'>
            {t('The full Key is read only when you reveal or copy it.')}
          </dd>
        </div>
      </dl>
      {role === ROLE.SUPER_ADMIN && (
        <div className='flex min-w-0 flex-wrap gap-2 border-t pt-4'>
          <Button variant='outline' onClick={() => setMaintaining(true)}>
            <HugeiconsIcon
              icon={SecurityCheckIcon}
              data-icon='inline-start'
              aria-hidden='true'
            />
            {t('Credential maintenance')}
          </Button>
        </div>
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
