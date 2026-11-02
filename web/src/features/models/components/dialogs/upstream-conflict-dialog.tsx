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
import { ArrowLeft01Icon, ArrowRight01Icon, Search01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StatusBadge } from '@/components/status-badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { useDebounce } from '@/hooks/use-debounce'
import { useIsMobile } from '@/hooks/use-mobile'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { applyUpstreamOverwrite } from '../../api'
import { modelsQueryKeys, vendorsQueryKeys } from '../../lib'
import type { SyncOverwritePayload } from '../../types'
import { useModels } from '../models-provider'

const FIELD_LABELS: Record<string, string> = {
  description: 'Description',
  icon: 'Icon',
  tags: 'Tags',
  vendor: 'Vendor',
  name_rule: 'Name Rule',
  status: 'Status',
  endpoints: 'Endpoints',
  quota_types: 'Quota Types',
  enable_groups: 'Enable Groups',
}
const PAGE_SIZE_OPTIONS = [5, 10, 20, 50] as const

function ValuePreview({ value }: { value: unknown }) {
  let display = '—'
  if (typeof value === 'string') display = value || '—'
  else if (typeof value === 'number' || typeof value === 'boolean') display = String(value)
  else if (value !== undefined && value !== null) {
    try {
      display = JSON.stringify(value, null, 2) ?? '—'
    } catch {
      display = String(value)
    }
  }
  return (
    <pre className='bg-muted/50 text-muted-foreground min-w-0 rounded-md border p-3 font-mono text-xs break-words whitespace-pre-wrap [overflow-wrap:anywhere]'>
      {display}
    </pre>
  )
}

type UpstreamConflictDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  returnFocus?: () => HTMLElement | null
}

