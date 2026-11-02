import {
  useIsFetching,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogFooter,
  DialogTitle,
} from '@/components/ui/dialog'
import { useStatus } from '@/hooks/use-status'
import { createOperationId } from '@/lib/operation-id'
import { useAuthStore } from '@/stores/auth-store'

import { resolveUsagePolicyPresentation } from '../lib/user-usage-policy-presentation'
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
  returnFocus?: () => HTMLElement | null
}) {
  const actor = useAuthStore((state) => state.auth.user)
  const sid = useAuthStore((state) => state.auth.session?.sid ?? null)
  if (!actor) return null
  return (
    <UserUsagePolicySession
      key={`${actor.id}:${actor.role}:${sid}:${props.userId}`}
      {...props}
      actorId={actor.id}
      role={actor.role}
      sid={sid}
    />
  )
}
function UserUsagePolicySession(props: {
  userId: number
  actorId: number
  role: number
  sid: string | null
  onClose: () => void
  onSaved?: () => void
  returnFocus?: () => HTMLElement | null
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const funding = useStatus()
  const fundingFetching =
    useIsFetching({ queryKey: ['status'], exact: true }) > 0
  const queryKey = [
    'user-usage-policy',
    props.actorId,
    props.role,
    props.sid,
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
        (useAuthStore.getState().auth.session?.sid ?? null) !== props.sid ||
        actor.role !== 100
      ) {
        throw new Error('Session changed')
      }
      return writeUserUsagePolicy(props.userId, body)
    },
    retry: false,
    onSuccess: (data) => {
      const actor = useAuthStore.getState().auth.user
      if (actor?.id !== props.actorId || actor.role !== props.role || (useAuthStore.getState().auth.session?.sid ?? null) !== props.sid) return
      client.setQueryData(queryKey, data)
      operation.current = null
      setLocked(false)
      props.onSaved?.()
    },
    onSettled: () => {
      sending.current = false
    },
  })
  const busy = mutation.isPending || query.isFetching || fundingFetching
  const presentation = resolveUsagePolicyPresentation({
    userId: props.userId,
    policy: query.data,
    policyConfirmed: !query.isError && !query.isFetching && !locked,
    status: funding.status,
    statusConfirmed: funding.confirmed && !fundingFetching,
  })
  let fundingLabel = t('Unavailable')
  if (presentation.mode === 'enabled') {
    fundingLabel = t('Commercial funding enabled')
  }
  if (presentation.mode === 'retirement') {
    fundingLabel = t('Commercial funding retiring')
  }
  if (presentation.mode === 'disabled') {
    fundingLabel = t('Commercial funding disabled')
  }
  let conditionLabel = t('Configuration could not be confirmed.')
  if (presentation.condition === 'wallet') {
    conditionLabel = t('The saved preference uses the stored user allowance.')
  }
  if (presentation.condition === 'inactive') {
    conditionLabel = t(
      'The saved no-wallet preference is inactive while commercial funding is enabled or retiring.'
    )
  }
  if (presentation.condition === 'supported') {
    conditionLabel = t(
      'No-wallet configuration conditions are met for supported requests.'
    )
  }
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
        finalFocus={props.returnFocus}
        className='flex h-dvh max-h-dvh w-screen min-w-0 max-w-none flex-col overflow-hidden sm:h-auto sm:max-h-[calc(100dvh-2rem)] sm:max-w-lg [&_[data-slot=button]]:min-h-11 [&_[data-slot=button]]:min-w-11'
        showCloseButton={!mutation.isPending}
      >
        <DialogHeader className='shrink-0 text-start'>
          <DialogTitle className='pr-12'>{t('User usage policy')}</DialogTitle>
          <DialogDescription>
            {t(
              'This policy applies only while commercial funding is disabled. It does not remove Key limits.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='min-h-0 space-y-4 overflow-y-auto overscroll-contain'>
        <p className='text-muted-foreground text-sm'>{t('User ID')}: <span className='font-mono'>{props.userId}</span></p>
        <section
          aria-label={t('Current configuration')}
          className='flex min-w-0 flex-col gap-3 rounded-md border p-3 text-sm'
        >
          <dl className='flex flex-col gap-2'>
            <div>
              <dt className='font-medium'>{t('Saved wallet preference')}</dt>
              <dd>
                {presentation.savedNoBalance === null
                  ? t('Unavailable')
                  : policyLabel}
              </dd>
            </div>
            <div>
              <dt className='font-medium'>{t('Global funding mode')}</dt>
              <dd>{fundingLabel}</dd>
            </div>
          </dl>
          <p role='status'>{conditionLabel}</p>
          <p className='text-muted-foreground text-xs'>
            {t(
              'These are separate configuration snapshots, not a guarantee that a request is eligible. The server checks the user, Key, request path and limits at admission.'
            )}
          </p>
        </section>
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
              <section
                aria-label={t('Change saved preference')}
                className='flex flex-col gap-3'
              >
                <h3 className='text-sm font-medium'>
                  {t('Change saved preference')}
                </h3>
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
              </section>
            ) : (
              <p className='text-muted-foreground text-sm'>
                {t('Only Root can change this policy.')}
              </p>
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
        </div>
        <DialogFooter className='shrink-0 flex-wrap gap-2 border-t pt-4'>
          <Button
            variant='outline'
            disabled={busy || locked}
            onClick={() => {
              void Promise.all([
                query.refetch(),
                client.refetchQueries({ queryKey: ['status'], exact: true }),
              ])
            }}
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
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
