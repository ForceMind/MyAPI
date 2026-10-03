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
import { usdLessThan } from '@/lib/exact-usd'

import {
  tokenBudgetPolicySchema,
  tokenBudgetCancelSchema,
  type TokenBudgetPolicyValues,
  type TokenBudgetCancelValues,
} from '../lib/token-budget-schema'
import type { TokenBudgetView } from '../token-budget-api'

export function TokenBudgetPolicyForm(props: {
  policy: TokenBudgetView['policy']
  busy: boolean
  locked: boolean
  onSubmit: (value: TokenBudgetPolicyValues) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const form = useForm<TokenBudgetPolicyValues>({
    resolver: zodResolver(
      tokenBudgetPolicySchema
        .refine(
          (value) => !value.enabled || Number(value.limit) >= props.policy.used,
          { path: ['limit'], message: 'Limit is below confirmed usage' }
        )
        .refine(
          (value) =>
            !value.feeEnabled ||
            !usdLessThan(value.feeLimit, props.policy.fee_used_usd),
          { path: ['feeLimit'], message: 'Limit is below confirmed usage' }
        )
    ),
    defaultValues: {
      enabled: props.policy.enabled,
      accountThresholdEnabled: props.policy.account_threshold_enabled,
      minimumRemainingPercent: String(
        props.policy.account_min_remaining_bps / 100
      ),
      maxAgeSeconds: String(props.policy.account_max_age_seconds),
      feeEnabled: props.policy.fee_enabled,
      feeLimit: props.policy.fee_limit_usd,
      limit: String(props.policy.limit),
      confirmed: false,
    },
  })
  return (
    <form
      onSubmit={form.handleSubmit((value) => {
        if (!props.busy) props.onSubmit(value)
      })}
    >
      <FieldGroup>
        <Field orientation='horizontal'>
          <Checkbox
            id={`${id}-enabled`}
            checked={form.watch('enabled')}
            onCheckedChange={(value) => {
              form.setValue('enabled', value === true)
              void form.trigger('accountThresholdEnabled')
            }}
            disabled={props.busy || props.locked}
          />
          <FieldLabel htmlFor={`${id}-enabled`}>
            {t('Enable strict Token budget')}
          </FieldLabel>
        </Field>
        <Field>
          <FieldLabel htmlFor={`${id}-limit`}>
            {t('Token total limit')}
          </FieldLabel>
          <Input
            id={`${id}-limit`}
            inputMode='numeric'
            {...form.register('limit')}
            disabled={props.busy}
            readOnly={props.locked}
            aria-invalid={!!form.formState.errors.limit}
          />
          {form.formState.errors.limit && (
            <FieldError>{t('Please enter a valid number')}</FieldError>
          )}
        </Field>
        <Field orientation='horizontal'>
          <Checkbox
            id={`${id}-fee-enabled`}
            checked={form.watch('feeEnabled')}
            onCheckedChange={(value) => {
              form.setValue('feeEnabled', value === true)
              void form.trigger('accountThresholdEnabled')
            }}
            disabled={props.busy || props.locked}
          />
          <FieldLabel htmlFor={`${id}-fee-enabled`}>
            {t('Enable USD fee budget')}
          </FieldLabel>
        </Field>
        <Field>
          <FieldLabel htmlFor={`${id}-fee-limit`}>
            {t('USD total limit')}
          </FieldLabel>
          <Input
            id={`${id}-fee-limit`}
            inputMode='decimal'
            {...form.register('feeLimit')}
            disabled={props.busy}
            readOnly={props.locked}
            maxLength={128}
            aria-invalid={!!form.formState.errors.feeLimit}
          />
          {form.formState.errors.feeLimit && (
            <FieldError>{t('Please enter a valid number')}</FieldError>
          )}
        </Field>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Fee budgets require an exact reported model, Standard tier and explicit cache-read/write counts; missing evidence pauses this key.'
          )}
        </p>
        <p className='text-muted-foreground text-xs'>
          {t('Fee requests must explicitly set service_tier=default.')}
        </p>
        <fieldset className='min-w-0 space-y-3 rounded-md border p-3'>
          <legend className='px-1 text-sm'>
            {t('Account safety threshold')}
          </legend>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Native Codex only. Each account window must have fresh evidence above this threshold before dispatch. Missing, expired or reset observations block the account.'
            )}
          </p>
          <p className='text-muted-foreground text-xs'>
            {t(
              'This is not a per-key budget and cannot guarantee the remaining percentage after a request.'
            )}
          </p>
          <Field orientation='horizontal'>
            <Checkbox
              id={`${id}-threshold-enabled`}
              checked={form.watch('accountThresholdEnabled')}
              onCheckedChange={(value) => {
                form.setValue('accountThresholdEnabled', value === true)
                void form.trigger('accountThresholdEnabled')
              }}
              disabled={props.busy || props.locked}
            />
            <FieldLabel htmlFor={`${id}-threshold-enabled`}>
              {t('Enable account safety threshold')}
            </FieldLabel>
          </Field>
          {form.formState.errors.accountThresholdEnabled && (
            <FieldError>
              {t(
                'Account thresholds cannot be combined with Token or USD budgets on this key.'
              )}
            </FieldError>
          )}
          <Field>
            <FieldLabel htmlFor={`${id}-threshold-percent`}>
              {t('Minimum remaining percentage')}
            </FieldLabel>
            <Input
              id={`${id}-threshold-percent`}
              inputMode='decimal'
              maxLength={6}
              {...form.register('minimumRemainingPercent')}
              disabled={props.busy}
              readOnly={props.locked}
              aria-invalid={!!form.formState.errors.minimumRemainingPercent}
            />
            {form.formState.errors.minimumRemainingPercent && (
              <FieldError>
                {t('Use 0 to 100 percent with at most two decimal places.')}
              </FieldError>
            )}
          </Field>
          <Field>
            <FieldLabel htmlFor={`${id}-threshold-age`}>
              {t('Maximum observation age (seconds)')}
            </FieldLabel>
            <Input
              id={`${id}-threshold-age`}
              inputMode='numeric'
              maxLength={4}
              {...form.register('maxAgeSeconds')}
              disabled={props.busy}
              readOnly={props.locked}
              aria-invalid={!!form.formState.errors.maxAgeSeconds}
            />
            {form.formState.errors.maxAgeSeconds && (
              <FieldError>{t('Use 30 to 3600 seconds.')}</FieldError>
            )}
          </Field>
        </fieldset>
        <Field orientation='horizontal'>
          <Checkbox
            id={`${id}-confirmed`}
            checked={form.watch('confirmed')}
            onCheckedChange={(value) =>
              form.setValue('confirmed', value === true, {
                shouldValidate: true,
              })
            }
            disabled={props.busy || props.locked}
          />
          <FieldLabel htmlFor={`${id}-confirmed`}>
            {t(
              'I confirm all running instances support this budget and I understand its request restrictions.'
            )}
          </FieldLabel>
        </Field>
        {form.formState.errors.confirmed && (
          <FieldError>{t('Required')}</FieldError>
        )}
        <Button type='submit' disabled={props.busy}>
          {props.busy ? t('Processing...') : t('Save')}
        </Button>
      </FieldGroup>
    </form>
  )
}

