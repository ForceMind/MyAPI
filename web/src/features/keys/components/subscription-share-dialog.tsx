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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { RadioGroup } from '@/components/ui/radio-group'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { useAuthStore } from '@/stores/auth-store'

import {
  closeSubscriptionSharePool,
  createSubscriptionShareOperationId,
  getSubscriptionShare,
  writeSubscriptionShare,
  type SubscriptionShareCommand,
  type SubscriptionShareCloseCommand,
  type SubscriptionSharePool,
  type SubscriptionShareView,
} from '../subscription-share-api'
import type { ApiKey } from '../types'
import { PersonalSubscriptionShareForm } from './personal-subscription-share-form'
import { SubscriptionSharePoolCard } from './subscription-share-pool-card'

const ROOT_ROLE = 100

function poolSelectionValue(pool: SubscriptionSharePool): string {
  return `${pool.pool_id}:${pool.snapshot_id}`
}

function basisPointsInput(value: number): string {
  const whole = Math.floor(value / 100)
  const fraction = String(value % 100)
    .padStart(2, '0')
    .replace(/0+$/, '')
  return fraction ? `${whole}.${fraction}` : String(whole)
}

function parseBasisPoints(value: string): number | null {
  const normalized = value.trim().replace(',', '.')
  if (!/^(?:100(?:\.0{1,2})?|(?:\d|[1-9]\d)(?:\.\d{1,2})?)$/.test(normalized)) {
    return null
  }
  const [whole, fraction = ''] = normalized.split('.')
  const basisPoints = Number(whole) * 100 + Number(fraction.padEnd(2, '0'))
  return basisPoints >= 0 && basisPoints <= 10000 ? basisPoints : null
}

export function SubscriptionShareDialog(props: {
  tokenId: number
  personalKey?: ApiKey
  onClose: () => void
}) {
  const userId = useAuthStore((state) => state.auth.user?.id)
  const role = useAuthStore((state) => state.auth.user?.role)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  if (!userId || role === undefined) return null

  return (
    <SubscriptionShareSession
      key={`${userId}:${role}:${sessionId ?? 'local'}:${props.tokenId}`}
      tokenId={props.tokenId}
      personalKey={props.personalKey}
      userId={userId}
      role={role}
      sessionId={sessionId}
      onClose={props.onClose}
    />
  )
}

