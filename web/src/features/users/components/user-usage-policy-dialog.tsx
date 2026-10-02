import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { createOperationId } from '@/lib/operation-id'
import { useAuthStore } from '@/stores/auth-store'

import {
  getUserUsagePolicy,
  writeUserUsagePolicy,
  type UserUsagePolicyCommand,
} from '../user-usage-policy-api'
import { UserUsagePolicyForm } from './user-usage-policy-form'

export function UserUsagePolicyDialog(props: {
  userId: number
  onClose: () => void
  onSaved?: () => void
}) {
  const actor = useAuthStore((state) => state.auth.user)
  if (!actor) return null
  return (
    <UserUsagePolicySession
      key={`${actor.id}:${actor.role}:${props.userId}`}
      {...props}
      actorId={actor.id}
      role={actor.role}
    />
  )
}
function UserUsagePolicySession(props: {
  userId: number
  actorId: number
  role: number
  onClose: () => void
  onSaved?: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const queryKey = [
    'user-usage-policy',
    props.actorId,
    props.role,
    props.userId,
  ]
  const [locked, setLocked] = useState(false)
  const sending = useRef(false)
  const operation = useRef<UserUsagePolicyCommand | null>(null)
  const query = useQuery({
    queryKey,
    queryFn: ({ signal }) => getUserUsagePolicy(props.userId, signal),
    enabled: !locked,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  const mutation = useMutation({
    mutationFn: (body: UserUsagePolicyCommand) => {
      const actor = useAuthStore.getState().auth.user
      if (
        actor?.id !== props.actorId ||
        actor.role !== props.role ||
        actor.role !== 100
      ) {
        throw new Error('Session changed')
      }
      return writeUserUsagePolicy(props.userId, body)
    },
    retry: false,
    onSuccess: (data) => {
      const actor = useAuthStore.getState().auth.user
      if (actor?.id !== props.actorId || actor.role !== props.role) return
      client.setQueryData(queryKey, data)
      operation.current = null
      setLocked(false)
      props.onSaved?.()
    },
    onSettled: () => {
      sending.current = false
    },
  })
  const busy = mutation.isPending || query.isFetching
  const policyLabel = query.data?.no_balance
    ? t('Use Key limits without a user wallet')
    : t('Use the stored user allowance')
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !sending.current) props.onClose()
      }}
    >
      <DialogContent
        className='max-h-[85dvh] min-w-0 overflow-y-auto sm:max-w-lg'
        showCloseButton={!mutation.isPending}
      >
        <DialogHeader>
          <DialogTitle>{t('User usage policy')}</DialogTitle>
          <DialogDescription>
            {t(
              'This policy applies only while commercial funding is disabled. It does not remove Key limits.'
            )}
          </DialogDescription>
        </DialogHeader>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Chat and Responses only. Unsupported request paths are rejected before dispatch.'
          )}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Changing this policy does not grant credit, clear usage or change already admitted requests.'
          )}
        </p>
        {query.isError && <p role='alert'>{t('Operation failed')}</p>}
        {!query.data && !query.isError && (
          <p role='status'>{t('Loading...')}</p>
        )}
        {query.data && (
          <>
            <dl className='space-y-1 text-sm'>
              <dt>{t('Stored user allowance (internal units)')}</dt>
              <dd className='font-mono break-all'>
                {query.data.legacy_remaining_quota.toLocaleString()}
              </dd>
            </dl>
            {props.role === 100 ? (
              <UserUsagePolicyForm
                key={query.data.revision}
                policy={query.data}
                busy={busy}
                locked={locked}
                onSubmit={(noBalance) => {
                  if (sending.current) return
                  if (!operation.current) {
                    operation.current = {
                      id: createOperationId(),
                      expected_revision: query.data.revision,
                      no_balance: noBalance,
                      confirmed: true,
                    }
                  }
                  sending.current = true
                  setLocked(true)
                  mutation.mutate(operation.current)
                }}
              />
            ) : (
              <>
                <p>{policyLabel}</p>
                <p className='text-muted-foreground text-sm'>
                  {t('Only Root can change this policy.')}
                </p>
              </>
            )}
          </>
        )}
        {mutation.isError && (
          <p role='alert' className='text-destructive text-sm'>
            {t(
              'The outcome may already be saved. Retry the same request or close and refresh.'
            )}
          </p>
        )}
        <div className='flex justify-end gap-2'>
          <Button
            variant='outline'
            disabled={busy || locked}
            onClick={() => void query.refetch()}
          >
            {t('Refresh')}
          </Button>
          <Button
            variant='outline'
            disabled={mutation.isPending}
            onClick={props.onClose}
          >
            {t('Close')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
