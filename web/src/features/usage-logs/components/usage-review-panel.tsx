import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useAuthStore } from '@/stores/auth-store'

import type { UsageReviewFormValues } from '../lib/usage-review-schema'
import {
  canReconcileUsageReview,
  getUsageReview,
  reconcileUsageReview,
  recoverTextDispatchUsage,
  type UsageReview,
} from '../usage-review-api'
import { TokenBudgetEvidence } from './token-budget-evidence'
import { UsageReviewForm } from './usage-review-form'
import { UsageSettlementStatus } from './usage-settlement-status'

type UsageReviewOperation = {
  values: UsageReviewFormValues
  recover: boolean
  evidenceKey: string
  pricingEvidence?: string
}

function reviewEvidenceKey(review: UsageReview): string {
  return JSON.stringify([
    review.settlement_status,
    review.decision,
    review.actual_quota,
    review.review_metadata,
    !!review.token_budget,
    review.token_budget?.fee_enabled,
    review.token_budget?.actual_input,
    review.token_budget?.actual_output,
    review.token_budget?.actual_fee_usd,
  ])
}

function matchesReviewOperation(
  review: UsageReview,
  operation: UsageReviewOperation
): boolean {
  if (reviewEvidenceKey(review) === operation.evidenceKey) return true
  const decision = review.decision
  const values = operation.values
  const budget = review.token_budget
  // A newly visible decision may acknowledge the exact command whose response
  // was lost. Other changed evidence requires a new explicit confirmation.
  return (
    (!review.settlement_status || review.settlement_status === 'none') &&
    review.review_metadata === operation.pricingEvidence &&
    !!decision &&
    decision.actual_quota === Number(values.amount) &&
    (review.actual_quota === null ||
      review.actual_quota === Number(values.amount)) &&
    decision.evidence_reference === values.evidence &&
    values.requiresTokens === !!budget &&
    (!values.requiresTokens ||
      (decision.actual_input_tokens === Number(values.input) &&
        decision.actual_output_tokens === Number(values.output) &&
        (budget?.actual_input == null ||
          budget.actual_input === Number(values.input)) &&
        (budget?.actual_output == null ||
          budget.actual_output === Number(values.output)))) &&
    values.requiresFee === !!budget?.fee_enabled &&
    (!values.requiresFee ||
      (decision.actual_fee_usd === values.feeUSD &&
        (budget?.actual_fee_usd == null ||
          budget.actual_fee_usd === values.feeUSD)))
  )
}

export function UsageReviewPanel(props: { requestId: string }) {
  const userId = useAuthStore((state) => state.auth.user?.id)
  const role = useAuthStore((state) => state.auth.user?.role)
  return (
    <UsageReviewSession
      key={`${userId}:${role}:${props.requestId}`}
      requestId={props.requestId}
      userId={userId}
      role={role}
    />
  )
}

