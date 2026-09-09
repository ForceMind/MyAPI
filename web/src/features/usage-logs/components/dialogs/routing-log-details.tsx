/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useTranslation } from 'react-i18next'

import { getRoutingReasonLabel } from '@/features/channel-routing/labels'

import type { LogOtherData } from '../../types'

export function RoutingLogDetails(props: {
  isAdmin: boolean
  routing?: NonNullable<LogOtherData['admin_info']>['channel_routing']
}) {
  const { t } = useTranslation()
  if (!props.isAdmin || !props.routing) return null
  const routing = props.routing

  return (
    <section
      aria-label={t('Traffic Allocation')}
      className='space-y-2 rounded-lg border p-3 text-xs'
    >
      <h3 className='font-medium'>{t('Traffic Allocation')}</h3>
      <dl className='grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 break-words'>
        <dt>{t('Channel ID')}</dt>
        <dd>{routing.channel_id ?? '—'}</dd>
        <dt>{t('Group')}</dt>
        <dd>{routing.group || '—'}</dd>
        <dt>{t('Routing reason')}</dt>
        <dd>
          {routing.reason ? getRoutingReasonLabel(t, routing.reason) : '—'}
        </dd>
        <dt>{t('Keep conversation on the same channel')}</dt>
        <dd>{routing.sticky ? t('Yes') : t('No')}</dd>
        {Boolean(routing.switch_count) && (
          <>
            <dt>{t('Channel switches')}</dt>
            <dd>{routing.switch_count}</dd>
            <dt>{t('Switch reason')}</dt>
            <dd className='font-mono'>{routing.switch_reason || '—'}</dd>
          </>
        )}
      </dl>
    </section>
  )
}
