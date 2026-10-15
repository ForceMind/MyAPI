import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { AccountsState } from '@/features/personal-upstream/components/accounts-state'
import { WindowCard } from '@/features/personal-upstream/components/window-card'
import { useUpstreamAccounts } from '@/features/personal-upstream/use-upstream-accounts'
import { quotaSeriesKey, quotaWindowLabel } from '@/features/channels/lib/quota-history'
import { useAuthStore } from '@/stores/auth-store'

import { OverviewKeys } from './overview-keys'
import { OverviewRecent } from './overview-recent'

function PersonalWorkbenchSession() {
  const { t } = useTranslation()
  const data = useUpstreamAccounts()
  const [selectedSeries, setSelectedSeries] = useState('')
  const items = data.canRead && !data.accountsQuery.isError ? data.accountsQuery.data?.items.filter((item) => item.metric_type === 'codex_rate_limit' && item.unit === 'percent') ?? [] : []
  const selected = items.find((item) => quotaSeriesKey(item) === selectedSeries) ?? items[0]
  const accountErrors = items.filter((item) => item.status === 'error')
  return <div className='pc-page flex min-w-0 flex-col gap-5' data-testid='personal-workbench'>
    {accountErrors.length > 0 ? <Alert><AlertTitle>{t('An upstream account needs attention')}</AlertTitle><Button size='sm' variant='ghost' nativeButton={false} render={<Link to='/upstream-accounts' />}>{t('View upstream accounts')}</Button></Alert> : null}
    <header><h1 className='text-2xl leading-8 font-semibold tracking-tight sm:text-[32px] sm:leading-10'>{t('Your API workspace')}</h1><p className='text-muted-foreground mt-2 text-base'>{t('Connections, Keys, and calls at a glance')}</p></header>
    <div className='grid min-w-0 gap-5 md:grid-cols-[minmax(0,1.65fr)_minmax(0,1fr)]' data-testid='personal-workbench-layout'>
      <section className='flex min-w-0 flex-col gap-3' aria-label={t('Upstream account windows')} data-testid='account-windows-section'>
        {items.length > 1 ? <div className='flex min-w-0 flex-wrap gap-2' role='group' aria-label={t('Select account window')}>{items.map((item) => <Button key={quotaSeriesKey(item)} size='sm' variant={selected === item ? 'default' : 'outline'} aria-pressed={selected === item} onClick={() => setSelectedSeries(quotaSeriesKey(item))} className='h-auto min-w-0 whitespace-normal break-words'>{item.account_label || item.name} · {quotaWindowLabel(item.window_type, t)}</Button>)}</div> : null}
        {selected ? <WindowCard item={selected} compact /> : <AccountsState data={data} />}
        <Button variant='ghost' className='self-start' nativeButton={false} render={<Link to='/upstream-accounts' />}>{t('View upstream accounts')}</Button>
      </section>
      <OverviewKeys />
    </div>
    <OverviewRecent />
  </div>
}

export function PersonalWorkbench() {
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  if (!user) return null
  return <PersonalWorkbenchSession key={`${user.id}:${sessionId ?? 'no-session'}`} />
}