function UsageReviewSession(props: {
  requestId: string
  userId?: number
  role?: number
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const queryKey = ['usage-review', props.userId, props.role, props.requestId]
  const query = useQuery({
    queryKey,
    queryFn: ({ signal }) => getUsageReview(props.requestId, signal),
    enabled: !!props.userId,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const [locked, setLocked] = useState(false)
  const canResolve =
    props.role === 100 && !!query.data && canReconcileUsageReview(query.data)
  const operation = useRef<UsageReviewOperation | null>(null)
  const operationCurrent =
    !operation.current ||
    (!!query.data && matchesReviewOperation(query.data, operation.current))
  const mutation = useMutation({
    mutationFn: (values: UsageReviewFormValues) => {
      const actor = useAuthStore.getState().auth.user
      const current = client.getQueryData<UsageReview>(queryKey)
      if (
        actor?.id !== props.userId ||
        actor?.role !== 100 ||
        query.isError ||
        !current ||
        !canReconcileUsageReview(current)
      ) {
        throw new Error('Usage review is not available')
      }
      if (
        operation.current &&
        !matchesReviewOperation(current, operation.current)
      ) {
        operation.current = null
        setLocked(false)
        throw new Error('Usage review evidence changed')
      }
      operation.current ??= {
        values: { ...values },
        recover: !!current.can_recover_text_dispatch,
        evidenceKey: reviewEvidenceKey(current),
        pricingEvidence: current.review_metadata,
      }
      setLocked(true)
      const command = operation.current
      if (command.recover) {
        return recoverTextDispatchUsage(
          props.requestId,
          Number(command.values.amount),
          command.values.evidence
        )
      }
      return reconcileUsageReview(
        props.requestId,
        Number(command.values.amount),
        command.values.evidence,
        command.values.requiresTokens
          ? {
              input: Number(command.values.input),
              output: Number(command.values.output),
              ...(command.values.requiresFee
                ? { feeUSD: command.values.feeUSD }
                : {}),
            }
          : undefined
      )
    },
    retry: false,
    onSuccess: (data) => {
      const actor = useAuthStore.getState().auth.user
      if (actor?.id !== props.userId || actor?.role !== props.role) return
      client.setQueryData(queryKey, data)
      void client.invalidateQueries({
        queryKey: ['pending-usage-reviews', props.userId],
      })
      operation.current = null
      setLocked(false)
    },
    onError: () => {
      void client.invalidateQueries({ queryKey })
    },
  })
  useEffect(() => {
    if (query.data && (!canResolve || !operationCurrent)) {
      operation.current = null
      setLocked(false)
    }
  }, [query.data, canResolve, operationCurrent])
  if (!props.userId) return null
  if (query.isError) {
    return (
      <div role='alert' className='space-y-2 text-sm'>
        <p>{t('Operation failed')}</p>
        <Button
          variant='outline'
          onClick={() => {
            void query.refetch()
          }}
        >
          {t('Retry')}
        </Button>
      </div>
    )
  }
  if (!query.data) return <p role='status'>{t('Loading...')}</p>
  const tokenPending =
    !!query.data.token_budget && query.data.token_budget.state !== 'settled'
  const resolved = query.data.actual_quota !== null && !tokenPending
  const automatic =
    !!query.data.settlement_status && query.data.settlement_status !== 'none'
  return (
    <section
      className='min-w-0 space-y-3 rounded-md border p-3'
      aria-label={t('Usage pending review')}
    >
      {automatic ? (
        <UsageSettlementStatus status={query.data.settlement_status} />
      ) : (
        <p className='text-sm font-medium'>
          {resolved ? t('Reconciled') : t('Usage pending review')}
        </p>
      )}
      <p className='text-muted-foreground text-xs'>
        {t('Reserved quota (internal units)')}:{' '}
        {query.data.reserved_quota.toLocaleString()}
      </p>
      {query.data.actual_quota !== null && (
        <p className='text-sm'>
          {t('Confirmed quota (internal units)')}:{' '}
          {query.data.actual_quota?.toLocaleString()}
        </p>
      )}
      {query.data.token_budget && (
        <TokenBudgetEvidence evidence={query.data.token_budget} />
      )}
      {canResolve && query.data.review_metadata && (
        <details className='min-w-0 text-xs'>
          <summary>{t('Frozen pricing evidence')}</summary>
          <pre className='max-h-48 overflow-auto break-all whitespace-pre-wrap'>
            {query.data.review_metadata}
          </pre>
        </details>
      )}
      {canResolve && mutation.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Operation failed')}
        </p>
      )}
      {canResolve && (
        <UsageReviewForm
          key={reviewEvidenceKey(query.data)}
          review={query.data}
          locked={locked && operationCurrent}
          requireFinished={!!query.data.can_recover_text_dispatch}
          busy={mutation.isPending || query.isFetching}
          onSubmit={(values) => mutation.mutate(values)}
        />
      )}
      <Button
        variant='outline'
        disabled={mutation.isPending || query.isFetching}
        onClick={() => {
          void query.refetch()
        }}
      >
        {t('Refresh')}
      </Button>
    </section>
  )
}
