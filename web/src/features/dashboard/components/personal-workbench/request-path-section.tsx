import { Link } from '@tanstack/react-router'
import { Refresh01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  NativeSelect,
  NativeSelectOption,
} from '@/components/ui/native-select'
import type { PlaygroundKey } from '@/features/playground/types'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'

import {
  getPlaygroundKeyCondition,
  type PlaygroundKeyCondition,
} from './workbench-state'
import { RequestFlow } from './request-flow'
import { WorkbenchCard } from './workbench-card'

function getConditionPresentation(
  condition: PlaygroundKeyCondition,
  t: (key: string) => string
) {
  if (condition === 'available') {
    return { label: t('Available'), variant: 'success' as const }
  }
  if (condition === 'expired') {
    return { label: t('Expired'), variant: 'warning' as const }
  }
  if (condition === 'exhausted') {
    return { label: t('Out of internal quota'), variant: 'warning' as const }
  }
  if (condition === 'disabled') {
    return { label: t('Disabled'), variant: 'neutral' as const }
  }
  return { label: t('Unknown'), variant: 'neutral' as const }
}

export function RequestPathSection(props: {
  keys: PlaygroundKey[] | undefined
  selectedKey: PlaygroundKey | undefined
  selectedKeyId: number | null
  visibleCount: number
  isLoading: boolean
  isError: boolean
  isFetching: boolean
  onSelectKey: (keyId: number | null) => void
  onLoadMore: () => void
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  const showKeys = useIsSidebarModuleVisible('/keys')
  const keys = props.keys ?? []
  const visibleKeys = keys.slice(0, props.visibleCount)
  const selectedCondition = props.selectedKey
    ? getPlaygroundKeyCondition(props.selectedKey)
    : null
  const selectedPresentation = selectedCondition
    ? getConditionPresentation(selectedCondition, t)
    : null

  return (
    <WorkbenchCard
      id='personal-request-path-title'
      testId='request-path-section'
      title={t('Request path')}
      description={t(
        'Select the Key you intend to use. No Key is selected automatically.'
      )}
    >
      <div className='grid min-w-0 gap-5 lg:grid-cols-[minmax(14rem,0.8fr)_minmax(0,1.6fr)]'>
        <div className='min-w-0 space-y-3'>
          <label
            htmlFor='personal-workbench-key'
            className='text-sm font-medium'
          >
            {t('Select an API Key')}
          </label>
          {props.isLoading ? (
            <Skeleton className='h-9 w-full' />
          ) : (
            <NativeSelect
              id='personal-workbench-key'
              className='w-full'
              value={props.selectedKey ? String(props.selectedKeyId) : ''}
              disabled={props.isError || keys.length === 0}
              onChange={(event) => {
                const value = Number(event.currentTarget.value)
                const next = keys.some((key) => key.id === value) ? value : null
                props.onSelectKey(next)
              }}
            >
              <NativeSelectOption value=''>
                {t('Choose a Key')}
              </NativeSelectOption>
              {visibleKeys.map((key) => {
                const state = getConditionPresentation(
                  getPlaygroundKeyCondition(key),
                  t
                )
                return (
                  <NativeSelectOption key={key.id} value={key.id}>
                    {key.name} · {state.label}
                  </NativeSelectOption>
                )
              })}
            </NativeSelect>
          )}

          {props.isError ? (
            <Alert variant='destructive' data-testid='keys-error-state'>
              <AlertTitle>{t('Unable to load Keys')}</AlertTitle>
              <AlertDescription className='space-y-2'>
                <p>{t('The Key list could not be refreshed.')}</p>
                <Button size='sm' variant='outline' onClick={props.onRefresh}>
                  {t('Retry')}
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}
          {!props.isLoading && !props.isError && keys.length === 0 ? (
            <div
              data-testid='keys-empty-state'
              className='rounded-lg border border-dashed p-4 text-sm'
            >
              <p className='font-medium'>{t('No API Keys yet')}</p>
              <p className='text-muted-foreground mt-1'>
                {t('Create a Key before testing a request.')}
              </p>
              {showKeys ? (
                <Button
                  className='mt-3'
                  size='sm'
                  variant='outline'
                  render={<Link to='/keys' />}
                >
                  {t('Create API Key')}
                </Button>
              ) : null}
            </div>
          ) : null}
          {keys.length > 0 ? (
            <div className='text-muted-foreground flex flex-wrap items-center gap-2 text-xs'>
              <span>
                {t('Showing {{shown}} of {{total}} Keys.', {
                  shown: Math.min(props.visibleCount, keys.length),
                  total: keys.length,
                })}
              </span>
              {visibleKeys.length < keys.length ? (
                <Button size='xs' variant='ghost' onClick={props.onLoadMore}>
                  {t('Load more Keys')}
                </Button>
              ) : null}
              <Button
                size='xs'
                variant='ghost'
                disabled={props.isFetching}
                onClick={props.onRefresh}
              >
                <HugeiconsIcon
                  icon={Refresh01Icon}
                  strokeWidth={2}
                  aria-hidden='true'
                />
                {t('Refresh Keys')}
              </Button>
            </div>
          ) : null}
        </div>

        <RequestFlow
          selectedKey={props.selectedKey}
          condition={selectedCondition}
          statusLabel={selectedPresentation?.label}
          statusVariant={selectedPresentation?.variant}
        />
      </div>
    </WorkbenchCard>
  )
}
