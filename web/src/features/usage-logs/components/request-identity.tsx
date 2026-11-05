/*
Copyright (C) 2026 ForceMind
SPDX-License-Identifier: AGPL-3.0-or-later
*/
import { Link } from '@tanstack/react-router'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { useRememberReturnFocus, useReturnFocus } from '@/components/object-page/return-focus'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { hasPermission } from '@/lib/admin-permissions'
import { linkButtonClassName } from '@/lib/link-button'
import { useAuthStore } from '@/stores/auth-store'

import type { UsageLog } from '../data/schema'
import { RequestTime } from './request-summary'

/** Identity comes from the selected saved record, never from another Key's breadcrumb. */
export function RequestIdentity(props: { log: UsageLog; isAdmin: boolean }) {
  const { t } = useTranslation()
  const { log } = props
  const keyName = log.token_name || t('Unknown Key')
  const connectionName = log.channel_name || t('Unknown')
  const user = useAuthStore((state) => state.auth.user)
  const showConnections = useIsSidebarModuleVisible('/connections')
  const canViewConnection = props.isAdmin && showConnections && hasPermission(user, 'channel', 'read')
  const rememberFocus = useRememberReturnFocus()
  const identity = useRef<HTMLDListElement>(null)
  useReturnFocus({ attribute: 'data-request-related-link', settled: true, rows: log, fallback: identity })
  return (
    <dl ref={identity} tabIndex={-1} aria-label={t('Request identity')} className='bg-card grid min-w-0 gap-3 rounded-xl border p-4 sm:grid-cols-2 xl:grid-cols-4'>
      <div className='min-w-0 sm:col-span-2'>
        <dt className='text-caption text-muted-foreground'>{t('Request ID')}</dt>
        <dd className='font-mono text-sm wrap-anywhere'>{log.request_id || t('Not recorded')}</dd>
      </div>
      <div className='min-w-0'>
        <dt className='text-caption text-muted-foreground'>{t('Time')}</dt>
        <dd className='text-sm'><RequestTime seconds={log.created_at} withDate /></dd>
      </div>
      <div className='min-w-0'>
        <dt className='text-caption text-muted-foreground'>{t('Model')}</dt>
        <dd className='font-mono text-sm wrap-anywhere'>{log.model_name || t('Unknown')}</dd>
      </div>
      <div className='min-w-0 sm:col-span-2'>
        <dt className='text-caption text-muted-foreground'>{t('API Key')}</dt>
        <dd className='flex min-w-0 flex-wrap items-center gap-2 text-sm'>
          <span className='font-mono wrap-anywhere'>{keyName}{log.token_id > 0 ? ` (#${log.token_id})` : ` (${t('ID')}: ${t('Not recorded')})`}</span>
          {log.token_id > 0 && (
            <Link to='/keys/$keyId' params={{ keyId: String(log.token_id) }} search={{}} state={{ inspectorBack: true }} data-request-related-link={`key:${log.token_id}`} className={linkButtonClassName({ variant: 'ghost', className: 'min-h-11' })} onClick={(event) => {
              if (event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey) rememberFocus('data-request-related-link', `key:${log.token_id}`)
            }}>
              {t('View this Key')}
            </Link>
          )}
        </dd>
      </div>
      {props.isAdmin && (
        <div className='min-w-0 sm:col-span-2'>
          <dt className='text-caption text-muted-foreground'>{t('Connection')}</dt>
          <dd className='flex min-w-0 flex-wrap items-center gap-2 text-sm'>
            <span className='font-mono wrap-anywhere'>{connectionName}{log.channel > 0 ? ` (#${log.channel})` : ` (${t('ID')}: ${t('Not recorded')})`}</span>
            {log.channel > 0 && canViewConnection && <Link to='/connections/$connectionId' params={{ connectionId: String(log.channel) }} search={{}} state={{ inspectorBack: true }} data-request-related-link={`connection:${log.channel}`} className={linkButtonClassName({ variant: 'ghost', className: 'min-h-11' })} onClick={(event) => {
              if (event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey) rememberFocus('data-request-related-link', `connection:${log.channel}`)
            }}>{t('View connection')}</Link>}
          </dd>
        </div>
      )}
    </dl>
  )
}
