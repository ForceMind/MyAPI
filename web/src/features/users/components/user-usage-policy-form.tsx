import { zodResolver } from '@hookform/resolvers/zod'
import { useId } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'

import {
  userUsagePolicySchema,
  type UserUsagePolicyValues,
} from '../lib/user-usage-policy-schema'
import type { UserUsagePolicy } from '../user-usage-policy-api'

export function UserUsagePolicyForm(props: {
  policy: UserUsagePolicy
  busy: boolean
  locked: boolean
  onSubmit: (noBalance: boolean) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const form = useForm<UserUsagePolicyValues>({
    resolver: zodResolver(userUsagePolicySchema),
    defaultValues: { noBalance: props.policy.no_balance, confirmed: false },
  })
  return (
    <form
      className='space-y-4'
      onSubmit={form.handleSubmit((value) => {
        if (!props.busy) props.onSubmit(value.noBalance)
      })}
    >
      <Field orientation='horizontal'>
        <Checkbox
          id={`${id}-no-balance`}
          checked={form.watch('noBalance')}
          disabled={props.busy || props.locked}
          onCheckedChange={(value) =>
            form.setValue('noBalance', value === true)
          }
        />
        <FieldLabel htmlFor={`${id}-no-balance`}>
          {t('Use Key limits without a user wallet')}
        </FieldLabel>
      </Field>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Turning this off restores the stored user allowance, which may be zero.'
        )}
      </p>
      <Field orientation='horizontal'>
        <Checkbox
          id={`${id}-confirmed`}
          checked={form.watch('confirmed')}
          disabled={props.busy || props.locked}
          onCheckedChange={(value) =>
            form.setValue('confirmed', value === true, { shouldValidate: true })
          }
        />
        <FieldLabel htmlFor={`${id}-confirmed`}>
          {t(
            'I confirm all running instances support this policy and I understand the supported request paths.'
          )}
        </FieldLabel>
      </Field>
      {form.formState.errors.confirmed && (
        <FieldError>{t('Required')}</FieldError>
      )}
      <Button type='submit' disabled={props.busy}>
        {props.busy ? t('Processing...') : t('Save')}
      </Button>
    </form>
  )
}
