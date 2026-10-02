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
import { UsageReviewForm } from '@/features/usage-logs/components/usage-review-form'
import { useAuthStore } from '@/stores/auth-store'

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
          <DialogTitle>{t('Strict Token budget')}</DialogTitle>
          <DialogDescription>
            {t(
              'Actual input plus output tokens. Cache and reasoning are already included.'
            )}
          </DialogDescription>
        </DialogHeader>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Only official OpenAI Responses text requests with explicit max_output_tokens and per-token pricing are supported.'
          )}
        </p>
        <details className='text-xs'>
          <summary className='cursor-pointer'>
            {t('Supported request fields')}
          </summary>
          <p className='mt-2 font-mono break-words'>
            model, input, instructions, max_output_tokens, stream, store,
            service_tier
          </p>
          <p className='mt-2'>
            {t(
              'Tools, images, external context, free or per-call pricing, and subscription channels are not supported by this strict mode.'
            )}
          </p>
        </details>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Existing quota limits remain active. This is not a fee or account-percentage budget.'
          )}
        </p>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Only requests admitted while this budget is enabled are counted. Historical usage is not backfilled.'
          )}
        </p>
        {query.isError && <p role='alert'>{t('Operation failed')}</p>}
        {!data && !query.isError && <p role='status'>{t('Loading...')}</p>}
        {data && (
          <>
            <p className='text-sm'>
              {data.policy.enabled ? t('Enabled') : t('Disabled')}
            </p>
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
                <dt>{t('Token total limit')}</dt>
                <dd className='font-medium break-all'>
                  {data.policy.limit.toLocaleString()}
                </dd>
              </div>
            </dl>
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