export function TokenBudgetCancelForm(props: {
  busy: boolean
  locked: boolean
  onSubmit: (value: TokenBudgetCancelValues) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const form = useForm<TokenBudgetCancelValues>({
    resolver: zodResolver(tokenBudgetCancelSchema),
    defaultValues: { evidence: '', confirmed: false },
  })
  return (
    <form
      onSubmit={form.handleSubmit((value) => {
        if (!props.busy) props.onSubmit(value)
      })}
    >
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor={`${id}-evidence`}>
            {t('Evidence reference')}
          </FieldLabel>
          <Input
            id={`${id}-evidence`}
            {...form.register('evidence')}
            disabled={props.busy}
            readOnly={props.locked}
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
            onCheckedChange={(value) =>
              form.setValue('confirmed', value === true, {
                shouldValidate: true,
              })
            }
            disabled={props.busy || props.locked}
          />
          <FieldLabel htmlFor={`${id}-confirmed`}>
            {t(
              'I confirm the request was never dispatched. Release only its reservation.'
            )}
          </FieldLabel>
        </Field>
        {form.formState.errors.confirmed && (
          <FieldError>{t('Required')}</FieldError>
        )}
        <Button type='submit' disabled={props.busy}>
          {props.busy ? t('Processing...') : t('Cancel undispatched request')}
        </Button>
      </FieldGroup>
    </form>
  )
}
