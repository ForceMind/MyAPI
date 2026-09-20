/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { RefreshCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useIsAdmin } from '@/hooks/use-admin'
import { formatTimestampToDate } from '@/lib/format'

import { SettingsSection } from '../../components/settings-section'
import { QUOTA_WRITER_MODE_VARIANTS } from '../constants'
import { useQuotaWriterStatus } from '../hooks/use-quota-writer'
import { ApplyTransitionCard } from './apply-transition-card'
import { AuditChecklist } from './audit-checklist'
import { DrainCard } from './drain-card'
import { TransitionsTable } from './transitions-table'

export function QuotaWriterSection() {
  const { t } = useTranslation()
  const isAdmin = useIsAdmin()
  const statusQuery = useQuotaWriterStatus()

  if (!isAdmin) {
    return (
      <SettingsSection title={t('Quota Writer Mode')}>
        <Alert variant='destructive'>
          <AlertTitle>{t('Admin access required')}</AlertTitle>
          <AlertDescription>
            {t('You do not have permission to manage the quota writer mode.')}
          </AlertDescription>
        </Alert>
      </SettingsSection>
    )
  }

  if (statusQuery.isLoading) {
    return (
      <SettingsSection title={t('Quota Writer Mode')}>
        <LoadingState message={t('Loading quota writer status...')} />
      </SettingsSection>
    )
  }

  if (statusQuery.isError || !statusQuery.data) {
    return (
      <SettingsSection title={t('Quota Writer Mode')}>
        <ErrorState
          description={t(
            'Failed to load quota writer status. It may require admin permission or the backend may be unavailable.'
          )}
          onRetry={() => statusQuery.refetch()}
        />
      </SettingsSection>
    )
  }

  const status = statusQuery.data
  const state = status.state
  const audit = status.audit

  return (
    <SettingsSection title={t('Quota Writer Mode')}>
      <div className='flex flex-col gap-4'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Controls how quota writes are persisted. Modes only move forward: legacy → bridge → authoritative; authoritative mode is not downgradable.'
          )}
        </p>

        {state.mode === 'bridge' && (
          <Alert className='border-warning/50 text-warning *:data-[slot=alert-description]:text-warning/90'>
            <AlertTitle>{t('Bridge mode is active')}</AlertTitle>
            <AlertDescription>
              {t(
                'Bridge is a silent drain mode: new relay sessions are rejected while in-flight sessions can still settle. Subscription balance purchases and admin quota adjustments are unavailable in bridge mode.'
              )}
            </AlertDescription>
          </Alert>
        )}

        <div className='flex flex-col gap-3 rounded-lg border p-4'>
          <div className='flex items-center justify-between gap-2'>
            <h4 className='text-sm font-semibold'>{t('Current status')}</h4>
            <Button
              variant='outline'
              size='sm'
              onClick={() => statusQuery.refetch()}
              disabled={statusQuery.isFetching}
            >
              <RefreshCcw
                className={statusQuery.isFetching ? 'animate-spin' : undefined}
                aria-hidden='true'
              />
              {t('Refresh')}
            </Button>
          </div>
          <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-5'>
            <div className='bg-muted/40 rounded-md p-3'>
              <div className='text-muted-foreground text-xs'>
                {t('Writer mode')}
              </div>
              <div className='mt-1'>
                <StatusBadge
                  label={state.mode}
                  variant={QUOTA_WRITER_MODE_VARIANTS[state.mode]}
                  copyable={false}
                />
              </div>
            </div>
            <div className='bg-muted/40 rounded-md p-3'>
              <div className='text-muted-foreground text-xs'>{t('Epoch')}</div>
              <div className='text-lg font-semibold'>{state.epoch}</div>
            </div>
            <div className='bg-muted/40 rounded-md p-3'>
              <div className='text-muted-foreground text-xs'>
                {t('Lock version')}
              </div>
              <div className='text-lg font-semibold'>{state.lock_version}</div>
            </div>
            <div className='bg-muted/40 rounded-md p-3'>
              <div className='text-muted-foreground text-xs'>
                {t('In-flight sessions')}
              </div>
              <div className='text-lg font-semibold'>
                {status.inflight_sessions}
              </div>
            </div>
            <div className='bg-muted/40 rounded-md p-3'>
              <div className='text-muted-foreground text-xs'>
                {t('Last updated')}
              </div>
              <div className='text-sm font-medium'>
                {formatTimestampToDate(state.updated_at)}
              </div>
            </div>
          </div>
        </div>

        <div className='flex flex-col gap-3 rounded-lg border p-4'>
          <div className='flex flex-col gap-1'>
            <h4 className='text-sm font-semibold'>{t('Readiness audit')}</h4>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Every hard condition must pass before authoritative mode can be enabled.'
              )}
            </p>
          </div>
          <AuditChecklist audit={audit} />
        </div>

        <ApplyTransitionCard state={state} />
        <DrainCard />
        <TransitionsTable />
      </div>
    </SettingsSection>
  )
}
