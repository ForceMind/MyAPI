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
import {
  Alert02Icon,
  Delete02Icon,
  Refresh01Icon,
  ServerStack01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { ErrorState } from '@/components/error-state'
import { StatusBadge } from '@/components/status-badge'
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
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useMediaQuery } from '@/hooks/use-media-query'
import { toIntlLocale } from '@/i18n/languages'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { formatTimestampRelative, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { deleteStaleSystemInstance, deleteStaleSystemInstances } from '../api'
import { INSTANCE_POLL_INTERVAL_MS, useSystemInstances } from '../hooks'
import type { SystemInstance } from '../types'

const INSTANCE_SKELETON_KEYS = [
  'system-instance-skeleton-1',
  'system-instance-skeleton-2',
  'system-instance-skeleton-3',
]

function roleLabel(instance: SystemInstance) {
  if (instance.info?.role?.is_master) return 'master'
  return 'worker'
}

function roleDescriptionKey(instance: SystemInstance) {
  if (instance.info?.role?.is_master) {
    return 'Master instances run scheduled background tasks.'
  }
  return 'Worker instances do not run master-only background tasks.'
}

function runtimeLabel(instance: SystemInstance) {
  const runtime = instance.info?.runtime
  if (!runtime?.goos && !runtime?.goarch) return '-'

  const parts: string[] = []
  if (runtime.goos || runtime.goarch) {
    parts.push([runtime.goos, runtime.goarch].filter(Boolean).join('/'))
  }
  return parts.join(' · ')
}

function getNodeName(instance: SystemInstance) {
  return instance.info?.node?.name || instance.node_name
}

function formatPercent(value?: number) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '-'
  return `${new Intl.NumberFormat(undefined, {
    maximumFractionDigits: 1,
  }).format(value)}%`
}

function formatBytes(bytes?: number): string {
  if (typeof bytes !== 'number' || Number.isNaN(bytes)) return '-'
  if (bytes === 0) return '0 B'
  if (bytes < 0) return `-${formatBytes(-bytes)}`

  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1
  )
  const value = bytes / 1024 ** index
  return `${new Intl.NumberFormat(undefined, {
    maximumFractionDigits: index === 0 ? 0 : 1,
  }).format(value)} ${units[index]}`
}

// Usage rings use the meter colours: fill below 70%, warning from 70%,
// critical from 90%; the value is always written next to the ring.
function ringColorClass(percent: number | null) {
  if (percent === null) return 'text-muted-foreground/40'
  if (percent >= 90) return 'text-destructive'
  if (percent >= 70) return 'text-warning'
  return 'text-meter'
}

type RingProgressProps = {
  percent: number | null
  size?: number
}

function RingProgress(props: RingProgressProps) {
  const size = props.size ?? 22
  const stroke = 2.5
  const radius = (size - stroke) / 2
  const circumference = 2 * Math.PI * radius
  const offset =
    props.percent === null
      ? circumference
      : circumference - (props.percent / 100) * circumference

  return (
    <svg
      width={size}
      height={size}
      viewBox={`0 0 ${size} ${size}`}
      className='shrink-0 -rotate-90'
      aria-hidden='true'
    >
      <circle
        cx={size / 2}
        cy={size / 2}
        r={radius}
        fill='none'
        strokeWidth={stroke}
        stroke='currentColor'
        className='text-meter-track'
      />
      <circle
        cx={size / 2}
        cy={size / 2}
        r={radius}
        fill='none'
        strokeWidth={stroke}
        strokeLinecap='round'
        stroke='currentColor'
        strokeDasharray={circumference}
        strokeDashoffset={offset}
        className={cn(
          'transition-[stroke-dashoffset] duration-500',
          ringColorClass(props.percent)
        )}
      />
    </svg>
  )
}

type ResourceCellProps = {
  value?: number
  tooltip?: ReactNode
}

