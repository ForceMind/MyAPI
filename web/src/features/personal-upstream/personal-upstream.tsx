/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Link } from '@tanstack/react-router'
import { useEffect, useState, type ReactNode, type RefObject } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { StatTile } from '@/components/stat-tile'
import { Button } from '@/components/ui/button'
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import { formatQuotaDuration, quotaSeriesKey, quotaWindowLabel } from '@/features/channels/lib/quota-history'
import { AccountQuotaChangesPanel } from '@/features/dashboard/components/overview/account-quota-changes-panel'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { hasPermission } from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { AccountsState } from './components/accounts-state'
import { WindowCard } from './components/window-card'
import { WindowShareObservation } from './components/window-share-observation'
import { filterUpstreamWindows, upstreamWindowPeriod, type UpstreamViewState } from './upstream-view-state'
import { useUpstreamAccounts } from './use-upstream-accounts'
import { nativeWindowState } from './window-state'

type PersonalUpstreamProps = {
  view?: UpstreamViewState
  onViewChange?: (view: UpstreamViewState) => void
  onOpenConnection?: (channelId: number) => void
  onInventoryReady?: (ready: boolean) => void
  inventoryHeadingRef?: RefObject<HTMLHeadingElement | null>
}

export function PersonalUpstream(props: PersonalUpstreamProps) {
  const identity = useAuthStore((state) => [state.auth.user?.id, state.auth.user?.role, state.auth.session?.sid, hasPermission(state.auth.user, 'channel', 'read')].join(':'))
  return <PersonalUpstreamSession key={identity} {...props} />
}

