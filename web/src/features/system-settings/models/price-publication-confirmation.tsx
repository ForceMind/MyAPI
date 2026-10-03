import { zodResolver } from '@hookform/resolvers/zod'
import { useId } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'

const schema = z.object({ confirmed: z.boolean().refine(Boolean) })

export function PricePublicationConfirmation(props: {
  title: string
  before?: string
  after?: string
  pending: boolean
  onConfirm: () => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { confirmed: false },
  })
  return (
    <form
      className='space-y-3 rounded-md border p-3'
      onSubmit={form.handleSubmit(props.onConfirm)}
    >
      <p className='font-medium break-all'>{props.title}</p>
      {props.before !== undefined && (
        <div>
          <p className='text-sm'>{t('Current configuration')}</p>
          <pre className='text-muted-foreground text-xs break-all whitespace-pre-wrap'>
            {props.before}
          </pre>
        </div>
      )}
      {props.after !== undefined && (
        <div>
          <p className='text-sm'>{t('Proposed configuration')}</p>
          <pre className='text-xs break-all whitespace-pre-wrap'>
            {props.after}
          </pre>
        </div>
      )}
      <Field data-invalid={!!form.formState.errors.confirmed}>
        <div className='flex items-start gap-2'>
          <Checkbox
            id={id}
            checked={form.watch('confirmed')}
            disabled={props.pending}
            onCheckedChange={(value) =>
              form.setValue('confirmed', value === true, {
                shouldValidate: true,
              })
            }
            aria-invalid={!!form.formState.errors.confirmed}
          />
          <FieldLabel htmlFor={id}>
            {t('I reviewed this price change and its reference-only scope.')}
          </FieldLabel>
        </div>
        {form.formState.errors.confirmed && (
          <FieldError>{t('Required')}</FieldError>
        )}
      </Field>
      <div className='flex flex-wrap gap-2'>
        <Button type='submit' disabled={props.pending}>
          {props.pending ? t('Saving...') : t('Confirm')}
        </Button>
        <Button
          type='button'
          variant='outline'
          disabled={props.pending}
          onClick={props.onCancel}
        >
          {t('Cancel')}
        </Button>
      </div>
    </form>
  )
}