function SubscriptionShareSession(props: {
  personalKey?: ApiKey
  tokenId: number
  userId: number
  role: number
  sessionId: string | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const queryKey = [
    'subscription-share',
    props.userId,
    props.role,
    props.sessionId,
    props.tokenId,
  ] as const
  const [advancedPersonal, setAdvancedPersonal] = useState(false)
  const [selection, setSelection] = useState('')
  const [percentage, setPercentage] = useState('')
  const [confirmed, setConfirmed] = useState(false)
  const [windowChangeConfirmed, setWindowChangeConfirmed] = useState(false)
  const [poolToClose, setPoolToClose] = useState<SubscriptionSharePool | null>(
    null
  )
  const [closeConfirmed, setCloseConfirmed] = useState(false)
  const [mustReload, setMustReload] = useState(false)
  const sending = useRef(false)
  const closeSending = useRef(false)
  const operation = useRef<SubscriptionShareCommand | null>(null)
  const query = useQuery({
    queryKey,
    queryFn: ({ signal }) => getSubscriptionShare(props.tokenId, signal),
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
  const view = query.isError ? undefined : query.data
  const selectedPool = view?.pools.find(
    (pool) => pool.state !== 'closed' && poolSelectionValue(pool) === selection
  )
  const requiresWindowChangeConfirmation = Boolean(
    selectedPool?.replacement_required || selectedPool?.rebaseline_required
  )
  const shareBasisPoints = parseBasisPoints(percentage)
  const changesRebaselineShare = Boolean(
    selectedPool?.rebaseline_required &&
    shareBasisPoints !== selectedPool.share_bps
  )
  const exceedsPool =
    !!selectedPool &&
    shareBasisPoints !== null &&
    selectedPool.allocated_bps - selectedPool.share_bps + shareBasisPoints >
      10000
  let inputError = ''
  if (percentage && shareBasisPoints === null) {
    inputError = t(
      'Enter a percentage from 0 to 100 with no more than two decimal places.'
    )
  } else if (exceedsPool) {
    inputError = t('This share would make the pool allocation exceed 100%.')
  }

  const sameSession = () => {
    const auth = useAuthStore.getState().auth
    return (
      auth.user?.id === props.userId &&
      auth.user.role === props.role &&
      (auth.session?.sid ?? null) === props.sessionId
    )
  }
  const mutation = useMutation({
    mutationFn: (command: SubscriptionShareCommand) => {
      if (!sameSession() || props.role !== ROOT_ROLE) {
        throw new Error('Session changed')
      }
      return writeSubscriptionShare(props.tokenId, command)
    },
    retry: false,
    onSuccess: (view) => {
      if (!sameSession()) return
      client.setQueryData<SubscriptionShareView>(queryKey, view)
      operation.current = null
      sending.current = false
      toast.success(t('Subscription share saved'))
      props.onClose()
    },
    onError: () => {
      if (!sameSession()) return
      sending.current = false
      setMustReload(true)
    },
  })
  const closeMutation = useMutation({
    mutationFn: ({
      poolId,
      command,
    }: {
      poolId: number
      command: SubscriptionShareCloseCommand
    }) => {
      if (!sameSession() || props.role !== ROOT_ROLE) {
        throw new Error('Session changed')
      }
      return closeSubscriptionSharePool(poolId, command)
    },
    retry: false,
    onSuccess: async () => {
      if (!sameSession()) return
      setSelection('')
      setPercentage('')
      setConfirmed(false)
      setWindowChangeConfirmed(false)
      setPoolToClose(null)
      setCloseConfirmed(false)
      closeSending.current = false
      toast.success(t('Subscription pool closed'))
      await client.invalidateQueries({ queryKey, exact: true })
    },
    onError: () => {
      if (!sameSession()) return
      setPoolToClose(null)
      setCloseConfirmed(false)
      closeSending.current = false
      setMustReload(true)
    },
  })

  const submit = () => {
    if (
      sending.current ||
      mustReload ||
      !selectedPool ||
      shareBasisPoints === null ||
      exceedsPool ||
      changesRebaselineShare ||
      !confirmed ||
      (requiresWindowChangeConfirmation && !windowChangeConfirmed)
    ) {
      return
    }
    operation.current = {
      id: createSubscriptionShareOperationId(),
      pool_id: selectedPool.pool_id,
      snapshot_id: selectedPool.snapshot_id,
      expected_revision: selectedPool.revision,
      share_bps: shareBasisPoints,
      confirmed_approximate: true,
      ...(requiresWindowChangeConfirmation
        ? { replace_window: true as const }
        : {}),
    }
    sending.current = true
    mutation.mutate(operation.current)
  }

  const reload = async () => {
    const result = await query.refetch()
    if (!result.isSuccess || !sameSession()) return
    operation.current = null
    sending.current = false
    setMustReload(false)
    mutation.reset()
    closeMutation.reset()
    closeSending.current = false
    setSelection('')
    setPercentage('')
    setConfirmed(false)
    setWindowChangeConfirmed(false)
  }

  const closePool = () => {
    if (
      !poolToClose ||
      poolToClose.pool_id === 0 ||
      poolToClose.state === 'closed' ||
      !closeConfirmed ||
      closeSending.current ||
      mustReload
    ) {
      return
    }
    closeSending.current = true
    closeMutation.mutate({
      poolId: poolToClose.pool_id,
      command: {
        id: createSubscriptionShareOperationId(),
        expected_revision: poolToClose.revision,
        confirmed_close: true,
      },
    })
  }

  const root = props.role === ROOT_ROLE
  const visiblePools = root
    ? (view?.pools ?? [])
    : (view?.pools ?? []).filter(
        (pool) => pool.pool_id > 0 && pool.state !== 'candidate'
      )
  const busy = query.isFetching || mutation.isPending || closeMutation.isPending

  if (props.personalKey && root && !advancedPersonal) {
    return (
      <PersonalSubscriptionShareForm
        apiKey={props.personalKey}
        pools={visiblePools}
        selectedPool={selectedPool}
        selection={selection}
        percentage={percentage}
        shareBasisPoints={shareBasisPoints}
        inputError={inputError}
        confirmed={confirmed}
        windowChangeConfirmed={windowChangeConfirmed}
        busy={busy}
        saving={mutation.isPending}
        loading={!query.data && !query.isError}
        loadError={query.isError}
        mustReload={mustReload}
        saveDisabled={
          busy ||
          mustReload ||
          !selectedPool ||
          shareBasisPoints === null ||
          exceedsPool ||
          changesRebaselineShare ||
          !confirmed ||
          (requiresWindowChangeConfirmation && !windowChangeConfirmed)
        }
        onSelect={(value) => {
          const pool = visiblePools.find(
            (item) => poolSelectionValue(item) === value
          )
          setSelection(value)
          setPercentage(pool ? basisPointsInput(pool.share_bps) : '')
          setConfirmed(false)
          setWindowChangeConfirmed(false)
        }}
        onPercentageChange={setPercentage}
        onConfirmedChange={setConfirmed}
        onWindowChangeConfirmed={setWindowChangeConfirmed}
        onReload={reload}
        onSave={submit}
        onAdvanced={() => setAdvancedPersonal(true)}
        onCancel={props.onClose}
      />
    )
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !mutation.isPending && !closeMutation.isPending) {
          props.onClose()
        }
      }}
    >
      <DialogContent
        className='max-h-[90dvh] min-w-0 overflow-y-auto sm:max-w-2xl'
        showCloseButton={!mutation.isPending && !closeMutation.isPending}
      >
        <DialogHeader>
          <DialogTitle className='pr-10'>
            {t('Subscription shares')}
          </DialogTitle>
          <DialogDescription>
            {t(
              'Allocate part of one native subscription window to this API Key.'
            )}
          </DialogDescription>
        </DialogHeader>

        <Alert>
          <AlertTitle>{t('Approximate usage allocation')}</AlertTitle>
          <AlertDescription className='space-y-2 break-words'>
            <p>
              {t(
                'Positive account-window changes are divided approximately by each Key’s sent request attempts. This does not convert tokens or USD into subscription usage.'
              )}
            </p>
            <p>
              {t(
                'Request size, model, failures, external clients, measurement delay and rounding can all cause error. There is no verified numerical error bound.'
              )}
            </p>
            <p>
              {t(
                'Reaching the share blocks later requests. In-flight requests can exceed it, and the share does not reserve capacity against other clients.'
              )}
            </p>
          </AlertDescription>
        </Alert>

        {query.isError && (
          <div role='alert' className='space-y-2'>
            <p>{t('Could not load subscription shares.')}</p>
            <Button
              variant='outline'
              disabled={query.isFetching}
              onClick={reload}
            >
              {t('Retry')}
            </Button>
          </div>
        )}
        {!query.data && !query.isError && (
          <div role='status' className='space-y-2'>
            <span className='sr-only'>{t('Loading...')}</span>
            <Skeleton className='h-24 w-full' />
            <Skeleton className='h-24 w-full' />
          </div>
        )}
        {view && visiblePools.length === 0 && (
          <Empty role='status' className='border'>
            <EmptyHeader>
              <EmptyTitle>
                {root
                  ? t('No eligible subscription windows are available.')
                  : t('No subscription share is configured for this Key.')}
              </EmptyTitle>
            </EmptyHeader>
          </Empty>
        )}

        {view && visiblePools.length > 0 && root && (
          <FieldGroup className='gap-4'>
            <FieldSet>
              <FieldLegend variant='label'>
                {t('Choose one subscription window')}
              </FieldLegend>
              <FieldDescription>
                {t(
                  'No window is selected automatically. Each window is measured and limited separately.'
                )}
              </FieldDescription>
              <RadioGroup
                value={selection}
                disabled={busy || mustReload}
                onValueChange={(value) => {
                  const pool = visiblePools.find(
                    (item) => poolSelectionValue(item) === value
                  )
                  setSelection(value)
                  setPercentage(pool ? basisPointsInput(pool.share_bps) : '')
                  setConfirmed(false)
                  setWindowChangeConfirmed(false)
                }}
              >
                {visiblePools.map((pool) => (
                  <SubscriptionSharePoolCard
                    key={poolSelectionValue(pool)}
                    pool={pool}
                    selectable={pool.state !== 'closed'}
                    showAdminDetails
                    closeDisabled={busy || mustReload}
                    onClosePool={
                      pool.pool_id > 0 && pool.state !== 'closed'
                        ? () => {
                            setPoolToClose(pool)
                            setCloseConfirmed(false)
                          }
                        : undefined
                    }
                  />
                ))}
              </RadioGroup>
            </FieldSet>

            {selectedPool && (
              <>
                {selectedPool.pool_id === 0 && (
                  <Alert variant='destructive'>
                    <AlertTitle>{t('This creates a managed pool')}</AlertTitle>
                    <AlertDescription>
                      {t(
                        'After activation, other Keys without an allocation cannot use this managed subscription window. Existing Keys and channel credentials are not recreated.'
                      )}
                    </AlertDescription>
                  </Alert>
                )}
                {selectedPool.replacement_required && (
                  <>
                    <Alert variant='destructive'>
                      <AlertTitle>
                        {t(
                          'This replaces the managed window for every Key in this pool'
                        )}
                      </AlertTitle>
                      <AlertDescription>
                        {t(
                          'Existing Key shares and old window history are kept. The replacement starts from a new fresh baseline and does not require signing in again.'
                        )}
                      </AlertDescription>
                    </Alert>
                    <Field
                      orientation='horizontal'
                      data-disabled={busy || mustReload}
                    >
                      <Checkbox
                        id='subscription-share-replacement-confirmation'
                        checked={windowChangeConfirmed}
                        disabled={busy || mustReload}
                        onCheckedChange={(checked) =>
                          setWindowChangeConfirmed(checked === true)
                        }
                      />
                      <FieldLabel htmlFor='subscription-share-replacement-confirmation'>
                        {t(
                          'I understand this replacement affects every Key in the pool.'
                        )}
                      </FieldLabel>
                    </Field>
                  </>
                )}
                {selectedPool.rebaseline_required && (
                  <>
                    <Alert variant='destructive'>
                      <AlertTitle>
                        {t('Resume observation from a fresh baseline')}
                      </AlertTitle>
                      <AlertDescription>
                        {t(
                          'Existing estimated usage is kept. Requests that could not be attributed remain uncertain, and observation resumes from a new fresh sample.'
                        )}
                      </AlertDescription>
                    </Alert>
                    <Field
                      orientation='horizontal'
                      data-disabled={busy || mustReload}
                    >
                      <Checkbox
                        id='subscription-share-rebaseline-confirmation'
                        checked={windowChangeConfirmed}
                        disabled={busy || mustReload}
                        onCheckedChange={(checked) =>
                          setWindowChangeConfirmed(checked === true)
                        }
                      />
                      <FieldLabel htmlFor='subscription-share-rebaseline-confirmation'>
                        {t(
                          'I understand this recovery affects every Key in the pool.'
                        )}
                      </FieldLabel>
                    </Field>
                  </>
                )}
                <Field data-invalid={!!inputError}>
                  <FieldLabel htmlFor='subscription-share-percentage'>
                    {t('Share percentage')}
                  </FieldLabel>
                  <Input
                    id='subscription-share-percentage'
                    type='text'
                    inputMode='decimal'
                    autoComplete='off'
                    value={percentage}
                    disabled={
                      busy || mustReload || selectedPool.rebaseline_required
                    }
                    aria-invalid={!!inputError}
                    aria-describedby='subscription-share-percentage-help'
                    onChange={(event) => setPercentage(event.target.value)}
                  />
                  <FieldDescription id='subscription-share-percentage-help'>
                    {selectedPool.rebaseline_required
                      ? t(
                          'This recovery only resumes observation; the current Key share does not change.'
                        )
                      : t(
                          'Use 0% to 100% with at most two decimal places. A 0% share blocks this Key’s requests for the window while keeping its binding and history.'
                        )}
                  </FieldDescription>
                  {inputError && <FieldError>{inputError}</FieldError>}
                </Field>
                <Field
                  orientation='horizontal'
                  data-disabled={busy || mustReload}
                >
                  <Checkbox
                    id='subscription-share-confirmation'
                    checked={confirmed}
                    disabled={busy || mustReload}
                    onCheckedChange={(checked) =>
                      setConfirmed(checked === true)
                    }
                  />
                  <FieldLabel htmlFor='subscription-share-confirmation'>
                    {t(
                      'I understand this is an estimate with no verified error bound and may stop requests only after the share is exceeded.'
                    )}
                  </FieldLabel>
                </Field>
              </>
            )}
          </FieldGroup>
        )}

        {view && visiblePools.length > 0 && !root && (
          <section
            aria-label={t('Your subscription share')}
            className='space-y-3'
          >
            <p className='text-muted-foreground text-sm'>
              {t('Only Root can change subscription shares.')}
            </p>
            {visiblePools.map((pool) => (
              <SubscriptionSharePoolCard
                key={poolSelectionValue(pool)}
                pool={pool}
                selectable={false}
                showAdminDetails={false}
              />
            ))}
          </section>
        )}

        {mustReload && (
          <Alert variant='destructive'>
            <AlertTitle>
              {t('The save result is unknown or out of date')}
            </AlertTitle>
            <AlertDescription className='space-y-2'>
              <p>
                {t(
                  'Reload the current server state before trying again. The save will not be repeated automatically.'
                )}
              </p>
              <Button
                variant='outline'
                disabled={query.isFetching}
                onClick={reload}
              >
                {t('Reload current state')}
              </Button>
            </AlertDescription>
          </Alert>
        )}

        <DialogFooter>
          <Button
            variant='outline'
            disabled={mutation.isPending || closeMutation.isPending}
            onClick={props.onClose}
          >
            {t('Close')}
          </Button>
          {root && view && visiblePools.length > 0 && (
            <Button
              disabled={
                busy ||
                mustReload ||
                !selectedPool ||
                shareBasisPoints === null ||
                exceedsPool ||
                changesRebaselineShare ||
                !confirmed ||
                (requiresWindowChangeConfirmation && !windowChangeConfirmed)
              }
              onClick={submit}
            >
              {mutation.isPending && <Spinner data-icon='inline-start' />}
              {mutation.isPending ? t('Saving...') : t('Save share')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>

      <AlertDialog
        open={poolToClose !== null}
        onOpenChange={(open) => {
          if (!open && !closeMutation.isPending) {
            setPoolToClose(null)
            setCloseConfirmed(false)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('Close this subscription pool?')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Closing affects every Key in this pool. It removes this pool’s share limits; other limits still apply. In-flight requests may continue consuming capacity. History is retained, and this window description cannot be enabled again in this version.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <Field
            orientation='horizontal'
            data-disabled={closeMutation.isPending}
          >
            <Checkbox
              id='subscription-share-close-confirmation'
              checked={closeConfirmed}
              disabled={closeMutation.isPending}
              onCheckedChange={(checked) => setCloseConfirmed(checked === true)}
            />
            <FieldLabel htmlFor='subscription-share-close-confirmation'>
              {t(
                'I understand this pool cannot be enabled again in this version.'
              )}
            </FieldLabel>
          </Field>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={closeMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              disabled={!closeConfirmed || closeMutation.isPending}
              onClick={closePool}
            >
              {closeMutation.isPending && <Spinner data-icon='inline-start' />}
              {closeMutation.isPending ? t('Closing...') : t('Close pool')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Dialog>
  )
}
