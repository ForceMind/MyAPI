import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { useAuthStore } from '@/stores/auth-store'

import {
  getUsageReview,
  reconcileUsageReview,
  type UsageReview,
} from '../usage-review-api'

const formSchema = z.object({
  amount: z
    .string()
    .regex(/^\d+$/)
    .refine((value) => Number(value) <= 2147483647),
  evidence: z.string().trim().min(1).max(2048),
  confirmed: z.boolean().refine((value) => value),
})

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
    mutationFn: (values: z.infer<typeof formSchema>) =>
      reconcileUsageReview(
        props.requestId,
        Number(values.amount),
        values.evidence
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
  const resolved = query.data.actual_quota !== null
  const canResolve =
    props.role === 100 &&
    ['usage_unknown', 'review_pending'].includes(query.data.state)
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
      {resolved && (
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

function UsageReviewForm(props: {
  review: UsageReview
  busy: boolean
  onSubmit: (values: z.infer<typeof formSchema>) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      amount: props.review.decision
        ? String(props.review.decision.actual_quota)
        : '',
      evidence: props.review.decision?.evidence_reference ?? '',
      confirmed: false,
    },
  })
  return (
    <form
      onSubmit={form.handleSubmit((values) => {
        if (!props.busy) props.onSubmit(values)
      })}
    >
      <FieldGroup>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Use verified usage and the frozen request price. Do not enter an estimate or credentials.'
          )}
        </p>
        <Field>
          <FieldLabel htmlFor={`${id}-amount`}>
            {t('Confirmed quota (internal units)')}
          </FieldLabel>
          <Input
            id={`${id}-amount`}
            inputMode='numeric'
            {...form.register('amount')}
            disabled={props.busy}
            readOnly={!!props.review.decision}
            aria-invalid={!!form.formState.errors.amount}
          />
          {form.formState.errors.amount && (
            <FieldError>{t('Please enter a valid number')}</FieldError>
          )}
        </Field>
        <Field>
          <FieldLabel htmlFor={`${id}-evidence`}>
            {t('Evidence reference')}
          </FieldLabel>
          <Input
            id={`${id}-evidence`}
            {...form.register('evidence')}
            disabled={props.busy}
            readOnly={!!props.review.decision}
            maxLength={2048}
            autoComplete='off'
            aria-invalid={!!form.formState.errors.evidence}
          />
          {form.formState.errors.evidence && (
            <FieldError>{t('Required')}</FieldError>
          )}
        </Field>
        <Field orientation='horizontal'>
          <Checkbox
            id={`${id}-confirmed`}
            checked={form.watch('confirmed')}
            onCheckedChange={(checked) =>
              form.setValue('confirmed', checked === true, {
                shouldValidate: true,
              })
            }
            disabled={props.busy}
          />
          <FieldLabel htmlFor={`${id}-confirmed`}>
            {t('I verified the evidence and frozen pricing.')}
          </FieldLabel>
        </Field>
        {form.formState.errors.confirmed && (
          <FieldError>{t('Required')}</FieldError>
        )}
        <Button type='submit' disabled={props.busy}>
          {props.busy ? t('Processing...') : t('Confirm reconciliation')}
        </Button>
      </FieldGroup>
    </form>
  )
}
