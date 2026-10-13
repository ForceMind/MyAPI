import {
  Refresh01Icon,
  Wrench01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import type { TokenBudgetView } from '@/features/keys/token-budget-api'
import type { PlaygroundKey } from '@/features/playground/types'

import { BudgetLimitGrid } from './budget-limit-grid'
import { WorkbenchCard } from './workbench-card'

export function KeyBudgetSection(props: {
  selectedKey: PlaygroundKey | undefined
  budget: TokenBudgetView | undefined
  isLoading: boolean
  isError: boolean
  isFetching: boolean
  onRefresh: () => void
  onOpenSettings: () => void
}) {
  const { t } = useTranslation()

  return (
    <WorkbenchCard
      id='personal-key-budget-title'
      testId='key-budget-section'
      title={t('Key and request limits')}
      description={t(
        'These controls apply to different units and must be read separately.'
      )}
    >
      {!props.selectedKey ? (
        <div
          data-testid='key-selection-empty'
          className='rounded-xl border border-dashed px-4 py-8 text-center'
        >
          <p className='font-medium'>{t('Choose a Key to inspect its limits')}</p>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('No Key is selected automatically.')}
          </p>
        </div>
      ) : (
        <div className='space-y-4'>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <p className='min-w-0 truncate text-sm font-medium'>
              {props.selectedKey.name}
            </p>
            <div className='flex flex-wrap gap-2'>
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
                {t('Refresh budget')}
              </Button>
              <Button
                size='sm'
                variant='secondary'
                onClick={props.onOpenSettings}
              >
                <HugeiconsIcon
                  icon={Wrench01Icon}
                  strokeWidth={2}
                  aria-hidden='true'
                />
                {t('Open budget settings')}
              </Button>
            </div>
          </div>

          {props.isError ? (
            <Alert variant='destructive' data-testid='budget-error-state'>
              <AlertTitle>{t('Unable to load this Key budget')}</AlertTitle>
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
          ) : null}

          <BudgetLimitGrid
            selectedKey={props.selectedKey}
            budget={props.budget}
            loading={props.isLoading}
          />

          {props.budget?.pending ? (
            <Alert>
              <AlertTitle>{t('Pending request review')}</AlertTitle>
              <AlertDescription>
                {t(
                  'This Key has a pending request that requires a separate review.'
                )}
              </AlertDescription>
            </Alert>
          ) : null}
          <p className='text-muted-foreground text-xs'>
            {t('Budget information is read only until you open settings.')}
          </p>
        </div>
      )}
    </WorkbenchCard>
  )
}
