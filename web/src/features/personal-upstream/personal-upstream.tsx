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

import { Button } from '@/components/ui/button'
import { quotaSeriesKey } from '@/features/channels/lib/quota-history'
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
    <main
      id='content'
      tabIndex={-1}
      className='pc-page flex min-w-0 flex-col gap-5'
      data-testid='personal-upstream'
    >
      <p className='text-muted-foreground text-sm'>
        {t('Upstream accounts')}
        {selectedAccount
          ? ` / ${selectedAccount.account_label || selectedAccount.name}`
          : ''}
      </p>
      <header className='flex min-w-0 flex-wrap items-center justify-between gap-3'>
        <h1 className='min-w-0 text-2xl leading-8 font-semibold tracking-tight break-words sm:text-[32px] sm:leading-10'>
          {t('Upstream accounts')}
        </h1>
        <div className='flex flex-wrap gap-2'>
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
              render={<Link to='/channels' />}
            >
              {t('Manage account connection')}
            </Button>
          ) : null}
        </div>
      </header>
      <p className='text-muted-foreground text-sm'>
        {t('Saved subscription observations')}
      </p>
      {accounts.length > 0 ? (
        <div className='grid min-w-0 gap-5 lg:grid-cols-[minmax(0,14rem)_minmax(0,1fr)]'>
          <aside
            className='bg-card hidden min-w-0 self-start rounded-xl border p-3 lg:block'
            aria-label={t('Select upstream account')}
          >
            <p className='text-muted-foreground px-2 py-2 text-xs font-medium'>
              {t('Observed subscription accounts')}
            </p>
            <div className='flex min-w-0 flex-col gap-1'>
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
                  className='h-auto min-h-10 w-full min-w-0 justify-start py-2 text-left break-all whitespace-normal'
                >
                  {account.account_label || account.name}
                </Button>
              ))}
            </div>
          </aside>
          <div className='flex min-w-0 flex-col gap-4'>
            <div className='flex min-w-0 flex-col gap-2 lg:hidden'>
              <label htmlFor='upstream-account' className='text-sm font-medium'>
                {t('Select upstream account')}
              </label>
              <select
                id='upstream-account'
                value={selectedAccount?.channel_id ?? ''}
                onChange={(event) => setChannelId(Number(event.target.value))}
                className='bg-background h-10 w-full min-w-0 rounded-md border px-2 text-sm'
              >
                {accounts.map((account) => (
                  <option key={account.channel_id} value={account.channel_id}>
                    {account.account_label || account.name}
                  </option>
                ))}
              </select>
            </div>
            <h2 className='min-w-0 text-lg font-semibold break-all'>
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
                data.canRead && data.selfQuery.data?.role === ROLE.SUPER_ADMIN
              }
            />
          </div>
        </div>
      ) : (
        <AccountsState data={data} />
      )}
    </main>
  )
}
