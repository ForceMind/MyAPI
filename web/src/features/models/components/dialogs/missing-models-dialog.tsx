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
import { PlusSignIcon, Search01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { LogPagination } from '@/components/log-pagination'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { useIsMobile } from '@/hooks/use-mobile'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getMissingModels } from '../../api'
import { modelsQueryKeys } from '../../lib'
import { findModelAction } from '../../model-action-focus'
import type { Model } from '../../types'
import { useModels } from '../models-provider'

type MissingModelsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  returnFocus?: () => HTMLElement | null
}

export function MissingModelsDialog(props: MissingModelsDialogProps) {
  const { t } = useTranslation()
  const {
    setOpen,
    setCurrentRow,
    missingView,
    setMissingView,
    missingReturnTarget,
    setMissingReturnTarget,
  } = useModels()
  const isMobile = useIsMobile()
  const identity = useAuthStore(
    (state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`
  )
  const isAdmin = useAuthStore(
    (state) => (state.auth.user?.role ?? 0) >= ROLE.ADMIN
  )
  const changingView = useRef(false)
  const restorePending = useRef(false)
  const query = useQuery({
    queryKey: [...modelsQueryKeys.missing(), identity],
    queryFn: async ({ signal }) => {
      const response = await getMissingModels(signal)
      if (!response.success) {
        throw new Error(response.message || t('Unable to load missing models'))
      }
      // The backend returns null when all referenced models are configured.
      if (response.data === null) return []
      if (!Array.isArray(response.data) || response.data.some((name) => typeof name !== 'string')) {
        throw new Error(t('Invalid missing model response'))
      }
      return response.data
    },
    enabled: props.open && isAdmin,
    staleTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const missingModels = query.data ?? []
  const keyword = missingView.search.trim().toLowerCase()
  const filteredModels = missingModels.filter((modelName) => !keyword || modelName.toLowerCase().includes(keyword))
  const totalPages = Math.max(1, Math.ceil(filteredModels.length / missingView.pageSize))
  const currentPage = query.data ? Math.min(missingView.page, totalPages) : missingView.page
  const startIndex = (currentPage - 1) * missingView.pageSize
  const pageModels = filteredModels.slice(startIndex, startIndex + missingView.pageSize)
  const hasResult = query.isSuccess && !query.isFetching

  useEffect(() => {
    if (!props.open || !restorePending.current || query.isFetching) return
    restorePending.current = false
    if (missingReturnTarget) findModelAction(missingReturnTarget)?.focus()
  }, [props.open, query.isFetching, query.data, missingReturnTarget])

  const handleConfigureModel = (modelName: string, index: number) => {
    if (!isAdmin) return
    setMissingReturnTarget({ kind: 'missing-configure', modelName, index })
    setCurrentRow({ model_name: modelName } as Model)
    changingView.current = true
    setOpen('create-model')
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent
        className='flex h-dvh max-h-dvh w-full max-w-none flex-col gap-0 overflow-hidden rounded-none p-0 sm:h-[min(85dvh,800px)] sm:max-h-[calc(100dvh-3rem)] sm:max-w-3xl sm:rounded-xl [&_[data-slot=dialog-close]]:min-h-11 [&_[data-slot=dialog-close]]:min-w-11'
        initialFocus={() => {
          changingView.current = false
          restorePending.current = missingReturnTarget !== null
          if (missingReturnTarget) return findModelAction(missingReturnTarget)
          if (isMobile) return false
          return document.querySelector<HTMLElement>('[data-model-missing-search]')
        }}
        finalFocus={() => changingView.current ? false : props.returnFocus?.() ?? false}
      >
        <DialogHeader className='shrink-0 border-b px-4 py-4 pr-16 sm:px-6 sm:pr-16'>
          <DialogTitle>{t('Missing Models')}</DialogTitle>
          <DialogDescription>{t('Models that are being used but not configured in the system')}</DialogDescription>
        </DialogHeader>
        <section aria-label={t('Missing model results')} className='flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto overscroll-contain px-4 py-4 sm:px-6'>
          <div className='relative'>
            <HugeiconsIcon icon={Search01Icon} aria-hidden='true' className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2' />
            <Input
              data-model-missing-search
              aria-label={t('Search missing models')}
              placeholder={t('Search models...')}
              className='min-h-11 pl-9'
              value={missingView.search}
              onChange={(event) => setMissingView((current) => ({ ...current, search: event.target.value, page: 1 }))}
            />
          </div>
          {query.isFetching && <p role='status' className='text-muted-foreground flex items-center justify-center gap-2 py-8'><Spinner aria-hidden='true' />{t('Loading missing models...')}</p>}
          {query.isError && <Alert variant='destructive'><AlertTitle>{t('Unable to load missing models')}</AlertTitle><AlertDescription>{query.error instanceof Error ? query.error.message : t('Please try again.')}</AlertDescription><Button className='mt-3 min-h-11 w-fit' variant='outline' disabled={query.isFetching} onClick={() => { void query.refetch() }}>{t('Retry')}</Button></Alert>}
          {hasResult && missingModels.length === 0 && <Empty className='border'><EmptyHeader><EmptyTitle>{t('No missing models found.')}</EmptyTitle><EmptyDescription>{t('All models in use are properly configured.')}</EmptyDescription></EmptyHeader></Empty>}
          {hasResult && missingModels.length > 0 && filteredModels.length === 0 && <Empty className='border'><EmptyHeader><EmptyMedia variant='icon'><HugeiconsIcon icon={Search01Icon} aria-hidden='true' className='size-5' /></EmptyMedia><EmptyTitle>{t('No matches found')}</EmptyTitle><EmptyDescription>{t('Try adjusting your search to locate a missing model.')}</EmptyDescription></EmptyHeader></Empty>}
          {hasResult && pageModels.length > 0 && <div className='divide-y rounded-lg border'>{pageModels.map((modelName, index) => <article key={modelName} aria-label={modelName} className='flex min-w-0 flex-col gap-3 p-4 sm:flex-row sm:items-start sm:justify-between'>
            <p className='min-w-0 flex-1 font-mono text-sm break-words [overflow-wrap:anywhere]'>{modelName}</p>
            <div className='flex shrink-0 flex-wrap items-center gap-2'>
              <CopyButton value={modelName} className='min-h-11 min-w-11' aria-label={t('Copy model name "{{name}}"', { name: modelName, interpolation: { escapeValue: false } })} />
              <Button className='min-h-11' data-model-missing-configure={modelName} disabled={!isAdmin} aria-label={t('Configure model "{{name}}"', { name: modelName, interpolation: { escapeValue: false } })} onClick={() => handleConfigureModel(modelName, index)}><HugeiconsIcon icon={PlusSignIcon} aria-hidden='true' className='size-4' />{t('Configure')}</Button>
            </div>
          </article>)}</div>}
        </section>
        <DialogFooter className='m-0 shrink-0 flex-col gap-3 rounded-none px-4 py-3 sm:flex-col sm:px-6'>
          <LogPagination label={t('Missing models pagination')} page={currentPage} pageSize={missingView.pageSize} total={hasResult ? filteredModels.length : undefined} busy={query.isFetching} onPageChange={(page) => setMissingView((current) => ({ ...current, page }))} onPageSizeChange={(pageSize) => setMissingView((current) => ({ ...current, pageSize, page: 1 }))} />
          <div className='flex justify-end'><Button variant='outline' className='min-h-11' onClick={() => props.onOpenChange(false)}>{t('Close')}</Button></div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
