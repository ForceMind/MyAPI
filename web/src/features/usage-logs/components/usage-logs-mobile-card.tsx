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
import { ArrowRight01Icon, Database01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { flexRender, type Row, type Table } from '@tanstack/react-table'
import { Fragment, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { StatusBadge, type StatusBadgeProps } from '@/components/status-badge'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { getUserAvatarFallback, getUserAvatarStyle } from '@/lib/avatar'
import { cn } from '@/lib/utils'

import type { UsageLog } from '../data/schema'
import { parseLogOther, renderAuditContent } from '../lib/format'
import { getRequestFacts, isRequestLog } from '../lib/request-facts'
import { getLogTypeConfig, isTimingLogType } from '../lib/utils'
import type { LogCategory } from '../types'
import { DetailsDialog } from './dialogs/details-dialog'
import { LogCostDisplay } from './log-cost-display'
import { RequestTime } from './request-summary'
import { StreamTpsCell, TimingMetricsCell } from './timing-metrics-cell'
import {
  useLogMoneyVisibility,
  useLogsViewScope,
  useUsageLogsContext,
} from './usage-logs-provider'

// Same row as the Key page's recent requests: the whole row is one target.
const ROW_TARGET_CLASS =
  'hover:bg-muted flex min-h-12 min-w-0 flex-1 items-center gap-3 px-4 py-2.5 text-left focus-visible:outline-2 focus-visible:-outline-offset-2'
const SKELETON_ROWS = ['first', 'second', 'third', 'fourth'] as const

type TaskRowFields = {
  /** What names the task, on the first line. */
  headline: string[]
  /** Its state, at the end of the first line. */
  status: string[]
  /** Time, duration, progress and recorded cost. */
  meta: string[]
  /** Results with their own dialogs, each with a label. */
  details: { id: string; label: string }[]
}

interface UsageLogsMobileListProps<TData> {
  table: Table<TData>
  isLoading?: boolean
  emptyTitle?: string
  emptyDescription?: string
  logCategory: LogCategory
}

/**
 * One call record on a phone: time, type and Request ID; then model, usage,
 * Key and (for an administrator) connection and user; then the timing. The
 * whole row opens the request page, or the record's dialog when it is not a
 * relayed request with an ID. Missing usage is "not provided", never zero.
 */
function CommonLogRow(props: { log: UsageLog }) {
  const { t } = useTranslation()
  const showCosts = useLogMoneyVisibility()
  const { isAdminView } = useLogsViewScope()
  const { sensitiveVisible, setSelectedUserId, setUserInfoDialogOpen, rememberRequestFocus } =
    useUsageLogsContext()
  const [dialogOpen, setDialogOpen] = useState(false)
  const log = props.log
  const other = parseLogOther(log.other)
  const config = getLogTypeConfig(log.type, other)
  const relayed = isRequestLog(log)
  const requestFacts = getRequestFacts(log)
  const hidden = '••••'
  const hasUnconfirmedSettlement =
    other?.settlement_status === 'pending' ||
    other?.settlement_status === 'applied_journal_pending'

  const facts: { key: string; text: string; className?: string }[] = []
  if (relayed) {
    facts.push({
      key: 'model',
      text: log.model_name || t('Unknown model'),
      className: 'text-foreground font-mono',
    })
    facts.push({
      key: 'usage',
      text: requestFacts.usageKnown
        ? t('{{count}} Tokens', {
            count: log.prompt_tokens + log.completion_tokens,
          })
        : t('Usage not provided'),
    })
    if (log.token_name) {
      facts.push({
        key: 'key',
        text: sensitiveVisible ? log.token_name : hidden,
      })
    }
    if (isAdminView && log.channel) {
      facts.push({
        key: 'connection',
        text:
          sensitiveVisible && log.channel_name
            ? log.channel_name
            : `#${log.channel}`,
      })
    }
  } else {
    const content = renderAuditContent(other, t) ?? log.content
    if (content) facts.push({ key: 'content', text: content })
  }
  if (isAdminView && log.username) {
    facts.push({
      key: 'user',
      text: sensitiveVisible ? log.username : hidden,
    })
  }

  const useTime = log.use_time || 0
  const body = (
    <>
      <div className='flex min-w-0 flex-1 flex-col gap-1'>
        {/* A long type label pushes the Request ID to its own line instead
            of squeezing it to nothing. */}
        <div className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5'>
          <span className='text-muted-foreground shrink-0 font-mono text-xs'>
            <RequestTime seconds={log.created_at} />
          </span>
          <StatusBadge
            label={t(config.label)}
            variant={config.color as StatusBadgeProps['variant']}
            copyable={false}
            className='h-auto min-h-5 shrink-0 whitespace-normal'
          />
          {log.request_id && (
            <span className='text-muted-foreground min-w-0 truncate font-mono text-xs'>
              {log.request_id}
            </span>
          )}
        </div>
        {relayed && (
          <div className='flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-xs'>
            <StatusBadge
              label={t(requestFacts.outcome.label)}
              variant={requestFacts.outcome.variant}
              copyable={false}
              className='h-auto min-h-5 whitespace-normal'
            />
            <span className='text-muted-foreground font-mono'>
              {requestFacts.httpStatus == null
                ? t('HTTP status not recorded')
                : `HTTP ${requestFacts.httpStatus}`}
            </span>
            <StatusBadge
              label={t(requestFacts.settlement.label)}
              variant={requestFacts.settlement.variant}
              copyable={false}
              className='h-auto min-h-5 whitespace-normal'
            />
          </div>
        )}
        {facts.length > 0 && (
          <div className='text-muted-foreground flex min-w-0 flex-wrap items-baseline gap-x-1.5 text-xs'>
            {facts.map((fact, index) => (
              <Fragment key={fact.key}>
                {index > 0 && <span aria-hidden='true'>·</span>}
                <span className={cn('min-w-0 wrap-anywhere', fact.className)}>
                  {fact.text}
                </span>
              </Fragment>
            ))}
          </div>
        )}
        {isTimingLogType(log.type) && (
          <div
            data-slot='mobile-log-timing'
            className='flex min-w-0 flex-wrap items-center gap-x-4 gap-y-2'
          >
            <TimingMetricsCell
              useTimeSec={useTime}
              completionTokens={log.completion_tokens}
              frtMs={other?.frt}
              isStream={log.is_stream}
              indicator='dot'
              className='min-w-0 flex-1 basis-36'
            />
            <StreamTpsCell
              isStream={log.is_stream}
              tokensPerSecond={
                useTime > 0 && log.completion_tokens > 0
                  ? log.completion_tokens / useTime
                  : null
              }
              streamStatus={other?.stream_status}
              className='shrink-0'
            />
          </div>
        )}
      </div>
      <HugeiconsIcon
        icon={ArrowRight01Icon}
        className='text-muted-foreground size-4 shrink-0'
        aria-hidden='true'
      />
    </>
  )

  return (
    <li data-log-row='' className='flex min-w-0 flex-col'>
      <div className='flex min-w-0 items-stretch'>
        {log.request_id && relayed ? (
          <Link
            to='/usage-logs/request/$requestId'
            params={{ requestId: log.request_id }}
            state={{ inspectorBack: true }}
            data-usage-request-link={log.id}
            onClick={(event) => {
              if (event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey) rememberRequestFocus?.(log.id)
            }}
            title={t('Click to view full details')}
            className={ROW_TARGET_CLASS}
          >
            {body}
          </Link>
        ) : (
          <button
            type='button'
            title={t('Click to view full details')}
            className={ROW_TARGET_CLASS}
            onClick={() => setDialogOpen(true)}
          >
            {body}
          </button>
        )}
        {log.request_id && (
          <CopyButton
            value={log.request_id}
            aria-label={t('Copy Request ID')}
            className='min-h-11 min-w-11 self-center'
          />
        )}
        {isAdminView && log.username && (
          <button
            type='button'
            className='flex min-h-11 min-w-11 shrink-0 items-center justify-center px-2'
            aria-label={t('User: {{name}}', {
              name: sensitiveVisible ? log.username : hidden,
            })}
            onClick={() => {
              setSelectedUserId(log.user_id)
              setUserInfoDialogOpen(true)
            }}
          >
            <Avatar className='ring-border/60 size-7 ring-1'>
              <AvatarFallback
                className={cn(
                  'text-[11px] font-semibold',
                  !sensitiveVisible && 'bg-muted text-muted-foreground'
                )}
                style={
                  sensitiveVisible ? getUserAvatarStyle(log.username) : undefined
                }
              >
                {sensitiveVisible ? getUserAvatarFallback(log.username) : '•'}
              </AvatarFallback>
            </Avatar>
          </button>
        )}
      </div>
      {showCosts && !hasUnconfirmedSettlement && (log.type === 2 || log.type === 6) && (
        <div className='px-4 pb-2.5'>
          {Number.isFinite(log.quota) ? (
            <LogCostDisplay
              quota={log.quota}
              other={other}
              showUsageAccuracy={log.type === 2}
            />
          ) : (
            <span className='text-muted-foreground text-xs'>{t('Unknown')}</span>
          )}
        </div>
      )}
      {!(log.request_id && relayed) && (
        <DetailsDialog
          log={log}
          isAdmin={isAdminView}
          open={dialogOpen}
          onOpenChange={setDialogOpen}
        />
      )}
    </li>
  )
}

/**
 * A task or drawing on a phone, in the same row as a call record. Its cells
 * come from the PC columns so previews, prompts and failure reasons keep
 * their own dialogs.
 */
function TaskLogRow<TData>(props: { row: Row<TData>; fields: TaskRowFields }) {
  const { t } = useTranslation()
  // Desktop column visibility must never remove a task's mobile identity,
  // state or result. Role restrictions are already applied by the factories.
  const allCells = props.row.getAllCells()
  const cells = new Map(allCells.map((cell) => [cell.column.id, cell]))
  const primaryIds = new Set([
    ...props.fields.headline,
    ...props.fields.status,
    ...props.fields.meta,
    ...props.fields.details.map((detail) => detail.id),
  ])
  const extraCells = allCells.filter((cell) => !primaryIds.has(cell.column.id))
  const render = (id: string): ReactNode => {
    const cell = cells.get(id)
    if (!cell) return null
    return (
      <div key={id} className='min-w-0'>
        {id === 'quota' && (
          <span className='text-muted-foreground block text-xs'>
            {t('Recorded quota')}
          </span>
        )}
        {flexRender(cell.column.columnDef.cell, cell.getContext())}
      </div>
    )
  }
  const details = props.fields.details.filter((detail) => cells.has(detail.id))

  return (
    <li data-log-row='' className='flex min-w-0 flex-col gap-1.5 px-4 py-2.5 [&_a]:min-h-11 [&_a]:min-w-11 [&_button]:min-h-11 [&_button]:min-w-11'>
      <div className='flex min-w-0 items-start justify-between gap-3'>
        <div className='flex min-w-0 flex-wrap items-center gap-1.5'>
          {props.fields.headline.map(render)}
        </div>
        <div className='flex min-w-0 max-w-[45%] flex-wrap items-center gap-1.5'>
          {props.fields.status.map(render)}
        </div>
      </div>
      <div className='text-muted-foreground flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-xs'>
        {props.fields.meta.map(render)}
      </div>
      {details.length > 0 && (
        <dl className='flex min-w-0 flex-col gap-1 text-xs'>
          {details.map((detail) => (
            <div key={detail.id} className='flex min-w-0 items-center gap-2'>
              <dt className='text-muted-foreground shrink-0'>{detail.label}</dt>
              <dd className='min-w-0'>{render(detail.id)}</dd>
            </div>
          ))}
        </dl>
      )}
      {extraCells.length > 0 && (
        <details className='min-w-0'>
          <summary className='text-primary flex min-h-11 cursor-pointer items-center text-xs font-medium focus-visible:outline-2'>
            {t('More details')}
          </summary>
          <dl className='flex min-w-0 flex-col gap-2 pb-1 text-xs'>
            {extraCells.map((cell) => {
              const definition = cell.column.columnDef
              const label =
                definition.meta?.label ??
                (typeof definition.header === 'string'
                  ? definition.header
                  : cell.column.id)
              return (
                <div key={cell.column.id} className='min-w-0 space-y-1'>
                  <dt className='text-muted-foreground'>{label}</dt>
                  <dd className='min-w-0'>{render(cell.column.id)}</dd>
                </div>
              )
            })}
          </dl>
        </details>
      )}
    </li>
  )
}

export function UsageLogsMobileList<TData>(
  props: UsageLogsMobileListProps<TData>
) {
  const { t } = useTranslation()

  if (props.isLoading) {
    return (
      <div
        className='bg-card flex flex-col gap-2 rounded-xl border p-3'
        aria-busy='true'
      >
        <span className='sr-only'>{t('Loading...')}</span>
        {SKELETON_ROWS.map((id) => (
          <Skeleton key={id} className='h-14 w-full' />
        ))}
      </div>
    )
  }

  const rows = props.table.getRowModel().rows
  if (rows.length === 0) {
    return (
      <Empty className='bg-card rounded-xl border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={Database01Icon} aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{props.emptyTitle ?? t('No Logs Found')}</EmptyTitle>
          <EmptyDescription>
            {props.emptyDescription ??
              t(
                'No usage logs available. Logs will appear here once API calls are made.'
              )}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  const taskFields: TaskRowFields =
    props.logCategory === 'drawing'
      ? {
          headline: ['mj_id', 'platform', 'action'],
          status: ['status'],
          meta: ['submit_time', 'duration', 'progress', 'quota'],
          details: [
            { id: 'image_url', label: t('Image') },
            { id: 'fail_reason', label: t('Fail Reason') },
          ],
        }
      : {
          headline: ['task_id', 'platform', 'action'],
          status: ['status'],
          meta: ['submit_time', 'duration', 'progress', 'quota'],
          details: [{ id: 'fail_reason', label: t('Details') }],
        }

  return (
    <ul
      role='list'
      className='bg-card flex min-w-0 flex-col divide-y overflow-hidden rounded-xl border'
    >
      {rows.map((row) =>
        props.logCategory === 'common' ? (
          <CommonLogRow key={row.id} log={row.original as UsageLog} />
        ) : (
          <TaskLogRow key={row.id} row={row} fields={taskFields} />
        )
      )}
    </ul>
  )
}
