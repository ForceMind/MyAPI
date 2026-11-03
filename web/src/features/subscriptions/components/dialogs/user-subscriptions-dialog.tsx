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
import { Ban, Plus, RotateCcw, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  DataTableRowActionMenu,
  StaticDataTable,
} from '@/components/data-table'
import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
  sideDrawerFooterClassName,
} from '@/components/drawer-layout'
import { LogPagination } from '@/components/log-pagination'
import type { LogPageSize } from '@/lib/log-pagination'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { Button } from '@/components/ui/button'
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
} from '@/components/ui/dropdown-menu'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetFooter,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { formatQuota } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'
import { useUserTask } from '@/features/users/hooks/use-user-task'

import {
  getAdminPlans,
  getUserSubscriptions,
  createUserSubscription,
  invalidateUserSubscription,
  deleteUserSubscription,
  resetUserSubscriptionsByPlan,
} from '../../api'
import { formatTimestamp } from '../../lib'
import type { PlanRecord, UserSubscriptionRecord } from '../../types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: { id: number; username?: string } | null
  onSuccess?: () => void
  returnFocus?: () => HTMLElement | null
}

function SubscriptionStatusBadge(props: {
  sub: UserSubscriptionRecord['subscription']
  t: (key: string) => string
}) {
  // eslint-disable-next-line react-hooks/purity
  const now = Date.now() / 1000
  const isExpired = (props.sub.end_time || 0) > 0 && props.sub.end_time < now
  const isActive = props.sub.status === 'active' && !isExpired
  if (isActive) {
    return (
      <StatusBadge
        label={props.t('Active')}
        variant='success'
        copyable={false}
      />
    )
  }
  if (props.sub.status === 'cancelled') {
    return (
      <StatusBadge
        label={props.t('Invalidated')}
        variant='neutral'
        copyable={false}
      />
    )
  }
  return (
    <StatusBadge
      label={props.t('Expired')}
      variant='neutral'
      copyable={false}
    />
  )
}

export function UserSubscriptionsDialog(props: Props) {
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  return <UserSubscriptionsTask key={JSON.stringify([identity, props.user?.id])} {...props} />
}

