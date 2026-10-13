import { Wrench01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import type { SystemStatus } from '@/features/auth/types'
import { resolveUsagePolicyPresentation } from '@/features/users/lib/user-usage-policy-presentation'
import { ROLE } from '@/lib/roles'
import type { AuthUser } from '@/stores/auth-store'

import { WorkbenchActionLinks } from './workbench-action-links'
import { getFreshUsagePolicy } from './workbench-state'
import { WorkbenchCard } from './workbench-card'

export function WorkbenchActions(props: {
  freshUser: AuthUser | undefined
  identityConfirmed: boolean
  identityLoading: boolean
  identityError: boolean
  canReadChannels: boolean
  fundingStatus: SystemStatus | null
  fundingConfirmed: boolean
  selectedKeyName: string | undefined
  budgetLoaded: boolean
  budgetError: boolean
  hasPendingBudget: boolean
  onRetryIdentity: () => void
  onOpenPolicy: () => void
}) {
  const { t } = useTranslation()
  const policy = getFreshUsagePolicy(props.freshUser)
  const presentation = resolveUsagePolicyPresentation({
    userId: props.freshUser?.id ?? 0,
    policy,
    policyConfirmed: props.identityConfirmed && Boolean(policy),
    status: props.fundingStatus,
    statusConfirmed: props.fundingConfirmed,
  })

  let policyLabel = t('Policy status unknown')
  let policyVariant: 'success' | 'warning' | 'neutral' = 'neutral'
  if (presentation.condition === 'supported') {
    policyLabel = t('No-wallet policy active')
    policyVariant = 'success'
  } else if (presentation.condition === 'wallet') {
    policyLabel = t('Stored user allowance remains in effect')
  } else if (presentation.condition === 'inactive') {
    policyLabel = t('Saved no-wallet preference is inactive')
    policyVariant = 'warning'
  }
  let reminderLabel = t('Choose a Key to check request reminders.')
  if (props.budgetError) {
    reminderLabel = t('Pending review status is unavailable.')
  } else if (props.selectedKeyName && props.budgetLoaded) {
    reminderLabel = t('No pending request review for the selected Key.')
  } else if (props.selectedKeyName) {
    reminderLabel = t('Loading...')
  }
  let identityContent: ReactNode
  if (props.identityLoading) {
    identityContent = (
      <p className='text-muted-foreground text-xs'>
        {t('Confirming your usage policy...')}
      </p>
    )
  } else if (props.identityError) {
    identityContent = (
      <Alert variant='destructive'>
        <AlertTitle>{t('Unable to confirm your usage policy')}</AlertTitle>
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
  } else {
    const policyDescription =
      presentation.condition === 'supported'
        ? t(
            'The server confirmed no-wallet conditions for supported requests; admission still checks the selected Key and route.'
          )
        : t('Request admission still checks the selected Key and route.')
    identityContent = (
      <p className='text-muted-foreground text-xs leading-relaxed'>
        {policyDescription}
      </p>
    )
  }

  return (
    <WorkbenchCard
      id='personal-actions-title'
      testId='workbench-actions'
      title={t('Actions and reminders')}
      description={t('Nothing here runs automatically.')}
    >
      <div className='space-y-2'>
        <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'>
          <h3 id='personal-policy-title' className='text-sm font-medium'>
            {t('User usage policy')}
          </h3>
          {!props.identityLoading && !props.identityError ? (
            <StatusBadge
              variant={policyVariant}
              copyable={false}
              className='h-auto min-h-5 max-w-full whitespace-normal py-1'
            >
              <span className='min-w-0 break-words whitespace-normal leading-snug'>{policyLabel}</span>
            </StatusBadge>
          ) : null}
        </div>
        {identityContent}
        {props.freshUser?.role === ROLE.SUPER_ADMIN ? (
          <Button size='sm' variant='outline' onClick={props.onOpenPolicy}>
            <HugeiconsIcon
              icon={Wrench01Icon}
              strokeWidth={2}
              aria-hidden='true'
            />
            {t('Manage usage policy')}
          </Button>
        ) : null}
      </div>

      <div className='mt-4 border-t pt-4'>
        <h3 id='personal-reminder-title' className='text-sm font-medium'>
          {t('Request reminders')}
        </h3>
        {props.hasPendingBudget ? (
          <Alert className='mt-2'>
            <AlertTitle>{t('Pending request review')}</AlertTitle>
            <AlertDescription>
              {t('The selected Key has a request that needs separate review.')}
            </AlertDescription>
          </Alert>
        ) : (
          <p className='text-muted-foreground mt-1 text-xs'>
            {reminderLabel}
          </p>
        )}
      </div>

      <WorkbenchActionLinks canReadChannels={props.canReadChannels} />
    </WorkbenchCard>
  )
}
