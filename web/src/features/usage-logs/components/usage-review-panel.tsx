import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useAuthStore } from '@/stores/auth-store'

import type { UsageReviewFormValues } from '../lib/usage-review-schema'
import { getUsageReview, reconcileUsageReview } from '../usage-review-api'
import { UsageReviewForm } from './usage-review-form'

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
  const mutation = useMutation({
    mutationFn: (values: UsageReviewFormValues) =>
      reconcileUsageReview(
        props.requestId,
        Number(values.amount),
        values.evidence,
        values.requiresTokens
          ? {
              input: Number(values.input),
              output: Number(values.output),
              ...(values.requiresFee ? { feeUSD: values.feeUSD } : {}),
            }
          : undefined
      ),
    retry: false,
    onSuccess: (data) => client.setQueryData(queryKey, data),
    onError: () => {
      void client.invalidateQueries({ queryKey })
    },
  })
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
  const canResolve =
    props.role === 100 &&
    (['usage_unknown', 'review_pending'].includes(query.data.state) ||
      tokenPending)
  return (
    <section
      className='min-w-0 space-y-3 rounded-md border p-3'
      aria-label={t('Usage pending review')}
    >
      <p className='text-sm font-medium'>
        {resolved ? t('Reconciled') : t('Usage pending review')}
      </p>
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
      {mutation.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Operation failed')}
        </p>
      )}
      {canResolve && (
        <UsageReviewForm
          key={query.data.decision?.id ?? 0}
          review={query.data}
          busy={mutation.isPending || query.isFetching}
          onSubmit={(values) => mutation.mutate(values)}
        />
      )}
    </section>
  )
}