function UserSubscriptionsTask(props: Props) {
  const { t } = useTranslation()
  const task = useUserTask(props.open, props.user?.id)
  const readGeneration = useRef(0)
  const opened = useRef<string | null>(null)
  const confirmationOpener = useRef<HTMLElement | null>(null)
  const closeButton = useRef<HTMLButtonElement>(null)
  const recordsHeading = useRef<HTMLHeadingElement>(null)
  const confirmationId = useRef<number | null>(null)
  const [mode, setMode] = useState<'records' | 'grant'>('records')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState<LogPageSize>(20)
  const [hasRead, setHasRead] = useState(false)
  const [readError, setReadError] = useState<string | null>(null)
  const [createError, setCreateError] = useState<string | null>(null)
  const [createUnconfirmed, setCreateUnconfirmed] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)
  const [actionBusy, setActionBusy] = useState(false)
  const [loading, setLoading] = useState(false)
  const [creating, setCreating] = useState(false)
  const [plans, setPlans] = useState<PlanRecord[]>([])
  const [subs, setSubs] = useState<UserSubscriptionRecord[]>([])
  const [selectedPlanId, setSelectedPlanId] = useState<string>('')
  const [resetting, setResetting] = useState(false)
  const busy = creating || resetting || actionBusy
  const [advanceResetTime, setAdvanceResetTime] = useState(true)
  const [resetAction, setResetAction] = useState<{
    planId: number
    planTitle: string
  } | null>(null)
  const [confirmAction, setConfirmAction] = useState<{
    type: 'invalidate' | 'delete'
    subId: number
  } | null>(null)

  const planTitleMap = useMemo(() => {
    const map = new Map<number, string>()
    plans.forEach((p) => {
      if (p.plan.id) map.set(p.plan.id, p.plan.title || `#${p.plan.id}`)
    })
    return map
  }, [plans])

  const loadData = useCallback(async (confirmCreation = false) => {
    if (!props.user?.id) return
    const snapshot = task.capture()
    const request = ++readGeneration.current
    setLoading(true)
    setReadError(null)
    setPlans([])
    setSubs([])
    try {
      const [plansRes, subsRes] = await Promise.all([
        getAdminPlans(),
        getUserSubscriptions(props.user.id),
      ])
      if (!snapshot.isCurrent() || request !== readGeneration.current) return
      if (!plansRes.success || !Array.isArray(plansRes.data) || !subsRes.success || !Array.isArray(subsRes.data) || subsRes.data.some((record) => record.subscription.user_id !== props.user?.id)) { setReadError(t('Loading failed')); return }
      const loadedSubscriptions = subsRes.data
      setPlans(plansRes.data || [])
      setSubs(loadedSubscriptions)
      setHasRead(true)
      setPage((previous) => Math.min(previous, Math.max(1, Math.ceil(loadedSubscriptions.length / pageSize))))
      if (confirmCreation) { setCreateUnconfirmed(false); setCreateError(null) }
    } catch {
      if (!snapshot.isCurrent() || request !== readGeneration.current) return
      setReadError(t('Loading failed'))
      toast.error(t('Loading failed'))
    } finally {
      if (snapshot.isCurrent() && request === readGeneration.current) setLoading(false)
    }
  }, [props.user?.id, t, task, pageSize])

  useEffect(() => {
    if (!props.open) { opened.current = null; return }
    if (props.open && props.user?.id) {
      const key = JSON.stringify([task.identity, props.user.id])
      if (opened.current === key) return
      opened.current = key
      setSelectedPlanId('')
      setMode('records')
      setPage(1)
      setHasRead(false)
      setCreating(false)
      setResetting(false)
      setActionBusy(false)
      setCreateError(null)
      setActionError(null)
      setConfirmAction(null)
      setResetAction(null)
      loadData()
    }
  }, [props.open, props.user?.id, loadData, task])

  const rememberConfirmation = (id: number) => {
    confirmationId.current = id
    const row = [...document.querySelectorAll<HTMLElement>('[data-user-subscription-actions]')].find((element) => element.dataset.userSubscriptionActions === String(id))
    confirmationOpener.current = row?.querySelector<HTMLElement>('button[aria-haspopup]') ?? null
    setActionError(null)
  }

  const returnToRecord = () => {
    if (confirmationOpener.current?.isConnected && !confirmationOpener.current.closest('[hidden]')) return confirmationOpener.current
    const row = [...document.querySelectorAll<HTMLElement>('[data-user-subscription-actions]')].find((element) => element.dataset.userSubscriptionActions === String(confirmationId.current) && !element.closest('[hidden]'))
    return row?.querySelector<HTMLElement>('button[aria-haspopup]') ?? recordsHeading.current ?? closeButton.current
  }

  const handleCreate = async () => {
    if (!props.user?.id || !selectedPlanId) {
      toast.error(t('Please select a subscription plan'))
      return
    }
    if (loading || readError || createUnconfirmed || !plans.some((record) => String(record.plan.id) === selectedPlanId)) return
    const operation = task.begin()
    if (!operation) return
    setCreateError(null)
    setCreating(true)
    try {
      const res = await createUserSubscription(props.user.id, {
        plan_id: Number(selectedPlanId),
      })
      if (!operation.isCurrent()) return
      if (res.success) {
        toast.success(res.data?.message || t('Added successfully'))
        setSelectedPlanId('')
        await loadData()
        if (!operation.isCurrent()) return
        props.onSuccess?.()
      } else setCreateError(res.message || t('Request failed'))
    } catch (error) {
      if (!operation.isCurrent()) return
      let status: unknown
      if (typeof error === 'object' && error !== null && 'response' in error) {
        const response = error.response
        if (typeof response === 'object' && response !== null && 'status' in response) status = response.status
      }
      if (typeof status !== 'number' || status < 400 || status >= 500) {
        setCreateUnconfirmed(true)
        setCreateError(null)
      } else setCreateError(t('Request failed'))
      toast.error(t('Request failed'))
    } finally {
      if (operation.isCurrent()) setCreating(false)
      operation.finish()
    }
  }

  const handleConfirmAction = async () => {
    if (!confirmAction) return
    const operation = task.begin()
    if (!operation) return
    setActionError(null)
    setActionBusy(true)
    try {
      if (confirmAction.type === 'invalidate') {
        const res = await invalidateUserSubscription(confirmAction.subId)
        if (!operation.isCurrent()) return
        if (res.success) {
          toast.success(res.data?.message || t('Has been invalidated'))
          await loadData()
          if (!operation.isCurrent()) return
          props.onSuccess?.()
          setConfirmAction(null)
        } else setActionError(res.message || t('Operation failed'))
      } else {
        const res = await deleteUserSubscription(confirmAction.subId)
        if (!operation.isCurrent()) return
        if (res.success) {
          toast.success(t('Deleted'))
          await loadData()
          if (!operation.isCurrent()) return
          props.onSuccess?.()
          setConfirmAction(null)
        } else setActionError(res.message || t('Operation failed'))
      }
    } catch {
      if (!operation.isCurrent()) return
      setActionError(t('Operation failed'))
      toast.error(t('Operation failed'))
    } finally {
      if (operation.isCurrent()) setActionBusy(false)
      operation.finish()
    }
  }

  const handleResetConfirm = async () => {
    if (!props.user?.id || !resetAction) return
    const operation = task.begin()
    if (!operation) return
    setActionError(null)
    setResetting(true)
    try {
      const res = await resetUserSubscriptionsByPlan(props.user.id, {
        plan_id: resetAction.planId,
        advance_reset_time: advanceResetTime,
      })
      if (!operation.isCurrent()) return
      if (res.success) {
        const count = res.data?.reset_count
        toast.success(typeof count === 'number' && Number.isSafeInteger(count) && count >= 0 ? t('Reset {{count}} active subscriptions', { count }) : t('Operation successful'))
        await loadData()
        if (!operation.isCurrent()) return
        props.onSuccess?.()
        setResetAction(null)
      } else setActionError(res.message || t('Operation failed'))
    } catch {
      if (!operation.isCurrent()) return
      setActionError(t('Operation failed'))
      toast.error(t('Operation failed'))
    } finally {
      if (operation.isCurrent()) setResetting(false)
      operation.finish()
    }
  }

  let emptyContent = t('No subscription records')
  if (loading) emptyContent = t('Loading...')
  else if (readError || !hasRead) emptyContent = t('Unknown')
  const recordsKnown = hasRead && !loading && !readError
  const pageRows = subs.slice((page - 1) * pageSize, page * pageSize)

  return (
    <>
      <Sheet open={props.open} onOpenChange={(next) => { if (!task.isPending()) props.onOpenChange(next) }}>
        <SheetContent showCloseButton={!busy} finalFocus={props.returnFocus} className={sideDrawerContentClassName('sm:max-w-3xl [&_[data-slot=button]]:min-h-11 [&_[data-slot=button]]:min-w-11 [&_[data-slot=select-trigger]]:min-h-11')}>
          <SheetHeader className={sideDrawerHeaderClassName()}>
            <SheetTitle>{t('User Subscription Management')}</SheetTitle>
            <SheetDescription>
              {props.user?.username || '-'} · {t('User ID')}: {props.user?.id ?? t('Unknown')}
            </SheetDescription>
          </SheetHeader>

          <div className={sideDrawerFormClassName()}>
            {readError ? <div role='alert' className='text-destructive space-y-3 text-sm'><p>{readError}</p><Button variant='outline' onClick={() => void loadData(true)}>{t('Retry')}</Button></div> : null}
            {createUnconfirmed ? <div role='alert' className='text-destructive text-sm break-words'>{t('The subscription creation outcome is unconfirmed. Refresh and check the records before submitting again.')}</div> : null}
            {createError ? <div role='alert' className='text-destructive text-sm break-words'>{createError}</div> : null}
            <p role='status' className='text-muted-foreground text-sm'>{recordsKnown ? t('Loaded subscription records: {{count}}', { count: subs.length }) : t('Unknown')}</p>
            <Tabs value={mode} onValueChange={(value) => { if (!task.isPending() && (value === 'records' || value === 'grant')) setMode(value) }}>
              <TabsList aria-label={t('Subscription task')} className='grid h-auto w-full grid-cols-2 group-data-horizontal/tabs:h-auto'>
                <TabsTrigger value='records' disabled={busy} className='min-h-11 whitespace-normal'>{t('Subscription records')}</TabsTrigger>
                <TabsTrigger value='grant' disabled={busy} className='min-h-11 whitespace-normal'>{t('Grant subscription')}</TabsTrigger>
              </TabsList>
              <TabsContent value='records' keepMounted className='min-w-0 space-y-4'>
                <h3 ref={recordsHeading} tabIndex={-1} className='font-medium outline-none'>{t('Subscription records')}</h3>
            <StaticDataTable
              data={loading ? [] : pageRows}
              getRowKey={(record) => record.subscription.id}
              emptyClassName={loading ? 'py-8' : 'text-muted-foreground py-8'}
              emptyContent={emptyContent}
              columns={[
                {
                  id: 'id',
                  header: t('ID'),
                  cell: (record) => <TableId value={record.subscription.id} />,
                },
                {
                  id: 'plan',
                  header: t('Plan'),
                  cell: (record) => {
                    const sub = record.subscription

                    return (
                      <div>
                        <div className='font-medium whitespace-normal break-all'>
                          {planTitleMap.get(sub.plan_id) || `#${sub.plan_id}`}
                        </div>
                        <div className='text-muted-foreground text-sm'>
                          {t('Source')}: {sub.source || '-'}
                        </div>
                      </div>
                    )
                  },
                },
                {
                  id: 'status',
                  header: t('Status'),
                  cell: (record) => (
                    <SubscriptionStatusBadge sub={record.subscription} t={t} />
                  ),
                },
                {
                  id: 'validity',
                  header: t('Validity'),
                  cell: (record) => {
                    const sub = record.subscription

                    return (
                      <div className='text-sm'>
                        <div>
                          {t('Start')}: {formatTimestamp(sub.start_time)}
                        </div>
                        <div>
                          {t('End')}: {formatTimestamp(sub.end_time)}
                        </div>
                      </div>
                    )
                  },
                },
                {
                  id: 'quota',
                  header: t('Total Quota'),
                  cell: (record) => {
                    const sub = record.subscription
                    if (typeof sub.amount_total !== 'number' || !Number.isFinite(sub.amount_total)) return t('Unknown')
                    const total = Number(sub.amount_total)
                    if (total > 0 && (typeof sub.amount_used !== 'number' || !Number.isFinite(sub.amount_used))) return t('Unknown')
                    const used = Number(sub.amount_used)
                    return total > 0
                      ? `${formatQuota(used)}/${formatQuota(total)}`
                      : t('Unlimited')
                  },
                },
                {
                  id: 'actions',
                  header: t('Actions'),
                  className: 'text-right',
                  cellClassName: 'text-right',
                  cell: (record) => {
                    const sub = record.subscription
                    const now = Date.now() / 1000
                    const isExpired =
                      (sub.end_time || 0) > 0 && sub.end_time < now
                    const isActive = sub.status === 'active' && !isExpired

                    return (
                      <div data-user-subscription-actions={String(sub.id)}><DataTableRowActionMenu ariaLabel={t('Actions')}>
                        <DropdownMenuItem
                          disabled={!isActive}
                          onClick={() => {
                            rememberConfirmation(sub.id)
                            setAdvanceResetTime(true)
                            setResetAction({
                              planId: sub.plan_id,
                              planTitle:
                                planTitleMap.get(sub.plan_id) ||
                                `#${sub.plan_id}`,
                            })
                          }}
                        >
                          {t('Reset quota')}
                          <DropdownMenuShortcut>
                            <RotateCcw size={16} />
                          </DropdownMenuShortcut>
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          disabled={!isActive}
                          onClick={() => {
                            rememberConfirmation(sub.id)
                            setConfirmAction({
                              type: 'invalidate',
                              subId: sub.id,
                            })
                          }}
                        >
                          {t('Invalidate')}
                          <DropdownMenuShortcut>
                            <Ban size={16} />
                          </DropdownMenuShortcut>
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          variant='destructive'
                          onClick={() => {
                            rememberConfirmation(sub.id)
                            setConfirmAction({
                              type: 'delete',
                              subId: sub.id,
                            })
                          }}
                        >
                          {t('Delete')}
                          <DropdownMenuShortcut>
                            <Trash2 size={16} />
                          </DropdownMenuShortcut>
                        </DropdownMenuItem>
                      </DataTableRowActionMenu></div>
                    )
                  },
                },
              ]}
            />
                <LogPagination label={t('User subscription records')} page={page} pageSize={pageSize} total={recordsKnown ? subs.length : undefined} busy={loading || busy} onPageChange={setPage} onPageSizeChange={(size) => { setPage(1); setPageSize(size) }} />
              </TabsContent>
              <TabsContent value='grant' keepMounted className='min-w-0 space-y-4'>
            <div className='flex flex-wrap gap-2'>
              <Select
                items={plans.map((p) => ({
                  value: String(p.plan.id),
                  label: (
                    <>
                      {p.plan.title} ({typeof p.plan.price_amount === 'number' && Number.isFinite(p.plan.price_amount) ? `$${Number(p.plan.price_amount).toFixed(2)}` : t('Unknown')})
                    </>
                  ),
                }))}
                value={selectedPlanId}
                onValueChange={(v) => v !== null && setSelectedPlanId(v)}
                disabled={busy || loading || Boolean(readError) || createUnconfirmed}
              >
                <SelectTrigger className='flex-1'>
                  <SelectValue placeholder={t('Select subscription plan')} />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {plans.map((p) => (
                      <SelectItem key={p.plan.id} value={String(p.plan.id)}>
                        {p.plan.title} ({typeof p.plan.price_amount === 'number' && Number.isFinite(p.plan.price_amount) ? `$${Number(p.plan.price_amount).toFixed(2)}` : t('Unknown')})
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <Button
                onClick={handleCreate}
                disabled={busy || loading || Boolean(readError) || createUnconfirmed || !selectedPlanId || !plans.some((record) => String(record.plan.id) === selectedPlanId)}
              >
                <Plus className='mr-1 h-4 w-4' />
                {t('Add subscription')}
              </Button>
            </div>

              </TabsContent>
            </Tabs>
          </div>
          <SheetFooter className={sideDrawerFooterClassName()}>
            <Button variant='outline' disabled={loading || busy} onClick={() => void loadData(true)}>{t('Refresh')}</Button>
            <Button ref={closeButton} variant='outline' disabled={busy} onClick={() => props.onOpenChange(false)}>{t('Close')}</Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

        <ConfirmDialog
          open={confirmAction !== null}
          onOpenChange={(v) => { if (!v && !task.isPending()) setConfirmAction(null) }}
          returnFocus={returnToRecord}
          title={
            confirmAction?.type === 'invalidate'
              ? t('Confirm invalidate')
              : t('Confirm delete')
          }
          desc={
            confirmAction?.type === 'invalidate'
              ? t(
                  'After invalidating, this subscription will be immediately deactivated. Historical records are not affected. Continue?'
                )
              : t(
                  'Deleting will permanently remove this subscription record (including benefit details). Continue?'
                )
          }
          handleConfirm={handleConfirmAction}
          isLoading={actionBusy}
          disabled={!confirmAction}
          destructive={confirmAction?.type === 'delete'}
        >
          {confirmAction && <p className='text-muted-foreground text-sm break-words'>{props.user?.username} · {t('User ID')}: {props.user?.id} · {t('ID')}: {confirmAction.subId}</p>}
          {actionError ? <div role='alert' className='text-destructive text-sm break-words'>{actionError}</div> : null}
        </ConfirmDialog>

        <ConfirmDialog
          open={resetAction !== null}
          onOpenChange={(v) => { if (!v && !task.isPending()) setResetAction(null) }}
          returnFocus={returnToRecord}
          title={t('Reset subscription quota')}
          desc={t('Reset active {{plan}} subscriptions for this user?', {
            plan: resetAction?.planTitle ?? t('Unknown'),
          })}
          confirmText={t('Reset quota')}
          handleConfirm={handleResetConfirm}
          isLoading={resetting}
          disabled={!resetAction}
        >
          <p className='text-muted-foreground text-sm break-words'>{props.user?.username} · {t('User ID')}: {props.user?.id}</p>
          {actionError ? <div role='alert' className='text-destructive text-sm break-words'>{actionError}</div> : null}
          <label className='flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm'>
            <span>{t('Advance next reset time')}</span>
            <Switch
              checked={advanceResetTime}
              onCheckedChange={(checked) => setAdvanceResetTime(!!checked)}
              aria-label={t('Advance next reset time')}
            />
          </label>
        </ConfirmDialog>
    </>
  )
}
