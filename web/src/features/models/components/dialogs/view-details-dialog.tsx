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
import { useQuery } from '@tanstack/react-query'
import { Copy01Icon, LinkSquare02Icon, Loading03Icon, Refresh01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { SummaryDisclosure } from '@/components/summary-disclosure'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Separator } from '@/components/ui/separator'

import { getDeployment, listDeploymentContainers } from '../../api'
import { useDeploymentTask } from '../../hooks/use-deployment-task'

export function ViewDetailsDialog({
  open,
  onOpenChange,
  deploymentId,
  returnFocus,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  deploymentId: string | number | null
  returnFocus?: () => HTMLElement | null
}) {
  const { t } = useTranslation()
  const { identity } = useDeploymentTask(open, deploymentId)

  const {
    data: detailsRes,
    isLoading: isLoadingDetails,
    refetch: refetchDetails,
    isFetching: isFetchingDetails,
    isError: isDetailsError,
    error: detailsError,
  } = useQuery({
    queryKey: ['deployment-details', deploymentId, identity],
    queryFn: () => (deploymentId ? getDeployment(deploymentId) : null),
    enabled: open && deploymentId !== null,
  })

  const {
    data: containersRes,
    isLoading: isLoadingContainers,
    refetch: refetchContainers,
    isFetching: isFetchingContainers,
    isError: isContainersError,
    error: containersError,
  } = useQuery({
    queryKey: ['deployment-details-containers', deploymentId, identity],
    queryFn: () =>
      deploymentId ? listDeploymentContainers(deploymentId) : null,
    enabled: open && deploymentId !== null,
  })

  const details = isDetailsError ? undefined : detailsRes?.data
  const containersKnown = !isContainersError && containersRes?.success === true && Array.isArray(containersRes.data?.containers)
  const containers = useMemo(() => {
    const items = containersRes?.data?.containers
    return containersKnown && Array.isArray(items) ? items : []
  }, [containersKnown, containersRes?.data?.containers])

  const locations = useMemo(() => {
    const items = details?.locations
    if (!Array.isArray(items)) {
      return []
    }
    return items
      .map((x) => {
        if (!x || typeof x !== 'object') {
          return null
        }
        const name = (x as Record<string, unknown>)?.name
        const iso2 = (x as Record<string, unknown>)?.iso2
        const id = (x as Record<string, unknown>)?.id
        return `${String(name ?? id ?? '')}${iso2 ? ` (${iso2})` : ''}`.trim()
      })
      .filter(Boolean) as string[]
  }, [details])

  const containerStatusCounts = useMemo(() => {
    const counts = new Map<string | undefined, number>()
    for (const container of containers) {
      const status = typeof container?.status === 'string' && container.status ? container.status : undefined
      counts.set(status, (counts.get(status) ?? 0) + 1)
    }
    return [...counts.entries()]
  }, [containers])

  const handleCopyId = async () => {
    if (deploymentId === null || deploymentId === undefined) {
      return
    }
    try {
      await navigator.clipboard.writeText(String(deploymentId))
      toast.success(t('Copied'))
    } catch {
      toast.error(t('Copy failed'))
    }
  }

  const handleRefresh = () => {
    refetchDetails()
    refetchContainers()
  }

  const payloadJson = useMemo(() => {
    if (!details) return ''
    try {
      return JSON.stringify(details, null, 2)
    } catch {
      return ''
    }
  }, [details])
  const isDetailsLoading = isLoadingDetails || isLoadingContainers
  const showDetailsError = !isDetailsLoading && (isDetailsError || !detailsRes?.success)
  const showDetailsContent = !isDetailsLoading && !isDetailsError && detailsRes?.success

  return (
    <Dialog
      open={open}
      returnFocus={returnFocus}
      onOpenChange={onOpenChange}
      title={t('Deployment details')}
      contentClassName='h-dvh max-h-dvh w-screen max-w-none sm:w-full rounded-none sm:h-auto sm:max-h-[calc(100dvh-2rem)] sm:max-w-3xl sm:rounded-xl [&_button]:min-h-11 [&_button]:min-w-11'
      contentHeight='auto'
      bodyClassName='space-y-4 break-words'
      footerClassName='border-t pt-4'
      footer={
        <Button
          variant='outline'
          onClick={() => onOpenChange(false)}
          className='w-full sm:w-auto'
        >
          {t('Close')}
        </Button>
      }
    >
      <div className='space-y-3 py-2 sm:space-y-4'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <div className='text-muted-foreground text-sm'>
            {t('Deployment ID')}:{' '}
            <span className='font-mono break-all'>{deploymentId}</span>
          </div>
          <div className='grid grid-cols-2 gap-2 sm:flex sm:items-center'>
            <Button variant='outline' size='sm' onClick={handleCopyId}>
              <HugeiconsIcon icon={Copy01Icon} aria-hidden='true' className='mr-2 h-4 w-4' />
              {t('Copy')}
            </Button>
            <Button
              variant='outline'
              size='sm'
              onClick={handleRefresh}
              disabled={isFetchingDetails || isFetchingContainers}
            >
              {isFetchingDetails || isFetchingContainers ? (
                <HugeiconsIcon icon={Loading03Icon} aria-hidden='true' className='mr-2 h-4 w-4 animate-spin' />
              ) : (
                <HugeiconsIcon icon={Refresh01Icon} aria-hidden='true' className='mr-2 h-4 w-4' />
              )}
              {t('Refresh')}
            </Button>
          </div>
        </div>

        <Separator />

        {isDetailsLoading ? (
          <div className='flex items-center justify-center py-10'>
            <HugeiconsIcon icon={Loading03Icon} aria-hidden='true' className='text-muted-foreground h-6 w-6 animate-spin' />
          </div>
        ) : null}
        {showDetailsError ? (
          <div role='alert' className='text-destructive py-10 text-center text-sm'>
            {detailsError?.message || detailsRes?.message || t('Failed to fetch deployment details')}
          </div>
        ) : null}
        {showDetailsContent ? (
          <>
            {!containersKnown ? <div role='alert' className='text-destructive text-sm break-words'>{containersError?.message || containersRes?.message || t('Failed to load containers')}</div> : null}
            <div className='grid gap-3 sm:grid-cols-2'>
              <div className='rounded-lg border p-3'>
                <div className='text-muted-foreground text-xs'>
                  {t('Status')}
                </div>
                <div className='mt-1 font-medium'>
                  {String(details?.status ?? '-')}
                </div>
              </div>
              <div className='rounded-lg border p-3'>
                <div className='text-muted-foreground text-xs'>
                  {t('Hardware')}
                </div>
                <div className='mt-1 font-medium'>
                  {String(details?.brand_name ?? '')}{' '}
                  {String(details?.hardware_name ?? '')}
                </div>
              </div>
              <div className='rounded-lg border p-3'>
                <div className='text-muted-foreground text-xs'>
                  {t('Total GPUs')}
                </div>
                <div className='mt-1 font-medium'>
                  {String(details?.total_gpus ?? details?.hardware_qty ?? '-')}
                </div>
              </div>
              <div className='rounded-lg border p-3'>
                <div className='text-muted-foreground text-xs'>
                  {t('Containers')}
                </div>
                <div className='mt-1 font-medium'>{containersKnown ? containers.length : t('Unknown')}</div>
              </div>
            </div>

            {locations.length ? (
              <SummaryDisclosure key={`locations:${identity}:${deploymentId}`} summary={t('Locations ({{count}})', { count: locations.length })}>
                <div className='mt-1 flex flex-wrap gap-2 text-sm'>
                  {locations.map((x) => (
                    <span key={x} className='bg-muted rounded-md px-2 py-1'>
                      {x}
                    </span>
                  ))}
                </div>
              </SummaryDisclosure>
            ) : null}

            {containersKnown && containers.length > 0 && <div className='space-y-1 text-sm'>
              <div className='text-muted-foreground'>{t('Container statuses')}</div>
              <ul className='flex flex-wrap gap-x-4 gap-y-1'>
                {containerStatusCounts.map(([status, count]) => <li key={status ?? ''} className='min-w-0 [overflow-wrap:anywhere]'>{status ?? t('Unknown')}: {count}</li>)}
              </ul>
            </div>}
            {containers.length ? (
              <SummaryDisclosure key={`containers:${identity}:${deploymentId}`} summary={t('Containers ({{count}})', { count: containers.length })}>
                <div className='space-y-2'>
                  {containers.map((c) => {
                    const id = c?.container_id
                    if (typeof id !== 'string' || !id) {
                      return null
                    }
                    const status =
                      typeof c?.status === 'string' ? c.status : undefined
                    const url =
                      typeof c?.public_url === 'string' ? c.public_url : ''
                    return (
                      <div
                        key={id}
                        className='flex flex-wrap items-center justify-between gap-2 rounded-md border px-3 py-2'
                      >
                        <div className='min-w-0'>
                          <div className='font-mono text-sm break-all'>{id}</div>
                          <div className='text-muted-foreground text-xs'>
                            {status ? `${t('Status')}: ${status}` : ''}
                          </div>
                        </div>
                        {url ? (
                          <Button
                            variant='outline'
                            size='sm'
                            onClick={() => window.open(url, '_blank')}
                          >
                            <HugeiconsIcon icon={LinkSquare02Icon} aria-hidden='true' className='mr-2 h-4 w-4' />
                            {t('Open')}
                          </Button>
                        ) : null}
                      </div>
                    )
                  })}
                </div>
              </SummaryDisclosure>
            ) : null}

            <Collapsible className='rounded-lg border p-3'>
              <CollapsibleTrigger className='cursor-pointer text-sm font-medium'>
                {t('Raw JSON')}
              </CollapsibleTrigger>
              <CollapsibleContent>
                <pre className='bg-muted text-foreground mt-3 rounded-md p-3 text-xs whitespace-pre-wrap break-all'>
                  {payloadJson || '-'}
                </pre>
              </CollapsibleContent>
            </Collapsible>
          </>
        ) : null}
      </div>
    </Dialog>
  )
}
