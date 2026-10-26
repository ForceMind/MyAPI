/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Refresh01Icon, Task01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type {
  SystemTask,
  SystemTaskStatus,
} from '@/features/system-settings/types'
import { useMediaQuery } from '@/hooks/use-media-query'
import { toIntlLocale } from '@/i18n/languages'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { formatTimestampRelative, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  ACTIVE_TASK_POLL_INTERVAL_MS,
  isActiveTaskStatus,
  useSystemTasks,
} from '../hooks'

const STATUS_VARIANT: Record<SystemTaskStatus, StatusVariant> = {
  pending: 'warning',
  running: 'info',
  succeeded: 'success',
  failed: 'danger',
}

const PROGRESS_BAR_CLASS_NAME: Record<SystemTaskStatus, string> = {
  pending: '[&_[data-slot=progress-indicator]]:bg-warning',
  running: '[&_[data-slot=progress-indicator]]:bg-info',
  succeeded: '[&_[data-slot=progress-indicator]]:bg-success',
  failed: '[&_[data-slot=progress-indicator]]:bg-destructive',
}

// Maps backend system task type constants to i18n source keys. Unknown/future
// types fall back to their raw identifier so the panel never shows blank.
const TYPE_LABEL: Record<string, string> = {
  log_cleanup: 'Log cleanup',
  channel_test: 'Batch channel test',
  model_update: 'Batch upstream model update',
  midjourney_poll: 'Drawing task polling',
  async_task_poll: 'Async task polling',
}

const TYPE_DISPLAY_ID: Record<string, string> = {
  midjourney_poll: 'drawing_task_poll',
}

function getProgress(task: SystemTask): number | null {
  const progress = (task.state as { progress?: unknown } | undefined)?.progress
  if (typeof progress !== 'number' || Number.isNaN(progress)) return null
  return Math.min(100, Math.max(0, progress))
}

type SystemTasksListProps = {
  tasks: SystemTask[]
}

