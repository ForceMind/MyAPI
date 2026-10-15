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
import { ROLE } from '@/lib/roles'

import { AccountsState } from './components/accounts-state'
import { WindowCard } from './components/window-card'
import { WindowShareObservation } from './components/window-share-observation'
import { useUpstreamAccounts } from './use-upstream-accounts'

export function PersonalUpstream() {
  const { t } = useTranslation()
  const data = useUpstreamAccounts()
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
          {selectedAccount?.account_label ||
            selectedAccount?.name ||
            t('Upstream accounts')}
        </h1>
        <div className='flex flex-wrap gap-2'>
          <Button
            variant='outline'
            disabled={data.accountsQuery.isFetching}
            onClick={() => void data.accountsQuery.refetch()}
          >
            {t('Refresh account windows')}
          </Button>
          <Button
            variant='outline'
            nativeButton={false}
            render={<Link to='/channels' />}
          >
            {t('Manage account connection')}
          </Button>
        </div>
      </header>
      {accounts.length > 1 ? (
        <div
          className='flex min-w-0 flex-wrap gap-2'
          role='group'
          aria-label={t('Select upstream account')}
        >
          {accounts.map((account) => (
            <Button
              key={account.channel_id}
              variant={
                selectedAccount?.channel_id === account.channel_id
                  ? 'default'
                  : 'outline'
              }
              aria-pressed={selectedAccount?.channel_id === account.channel_id}
              onClick={() => setChannelId(account.channel_id)}
              className='h-auto max-w-full break-words whitespace-normal'
            >
              {account.account_label || account.name}
            </Button>
          ))}
        </div>
      ) : null}
      {windows.length > 0 ? (
        <>
          <div className='grid min-w-0 gap-5 md:grid-cols-2'>
            {windows.map((item) => (
              <WindowCard key={quotaSeriesKey(item)} item={item} />
            ))}
          </div>
          <WindowShareObservation
            key={selectedAccount.channel_id}
            channelId={selectedAccount.channel_id}
            canReadPoolUsage={
              data.canRead && data.selfQuery.data?.role === ROLE.SUPER_ADMIN
            }
          />
        </>
      ) : (
        <AccountsState data={data} />
      )}
    </main>
  )
}
