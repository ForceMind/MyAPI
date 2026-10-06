import { zodResolver } from '@hookform/resolvers/zod'
import { useId } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'

const schema = z.object({ digest: z.string().regex(/^[0-9a-f]{64}$/) })

export function OpenAIPriceVersionForm(props: {
  disabled: boolean
  onRead: (digest: string) => void
}) {
  const { t } = useTranslation()
  const inputID = useId()
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { digest: '' },
  })
  return (
    <form onSubmit={form.handleSubmit((values) => props.onRead(values.digest))}>
      <FieldGroup>
        <Field
          data-invalid={!!form.formState.errors.digest}
          data-disabled={props.disabled}
        >
          <FieldLabel htmlFor={inputID}>{t('Saved source SHA256')}</FieldLabel>
          <Input
            id={inputID}
            {...form.register('digest')}
            disabled={props.disabled}
            aria-invalid={!!form.formState.errors.digest}
            autoComplete='off'
            className='font-mono'
          />
          {form.formState.errors.digest && (
            <FieldError>
              {t('Enter a 64-character lowercase SHA256.')}
            </FieldError>
          )}
        </Field>
        <Button
          type='submit'
          variant='outline'
          disabled={props.disabled}
          className='w-full sm:w-fit'
        >
          {t('Read saved source version')}
        </Button>
      </FieldGroup>
    </form>
  )
}
