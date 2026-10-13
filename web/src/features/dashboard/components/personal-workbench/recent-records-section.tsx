import { Link } from '@tanstack/react-router'
import { Refresh01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import type {
  RecentLogOverview,
  RecentLogOverviewItem,
} from '@/features/dashboard/types'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { toIntlLocale } from '@/i18n/languages'

import { WorkbenchCard } from './workbench-card'

function RecentList(props: {
  rows: RecentLogOverviewItem[]
  emptyLabel: string
  dateFormatter: Intl.DateTimeFormat
}) {
  const { t } = useTranslation()
  if (props.rows.length === 0) {
    return (
      <p className='text-muted-foreground py-4 text-center text-xs'>
        {props.emptyLabel}
      </p>
    )
  }
  return (
    <ul className='divide-y'>
      {props.rows.slice(0, 5).map((row) => {
        const date = Number.isFinite(row.created_at)
          ? new Date(row.created_at * 1000)
          : null
        const validDate = date && Number.isFinite(date.getTime()) ? date : null
        return (
          <li
            key={row.id}
            className='flex min-w-0 items-center justify-between gap-3 py-2.5 text-xs'
          >
            <span className='min-w-0 flex-1 truncate font-medium'>
              {row.model_name?.trim() || t('Unknown')}
            </span>
            {validDate ? (
              <time
                className='text-muted-foreground shrink-0 tabular-nums'
                dateTime={validDate.toISOString()}
              >
                {props.dateFormatter.format(validDate)}
              </time>
            ) : (
              <span className='text-muted-foreground'>{t('Unknown')}</span>
            )}
          </li>
        )
      })}
    </ul>
  )
}

export function RecentRecordsSection(props: {
  data: RecentLogOverview | undefined
  isLoading: boolean
  isError: boolean
  isFetching: boolean
  onRefresh: () => void
}) {
  const { i18n, t } = useTranslation()
  const showLogs = useIsSidebarModuleVisible('/usage-logs/common')
  const dateFormatter = useMemo(
    () =>
      new Intl.DateTimeFormat(toIntlLocale(i18n.language), {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      }),
    [i18n.language]
  )
  const empty =
    props.data?.requests.length === 0 && props.data.errors.length === 0
  let body: ReactNode
  if (props.isLoading) {
    body = (
      <div className='grid gap-3 sm:grid-cols-2'>
        <Skeleton className='h-36 w-full' />
        <Skeleton className='h-36 w-full' />
      </div>
    )
  } else if (props.isError) {
    body = (
      <Alert variant='destructive' data-testid='recent-error-state'>
        <AlertTitle>{t('Unable to load your recent records')}</AlertTitle>
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
  } else if (empty) {
    body = (
      <p
        data-testid='recent-empty-state'
        className='text-muted-foreground rounded-lg border border-dashed px-3 py-8 text-center text-sm'
      >
        {t('No recent records')}
      </p>
    )
  } else if (props.data) {
    body = (
      <div className='grid min-w-0 gap-3 sm:grid-cols-2'>
        <div>
          <h3 id='personal-recorded-usage-title' className='text-sm font-medium'>
            {t('Recent request records')}
          </h3>
          <RecentList
            rows={props.data.requests}
            emptyLabel={t('No recent request records')}
            dateFormatter={dateFormatter}
          />
        </div>
        <div>
          <h3 id='personal-errors-title' className='text-sm font-medium'>
            {t('Errors to inspect')}
          </h3>
          <RecentList
            rows={props.data.errors}
            emptyLabel={t('No errors in this summary')}
            dateFormatter={dateFormatter}
          />
        </div>
      </div>
    )
  } else {
    body = null
  }

  return (
    <WorkbenchCard
      id='personal-recent-records-title'
      testId='recent-records-section'
      title={t('Your recent records')}
      description={t('Only your recent model and time metadata is shown.')}
    >
      {body}

      <div className='text-muted-foreground mt-3 flex flex-wrap items-center justify-between gap-2 border-t pt-3 text-xs'>
        <p className='max-w-xl'>
          {t(
            'This summary does not establish request completion, usage evidence, or settlement status.'
          )}
        </p>
        <div className='flex flex-wrap gap-1'>
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
            {t('Refresh records')}
          </Button>
          {showLogs ? (
            <Button
              size='xs'
              variant='ghost'
              render={
                <Link
                  to='/usage-logs/$section'
                  params={{ section: 'common' }}
                />
              }
            >
              {t('View logs')}
            </Button>
          ) : null}
        </div>
      </div>
    </WorkbenchCard>
  )
}
