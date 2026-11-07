/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { linkButtonClassName } from '@/lib/link-button'

import type { GatewayAddress } from '../flow-board/gateway-core-card'

/** Existing management pages are task destinations, not completion evidence. */
export function AccessStartPanel(props: {
  address: GatewayAddress
  canReadConnections: boolean
  canAddConnection: boolean
  showKeys: boolean
}) {
  const { t } = useTranslation()
  const desktop =
    (window as Window & { electron?: { isElectron?: unknown } }).electron
      ?.isElectron === true
  let connectionDescription = t(
    'Open connections to review saved upstream settings.'
  )
  if (!props.canReadConnections) {
    connectionDescription = t(
      'Ask an administrator to configure a connection for this account.'
    )
  } else if (props.canAddConnection) {
    connectionDescription = t('Open connections to add or manage an upstream.')
  }

  return (
    <section
      aria-labelledby='overview-access-title'
      className='bg-card rounded-card flex min-w-0 flex-col gap-4 border p-4'
    >
      <div className='flex min-w-0 flex-wrap items-start justify-between gap-3'>
        <h2 id='overview-access-title' className='text-title font-semibold'>
          {t('Start using MyAPI')}
        </h2>
        <div className='flex min-w-0 flex-1 basis-72 flex-col gap-1 lg:max-w-xl'>
          <p className='text-caption text-muted-foreground'>{t('Base URL')}</p>
          <div className='bg-muted flex min-w-0 items-center gap-2 rounded-md py-1 ps-3'>
            <code className='text-data min-w-0 flex-1 font-mono wrap-anywhere'>
              {props.address.baseUrl}
            </code>
            <CopyButton
              value={props.address.baseUrl}
              tooltip={t('Copy Base URL')}
              aria-label={t('Copy Base URL')}
              className='pointer-coarse:min-h-11 pointer-coarse:min-w-11'
            />
          </div>
          <p className='text-caption text-muted-foreground'>
            {props.address.source === 'server'
              ? t('Configured server address')
              : t('Current access address')}
          </p>
        </div>
      </div>
      <ol className='grid min-w-0 gap-3 lg:grid-cols-3'>
        <li className='flex min-w-0 flex-col items-start gap-3 rounded-lg border p-3'>
          <h3 className='text-body font-semibold'>
            <span aria-hidden='true' className='text-muted-foreground me-2'>
              1
            </span>
            {t('Add or manage connections')}
          </h3>
          <p className='text-caption text-muted-foreground flex-1 wrap-anywhere'>
            {connectionDescription}
          </p>
          {props.canReadConnections ? (
            <Link
              to='/connections'
              data-myapi-button=''
              className={linkButtonClassName({
                variant: 'outline',
                className: 'h-auto min-h-11 whitespace-normal text-left',
              })}
            >
              {t('Open connections')}
            </Link>
          ) : (
            <p className='text-caption text-muted-foreground'>
              {t('This task is not available for this account.')}
            </p>
          )}
        </li>
        <li className='flex min-w-0 flex-col items-start gap-3 rounded-lg border p-3'>
          <h3 className='text-body font-semibold'>
            <span aria-hidden='true' className='text-muted-foreground me-2'>
              2
            </span>
            {t('Create or manage API Keys')}
          </h3>
          <p className='text-caption text-muted-foreground flex-1 wrap-anywhere'>
            {props.showKeys
              ? t('Open API Keys to create a Key or select an existing one.')
              : t('Ask an administrator how to obtain an API Key.')}
          </p>
          {props.showKeys ? (
            <Link
              to='/keys'
              data-myapi-button=''
              className={linkButtonClassName({
                className: 'h-auto min-h-11 whitespace-normal text-left',
              })}
            >
              {t('Open API Keys')}
            </Link>
          ) : (
            <p className='text-caption text-muted-foreground'>
              {t('This task is not available for this account.')}
            </p>
          )}
        </li>
        <li className='flex min-w-0 flex-col items-start gap-3 rounded-lg border p-3'>
          <h3 className='text-body font-semibold'>
            <span aria-hidden='true' className='text-muted-foreground me-2'>
              3
            </span>
            {t('Choose a Key for client setup')}
          </h3>
          <p className='text-caption text-muted-foreground flex-1 wrap-anywhere'>
            {props.showKeys
              ? t(
                  'Select a Key, then open its Delivery tab to copy the address and Key or import a client configuration.'
                )
              : t('Ask an administrator how to obtain an API Key.')}
          </p>
          {props.showKeys ? (
            <Link
              to='/keys'
              data-myapi-button=''
              className={linkButtonClassName({
                variant: 'outline',
                className: 'h-auto min-h-11 whitespace-normal text-left',
              })}
            >
              {t('Choose a Key')}
            </Link>
          ) : (
            <p className='text-caption text-muted-foreground'>
              {t('This task is not available for this account.')}
            </p>
          )}
        </li>
      </ol>
      {desktop && (
        <aside
          aria-label={t('Desktop window and tray')}
          className='bg-muted text-caption flex min-w-0 flex-col gap-1 rounded-md p-3'
        >
          <p>
            {t(
              'Manage the bundled service from the tray. Closing this window leaves it running; exiting from the tray stops it and quits MyAPI.'
            )}
          </p>
          <p className='text-muted-foreground'>
            {t(
              'Manage the access scope from the tray. This page does not change the local service.'
            )}
          </p>
        </aside>
      )}
    </section>
  )
}