function SystemTasksTable(props: SystemTasksListProps) {
  const { t, i18n } = useTranslation()

  return (
    <div className='overflow-x-auto rounded-lg border'>
      <Table className='min-w-[900px]'>
        <TableHeader>
          <TableRow className='hover:bg-transparent'>
            <TableHead className='h-9 w-[260px] px-4'>{t('Type')}</TableHead>
            <TableHead className='h-9 w-[130px]'>{t('Status')}</TableHead>
            <TableHead className='h-9 w-[180px]'>{t('Progress')}</TableHead>
            <TableHead className='h-9 min-w-[260px]'>{t('Executor')}</TableHead>
            <TableHead className='h-9 w-[190px]'>{t('Updated')}</TableHead>
            <TableHead className='h-9 w-[220px] pr-4'>{t('Detail')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.tasks.map((task) => {
            const progress = getProgress(task)
            return (
              <TableRow key={task.task_id}>
                <TableCell className='px-4 py-3 align-middle'>
                  <div className='flex flex-col gap-0.5'>
                    <div className='font-medium'>
                      {t(TYPE_LABEL[task.type] ?? task.type)}
                    </div>
                    <div className='text-muted-foreground font-mono text-[11px]'>
                      {TYPE_DISPLAY_ID[task.type] ?? task.type}
                    </div>
                  </div>
                </TableCell>
                <TableCell className='py-3 align-middle'>
                  <StatusBadge
                    label={t(task.status)}
                    variant={STATUS_VARIANT[task.status]}
                    copyable={false}
                  />
                </TableCell>
                <TableCell className='py-3 align-middle'>
                  <div className='flex items-center gap-2'>
                    <Progress
                      value={progress ?? 0}
                      className={cn(
                        'w-24',
                        PROGRESS_BAR_CLASS_NAME[task.status]
                      )}
                    />
                    <span className='text-muted-foreground w-10 text-right text-xs tabular-nums'>
                      {progress === null ? '-' : `${progress}%`}
                    </span>
                  </div>
                </TableCell>
                <TableCell className='text-muted-foreground max-w-[280px] truncate py-3 align-middle font-mono text-xs'>
                  {task.locked_by || '-'}
                </TableCell>
                <TableCell
                  className='text-muted-foreground py-3 align-middle text-xs whitespace-nowrap'
                  title={formatTimestampToDate(task.updated_at)}
                >
                  {formatTimestampRelative(
                    task.updated_at,
                    'seconds',
                    toIntlLocale(i18n.language)
                  )}
                </TableCell>
                <TableCell
                  className='text-destructive max-w-[220px] truncate py-3 pr-4 align-middle text-xs'
                  title={task.error || undefined}
                >
                  {task.error || '-'}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}

/** Phones: one record per task instead of the wide table. */
function SystemTaskRecords(props: SystemTasksListProps) {
  const { t, i18n } = useTranslation()

  return (
    <ul className='divide-border flex flex-col divide-y overflow-hidden rounded-lg border'>
      {props.tasks.map((task) => {
        const progress = getProgress(task)
        return (
          <li key={task.task_id} className='flex min-w-0 flex-col gap-2 p-3'>
            <div className='flex min-w-0 items-start justify-between gap-2'>
              <div className='min-w-0'>
                <div className='text-sm font-medium wrap-anywhere'>
                  {t(TYPE_LABEL[task.type] ?? task.type)}
                </div>
                <div className='text-muted-foreground font-mono text-[11px] wrap-anywhere'>
                  {TYPE_DISPLAY_ID[task.type] ?? task.type}
                </div>
              </div>
              <StatusBadge
                label={t(task.status)}
                variant={STATUS_VARIANT[task.status]}
                copyable={false}
                className='shrink-0'
              />
            </div>
            <div className='flex items-center gap-2'>
              <Progress
                value={progress ?? 0}
                aria-label={t('Progress')}
                className={cn(
                  'min-w-0 flex-1',
                  PROGRESS_BAR_CLASS_NAME[task.status]
                )}
              />
              <span className='text-muted-foreground w-10 shrink-0 text-right text-xs tabular-nums'>
                {progress === null ? '-' : `${progress}%`}
              </span>
            </div>
            <div className='text-caption text-muted-foreground flex min-w-0 flex-wrap gap-x-3 gap-y-0.5'>
              <span title={formatTimestampToDate(task.updated_at)}>
                {t('Updated')}:{' '}
                {formatTimestampRelative(
                  task.updated_at,
                  'seconds',
                  toIntlLocale(i18n.language)
                )}
              </span>
              <span className='min-w-0 font-mono wrap-anywhere'>
                {t('Executor')}: {task.locked_by || '-'}
              </span>
            </div>
            {task.error && (
              <p className='text-caption text-destructive wrap-anywhere'>
                {task.error}
              </p>
            )}
          </li>
        )
      })}
    </ul>
  )
}

function SystemTaskGroup(props: {
  title: string
  description: string
  emptyText: string
  tasks: SystemTask[]
  mobile: boolean
}) {
  let content = (
    <div className='text-muted-foreground rounded-lg border border-dashed px-4 py-6 text-center text-sm'>
      {props.emptyText}
    </div>
  )
  if (props.tasks.length > 0) {
    content = props.mobile ? (
      <SystemTaskRecords tasks={props.tasks} />
    ) : (
      <SystemTasksTable tasks={props.tasks} />
    )
  }

  return (
    <div className='flex min-w-0 flex-col gap-2'>
      <div className='flex items-center justify-between gap-3'>
        <div className='min-w-0'>
          <h3 className='text-sm font-semibold'>{props.title}</h3>
          <p className='text-muted-foreground mt-0.5 text-xs'>
            {props.description}
          </p>
        </div>
        <Badge variant='outline' className='shrink-0 tabular-nums'>
          {props.tasks.length}
        </Badge>
      </div>
      {content}
    </div>
  )
}

export function SystemTasksPanel() {
  const { t } = useTranslation()
  const mobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const tasksQuery = useSystemTasks()

  const tasks = tasksQuery.data ?? []
  const loading = tasksQuery.isLoading
  const refreshing = tasksQuery.isFetching && !tasksQuery.isLoading
  const activeTasks = tasks.filter((task) => isActiveTaskStatus(task.status))
  const historyTasks = tasks.filter((task) => !isActiveTaskStatus(task.status))
  const hasActiveTasks = activeTasks.length > 0

  return (
    <Card data-card-hover='false'>
      <CardHeader className='has-data-[slot=card-action]:grid-cols-1 sm:has-data-[slot=card-action]:grid-cols-[1fr_auto]'>
        <CardTitle>
          <h2>{t('System Tasks')}</h2>
        </CardTitle>
        <CardDescription>
          {t(
            'Recent maintenance tasks running across instances and their execution status.'
          )}
        </CardDescription>
        <CardAction className='col-start-1 row-span-1 row-start-3 justify-self-stretch sm:col-start-2 sm:row-span-2 sm:row-start-1 sm:justify-self-end'>
          <div className='flex flex-wrap items-center gap-x-3 gap-y-2'>
            <span
              className='text-muted-foreground inline-flex items-center gap-1.5 text-xs'
              aria-live='polite'
            >
              <span
                className={cn(
                  'size-1.5 rounded-full',
                  hasActiveTasks ? 'bg-success' : 'bg-muted-foreground/40'
                )}
                aria-hidden='true'
              />
              {hasActiveTasks
                ? t('Auto-refreshing every {{seconds}}s', {
                    seconds: ACTIVE_TASK_POLL_INTERVAL_MS / 1000,
                  })
                : t('Live refresh pauses when no task is running')}
            </span>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => void tasksQuery.refetch()}
              disabled={tasksQuery.isFetching}
              aria-label={t('Refresh')}
            >
              <HugeiconsIcon
                icon={Refresh01Icon}
                data-icon='inline-start'
                aria-hidden='true'
                className={cn(refreshing && 'animate-spin')}
              />
              {refreshing ? t('Refreshing...') : t('Refresh')}
            </Button>
          </div>
        </CardAction>
      </CardHeader>

      <CardContent aria-busy={tasksQuery.isFetching}>
        {loading && (
          <div className='flex flex-col gap-2'>
            {['task-1', 'task-2', 'task-3', 'task-4'].map((id) => (
              <Skeleton key={id} className='h-9 w-full rounded-md' />
            ))}
          </div>
        )}
        {!loading && tasksQuery.isError && (
          <ErrorState
            title={t('We could not load system tasks.')}
            description={
              tasksQuery.error instanceof Error
                ? tasksQuery.error.message
                : undefined
            }
            onRetry={() => {
              void tasksQuery.refetch()
            }}
            className='min-h-[260px]'
          />
        )}
        {!loading && !tasksQuery.isError && tasks.length === 0 && (
          <Empty className='border-none p-6'>
            <EmptyHeader>
              <EmptyMedia variant='icon'>
                <HugeiconsIcon icon={Task01Icon} aria-hidden='true' />
              </EmptyMedia>
              <EmptyTitle>{t('No system tasks yet.')}</EmptyTitle>
            </EmptyHeader>
          </Empty>
        )}
        {!loading && !tasksQuery.isError && tasks.length > 0 && (
          <div className='flex flex-col gap-5'>
            <SystemTaskGroup
              title={t('Active Tasks')}
              description={t('Tasks currently pending or running.')}
              emptyText={t('No active system tasks.')}
              tasks={activeTasks}
              mobile={mobile}
            />
            <SystemTaskGroup
              title={t('Task History')}
              description={t('Recently completed or failed system task runs.')}
              emptyText={t('No historical system tasks.')}
              tasks={historyTasks}
              mobile={mobile}
            />
          </div>
        )}
      </CardContent>
    </Card>
  )
}
