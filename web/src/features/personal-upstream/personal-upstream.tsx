/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { quotaSeriesKey } from '@/features/channels/lib/quota-history'
import { AccountQuotaChangesPanel } from '@/features/dashboard/components/overview/account-quota-changes-panel'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { ROLE } from '@/lib/roles'

import { AccountsState } from './components/accounts-state'
import { WindowCard } from './components/window-card'
import { WindowShareObservation } from './components/window-share-observation'
import { useUpstreamAccounts } from './use-upstream-accounts'

export function PersonalUpstream() {
  const { t } = useTranslation()
  const data = useUpstreamAccounts()
  const showChannels = useIsSidebarModuleVisible('/channels')
  const [channelId, setChannelId] = useState<number | null>(null)
  const [showChanges, setShowChanges] = useState(false)
  const items =
    data.canRead && !data.accountsQuery.isError
      ? (data.accountsQuery.data?.items.filter(
          (item) =>
            item.metric_type === 'codex_rate_limit' && item.unit === 'percent'
        ) ?? [])
      : []
  const accounts = [
    ...new Map(items.map((item) => [item.channel_id, item])).values(),
  ]
  const selectedAccount =
    accounts.find((item) => item.channel_id === channelId) ?? accounts[0]
  const windows = items.filter(
    (item) => item.channel_id === selectedAccount?.channel_id
  )
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Account windows')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('Saved subscription observations')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Actions>
        {data.canRead ? (
          <Button
            variant='outline'
            disabled={data.accountsQuery.isFetching}
            onClick={() => void data.accountsQuery.refetch()}
          >
            {t('Refresh account windows')}
          </Button>
        ) : null}
        {data.canRead && showChannels ? (
          <Button
            variant='outline'
            nativeButton={false}
            render={
              selectedAccount ? (
                <Link
                  to='/connections/$connectionId'
                  params={{ connectionId: String(selectedAccount.channel_id) }}
                />
              ) : (
                <Link to='/connections' />
              )
            }
          >
            {t('Manage account connection')}
          </Button>
        ) : null}
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div
          className='flex min-w-0 flex-col gap-5'
          data-testid='personal-upstream'
        >
          {accounts.length > 0 ? (
            <div className='grid min-w-0 gap-5 lg:grid-cols-[minmax(0,14rem)_minmax(0,1fr)]'>
              {/* One account list for every width; on a phone it scrolls
                  sideways instead of a native select (design 6.2). */}
              <aside
                className='bg-card min-w-0 self-start rounded-xl border p-3'
                aria-label={t('Select upstream account')}
              >
                <p className='text-caption text-muted-foreground px-2 py-2'>
                  {t('Observed subscription accounts')}
                </p>
                <div className='flex min-w-0 gap-1 overflow-x-auto lg:flex-col lg:overflow-visible'>
                  {accounts.map((account) => (
                    <Button
                      key={account.channel_id}
                      variant={
                        selectedAccount?.channel_id === account.channel_id
                          ? 'secondary'
                          : 'ghost'
                      }
                      aria-pressed={
                        selectedAccount?.channel_id === account.channel_id
                      }
                      onClick={() => setChannelId(account.channel_id)}
                      className='h-auto min-h-10 max-w-64 shrink-0 justify-start py-2 text-left wrap-anywhere whitespace-normal lg:w-full lg:max-w-none lg:min-w-0'
                    >
                      {account.account_label || account.name}
                    </Button>
                  ))}
                </div>
              </aside>
              <div className='flex min-w-0 flex-col gap-4'>
                <h2 className='min-w-0 text-lg font-semibold wrap-anywhere'>
                  {selectedAccount?.account_label || selectedAccount?.name}
                </h2>
                {data.accountsQuery.data?.items_complete === false ||
                data.accountsQuery.data?.source_complete === false ? (
                  <p role='status' className='text-muted-foreground text-sm'>
                    {t('Only returned window observations are shown.')}
                  </p>
                ) : null}
                <div className='grid min-w-0 gap-4 xl:grid-cols-2'>
                  {windows.map((item) => (
                    <WindowCard
                      key={quotaSeriesKey(item)}
                      item={item}
                      showHistory={showChannels}
                    />
                  ))}
                </div>
                <WindowShareObservation
                  key={selectedAccount.channel_id}
                  channelId={selectedAccount.channel_id}
                  canReadPoolUsage={
                    data.canRead &&
                    data.selfQuery.data?.role === ROLE.SUPER_ADMIN
                  }
                />
              </div>
            </div>
          ) : (
            <AccountsState data={data} />
          )}
          {data.canRead && (
            <section
              aria-labelledby='account-window-changes'
              className='flex min-w-0 flex-col gap-3'
            >
              <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'>
                <h2 id='account-window-changes' className='text-section'>
                  {t('Window changes over time')}
                </h2>
                <Button
                  variant='outline'
                  aria-expanded={showChanges}
                  onClick={() => setShowChanges((open) => !open)}
                >
                  {showChanges
                    ? t('Hide window changes')
                    : t('Show window changes')}
                </Button>
              </div>
              {/* Read only on request: the comparison is a second, larger
                  read of saved observations. */}
              {showChanges && <AccountQuotaChangesPanel />}
            </section>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
