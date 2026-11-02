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
import { ArrowDown01Icon, CodeIcon, Loading03Icon, Refresh01Icon } from '@hugeicons/core-free-icons'
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
import { type ReactNode, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { IconBadge } from '@/components/ui/icon-badge'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import { getDeploymentLogs, listDeploymentContainers } from '../../api'
import { useDeploymentTask } from '../../hooks/use-deployment-task'

interface ViewLogsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  deploymentId: string | number | null
  returnFocus?: () => HTMLElement | null
}

export function ViewLogsDialog({
  open,
  onOpenChange,
  deploymentId,
  returnFocus,
}: ViewLogsDialogProps) {
  const { t } = useTranslation()
  const { identity } = useDeploymentTask(open, deploymentId)
  const scrollRef = useRef<HTMLDivElement>(null)
  const [autoScroll, setAutoScroll] = useState(true)
  const [autoRefresh, setAutoRefresh] = useState(false)
  const [stream, setStream] = useState<'stdout' | 'stderr' | 'all'>('stdout')
  const [containerId, setContainerId] = useState<string>('')

  const {
    data: containersData,
    isLoading: isLoadingContainers,
    refetch: refetchContainers,
    isFetching: isFetchingContainers,
    isError: isContainersError,
    error: containersError,
  } = useQuery({
    queryKey: ['deployment-containers', deploymentId, identity],
    queryFn: () =>
      deploymentId ? listDeploymentContainers(deploymentId) : null,
    enabled: open && deploymentId !== null,
  })

  const containers = useMemo(() => {
    const items = containersData?.data?.containers
    return !isContainersError && containersData?.success && Array.isArray(items) ? items : []
  }, [containersData?.success, containersData?.data?.containers, isContainersError])

  useEffect(() => {
    if (!open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setContainerId('')

      setStream('stdout')

      setAutoScroll(true)

      setAutoRefresh(false)
      return
    }

    if (open && containers.length > 0 && !containerId) {
      const first = containers[0]?.container_id
      if (typeof first === 'string' && first) {
        setContainerId(first)
      }
    }
  }, [open, containers, containerId])

  const {
    data: logsData,
    isLoading: isLoadingLogs,
    refetch: refetchLogs,
    isFetching: isFetchingLogs,
    isError: isLogsError,
    error: logsError,
  } = useQuery({
    queryKey: ['deployment-logs', deploymentId, containerId, stream, identity],
    queryFn: () =>
      deploymentId && containerId
        ? getDeploymentLogs(deploymentId, {
            container_id: containerId,
            stream,
            limit: 500,
          })
        : null,
    enabled: open && deploymentId !== null && Boolean(containerId),
    refetchInterval: open && autoRefresh ? 5000 : false,
  })

  const logsText = useMemo(() => {
    const raw = logsData?.data
    return !isLogsError && logsData?.success && typeof raw === 'string' ? raw : ''
  }, [logsData?.data, logsData?.success, isLogsError])

  const logLines = useMemo(() => {
    const normalized = logsText.replaceAll(/\r\n?/g, '\n')
    return normalized ? normalized.split('\n') : []
  }, [logsText])
  const keyedLogLines = useMemo(() => {
    const seen = new Map<string, number>()
    return logLines.map((line) => {
      const count = seen.get(line) ?? 0
      seen.set(line, count + 1)
      return { key: `${line}-${count}`, line }
    })
  }, [logLines])

  // Auto-scroll to bottom
  useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [logLines, autoScroll])

  const handleDownload = () => {
    if (!logsText.trim()) return
    const blob = new Blob([logsText], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `deployment-${deploymentId}-${containerId || 'logs'}.txt`
    a.click()
    URL.revokeObjectURL(url)
  }
  let containerPlaceholder = t('Select')
  if (isLoadingContainers) {
    containerPlaceholder = t('Loading...')
  } else if (containers.length === 0) {
    containerPlaceholder = t('No containers')
  }
  let logsContent: ReactNode
  if (isLoadingContainers || isLoadingLogs) {
    logsContent = (
      <div className='flex items-center justify-center py-8'>
        <HugeiconsIcon icon={Loading03Icon} aria-hidden='true' className='text-muted-foreground h-6 w-6 animate-spin' />
      </div>
    )
  } else if (isContainersError || containersData?.success === false) {
    logsContent = <div role='alert' className='text-destructive py-8 text-center break-words'>{containersError?.message || containersData?.message || t('Failed to load containers')}</div>
  } else if (isLogsError || logsData?.success === false) {
    logsContent = <div role='alert' className='text-destructive py-8 text-center break-words'>{logsError?.message || logsData?.message || t('Failed to fetch deployment logs')}</div>
  } else if (containers.length === 0) {
    logsContent = (
      <div className='text-muted-foreground py-8 text-center'>{t('No containers')}</div>
    )
  } else if (!containerId) {
    logsContent = (
      <div className='text-muted-foreground py-8 text-center'>
        {t('Please select a container')}
      </div>
    )
  } else if (!logsText.trim()) {
    logsContent = (
      <div className='text-muted-foreground py-8 text-center'>{t('No logs')}</div>
    )
  } else {
    logsContent = (
      <div className='font-mono text-sm'>
        {keyedLogLines.map(({ key, line }) => (
          <div key={key} className='text-foreground whitespace-pre-wrap break-all'>
            {line}
          </div>
        ))}
      </div>
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent finalFocus={returnFocus} className='flex h-dvh max-h-dvh w-screen max-w-none sm:w-full flex-col gap-4 overflow-hidden rounded-none p-4 sm:h-[80dvh] sm:max-h-[calc(100dvh-2rem)] sm:max-w-4xl sm:rounded-xl sm:p-6 [&_button]:min-h-11 [&_button]:min-w-11'>
        <DialogHeader className='shrink-0 text-start'>
          <DialogTitle className='flex items-center gap-2'>
          <IconBadge tone='chart-3' size='sm'>
            <HugeiconsIcon icon={CodeIcon} aria-hidden='true' />
          </IconBadge>
          {t('Deployment logs')}
          </DialogTitle>
          <div className='text-muted-foreground text-sm break-all'>{t('Deployment ID')}: {deploymentId}</div>
        </DialogHeader>
      <div ref={scrollRef} className='min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain'
        onScroll={(e) => {
          const target = e.currentTarget
          const isAtBottom = target.scrollHeight - target.scrollTop - target.clientHeight < 50
          setAutoScroll(isAtBottom)
        }}>
      <div className='mb-3 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-3'>
        <div className='grid grid-cols-2 gap-2 sm:flex sm:flex-wrap sm:items-center'>
          <Button
            variant='outline'
            size='sm'
            onClick={() => {
              refetchContainers()
              refetchLogs()
            }}
            disabled={isFetchingLogs || isFetchingContainers}
          >
            {isFetchingLogs || isFetchingContainers ? (
              <HugeiconsIcon icon={Loading03Icon} aria-hidden='true' className='mr-2 h-4 w-4 animate-spin' />
            ) : (
              <HugeiconsIcon icon={Refresh01Icon} aria-hidden='true' className='mr-2 h-4 w-4' />
            )}
            {t('Refresh')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            onClick={handleDownload}
            disabled={!logsText.trim()}
          >
            <HugeiconsIcon icon={ArrowDown01Icon} aria-hidden='true' className='mr-2 h-4 w-4' />
            {t('Download')}
          </Button>
          <div className='col-span-2 flex items-center justify-between gap-2 rounded-md border px-3 py-1.5 sm:col-span-1'>
            <span className='text-xs'>{t('Auto refresh')}</span>
            <Switch aria-label={t('Auto refresh')} checked={autoRefresh} onCheckedChange={setAutoRefresh} />
          </div>
        </div>
      </div>
      <div className='mb-3 grid gap-2 sm:grid-cols-2 sm:gap-3'>
        <div className='space-y-1'>
          <div className='text-muted-foreground text-xs'>{t('Container')}</div>
          <Select
            items={containers.flatMap((c) => {
              const id = c?.container_id
              if (typeof id !== 'string' || !id) return []
              const status =
                typeof c?.status === 'string' && c.status
                  ? ` (${c.status})`
                  : ''
              return [
                {
                  value: id,
                  label: (
                    <>
                      {id}
                      {status}
                    </>
                  ),
                },
              ]
            })}
            value={containerId}
            onValueChange={(v) => v !== null && setContainerId(v)}
            disabled={isLoadingContainers || containers.length === 0}
          >
            <SelectTrigger aria-label={t('Container')} className='min-h-11 w-full'>
              <SelectValue placeholder={containerPlaceholder} />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {containers.map((c) => {
                  const id = c?.container_id
                  if (typeof id !== 'string' || !id) {
                    return null
                  }
                  const status =
                    typeof c?.status === 'string' && c.status
                      ? ` (${c.status})`
                      : ''
                  return (
                    <SelectItem key={id} value={id}>
                      {id}
                      {status}
                    </SelectItem>
                  )
                })}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
        <div className='space-y-1'>
          <div className='text-muted-foreground text-xs'>{t('Stream')}</div>
          <Select
            items={[
              { value: 'stdout', label: 'stdout' },
              { value: 'stderr', label: 'stderr' },
              { value: 'all', label: 'all' },
            ]}
            value={stream}
            onValueChange={(v) => {
              if (v === 'stderr' || v === 'all' || v === 'stdout') {
                setStream(v)
              } else {
                setStream('stdout')
              }
            }}
          >
            <SelectTrigger aria-label={t('Stream')} className='min-h-11 w-full'>
              <SelectValue placeholder={t('Select')} />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectItem value='stdout'>stdout</SelectItem>
                <SelectItem value='stderr'>stderr</SelectItem>
                <SelectItem value='all'>all</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
      </div>
      <div className='bg-muted min-w-0 rounded-md border p-3 sm:p-4'>
        {logsContent}
      </div>
      </div>
      <DialogFooter className='shrink-0 border-t pt-4'>
        <Button variant='outline' onClick={() => onOpenChange(false)}>{t('Close')}</Button>
      </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
