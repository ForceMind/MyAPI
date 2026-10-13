import { TerminalIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { TokenBudgetDialog } from '@/features/keys/components/token-budget-dialog'
import { UserUsagePolicyDialog } from '@/features/users/components/user-usage-policy-dialog'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { ROLE } from '@/lib/roles'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import { AccountWindowsSection } from './account-windows-section'
import { KeyBudgetSection } from './key-budget-section'
import { RecentRecordsSection } from './recent-records-section'
import { RequestPathSection } from './request-path-section'
import { usePersonalWorkbenchData } from './use-personal-workbench-data'
import { WorkbenchActions } from './workbench-actions'
import { getPlaygroundKeyCondition } from './workbench-state'

const INITIAL_KEY_COUNT = 10

function PersonalWorkbenchSession(props: {
  user: AuthUser
  sessionId: string | null
}) {
  const { t } = useTranslation()
  const showPlayground = useIsSidebarModuleVisible('/playground')
  const [selectedKeyId, setSelectedKeyId] = useState<number | null>(null)
  const [visibleKeyCount, setVisibleKeyCount] = useState(INITIAL_KEY_COUNT)
  const [budgetDialogOpen, setBudgetDialogOpen] = useState(false)
  const [policyDialogOpen, setPolicyDialogOpen] = useState(false)
  const data = usePersonalWorkbenchData({
    user: props.user,
    sessionId: props.sessionId,
    selectedKeyId,
  })
  const budgetError =
    data.budgetQuery.isError || data.budgetQuery.isRefetchError
  const accountsError =
    data.accountsQuery.isError || data.accountsQuery.isRefetchError
  const recentError =
    data.recentQuery.isError || data.recentQuery.isRefetchError
  const selectedKeyAvailable = Boolean(
    data.selectedKey &&
      getPlaygroundKeyCondition(data.selectedKey) === 'available'
  )
  const budget = budgetError ? undefined : data.budgetQuery.data
  const accounts = accountsError ? undefined : data.accountsQuery.data
  const recent = recentError ? undefined : data.recentQuery.data

  return (
    <div
      data-testid='personal-workbench'
      className='flex min-w-0 flex-col gap-4 pb-6'
    >
      <header className='from-primary/10 via-primary/5 relative overflow-hidden rounded-2xl border bg-gradient-to-br to-transparent px-4 py-5 shadow-sm sm:px-6 sm:py-6'>
        <div className='bg-primary/10 absolute -top-20 -right-16 size-48 rounded-full blur-3xl' />
        <div className='relative flex min-w-0 flex-col gap-5 sm:flex-row sm:items-end sm:justify-between'>
          <div className='min-w-0 max-w-2xl'>
            <p className='text-primary text-xs font-semibold tracking-[0.18em] uppercase'>
              {t('Personal request workspace')}
            </p>
            <h2 className='mt-2 text-2xl font-semibold tracking-tight sm:text-3xl'>
              {t('Choose a Key. Check its limits. Test when ready.')}
            </h2>
            <p className='text-muted-foreground mt-2 max-w-xl text-sm leading-relaxed'>
              {t(
                'Your Key, request limits, upstream windows, and recent records are gathered in one place.'
              )}
            </p>
          </div>
          {showPlayground && selectedKeyAvailable ? (
            <Button size='lg' render={<Link to='/playground' />}>
              <HugeiconsIcon
                icon={TerminalIcon}
                strokeWidth={2}
                aria-hidden='true'
              />
              {t('Open Playground')}
            </Button>
          ) : null}
          {showPlayground && !selectedKeyAvailable ? (
            <Button size='lg' disabled>
              <HugeiconsIcon
                icon={TerminalIcon}
                strokeWidth={2}
                aria-hidden='true'
              />
              {t('Open Playground')}
            </Button>
          ) : null}
        </div>
      </header>

      <div
        data-testid='personal-workbench-layout'
        className='grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1.6fr)_minmax(19rem,0.75fr)]'
      >
        <div className='flex min-w-0 flex-col gap-4'>
          <RequestPathSection
            keys={data.confirmedKeys}
            selectedKey={data.selectedKey}
            selectedKeyId={selectedKeyId}
            visibleCount={visibleKeyCount}
            isLoading={data.keysQuery.isLoading}
            isError={data.keysError}
            isFetching={data.keysQuery.isFetching}
            onSelectKey={(keyId) => {
              setSelectedKeyId(keyId)
              setBudgetDialogOpen(false)
            }}
            onLoadMore={() =>
              setVisibleKeyCount((count) => count + INITIAL_KEY_COUNT)
            }
            onRefresh={() => void data.keysQuery.refetch()}
          />
          <KeyBudgetSection
            selectedKey={data.selectedKey}
            budget={budget}
            isLoading={data.budgetQuery.isLoading}
            isError={budgetError}
            isFetching={data.budgetQuery.isFetching}
            onRefresh={() => void data.budgetQuery.refetch()}
            onOpenSettings={() => setBudgetDialogOpen(true)}
          />
          <RecentRecordsSection
            data={recent}
            isLoading={data.recentQuery.isLoading}
            isError={recentError}
            isFetching={data.recentQuery.isFetching}
            onRefresh={() => void data.recentQuery.refetch()}
          />
        </div>

        <aside className='flex min-w-0 flex-col gap-4'>
          <AccountWindowsSection
            freshUser={data.freshUser}
            canReadChannels={data.canReadChannels}
            data={accounts}
            identityLoading={data.selfQuery.isLoading}
            identityError={data.identityError}
            isLoading={data.accountsQuery.isLoading}
            isError={accountsError}
            isFetching={data.accountsQuery.isFetching}
            onRetryIdentity={() => void data.selfQuery.refetch()}
            onRefresh={() => void data.accountsQuery.refetch()}
          />
          <WorkbenchActions
            freshUser={data.freshUser}
            identityConfirmed={data.identityConfirmed}
            identityLoading={data.selfQuery.isLoading}
            identityError={data.identityError}
            canReadChannels={data.canReadChannels}
            fundingStatus={data.status.status}
            fundingConfirmed={data.status.confirmed}
            selectedKeyName={data.selectedKey?.name}
            budgetLoaded={Boolean(budget)}
            budgetError={budgetError}
            hasPendingBudget={Boolean(budget?.pending)}
            onRetryIdentity={() => void data.selfQuery.refetch()}
            onOpenPolicy={() => setPolicyDialogOpen(true)}
          />
        </aside>
      </div>

      {budgetDialogOpen && data.selectedKey ? (
        <TokenBudgetDialog
          tokenId={data.selectedKey.id}
          onClose={() => setBudgetDialogOpen(false)}
        />
      ) : null}
      {policyDialogOpen && data.freshUser?.role === ROLE.SUPER_ADMIN ? (
        <UserUsagePolicyDialog
          userId={data.freshUser.id}
          onClose={() => setPolicyDialogOpen(false)}
          onSaved={() => void data.selfQuery.refetch()}
        />
      ) : null}
    </div>
  )
}

export function PersonalWorkbench() {
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  if (!user) return null
  return (
    <PersonalWorkbenchSession
      key={`${user.id}:${sessionId ?? 'no-session'}`}
      user={user}
      sessionId={sessionId}
    />
  )
}
