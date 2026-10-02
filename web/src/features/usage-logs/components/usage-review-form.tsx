import { zodResolver } from '@hookform/resolvers/zod'
import { useId } from 'react'
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
  onSubmit: (values: UsageReviewFormValues) => void
}) {
  const { t } = useTranslation()
  const id = useId()
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
      amount: props.review.decision
        ? String(props.review.decision.actual_quota)
        : '',
      input: input == null ? '' : String(input),
      output: output == null ? '' : String(output),
      requiresTokens,
      evidence: props.review.decision?.evidence_reference ?? '',
      confirmed: false,
    },
  })
  const locked = props.locked || !!props.review.decision
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
            readOnly={locked}
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
            {requiresTokens
              ? t(
                  'I verified that the request ended and the actual token counts and frozen pricing are correct.'
                )
              : t('I verified the evidence and frozen pricing.')}
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
