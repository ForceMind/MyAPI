/* Copyright (C) 2026 ForceMind. Licensed under AGPL-3.0-or-later. */
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldLabel } from '@/components/ui/field'
import { SecureVerificationDialog } from '@/features/auth/secure-verification/components/secure-verification-dialog'
import type { SecureVerificationState, VerificationMethod } from '@/features/auth/secure-verification/types'

import { approveOperationsPlan, getOperationsPlan, OperationsError, operationsApprovalBlock, operationsProofScope, operationsSessionKey, operationsVerificationMethods, verifyOperationsApproval, type ApprovalBlock, type OperationsPlan } from '../operations-api'
import { OperationsPlanDetails } from './operations-plan-details'

export function OperationsApprovalDialog(props: { plan: OperationsPlan; scopeKey: string; onClose: () => void; onResult: (plan: OperationsPlan) => void; onUnknown: (plan: OperationsPlan) => void; returnFocus: () => HTMLElement | null }) {
  const { t } = useTranslation()
  const [confirmed, setConfirmed] = useState(false)
  const [busy, setBusy] = useState(false)
  const [verificationOpen, setVerificationOpen] = useState(false)
  const [feedback, setFeedback] = useState('')
  const [terminal, setTerminal] = useState(false)
  const flight = useRef(false)
  const posted = useRef(false)
  const lifetime = useRef(new AbortController())
  useEffect(() => {
    const controller = new AbortController()
    lifetime.current = controller
    return () => controller.abort()
  }, [])
  const isCurrent = useCallback(() => !lifetime.current.signal.aborted && operationsSessionKey() === props.scopeKey, [props.scopeKey])

  function blockMessage(block: ApprovalBlock) {
    if (block === 'changed') return t('This plan changed. Close this review and inspect the current plan; the confirmed target was not replaced.')
    if (block === 'expired') return t('This upgrade plan has expired. Prepare a new plan before approving.')
    if (block === 'approved') return t('This plan is already approved. Approval does not execute the upgrade.')
    return t('Only a prepared upgrade plan can be approved. Inspect its current state before continuing.')
  }
  const initialBlock = operationsApprovalBlock(props.plan, props.plan)

  async function checkPlan(): Promise<boolean> {
    const current = await getOperationsPlan(props.plan.id, lifetime.current.signal)
    if (!isCurrent()) return false
    const block = operationsApprovalBlock(props.plan, current)
    if (block) {
      setFeedback(blockMessage(block))
      setTerminal(true)
      return false
    }
    return true
  }

  async function startVerification() {
    if (!isCurrent() || flight.current || terminal || !confirmed || posted.current) return
    flight.current = true
    setBusy(true)
    setFeedback('')
    try {
      if (await checkPlan() && isCurrent()) setVerificationOpen(true)
    } catch {
      if (isCurrent()) setFeedback(t('The current upgrade plan could not be verified. No approval was submitted.'))
    } finally {
      flight.current = false
      if (isCurrent()) setBusy(false)
    }
  }

  async function approve(proof: string) {
    if (!isCurrent() || flight.current || posted.current || terminal || !confirmed) return
    flight.current = true
    setBusy(true)
    let submitted = false
    try {
      if (!await checkPlan() || !isCurrent()) return
      // Once sent, this review cannot submit again, even after a network failure.
      posted.current = true
      submitted = true
      const approved = await approveOperationsPlan(props.plan, proof, lifetime.current.signal)
      if (!isCurrent()) return
      setTerminal(true)
      setFeedback(t('Upgrade approved. A trusted operations tool must request execution separately.'))
      props.onResult(approved)
    } catch (error) {
      if (!isCurrent()) return
      if (!submitted) {
        setFeedback(t('The current upgrade plan could not be verified. No approval was submitted.'))
      } else if (error instanceof OperationsError && error.kind === 'rejected') {
        setTerminal(true)
        setFeedback(t('Upgrade approval was rejected. Check your session, verification and plan state.'))
      } else {
        setTerminal(true)
        setFeedback(t('Approval result unknown. Inspect the plan with a read-only status check; this approval will not be replayed.'))
        props.onUnknown(props.plan)
      }
    } finally {
      flight.current = false
      if (isCurrent()) { setVerificationOpen(false); setBusy(false) }
    }
  }

  return <>
    <Dialog open onOpenChange={(open) => { if (!open && !busy && !verificationOpen) props.onClose() }} title={t('Review upgrade approval')} description={t('Confirm the exact instance, images and digest. Approval authorizes this plan only; it does not start an upgrade.')} returnFocus={props.returnFocus} showCloseButton={!busy && !verificationOpen} contentClassName='sm:max-w-3xl' footer={<>
      <Button type='button' variant='outline' disabled={busy || verificationOpen} onClick={props.onClose}>{t('Close')}</Button>
      <Button type='button' disabled={!confirmed || busy || terminal || !!initialBlock || verificationOpen} onClick={() => void startVerification()}>{t('Verify and approve this plan')}</Button>
    </>}>
      <div className='space-y-4'>
        {(feedback || initialBlock) && <Alert role='alert'><AlertDescription>{feedback || (initialBlock && blockMessage(initialBlock))}</AlertDescription></Alert>}
        <Alert><AlertDescription>{t('The retained old container will have automatic restart disabled. The new container will keep the current restart policy.')}</AlertDescription></Alert>
        <OperationsPlanDetails plan={props.plan} />
        <Field orientation='horizontal'><Checkbox id='confirm-upgrade-digest' checked={confirmed} disabled={busy || terminal || !!initialBlock || verificationOpen} onCheckedChange={(value) => setConfirmed(value === true)} /><FieldLabel htmlFor='confirm-upgrade-digest'>{t('I confirm this exact upgrade target and plan digest.')}</FieldLabel></Field>
      </div>
    </Dialog>
    {verificationOpen && <OperationsProofDialog plan={props.plan} signal={lifetime.current.signal} isCurrent={isCurrent} onProof={approve} onCancel={() => setVerificationOpen(false)} />}
  </>
}

