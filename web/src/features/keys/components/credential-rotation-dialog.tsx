/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import {
  AlertTriangle,
  CheckCircle2,
  Copy,
  Eye,
  EyeOff,
  RefreshCw,
  RotateCw,
  ShieldCheck,
} from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
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
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { formatTimestampToDate } from '@/lib/format'

import {
  commitCredentialRotation,
  createCredentialRotationOperationId,
  getCredentialRotationCommitDisposition,
  getCredentialRotationReceipt,
  previewCredentialRotation,
  type CredentialRotationPreview,
  type CredentialRotationReceipt,
} from '../credential-rotation-api'
import {
  clearPendingCredentialRotation,
  readPendingCredentialRotation,
  savePendingCredentialRotation,
} from '../credential-rotation-pending-operation'
import { useKeyMaintenanceSession } from '../hooks/use-key-maintenance-session'

type RotationPhase =
  | 'idle'
  | 'previewing'
  | 'review'
  | 'recovering'
  | 'verifying'
  | 'submitting'
  | 'unknown'
  | 'revealed'
  | 'committed'

const mobileActionClass =
  'h-auto min-h-9 w-full min-w-0 whitespace-normal sm:w-auto'

export function CredentialRotationDialog(props: {
  open: boolean
  tokenId: number
  tokenName: string
  maskedKey: string
  userId: number
  onOpenChange: (open: boolean) => void
  onWriteLockChange: (locked: boolean) => void
  onCommitted: (receipt: CredentialRotationReceipt) => void
}) {
  const { t } = useTranslation()
  const isCurrentSession = useKeyMaintenanceSession()
  const [pendingAtMount] = useState(() => {
    try {
      return {
        operationId: readPendingCredentialRotation(props.userId, props.tokenId),
        storageFailed: false,
      }
    } catch {
      return { operationId: null, storageFailed: true }
    }
  })
  const operationId = useRef(pendingAtMount.operationId ?? '')
  const commitStarted = useRef(Boolean(pendingAtMount.operationId))
  const recoveryStarted = useRef(false)
  const receiptInFlight = useRef(false)
  const wasOpen = useRef(props.open)
  const requestController = useRef<AbortController | null>(null)
  const [phase, setPhase] = useState<RotationPhase>(() => {
    if (pendingAtMount.storageFailed) return 'unknown'
    return pendingAtMount.operationId ? 'recovering' : 'idle'
  })
  const [preview, setPreview] = useState<CredentialRotationPreview | null>(null)
  const [receipt, setReceipt] = useState<CredentialRotationReceipt | null>(null)
  const [secret, setSecret] = useState('')
  const [secretVisible, setSecretVisible] = useState(false)
  const [secretSaved, setSecretSaved] = useState(false)
  const [copied, setCopied] = useState(false)
  const [oldValueConfirmed, setOldValueConfirmed] = useState(false)
  const [settlementConfirmed, setSettlementConfirmed] = useState(false)
  const [oneTimeConfirmed, setOneTimeConfirmed] = useState(false)
  const [previewError, setPreviewError] = useState(false)
  const [commitError, setCommitError] = useState(false)
  const [receiptChecking, setReceiptChecking] = useState(false)
  const [persistenceBlocked, setPersistenceBlocked] = useState(
    pendingAtMount.storageFailed
  )

  const writeLocked =
    commitStarted.current ||
    phase === 'recovering' ||
    phase === 'submitting' ||
    phase === 'unknown' ||
    persistenceBlocked

  useEffect(() => {
    props.onWriteLockChange(writeLocked)
  }, [props, writeLocked])

  useEffect(
    () => () => {
      requestController.current?.abort()
    },
    []
  )

  useEffect(() => {
    if (props.open) return
    setSecret('')
    setSecretVisible(false)
    setCopied(false)
  }, [props.open])

  const readReceipt = useCallback(async () => {
    if (
      !operationId.current ||
      receiptInFlight.current ||
      !isCurrentSession()
    ) {
      return null
    }
    const expectedOperationId = operationId.current
    receiptInFlight.current = true
    setReceiptChecking(true)
    const controller = new AbortController()
    requestController.current = controller
    try {
      const result = await getCredentialRotationReceipt(
        props.tokenId,
        expectedOperationId,
        controller.signal
      )
      if (!isCurrentSession()) return null
      setReceipt(result)
      setSecret('')
      setPhase('committed')
      setCommitError(false)
      try {
        clearPendingCredentialRotation(
          props.userId,
          props.tokenId,
          expectedOperationId
        )
        operationId.current = ''
        commitStarted.current = false
        setPersistenceBlocked(false)
      } catch {
        setPersistenceBlocked(true)
      }
      props.onCommitted(result)
      return result
    } catch {
      if (isCurrentSession()) {
        setPhase('unknown')
        setCommitError(false)
      }
      return null
    } finally {
      receiptInFlight.current = false
      if (isCurrentSession()) setReceiptChecking(false)
    }
  }, [isCurrentSession, props])

  useEffect(() => {
    if (
      phase !== 'recovering' ||
      !operationId.current ||
      recoveryStarted.current
    ) {
      return
    }
    recoveryStarted.current = true
    void readReceipt()
  }, [phase, readReceipt])

  useEffect(() => {
    const reopened = props.open && !wasOpen.current
    wasOpen.current = props.open
    if (reopened && phase === 'unknown' && operationId.current) {
      void readReceipt()
    }
  }, [phase, props.open, readReceipt])

  const requestPreview = async () => {
    if (phase !== 'idle' || persistenceBlocked || !isCurrentSession()) return
    setPhase('previewing')
    setPreviewError(false)
    setCommitError(false)
    const controller = new AbortController()
    requestController.current = controller
    try {
      const result = await previewCredentialRotation(
        props.tokenId,
        controller.signal
      )
      if (!isCurrentSession()) return
      setPreview(result)
      setOldValueConfirmed(false)
      setSettlementConfirmed(false)
      setOneTimeConfirmed(false)
      setPhase('review')
    } catch {
      if (!isCurrentSession()) return
      setPreview(null)
      setPreviewError(true)
      setPhase('idle')
    }
  }

  const commit = async (proofToken?: string) => {
    if (
      !proofToken ||
      !preview ||
      preview.status !== 'ready' ||
      !preview.preview_revision ||
      !operationId.current ||
      persistenceBlocked ||
      commitStarted.current ||
      !isCurrentSession()
    ) {
      return null
    }
    const submittedOperationId = operationId.current
    commitStarted.current = true
    try {
      savePendingCredentialRotation(
        props.userId,
        props.tokenId,
        submittedOperationId
      )
      setPersistenceBlocked(false)
    } catch {
      setPersistenceBlocked(true)
      setCommitError(true)
      setPhase('review')
      return null
    }
    if (!isCurrentSession()) return null
    setPhase('submitting')
    setCommitError(false)
    const controller = new AbortController()
    requestController.current = controller
    try {
      const result = await commitCredentialRotation(
        props.tokenId,
        {
          operation_id: submittedOperationId,
          preview_revision: preview.preview_revision,
          confirmed: true,
        },
        proofToken,
        controller.signal
      )
      if (!isCurrentSession()) return null
      setReceipt(result.receipt)
      setSecret(result.reveal?.secret ?? '')
      setSecretVisible(Boolean(result.reveal))
      setSecretSaved(false)
      setCopied(false)
      setPhase(result.reveal ? 'revealed' : 'committed')
      try {
        clearPendingCredentialRotation(
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
      props.onCommitted(result.receipt)
      return result
    } catch (error) {
      if (!isCurrentSession()) return null
      if (getCredentialRotationCommitDisposition(error) === 'not_accepted') {
        try {
          clearPendingCredentialRotation(
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
        setPreview(null)
        setPhase('idle')
        setCommitError(true)
      } else {
        setPhase('unknown')
        await readReceipt()
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
      phase !== 'review' ||
      preview?.status !== 'ready' ||
      !oldValueConfirmed ||
      !settlementConfirmed ||
      !oneTimeConfirmed ||
      persistenceBlocked
    ) {
      return
    }
    operationId.current = createCredentialRotationOperationId()
    commitStarted.current = false
    setPhase('verifying')
    try {
      const started = await startVerification(commit, {
        scope: 'token.credential_rotation',
        preferredMethod: 'passkey',
        title: t('Verify credential rotation'),
        description: t('Use Passkey or 2FA before rotating this Key value.'),
      })
      if (started || !isCurrentSession()) return
    } catch {
      setCommitError(true)
    }
    if (isCurrentSession()) {
      operationId.current = ''
      commitStarted.current = false
      setPhase('review')
    }
  }

  const reloadCurrentState = () => {
    try {
      const pending = readPendingCredentialRotation(props.userId, props.tokenId)
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
    setPreview(null)
    setReceipt(null)
    setSecret('')
    setPhase('idle')
  }

  const resetRejectedOperation = () => {
    if (!operationId.current) {
      setPersistenceBlocked(false)
      return
    }
    const expectedOperationId = operationId.current
    try {
      clearPendingCredentialRotation(
        props.userId,
        props.tokenId,
        expectedOperationId
      )
      operationId.current = ''
      commitStarted.current = false
      setPersistenceBlocked(false)
      setPreview(null)
      setCommitError(true)
      setPhase('idle')
    } catch {
      setPersistenceBlocked(true)
    }
  }

  const retryClearCommittedState = () => {
    if (!operationId.current) return
    const expectedOperationId = operationId.current
    try {
      clearPendingCredentialRotation(
        props.userId,
        props.tokenId,
        expectedOperationId
      )
      operationId.current = ''
      commitStarted.current = false
      setPersistenceBlocked(false)
    } catch {
      setPersistenceBlocked(true)
    }
  }

  const copySecret = async () => {
    if (!secret) return
    try {
      await navigator.clipboard.writeText(secret)
      if (isCurrentSession()) setCopied(true)
    } catch {
      if (isCurrentSession()) setCopied(false)
    }
  }

  const close = () => {
    if (
      phase === 'submitting' ||
      phase === 'verifying' ||
      (phase === 'revealed' && !secretSaved)
    ) {
      return
    }
    if (phase === 'previewing') requestController.current?.abort()
    setSecret('')
    setSecretVisible(false)
    setCopied(false)
    if (phase === 'revealed' && writeLocked) {
      setPhase('committed')
    } else if (phase !== 'unknown' && phase !== 'recovering' && !writeLocked) {
      setPreview(null)
      setReceipt(null)
      setOldValueConfirmed(false)
      setSettlementConfirmed(false)
      setOneTimeConfirmed(false)
      setPreviewError(false)
      setCommitError(false)
      setPhase('idle')
    }
    props.onOpenChange(false)
  }

  const previewReady = preview?.status === 'ready'
  let previewStatusLabel = t('Blocked')
  if (preview?.status === 'ready') previewStatusLabel = t('Ready')
  if (preview?.status === 'unknown') previewStatusLabel = t('Unknown')

  return (
    <>
      <Dialog open={props.open} onOpenChange={(open) => !open && close()}>
        <DialogContent
          className='max-h-[calc(100dvh-3rem-env(safe-area-inset-bottom))] min-w-0 overflow-y-auto sm:max-h-[90dvh] sm:max-w-xl'
          showCloseButton={
            phase !== 'submitting' &&
            phase !== 'verifying' &&
            (phase !== 'revealed' || secretSaved)
          }
        >
          <DialogHeader>
            <DialogTitle className='flex items-center gap-2'>
              <RotateCw className='text-primary shrink-0' aria-hidden='true' />
              {t('Credential rotation')}
            </DialogTitle>
            <DialogDescription>
              {t(
                'Rotate this Key value without changing its Token ID or settlement writer.'
              )}
            </DialogDescription>
          </DialogHeader>

          <section className='bg-muted/30 rounded-xl border p-4 text-sm'>
            <p className='font-medium break-words'>{props.tokenName}</p>
            <p className='text-muted-foreground mt-1 font-mono break-all'>
              #{props.tokenId} · {props.maskedKey}
            </p>
          </section>

          {(phase === 'idle' || phase === 'previewing') && (
            <div className='flex flex-col gap-4'>
              <Alert>
                <ShieldCheck aria-hidden='true' />
                <AlertTitle>{t('Check rotation eligibility')}</AlertTitle>
                <AlertDescription>
                  {t(
                    'MyAPI will verify the current settlement path before allowing rotation.'
                  )}
                </AlertDescription>
              </Alert>
              {previewError && (
                <Alert variant='destructive'>
                  <AlertTitle>
                    {t('Could not check rotation eligibility')}
                  </AlertTitle>
                  <AlertDescription>
                    {t('Reload maintenance data and try the check again.')}
                  </AlertDescription>
                </Alert>
              )}
              {commitError && (
                <Alert variant='destructive'>
                  <AlertTitle>{t('Rotation was not accepted')}</AlertTitle>
                  <AlertDescription>
                    {t('Run a new eligibility check before rotating again.')}
                  </AlertDescription>
                </Alert>
              )}
              {commitError && persistenceBlocked && (
                <Button
                  className={mobileActionClass}
                  type='button'
                  variant='outline'
                  onClick={resetRejectedOperation}
                >
                  <RefreshCw data-icon='inline-start' aria-hidden='true' />
                  {t('Reload current state')}
                </Button>
              )}
              <Button
                className={mobileActionClass}
                type='button'
                disabled={phase === 'previewing' || persistenceBlocked}
                onClick={() => void requestPreview()}
              >
                {phase === 'previewing' && <Spinner data-icon='inline-start' />}
                {t('Check rotation eligibility')}
              </Button>
            </div>
          )}

          {phase === 'review' && preview && (
            <div className='flex flex-col gap-4'>
              <div className='flex items-center justify-between gap-3'>
                <h3 className='font-semibold'>{t('Rotation preview')}</h3>
                <Badge variant={previewReady ? 'default' : 'outline'}>
                  {previewStatusLabel}
                </Badge>
              </div>
              {previewReady ? (
                <Alert>
                  <CheckCircle2 aria-hidden='true' />
                  <AlertTitle>{t('Rotation is ready')}</AlertTitle>
                  <AlertDescription>
                    <ul className='list-disc space-y-1 pl-4'>
                      <li>
                        {t(
                          'The old Key value stops working immediately after rotation.'
                        )}
                      </li>
                      <li>
                        {t(
                          'Requests accepted before rotation can continue settling against this Token.'
                        )}
                      </li>
                      <li>{t('The settlement writer stays unchanged.')}</li>
                    </ul>
                  </AlertDescription>
                </Alert>
              ) : (
                <Alert>
                  <AlertTriangle aria-hidden='true' />
                  <AlertTitle>
                    {preview.status === 'unknown'
                      ? t('Rotation eligibility is unknown')
                      : t('Rotation is blocked')}
                  </AlertTitle>
                  <AlertDescription>
                    {t(
                      'Keep the current Key value and check eligibility again after the current qualification checks are resolved.'
                    )}
                  </AlertDescription>
                </Alert>
              )}
              {previewReady && (
                <div className='flex flex-col gap-3'>
                  <Field orientation='horizontal'>
                    <Checkbox
                      id={`rotation-old-value-${props.tokenId}`}
                      checked={oldValueConfirmed}
                      disabled={phase !== 'review'}
                      onCheckedChange={(checked) =>
                        setOldValueConfirmed(checked === true)
                      }
                    />
                    <FieldLabel htmlFor={`rotation-old-value-${props.tokenId}`}>
                      {t(
                        'I understand the old Key value will stop working immediately.'
                      )}
                    </FieldLabel>
                  </Field>
                  <Field orientation='horizontal'>
                    <Checkbox
                      id={`rotation-settlement-${props.tokenId}`}
                      checked={settlementConfirmed}
                      disabled={phase !== 'review'}
                      onCheckedChange={(checked) =>
                        setSettlementConfirmed(checked === true)
                      }
                    />
                    <FieldLabel
                      htmlFor={`rotation-settlement-${props.tokenId}`}
                    >
                      {t(
                        'I understand accepted requests may continue settling after rotation.'
                      )}
                    </FieldLabel>
                  </Field>
                  <Field orientation='horizontal'>
                    <Checkbox
                      id={`rotation-once-${props.tokenId}`}
                      checked={oneTimeConfirmed}
                      disabled={phase !== 'review'}
                      onCheckedChange={(checked) =>
                        setOneTimeConfirmed(checked === true)
                      }
                    />
                    <FieldLabel htmlFor={`rotation-once-${props.tokenId}`}>
                      {t(
                        'I understand the new Key value is shown automatically only once.'
                      )}
                    </FieldLabel>
                  </Field>
                  {commitError && (
                    <Alert variant='destructive'>
                      <AlertTitle>{t('Rotation was not accepted')}</AlertTitle>
                      <AlertDescription>
                        {t(
                          'Reload maintenance data and run a new eligibility check.'
                        )}
                      </AlertDescription>
                    </Alert>
                  )}
                  {commitError && persistenceBlocked && (
                    <Button
                      className={mobileActionClass}
                      type='button'
                      variant='outline'
                      onClick={resetRejectedOperation}
                    >
                      <RefreshCw data-icon='inline-start' aria-hidden='true' />
                      {t('Reload current state')}
                    </Button>
                  )}
                  <Button
                    className={mobileActionClass}
                    type='button'
                    disabled={
                      !oldValueConfirmed ||
                      !settlementConfirmed ||
                      !oneTimeConfirmed ||
                      persistenceBlocked
                    }
                    onClick={() => void beginCommit()}
                  >
                    <ShieldCheck data-icon='inline-start' aria-hidden='true' />
                    {t('Verify and rotate Key')}
                  </Button>
                </div>
              )}
            </div>
          )}

          {(phase === 'recovering' || phase === 'unknown') && (
            <Alert>
              <AlertTriangle aria-hidden='true' />
              <AlertTitle>{t('Rotation result is unknown')}</AlertTitle>
              <AlertDescription className='flex flex-col items-start gap-3'>
                <span>
                  {t(
                    'The request may have committed. MyAPI will not rotate it again automatically.'
                  )}
                </span>
                <span>
                  {t(
                    'A missing receipt does not prove that the old Key value still works.'
                  )}
                </span>
                {operationId.current && phase !== 'recovering' && (
                  <Button
                    className={mobileActionClass}
                    type='button'
                    variant='outline'
                    disabled={receiptChecking}
                    onClick={() => void readReceipt()}
                  >
                    {receiptChecking ? (
                      <Spinner data-icon='inline-start' />
                    ) : (
                      <RefreshCw data-icon='inline-start' aria-hidden='true' />
                    )}
                    {t('Check rotation receipt')}
                  </Button>
                )}
                {!operationId.current && persistenceBlocked && (
                  <Button
                    className={mobileActionClass}
                    type='button'
                    variant='outline'
                    onClick={reloadCurrentState}
                  >
                    <RefreshCw data-icon='inline-start' aria-hidden='true' />
                    {t('Reload current state')}
                  </Button>
                )}
              </AlertDescription>
            </Alert>
          )}

          {(phase === 'verifying' || phase === 'submitting') && (
            <div role='status' className='flex items-center gap-3 text-sm'>
              <Spinner aria-hidden='true' />
              <span>
                {phase === 'verifying'
                  ? t('Waiting for security verification…')
                  : t('Rotating Key value…')}
              </span>
            </div>
          )}

          {phase === 'revealed' && receipt && secret && (
            <div className='flex flex-col gap-4'>
              <Alert>
                <CheckCircle2 aria-hidden='true' />
                <AlertTitle>{t('Key value rotated')}</AlertTitle>
                <AlertDescription>
                  {t(
                    'Copy the new Key value now. It will not be shown automatically again.'
                  )}
                </AlertDescription>
              </Alert>
              <div className='flex flex-col gap-2'>
                <label
                  className='text-sm font-medium'
                  htmlFor={`rotated-key-value-${props.tokenId}`}
                >
                  {t('New Key value')}
                </label>
                <div className='flex min-w-0 flex-col gap-2 sm:flex-row'>
                  <Input
                    id={`rotated-key-value-${props.tokenId}`}
                    className='min-w-0 font-mono'
                    type={secretVisible ? 'text' : 'password'}
                    readOnly
                    value={secret}
                  />
                  <Button
                    className={mobileActionClass}
                    type='button'
                    variant='outline'
                    onClick={() => setSecretVisible((visible) => !visible)}
                  >
                    {secretVisible ? (
                      <EyeOff data-icon='inline-start' aria-hidden='true' />
                    ) : (
                      <Eye data-icon='inline-start' aria-hidden='true' />
                    )}
                    {secretVisible ? t('Hide value') : t('Show value')}
                  </Button>
                  <Button
                    className={mobileActionClass}
                    type='button'
                    variant='outline'
                    onClick={() => void copySecret()}
                  >
                    <Copy data-icon='inline-start' aria-hidden='true' />
                    {copied ? t('Copied') : t('Copy value')}
                  </Button>
                </div>
              </div>
              <Field orientation='horizontal'>
                <Checkbox
                  id={`rotation-saved-${props.tokenId}`}
                  checked={secretSaved}
                  onCheckedChange={(checked) =>
                    setSecretSaved(checked === true)
                  }
                />
                <FieldLabel htmlFor={`rotation-saved-${props.tokenId}`}>
                  {t('I saved the new Key value.')}
                </FieldLabel>
              </Field>
            </div>
          )}

          {phase === 'committed' && receipt && (
            <div className='flex flex-col gap-3'>
              <Alert>
                <CheckCircle2 aria-hidden='true' />
                <AlertTitle>{t('Credential rotation committed')}</AlertTitle>
                <AlertDescription>
                  <p>{t('The old Key value is no longer valid.')}</p>
                  <p className='mt-2'>
                    {t(
                      'This receipt does not contain the new Key value. Use Reveal Key from the Key menu when you need to view the current value.'
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
              {persistenceBlocked && (
                <Button
                  className={mobileActionClass}
                  type='button'
                  variant='outline'
                  onClick={retryClearCommittedState}
                >
                  <RefreshCw data-icon='inline-start' aria-hidden='true' />
                  {t('Reload current state')}
                </Button>
              )}
            </div>
          )}

          <DialogFooter>
            <Button
              className={mobileActionClass}
              type='button'
              variant='outline'
              disabled={
                phase === 'submitting' ||
                phase === 'verifying' ||
                (phase === 'revealed' && !secretSaved)
              }
              onClick={close}
            >
              {phase === 'revealed' || phase === 'committed'
                ? t('Done')
                : t('Close')}
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
            setPhase('review')
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
          setPhase('review')
        }}
        onCodeChange={setVerificationCode}
        onMethodChange={switchVerificationMethod}
      />
    </>
  )
}
