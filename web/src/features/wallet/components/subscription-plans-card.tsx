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
import { RefreshCw, Sparkles, Check } from 'lucide-react'
import { useState, useEffect, useLayoutEffect, useMemo, useCallback, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  StatusBadge,
  dotColorMap,
  textColorMap,
} from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { LogPagination } from '@/components/log-pagination'
import type { LogPageSize } from '@/lib/log-pagination'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { useAuthStore } from '@/stores/auth-store'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  getPublicPlans,
  getSelfSubscriptionFull,
  updateBillingPreference,
} from '@/features/subscriptions/api'
import { SubscriptionPurchaseDialog } from '@/features/subscriptions/components/dialogs/subscription-purchase-dialog'
import { formatDuration, formatResetPeriod } from '@/features/subscriptions/lib'
import type {
  PlanRecord,
  UserSubscriptionRecord,
} from '@/features/subscriptions/types'
import { formatQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { PaymentMethod, TopupInfo } from '../types'

interface SubscriptionPlansCardProps {
  topupInfo: TopupInfo | null
  onAvailabilityChange?: (available: boolean) => void
  userQuota?: number
  onPurchaseSuccess?: () => void | Promise<void>
  canPurchaseSubscription?: boolean
}

function getEpayMethods(payMethods: PaymentMethod[] = []): PaymentMethod[] {
  return payMethods.filter(
    (m) => m?.type && m.type !== 'stripe' && m.type !== 'creem'
  )
}

function getBillingPreferenceLabel(
  preference: string,
  t: (key: string) => string
): string {
  switch (preference) {
    case 'subscription_first':
      return t('Subscription First')
    case 'wallet_first':
      return t('Wallet First')
    case 'subscription_only':
      return t('Subscription Only')
    case 'wallet_only':
      return t('Wallet Only')
    default:
      return preference
  }
}

export function SubscriptionPlansCard(props: SubscriptionPlansCardProps) {
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  return <SubscriptionPlansSession key={identity} {...props} identity={identity} />
}

function SubscriptionPlansSession({ topupInfo, onAvailabilityChange, userQuota, onPurchaseSuccess, canPurchaseSubscription = true, identity }: SubscriptionPlansCardProps & { identity: string }) {
  const { t } = useTranslation()

  const alive = useRef(true)
  const initialized = useRef(false)
  const initialCapability = useRef(canPurchaseSubscription)
  const plansRead = useRef(0)
  const selfRead = useRef(0)
  const preferenceFlight = useRef(false)
  const cardHeading = useRef<HTMLHeadingElement>(null)
  const catalogHeading = useRef<HTMLHeadingElement>(null)
  const purchaseOpener = useRef<HTMLElement | null>(null)
  useLayoutEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  const isCurrent = useCallback(() => {
    const { auth } = useAuthStore.getState()
    return alive.current && identity === `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`
  }, [identity])
  const [mode, setMode] = useState<'current' | 'records' | 'plans'>('current')
  const activeMode = mode === 'plans' && !canPurchaseSubscription ? 'current' : mode
  const [pages, setPages] = useState({ current: 1, records: 1, plans: 1 })
  const [pageSize, setPageSize] = useState<LogPageSize>(20)
  const [plansKnown, setPlansKnown] = useState(false)
  const [selfKnown, setSelfKnown] = useState(false)
  const [plansError, setPlansError] = useState('')
  const [selfError, setSelfError] = useState('')
  const [preferenceError, setPreferenceError] = useState('')
  const [preferencePending, setPreferencePending] = useState(false)
  const [plansReading, setPlansReading] = useState(false)
  const [plans, setPlans] = useState<PlanRecord[]>([])
  const [activeSubscriptions, setActiveSubscriptions] = useState<
    UserSubscriptionRecord[]
  >([])
  const [allSubscriptions, setAllSubscriptions] = useState<
    UserSubscriptionRecord[]
  >([])
  const [billingPreference, setBillingPreference] =
    useState('subscription_first')
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)

  const [purchaseOpen, setPurchaseOpen] = useState(false)
  const [selectedPlan, setSelectedPlan] = useState<PlanRecord | null>(null)

  const enableStripe = !!topupInfo?.enable_stripe_topup
  const enableCreem = !!topupInfo?.enable_creem_topup
  const enableWaffoPancake = !!topupInfo?.enable_waffo_pancake_topup
  const enableOnlineTopUp = !!topupInfo?.enable_online_topup
  const epayMethods = useMemo(
    () => getEpayMethods(topupInfo?.pay_methods),
    [topupInfo?.pay_methods]
  )

  const fetchPlans = useCallback(async () => {
    if (!isCurrent()) return
    const generation = ++plansRead.current
    setPlansError('')
    if (!canPurchaseSubscription) {
      setPlans([])
      setPlansKnown(true)
      setPlansReading(false)
      return
    }
    setPlansReading(true)
    setPlansKnown(false)
    try {
      const res = await getPublicPlans()
      if (!isCurrent() || generation !== plansRead.current) return
      if (!res.success || !Array.isArray(res.data)) throw new Error(res.message || t('Unable to load subscription plans'))
      setPlans(res.data)
      setPlansKnown(true)
    } catch (error) {
      if (!isCurrent() || generation !== plansRead.current) return
      setPlansError(error instanceof Error && error.message ? error.message : t('Unable to load subscription plans'))
    } finally {
      if (isCurrent() && generation === plansRead.current) setPlansReading(false)
    }
  }, [canPurchaseSubscription, isCurrent, t])

  const fetchSelfSubscription = useCallback(async () => {
    if (!isCurrent() || preferenceFlight.current) return
    const generation = ++selfRead.current
    setRefreshing(true)
    setSelfKnown(false)
    setSelfError('')
    try {
      const res = await getSelfSubscriptionFull()
      if (!isCurrent() || generation !== selfRead.current) return
      const owner = useAuthStore.getState().auth.user?.id
      if (!res.success || !res.data || !Array.isArray(res.data.subscriptions) || !Array.isArray(res.data.all_subscriptions) || res.data.all_subscriptions.some((record) => owner != null && record.subscription.user_id !== owner) || res.data.subscriptions.some((record) => owner != null && record.subscription.user_id !== owner)) throw new Error(res.message || t('Unable to load subscriptions'))
      setBillingPreference(res.data.billing_preference || 'subscription_first')
      setActiveSubscriptions(res.data.subscriptions)
      setAllSubscriptions(res.data.all_subscriptions)
      setSelfKnown(true)
    } catch (error) {
      if (!isCurrent() || generation !== selfRead.current) return
      setSelfError(error instanceof Error && error.message ? error.message : t('Unable to load subscriptions'))
    } finally {
      if (isCurrent() && generation === selfRead.current) setRefreshing(false)
    }
  }, [isCurrent, t])

  useEffect(() => {
    if (initialized.current && initialCapability.current === canPurchaseSubscription) return
    initialized.current = true
    initialCapability.current = canPurchaseSubscription
    const init = async () => {
      setLoading(true)
      await Promise.all([fetchPlans(), fetchSelfSubscription()])
      if (isCurrent()) setLoading(false)
    }
    void init()
  }, [canPurchaseSubscription, fetchPlans, fetchSelfSubscription, isCurrent])
  useEffect(() => {
    if (!canPurchaseSubscription) {
      setPurchaseOpen(false)
      setSelectedPlan(null)
      setMode('current')
    }
  }, [canPurchaseSubscription])

  const handleRefresh = async () => { if (!preferenceFlight.current) await fetchSelfSubscription() }
  const handlePreferenceChange = async (pref: string) => {
    if (!isCurrent() || !selfKnown || preferenceFlight.current || refreshing) return
    preferenceFlight.current = true
    selfRead.current += 1
    setPreferencePending(true)
    setPreferenceError('')
    const previous = billingPreference
    setBillingPreference(pref)
    try {
      const res = await updateBillingPreference(pref)
      if (!isCurrent()) return
      if (res.success) {
        toast.success(t('Updated successfully'))
        const normalized = res.data?.billing_preference || pref
        setBillingPreference(normalized)
      } else {
        setPreferenceError(res.message || t('Update failed'))
        toast.error(res.message || t('Update failed'))
        setBillingPreference(previous)
      }
    } catch {
      if (!isCurrent()) return
      setPreferenceError(t('Request failed'))
      toast.error(t('Request failed'))
      setBillingPreference(previous)
    } finally {
      preferenceFlight.current = false
      if (isCurrent()) setPreferencePending(false)
    }
  }

  const hasActive = activeSubscriptions.length > 0
  const hasAny = allSubscriptions.length > 0
  const purchaseCountsKnown = selfKnown && allSubscriptions.every((record) => Number.isSafeInteger(record.subscription.plan_id) && record.subscription.plan_id > 0)
  const unknownActiveFields = selfKnown && activeSubscriptions.some(({ subscription }) => typeof subscription.amount_total !== 'number' || !Number.isFinite(subscription.amount_total) || subscription.amount_total < 0 || (subscription.amount_total > 0 && (typeof subscription.amount_used !== 'number' || !Number.isFinite(subscription.amount_used) || subscription.amount_used < 0)) || typeof subscription.end_time !== 'number' || !Number.isFinite(subscription.end_time))
  const isAvailable = loading || !selfKnown || Boolean(selfError || plansError) || (canPurchaseSubscription && (!plansKnown || plans.length > 0)) || hasAny
  const disablePref = selfKnown && !hasActive
  const isSubPref =
    billingPreference === 'subscription_first' ||
    billingPreference === 'subscription_only'
  const displayPref =
    selfKnown && disablePref && isSubPref ? 'wallet_first' : billingPreference

  const planPurchaseCountMap = useMemo(() => {
    const map = new Map<number, number>()
    for (const sub of allSubscriptions) {
      const planId = sub?.subscription?.plan_id
      if (!planId) continue
      map.set(planId, (map.get(planId) || 0) + 1)
    }
    return map
  }, [allSubscriptions])

  useEffect(() => {
    if (isCurrent()) onAvailabilityChange?.(isAvailable)
  }, [isAvailable, onAvailabilityChange, isCurrent])

  const planTitleMap = useMemo(() => {
    const map = new Map<number, string>()
    for (const p of plans) {
      if (p?.plan?.id) {
        map.set(p.plan.id, p.plan.title || '')
      }
    }
    return map
  }, [plans])

  const getRemainingDays = (sub: UserSubscriptionRecord) => {
    const endTime = sub?.subscription?.end_time || 0
    if (!endTime) return 0
    const now = Date.now() / 1000
    return Math.max(0, Math.ceil((endTime - now) / 86400))
  }

  const getUsagePercent = (sub: UserSubscriptionRecord) => {
    const total = Number(sub?.subscription?.amount_total || 0)
    const used = Number(sub?.subscription?.amount_used || 0)
    if (total <= 0) return 0
    return Math.round((used / total) * 100)
  }

  const renderSubscription = (sub: UserSubscriptionRecord) => {
    const subscription = sub.subscription
    const totalKnown = typeof subscription.amount_total === 'number' && Number.isFinite(subscription.amount_total) && subscription.amount_total >= 0
    const usedKnown = typeof subscription.amount_used === 'number' && Number.isFinite(subscription.amount_used) && subscription.amount_used >= 0
    const endKnown = typeof subscription.end_time === 'number' && Number.isFinite(subscription.end_time)
    const quotaKnown = totalKnown && (subscription.amount_total === 0 || usedKnown)
    const totalAmount = Number(subscription?.amount_total || 0)
    const usedAmount = Number(subscription?.amount_used || 0)
    const remainAmount =
      totalAmount > 0 ? Math.max(0, totalAmount - usedAmount) : 0
    const planTitle =
      planTitleMap.get(subscription?.plan_id) || ''
    const remainDays = endKnown ? getRemainingDays(sub) : 0
    const usagePercent = quotaKnown ? getUsagePercent(sub) : 0
    const now = Date.now() / 1000
    const isExpired = (subscription?.end_time || 0) < now
    const isCancelled = subscription?.status === 'cancelled'
    const isActive =
      endKnown && subscription?.status === 'active' && !isExpired
    const nextResetTime = subscription?.next_reset_time ?? 0
    let statusBadge = (
      <StatusBadge
        label={t('Expired')}
        variant='neutral'
        copyable={false}
      />
    )
    if (isActive) {
      statusBadge = (
        <StatusBadge
          label={t('Active')}
          variant='success'
          copyable={false}
        />
      )
    } else if (isCancelled) {
      statusBadge = (
        <StatusBadge
          label={t('Cancelled')}
          variant='neutral'
          copyable={false}
        />
      )
    }

    if ((subscription.status === 'active' && !endKnown) || !['active', 'cancelled', 'expired'].includes(subscription.status)) statusBadge = <StatusBadge label={t('Unknown')} variant='neutral' copyable={false} />
    let endTimeLabel = t('Expired at')
    if (isCancelled) {
      endTimeLabel = t('Cancelled at')
    } else if (isActive) {
      endTimeLabel = t('Until')
    } else if (!endKnown && subscription.status !== 'expired') {
      endTimeLabel = t('Validity')
    }

    return (
      <div
        key={subscription?.id}
        data-wallet-subscription-record={String(subscription.id)}
        className='bg-background rounded-md border p-3 text-xs'
      >
        <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'>
          <div className='flex min-w-0 flex-wrap items-center gap-2'>
            <span className='min-w-0 font-medium [overflow-wrap:anywhere]'>
              {planTitle
                ? `${planTitle} · ${t('Subscription')} #${subscription?.id}`
                : `${t('Subscription')} #${subscription?.id}`}
            </span>
            {statusBadge}
          </div>
          {isActive && (
            <span className='text-muted-foreground'>
              {t('{{count}} days remaining', { count: remainDays })}
            </span>
          )}
        </div>
        <div className='text-muted-foreground mt-1.5'>
          {endTimeLabel}{' '}
          {endKnown ? new Date(subscription.end_time * 1000).toLocaleString() : t('Unknown')}
        </div>
        {isActive && nextResetTime > 0 && (
          <div className='text-muted-foreground mt-1'>
            {t('Next reset')}:{' '}
            {new Date(nextResetTime * 1000).toLocaleString()}
          </div>
        )}
        <div className='text-muted-foreground mt-1'>
          {t('Total Quota')}:{' '}
          {!totalKnown && t('Unknown')}
          {totalKnown && totalAmount > 0 && !usedKnown && <>{formatQuota(totalAmount)} · {t('Used')} {t('Unknown')} · {t('Remaining')} {t('Unknown')}</>}
          {quotaKnown && totalAmount > 0 && (
            <Tooltip>
              <TooltipTrigger render={<span className='cursor-help' />}>
                {formatQuota(usedAmount)}/{formatQuota(totalAmount)} · {t('Remaining')}{' '}{formatQuota(remainAmount)}
              </TooltipTrigger>
              <TooltipContent>
                {t('Raw Quota')}: {usedAmount}/{totalAmount} ·{' '}{t('Remaining')} {remainAmount}
              </TooltipContent>
            </Tooltip>
          )}
          {quotaKnown && totalAmount <= 0 && t('Unlimited')}
          {quotaKnown && totalAmount > 0 && <span className='ml-2'>{t('Used')} {usagePercent}%</span>}
        </div>
        {quotaKnown && totalAmount > 0 && isActive && <Progress value={usagePercent} className='mt-2 h-1.5' />}
      </div>
    )
  }
  useEffect(() => {
    setPages((previous) => {
      const next = { ...previous }
      if (selfKnown) {
        next.current = Math.min(previous.current, Math.max(1, Math.ceil(activeSubscriptions.length / pageSize)))
        next.records = Math.min(previous.records, Math.max(1, Math.ceil(allSubscriptions.length / pageSize)))
      }
      if (plansKnown) next.plans = Math.min(previous.plans, Math.max(1, Math.ceil(plans.length / pageSize)))
      return next.current === previous.current && next.records === previous.records && next.plans === previous.plans ? previous : next
    })
  }, [selfKnown, plansKnown, activeSubscriptions.length, allSubscriptions.length, plans.length, pageSize])
  const returnPurchaseFocus = () => {
    if (!isCurrent()) return null
    const live = document.querySelector<HTMLElement>(`[data-wallet-subscription-plan="${selectedPlan?.plan.id}"]`)
    const original = purchaseOpener.current
    for (const node of [original, live]) {
      if (node?.isConnected && !node.closest('[hidden]') && !(node instanceof HTMLButtonElement && node.disabled)) return node
    }
    if (activeMode === 'plans' && catalogHeading.current?.isConnected && !catalogHeading.current.closest('[hidden]')) return catalogHeading.current
    return cardHeading.current
  }
  const forwardPurchaseSuccess = async () => { if (isCurrent()) await onPurchaseSuccess?.() }

  if (loading && !selfKnown && !selfError && !plansError) {
    return (
      <Card data-card-hover='false'>
        <CardHeader>
          <Skeleton className='h-5 w-32' />
        </CardHeader>
        <CardContent className='space-y-4'>
          <Skeleton className='h-20 w-full' />
          <div className='grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3'>
            {['first', 'second', 'third'].map((key) => (
              <Skeleton key={key} className='h-48 w-full' />
            ))}
          </div>
        </CardContent>
      </Card>
    )
  }

  if (!isAvailable) {
    return null
  }

  return (
    <>
      <Card data-card-hover='false'>
        <CardHeader>
          <CardTitle>
            <h2 ref={cardHeading} tabIndex={-1}>{t('Subscription Plans')}</h2>
          </CardTitle>
          <CardDescription>
            {t('Subscribe to a plan for model access')}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-4 sm:space-y-5'>
          {selfError && <Alert variant='destructive'><AlertDescription>{selfError}<Button type='button' variant='outline' className='min-h-11' disabled={refreshing || preferencePending} onClick={() => void fetchSelfSubscription()}>{t('Retry subscriptions')}</Button></AlertDescription></Alert>}
          {plansError && <Alert variant='destructive'><AlertDescription>{plansError}<Button type='button' variant='outline' className='min-h-11' disabled={plansReading} onClick={() => void fetchPlans()}>{t('Retry plans')}</Button></AlertDescription></Alert>}
          {unknownActiveFields && <p role='status' className='text-warning text-sm'>{t('Some current subscription fields are unknown. Inspect the records before relying on them.')}</p>}
          {preferenceError && <Alert variant='destructive'><AlertDescription>{preferenceError}</AlertDescription></Alert>}
          {!canPurchaseSubscription && <p className='text-muted-foreground text-sm'>{t('Subscription purchases are unavailable. Existing records remain readable.')}</p>}
          {/* My subscriptions & billing preference */}
          <div className='rounded-lg border p-3 sm:p-4'>
            <div className='flex flex-wrap items-center justify-between gap-2.5 sm:gap-3'>
              <div className='flex min-w-0 flex-wrap items-center gap-2'>
                <span className='text-sm font-medium'>
                  {t('My Subscriptions')}
                </span>
                <span className='flex items-center gap-1.5 text-xs font-medium'>
                  <span
                    className={cn(
                      'size-1.5 shrink-0 rounded-full',
                      selfKnown && hasActive ? dotColorMap.success : dotColorMap.neutral
                    )}
                    aria-hidden='true'
                  />
                  {!selfKnown && <span>{t('Unknown')}</span>}
                  {selfKnown && hasActive && (
                    <span className={cn(textColorMap.success)}>
                      {activeSubscriptions.length} {t('active')}
                    </span>
                  )}
                  {selfKnown && !hasActive && <span className='text-muted-foreground'>{t('No Active')}</span>}
                  {selfKnown && <span className='text-muted-foreground'>· {t('Loaded subscription records: {{count}}', { count: allSubscriptions.length })}</span>}
                </span>
              </div>
              <div className='flex w-full items-center gap-2 sm:w-auto'>
                <Select
                  items={[
                    {
                      value: 'subscription_first',
                      label: (
                        <>
                          {getBillingPreferenceLabel('subscription_first', t)}
                          {disablePref ? ` (${t('No Active')})` : ''}
                        </>
                      ),
                    },
                    {
                      value: 'wallet_first',
                      label: getBillingPreferenceLabel('wallet_first', t),
                    },
                    {
                      value: 'subscription_only',
                      label: (
                        <>
                          {getBillingPreferenceLabel('subscription_only', t)}
                          {disablePref ? ` (${t('No Active')})` : ''}
                        </>
                      ),
                    },
                    {
                      value: 'wallet_only',
                      label: getBillingPreferenceLabel('wallet_only', t),
                    },
                  ]}
                  value={displayPref}
                  disabled={!selfKnown || refreshing || preferencePending}
                  onValueChange={(v) => v !== null && handlePreferenceChange(v)}
                >
                  <SelectTrigger aria-label={t('Billing preference')} className='h-auto min-h-11 flex-1 whitespace-normal text-xs sm:w-[180px] sm:flex-none'>
                    <SelectValue>
                      {selfKnown ? getBillingPreferenceLabel(displayPref, t) : t('Unknown')}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      <SelectItem
                        value='subscription_first'
                        disabled={disablePref}
                      >
                        {getBillingPreferenceLabel('subscription_first', t)}
                        {disablePref ? ` (${t('No Active')})` : ''}
                      </SelectItem>
                      <SelectItem value='wallet_first'>
                        {getBillingPreferenceLabel('wallet_first', t)}
                      </SelectItem>
                      <SelectItem
                        value='subscription_only'
                        disabled={disablePref}
                      >
                        {getBillingPreferenceLabel('subscription_only', t)}
                        {disablePref ? ` (${t('No Active')})` : ''}
                      </SelectItem>
                      <SelectItem value='wallet_only'>
                        {getBillingPreferenceLabel('wallet_only', t)}
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon'
                  className='min-h-11 min-w-11'
                  onClick={handleRefresh}
                  disabled={refreshing || preferencePending}
                  aria-label={t('Refresh')}
                >
                  <RefreshCw
                    className={`h-3.5 w-3.5 ${refreshing ? 'animate-spin' : ''}`}
                  />
                </Button>
              </div>
            </div>

            {selfKnown && disablePref && isSubPref && (
              <p className='text-muted-foreground mt-2 text-xs'>
                {t(
                  'Preference saved as {{pref}}, but no active subscription. Wallet will be used automatically.',
                  {
                    pref:
                      billingPreference === 'subscription_only'
                        ? t('Subscription Only')
                        : t('Subscription First'),
                  }
                )}
              </p>
            )}

          </div>

          <Tabs value={activeMode} onValueChange={(value) => { if (value === 'current' || value === 'records' || (value === 'plans' && canPurchaseSubscription)) setMode(value) }}>
            <TabsList aria-label={t('Subscription workspace')} className='grid h-auto w-full grid-cols-3 group-data-horizontal/tabs:h-auto'>
              <TabsTrigger value='current' className='min-h-11 whitespace-normal'>{t('Current subscriptions')}</TabsTrigger>
              <TabsTrigger value='records' className='min-h-11 whitespace-normal'>{t('Subscription records')}</TabsTrigger>
              <TabsTrigger value='plans' disabled={!canPurchaseSubscription} className='min-h-11 whitespace-normal'>{t('Browse plans')}</TabsTrigger>
            </TabsList>
            {(['current', 'records'] as const).map((view) => {
              const entries = view === 'current' ? activeSubscriptions : allSubscriptions
              return <TabsContent key={view} value={view} keepMounted className='min-w-0 space-y-3'>
                {!selfKnown && <p role='status'>{refreshing ? t('Loading...') : t('Unknown')}</p>}
                {selfKnown && entries.length === 0 && <p>{view === 'current' ? t('No Active') : t('No subscription records')}</p>}
                {selfKnown && entries.slice((pages[view] - 1) * pageSize, pages[view] * pageSize).map(renderSubscription)}
                <LogPagination label={view === 'current' ? t('Current subscription pagination') : t('User subscription records')} page={pages[view]} pageSize={pageSize} total={selfKnown ? entries.length : undefined} busy={refreshing || preferencePending} onPageChange={(page) => setPages((previous) => ({ ...previous, [view]: page }))} onPageSizeChange={(size) => { setPageSize(size); setPages({ current: 1, records: 1, plans: 1 }) }} />
              </TabsContent>
            })}
            <TabsContent value='plans' keepMounted className='min-w-0 space-y-3'>
              <h3 ref={catalogHeading} tabIndex={-1} className='font-medium outline-none'>{t('Browse plans')}</h3>
              {!plansKnown && <p role='status'>{plansReading ? t('Loading...') : t('Unknown')}</p>}
          {/* Available plans grid */}
          {canPurchaseSubscription &&
            (plansKnown && plans.length > 0 ? (
              <div className='grid grid-cols-1 gap-3 2xl:grid-cols-2 2xl:gap-4'>
                {plans.slice((pages.plans - 1) * pageSize, pages.plans * pageSize).map((p, index) => {
                  const plan = p?.plan
                  if (!plan) return null
                  const totalKnown = typeof plan.total_amount === 'number' && Number.isFinite(plan.total_amount) && plan.total_amount >= 0
                  const priceKnown = typeof plan.price_amount === 'number' && Number.isFinite(plan.price_amount) && plan.price_amount >= 0
                  const limitKnown = typeof plan.max_purchase_per_user === 'number' && Number.isSafeInteger(plan.max_purchase_per_user) && plan.max_purchase_per_user >= 0
                  const totalAmount = Number(plan.total_amount || 0)
                  const price = priceKnown ? Number(plan.price_amount).toFixed(2) : t('Unknown')
                  const isPopular = (pages.plans - 1) * pageSize + index === 0 && plans.length > 1
                  const limit = Number(plan.max_purchase_per_user || 0)
                  const count = planPurchaseCountMap.get(plan.id) || 0
                  const quoteKnown = Number.isSafeInteger(plan.id) && plan.id > 0 && totalKnown && priceKnown && limitKnown && selfKnown && (limit === 0 || purchaseCountsKnown) && !refreshing && !preferencePending
                  const reached = limit > 0 && count >= limit
                  let quotaBenefit = `${t('Total Quota')}: ${t('Unknown')}`
                  if (totalKnown) quotaBenefit = `${t('Total Quota')}: ${totalAmount > 0 ? formatQuota(totalAmount) : t('Unlimited')}`
                  let limitBenefit: string | null = `${t('Purchase Limit')}: ${t('Unknown')}`
                  if (limitKnown) limitBenefit = limit > 0 ? `${t('Purchase Limit')}: ${limit}` : null

                  const benefits = [
                    `${t('Validity Period')}: ${formatDuration(plan, t)}`,
                    formatResetPeriod(plan, t) !== t('No Reset')
                      ? `${t('Quota Reset')}: ${formatResetPeriod(plan, t)}`
                      : null,
                    quotaBenefit,
                    limitBenefit,
                    plan.upgrade_group
                      ? `${t('Upgrade Group')}: ${plan.upgrade_group}`
                      : null,
                  ].filter(Boolean) as string[]

                  return (
                    <Card
                      key={plan.id}
                      data-card-hover='false'
                      className={cn(isPopular && 'border-primary')}
                    >
                      <CardContent className='flex h-full flex-col'>
                        <div className='mb-2 flex items-start justify-between gap-3'>
                          <div className='min-w-0'>
                            <h3 className='font-semibold break-all'>
                              {plan.title || t('Subscription Plans')}
                            </h3>
                            {plan.subtitle && (
                              <p className='text-muted-foreground whitespace-normal break-all text-xs'>
                                {plan.subtitle}
                              </p>
                            )}
                          </div>
                          {isPopular && (
                            <StatusBadge
                              variant='info'
                              copyable={false}
                              className='shrink-0'
                            >
                              <Sparkles className='h-3 w-3' />
                              {t('Recommended')}
                            </StatusBadge>
                          )}
                        </div>

                        <div className='py-2'>
                          <span className='text-2xl font-semibold tabular-nums'>
                            {priceKnown ? `$${price}` : t('Unknown')}
                          </span>
                        </div>

                        <div className='flex-1 space-y-1.5 pb-3'>
                          {benefits.map((label) => (
                            <div
                              key={label}
                              className='text-muted-foreground flex items-center gap-2 text-xs'
                            >
                              <Check className='text-primary h-3 w-3 shrink-0' />
                              <span>{label}</span>
                            </div>
                          ))}
                        </div>

                        <Separator className='mb-3' />

                        {!quoteKnown && <Button type='button' disabled className='min-h-11 w-full'>{t('Quote unavailable')}</Button>}
                        {quoteKnown && reached && (
                          <Tooltip>
                            <TooltipTrigger render={<div />}>
                              <Button
                                variant='outline'
                                className='min-h-11 w-full'
                                disabled
                              >
                                {t('Limit Reached')}
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent>
                              {t('Purchase limit reached')} ({count}/{limit})
                            </TooltipContent>
                          </Tooltip>
                        )}
                        {quoteKnown && !reached && (
                          <Button
                            variant='outline'
                            className='min-h-11 w-full'
                            data-wallet-subscription-plan={String(plan.id)}
                            onClick={(event) => {
                              if (!isCurrent() || !canPurchaseSubscription || !quoteKnown) return
                              purchaseOpener.current = event.currentTarget
                              setSelectedPlan(p)
                              setPurchaseOpen(true)
                            }}
                          >
                            {t('Subscribe Now')}
                          </Button>
                        )}
                      </CardContent>
                    </Card>
                  )
                })}
              </div>
            ) : (
              <p className='text-muted-foreground py-4 text-center text-sm'>
                {plansKnown ? t('No plans available') : t('Unknown')}
              </p>
            ))}
              <LogPagination label={t('Subscription plan pagination')} page={pages.plans} pageSize={pageSize} total={plansKnown ? plans.length : undefined} busy={plansReading || preferencePending} onPageChange={(page) => setPages((previous) => ({ ...previous, plans: page }))} onPageSizeChange={(size) => { setPageSize(size); setPages({ current: 1, records: 1, plans: 1 }) }} />
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>

      {canPurchaseSubscription ? (
        <SubscriptionPurchaseDialog
          open={purchaseOpen}
          returnFocus={returnPurchaseFocus}
          onOpenChange={(open) => {
            if (!isCurrent()) return
            setPurchaseOpen(open)
            if (!open) {
              fetchSelfSubscription()
            }
          }}
          plan={selectedPlan}
          enableStripe={enableStripe}
          enableCreem={enableCreem}
          enableWaffoPancake={enableWaffoPancake}
          enableOnlineTopUp={enableOnlineTopUp}
          epayMethods={epayMethods}
          userQuota={userQuota}
          onPurchaseSuccess={forwardPurchaseSuccess}
          purchaseLimit={
            selectedPlan?.plan?.max_purchase_per_user
              ? Number(selectedPlan.plan.max_purchase_per_user)
              : undefined
          }
          purchaseCount={
            selectedPlan?.plan?.id
              ? planPurchaseCountMap.get(selectedPlan.plan.id)
              : undefined
          }
        />
      ) : null}
    </>
  )
}
