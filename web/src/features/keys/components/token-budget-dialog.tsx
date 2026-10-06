import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { TokenBudgetEvidence } from '@/features/usage-logs/components/token-budget-evidence'
import { UsageReviewForm } from '@/features/usage-logs/components/usage-review-form'
import { useAuthStore } from '@/stores/auth-store'

import { percentageToBasisPoints } from '../lib/token-budget-schema'
import {
  createTokenBudgetOperationId,
  getTokenBudget,
  writeTokenBudget,
  type TokenBudgetCommand,
} from '../token-budget-api'
import {
  TokenBudgetCancelForm,
  TokenBudgetPolicyForm,
} from './token-budget-forms'

export function TokenBudgetDialog(props: {
  tokenId: number
  onClose: () => void
}) {
  const userId = useAuthStore((state) => state.auth.user?.id)
  const role = useAuthStore((state) => state.auth.user?.role)
  if (!userId) return null
  return (
    <TokenBudgetSession
      key={`${userId}:${role}:${props.tokenId}`}
      tokenId={props.tokenId}
      userId={userId}
      role={role}
      onClose={props.onClose}
    />
  )
}

function TokenBudgetSession(props: {
  tokenId: number
  userId: number
  role?: number
  onClose: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const queryKey = ['token-budget', props.userId, props.role, props.tokenId]
  const sending = useRef(false)
  const operation = useRef<TokenBudgetCommand | null>(null)
  const [locked, setLocked] = useState(false)
  const query = useQuery({
    queryKey,
    queryFn: ({ signal }) => getTokenBudget(props.tokenId, signal),
    enabled: !locked,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    refetchInterval: (state) =>
      !locked && state.state.data?.pending ? 5000 : false,
  })
  const mutation = useMutation({
    mutationFn: (command: TokenBudgetCommand) => {
      const user = useAuthStore.getState().auth.user
      if (user?.id !== props.userId || user.role !== props.role) {
        throw new Error('Session changed')
      }
      return writeTokenBudget(props.tokenId, command)
    },
    retry: false,
    onSuccess: (data) => {
      const user = useAuthStore.getState().auth.user
      if (user?.id !== props.userId || user.role !== props.role) return
      client.setQueryData(queryKey, data)
      operation.current = null
      setLocked(false)
    },
    onSettled: () => {
      sending.current = false
    },
  })
  const submit = (command: TokenBudgetCommand) => {
    if (sending.current) return
    if (!operation.current) operation.current = command
    sending.current = true
    setLocked(true)
    mutation.mutate(operation.current)
  }
  const data = query.data
  const pending = data?.pending
  const busy = mutation.isPending || query.isFetching
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !sending.current) props.onClose()
      }}
    >
      <DialogContent
        className='max-h-[85dvh] min-w-0 overflow-y-auto sm:max-w-xl'
        showCloseButton={!mutation.isPending}
      >
        <DialogHeader>
          <DialogTitle>{t('API Key usage budgets')}</DialogTitle>
          <DialogDescription>
            {t(
              'Actual input plus output tokens. Cache and reasoning are already included.'
            )}
          </DialogDescription>
        </DialogHeader>
        {!data?.policy.account_threshold_enabled && (
          <>
            <p className='text-muted-foreground text-xs'>
              {t('Official OpenAI Responses')}: max_output_tokens
            </p>
            <p className='text-muted-foreground text-xs'>
              {t('Official OpenAI Chat')} · {t('Exact Match')}: gpt-6.1-sol ·
              max_completion_tokens
            </p>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Chat reserves 1,050,000 total tokens for input and completion, including reasoning. This is a conservative bound, not measured usage or a tokenizer estimate. Even a small request can fail if the remaining budget cannot cover this bound.'
              )}
            </p>
            <details className='text-xs'>
              <summary className='cursor-pointer'>
                {t('Supported request fields')}
              </summary>
              <p className='mt-2 font-mono break-words'>
                Responses: model, input, instructions, max_output_tokens,
                stream, store, service_tier
              </p>
              <p className='mt-2 font-mono break-words'>
                Chat: model, messages, max_completion_tokens, n, stream,
                stream_options, store, service_tier
              </p>
              <p className='mt-2'>
                {t(
                  'Responses requires explicit max_output_tokens. Chat requires explicit max_completion_tokens from 1 to 128,000 and n omitted or 1; streaming requires stream_options.include_usage=true.'
                )}
              </p>
              <p className='mt-2'>
                {t(
                  'Chat USD budgets require service_tier=default, exact published short/long prices frozen at dispatch, and explicit cache-read and cache-write usage counters, including zero. Missing evidence retains the reservation for review.'
                )}
              </p>
              <p className='mt-2'>
                {t(
                  'Tools, images, external context, free or per-call pricing, and subscription channels are not supported by this strict mode.'
                )}
              </p>
            </details>
          </>
        )}
        <p className='text-muted-foreground text-xs'>
          {t('Existing quota limits remain active.')}
        </p>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Token counts are tracked while Token or USD budgets are enabled; USD costs only while fee budgets are enabled. Account thresholds do not track per-key usage. Earlier usage is not backfilled.'
          )}
        </p>
        {query.isError && <p role='alert'>{t('Operation failed')}</p>}
        {!data && !query.isError && <p role='status'>{t('Loading...')}</p>}
        {data && (
          <>
            <p className='text-sm'>
              {data.policy.enabled ||
              data.policy.fee_enabled ||
              data.policy.account_threshold_enabled
                ? t('Enabled')
                : t('Disabled')}
            </p>
            <section className='min-w-0 space-y-2 rounded-md border p-3 text-sm'>
              <p>
                {t('Account safety threshold')}:{' '}
                {data.policy.account_threshold_enabled
                  ? t('Enabled')
                  : t('Disabled')}
              </p>
              <dl className='grid grid-cols-2 gap-2'>
                <div>
                  <dt>{t('Minimum remaining percentage')}</dt>
                  <dd>{data.policy.account_min_remaining_bps / 100}%</dd>
                </div>
                <div>
                  <dt>{t('Maximum observation age (seconds)')}</dt>
                  <dd>{data.policy.account_max_age_seconds}</dd>
                </div>
              </dl>
              {data.policy.account_threshold_enabled && (
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'Native Codex only. Each account window must have fresh evidence above this threshold before dispatch. Missing, expired or reset observations block the account.'
                  )}
                </p>
              )}
            </section>
            <dl className='grid min-w-0 grid-cols-3 gap-2 text-sm'>
              <div className='min-w-0'>
                <dt>{t('Used tokens')}</dt>
                <dd className='font-medium break-all'>
                  {data.policy.used.toLocaleString()}
                </dd>
              </div>
              <div className='min-w-0'>
                <dt>{t('Reserved tokens')}</dt>
                <dd className='font-medium break-all'>
                  {data.policy.reserved.toLocaleString()}
                </dd>
              </div>
              <div className='min-w-0'>
                <dt>
                  {t('Token total limit')}
                  {!data.policy.enabled && ` (${t('Disabled')})`}
                </dt>
                <dd className='font-medium break-all'>
                  {data.policy.limit.toLocaleString()}
                </dd>
              </div>
            </dl>
            <dl className='grid min-w-0 grid-cols-3 gap-2 text-sm'>
              <div className='min-w-0'>
                <dt>{t('Used USD')}</dt>
                <dd className='font-medium break-all'>
                  {data.policy.fee_used_usd}
                </dd>
              </div>
              <div className='min-w-0'>
                <dt>{t('Reserved USD')}</dt>
                <dd className='font-medium break-all'>
                  {data.policy.fee_reserved_usd}
                </dd>
              </div>
              <div className='min-w-0'>
                <dt>
                  {t('USD total limit')}
                  {!data.policy.fee_enabled && ` (${t('Disabled')})`}
                </dt>
                <dd className='font-medium break-all'>
                  {data.policy.fee_limit_usd}
                </dd>
              </div>
            </dl>
            <p className='text-muted-foreground text-xs'>
              {t(
                'USD cost is calculated from actual classified tokens and the frozen published official price. It is not a provider invoice.'
              )}
            </p>
            {pending && (
              <section className='min-w-0 space-y-2 rounded-md border p-3'>
                <p className='font-medium'>
                  {pending.state === 'usage_unknown'
                    ? t('Usage pending review')
                    : t('Processing...')}
                </p>
                <p className='text-xs break-words'>
                  {t('Model')}: {pending.model_name}
                </p>
                <p className='font-mono text-xs break-all'>
                  {pending.request_id}
                </p>
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'Further requests remain paused until this reservation is resolved.'
                  )}
                </p>
                <TokenBudgetEvidence evidence={pending} />
                {props.role === 100 && pending.state === 'prepared' && (
                  <TokenBudgetCancelForm
                    key={pending.request_id}
                    busy={busy}
                    locked={locked}
                    onSubmit={(value) =>
                      submit({
                        kind: 'recover',
                        body: {
                          request_id: pending.request_id,
                          action: 'cancel_not_sent',
                          evidence_reference: value.evidence,
                          confirmed_reliable_evidence: true,
                        },
                      })
                    }
                  />
                )}
                {props.role === 100 &&
                  pending.state !== 'prepared' &&
                  data.review && (
                    <UsageReviewForm
                      key={`${pending.request_id}:${data.review.decision?.id ?? 0}`}
                      review={data.review}
                      requireTokenCounts
                      requireFee={pending.fee_enabled}
                      busy={busy}
                      locked={locked}
                      onSubmit={(value) =>
                        submit({
                          kind: 'recover',
                          body: {
                            request_id: pending.request_id,
                            action: 'reconcile',
                            evidence_reference: value.evidence,
                            confirmed_reliable_evidence: true,
                            actual_quota: Number(value.amount),
                            actual_input_tokens: Number(value.input),
                            actual_output_tokens: Number(value.output),
                            ...(value.requiresFee
                              ? { actual_fee_usd: value.feeUSD }
                              : {}),
                          },
                        })
                      }
                    />
                  )}
              </section>
            )}
            {props.role === 100 && !pending && (
              <TokenBudgetPolicyForm
                key={data.policy.revision}
                policy={data.policy}
                busy={busy}
                locked={locked}
                onSubmit={(value) =>
                  submit({
                    kind: 'policy',
                    body: {
                      id: createTokenBudgetOperationId(),
                      expected_revision: data.policy.revision,
                      enabled: value.enabled,
                      limit: Number(value.limit),
                      confirmed: true,
                      account_threshold: {
                        enabled: value.accountThresholdEnabled,
                        minimum_remaining_bps: percentageToBasisPoints(
                          value.minimumRemainingPercent
                        ),
                        max_age_seconds: Number(value.maxAgeSeconds),
                      },
                      fee: {
                        enabled: value.feeEnabled,
                        limit_usd: value.feeLimit,
                      },
                    },
                  })
                }
              />
            )}
            {props.role !== 100 && (
              <p className='text-muted-foreground text-xs'>
                {t('Only Root can change budgets or reconcile usage.')}
              </p>
            )}
          </>
        )}
        {mutation.isError && (
          <p role='alert' className='text-destructive text-sm'>
            {t(
              'The outcome may already be saved. Retry the same request or close and refresh.'
            )}
          </p>
        )}
        <div className='flex flex-wrap justify-end gap-2'>
          <Button
            variant='outline'
            disabled={busy || locked}
            onClick={() => {
              void query.refetch()
            }}
          >
            {t('Refresh')}
          </Button>
          <Button
            variant='outline'
            disabled={mutation.isPending}
            onClick={() => {
              if (!sending.current) props.onClose()
            }}
          >
            {t('Close')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
