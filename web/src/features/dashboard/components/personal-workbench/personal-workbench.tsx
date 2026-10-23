import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  quotaSeriesKey,
  quotaWindowLabel,
} from '@/features/channels/lib/quota-history'
import { AccountsState } from '@/features/personal-upstream/components/accounts-state'
import { WindowCard } from '@/features/personal-upstream/components/window-card'
import { useUpstreamAccounts } from '@/features/personal-upstream/use-upstream-accounts'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { useAuthStore } from '@/stores/auth-store'

import { OverviewConnections } from './overview-connections'
import { OverviewKeys } from './overview-keys'
import { OverviewRecent } from './overview-recent'

function PersonalWorkbenchSession() {
  const { t } = useTranslation()
  const data = useUpstreamAccounts()
  const showChannels = useIsSidebarModuleVisible('/channels')
  const showModels = useIsSidebarModuleVisible('/models/metadata')
  const showUpstream = useIsSidebarModuleVisible('/upstream-accounts')
  const [selectedSeries, setSelectedSeries] = useState('')
  const items =
    data.canRead && !data.accountsQuery.isError
      ? (data.accountsQuery.data?.items.filter(
          (item) =>
            item.metric_type === 'codex_rate_limit' && item.unit === 'percent'
        ) ?? [])
      : []
  const selected =
    items.find((item) => quotaSeriesKey(item) === selectedSeries) ?? items[0]
  const failedAccounts = new Set(
    items
      .filter((item) => item.status === 'error')
      .map((item) => item.channel_id)
  ).size
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Your API workspace')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('Connections, Keys, and calls at a glance')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Actions>
        {failedAccounts > 0 && showUpstream ? (
          <Link
            to='/upstream-accounts'
            className='max-w-full rounded-lg focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2'
          >
            <StatusBadge
              className='h-auto min-h-5 py-1 text-left whitespace-normal'
              variant='warning'
              copyable={false}
            >
              {t('Accounts with failed samples: {{count}}', {
                count: failedAccounts,
              })}
            </StatusBadge>
          </Link>
        ) : null}
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div
          className='flex min-w-0 flex-col gap-5'
          data-testid='personal-workbench'
        >
          <div
            className='grid min-w-0 items-stretch gap-5 lg:grid-cols-2 xl:grid-cols-3'
            data-testid='personal-workbench-layout'
          >
            <OverviewConnections
              canRead={data.canRead && showChannels}
              showModels={showModels}
            />
            <OverviewKeys />
            <section
              className='pc-card pc-overview-windows flex min-w-0 flex-col gap-4 p-4 lg:col-span-2 xl:col-span-1'
              aria-label={t('Upstream account windows')}
              data-testid='account-windows-section'
            >
              <h2 className='text-base font-semibold'>
                {t('Upstream account windows')}
              </h2>
              {items.length > 1 ? (
                <div className='flex min-w-0 flex-col gap-2'>
                  <label
                    htmlFor='overview-window'
                    className='text-sm font-medium'
                  >
                    {t('Select account window')}
                  </label>
                  <select
                    id='overview-window'
                    className='bg-background h-10 w-full min-w-0 rounded-md border px-2 text-sm'
                    value={selected ? quotaSeriesKey(selected) : ''}
                    onChange={(event) => setSelectedSeries(event.target.value)}
                  >
                    {items.map((item) => (
                      <option
                        key={quotaSeriesKey(item)}
                        value={quotaSeriesKey(item)}
                      >
                        {item.account_label || item.name} ·{' '}
                        {quotaWindowLabel(item.window_type, t)}
                      </option>
                    ))}
                  </select>
                </div>
              ) : null}
              {selected ? (
                <WindowCard
                  item={selected}
                  compact
                  showHistory={showChannels}
                />
              ) : (
                <AccountsState data={data} />
              )}
              {showUpstream ? (
                <Button
                  variant='ghost'
                  className='self-start'
                  nativeButton={false}
                  render={<Link to='/upstream-accounts' />}
                >
                  {t('View upstream accounts')}
                </Button>
              ) : null}
            </section>
          </div>
          <OverviewRecent />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

export function PersonalWorkbench() {
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  if (!user) return null
  return (
    <PersonalWorkbenchSession key={`${user.id}:${sessionId ?? 'no-session'}`} />
  )
}
