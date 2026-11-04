/* Copyright (C) 2026 ForceMind. Licensed under AGPL-3.0-or-later. */
import { useQuery } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SummaryDisclosure } from '@/components/summary-disclosure'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import { useAuthStore } from '@/stores/auth-store'

import { confirmOperationsApprovalStatus, getOperationsPlan, listOperationsInstances, listOperationsPlans, OperationsError, operationsApprovalBlock, operationsSessionKey, unconfirmedOperationsApprovals, wasOperationsApprovalSubmitted, type OperationsPlan } from '../operations-api'
import { OperationsApprovalDialog } from './operations-approval-dialog'
import { OperationsPlanDetails } from './operations-plan-details'

export function OperationsUpgradePanel() {
  const scopeKey = useAuthStore(() => operationsSessionKey())
  if (!scopeKey) return null
  return <OperationsPanel key={scopeKey} scopeKey={scopeKey} />
}

function OperationsPanel(props: { scopeKey: string }) {
  const { t } = useTranslation()
  const [review, setReview] = useState<OperationsPlan | null>(null)
  const [unknown, setUnknown] = useState<Record<string, OperationsPlan>>(() => Object.fromEntries(unconfirmedOperationsApprovals(props.scopeKey).map((plan) => [plan.id, plan])))
  const [recovering, setRecovering] = useState(false)
  const [feedback, setFeedback] = useState('')
  const trigger = useRef<HTMLElement | null>(null)
  const plans = useQuery({ queryKey: ['operations', 'plans', props.scopeKey], queryFn: ({ signal }) => listOperationsPlans(signal), retry: false, gcTime: 0, refetchOnWindowFocus: false })
  const instances = useQuery({ queryKey: ['operations', 'instances', props.scopeKey], queryFn: ({ signal }) => listOperationsInstances(signal), retry: false, gcTime: 0, refetchOnWindowFocus: false })
  const unconfigured = (plans.error instanceof OperationsError && plans.error.kind === 'unavailable') || (instances.error instanceof OperationsError && instances.error.kind === 'unavailable')
  const sorted = [...(plans.data ?? [])].sort((a, b) => b.created_at - a.created_at)
  const terminalStates = new Set(['succeeded', 'failed_before_start'])
  const active = sorted.filter((plan) => !terminalStates.has(plan.state))
  const recentCompleted = sorted.filter((plan) => terminalStates.has(plan.state)).slice(0, 10)
  const visible = [...active, ...recentCompleted]
  const riskPlans = sorted.filter((plan) => plan.state === 'recovery_required' || plan.state === 'interrupted')
  const missingUnknown = Object.values(unknown).filter((plan) => !visible.some((candidate) => candidate.id === plan.id))

  async function recover(plan: OperationsPlan) {
    if (recovering || operationsSessionKey() !== props.scopeKey) return
    setRecovering(true)
    setFeedback('')
    try {
      const current = await getOperationsPlan(plan.id, new AbortController().signal)
      if (operationsSessionKey() !== props.scopeKey) return
      if (operationsApprovalBlock(plan, current) === 'changed') {
        setFeedback(t('The returned plan no longer matches the reviewed digest. Keep this result unknown and inspect the operations service.'))
      } else if (current.state !== 'prepared') {
        confirmOperationsApprovalStatus(props.scopeKey, plan.id)
        setUnknown((previous) => {
          const remaining = { ...previous }
          delete remaining[plan.id]
          return remaining
        })
        setFeedback(t('Plan status checked. No approval or execution request was replayed.'))
        void plans.refetch()
      } else {
        setFeedback(t('The plan is still prepared. The previous approval result remains unknown; do not submit it again.'))
      }
    } catch {
      if (operationsSessionKey() === props.scopeKey) setFeedback(t('Plan status could not be checked. Approval remains unknown and will not be replayed.'))
    } finally {
      if (operationsSessionKey() === props.scopeKey) setRecovering(false)
    }
  }

  return <Card data-card-hover='false'>
    <CardHeader><CardTitle><h2>{t('Upgrade approvals')}</h2></CardTitle><CardDescription>{t('Review upgrade plans prepared by trusted operations tools. Approval and execution are separate actions.')}</CardDescription></CardHeader>
    <CardContent className='min-w-0 space-y-4'>
      <Button type='button' variant='outline' disabled={plans.isFetching || instances.isFetching || recovering} onClick={() => { void plans.refetch(); void instances.refetch() }}>{t('Refresh upgrade plans')}</Button>
      {unconfigured && <Alert><AlertTitle>{t('Operations service not configured')}</AlertTitle><AlertDescription>{t('Configure the restricted operations service before preparing or approving upgrades. Existing system status remains available.')}</AlertDescription></Alert>}
      {!unconfigured && (plans.isError || instances.isError) && <Alert role='alert'><AlertDescription>{t('Upgrade information could not be loaded. Its current state is unknown. Refresh to check your session and service connection.')}</AlertDescription></Alert>}
      {(plans.isPending || instances.isPending) && <p role='status'>{t('Loading upgrade information...')}</p>}
      {feedback && <p role='status' className='text-sm [overflow-wrap:anywhere]'>{feedback}</p>}
      {riskPlans.length > 0 && <Alert variant='destructive' role='alert'><AlertTitle>{t('Upgrade recovery needs attention')}</AlertTitle><AlertDescription>{t('An interrupted upgrade or recovery-required plan is present. Inspect backup and target-start markers before any further operation.')}<ul className='mt-2 space-y-1'>{riskPlans.map((plan) => <li key={plan.id} className='[overflow-wrap:anywhere]'>{plan.instance_id}: <code>{plan.id}</code> — <code>{plan.state}</code></li>)}</ul></AlertDescription></Alert>}
      {Object.values(unknown).map((plan) => <Alert key={plan.id} role='alert'><AlertTitle>{t('Approval result unknown')}</AlertTitle><AlertDescription><p className='[overflow-wrap:anywhere]'>{plan.instance_id}: <code>{plan.id}</code></p><p>{t('Approval result unknown. Inspect the plan with a read-only status check; this approval will not be replayed.')}</p><Button type='button' className='mt-2' variant='outline' disabled={recovering} onClick={() => void recover(plan)}>{t('Check approval status')}</Button></AlertDescription></Alert>)}
      {instances.data && <SummaryDisclosure summary={t('Operations instances ({{count}})', { count: instances.data.length })}><ul className='space-y-3'>{instances.data.map((instance) => <li className='min-w-0 space-y-1 text-sm [overflow-wrap:anywhere]' key={instance.id}><p>{instance.id} — {instance.state}</p><p>{t('Current image ID')}: <code>{instance.current_image_id || t('Unknown')}</code></p><p>{t('Current container ID')}: <code>{instance.current_container_id || t('Unknown')}</code></p></li>)}</ul></SummaryDisclosure>}
      {plans.isSuccess && visible.length === 0 && missingUnknown.length === 0 && <Empty><EmptyHeader><EmptyTitle>{t('No upgrade plans returned.')}</EmptyTitle></EmptyHeader></Empty>}
      {(visible.length > 0 || missingUnknown.length > 0) && <>
        <p className='text-muted-foreground text-sm'>{t('Showing returned active plans and the latest 10 completed plans. This is not a complete upgrade history.')}</p>
        <ul aria-label={t('Upgrade plan list')} className='space-y-3'>
          {[...missingUnknown, ...visible].map((plan) => <li key={plan.id} className='min-w-0 space-y-3 rounded-lg border p-3'>
            <div className='flex flex-wrap items-center justify-between gap-2'><p className='min-w-0 text-sm font-medium [overflow-wrap:anywhere]'>{plan.source_version && <>{plan.source_version} → </>}{plan.version} · {plan.instance_id}</p><Badge variant={plan.state === 'recovery_required' || plan.state === 'interrupted' ? 'destructive' : 'outline'}>{plan.state}</Badge></div>
            <SummaryDisclosure summary={t('Upgrade plan details')}><OperationsPlanDetails plan={plan} /></SummaryDisclosure>
            <Button type='button' variant='outline' disabled={wasOperationsApprovalSubmitted(props.scopeKey, plan.id) || recovering} onClick={(event) => { if (operationsSessionKey() !== props.scopeKey) return; trigger.current = event.currentTarget; setReview(plan) }}>{t('Review upgrade approval')}</Button>
          </li>)}
        </ul>
      </>}
      {review && <OperationsApprovalDialog plan={review} scopeKey={props.scopeKey} onClose={() => setReview(null)} returnFocus={() => trigger.current} onResult={() => { void plans.refetch() }} onUnknown={(plan) => { setUnknown((previous) => ({ ...previous, [plan.id]: plan })) }} />}
    </CardContent>
  </Card>
}