function PersonalUpstreamSession(props: PersonalUpstreamProps) {
  const { t } = useTranslation()
  const data = useUpstreamAccounts()
  const showChannels = useIsSidebarModuleVisible('/channels')
  const [localView, setLocalView] = useState<UpstreamViewState>({})
  const view = props.view ?? localView
  const [searchText, setSearchText] = useState(view.filter ?? '')
  const [, refreshClock] = useState(0)
  const now = Date.now() / 1000
  useEffect(() => setSearchText(view.filter ?? ''), [view.filter])
  useEffect(() => {
    const timer = window.setInterval(() => refreshClock((version) => version + 1), 30_000)
    return () => window.clearInterval(timer)
  }, [])
  const ready = !data.selfQuery.isLoading && (!data.canRead || data.accountsQuery.isSuccess || data.accountsQuery.isError)
  const inventoryReady = props.onInventoryReady
  useEffect(() => inventoryReady?.(ready), [inventoryReady, ready])
  const updateView = (next: UpstreamViewState) => {
    if (props.onViewChange) props.onViewChange(next)
    else setLocalView(next)
  }
  const known = data.canRead && data.accountsQuery.isSuccess && !data.accountsQuery.isError
  const items = known ? (data.accountsQuery.data?.items.filter((item) => item.metric_type === 'codex_rate_limit' && item.unit === 'percent') ?? []) : []
  const filteredItems = filterUpstreamWindows(items, view, now)
  const accountMap = new Map<number, ChannelQuotaChangeItem>()
  for (const item of filteredItems) {
    if (!accountMap.has(item.channel_id)) accountMap.set(item.channel_id, item)
  }
  const accounts = [...accountMap.values()]
  const selectedAccount = accounts.find((item) => item.channel_id === view.channelId) ?? accounts[0]
  const windows = filteredItems.filter((item) => item.channel_id === selectedAccount?.channel_id)
  const returnedConnections = new Set(items.map((item) => item.channel_id)).size
  const current = items.filter((item) => nativeWindowState(item, now) === 'available').length
  const periods = [...new Map(items.map((item) => [upstreamWindowPeriod(item), item])).entries()]
  const upstreamReturn = selectedAccount ? { ...view, channelId: selectedAccount.channel_id } : view

  let inventory: ReactNode
  if (accounts.length > 0 && selectedAccount) {
    inventory = (
            <div className='grid min-w-0 gap-5 lg:grid-cols-[minmax(0,14rem)_minmax(0,1fr)]'>
              <aside className='bg-card min-w-0 self-start rounded-xl border p-3' aria-label={t('Select upstream account')}>
                <p className='text-caption text-muted-foreground px-2 py-2'>{t('Observed subscription accounts')}</p>
                <div className='flex min-w-0 gap-1 overflow-x-auto lg:flex-col lg:overflow-visible'>
                  {accounts.map((account) => <Button key={account.channel_id} data-upstream-account={account.channel_id} variant={selectedAccount.channel_id === account.channel_id ? 'secondary' : 'ghost'} aria-pressed={selectedAccount.channel_id === account.channel_id} onClick={() => { if (account.channel_id !== view.channelId) updateView({ ...view, channelId: account.channel_id, keyId: account.channel_id === selectedAccount.channel_id ? view.keyId : undefined }) }} className='h-auto min-h-11 max-w-64 shrink-0 justify-start py-2 text-left wrap-anywhere whitespace-normal lg:w-full lg:max-w-none lg:min-w-0'>{account.account_label || account.name}</Button>)}
                </div>
              </aside>
              <div className='flex min-w-0 flex-col gap-4'>
                <h2 className='text-section min-w-0 wrap-anywhere'>{selectedAccount.account_label || selectedAccount.name}</h2>
                <div className='grid min-w-0 gap-4 xl:grid-cols-2'>{windows.map((item) => <WindowCard key={quotaSeriesKey(item)} item={item} windows={items} now={now} showHistory={showChannels} />)}</div>
                <WindowShareObservation connectionContext={props.view ? { connection: { connection: selectedAccount.channel_id, upstreamReturn } } : undefined} key={selectedAccount.channel_id} channelId={selectedAccount.channel_id} selectedKeyId={view.keyId === undefined ? '' : String(view.keyId)} onKeyChange={(keyId) => updateView({ ...view, channelId: selectedAccount.channel_id, keyId: keyId ? Number(keyId) : undefined })} canReadPoolUsage={data.canRead && data.selfQuery.data?.role === ROLE.SUPER_ADMIN} />
              </div>
            </div>
    )
  } else if (known && items.length > 0) {
    inventory = (
            <Empty className='border'><EmptyHeader><EmptyTitle>{t('No window observations match these filters.')}</EmptyTitle><EmptyDescription>{t('Change the filters to inspect the saved observations returned by this read.')}</EmptyDescription></EmptyHeader></Empty>
    )
  } else {
    inventory = <AccountsState data={data} />
  }

  return (
    <SectionPageLayout fixedContent={false}>
      <SectionPageLayout.Title>{t('Account windows')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>{t('Saved subscription observations')}</SectionPageLayout.Description>
      <SectionPageLayout.Actions>
        {data.canRead && <Button variant='outline' disabled={data.accountsQuery.isFetching} onClick={() => { refreshClock((version) => version + 1); void data.accountsQuery.refetch() }}>{t('Refresh account windows')}</Button>}
        {data.canRead && showChannels && (
          <Button variant='outline' nativeButton={false} render={selectedAccount ? (
            <Link to='/connections/$connectionId' params={{ connectionId: String(selectedAccount.channel_id) }} search={props.view ? { connectionReturn: { upstreamReturn } } : undefined} state={props.view ? { inspectorBack: true } : undefined} onClick={() => props.onOpenConnection?.(selectedAccount.channel_id)} />
          ) : <Link to='/connections' />}>
            {t('Manage account connection')}
          </Button>
        )}
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex min-w-0 flex-col gap-5' data-testid='personal-upstream'>
          <h2 ref={props.inventoryHeadingRef} tabIndex={-1} className='sr-only'>{t('Account window inventory')}</h2>
          {data.canRead && (
            <>
              <dl className='grid min-w-0 gap-3 sm:grid-cols-3'>
                <StatTile label={t('Returned connections')} value={known ? returnedConnections : t('Unknown')} loading={data.accountsQuery.isLoading} />
                <StatTile label={t('Fresh window observations')} value={known ? current : t('Unknown')} loading={data.accountsQuery.isLoading} />
                <StatTile label={t('Window observations not confirmed current')} value={known ? items.length - current : t('Unknown')} loading={data.accountsQuery.isLoading} />
              </dl>
              <section aria-label={t('Filter saved window observations')} className='bg-card flex min-w-0 flex-col gap-3 rounded-xl border p-4'>
                <form role='search' className='grid min-w-0 gap-3 sm:grid-cols-2 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_auto]' onSubmit={(event) => { event.preventDefault(); updateView({ ...view, filter: searchText.trim() || undefined }) }}>
                  <label className='flex min-w-0 flex-col gap-2'><span className='text-caption text-muted-foreground'>{t('Account or connection')}</span><Input value={searchText} maxLength={200} onChange={(event) => setSearchText(event.target.value)} className='min-h-11' /></label>
                  <label className='flex min-w-0 flex-col gap-2'><span className='text-caption text-muted-foreground'>{t('Window period')}</span><NativeSelect className='[&_select]:min-h-11' value={view.period ?? ''} onChange={(event) => updateView({ ...view, period: event.target.value || undefined })}><NativeSelectOption value=''>{t('All windows')}</NativeSelectOption>{view.period && !periods.some(([value]) => value === view.period) && <NativeSelectOption value={view.period}>{t('Selected window is not in returned observations')}</NativeSelectOption>}{periods.map(([value, item]) => <NativeSelectOption key={value} value={value}>{value.startsWith('duration:') ? formatQuotaDuration(item.window_seconds as number, t) : quotaWindowLabel(item.window_type, t)}</NativeSelectOption>)}</NativeSelect></label>
                  <label className='flex min-w-0 flex-col gap-2'><span className='text-caption text-muted-foreground'>{t('Snapshot state')}</span><NativeSelect className='[&_select]:min-h-11' value={view.status ?? ''} onChange={(event) => updateView({ ...view, status: (event.target.value || undefined) as UpstreamViewState['status'] })}>
                    <NativeSelectOption value=''>{t('All Status')}</NativeSelectOption>
                    <NativeSelectOption value='available'>{t('Snapshot available')}</NativeSelectOption>
                    <NativeSelectOption value='stale'>{t('Stale sample')}</NativeSelectOption>
                    <NativeSelectOption value='unverified'>{t('Sample time unverified')}</NativeSelectOption>
                    <NativeSelectOption value='error'>{t('Saved errors and limit events')}</NativeSelectOption>
                    <NativeSelectOption value='unsupported'>{t('Window reporting unsupported')}</NativeSelectOption>
                    <NativeSelectOption value='unavailable'>{t('No reliable usage sample')}</NativeSelectOption>
                  </NativeSelect></label>
                  <Button type='submit' variant='outline' className='min-h-11 self-end'>{t('Search')}</Button>
                </form>
                <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'><p className='text-caption text-muted-foreground'>{t('Statistics and filters cover returned observations only.')}</p><Button variant='ghost' className='min-h-11' onClick={() => { setSearchText(''); updateView({ ...view, filter: undefined, period: undefined, status: undefined }) }}>{t('Reset filters')}</Button></div>
                {known && (data.accountsQuery.data?.items_complete !== true || data.accountsQuery.data?.source_complete !== true) && <p role='status' className='text-caption text-muted-foreground'>{t('Only returned window observations are shown.')}</p>}
              </section>
            </>
          )}
          {inventory}
          {data.canRead && <section aria-labelledby='account-window-changes' className='flex min-w-0 flex-col gap-3'>
            <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'><h2 id='account-window-changes' className='text-section'>{t('Window changes over time')}</h2><Button variant='outline' className='min-h-11' aria-expanded={view.changes === true} onClick={() => updateView({ ...view, changes: !view.changes || undefined })}>{view.changes ? t('Hide window changes') : t('Show window changes')}</Button></div>
            {view.changes && <AccountQuotaChangesPanel />}
          </section>}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
