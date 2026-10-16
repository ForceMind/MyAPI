/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import {
  AlertTriangle,
  ArrowRight,
  CheckCircle2,
  KeyRound,
  RefreshCw,
  ShieldCheck,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
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
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { quotaWindowLabel } from '@/features/channels/lib/quota-history'
import { formatTimestampToDate } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import {
  commitCredentialMigration,
  createCredentialMigrationOperationId,
  getCredentialMaintenance,
  getCredentialMigrationReceipt,
  isUncertainCredentialCommit,
  previewCredentialMigration,
  type CredentialMaintenance,
  type CredentialMigrationInput,
  type CredentialMigrationPreview,
  type CredentialMigrationReceipt,
} from '../credential-maintenance-api'
import {
  clearPendingCredentialMaintenanceOperation,
  readPendingCredentialMaintenanceOperation,
  savePendingCredentialMaintenanceOperation,
} from '../credential-maintenance-pending-operation'
import { useKeyMaintenanceSession } from '../hooks/use-key-maintenance-session'
import { CredentialMaintenanceBlockers } from './credential-maintenance-blockers'
import { CredentialMaintenanceRotation } from './credential-maintenance-rotation'

const ROOT_ROLE = 100

type CommitPhase =
  | 'idle'
  | 'recovering'
  | 'verifying'
  | 'submitting'
  | 'unknown'
  | 'committed'

function redactedHint(value: string): string {
  if (value.includes('…') || value.includes('...') || value.includes('•')) {
    return value
  }
  if (value.length < 9) return '••••'
  return `${value.slice(0, 4)}…${value.slice(-4)}`
}

export function CredentialMaintenanceDialog(props: {
  tokenId: number
  onClose: () => void
}) {
  const actor = useAuthStore((state) => state.auth.user)
  const sid = useAuthStore((state) => state.auth.session?.sid ?? null)
  if (!actor) return null
  if (actor.role !== ROOT_ROLE) {
    return <CredentialMaintenanceDenied onClose={props.onClose} />
  }
  return (
    <CredentialMaintenanceSession
      key={`${actor.id}:${actor.role}:${sid}:${props.tokenId}`}
      tokenId={props.tokenId}
      userId={actor.id}
      role={actor.role}
      sid={sid}
      onClose={props.onClose}
    />
  )
}

function CredentialMaintenanceSession(props: {
  tokenId: number
  userId: number
  role: number
  sid: string | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  const isCurrentSession = useKeyMaintenanceSession()
  const [pendingAtMount] = useState(() => {
    try {
      return {
        operationId: readPendingCredentialMaintenanceOperation(
          props.userId,
          props.tokenId
        ),
        storageFailed: false,
      }
    } catch {
      return { operationId: null, storageFailed: true }
    }
  })
  const mountId = useRef(createCredentialMigrationOperationId())
  const dialogRef = useRef<HTMLDivElement | null>(null)
  const writeController = useRef<AbortController | null>(null)
  const operationId = useRef(pendingAtMount.operationId ?? '')
  const commitStarted = useRef(Boolean(pendingAtMount.operationId))
  const recoveryStarted = useRef(false)
  const [targetChannelId, setTargetChannelId] = useState<number | null>(null)
  const [targetPools, setTargetPools] = useState<Record<number, number>>({})
  const [preview, setPreview] = useState<CredentialMigrationPreview | null>(
    null
  )
  const [previewing, setPreviewing] = useState(false)
  const [previewError, setPreviewError] = useState(false)
  const [identityConfirmed, setIdentityConfirmed] = useState(false)
  const [allocationConfirmed, setAllocationConfirmed] = useState(false)
  const [phase, setPhase] = useState<CommitPhase>(() => {
    if (pendingAtMount.storageFailed) return 'unknown'
    return pendingAtMount.operationId ? 'recovering' : 'idle'
  })
  const [persistenceBlocked, setPersistenceBlocked] = useState(
    pendingAtMount.storageFailed
  )
  const [receipt, setReceipt] = useState<CredentialMigrationReceipt | null>(
    null
  )
  const [commitError, setCommitError] = useState(false)
  const [receiptChecking, setReceiptChecking] = useState(false)
  const [rotationActivity, setRotationActivity] = useState(false)

  useEffect(() => () => writeController.current?.abort(), [])

  const maintenance = useQuery({
    queryKey: [
      'credential-maintenance',
      props.userId,
      props.role,
      props.sid,
      props.tokenId,
      mountId.current,
    ],
    queryFn: async ({ signal }) => {
      const data = await getCredentialMaintenance(props.tokenId, signal)
      if (
        !isCurrentSession() ||
        data.token.owner_user_id !== props.userId ||
        data.token.id !== props.tokenId
      ) {
        throw new Error('Credential maintenance identity mismatch')
      }
      return data
    },
    enabled: true,
    retry: false,
    staleTime: 0,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })

  const data = maintenance.isError ? undefined : maintenance.data
  const sourceWindows = useMemo(
    () =>
      data?.migration.source?.windows.filter(
        (window) => window.share_bps > 0
      ) ?? [],
    [data]
  )
  const migrationBlockers = useMemo(
    () => [
      ...(data?.migration.blockers ?? []),
      ...(data?.migration.candidates.flatMap(
        (candidate) => candidate.blockers
      ) ?? []),
    ],
    [data]
  )
  const selectedCandidate = data?.migration.candidates.find(
    (candidate) => candidate.channel_id === targetChannelId
  )
  const targetShares = useMemo(() => {
    if (!selectedCandidate || sourceWindows.length === 0) return []
    return sourceWindows.flatMap((source) => {
      const targetPoolId = targetPools[source.pool_id]
      const target = selectedCandidate.windows.find(
        (window) =>
          window.pool_id === targetPoolId &&
          window.window_slot === source.window_slot
      )
      if (!target || target.available_bps < source.share_bps) return []
      return [
        {
          source_pool_id: source.pool_id,
          target_pool_id: target.pool_id,
          share_bps: source.share_bps,
        },
      ]
    })
  }, [selectedCandidate, sourceWindows, targetPools])
  const input: CredentialMigrationInput | null =
    targetChannelId && targetShares.length === sourceWindows.length
      ? { target_channel_id: targetChannelId, target_shares: targetShares }
      : null
  const migrationBlocked = Boolean(
    data && (!data.permissions.can_migrate || data.migration.status !== 'ready')
  )
  const missingSourceWindows = Boolean(
    data &&
    !migrationBlocked &&
    (!data.migration.source || sourceWindows.length === 0)
  )

  const clearReview = () => {
    setPreview(null)
    setPreviewError(false)
    setIdentityConfirmed(false)
    setAllocationConfirmed(false)
    setCommitError(false)
  }

  const selectCandidate = (value: string | null) => {
    if (!value || phase !== 'idle' || persistenceBlocked || rotationActivity) {
      return
    }
    const channelId = Number(value)
    const candidate = data?.migration.candidates.find(
      (item) => item.channel_id === channelId
    )
    if (!candidate || candidate.status !== 'ready') return
    const nextPools: Record<number, number> = {}
    for (const source of sourceWindows) {
      const target = candidate.windows.find(
        (window) =>
          window.window_slot === source.window_slot &&
          window.available_bps >= source.share_bps
      )
      if (target) nextPools[source.pool_id] = target.pool_id
    }
    setTargetChannelId(channelId)
    setTargetPools(nextPools)
    clearReview()
  }

  const requestPreview = async () => {
    if (
      !input ||
      previewing ||
      phase !== 'idle' ||
      persistenceBlocked ||
      rotationActivity ||
      !isCurrentSession()
    ) {
      return
    }
    setPreviewing(true)
    setPreviewError(false)
    setCommitError(false)
    const controller = new AbortController()
    writeController.current = controller
    try {
      const result = await previewCredentialMigration(
        props.tokenId,
        input,
        controller.signal
      )
      if (!isCurrentSession()) return
      setPreview(result)
      setIdentityConfirmed(false)
      setAllocationConfirmed(false)
    } catch {
      if (isCurrentSession()) setPreviewError(true)
    } finally {
      if (isCurrentSession()) setPreviewing(false)
    }
  }

  const readReceipt = useCallback(
    async (automatic: boolean) => {
      if (!operationId.current || !isCurrentSession()) return null
      const expectedOperationId = operationId.current
      if (!automatic) setReceiptChecking(true)
      const controller = new AbortController()
      writeController.current = controller
      try {
        const result = await getCredentialMigrationReceipt(
          props.tokenId,
          expectedOperationId,
          controller.signal
        )
        if (!isCurrentSession()) return null
        setReceipt(result)
        setPhase('committed')
        setCommitError(false)
        try {
          clearPendingCredentialMaintenanceOperation(
            props.userId,
            props.tokenId,
            expectedOperationId
          )
          setPersistenceBlocked(false)
        } catch {
          setPersistenceBlocked(true)
        }
        return result
      } catch {
        if (isCurrentSession()) {
          setPhase('unknown')
          setCommitError(false)
        }
        return null
      } finally {
        if (!automatic && isCurrentSession()) setReceiptChecking(false)
      }
    },
    [isCurrentSession, props.tokenId, props.userId]
  )

  useEffect(() => {
    if (
      phase !== 'recovering' ||
      !operationId.current ||
      recoveryStarted.current
    ) {
      return
    }
    recoveryStarted.current = true
    void readReceipt(true)
  }, [phase, readReceipt])

  const commit = async (proofToken?: string) => {
    if (
      !proofToken ||
      !input ||
      !preview ||
      preview.status !== 'ready' ||
      !preview.preview_revision ||
      !operationId.current ||
      persistenceBlocked ||
      rotationActivity ||
      commitStarted.current ||
      !isCurrentSession()
    ) {
      return null
    }
    const submittedOperationId = operationId.current
    commitStarted.current = true
    try {
      savePendingCredentialMaintenanceOperation(
        props.userId,
        props.tokenId,
        submittedOperationId
      )
      setPersistenceBlocked(false)
    } catch {
      setPersistenceBlocked(true)
      setPhase('idle')
      setCommitError(true)
      return null
    }
    if (!isCurrentSession()) return null
    setPhase('submitting')
    setCommitError(false)
    const controller = new AbortController()
    writeController.current = controller
    try {
      const result = await commitCredentialMigration(
        props.tokenId,
        {
          ...input,
          operation_id: submittedOperationId,
          preview_revision: preview.preview_revision,
          confirmed: true,
        },
        proofToken,
        controller.signal
      )
      if (!isCurrentSession()) return null
      setReceipt(result)
      setPhase('committed')
      try {
        clearPendingCredentialMaintenanceOperation(
          props.userId,
          props.tokenId,
          submittedOperationId
        )
        setPersistenceBlocked(false)
      } catch {
        setPersistenceBlocked(true)
      }
      return result
    } catch (error) {
      if (!isCurrentSession()) return null
      if (isUncertainCredentialCommit(error)) {
        setPhase('unknown')
        await readReceipt(true)
      } else {
        try {
          clearPendingCredentialMaintenanceOperation(
            props.userId,
            props.tokenId,
            submittedOperationId
          )
          operationId.current = ''
          commitStarted.current = false
          setPersistenceBlocked(false)
        } catch {
          setPersistenceBlocked(true)
        }
        setPhase('idle')
        setCommitError(true)
      }
      return null
    }
  }

  const {
    open: verificationOpen,
    methods: verificationMethods,
    state: verificationState,
    startVerification,
    executeVerification,
    cancel: cancelVerification,
    setCode: setVerificationCode,
    switchMethod: switchVerificationMethod,
  } = useSecureVerification()

  const beginCommit = async () => {
    if (
      phase !== 'idle' ||
      persistenceBlocked ||
      rotationActivity ||
      !identityConfirmed ||
      !allocationConfirmed ||
      preview?.status !== 'ready'
    ) {
      return
    }
    operationId.current = createCredentialMigrationOperationId()
    commitStarted.current = false
    setPhase('verifying')
    try {
      const started = await startVerification(commit, {
        scope: 'token.account_migration',
        preferredMethod: 'passkey',
        title: t('Verify account migration'),
        description: t(
          'Use Passkey or 2FA before moving this Key to the selected Codex account.'
        ),
      })
      if (started || !isCurrentSession()) return
    } catch {
      setCommitError(true)
    }
    if (isCurrentSession()) {
      operationId.current = ''
      commitStarted.current = false
      setPhase('idle')
    }
  }

  const reloadCurrentState = () => {
    try {
      const pending = readPendingCredentialMaintenanceOperation(
        props.userId,
        props.tokenId
      )
      setPersistenceBlocked(false)
      setCommitError(false)
      if (pending) {
        operationId.current = pending
        commitStarted.current = true
        recoveryStarted.current = false
        setPhase('recovering')
        return
      }
    } catch {
      setPersistenceBlocked(true)
      setCommitError(true)
      return
    }
    operationId.current = ''
    commitStarted.current = false
    setPhase('idle')
    setTargetChannelId(null)
    setTargetPools({})
    setPreview(null)
    setIdentityConfirmed(false)
    setAllocationConfirmed(false)
    void maintenance.refetch()
  }

  const close = () => {
    if (phase === 'submitting') return
    props.onClose()
  }

  return (
    <>
      <Dialog open onOpenChange={(open) => !open && close()}>
        <DialogContent
          ref={dialogRef}
          initialFocus={dialogRef}
          className='max-h-[calc(100dvh-10rem-env(safe-area-inset-bottom))] min-w-0 overflow-y-auto sm:max-h-[90dvh] sm:max-w-3xl'
          showCloseButton={phase !== 'submitting'}
        >
          <DialogHeader>
            <DialogTitle className='flex items-center gap-2'>
              <KeyRound className='text-primary shrink-0' aria-hidden='true' />
              {t('Credential maintenance')}
            </DialogTitle>
            <DialogDescription>
              {t(
                'Review credential compatibility and move this existing Key to another Codex account.'
              )}
            </DialogDescription>
          </DialogHeader>

          {maintenance.isPending && (
            <div role='status' className='flex flex-col gap-3'>
              <span className='sr-only'>{t('Loading...')}</span>
              <Skeleton className='h-20 rounded-xl' />
              <Skeleton className='h-44 rounded-xl' />
              <Skeleton className='h-64 rounded-xl' />
            </div>
          )}
          {maintenance.isError && (
            <Alert variant='destructive'>
              <AlertTitle>
                {t('Could not load credential maintenance')}
              </AlertTitle>
              <AlertDescription className='flex flex-col items-start gap-3'>
                <span>
                  {t(
                    'The current session may not own this Key, or maintenance data is unavailable.'
                  )}
                </span>
                <Button
                  type='button'
                  variant='outline'
                  disabled={maintenance.isFetching}
                  onClick={() => void maintenance.refetch()}
                >
                  <RefreshCw data-icon='inline-start' aria-hidden='true' />
                  {t('Retry')}
                </Button>
              </AlertDescription>
            </Alert>
          )}

          {data && (
            <div className='flex min-w-0 flex-col gap-5'>
              <section className='bg-muted/30 rounded-xl border p-4'>
                <dl className='grid min-w-0 gap-3 text-sm sm:grid-cols-2'>
                  <div className='min-w-0'>
                    <dt className='text-muted-foreground'>{t('Key')}</dt>
                    <dd className='font-medium break-words'>
                      {data.token.name}
                    </dd>
                  </div>
                  <div className='min-w-0'>
                    <dt className='text-muted-foreground'>
                      {t('Key identifier')}
                    </dt>
                    <dd className='font-mono break-all'>
                      #{data.token.id} · {redactedHint(data.token.masked_key)}
                    </dd>
                  </div>
                </dl>
              </section>

              <CredentialMaintenanceRotation
                rotation={data.rotation}
                token={{
                  ...data.token,
                  masked_key: redactedHint(data.token.masked_key),
                }}
                userId={props.userId}
                otherWriteLocked={
                  phase !== 'idle' || persistenceBlocked || previewing
                }
                onBeforeOpen={() => {
                  setTargetChannelId(null)
                  setTargetPools({})
                  clearReview()
                }}
                onActivityChange={setRotationActivity}
                onCommitted={() => {
                  setTargetChannelId(null)
                  setTargetPools({})
                  clearReview()
                  void maintenance.refetch()
                }}
              />

              <section className='flex min-w-0 flex-col gap-4 rounded-xl border p-4'>
                <div className='flex flex-wrap items-start justify-between gap-3'>
                  <div className='min-w-0'>
                    <h2 className='text-base font-semibold'>
                      {t('Move Key to another Codex account')}
                    </h2>
                    <p className='text-muted-foreground mt-1 text-sm'>
                      {t(
                        'The Key identity and history stay in place while its upstream account changes.'
                      )}
                    </p>
                  </div>
                  <MigrationStatus status={data.migration.status} />
                </div>

                {data.migration.source && (
                  <div className='grid min-w-0 gap-3 rounded-lg border p-3 sm:grid-cols-[1fr_auto_1fr] sm:items-center'>
                    <AccountSummary
                      label={t('Current account')}
                      name={data.migration.source.channel_name}
                      subject={redactedHint(data.migration.source.subject_hint)}
                    />
                    <ArrowRight
                      className='text-muted-foreground hidden sm:block'
                      aria-hidden='true'
                    />
                    <AccountSummary
                      label={t('Target account')}
                      name={
                        selectedCandidate?.channel_name ?? t('Not selected')
                      }
                      subject={
                        selectedCandidate
                          ? redactedHint(selectedCandidate.subject_hint)
                          : '—'
                      }
                    />
                  </div>
                )}

                {migrationBlocked && (
                  <CredentialMaintenanceBlockers blockers={migrationBlockers} />
                )}
                {missingSourceWindows && (
                  <Alert>
                    <AlertTriangle aria-hidden='true' />
                    <AlertTitle>{t('No allocated source windows')}</AlertTitle>
                    <AlertDescription>
                      {t(
                        'Configure this Key’s existing window shares before migrating its account.'
                      )}
                    </AlertDescription>
                  </Alert>
                )}
                {!migrationBlocked && !missingSourceWindows && (
                  <FieldGroup>
                    <Field>
                      <FieldLabel id={`credential-target-${props.tokenId}`}>
                        {t('Target Codex account')}
                      </FieldLabel>
                      <Select
                        value={targetChannelId ? String(targetChannelId) : null}
                        onValueChange={selectCandidate}
                        disabled={
                          phase !== 'idle' ||
                          previewing ||
                          persistenceBlocked ||
                          rotationActivity
                        }
                      >
                        <SelectTrigger
                          className='w-full'
                          aria-labelledby={`credential-target-${props.tokenId}`}
                        >
                          <SelectValue
                            placeholder={t('Select a target account')}
                          >
                            {selectedCandidate
                              ? `${selectedCandidate.channel_name} · ${redactedHint(selectedCandidate.subject_hint)}`
                              : undefined}
                          </SelectValue>
                        </SelectTrigger>
                        <SelectContent>
                          <SelectGroup>
                            {data.migration.candidates.map((candidate) => (
                              <SelectItem
                                key={candidate.channel_id}
                                value={String(candidate.channel_id)}
                                disabled={candidate.status !== 'ready'}
                              >
                                {candidate.channel_name} ·{' '}
                                {redactedHint(candidate.subject_hint)}
                              </SelectItem>
                            ))}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      <FieldDescription>
                        {t(
                          'Only single-credential Codex channels with trusted existing baseline windows can be selected.'
                        )}
                      </FieldDescription>
                    </Field>

                    {selectedCandidate &&
                      sourceWindows.map((source) => {
                        const options = selectedCandidate.windows.filter(
                          (target) =>
                            target.window_slot === source.window_slot &&
                            target.available_bps >= source.share_bps
                        )
                        return (
                          <Field key={source.pool_id}>
                            <FieldLabel
                              id={`credential-window-${source.pool_id}`}
                            >
                              {quotaWindowLabel(source.window_slot, t)}
                            </FieldLabel>
                            <div className='grid min-w-0 gap-2 sm:grid-cols-[1fr_auto_1fr] sm:items-center'>
                              <div className='bg-muted/30 min-w-0 rounded-lg border p-3 text-sm'>
                                <p className='font-medium'>
                                  {t('Current pool')} #{source.pool_id}
                                </p>
                                <p className='text-muted-foreground'>
                                  {t('Share')}: {source.share_bps / 100}%
                                </p>
                              </div>
                              <ArrowRight
                                className='text-muted-foreground hidden sm:block'
                                aria-hidden='true'
                              />
                              <Select
                                value={
                                  targetPools[source.pool_id]
                                    ? String(targetPools[source.pool_id])
                                    : null
                                }
                                onValueChange={(value) => {
                                  if (
                                    !value ||
                                    phase !== 'idle' ||
                                    rotationActivity
                                  ) {
                                    return
                                  }
                                  setTargetPools((current) => ({
                                    ...current,
                                    [source.pool_id]: Number(value),
                                  }))
                                  clearReview()
                                }}
                                disabled={
                                  phase !== 'idle' ||
                                  previewing ||
                                  rotationActivity
                                }
                              >
                                <SelectTrigger
                                  className='w-full'
                                  aria-labelledby={`credential-window-${source.pool_id}`}
                                >
                                  <SelectValue
                                    placeholder={t(
                                      'Select an existing target pool'
                                    )}
                                  >
                                    {targetPools[source.pool_id]
                                      ? `${t('Pool')} #${targetPools[source.pool_id]}`
                                      : undefined}
                                  </SelectValue>
                                </SelectTrigger>
                                <SelectContent>
                                  <SelectGroup>
                                    {options.map((target) => (
                                      <SelectItem
                                        key={target.pool_id}
                                        value={String(target.pool_id)}
                                      >
                                        {t('Pool')} #{target.pool_id} ·{' '}
                                        {t('{{percent}}% available', {
                                          percent: target.available_bps / 100,
                                        })}
                                      </SelectItem>
                                    ))}
                                  </SelectGroup>
                                </SelectContent>
                              </Select>
                            </div>
                            <FieldDescription>
                              {t(
                                'This migration keeps the current {{percent}}% share.',
                                { percent: source.share_bps / 100 }
                              )}
                            </FieldDescription>
                            {options.length === 0 && (
                              <p
                                role='alert'
                                className='text-destructive text-sm'
                              >
                                {t(
                                  'No compatible target pool exists for this window. Configure it in window shares before migrating.'
                                )}
                              </p>
                            )}
                          </Field>
                        )
                      })}

                    <div className='flex flex-wrap gap-2'>
                      <Button
                        type='button'
                        variant='outline'
                        disabled={
                          !input ||
                          previewing ||
                          phase !== 'idle' ||
                          persistenceBlocked ||
                          rotationActivity
                        }
                        onClick={() => void requestPreview()}
                      >
                        {previewing && <Spinner data-icon='inline-start' />}
                        {t('Preview migration')}
                      </Button>
                    </div>
                  </FieldGroup>
                )}

                {previewError && (
                  <Alert variant='destructive'>
                    <AlertTitle>{t('Migration preview failed')}</AlertTitle>
                    <AlertDescription>
                      {t(
                        'No change was submitted. Review the current state and try again.'
                      )}
                    </AlertDescription>
                  </Alert>
                )}
                {preview && preview.status !== 'ready' && (
                  <CredentialMaintenanceBlockers blockers={preview.blockers} />
                )}
                {preview?.status === 'ready' && phase !== 'committed' && (
                  <div className='flex flex-col gap-4 rounded-xl border p-4'>
                    <div>
                      <h3 className='font-semibold'>
                        {t('Confirm migration')}
                      </h3>
                      <p className='text-muted-foreground mt-1 text-sm'>
                        {t(
                          '{{count}} existing window allocations will move without changing their shares.',
                          { count: preview.changes.allocations.length }
                        )}
                      </p>
                    </div>
                    <Field orientation='horizontal'>
                      <Checkbox
                        id={`credential-identity-${props.tokenId}`}
                        checked={identityConfirmed}
                        disabled={phase !== 'idle' || rotationActivity}
                        onCheckedChange={(checked) =>
                          setIdentityConfirmed(checked === true)
                        }
                      />
                      <FieldLabel
                        htmlFor={`credential-identity-${props.tokenId}`}
                      >
                        {t(
                          'I understand this keeps the Key identity, secret, and request history.'
                        )}
                      </FieldLabel>
                    </Field>
                    <Field orientation='horizontal'>
                      <Checkbox
                        id={`credential-allocation-${props.tokenId}`}
                        checked={allocationConfirmed}
                        disabled={phase !== 'idle' || rotationActivity}
                        onCheckedChange={(checked) =>
                          setAllocationConfirmed(checked === true)
                        }
                      />
                      <FieldLabel
                        htmlFor={`credential-allocation-${props.tokenId}`}
                      >
                        {t(
                          'I understand usage estimates remain approximate and the old pools are not closed.'
                        )}
                      </FieldLabel>
                    </Field>
                    {commitError && (
                      <Alert variant='destructive'>
                        <AlertTitle>
                          {t('Migration was not accepted')}
                        </AlertTitle>
                        <AlertDescription className='flex flex-col items-start gap-3'>
                          <span>
                            {t(
                              'Reload maintenance data and create a new preview before trying again.'
                            )}
                          </span>
                          <Button
                            type='button'
                            variant='outline'
                            onClick={reloadCurrentState}
                          >
                            <RefreshCw
                              data-icon='inline-start'
                              aria-hidden='true'
                            />
                            {t('Reload current state')}
                          </Button>
                        </AlertDescription>
                      </Alert>
                    )}
                    <Button
                      type='button'
                      className='h-auto min-h-9 w-full min-w-0 whitespace-normal sm:w-auto'
                      disabled={
                        !identityConfirmed ||
                        !allocationConfirmed ||
                        commitError ||
                        phase !== 'idle' ||
                        rotationActivity
                      }
                      onClick={() => void beginCommit()}
                    >
                      {phase === 'verifying' || phase === 'submitting' ? (
                        <Spinner data-icon='inline-start' />
                      ) : (
                        <ShieldCheck
                          data-icon='inline-start'
                          aria-hidden='true'
                        />
                      )}
                      <span className='min-w-0 break-words'>
                        {t('Verify and move account')}
                      </span>
                    </Button>
                  </div>
                )}

                {(phase === 'unknown' || phase === 'recovering') && (
                  <Alert>
                    <AlertTriangle aria-hidden='true' />
                    <AlertTitle>{t('Migration result is unknown')}</AlertTitle>
                    <AlertDescription className='flex flex-col items-start gap-3'>
                      <span>
                        {t(
                          'The request may have committed. MyAPI will not submit it again. A missing receipt does not prove that no change occurred.'
                        )}
                      </span>
                      {operationId.current && phase !== 'recovering' && (
                        <Button
                          type='button'
                          variant='outline'
                          disabled={receiptChecking}
                          onClick={() => void readReceipt(false)}
                        >
                          {receiptChecking ? (
                            <Spinner data-icon='inline-start' />
                          ) : (
                            <RefreshCw
                              data-icon='inline-start'
                              aria-hidden='true'
                            />
                          )}
                          {t('Check receipt again')}
                        </Button>
                      )}
                      {!operationId.current && persistenceBlocked && (
                        <Button
                          type='button'
                          variant='outline'
                          onClick={reloadCurrentState}
                        >
                          <RefreshCw
                            data-icon='inline-start'
                            aria-hidden='true'
                          />
                          {t('Reload current state')}
                        </Button>
                      )}
                    </AlertDescription>
                  </Alert>
                )}

                {receipt && phase === 'committed' && (
                  <Alert>
                    <CheckCircle2 aria-hidden='true' />
                    <AlertTitle>{t('Account migration committed')}</AlertTitle>
                    <AlertDescription>
                      <p>
                        {t(
                          'Key #{{tokenId}} now uses the selected account. The Key secret was not returned.',
                          { tokenId: receipt.token_id }
                        )}
                      </p>
                      <p className='mt-2 font-mono text-xs break-all'>
                        {receipt.operation_id}
                      </p>
                      <p className='text-muted-foreground mt-1 text-xs'>
                        {t('Committed at')}:{' '}
                        {formatTimestampToDate(receipt.created_at)}
                      </p>
                    </AlertDescription>
                  </Alert>
                )}
              </section>
            </div>
          )}

          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              disabled={phase === 'submitting'}
              onClick={close}
            >
              {phase === 'committed' ? t('Done') : t('Close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <SecureVerificationDialog
        open={verificationOpen}
        onOpenChange={(open) => {
          if (!open && !commitStarted.current) {
            cancelVerification()
            operationId.current = ''
            setPhase('idle')
          }
        }}
        methods={verificationMethods}
        state={verificationState}
        onVerify={async (method, code) => {
          await executeVerification(method, code)
        }}
        onCancel={() => {
          if (commitStarted.current) return
          cancelVerification()
          operationId.current = ''
          setPhase('idle')
        }}
        onCodeChange={setVerificationCode}
        onMethodChange={switchVerificationMethod}
      />
    </>
  )
}

function MigrationStatus(props: {
  status: CredentialMaintenance['migration']['status']
}) {
  const { t } = useTranslation()
  if (props.status === 'ready') return <Badge>{t('Ready')}</Badge>
  if (props.status === 'unknown') {
    return <Badge variant='outline'>{t('Unknown')}</Badge>
  }
  return <Badge variant='outline'>{t('Blocked')}</Badge>
}

function CredentialMaintenanceDenied(props: { onClose: () => void }) {
  const { t } = useTranslation()
  return (
    <Dialog open onOpenChange={(open) => !open && props.onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Credential maintenance')}</DialogTitle>
          <DialogDescription>
            {t('Only Root can maintain credentials for a Key they own.')}
          </DialogDescription>
        </DialogHeader>
        <Alert variant='destructive'>
          <AlertTitle>{t('Root access is required')}</AlertTitle>
          <AlertDescription>
            {t('Only Root can maintain credentials for a Key they own.')}
          </AlertDescription>
        </Alert>
        <DialogFooter>
          <Button type='button' variant='outline' onClick={props.onClose}>
            {t('Close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function AccountSummary(props: {
  label: string
  name: string
  subject: string
}) {
  return (
    <div className='min-w-0'>
      <p className='text-muted-foreground text-xs'>{props.label}</p>
      <p className='mt-1 font-medium break-words'>{props.name}</p>
      <p className='text-muted-foreground mt-1 font-mono text-xs break-all'>
        {props.subject}
      </p>
    </div>
  )
}