function ResourceCell(props: ResourceCellProps) {
  const percent =
    typeof props.value === 'number' && !Number.isNaN(props.value)
      ? Math.max(0, Math.min(100, props.value))
      : null
  const content = (
    <div className='flex items-center gap-2'>
      <RingProgress percent={percent} />
      <span className='font-mono text-[11px] tabular-nums'>
        {formatPercent(props.value)}
      </span>
    </div>
  )

  if (!props.tooltip) return content

  return (
    <TooltipProvider delay={100}>
      <Tooltip>
        <TooltipTrigger className='block w-full rounded-sm text-left focus-visible:ring-2 focus-visible:outline-none'>
          {content}
        </TooltipTrigger>
        <TooltipContent className='max-w-80'>{props.tooltip}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

function InstanceStatusBadge(props: { instance: SystemInstance }) {
  const { t } = useTranslation()
  return (
    <StatusBadge
      label={t(props.instance.status)}
      variant={props.instance.status === 'online' ? 'success' : 'warning'}
      copyable={false}
      className='shrink-0'
    />
  )
}

/** Shown when the node runs on an automatic hostname. */
function NodeNameHint() {
  const { t } = useTranslation()
  return (
    <Popover>
      <PopoverTrigger
        className='inline-flex shrink-0 rounded-full focus-visible:ring-2 focus-visible:outline-none'
        aria-label={t('Configure NODE_NAME')}
      >
        <StatusBadge variant='warning' copyable={false}>
          <HugeiconsIcon
            icon={Alert02Icon}
            className='size-3.5'
            aria-hidden='true'
          />
        </StatusBadge>
      </PopoverTrigger>
      <PopoverContent align='start' className='w-80'>
        <PopoverHeader>
          <PopoverTitle>{t('Configure NODE_NAME')}</PopoverTitle>
          <PopoverDescription>
            {t(
              'This instance is using an automatic hostname. Set NODE_NAME to a stable unique value for multi-instance management.'
            )}
          </PopoverDescription>
        </PopoverHeader>
        <div className='flex flex-col gap-2 text-xs'>
          <div>
            <div className='mb-1 font-medium'>{t('Example')}</div>
            <code className='bg-muted block rounded-md px-2 py-1.5 font-mono text-[11px] break-all'>
              NODE_NAME=my-api-master-1
            </code>
          </div>
          <p className='text-muted-foreground'>
            {t(
              'Use a different stable value for each instance, then restart the service.'
            )}
          </p>
        </div>
      </PopoverContent>
    </Popover>
  )
}

type SystemInstancesListProps = {
  instances: SystemInstance[]
  deletingNodeName: string | null
  isDeletingInstance: boolean
  onDeleteStaleInstance: (instance: SystemInstance) => void
}

function SystemInstancesTable(props: SystemInstancesListProps) {
  const { t, i18n } = useTranslation()

  return (
    <div className='overflow-x-auto rounded-lg border'>
      <Table className='min-w-[1230px]'>
        <TableHeader>
          <TableRow className='hover:bg-transparent'>
            <TableHead className='h-9 min-w-[240px] px-4'>
              {t('Instances')}
            </TableHead>
            <TableHead className='h-9 w-[110px]'>{t('Status')}</TableHead>
            <TableHead className='h-9 w-[100px]'>{t('Role')}</TableHead>
            <TableHead className='h-9 w-[96px]'>{t('CPU')}</TableHead>
            <TableHead className='h-9 w-[96px]'>{t('Memory')}</TableHead>
            <TableHead className='h-9 w-[96px]'>{t('Storage')}</TableHead>
            <TableHead className='h-9 w-[100px]'>{t('Version')}</TableHead>
            <TableHead className='h-9 w-[140px]'>{t('Runtime')}</TableHead>
            <TableHead className='h-9 w-[170px]'>{t('Started')}</TableHead>
            <TableHead className='h-9 w-[170px]'>{t('Last Seen')}</TableHead>
            <TableHead className='h-9 w-[90px] pr-4 text-right'>
              {t('Actions')}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.instances.map((instance) => {
            const resources = instance.info?.resources
            const storage = resources?.storage
            const isDeletingThisInstance =
              props.isDeletingInstance &&
              props.deletingNodeName === instance.node_name
            return (
              <TableRow key={instance.node_name}>
                <TableCell className='px-4 py-2.5 align-middle'>
                  <div className='min-w-0'>
                    <div className='flex min-w-0 items-center gap-1.5'>
                      <span className='truncate text-sm font-medium'>
                        {getNodeName(instance)}
                      </span>
                      {instance.info?.node?.should_configure_manually ===
                        true && <NodeNameHint />}
                    </div>
                    <div className='text-muted-foreground truncate font-mono text-[11px]'>
                      {instance.info?.host?.hostname || '-'}
                    </div>
                  </div>
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <InstanceStatusBadge instance={instance} />
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <TooltipProvider delay={100}>
                    <Tooltip>
                      <TooltipTrigger
                        className='inline-flex shrink-0 rounded-full focus-visible:ring-2 focus-visible:outline-none'
                        aria-label={t('Node role')}
                      >
                        <Badge variant='outline'>{roleLabel(instance)}</Badge>
                      </TooltipTrigger>
                      <TooltipContent>
                        {t(roleDescriptionKey(instance))}
                      </TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <ResourceCell value={resources?.cpu?.usage_percent} />
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <ResourceCell value={resources?.memory?.usage_percent} />
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <ResourceCell
                    value={storage?.used_percent}
                    tooltip={
                      storage ? (
                        <div className='grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs'>
                          <span className='text-muted-foreground'>
                            {t('Used')}
                          </span>
                          <span className='font-mono'>
                            {formatBytes(storage.used_bytes)}
                          </span>
                          <span className='text-muted-foreground'>
                            {t('Free')}
                          </span>
                          <span className='font-mono'>
                            {formatBytes(storage.free_bytes)}
                          </span>
                          <span className='text-muted-foreground'>
                            {t('Total')}
                          </span>
                          <span className='font-mono'>
                            {formatBytes(storage.total_bytes)}
                          </span>
                        </div>
                      ) : undefined
                    }
                  />
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <div className='truncate font-mono text-xs'>
                    {instance.info?.runtime?.version || '-'}
                  </div>
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <div className='truncate font-mono text-xs'>
                    {runtimeLabel(instance)}
                  </div>
                </TableCell>
                <TableCell className='text-muted-foreground py-2.5 align-middle text-xs whitespace-nowrap'>
                  {formatTimestampToDate(instance.started_at)}
                </TableCell>
                <TableCell
                  className='text-muted-foreground py-2.5 align-middle text-xs whitespace-nowrap'
                  title={formatTimestampToDate(instance.last_seen_at)}
                >
                  {formatTimestampRelative(
                    instance.last_seen_at,
                    'seconds',
                    toIntlLocale(i18n.language)
                  )}
                </TableCell>
                <TableCell className='py-2.5 pr-4 text-right align-middle'>
                  {instance.status === 'stale' ? (
                    <TooltipProvider delay={100}>
                      <Tooltip>
                        <TooltipTrigger
                          render={
                            <Button
                              type='button'
                              variant='destructive'
                              size='icon-xs'
                              onClick={() =>
                                props.onDeleteStaleInstance(instance)
                              }
                              disabled={
                                props.isDeletingInstance ||
                                isDeletingThisInstance
                              }
                              aria-label={t('Delete stale instance')}
                            >
                              {isDeletingThisInstance ? (
                                <Loader2
                                  className='size-3 animate-spin'
                                  aria-hidden='true'
                                />
                              ) : (
                                <HugeiconsIcon
                                  icon={Delete02Icon}
                                  aria-hidden='true'
                                />
                              )}
                            </Button>
                          }
                        />
                        <TooltipContent>
                          {t('Delete stale instance')}
                        </TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  ) : (
                    <span className='text-muted-foreground text-xs'>-</span>
                  )}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}

/** Phones: one record per instance instead of the wide table. */
function SystemInstanceRecords(props: SystemInstancesListProps) {
  const { t, i18n } = useTranslation()

  return (
    <ul className='divide-border flex flex-col divide-y overflow-hidden rounded-lg border'>
      {props.instances.map((instance) => {
        const resources = instance.info?.resources
        const isDeletingThisInstance =
          props.isDeletingInstance &&
          props.deletingNodeName === instance.node_name
        return (
          <li
            key={instance.node_name}
            className='flex min-w-0 flex-col gap-2 p-3'
          >
            <div className='flex min-w-0 items-start justify-between gap-2'>
              <div className='min-w-0'>
                <div className='flex min-w-0 items-center gap-1.5'>
                  <span className='text-sm font-medium wrap-anywhere'>
                    {getNodeName(instance)}
                  </span>
                  {instance.info?.node?.should_configure_manually === true && (
                    <NodeNameHint />
                  )}
                </div>
                <div className='text-muted-foreground font-mono text-[11px] wrap-anywhere'>
                  {instance.info?.host?.hostname || '-'}
                </div>
              </div>
              <InstanceStatusBadge instance={instance} />
            </div>
            <dl className='text-caption grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1'>
              <dt className='text-muted-foreground'>{t('Role')}</dt>
              <dd>
                {roleLabel(instance)}
                <span className='text-muted-foreground'>
                  {' · '}
                  {t(roleDescriptionKey(instance))}
                </span>
              </dd>
              <dt className='text-muted-foreground'>{t('CPU')}</dt>
              <dd>
                <ResourceCell value={resources?.cpu?.usage_percent} />
              </dd>
              <dt className='text-muted-foreground'>{t('Memory')}</dt>
              <dd>
                <ResourceCell value={resources?.memory?.usage_percent} />
              </dd>
              <dt className='text-muted-foreground'>{t('Storage')}</dt>
              <dd>
                <ResourceCell value={resources?.storage?.used_percent} />
              </dd>
              <dt className='text-muted-foreground'>{t('Version')}</dt>
              <dd className='font-mono wrap-anywhere'>
                {instance.info?.runtime?.version || '-'}
                {' · '}
                {runtimeLabel(instance)}
              </dd>
              <dt className='text-muted-foreground'>{t('Last Seen')}</dt>
              <dd title={formatTimestampToDate(instance.last_seen_at)}>
                {formatTimestampRelative(
                  instance.last_seen_at,
                  'seconds',
                  toIntlLocale(i18n.language)
                )}
              </dd>
            </dl>
            {instance.status === 'stale' && (
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='text-destructive self-start'
                onClick={() => props.onDeleteStaleInstance(instance)}
                disabled={props.isDeletingInstance || isDeletingThisInstance}
              >
                {isDeletingThisInstance ? (
                  <Loader2
                    data-icon='inline-start'
                    className='animate-spin'
                    aria-hidden='true'
                  />
                ) : (
                  <HugeiconsIcon
                    icon={Delete02Icon}
                    data-icon='inline-start'
                    aria-hidden='true'
                  />
                )}
                {t('Delete stale instance')}
              </Button>
            )}
          </li>
        )
      })}
    </ul>
  )
}

export function SystemInstancesPanel() {
  const { t } = useTranslation()
  const mobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const queryClient = useQueryClient()
  const [deleteTarget, setDeleteTarget] = useState<SystemInstance | null>(null)
  const [deleteAllConfirmOpen, setDeleteAllConfirmOpen] = useState(false)
  const [deletingNodeName, setDeletingNodeName] = useState<string | null>(null)
  const instancesQuery = useSystemInstances()

  const instances = instancesQuery.data ?? []
  const staleInstances = instances.filter(
    (instance) => instance.status === 'stale'
  )
  const hasStaleInstances = staleInstances.length > 0
  const loading = instancesQuery.isLoading
  const refreshing = instancesQuery.isFetching && !instancesQuery.isLoading

  const invalidateInstances = async () => {
    await queryClient.invalidateQueries({
      queryKey: ['system-info', 'instances'],
    })
  }

  const deleteStaleInstanceMutation = useMutation({
    mutationFn: async (nodeName: string) => {
      const res = await deleteStaleSystemInstance(nodeName)
      if (!res.success) {
        throw new Error(res.message || t('Delete failed'))
      }
      return res
    },
    onMutate: (nodeName) => {
      setDeletingNodeName(nodeName)
    },
    onSuccess: async () => {
      toast.success(t('Deleted stale instance'))
      await invalidateInstances()
      setDeleteTarget(null)
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Delete failed'))
      void invalidateInstances()
    },
    onSettled: () => {
      setDeletingNodeName(null)
    },
  })

  const deleteStaleInstancesMutation = useMutation({
    mutationFn: async () => {
      const res = await deleteStaleSystemInstances()
      if (!res.success) {
        throw new Error(res.message || t('Delete failed'))
      }
      return res
    },
    onSuccess: async (res) => {
      toast.success(
        t('Deleted {{count}} stale instances', {
          count: res.data?.deleted_count ?? 0,
        })
      )
      await invalidateInstances()
      setDeleteAllConfirmOpen(false)
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Delete failed'))
    },
  })

  const isMutatingInstance =
    deletingNodeName !== null || deleteStaleInstanceMutation.isPending
  const listProps: SystemInstancesListProps = {
    instances,
    deletingNodeName,
    isDeletingInstance:
      isMutatingInstance || deleteStaleInstancesMutation.isPending,
    onDeleteStaleInstance: setDeleteTarget,
  }

  let instancesContent: ReactNode
  if (loading) {
    instancesContent = (
      <div className='flex flex-col gap-2'>
        {INSTANCE_SKELETON_KEYS.map((key) => (
          <Skeleton key={key} className='h-9 w-full rounded-md' />
        ))}
      </div>
    )
  } else if (instancesQuery.isError) {
    instancesContent = (
      <ErrorState
        title={t('We could not load instances.')}
        description={
          instancesQuery.error instanceof Error
            ? instancesQuery.error.message
            : undefined
        }
        onRetry={() => {
          void instancesQuery.refetch()
        }}
        className='min-h-[220px]'
      />
    )
  } else if (instances.length === 0) {
    instancesContent = (
      <Empty className='border-none p-6'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={ServerStack01Icon} aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('No instances have reported yet.')}</EmptyTitle>
        </EmptyHeader>
      </Empty>
    )
  } else if (mobile) {
    instancesContent = <SystemInstanceRecords {...listProps} />
  } else {
    instancesContent = <SystemInstancesTable {...listProps} />
  }

  return (
    <>
      <Card data-card-hover='false'>
        <CardHeader className='has-data-[slot=card-action]:grid-cols-1 sm:has-data-[slot=card-action]:grid-cols-[1fr_auto]'>
          <CardTitle>
            <h2>{t('Instances')}</h2>
          </CardTitle>
          <CardDescription>
            {t(
              'Nodes reporting from this deployment and their latest heartbeat.'
            )}
          </CardDescription>
          <CardAction className='col-start-1 row-span-1 row-start-3 justify-self-stretch sm:col-start-2 sm:row-span-2 sm:row-start-1 sm:justify-self-end'>
            <div className='flex flex-wrap items-center gap-x-3 gap-y-2'>
              <span
                className='text-muted-foreground text-xs'
                aria-live='polite'
              >
                {t('Auto-refreshing every {{seconds}}s', {
                  seconds: INSTANCE_POLL_INTERVAL_MS / 1000,
                })}
              </span>
              {hasStaleInstances ? (
                <Button
                  type='button'
                  variant='destructive'
                  size='sm'
                  onClick={() => setDeleteAllConfirmOpen(true)}
                  disabled={
                    isMutatingInstance || deleteStaleInstancesMutation.isPending
                  }
                >
                  {deleteStaleInstancesMutation.isPending ? (
                    <Loader2
                      data-icon='inline-start'
                      className='animate-spin'
                      aria-hidden='true'
                    />
                  ) : (
                    <HugeiconsIcon
                      icon={Delete02Icon}
                      data-icon='inline-start'
                      aria-hidden='true'
                    />
                  )}
                  {t('Delete all stale')}
                </Button>
              ) : null}
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() => void instancesQuery.refetch()}
                disabled={instancesQuery.isFetching}
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

        <CardContent aria-busy={instancesQuery.isFetching}>
          {instancesContent}
        </CardContent>
      </Card>

      <ConfirmDialog
        open={deleteAllConfirmOpen}
        onOpenChange={setDeleteAllConfirmOpen}
        title={t('Delete stale instances')}
        desc={t(
          'Delete {{count}} stale instance records? Online instances will not be deleted.',
          { count: staleInstances.length }
        )}
        destructive
        isLoading={deleteStaleInstancesMutation.isPending}
        confirmText={
          deleteStaleInstancesMutation.isPending
            ? t('Deleting...')
            : t('Delete')
        }
        handleConfirm={() => deleteStaleInstancesMutation.mutate()}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
        title={t('Delete stale instance')}
        desc={
          deleteTarget
            ? t(
                'Delete stale instance "{{name}}"? If it has reported again, it will not be deleted.',
                { name: getNodeName(deleteTarget) }
              )
            : ''
        }
        destructive
        isLoading={deleteStaleInstanceMutation.isPending}
        confirmText={
          deleteStaleInstanceMutation.isPending ? t('Deleting...') : t('Delete')
        }
        handleConfirm={() => {
          if (!deleteTarget) return
          deleteStaleInstanceMutation.mutate(deleteTarget.node_name)
        }}
      />
    </>
  )
}
