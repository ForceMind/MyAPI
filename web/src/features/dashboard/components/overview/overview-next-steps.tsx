/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { ArrowRight01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { hasPermission } from '@/lib/admin-permissions'
import { linkButtonClassName } from '@/lib/link-button'

import type { OverviewData } from './use-overview-data'

/**
 * Next steps for a new instance (design 8.1): add a connection, create a
 * Key, send the first request with the access address. Finished steps take
 * no space; when nothing is left the block disappears. Steps whose state is
 * unknown (a failed read) are not shown as done or as missing.
 */
export function OverviewNextSteps(props: {
  data: OverviewData
  showKeys: boolean
  showConnections: boolean
}) {
  const { t } = useTranslation()
  const user = props.data.user
  const connections = props.data.connections
  const needsConnection =
    props.showConnections &&
    props.data.canReadChannels &&
    hasPermission(user, 'channel', 'write') &&
    connections.isSuccess &&
    connections.data.total === 0
  const needsKey =
    props.showKeys && props.data.keys.isSuccess && props.data.keys.data === 0
  const needsRequest =
    Number.isSafeInteger(user?.request_count) && user?.request_count === 0
  if (!needsConnection && !needsKey && !needsRequest) return null
  const baseUrl = `${window.location.origin}/v1`

  return (
    <section
      aria-labelledby='overview-next-title'
      className='bg-card p-card flex min-w-0 flex-col gap-3 rounded-xl border'
    >
      <h2 id='overview-next-title' className='text-title'>
        {t('Next steps')}
      </h2>
      <ol className='flex flex-col gap-2'>
        {needsConnection && (
          <li>
            <Link
              to='/connections'
              data-myapi-button=''
              className={linkButtonClassName({
                variant: 'outline',
                className: 'min-h-11 w-full justify-between',
              })}
            >
              {t('Add an upstream connection')}
              <HugeiconsIcon
                icon={ArrowRight01Icon}
                data-icon='inline-end'
                aria-hidden='true'
              />
            </Link>
          </li>
        )}
        {needsKey && (
          <li>
            <Link
              to='/keys'
              data-myapi-button=''
              className={linkButtonClassName({
                variant: 'outline',
                className: 'min-h-11 w-full justify-between',
              })}
            >
              {t('Create an API Key for your client')}
              <HugeiconsIcon
                icon={ArrowRight01Icon}
                data-icon='inline-end'
                aria-hidden='true'
              />
            </Link>
          </li>
        )}
        {needsRequest && (
          <li className='flex min-w-0 flex-wrap items-center gap-2 rounded-lg border px-3 py-2'>
            <span className='text-body min-w-0 flex-1'>
              {t('Point your client at this address')}
            </span>
            <code className='text-data font-mono wrap-anywhere'>{baseUrl}</code>
            <CopyButton
              value={baseUrl}
              tooltip={t('Copy access address')}
              aria-label={t('Copy access address')}
            />
          </li>
        )}
      </ol>
    </section>
  )
}
