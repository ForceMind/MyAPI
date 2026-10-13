import { Link } from '@tanstack/react-router'
import { Refresh01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import type { ChannelQuotaChangesData } from '@/features/channels/types'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { ROLE } from '@/lib/roles'
import type { AuthUser } from '@/stores/auth-store'

import { AccountWindowList } from './account-window-list'
import { WorkbenchCard } from './workbench-card'

export function AccountWindowsSection(props: {
  freshUser: AuthUser | undefined
  canReadChannels: boolean
  data: ChannelQuotaChangesData | undefined
  identityLoading: boolean
  identityError: boolean
  isLoading: boolean
  isError: boolean
  isFetching: boolean
  onRetryIdentity: () => void
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  const showChannels = useIsSidebarModuleVisible('/channels')
  const items = props.data?.items.slice(0, 6) ?? []

  let content = null
  if (props.identityLoading) {
    content = (
      <div className='space-y-2'>
        <p className='text-muted-foreground text-xs'>
          {t('Confirming channel permissions...')}
        </p>
        <Skeleton className='h-16 w-full' />
      </div>
    )
  } else if (props.identityError || !props.freshUser) {
    content = (
      <Alert variant='destructive' data-testid='accounts-permission-state'>
        <AlertTitle>{t('Unable to confirm channel permissions')}</AlertTitle>
        <AlertDescription>
          <Button
            className='mt-2'
            size='sm'
            variant='outline'
            onClick={props.onRetryIdentity}
          >
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else if (props.freshUser.role < ROLE.ADMIN) {
    content = (
      <p
        data-testid='accounts-permission-state'
        className='bg-muted/35 rounded-lg border p-3 text-sm'
      >
        {t('Upstream account windows are available to channel administrators.')}
      </p>
    )
  } else if (!props.canReadChannels) {
    content = (
      <p
        data-testid='accounts-permission-state'
        className='bg-muted/35 rounded-lg border p-3 text-sm'
      >
        {t(
          'Channel read permission is required to view upstream account windows.'
        )}
      </p>
    )
  } else if (props.isLoading) {
    content = (
      <div className='space-y-2'>
        <Skeleton className='h-20 w-full' />
        <Skeleton className='h-20 w-full' />
      </div>
    )
  } else if (props.isError) {
    content = (
      <Alert variant='destructive' data-testid='accounts-error-state'>
        <AlertTitle>{t('Unable to load upstream account windows')}</AlertTitle>
        <AlertDescription>
          <Button
            className='mt-2'
            size='sm'
            variant='outline'
            onClick={props.onRefresh}
          >
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else if (items.length === 0) {
    content = (
      <p
        data-testid='accounts-empty-state'
        className='text-muted-foreground rounded-lg border border-dashed px-3 py-6 text-center text-sm'
      >
        {t('No upstream account windows were reported')}
      </p>
    )
  } else {
    content = <AccountWindowList items={items} />
  }

  return (
    <WorkbenchCard
      id='personal-account-windows-title'
      testId='account-windows-section'
      title={t('Upstream account windows')}
      description={t(
        'Read-only provider account snapshots. They are not Key or user balances.'
      )}
    >
      {content}
      {props.canReadChannels ? (
        <div className='mt-3 flex flex-wrap gap-2 border-t pt-3'>
          <Button
            size='sm'
            variant='outline'
            disabled={props.isFetching}
            onClick={props.onRefresh}
          >
            <HugeiconsIcon
              icon={Refresh01Icon}
              strokeWidth={2}
              aria-hidden='true'
            />
            {t('Refresh account windows')}
          </Button>
          {showChannels ? (
            <Button size='sm' variant='ghost' render={<Link to='/channels' />}>
              {t('Open Channels')}
            </Button>
          ) : null}
        </div>
      ) : null}
    </WorkbenchCard>
  )
}
