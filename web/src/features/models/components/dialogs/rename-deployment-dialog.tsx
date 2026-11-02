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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Loading03Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { checkClusterNameAvailability, updateDeploymentName } from '../../api'
import { deploymentsQueryKeys } from '../../lib'
import { useDeploymentTask } from '../../hooks/use-deployment-task'

export function RenameDeploymentDialog({
  open,
  onOpenChange,
  deploymentId,
  returnFocus,
  currentName,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  deploymentId: string | number | null
  returnFocus?: () => HTMLElement | null
  currentName?: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const task = useDeploymentTask(open, deploymentId)
  const initialized = useRef<string | null>(null)
  const taskKey = JSON.stringify([task.identity, deploymentId])
  const [name, setName] = useState(currentName || '')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  useEffect(() => {
    if (!open) {
      initialized.current = null
      return
    }
    if (initialized.current === taskKey) return
    initialized.current = taskKey
    setName(currentName || '')
    setIsSubmitting(false)
    setSubmitError(null)
  }, [open, taskKey, currentName])

  const trimmed = name.trim()

  const { data: checkRes, isFetching: isChecking } = useQuery({
    queryKey: ['deployment-rename-check', trimmed, task.identity],
    queryFn: () => (trimmed ? checkClusterNameAvailability(trimmed) : null),
    enabled: open && Boolean(trimmed),
    staleTime: 10_000,
  })

  const available =
    checkRes?.success === true ? checkRes?.data?.available : undefined

  const helper = useMemo(() => {
    if (!trimmed) return t('Enter a new name')
    if (isChecking) return t('Checking name...')
    if (available === true) return t('Name is available')
    if (available === false) return t('Name is not available')
    return ''
  }, [available, isChecking, t, trimmed])

  const canSubmit =
    Boolean(deploymentId) &&
    Boolean(trimmed) &&
    available !== false &&
    !isSubmitting

  const onSubmit = async () => {
    if (!deploymentId) return
    if (!trimmed) {
      toast.error(t('Please enter a name'))
      return
    }
    if (available === false) {
      toast.error(t('Name is not available'))
      return
    }

    const operation = task.begin()
    if (!operation) return

    setSubmitError(null)
    setIsSubmitting(true)
    try {
      const res = await updateDeploymentName(deploymentId, trimmed)
      if (!operation.isCurrent()) return
      if (res.success) {
        toast.success(t('Renamed successfully'))
        queryClient.invalidateQueries({
          queryKey: deploymentsQueryKeys.lists(),
        })
        queryClient.invalidateQueries({ queryKey: ['deployment-details'] })
        onOpenChange(false)
        return
      }
      setSubmitError(res.message || t('Rename failed'))
      toast.error(res.message || t('Rename failed'))
    } catch (err: unknown) {
      if (!operation.isCurrent()) return
      setSubmitError(err instanceof Error ? err.message : t('Rename failed'))
      toast.error(err instanceof Error ? err.message : t('Rename failed'))
    } finally {
      if (operation.isCurrent()) setIsSubmitting(false)
      operation.finish()
    }
  }

  return (
    <Dialog
      open={open}
      returnFocus={returnFocus}
      onOpenChange={(next) => { if (!task.isPending()) onOpenChange(next) }}
      showCloseButton={!isSubmitting}
      title={t('Rename deployment')}
      contentClassName='h-dvh max-h-dvh w-screen max-w-none sm:w-full rounded-none sm:h-auto sm:max-h-[calc(100dvh-2rem)] sm:max-w-lg sm:rounded-xl [&_button]:min-h-11 [&_button]:min-w-11 [&_input]:min-h-11'
      footerClassName='border-t pt-4'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' disabled={isSubmitting} onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={() => void onSubmit()} disabled={!canSubmit}>
            {isSubmitting ? (
              <HugeiconsIcon icon={Loading03Icon} aria-hidden='true' className='mr-2 h-4 w-4 animate-spin' />
            ) : null}
            {t('Rename')}
          </Button>
        </>
      }
    >
      {submitError ? <div role='alert' className='text-destructive text-sm break-words'>{submitError}</div> : null}
      <div className='space-y-2'>
        <div className='text-muted-foreground text-sm'>
          {t('Deployment ID')}:{' '}
          <span className='font-mono break-all'>{deploymentId}</span>
        </div>
        <Label htmlFor='deployment-rename-name'>{t('Name')}</Label>
        <Input
          id='deployment-rename-name'
          placeholder={t('Enter a new name')}
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoComplete='off'
          disabled={isSubmitting}
        />
        <div className='text-muted-foreground text-xs'>{helper}</div>
      </div>
    </Dialog>
  )
}