// Mounted only after the user explicitly reviews and confirms a plan. No status
// or security-method fetch is added to the ordinary SystemInfo page load.
function OperationsProofDialog(props: { plan: OperationsPlan; signal: AbortSignal; isCurrent: () => boolean; onProof: (proof: string) => Promise<void>; onCancel: () => void }) {
  const { t } = useTranslation()
  const [methods, setMethods] = useState({ has2FA: false, hasPasskey: false, passkeySupported: false })
  const [state, setState] = useState<SecureVerificationState>({ method: null, scope: operationsProofScope(props.plan), loading: true, code: '' })
  const [feedback, setFeedback] = useState('')
  const pending = useRef(false)
  const active = useRef(true)
  const verificationSignal = props.signal
  const isCurrentReview = props.isCurrent
  useEffect(() => {
    active.current = true
    async function loadMethods() {
      try {
        const result = await operationsVerificationMethods(verificationSignal)
        if (!active.current || !isCurrentReview()) return
        setMethods(result)
        const method = result.hasPasskey && result.passkeySupported ? 'passkey' : null
        setState((previous) => ({ ...previous, loading: false, method: method ?? (result.has2FA ? '2fa' : null) }))
      } catch {
        if (!active.current || !isCurrentReview()) return
        setFeedback(t('Account verification methods could not be loaded. No approval was submitted.'))
        setState((previous) => ({ ...previous, loading: false }))
      }
    }
    void loadMethods()
    return () => { active.current = false }
  }, [verificationSignal, isCurrentReview, t])

  async function verify(method: VerificationMethod, code?: string) {
    if (pending.current || !props.isCurrent()) return
    pending.current = true
    setState((previous) => ({ ...previous, loading: true, code: '' }))
    setFeedback('')
    try {
      const proof = await verifyOperationsApproval(props.plan, method, code, props.signal, props.isCurrent)
      if (active.current && props.isCurrent()) await props.onProof(proof)
    } catch {
      if (active.current && props.isCurrent()) setFeedback(t('Identity verification failed. No approval was submitted.'))
    } finally {
      pending.current = false
      if (active.current && props.isCurrent()) setState((previous) => ({ ...previous, loading: false, code: '' }))
    }
  }
  return <SecureVerificationDialog open methods={methods} state={{ ...state, title: t('Verify upgrade approval'), description: feedback || t('Use Two-factor Authentication or Passkey to approve only the reviewed upgrade plan.') }} onOpenChange={(open) => { if (!open && !state.loading) props.onCancel() }} onCancel={props.onCancel} onVerify={verify} onCodeChange={(code) => setState((previous) => ({ ...previous, code }))} onMethodChange={(method) => setState((previous) => ({ ...previous, method, code: '' }))} />
}
