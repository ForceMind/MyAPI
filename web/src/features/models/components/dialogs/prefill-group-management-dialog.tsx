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
import { FilterHorizontalIcon, PlusSignIcon, Refresh01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { useIsMobile } from '@/hooks/use-mobile'

import { deletePrefillGroup, getPrefillGroups } from '../../api'
import { prefillGroupsQueryKeys } from '../../lib'
import type { PrefillGroup } from '../../types'
import { PrefillGroupActions, PrefillGroupSummary } from '../prefill-group-summary'

type PrefillGroupManagementDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreateGroup: () => void
  onEditGroup: (group: PrefillGroup) => void
  returnFocus?: () => HTMLElement | null
}

export function PrefillGroupManagementDialog(props: PrefillGroupManagementDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isMobile = useIsMobile()
  const [search, setSearch] = useState('')
  const [typeFilter, setTypeFilter] = useState('all')
  const [page, setPage] = useState(1)
  const lastAction = useRef<number | 'create' | null>(null)
  const changingView = useRef(false)
  const [deleteGroup, setDeleteGroup] = useState<PrefillGroup | null>(null)
  const [deleteError, setDeleteError] = useState('')
  const [isDeleting, setIsDeleting] = useState(false)
  const deleting = useRef(false)
  const { data, isLoading, isFetching, error, refetch } = useQuery({
    queryKey: prefillGroupsQueryKeys.list(),
    queryFn: () => getPrefillGroups(),
    enabled: props.open,
  })
  let loadError = ''
  if (error) {
    loadError = error instanceof Error ? error.message : t('Unable to load groups')
  } else if (data?.success === false) {
    loadError = data.message || t('Unable to load groups')
  }
  const groups = useMemo(() => {
    const query = search.trim().toLocaleLowerCase()
    return [...(data?.success === false ? [] : data?.data ?? [])].filter((group) =>
      (typeFilter === 'all' || group.type === typeFilter) &&
      (!query || `${group.name} ${group.description ?? ''} ${group.id}`.toLocaleLowerCase().includes(query))
    ).sort((a, b) => a.type.localeCompare(b.type) || a.name.localeCompare(b.name))
  }, [data?.data, data?.success, search, typeFilter])
  const pageCount = Math.max(1, Math.ceil(groups.length / 10))
  const currentPage = Math.min(page, pageCount)
  const pageGroups = groups.slice((currentPage - 1) * 10, currentPage * 10)

  const editGroup = (group: PrefillGroup) => {
    lastAction.current = group.id
    changingView.current = true
    props.onEditGroup(group)
  }
  const requestDelete = (group: PrefillGroup) => {
    setDeleteError('')
    setDeleteGroup(group)
  }
  const confirmDelete = async () => {
    if (!deleteGroup || deleting.current) return
    const target = deleteGroup
    deleting.current = true
    setIsDeleting(true)
    setDeleteError('')
    try {
      const response = await deletePrefillGroup(target.id)
      if (!response.success) {
        setDeleteError(response.message || t('Failed to delete group'))
        return
      }
      toast.success(t('Deleted "{{name}}"', { name: target.name }))
      void queryClient.invalidateQueries({ queryKey: prefillGroupsQueryKeys.lists() })
      setDeleteGroup(null)
    } catch (cause: unknown) {
      setDeleteError(cause instanceof Error ? cause.message : t('Failed to delete group'))
    } finally {
      deleting.current = false
      setIsDeleting(false)
    }
  }

  return (
    <>
      <Dialog open={props.open} onOpenChange={props.onOpenChange}>
        <DialogContent
          className='flex h-dvh max-h-dvh w-full max-w-none flex-col gap-0 overflow-hidden rounded-none p-0 sm:h-auto sm:max-h-[calc(100dvh-3rem)] sm:max-w-4xl sm:rounded-xl [&_[data-slot=dialog-close]]:min-h-11 [&_[data-slot=dialog-close]]:min-w-11'
          initialFocus={() => {
            changingView.current = false
            if (lastAction.current === null) return document.querySelector<HTMLElement>('[data-prefill-search]')
            const selector = lastAction.current === 'create' ? '[data-prefill-create]' : `[data-prefill-edit="${lastAction.current}"]`
            return document.querySelector<HTMLElement>(selector) ?? document.querySelector<HTMLElement>('[data-prefill-search]')
          }}
          finalFocus={() => changingView.current ? false : props.returnFocus?.() ?? false}
        >
          <DialogHeader className='shrink-0 border-b px-4 py-4 pr-16 sm:px-6 sm:pr-16'>
            <DialogTitle className='flex items-center gap-2'><HugeiconsIcon icon={FilterHorizontalIcon} aria-hidden='true' className='size-5 shrink-0' />{t('Prefill Group Management')}</DialogTitle>
            <DialogDescription>{t('Manage reusable model, tag, and endpoint selections.')}</DialogDescription>
          </DialogHeader>
          <section className='min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4 sm:px-6' aria-label={t('Prefill groups')}>
            <div className='grid gap-3 sm:grid-cols-[1fr_auto_auto]'>
              <Input data-prefill-search className='min-h-11' aria-label={t('Search prefill groups')} placeholder={t('Search by name, description, or ID')} value={search} onChange={(event) => { setSearch(event.target.value); setPage(1) }} />
              <select className='border-input bg-background min-h-11 rounded-md border px-3 text-sm' aria-label={t('Group Type')} value={typeFilter} onChange={(event) => { setTypeFilter(event.target.value); setPage(1) }}>
                <option value='all'>{t('All types')}</option><option value='model'>{t('Model Group')}</option><option value='tag'>{t('Tag Group')}</option><option value='endpoint'>{t('Endpoint Group')}</option>
              </select>
              <Button className='min-h-11' variant='outline' disabled={isFetching} onClick={() => { void refetch() }}><HugeiconsIcon icon={Refresh01Icon} aria-hidden='true' className={isFetching ? 'size-4 animate-spin' : 'size-4'} />{t('Refresh')}</Button>
            </div>
            {loadError && <Alert variant='destructive'><AlertTitle>{t('Unable to load groups')}</AlertTitle><AlertDescription>{loadError}</AlertDescription></Alert>}
            {isLoading && <p role='status' className='text-muted-foreground flex items-center justify-center gap-2 py-12'><Spinner aria-hidden='true' className='size-4 animate-spin' />{t('Fetching prefill groups...')}</p>}
            {!isLoading && !loadError && groups.length === 0 && <div className='rounded-lg border border-dashed p-8 text-center'><h3 className='font-medium'>{search || typeFilter !== 'all' ? t('No matching prefill groups') : t('No prefill groups yet')}</h3><p className='text-muted-foreground mt-2 text-sm'>{t('Create reusable selections of models, tags, or endpoints.')}</p></div>}
            {pageGroups.length > 0 && (isMobile ? <div className='space-y-3'>{pageGroups.map((group) => <article key={group.id} className='min-w-0 space-y-3 rounded-xl border p-4' aria-label={group.name}><PrefillGroupSummary group={group} /><PrefillGroupActions group={group} onEdit={editGroup} onDelete={requestDelete} /></article>)}</div> :
              <StaticDataTable tableClassName='w-full table-fixed' data={pageGroups} getRowKey={(group) => group.id} columns={[
                { id: 'group', header: t('Group'), className: 'w-[35%]', cellClassName: 'align-top whitespace-normal', cell: (group) => <PrefillGroupSummary group={group} section='identity' /> },
                { id: 'items', header: t('Items'), cellClassName: 'align-top whitespace-normal', cell: (group) => <PrefillGroupSummary group={group} section='items' /> },
                { id: 'actions', header: t('Actions'), className: 'w-44', cellClassName: 'align-top whitespace-normal', cell: (group) => <PrefillGroupActions group={group} onEdit={editGroup} onDelete={requestDelete} /> },
              ]} />)}
          </section>
          <DialogFooter className='m-0 shrink-0 flex-row flex-wrap items-center justify-between rounded-none px-4 py-3 sm:justify-between sm:px-6'>
            <nav className='flex flex-wrap items-center gap-2' aria-label={t('Prefill groups pagination')}>
              <Button className='min-h-11' variant='outline' disabled={currentPage === 1} onClick={() => setPage(currentPage - 1)}>{t('Previous')}</Button>
              <span className='text-muted-foreground text-sm' aria-live='polite'>{t('Page {{page}} of {{total}}', { page: currentPage, total: pageCount })}</span>
              <Button className='min-h-11' variant='outline' disabled={currentPage === pageCount} onClick={() => setPage(currentPage + 1)}>{t('Next')}</Button>
            </nav>
            <div className='flex flex-wrap gap-2'>
              <Button className='min-h-11' variant='outline' onClick={() => props.onOpenChange(false)}>{t('Close')}</Button>
              <Button className='min-h-11' data-prefill-create onClick={() => { lastAction.current = 'create'; changingView.current = true; props.onCreateGroup() }}><HugeiconsIcon icon={PlusSignIcon} aria-hidden='true' className='size-4' />{t('New Group')}</Button>
            </div>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog open={deleteGroup !== null} onOpenChange={(next) => { if (!next && !deleting.current) { setDeleteGroup(null); setDeleteError('') } }} title={t('Delete group')} desc={<div className='space-y-3 break-words [overflow-wrap:anywhere]'><p>{t('Are you sure you want to delete group "{{name}}"? This action cannot be undone.', { name: deleteGroup?.name ?? '' })}</p><p className='text-muted-foreground font-mono'>#{deleteGroup?.id}</p>{deleteError && <p role='alert' className='text-destructive'>{deleteError}</p>}</div>} destructive isLoading={isDeleting} confirmText={isDeleting ? t('Deleting...') : t('Delete')} handleConfirm={() => { void confirmDelete() }} className='max-h-[calc(100dvh-2rem)] overflow-y-auto [&_button]:min-h-11 [&_button]:min-w-11' />
    </>
  )
}
