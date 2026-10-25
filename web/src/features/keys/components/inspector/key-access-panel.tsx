/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import {
  DashboardSpeed01Icon,
  Edit02Icon,
  FilterHorizontalIcon,
  UserSettings01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { formatQuota, formatTimestampToDate } from '@/lib/format'

import { checkedAddSafeIntegers } from '../../lib/quota-display'
import type { ApiKey } from '../../types'
import { IpRestrictionsCell, ModelLimitsCell } from '../api-keys-cells'
import { useApiKeys } from '../api-keys-provider'

/**
 * What a Key may reach and how long it lives, with the editors that change
 * them. Every editor is the existing dialog; nothing here writes directly.
 */
export function KeyAccessPanel(props: { apiKey: ApiKey; showQuota: boolean }) {
  const { t } = useTranslation()
  const { setCurrentRow, setOpen } = useApiKeys()
  const key = props.apiKey
  const open = (
    dialog: 'model-scope' | 'budget' | 'access-policy' | 'update'
  ) => {
    setCurrentRow(key)
    setOpen(dialog)
  }
  const total = checkedAddSafeIntegers(key.used_quota, key.remain_quota)

  return (
    <div className='flex min-w-0 flex-col gap-4'>
      <dl className='grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2'>
        <div className='min-w-0'>
          <dt className='text-caption text-muted-foreground'>
            {t('Access profile')}
          </dt>
          <dd className='text-data wrap-anywhere'>
            {key.access_profile?.label || key.group || t('Default')}
          </dd>
        </div>
        <div className='min-w-0'>
          <dt className='text-caption text-muted-foreground'>{t('Models')}</dt>
          <dd className='text-data min-w-0'>
            <ModelLimitsCell apiKey={key} />
          </dd>
        </div>
        <div className='min-w-0'>
          <dt className='text-caption text-muted-foreground'>
            {t('IP Restriction')}
          </dt>
          <dd className='text-data min-w-0'>
            <IpRestrictionsCell apiKey={key} />
          </dd>
        </div>
        <div className='min-w-0'>
          <dt className='text-caption text-muted-foreground'>{t('Expires')}</dt>
          <dd className='text-data tabular-nums'>
            {key.expired_time === -1
              ? t('Never')
              : formatTimestampToDate(key.expired_time)}
          </dd>
        </div>
        {props.showQuota && (
          <div className='min-w-0 sm:col-span-2'>
            <dt className='text-caption text-muted-foreground'>{t('Quota')}</dt>
            <dd className='text-data tabular-nums'>
              {key.unlimited_quota
                ? t('Unlimited')
                : t('{{remaining}} remaining of {{total}}', {
                    remaining: formatQuota(key.remain_quota),
                    total:
                      total === null ? t('Unavailable') : formatQuota(total),
                  })}
            </dd>
          </div>
        )}
        <div className='min-w-0'>
          <dt className='text-caption text-muted-foreground'>{t('Created')}</dt>
          <dd className='text-data tabular-nums'>
            {formatTimestampToDate(key.created_time)}
          </dd>
        </div>
      </dl>
      <div className='flex min-w-0 flex-wrap gap-2'>
        <Button variant='outline' onClick={() => open('model-scope')}>
          <HugeiconsIcon
            icon={FilterHorizontalIcon}
            data-icon='inline-start'
            aria-hidden='true'
          />
          {t('Model scope')}
        </Button>
        <Button variant='outline' onClick={() => open('budget')}>
          <HugeiconsIcon
            icon={DashboardSpeed01Icon}
            data-icon='inline-start'
            aria-hidden='true'
          />
          {t('Usage budgets')}
        </Button>
        <Button variant='outline' onClick={() => open('access-policy')}>
          <HugeiconsIcon
            icon={UserSettings01Icon}
            data-icon='inline-start'
            aria-hidden='true'
          />
          {t('Assigned access')}
        </Button>
        <Button variant='ghost' onClick={() => open('update')}>
          <HugeiconsIcon
            icon={Edit02Icon}
            data-icon='inline-start'
            aria-hidden='true'
          />
          {t('Edit Key')}
        </Button>
      </div>
    </div>
  )
}
