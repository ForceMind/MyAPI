import { zodResolver } from '@hookform/resolvers/zod'
import { useId, useState } from 'react'
import { useForm, type UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'

import {
  policyCandidate,
  policyFormDefaults,
  policyFormSchema,
  type PolicyFormValues,
} from '../lib/policy-form'
import type { AssignedPolicy, PolicyCandidate } from '../types'

function PolicyDimension(props: {
  name: 'public' | 'upstream' | 'channels'
  label: string
  form: UseFormReturn<PolicyFormValues>
  disabled: boolean
  onChange: () => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const inherited = props.form.watch(`${props.name}.inherit`)
  const error = props.form.formState.errors[props.name]?.text
  let errorMessage = t(
    'Enter positive channel IDs separated by commas or new lines.'
  )
  if (error?.message === 'scope_limit') {
    errorMessage = t('Each scope supports at most 128 values.')
  }
  if (error?.message === 'invalid_models') {
    errorMessage = t(
      'Model names must be at most 255 UTF-8 bytes and contain no control characters.'
    )
  }
  return (
    <fieldset
      className='min-w-0 space-y-2 rounded-md border p-3'
      disabled={props.disabled}
    >
      <legend className='px-1 font-medium'>{props.label}</legend>
      <FieldGroup className='gap-3'>
        <Field orientation='horizontal' data-disabled={props.disabled}>
          <Checkbox
            id={`${id}-inherit`}
            checked={inherited}
            disabled={props.disabled}
            onCheckedChange={(checked) => {
              props.form.setValue(`${props.name}.inherit`, checked === true)
              props.onChange()
            }}
          />
          <FieldLabel htmlFor={`${id}-inherit`}>
            {t('Inherit existing scope')}
          </FieldLabel>
        </Field>
        {!inherited && (
          <Field data-invalid={!!error} data-disabled={props.disabled}>
            <FieldLabel htmlFor={`${id}-values`}>{props.label}</FieldLabel>
            <Textarea
              id={`${id}-values`}
              rows={3}
              disabled={props.disabled}
              aria-invalid={!!error}
              aria-describedby={`${id}-help`}
              className='min-w-0 resize-y font-mono break-all'
              {...props.form.register(`${props.name}.text`, {
                onChange: props.onChange,
              })}
            />
            <p id={`${id}-help`} className='text-muted-foreground text-xs'>
              {t(
                'Separate exact values with commas or new lines. An empty list denies this entire dimension.'
              )}
            </p>
            {error && <FieldError>{errorMessage}</FieldError>}
          </Field>
        )}
      </FieldGroup>
    </fieldset>
  )
}

export function AccessPolicyForm(props: {
  policy: AssignedPolicy
  busy: boolean
  locked: boolean
  onSave: (candidate: PolicyCandidate) => void
  onPreview: (candidate: PolicyCandidate) => void
  onRemove: () => void
  onChange: () => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const [confirmRemoval, setConfirmRemoval] = useState(false)
  const form = useForm<PolicyFormValues>({
    resolver: zodResolver(policyFormSchema),
    defaultValues: policyFormDefaults(props.policy),
  })
  const disabled = props.busy || props.locked
  return (
    <form
      className='min-w-0 space-y-4'
      onSubmit={form.handleSubmit((value) => {
        if (!disabled) props.onSave(policyCandidate(value))
      })}
    >
      <FieldGroup className='gap-4'>
        <Field orientation='horizontal' data-disabled={disabled}>
          <Checkbox
            id={`${id}-enabled`}
            checked={form.watch('enabled')}
            disabled={disabled}
            onCheckedChange={(value) => {
              form.setValue('enabled', value === true)
              props.onChange()
            }}
          />
          <FieldLabel htmlFor={`${id}-enabled`}>
            {t('Enable assigned access')}
          </FieldLabel>
        </Field>
        <p className='text-muted-foreground text-xs'>
          {t(
            'A disabled assignment denies all access. Inherited dimensions keep existing restrictions.'
          )}
        </p>
        <PolicyDimension
          name='public'
          label={t('Public model scope')}
          form={form}
          disabled={disabled}
          onChange={props.onChange}
        />
        <PolicyDimension
          name='upstream'
          label={t('Upstream model scope')}
          form={form}
          disabled={disabled}
          onChange={props.onChange}
        />
        <PolicyDimension
          name='channels'
          label={t('Channel ID scope')}
          form={form}
          disabled={disabled}
          onChange={props.onChange}
        />
      </FieldGroup>
      <div className='flex flex-wrap gap-2'>
        <Button
          type='button'
          variant='outline'
          disabled={disabled}
          onClick={() =>
            void form.handleSubmit((value) => {
              if (!disabled) props.onPreview(policyCandidate(value))
            })()
          }
        >
          {t('Preview access')}
        </Button>
        <Button type='submit' disabled={disabled}>
          {t('Save assignment')}
        </Button>
      </div>
      {props.policy.assigned && (
        <fieldset
          className='min-w-0 space-y-3 rounded-md border p-3'
          disabled={disabled}
        >
          <legend className='px-1 font-medium'>{t('Remove assignment')}</legend>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Removing this assignment may restore access allowed by legacy rules. Other restrictions remain active.'
            )}
          </p>
          <Field orientation='horizontal' data-disabled={disabled}>
            <Checkbox
              id={`${id}-remove`}
              checked={confirmRemoval}
              disabled={disabled}
              onCheckedChange={(value) => setConfirmRemoval(value === true)}
            />
            <FieldLabel htmlFor={`${id}-remove`}>
              {t(
                'I understand that removing this assignment may broaden access.'
              )}
            </FieldLabel>
          </Field>
          <Button
            type='button'
            variant='destructive'
            className='h-auto min-h-8 whitespace-normal'
            disabled={disabled || !confirmRemoval}
            onClick={() => {
              if (!disabled && confirmRemoval) props.onRemove()
            }}
          >
            {t('Remove assignment')}
          </Button>
        </fieldset>
      )}
    </form>
  )
}
