import { useQueryClient } from '@tanstack/react-query'
import { lazy, Suspense } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { LoadingState } from '@/components/loading-state'
import { useConfirmedFundingMode } from '@/hooks/use-personal-workbench-mode'
import { useStatus } from '@/hooks/use-status'

const PersonalWorkbench = lazy(() =>
  import('../personal-workbench/personal-workbench').then(module => ({ default: module.PersonalWorkbench }))
)
const LegacyOverviewDashboard = lazy(() =>
  import('./legacy-overview-dashboard').then(module => ({ default: module.LegacyOverviewDashboard }))
)

export function OverviewDashboard() {
  const mode = useConfirmedFundingMode()
  const status = useStatus()
  const client = useQueryClient()
  const { t } = useTranslation()

  if (mode === 'disabled') {
    return <Suspense fallback={<LoadingState />}><PersonalWorkbench /></Suspense>
  }
  if (mode === 'enabled' || mode === 'retirement') {
    return <Suspense fallback={<LoadingState />}><LegacyOverviewDashboard /></Suspense>
  }
  if (status.loading || (!status.confirmed && !status.error && status.status !== null)) {
    return <LoadingState message={t('Confirming workspace mode...')} />
  }
  return (
    <Alert>
      <AlertTitle>{t('Unable to confirm workspace mode')}</AlertTitle>
      <AlertDescription>
        <p>{t('Refresh the server status to open your workspace.')}</p>
        <Button
          className='mt-3'
          variant='outline'
          onClick={() => void client.invalidateQueries({ queryKey: ['status'] })}
        >
          {t('Retry')}
        </Button>
      </AlertDescription>
    </Alert>
  )
}
