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
import { getRouteApi } from '@tanstack/react-router'
import { flexRender } from '@tanstack/react-table'
import { useCallback, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { getCellLabel } from '@/components/data-table/layout/card-cell-utils'
import { ErrorState } from '@/components/error-state'
import { LogPagination } from '@/components/log-pagination'
import { Button } from '@/components/ui/button'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { getLogPageCount, normalizeLogPagination } from '@/lib/log-pagination'
import { useAuthStore } from '@/stores/auth-store'

import { deleteDeployment, listDeployments, searchDeployments } from '../api'
import { getDeploymentStatusOptions } from '../constants'
import { deploymentsQueryKeys } from '../lib'
import type { Deployment } from '../types'
import { useDeploymentTask } from '../hooks/use-deployment-task'
import { useDeploymentsColumns } from './deployments-columns'
import { ExtendDeploymentDialog } from './dialogs/extend-deployment-dialog'
import { RenameDeploymentDialog } from './dialogs/rename-deployment-dialog'
import { UpdateConfigDialog } from './dialogs/update-config-dialog'
import { ViewDetailsDialog } from './dialogs/view-details-dialog'
import { ViewLogsDialog } from './dialogs/view-logs-dialog'

const route = getRouteApi('/_authenticated/models/$section')

export function DeploymentsTable() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  const isMobile = useMediaQuery('(max-width: 640px)')
  const opener = useRef<{ node: HTMLElement | null; id: string; logs: boolean } | null>(null)
  const heading = useRef<HTMLHeadingElement>(null)
  const rememberAction = (id: string | number, logs = false) => {
    const container = [...document.querySelectorAll<HTMLElement>('[data-deployment-actions]')].find((element) => element.dataset.deploymentActions === String(id))
    opener.current = { id: String(id), logs, node: container?.querySelector<HTMLElement>(logs ? '[data-deployment-logs]' : 'button[aria-haspopup]') ?? null }
  }
  const returnFocus = useCallback(() => {
    const action = opener.current
    if (action?.node?.isConnected) return action.node
    const container = [...document.querySelectorAll<HTMLElement>('[data-deployment-actions]')].find((element) => element.dataset.deploymentActions === action?.id)
    return container?.querySelector<HTMLElement>(action?.logs ? '[data-deployment-logs]' : 'button[aria-haspopup]') ?? heading.current
  }, [])

  // URL state (use dedicated keys so it won't collide with metadata table)
  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination: urlPagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: {
      pageKey: 'dPage',
      pageSizeKey: 'dPageSize',
      defaultPage: 1,
      defaultPageSize: isMobile ? 8 : 10,
    },
    globalFilter: { enabled: true, key: 'dFilter' },
    columnFilters: [
      { columnId: 'status', searchKey: 'dStatus', type: 'array' },
    ],
  })

  const pagination = normalizeLogPagination(urlPagination.pageIndex, urlPagination.pageSize)

  const keyword = globalFilter ?? ''
  const statusFilter =
    (columnFilters.find((f) => f.id === 'status')?.value as string[]) || []
  const activeStatus =
    statusFilter.length > 0 && !statusFilter.includes('all')
      ? statusFilter[0]
      : undefined

  // Dialog state
  const [logsOpen, setLogsOpen] = useState(false)
  const [logsDeploymentId, setLogsDeploymentId] = useState<
    string | number | null
  >(null)
  const [detailsOpen, setDetailsOpen] = useState(false)
  const [detailsDeploymentId, setDetailsDeploymentId] = useState<
    string | number | null
  >(null)
  const [updateOpen, setUpdateOpen] = useState(false)
  const [updateDeploymentId, setUpdateDeploymentId] = useState<
    string | number | null
  >(null)
  const [extendOpen, setExtendOpen] = useState(false)
  const [extendDeploymentId, setExtendDeploymentId] = useState<
    string | number | null
  >(null)
  const [renameOpen, setRenameOpen] = useState(false)
  const [renameDeploymentId, setRenameDeploymentId] = useState<
    string | number | null
  >(null)
  const [renameCurrentName, setRenameCurrentName] = useState<string>('')

  // Delete confirm
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<Deployment | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const deletion = useDeploymentTask(deleteOpen, deleteTarget?.id)

  const { data, isLoading, isFetching, isError, refetch } = useQuery({
    queryKey: [...deploymentsQueryKeys.list({
      keyword,
      status: activeStatus,
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
    }), identity],
    queryFn: async () => {
      const result = keyword.trim()
        ? await searchDeployments({
          keyword,
          status: activeStatus,
          p: pagination.pageIndex + 1,
          page_size: pagination.pageSize,
        })
        : await listDeployments({
        status: activeStatus,
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
      })
      if (!result.success) throw new Error(result.message || 'Failed to load deployments')
      return result
    },
  })

  const deployments = useMemo(() => isError ? [] : data?.data?.items || [], [data?.data?.items, isError])
  const returnedTotal = data?.data?.total
  const totalCount = !isError && typeof returnedTotal === 'number' && Number.isSafeInteger(returnedTotal) && returnedTotal >= deployments.length && returnedTotal >= 0 ? returnedTotal : undefined
  const pageCount = getLogPageCount(totalCount, pagination.pageSize)

  const handleDelete = async () => {
    if (!deleteTarget) return
    const operation = deletion.begin()
    if (!operation) return
    setDeleteError(null)
    setIsDeleting(true)
    try {
      const res = await deleteDeployment(deleteTarget.id)
      if (!operation.isCurrent()) return
      if (res?.success) {
        toast.success(t('Deleted successfully'))
        queryClient.invalidateQueries({
          queryKey: deploymentsQueryKeys.lists(),
        })
        setDeleteOpen(false)
        setDeleteTarget(null)
      } else {
        setDeleteError(res?.message || t('Delete failed'))
        toast.error(res?.message || t('Delete failed'))
      }
    } catch (err) {
      if (!operation.isCurrent()) return
      setDeleteError(err instanceof Error ? err.message : t('Delete failed'))
      toast.error(err instanceof Error ? err.message : t('Delete failed'))
    } finally {
      if (operation.isCurrent()) setIsDeleting(false)
      operation.finish()
    }
  }

  const columns = useDeploymentsColumns({
    onViewLogs: (id) => {
      rememberAction(id, true)
      setLogsDeploymentId(id)
      setLogsOpen(true)
    },
    onViewDetails: (id) => {
      rememberAction(id)
      setDetailsDeploymentId(id)
      setDetailsOpen(true)
    },
    onUpdateConfig: (id) => {
      rememberAction(id)
      setUpdateDeploymentId(id)
      setUpdateOpen(true)
    },
    onExtend: (id) => {
      rememberAction(id)
      setExtendDeploymentId(id)
      setExtendOpen(true)
    },
    onRename: (id, currentName) => {
      rememberAction(id)
      setRenameCurrentName(currentName)
      setRenameDeploymentId(id)
      setRenameOpen(true)
    },
    onDelete: (deployment) => {
      rememberAction(deployment.id)
      setDeleteError(null)
      setDeleteTarget(deployment)
      setDeleteOpen(true)
    },
  })

  const { table } = useDataTable({
    data: deployments,
    columns,
    totalCount,
    pageCount: pageCount ?? -1,
    getRowId: (deployment) => String(deployment.id),
    columnFilters,
    pagination,
    globalFilter,
    onColumnFiltersChange,
    onPaginationChange,
    onGlobalFilterChange,
    manualPagination: true,
    manualFiltering: true,
    withSortedRowModel: false,
    ensurePageInRange: totalCount === undefined ? undefined : ensurePageInRange,
  })

  const statusFilterOptions = useMemo(() => {
    return [...getDeploymentStatusOptions(t)].map((opt) => ({
      label: opt.label,
      value: opt.value,
    }))
  }, [t])

  return (
    <>
      <h2 ref={heading} tabIndex={-1} className='sr-only'>{t('Deployments')}</h2>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        fixedHeight={false}
        pagination={<LogPagination label={t('Deployment pagination')} page={pagination.pageIndex + 1} pageSize={pagination.pageSize} total={totalCount} busy={isLoading || isFetching} onPageChange={(page) => onPaginationChange({ pageIndex: page - 1, pageSize: pagination.pageSize })} onPageSizeChange={(pageSize) => onPaginationChange({ pageIndex: 0, pageSize })} />}
        errorState={isError ? <ErrorState title={t('Failed to load deployments')} description={t('Please try again later.')} onRetry={() => { if (!isFetching) void refetch() }} /> : undefined}
        mobile={isLoading ? <div role='status'>{t('Loading...')}</div> : (
          <div className='grid min-w-0 gap-4'>
            {table.getRowModel().rows.map((row) => (
              <article key={row.id} className='bg-card min-w-0 space-y-3 rounded-xl border p-4'>
                {row.getAllCells().filter((cell) => ['name', 'status', 'provider', 'time_remaining'].includes(cell.column.id)).map((cell) => (
                  <div key={cell.id} className='min-w-0 break-words'>{flexRender(cell.column.columnDef.cell, cell.getContext())}</div>
                ))}
                <details className='min-w-0 rounded-lg border p-3'>
                  <summary className='min-h-11 cursor-pointer py-3 font-medium'>{t('More details')}</summary>
                  <dl className='mt-3 grid min-w-0 gap-4'>
                    {row.getAllCells().filter((cell) => ['id', 'hardware', 'created_at'].includes(cell.column.id)).map((cell) => (
                      <div key={cell.id} className='min-w-0'><dt className='text-muted-foreground mb-1 text-xs'>{getCellLabel(cell)}</dt><dd className='min-w-0 break-words'>{flexRender(cell.column.columnDef.cell, cell.getContext())}</dd></div>
                    ))}
                  </dl>
                </details>
                {row.getAllCells().filter((cell) => cell.column.id === 'actions').map((cell) => <div key={cell.id} className='border-t pt-3'>{flexRender(cell.column.columnDef.cell, cell.getContext())}</div>)}
              </article>
            ))}
            {deployments.length === 0 ? <div className='rounded-xl border p-6 text-center'><h3>{t('No Deployments Found')}</h3><p className='text-muted-foreground mt-2 text-sm'>{t('No deployments available. Create one to get started.')}</p></div> : null}
          </div>
        )}
        emptyTitle={t('No Deployments Found')}
        emptyDescription={t(
          'No deployments available. Create one to get started.'
        )}
        skeletonKeyPrefix='deployment-skeleton'
        applyHeaderSize
        toolbarProps={{
          searchPlaceholder: t('Search deployments...'),
          searchDebounceMs: 500,
          filters: [
            {
              columnId: 'status',
              title: t('Status'),
              options: statusFilterOptions,
              singleSelect: true,
            },
          ],
        }}
      />

      <ViewLogsDialog
        open={logsOpen}
        onOpenChange={(open) => {
          setLogsOpen(open)
          if (!open) setLogsDeploymentId(null)
        }}
        deploymentId={logsDeploymentId}
        returnFocus={returnFocus}
      />

      <ViewDetailsDialog
        open={detailsOpen}
        onOpenChange={(open) => {
          setDetailsOpen(open)
          if (!open) setDetailsDeploymentId(null)
        }}
        deploymentId={detailsDeploymentId}
        returnFocus={returnFocus}
      />

      <UpdateConfigDialog
        open={updateOpen}
        onOpenChange={(open) => {
          setUpdateOpen(open)
          if (!open) setUpdateDeploymentId(null)
        }}
        deploymentId={updateDeploymentId}
        returnFocus={returnFocus}
      />

      <ExtendDeploymentDialog
        open={extendOpen}
        onOpenChange={(open) => {
          setExtendOpen(open)
          if (!open) setExtendDeploymentId(null)
        }}
        deploymentId={extendDeploymentId}
        returnFocus={returnFocus}
      />

      <RenameDeploymentDialog
        open={renameOpen}
        onOpenChange={(open) => {
          setRenameOpen(open)
          if (!open) setRenameDeploymentId(null)
        }}
        deploymentId={renameDeploymentId}
        currentName={renameCurrentName}
        returnFocus={returnFocus}
      />

      <AlertDialog open={deleteOpen} onOpenChange={(next) => { if (!deletion.isPending()) setDeleteOpen(next) }}>
        <AlertDialogContent finalFocus={returnFocus} className='[&_button]:min-h-11 [&_button]:min-w-11'>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Confirm delete')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Are you sure you want to delete deployment "{{name}}"? This action cannot be undone.',
                {
                  name:
                    deleteTarget?.container_name ||
                    deleteTarget?.deployment_name ||
                    deleteTarget?.id,
                }
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError ? <div role='alert' className='text-destructive text-sm break-words'>{deleteError}</div> : null}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeleting}>
              {t('Cancel')}
            </AlertDialogCancel>
            <Button
              onClick={handleDelete}
              disabled={isDeleting}
              variant='destructive'
            >
              {isDeleting ? t('Deleting...') : t('Delete')}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
