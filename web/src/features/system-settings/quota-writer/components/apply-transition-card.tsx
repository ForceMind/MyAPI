/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

import {
  QUOTA_WRITER_ACK_NOTE_MAX_LENGTH,
  QUOTA_WRITER_ALLOWED_TARGETS,
  translateQuotaWriterFinding,
} from '../constants'
import {
  useApplyQuotaWriterTransition,
  useQuotaWriterTransitionPlan,
} from '../hooks/use-quota-writer'
import type {
  QuotaWriterApplyConflictData,
  QuotaWriterEpochState,
  QuotaWriterMode,
} from '../types'

const applyFormSchema = z.object({
  target_mode: z.enum(['bridge', 'authoritative']),
  expected_epoch: z.number().int().positive(),
  ack_note: z.string().max(QUOTA_WRITER_ACK_NOTE_MAX_LENGTH),
})

type ApplyFormValues = z.infer<typeof applyFormSchema>

type ApplyTransitionCardProps = {
  state: QuotaWriterEpochState
}

export function ApplyTransitionCard(props: ApplyTransitionCardProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const allowedTargets = QUOTA_WRITER_ALLOWED_TARGETS[props.state.mode]
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [conflictMissing, setConflictMissing] = useState<string[] | null>(null)

  const form = useForm<ApplyFormValues>({
    resolver: zodResolver(applyFormSchema),
    defaultValues: {
      target_mode: allowedTargets[0] ?? 'bridge',
      expected_epoch: props.state.epoch,
      ack_note: '',
    },
  })

  // Keep expected_epoch in sync with the freshest status; the backend rejects
  // the apply when the persisted epoch moved on.
  useEffect(() => {
    form.setValue('expected_epoch', props.state.epoch)
  }, [form, props.state.epoch])

  const targetMode = form.watch('target_mode')
  const planQuery = useQuotaWriterTransitionPlan(
    allowedTargets.length > 0 ? targetMode : null
  )
  const applyMutation = useApplyQuotaWriterTransition()

  if (allowedTargets.length === 0) {
    return (
      <div className='rounded-lg border p-4'>
        <Alert>
          <AlertTitle>{t('No transition available')}</AlertTitle>
          <AlertDescription>
            {t(
              'Current mode is already authoritative and cannot be downgraded.'
            )}
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  const requiresAck = props.state.mode === 'legacy'
  const plan = planQuery.data ?? null
  const validationFindings = plan?.validation ?? []

  const onSubmit = (values: ApplyFormValues) => {
    if (requiresAck && values.ack_note.trim().length === 0) {
      form.setError('ack_note', {
        type: 'manual',
        message: t(
          'An acknowledgement note (1-512 characters) is required for this transition.'
        ),
      })
      return
    }
    setConfirmOpen(true)
  }

  const handleConfirm = () => {
    const values = form.getValues()
    applyMutation.mutate(
      {
        target_mode: values.target_mode as QuotaWriterMode,
        expected_epoch: values.expected_epoch,
        ack_note: values.ack_note.trim(),
      },
      {
        onSuccess: () => {
          setConfirmOpen(false)
          setConflictMissing(null)
          form.setValue('ack_note', '')
          toast.success(t('Transition applied successfully'))
        },
        onError: (error) => {
          setConfirmOpen(false)
          if (axios.isAxiosError(error) && error.response?.status === 409) {
            const body = error.response.data as {
              message?: string
              data?: QuotaWriterApplyConflictData
            }
            const missing = body?.data?.missing
            if (missing && missing.length > 0) {
              setConflictMissing(missing)
              toast.error(body.message || t('Preconditions are not met'))
            } else {
              toast.error(
                body.message ||
                  t('Epoch conflict: the status was refreshed, please retry.')
              )
            }
            queryClient.invalidateQueries({ queryKey: ['quota-writer'] })
            return
          }
          if (axios.isAxiosError(error)) {
            toast.error(
              (error.response?.data as { message?: string } | undefined)
                ?.message || t('Failed to apply transition')
            )
            queryClient.invalidateQueries({ queryKey: ['quota-writer'] })
            return
          }
          toast.error(t('Failed to apply transition'))
        },
      }
    )
  }

  return (
    <div className='flex flex-col gap-4 rounded-lg border p-4'>
      <div className='flex flex-col gap-1'>
        <h4 className='text-sm font-semibold'>{t('Apply transition')}</h4>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Modes can only move forward: legacy → bridge → authoritative. This switch is one-way and cannot be undone.'
          )}
        </p>
      </div>

      {planQuery.isLoading && (
        <LoadingState inline message={t('Loading transition plan...')} />
      )}

      {plan && (
        <div className='bg-muted/40 flex flex-col gap-2 rounded-md p-3 text-sm'>
          <div className='flex flex-wrap items-center gap-x-6 gap-y-1'>
            <span className='text-muted-foreground'>
              {t('Proposed epoch')}:{' '}
              <span className='text-foreground font-medium'>
                {plan.proposed_epoch}
              </span>
            </span>
            <span className='flex items-center gap-2'>
              <span className='text-muted-foreground'>{t('Readiness')}:</span>
              <StatusBadge
                label={plan.ready ? t('Ready to apply') : t('Not ready')}
                variant={plan.ready ? 'success' : 'warning'}
                copyable={false}
              />
            </span>
          </div>
          {validationFindings.length > 0 && (
            <ul className='text-warning list-inside list-disc'>
              {validationFindings.map((finding) => (
                <li key={finding}>{translateQuotaWriterFinding(finding)}</li>
              ))}
            </ul>
          )}
        </div>
      )}

      {conflictMissing && conflictMissing.length > 0 && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Preconditions are not met')}</AlertTitle>
          <AlertDescription>
            <ul className='list-inside list-disc'>
              {conflictMissing.map((missing) => (
                <li key={missing}>{translateQuotaWriterFinding(missing)}</li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      )}

      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(onSubmit)}
          className='flex flex-col gap-4'
        >
          <FormField
            control={form.control}
            name='target_mode'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Target mode')}</FormLabel>
                <Select
                  value={field.value}
                  onValueChange={(value) => {
                    field.onChange(value)
                    setConflictMissing(null)
                  }}
                >
                  <FormControl>
                    <SelectTrigger className='w-full sm:max-w-xs'>
                      <SelectValue placeholder={t('Select target mode')} />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    {allowedTargets.map((target) => (
                      <SelectItem key={target} value={target}>
                        {target}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='expected_epoch'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Expected epoch')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    className='w-full sm:max-w-xs'
                    value={Number.isNaN(field.value) ? '' : field.value}
                    name={field.name}
                    ref={field.ref}
                    onBlur={field.onBlur}
                    onChange={(event) =>
                      field.onChange(event.target.valueAsNumber)
                    }
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Auto-filled from the current status; the apply fails if the epoch changed meanwhile.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='ack_note'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Cluster drain acknowledgement note')}</FormLabel>
                <FormControl>
                  <Textarea
                    {...field}
                    rows={3}
                    maxLength={QUOTA_WRITER_ACK_NOTE_MAX_LENGTH}
                    placeholder={t(
                      'Describe how cluster-wide drain was confirmed (e.g. every node reports zero in-flight sessions).'
                    )}
                  />
                </FormControl>
                <FormDescription>
                  {requiresAck
                    ? t(
                        'Required for legacy → bridge (1-512 characters). Persisted as transition evidence.'
                      )
                    : t(
                        'Optional for this transition (1-512 characters). Persisted as transition evidence.'
                      )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div>
            <Button type='submit' disabled={applyMutation.isPending}>
              {applyMutation.isPending
                ? t('Applying...')
                : t('Apply transition')}
            </Button>
          </div>
        </form>
      </Form>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('Confirm mode transition')}
        desc={
          <div className='flex flex-col gap-2'>
            <p>
              {t(
                'Switch the quota writer mode from {{from}} to {{to}}? This is a one-way transition and cannot be undone.',
                { from: props.state.mode, to: targetMode }
              )}
            </p>
            <p>
              {targetMode === 'bridge'
                ? t(
                    'Bridge mode silently drains the fleet: new relay sessions are rejected while in-flight sessions settle, and subscription balance purchases and admin quota adjustments stay unavailable.'
                  )
                : t('Authoritative mode is final and cannot be downgraded.')}
            </p>
          </div>
        }
        confirmText={t('Apply transition')}
        isLoading={applyMutation.isPending}
        handleConfirm={handleConfirm}
      />
    </div>
  )
}
