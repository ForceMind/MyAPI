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
import { useQueryClient } from '@tanstack/react-query'
import { Refresh01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StatusBadge } from '@/components/status-badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Spinner } from '@/components/ui/spinner'
import { useIsMobile } from '@/hooks/use-mobile'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { syncUpstream, previewUpstreamDiff } from '../../api'
import { getSyncLocaleOptions, getSyncSourceOptions } from '../../constants'
import { modelsQueryKeys, vendorsQueryKeys } from '../../lib'
import type { SyncLocale, SyncSource } from '../../types'
import { useModels } from '../models-provider'

type SyncWizardDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  returnFocus?: () => HTMLElement | null
}

export function SyncWizardDialog(props: SyncWizardDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const {
    setOpen,
    setUpstreamConflicts,
    setSyncWizardOptions,
    syncWizardOptions,
  } = useModels()
  const isMobile = useIsMobile()
  const identity = useAuthStore(
    (state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`
  )
  const isAdmin = useAuthStore(
    (state) => (state.auth.user?.role ?? 0) >= ROLE.ADMIN
  )
  const [locale, setLocale] = useState<SyncLocale>('zh')
  const [source, setSource] = useState<SyncSource>('official')
  const [isSyncing, setIsSyncing] = useState(false)
  const [isWriting, setIsWriting] = useState(false)
  const [error, setError] = useState('')
  const generation = useRef(0)
  const request = useRef<AbortController | null>(null)
  const changingView = useRef(false)
  const sourceOptions = getSyncSourceOptions(t)
  const localeOptions = getSyncLocaleOptions(t)

  useEffect(() => {
    if (props.open) {
      setLocale(syncWizardOptions.locale || 'zh')
      setSource('official')
    }
  }, [props.open, syncWizardOptions.locale])

  useEffect(() => {
    setIsSyncing(false)
    setIsWriting(false)
    setError('')
    if (props.open) changingView.current = false
    return () => {
      generation.current += 1
      request.current?.abort()
      request.current = null
    }
  }, [props.open, identity])

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      generation.current += 1
      request.current?.abort()
      request.current = null
    }
    props.onOpenChange(nextOpen)
  }

  const isCurrentIdentity = () => {
    const auth = useAuthStore.getState().auth
    if ((auth.user?.role ?? 0) < ROLE.ADMIN) return false
    return `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}` === identity
  }

  const handleSync = async () => {
    if (!props.open || !isAdmin || !isCurrentIdentity() || request.current) return
    const token = ++generation.current
    const controller = new AbortController()
    request.current = controller
    setIsSyncing(true)
    setError('')
    try {
      setSyncWizardOptions({ locale, source })
      const preview = await previewUpstreamDiff(
        { locale, source }, controller.signal
      )
      if (token !== generation.current || controller.signal.aborted || !isCurrentIdentity()) return
      if (!preview.success) {
        throw new Error(preview.message || t('Failed to preview upstream diff'))
      }
      const conflicts = preview.data?.conflicts || []
      if (conflicts.length > 0) {
        setUpstreamConflicts(conflicts)
        changingView.current = true
        setOpen('upstream-conflict')
        return
      }
      if (!isCurrentIdentity()) return
      setIsWriting(true)
      const response = await syncUpstream({ locale, source }, controller.signal)
      if (token !== generation.current || controller.signal.aborted || !isCurrentIdentity()) return
      if (!response.success) {
        throw new Error(response.message || t('Sync failed'))
      }
      const counts = response.data
      if (counts && [
        counts.created_models,
        counts.updated_models,
        counts.created_vendors,
      ].every((count) => typeof count === 'number' && Number.isSafeInteger(count) && count >= 0)) {
        toast.success(t('Sync completed: {{models}} models created, {{updated}} updated, {{vendors}} vendors added.', {
          models: counts.created_models,
          updated: counts.updated_models,
          vendors: counts.created_vendors,
        }))
      } else {
        toast.success(t('Sync completed successfully.'))
      }
      void queryClient.invalidateQueries({ queryKey: modelsQueryKeys.lists() })
      void queryClient.invalidateQueries({ queryKey: vendorsQueryKeys.lists() })
      handleOpenChange(false)
    } catch (cause: unknown) {
      if (token !== generation.current || controller.signal.aborted || !isCurrentIdentity()) return
      setError(cause instanceof Error ? cause.message : t('Sync failed'))
    } finally {
      if (token === generation.current && isCurrentIdentity()) {
        request.current = null
        setIsSyncing(false)
        setIsWriting(false)
      }
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={handleOpenChange}>
      <DialogContent
        className='flex h-dvh max-h-dvh w-full max-w-none flex-col gap-0 overflow-hidden rounded-none p-0 sm:h-auto sm:max-h-[calc(100dvh-3rem)] sm:max-w-2xl sm:rounded-xl [&_[data-slot=dialog-close]]:min-h-11 [&_[data-slot=dialog-close]]:min-w-11'
        initialFocus={!isMobile}
        finalFocus={() => changingView.current ? false : props.returnFocus?.() ?? false}
      >
        <DialogHeader className='shrink-0 border-b px-4 py-4 pr-16 sm:px-6 sm:pr-16'>
          <DialogTitle>{t('Sync Upstream Models')}</DialogTitle>
          <DialogDescription>{t('Synchronize models and vendors from an upstream source')}</DialogDescription>
        </DialogHeader>
        <section aria-label={t('Sync options')} className='flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto overscroll-contain px-4 py-4 sm:px-6'>
          {error && <Alert variant='destructive'><AlertDescription>{error}</AlertDescription></Alert>}
          <fieldset className='min-w-0' disabled={isSyncing || !isAdmin}>
            <legend className='mb-3 text-sm font-medium'>{t('Select Sync Source')}</legend>
            <RadioGroup aria-label={t('Select Sync Source')} value={source} onValueChange={(value) => {
              const selected = sourceOptions.find((option) => option.value === value)
              if (selected && !selected.disabled) setSource(selected.value)
            }} className='grid gap-3 sm:grid-cols-2'>
              {sourceOptions.map((option) => (
                <Label key={option.value} htmlFor={`sync-source-${option.value}`} className={cn('min-h-11 min-w-0 items-start gap-3 rounded-lg border p-4 font-normal', source === option.value && 'border-primary ring-primary ring-1', option.disabled ? 'cursor-not-allowed opacity-60' : 'cursor-pointer')}>
                  <RadioGroupItem value={option.value} id={`sync-source-${option.value}`} disabled={option.disabled || isSyncing || !isAdmin} />
                  <span className='flex min-w-0 flex-col gap-1'>
                    <span className='flex flex-wrap items-center gap-2'><span className='font-medium'>{option.label}</span>{option.value === 'official' && <StatusBadge label={t('Default')} variant='neutral' copyable={false} />}</span>
                    <span className='text-muted-foreground text-sm'>{option.description}</span>
                  </span>
                </Label>
              ))}
            </RadioGroup>
          </fieldset>
          <fieldset className='min-w-0' disabled={isSyncing || !isAdmin}>
            <legend className='mb-3 text-sm font-medium'>{t('Select Language')}</legend>
            <RadioGroup aria-label={t('Select Language')} value={locale} onValueChange={(value) => setLocale(value as SyncLocale)} className='grid gap-3 sm:grid-cols-3'>
              {localeOptions.map((option) => <Label key={option.value} htmlFor={`locale-${option.value}`} className={cn('min-h-11 cursor-pointer gap-3 rounded-lg border p-3 font-normal', locale === option.value && 'border-primary ring-primary ring-1')}><RadioGroupItem value={option.value} id={`locale-${option.value}`} disabled={isSyncing || !isAdmin} />{option.label}</Label>)}
            </RadioGroup>
          </fieldset>
          <p className='text-muted-foreground rounded-lg border p-4 text-sm'>{t('The sync will fetch missing models and vendors from the selected source. Existing records are updated only when you approve conflicts.')}</p>
          {isWriting && <p role='status' className='text-muted-foreground text-sm'>{t('Closing this dialog does not undo a sync already submitted.')}</p>}
        </section>
        <DialogFooter className='m-0 shrink-0 flex-row flex-wrap justify-end rounded-none px-4 py-3 sm:px-6'>
          <Button variant='outline' className='min-h-11' onClick={() => handleOpenChange(false)}>{t('Cancel')}</Button>
          <Button className='min-h-11' onClick={() => { void handleSync() }} disabled={isSyncing || !isAdmin}>{isSyncing ? <Spinner aria-hidden='true' className='size-4' /> : <HugeiconsIcon icon={Refresh01Icon} aria-hidden='true' className='size-4' />}{isSyncing ? t('Syncing...') : t('Sync Now')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
