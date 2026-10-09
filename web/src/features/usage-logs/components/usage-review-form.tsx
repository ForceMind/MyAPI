import { zodResolver } from '@hookform/resolvers/zod'
import { useId, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'

import {
  usageReviewFormSchema,
  type UsageReviewFormValues,
} from '../lib/usage-review-schema'
import type { UsageReview } from '../usage-review-api'

export function UsageReviewForm(props: {
  review: UsageReview
  busy: boolean
  locked?: boolean
  requireTokenCounts?: boolean
  requireFee?: boolean
  requireFinished?: boolean
  onSubmit: (values: UsageReviewFormValues) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const [expanded, setExpanded] = useState(false)
  const requiresFee =
    props.requireFee ?? !!props.review.token_budget?.fee_enabled
  const feeUSD =
    props.review.decision?.actual_fee_usd ??
    props.review.token_budget?.actual_fee_usd
  const requiresTokens = props.requireTokenCounts ?? !!props.review.token_budget
  const input =
    props.review.decision?.actual_input_tokens ??
    props.review.token_budget?.actual_input
  const output =
    props.review.decision?.actual_output_tokens ??
    props.review.token_budget?.actual_output
  const form = useForm<UsageReviewFormValues>({
    resolver: zodResolver(usageReviewFormSchema),
    defaultValues: {
      amount:
        props.review.decision || props.review.actual_quota !== null
          ? String(
              props.review.decision?.actual_quota ?? props.review.actual_quota
            )
          : '',
      input: input == null ? '' : String(input),
      output: output == null ? '' : String(output),
      requiresTokens,
      requiresFee,
      feeUSD: feeUSD ?? '',
      evidence: props.review.decision?.evidence_reference ?? '',
      // A locked local operation was already confirmed before submission.
      // Keep that confirmation if a refreshed decision remounts its retry form.
      confirmed: !!props.locked,
    },
  })
  const locked = props.locked || !!props.review.decision
  let confirmation = t('I verified the evidence and frozen pricing.')
  if (props.requireFinished) {
    confirmation = t(
      'I verified that the request ended and the actual usage and frozen pricing are correct.'
    )
  }
  if (requiresTokens) {
    confirmation = t(
      'I verified that the request ended and the actual token counts and frozen pricing are correct.'
    )
  }
  if (requiresFee) {
    confirmation = t(
      'I verified that the request ended and the actual token counts, USD cost and frozen pricing are correct.'
    )
  }
  return (
    <div className='min-w-0 space-y-3'>
      <Button
        variant='outline'
        className='h-auto max-w-full text-left whitespace-normal'
        aria-expanded={expanded}
        aria-controls={`${id}-manual-review`}
        onClick={() => setExpanded(!expanded)}
      >
        {t('Advanced: manual reconciliation')}
      </Button>
      {expanded && (
        <form
          id={`${id}-manual-review`}
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
                readOnly={locked || props.review.actual_quota !== null}
                aria-invalid={!!form.formState.errors.amount}
              />
              {form.formState.errors.amount && (
                <FieldError>{t('Please enter a valid number')}</FieldError>
              )}
            </Field>
            {requiresTokens && (
              <div className='grid min-w-0 gap-3 sm:grid-cols-2'>
                <Field>
                  <FieldLabel htmlFor={`${id}-input`}>
                    {t('Confirmed input tokens')}
                  </FieldLabel>
                  <Input
                    id={`${id}-input`}
                    inputMode='numeric'
                    {...form.register('input')}
                    disabled={props.busy}
                    readOnly={locked || input != null}
                    aria-invalid={!!form.formState.errors.input}
                  />
                  {form.formState.errors.input && (
                    <FieldError>{t('Please enter a valid number')}</FieldError>
                  )}
                </Field>
                <Field>
                  <FieldLabel htmlFor={`${id}-output`}>
                    {t('Confirmed output tokens')}
                  </FieldLabel>
                  <Input
                    id={`${id}-output`}
                    inputMode='numeric'
                    {...form.register('output')}
                    disabled={props.busy}
                    readOnly={locked || output != null}
                    aria-invalid={!!form.formState.errors.output}
                  />
                  {form.formState.errors.output && (
                    <FieldError>{t('Please enter a valid number')}</FieldError>
                  )}
                </Field>
              </div>
            )}
            {requiresFee && (
              <Field>
                <FieldLabel htmlFor={`${id}-fee-usd`}>
                  {t('Confirmed API usage cost (USD)')}
                </FieldLabel>
                <Input
                  id={`${id}-fee-usd`}
                  inputMode='decimal'
                  {...form.register('feeUSD')}
                  disabled={props.busy}
                  readOnly={locked || feeUSD != null}
                  maxLength={128}
                  aria-invalid={!!form.formState.errors.feeUSD}
                />
                {form.formState.errors.feeUSD && (
                  <FieldError>{t('Please enter a valid number')}</FieldError>
                )}
              </Field>
            )}
            <Field>
              <FieldLabel htmlFor={`${id}-evidence`}>
                {t('Evidence reference')}
              </FieldLabel>
              <Input
                id={`${id}-evidence`}
                {...form.register('evidence')}
                disabled={props.busy}
                readOnly={locked}
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
                disabled={props.busy || !!props.locked}
              />
              <FieldLabel htmlFor={`${id}-confirmed`}>
                {confirmation}
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
      )}
    </div>
  )
}
