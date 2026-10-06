import { zodResolver } from '@hookform/resolvers/zod'
import { useId, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { useAuthStore } from '@/stores/auth-store'

import type { AccessSession } from '../lib/session'
import type { PolicyTarget } from '../types'
import { AccessPolicyEditor } from './access-policy-editor'
import { OwnerAccessView } from './owner-access-view'

const tokenIdSchema = z.object({
  tokenId: z
    .string()
    .regex(/^[1-9]\d*$/)
    .refine((value) => Number.isSafeInteger(Number(value))),
})
type DialogProps = {
  userId?: number
  tokenId?: number
  onClose: () => void
  onSaved?: () => void
}

export function AccessPolicyDialog(props: DialogProps) {
  const actor = useAuthStore((state) => state.auth.user)
  const sid = useAuthStore((state) => state.auth.session?.sid ?? null)
  if (!actor) return null
  return (
    <AccessPolicySession
      key={`${actor.id}:${actor.role}:${sid}:${props.userId}:${props.tokenId}`}
      {...props}
      scope={{ userId: actor.id, role: actor.role, sid }}
    />
  )
}

function AccessPolicySession(props: DialogProps & { scope: AccessSession }) {
  const { t } = useTranslation()
  const id = useId()
  const admin = props.scope.role >= 10
  const ownerUserId = props.userId ?? props.scope.userId
  const [target, setTarget] = useState<PolicyTarget | null>(() => ({
    subject: props.tokenId ? 'token' : 'user',
    id: props.tokenId ?? ownerUserId,
    ownerUserId,
  }))
  const [targetKind, setTargetKind] = useState(props.tokenId ? 'token' : 'user')
  const form = useForm<z.infer<typeof tokenIdSchema>>({
    resolver: zodResolver(tokenIdSchema),
    defaultValues: { tokenId: '' },
  })
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
    >
      <DialogContent
        className='max-h-[85dvh] min-w-0 overflow-y-auto sm:max-w-xl'
        showCloseButton={false}
      >
        <DialogHeader>
          <DialogTitle>{t('Assigned access')}</DialogTitle>
          <DialogDescription>
            {t(
              'Administrator assignments only narrow existing access. User and Key restrictions apply together.'
            )}
          </DialogDescription>
        </DialogHeader>
        <p className='text-muted-foreground text-xs break-words'>
          {t(
            'Assigned access supports OpenAI Chat and Responses, and Codex Responses. Other request paths are denied.'
          )}
        </p>
        <p className='text-muted-foreground text-xs break-words'>
          {t(
            'Assigned access requires an unambiguous JSON model and a final request body no larger than 1 MiB.'
          )}
        </p>
        <p className='text-muted-foreground text-xs break-words'>
          {t(
            'Availability is a current policy check, not a guarantee of future routing or quota.'
          )}
        </p>
        {admin && (
          <p className='text-muted-foreground text-xs break-words'>
            {t('User ID')}: {ownerUserId}
            {target?.subject === 'token' && (
              <>
                {' '}
                · {t('Key ID')}: {target.id}
              </>
            )}
          </p>
        )}
        {props.userId && admin && (
          <FieldGroup className='gap-3'>
            <Field>
              <FieldLabel htmlFor={`${id}-target`}>
                {t('Assignment target')}
              </FieldLabel>
              <NativeSelect
                id={`${id}-target`}
                className='w-full'
                value={targetKind}
                onChange={(event) => {
                  const kind = event.target.value
                  setTargetKind(kind)
                  setTarget(
                    kind === 'user'
                      ? { subject: 'user', id: ownerUserId, ownerUserId }
                      : null
                  )
                }}
              >
                <NativeSelectOption value='user'>
                  {t('User default')}
                </NativeSelectOption>
                <NativeSelectOption value='token'>
                  {t('Specific Key')}
                </NativeSelectOption>
              </NativeSelect>
            </Field>
            {targetKind === 'token' && (
              <form
                className='space-y-3'
                onSubmit={form.handleSubmit((value) =>
                  setTarget({
                    subject: 'token',
                    id: Number(value.tokenId),
                    ownerUserId,
                  })
                )}
              >
                <Field data-invalid={!!form.formState.errors.tokenId}>
                  <FieldLabel htmlFor={`${id}-token`}>{t('Key ID')}</FieldLabel>
                  <Input
                    id={`${id}-token`}
                    inputMode='numeric'
                    aria-invalid={!!form.formState.errors.tokenId}
                    {...form.register('tokenId', {
                      onChange: () => setTarget(null),
                    })}
                  />
                  {form.formState.errors.tokenId && (
                    <FieldError>{t('Enter a positive Key ID.')}</FieldError>
                  )}
                </Field>
                <Button type='submit'>{t('Load Key policy')}</Button>
              </form>
            )}
          </FieldGroup>
        )}
        {props.tokenId && (
          <OwnerAccessView tokenId={props.tokenId} scope={props.scope} />
        )}
        {target && admin && (
          <section
            className='min-w-0 space-y-3'
            aria-label={t('Admin assignment')}
          >
            <h3 className='font-medium'>{t('Admin assignment')}</h3>
            <AccessPolicyEditor
              key={`${target.subject}:${target.id}`}
              target={target}
              scope={props.scope}
              onSaved={props.onSaved}
            />
          </section>
        )}
        {!props.tokenId && !admin && (
          <p role='alert'>
            {t('Only administrators can change assigned access.')}
          </p>
        )}
        <div className='flex justify-end'>
          <Button type='button' variant='outline' onClick={props.onClose}>
            {t('Close')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