export function UpstreamConflictDialog(props: UpstreamConflictDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { upstreamConflicts, setUpstreamConflicts, syncWizardOptions } = useModels()
  const isMobile = useIsMobile()
  const identity = useAuthStore(
    (state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`
  )
  const isAdmin = useAuthStore(
    (state) => (state.auth.user?.role ?? 0) >= ROLE.ADMIN
  )
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebounce(search, 200)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [pageSize, setPageSize] = useState(10)
  const [pageIndex, setPageIndex] = useState(0)
  const [error, setError] = useState('')
  const generation = useRef(0)
  const request = useRef<AbortController | null>(null)
  const allFieldsId = useId()

  useEffect(() => {
    if (props.open) {
      setSelected(new Set())
      setSearch('')
      setPageIndex(0)
      setError('')
    }
  }, [props.open, upstreamConflicts])

  useEffect(() => {
    setIsSubmitting(false)
    return () => {
      generation.current += 1
      request.current?.abort()
      request.current = null
    }
  }, [props.open, identity])

  const rows = useMemo(() => (upstreamConflicts ?? []).flatMap((conflict) => (conflict.fields ?? []).map((field) => ({
    id: JSON.stringify([conflict.model_name, field.field]),
    modelName: conflict.model_name,
    fieldKey: field.field,
    localValue: field.local,
    upstreamValue: field.upstream,
  }))), [upstreamConflicts])

  const normalizedSearch = debouncedSearch.trim().toLowerCase()
  // A field match keeps its model's complete set of differences in view.
  const matchingModels = useMemo(() => new Set((upstreamConflicts ?? []).filter((conflict) =>
    !normalizedSearch || conflict.model_name.toLowerCase().includes(normalizedSearch) || conflict.fields?.some((field) =>
      field.field.toLowerCase().includes(normalizedSearch) || (FIELD_LABELS[field.field] ?? field.field).toLowerCase().includes(normalizedSearch)
    )
  ).map((conflict) => conflict.model_name)), [upstreamConflicts, normalizedSearch])
  const filteredRows = rows.filter((row) => matchingModels.has(row.modelName))
  const totalPages = Math.max(1, Math.ceil(filteredRows.length / pageSize))
  const currentPage = Math.min(pageIndex, totalPages - 1)
  const pageRows = filteredRows.slice(currentPage * pageSize, (currentPage + 1) * pageSize)
  const selectedRows = rows.filter((row) => selected.has(row.id))
  const selectedOutsidePage = selectedRows.filter((row) => !pageRows.some((pageRow) => pageRow.id === row.id)).length
  const allSelected = rows.length > 0 && selectedRows.length === rows.length
  const displayStart = filteredRows.length ? currentPage * pageSize + 1 : 0
  const displayEnd = Math.min((currentPage + 1) * pageSize, filteredRows.length)

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      generation.current += 1
      request.current?.abort()
      request.current = null
      setUpstreamConflicts([])
    }
    props.onOpenChange(nextOpen)
  }

  const toggleField = (id: string, checked: boolean) => {
    setSelected((current) => {
      const next = new Set(current)
      if (checked) next.add(id)
      else next.delete(id)
      return next
    })
  }

  const handleApplyOverwrite = async () => {
    if (!props.open || !isAdmin || request.current) return
    const grouped = new Map<string, Set<string>>()
    for (const row of selectedRows) {
      const fields = grouped.get(row.modelName) ?? new Set<string>()
      fields.add(row.fieldKey)
      grouped.set(row.modelName, fields)
    }
    const overwrite: SyncOverwritePayload[] = [...grouped].map(
      ([modelName, fields]) => ({ model_name: modelName, fields: [...fields] })
    )
    if (!overwrite.length) {
      setError(t('Select at least one field to overwrite.'))
      return
    }
    const token = ++generation.current
    const controller = new AbortController()
    request.current = controller
    setIsSubmitting(true)
    setError('')
    try {
      const response = await applyUpstreamOverwrite({
        overwrite,
        locale: syncWizardOptions.locale,
        source: syncWizardOptions.source,
      }, controller.signal)
      if (token !== generation.current || controller.signal.aborted) return
      if (!response.success) throw new Error(response.message || t('Failed to apply overwrite.'))
      toast.success(t('Selected conflicts were overwritten successfully.'))
      void queryClient.invalidateQueries({ queryKey: modelsQueryKeys.lists() })
      void queryClient.invalidateQueries({ queryKey: vendorsQueryKeys.lists() })
      handleOpenChange(false)
    } catch (cause: unknown) {
      if (token !== generation.current || controller.signal.aborted) return
      setError(cause instanceof Error ? cause.message : t('Failed to apply overwrite.'))
    } finally {
      if (token === generation.current) {
        request.current = null
        setIsSubmitting(false)
      }
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={handleOpenChange}>
      <DialogContent className='flex h-dvh max-h-dvh w-full max-w-none flex-col gap-0 overflow-hidden rounded-none p-0 sm:h-[min(85dvh,800px)] sm:max-h-[calc(100dvh-3rem)] sm:max-w-5xl sm:rounded-xl [&_[data-slot=dialog-close]]:min-h-11 [&_[data-slot=dialog-close]]:min-w-11' initialFocus={!isMobile} finalFocus={() => props.returnFocus?.() ?? false}>
        <DialogHeader className='shrink-0 border-b px-4 py-4 pr-16 sm:px-6 sm:pr-16'>
          <DialogTitle>{t('Resolve Conflicts')}</DialogTitle>
          <DialogDescription>{t('Sync adds missing models and vendors, then overwrites selected fields. Unselected existing fields keep their local values.')}</DialogDescription>
        </DialogHeader>
        <section aria-label={t('Conflicting fields')} className='flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto overscroll-contain px-4 py-4 sm:px-6'>
          {error && <Alert variant='destructive'><AlertDescription>{error}</AlertDescription></Alert>}
          {rows.length === 0 ? <p className='text-muted-foreground rounded-lg border border-dashed p-8 text-center text-sm'>{t('No conflict entries available.')}</p> : <>
            <div className='flex flex-col gap-3 sm:flex-row sm:items-center'>
              <div className='relative min-w-0 flex-1'>
                <HugeiconsIcon icon={Search01Icon} aria-hidden='true' className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2' />
                <Input className='min-h-11 pl-9' value={search} onChange={(event) => { setSearch(event.target.value); setPageIndex(0) }} placeholder={t('Search models or fields...')} aria-label={t('Search conflicting models or fields')} />
              </div>
              <Button className='min-h-11' variant='outline' onClick={() => setSelected(new Set())} disabled={!selectedRows.length || isSubmitting}>{t('Clear selection')}</Button>
            </div>
            <div className='flex flex-col gap-2 text-sm sm:flex-row sm:items-center sm:justify-between'>
              <div className='flex min-h-11 items-center gap-2'>
                <Checkbox id={allFieldsId} aria-labelledby={`${allFieldsId}-label`} className='after:absolute after:-inset-3 relative' checked={allSelected} indeterminate={selectedRows.length > 0 && !allSelected} disabled={isSubmitting || !isAdmin} onCheckedChange={(checked) => setSelected(checked ? new Set(rows.map((row) => row.id)) : new Set())} />
                <label id={`${allFieldsId}-label`} htmlFor={allFieldsId} className='flex min-h-11 cursor-pointer items-center'>{t('Select all conflict fields')}</label>
              </div>
              <div className='text-muted-foreground text-xs' aria-live='polite'>
                <p>{t('{{count}} selected fields across all pages', { count: selectedRows.length })}</p>
                {selectedOutsidePage > 0 && <p>{t('{{count}} selected fields outside this page', { count: selectedOutsidePage })}</p>}
              </div>
            </div>
            <p className='text-muted-foreground text-xs'>{t('Matching models: {{models}} · Conflict fields: {{fields}}', { models: matchingModels.size, fields: filteredRows.length })}</p>
            <p className='text-muted-foreground text-xs'>{t('Search shows all fields of matching models. Select all includes every conflict, including hidden results.')}</p>
            {!filteredRows.length && <p className='text-muted-foreground rounded-lg border border-dashed p-8 text-center text-sm'>{t('No conflicts match your search.')}</p>}
            {filteredRows.length > 0 && isMobile &&
              <div className='flex flex-col gap-3'>{pageRows.map((row) => <article key={row.id} className='flex min-w-0 flex-col gap-3 rounded-xl border p-4'>
                <label className='flex min-h-11 min-w-0 cursor-pointer items-start gap-3'>
                  <Checkbox className='relative after:absolute after:-inset-3' aria-label={t('Select row')} checked={selected.has(row.id)} disabled={isSubmitting || !isAdmin} onCheckedChange={(checked) => toggleField(row.id, !!checked)} />
                  <span className='flex min-w-0 flex-col gap-2'><span className='font-medium break-words [overflow-wrap:anywhere]'>{row.modelName}</span><StatusBadge label={t(FIELD_LABELS[row.fieldKey] ?? row.fieldKey)} variant='neutral' copyable={false} /><span className='text-muted-foreground font-mono text-xs break-words [overflow-wrap:anywhere]'>{row.fieldKey}</span></span>
                </label>
                <div className='flex min-w-0 flex-col gap-1'><span className='text-xs font-medium'>{t('Local Value')}</span><ValuePreview value={row.localValue} /></div>
                <div className='flex min-w-0 flex-col gap-1'><span className='text-xs font-medium'>{t('Upstream Value')}</span><ValuePreview value={row.upstreamValue} /></div>
              </article>)}</div>}
            {filteredRows.length > 0 && !isMobile &&
              <table className='w-full table-fixed border-collapse text-left text-sm'>
                <thead><tr className='border-b'><th scope='col' className='w-14 p-2'><span className='sr-only'>{t('Select')}</span></th><th scope='col' className='w-[24%] p-2'>{t('Model')}</th><th scope='col' className='w-[16%] p-2'>{t('Field')}</th><th scope='col' className='p-2'>{t('Local Value')}</th><th scope='col' className='p-2'>{t('Upstream Value')}</th></tr></thead>
                <tbody>{pageRows.map((row) => <tr key={row.id} className='border-b'>
                  <td className='p-2 align-top'><label className='flex min-h-11 min-w-11 cursor-pointer items-center justify-center'><Checkbox aria-label={t('Select row')} checked={selected.has(row.id)} disabled={isSubmitting || !isAdmin} onCheckedChange={(checked) => toggleField(row.id, !!checked)} /></label></td>
                  <td className='p-2 align-top font-medium break-words [overflow-wrap:anywhere]'>{row.modelName}</td>
                  <td className='p-2 align-top'><span className='flex flex-col gap-1 break-words [overflow-wrap:anywhere]'><span>{t(FIELD_LABELS[row.fieldKey] ?? row.fieldKey)}</span><span className='text-muted-foreground font-mono text-xs'>{row.fieldKey}</span></span></td>
                  <td className='p-2 align-top'><ValuePreview value={row.localValue} /></td><td className='p-2 align-top'><ValuePreview value={row.upstreamValue} /></td>
                </tr>)}</tbody>
              </table>}
          </>}
          {isSubmitting && <p role='status' className='text-muted-foreground text-sm'>{t('Closing this dialog does not undo a sync already submitted.')}</p>}
        </section>
        <DialogFooter className='m-0 shrink-0 flex-col gap-3 rounded-none px-4 py-3 sm:flex-col sm:px-6'>
          <nav aria-label={t('Conflict pagination')} className='flex w-full flex-wrap items-center justify-between gap-2'>
            <span className='text-muted-foreground text-xs' aria-live='polite'>{t('Showing {{start}}–{{end}} of {{total}} fields', { start: displayStart, end: displayEnd, total: filteredRows.length })}</span>
            <div className='flex flex-wrap items-center gap-2'>
              <label className='flex items-center gap-2 text-xs'><span className='sr-only sm:not-sr-only'>{t('Rows per page')}</span><select aria-label={t('Rows per page')} className='border-input bg-background min-h-11 rounded-md border px-2' value={pageSize} onChange={(event) => { setPageSize(Number(event.target.value)); setPageIndex(0) }}>{PAGE_SIZE_OPTIONS.map((size) => <option key={size} value={size}>{size}</option>)}</select></label>
              <Button className='min-h-11 min-w-11' size='icon' variant='outline' aria-label={t('Previous page')} disabled={currentPage === 0} onClick={() => setPageIndex(currentPage - 1)}><HugeiconsIcon icon={ArrowLeft01Icon} aria-hidden='true' className='size-4' /></Button>
              <span className='text-xs'>{t('Page {{current}} of {{total}}', { current: filteredRows.length ? currentPage + 1 : 0, total: filteredRows.length ? totalPages : 0 })}</span>
              <Button className='min-h-11 min-w-11' size='icon' variant='outline' aria-label={t('Next page')} disabled={currentPage >= totalPages - 1 || !filteredRows.length} onClick={() => setPageIndex(currentPage + 1)}><HugeiconsIcon icon={ArrowRight01Icon} aria-hidden='true' className='size-4' /></Button>
            </div>
          </nav>
          <div className='flex w-full flex-wrap justify-end gap-2'><Button className='min-h-11' variant='outline' onClick={() => handleOpenChange(false)}>{t('Cancel')}</Button><Button className='min-h-11' disabled={isSubmitting || !selectedRows.length || !isAdmin} onClick={() => { void handleApplyOverwrite() }}>{isSubmitting ? t('Applying...') : t('Apply Overwrite')}</Button></div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
