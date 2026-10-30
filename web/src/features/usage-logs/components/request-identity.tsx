/*
Copyright (C) 2026 ForceMind
SPDX-License-Identifier: AGPL-3.0-or-later
*/
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import type { UsageLog } from '../data/schema'
import { RequestTime } from './request-summary'

/** Identity comes from the selected saved record, never from another Key's breadcrumb. */
export function RequestIdentity(props: { log: UsageLog; isAdmin: boolean }) {
  const { t } = useTranslation()
  const { log } = props
  const keyName = log.token_name || t('Unknown Key')
  const connectionName = log.channel_name || t('Unknown')
  return (
    <dl aria-label={t('Request identity')} className='bg-card grid min-w-0 gap-3 rounded-xl border p-4 sm:grid-cols-2 xl:grid-cols-4'>
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
            <Button size='sm' variant='ghost' className='min-h-11' render={<Link to='/keys/$keyId' params={{ keyId: String(log.token_id) }} state={{ inspectorBack: true }} />}>
              {t('View this Key')}
            </Button>
          )}
        </dd>
      </div>
      {props.isAdmin && (
        <div className='min-w-0 sm:col-span-2'>
          <dt className='text-caption text-muted-foreground'>{t('Connection')}</dt>
          <dd className='font-mono text-sm wrap-anywhere'>{connectionName}{log.channel > 0 ? ` (#${log.channel})` : ` (${t('ID')}: ${t('Not recorded')})`}</dd>
        </div>
      )}
    </dl>
  )
}
