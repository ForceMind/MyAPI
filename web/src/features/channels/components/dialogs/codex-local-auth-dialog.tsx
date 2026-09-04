/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import {
  Copy01Icon,
  CopyCheckIcon,
  Loading03Icon,
  Refresh01Icon,
  TerminalIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
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
import { Textarea } from '@/components/ui/textarea'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { isVerificationRequiredError } from '@/lib/secure-verification'

import {
  getCodexLocalAuthStatus,
  importCodexLocalAuthForChannel,
  importCodexLocalAuthForNewChannel,
  type CodexLocalAuthImportResponse,
} from '../../api'
import type { AddChannelRequest, CodexLocalAuthStatus } from '../../types'

type CodexLocalAuthDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelId?: number
  disabled?: boolean
  getCreatePayload?: () => Promise<AddChannelRequest | null>
  onImportSuccess: (response: CodexLocalAuthImportResponse) => void
  onKeyImported: (key: string) => void
}

const UNIX_EXPORT_COMMAND =
  "node -e \"const fs=require('fs'),path=require('path'),os=require('os');const home=process.env.CODEX_HOME||path.join(os.homedir(),'.codex');const a=JSON.parse(fs.readFileSync(path.join(home,'auth.json'),'utf8'));const t=a.tokens||a;process.stdout.write(JSON.stringify({id_token:t.id_token,access_token:t.access_token,refresh_token:t.refresh_token,account_id:t.account_id,last_refresh:a.last_refresh,type:'codex'}))\""

const WINDOWS_EXPORT_COMMAND =
  "node -e \"const fs=require('fs'),path=require('path'),os=require('os');const home=process.env.CODEX_HOME||path.join(process.env.USERPROFILE||os.homedir(),'.codex');const a=JSON.parse(fs.readFileSync(path.join(home,'auth.json'),'utf8'));const t=a.tokens||a;process.stdout.write(JSON.stringify({id_token:t.id_token,access_token:t.access_token,refresh_token:t.refresh_token,account_id:t.account_id,last_refresh:a.last_refresh,type:'codex'}))\""

function getErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

function getCodexLocalAuthErrorCode(error: unknown): string | undefined {
  if (!error || typeof error !== 'object' || !('response' in error)) {
    return undefined
  }
  const response = error.response
  if (!response || typeof response !== 'object' || !('data' in response)) {
    return undefined
  }
  const data = response.data
  if (data === null || typeof data !== 'object' || !('code' in data)) {
    return undefined
  }
  return typeof data.code === 'string' ? data.code : undefined
}

function isCodexLocalAuthConflict(error: unknown): boolean {
  return getCodexLocalAuthErrorCode(error) === 'CODEX_LOCAL_AUTH_CONFLICT'
}

function isManualCredential(value: string): boolean {
  try {
    const parsed: unknown = JSON.parse(value)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return false
    }
    const credential = parsed as Record<string, unknown>
    return (
      typeof credential.access_token === 'string' &&
      credential.access_token.trim().length > 0 &&
      typeof credential.account_id === 'string' &&
      credential.account_id.trim().length > 0
    )
  } catch {
    return false
  }
}

function hasRefreshToken(value: string): boolean {
  try {
    const parsed = JSON.parse(value) as Record<string, unknown>
    return (
      typeof parsed.refresh_token === 'string' &&
      parsed.refresh_token.trim().length > 0
    )
  } catch {
    return false
  }
}

function statusMessage(
  status: CodexLocalAuthStatus,
  t: (key: string) => string
) {
  switch (status.state) {
    case 'ready':
      return t('Local Codex account is ready to import.')
    case 'not_found':
      return t('No Codex login file was found for the MyAPI process user.')
    case 'unreadable':
    case 'unsafe_file':
    case 'too_large':
    case 'changed_during_read':
      return t('MyAPI cannot read the local Codex login file.')
    case 'invalid_json':
    case 'unsupported_auth_method':
    case 'incomplete_credential':
      return t('The local Codex login file is invalid or incomplete.')
    case 'container_host_unavailable':
      return t(
        'MyAPI is running in a container and cannot access the host Codex login.'
      )
    case 'lan':
      return t('Local Codex import is unavailable in LAN Lite.')
    case 'disabled':
      return t('Local Codex import is disabled for this MyAPI environment.')
    default:
      return t('Unable to determine the local Codex login status.')
  }
}

export function CodexLocalAuthDialog(props: CodexLocalAuthDialogProps) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const [status, setStatus] = useState<CodexLocalAuthStatus | null>(null)
  const [statusError, setStatusError] = useState('')
  const [isChecking, setIsChecking] = useState(false)
  const [isImporting, setIsImporting] = useState(false)
  const [manualMode, setManualMode] = useState(false)
  const [manualCredential, setManualCredential] = useState('')
  const [importError, setImportError] = useState('')
  const manualCredentialRef = useRef<HTMLTextAreaElement>(null)

  const clearManualCredential = useCallback(() => {
    setManualCredential('')
    setManualMode(false)
  }, [])

  const handleDialogOpenChange = useCallback(
    (open: boolean) => {
      if (!open) {
        clearManualCredential()
      }
      props.onOpenChange(open)
    },
    [clearManualCredential, props]
  )

  const handleImportSuccess = useCallback(
    (response: CodexLocalAuthImportResponse) => {
      if (!response.success) {
        setImportError(t('Failed to import local Codex credential'))
        toast.error(t('Failed to import local Codex credential'))
        return
      }
      clearManualCredential()
      toast.success(t('Local Codex credential imported'))
      props.onImportSuccess(response)
      props.onOpenChange(false)
    },
    [clearManualCredential, props, t]
  )

  const {
    open: verificationOpen,
    methods: verificationMethods,
    state: verificationState,
    executeVerification,
    withVerification,
    cancel: cancelVerification,
    setCode: setVerificationCode,
    switchMethod: switchVerificationMethod,
  } = useSecureVerification({
    onSuccess: (result) => {
      handleImportSuccess(result as CodexLocalAuthImportResponse)
    },
  })

  const refreshStatus = useCallback(async () => {
    setIsChecking(true)
    setStatusError('')
    setImportError('')
    try {
      const response = await getCodexLocalAuthStatus()
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Unable to check local Codex status')
        )
      }
      setStatus(response.data)
    } catch (error) {
      if (getCodexLocalAuthErrorCode(error) === 'MYAPI_LAN_ROUTE_DISABLED') {
        setStatus({
          state: 'lan',
          platform: 'unknown',
          environment: 'lan',
          codex_installed: false,
          auth_file_exists: false,
          auth_readable: false,
          logged_in: false,
          auto_import_available: false,
          manual_import_available: true,
          can_refresh: false,
        })
        return
      }
      setStatus(null)
      setStatusError(
        getErrorMessage(error, t('Unable to check local Codex status'))
      )
    } finally {
      setIsChecking(false)
    }
  }, [t])

  useEffect(() => {
    if (!props.open) {
      setStatus(null)
      setStatusError('')
      setImportError('')
      clearManualCredential()
      return
    }
    void refreshStatus()
  }, [clearManualCredential, props.open, refreshStatus])

  useEffect(() => {
    if (manualMode) manualCredentialRef.current?.focus()
  }, [manualMode])

  const canAutoImport =
    status?.state === 'ready' &&
    status.auto_import_available &&
    !props.disabled &&
    !isImporting
  const canManualImport = status?.manual_import_available !== false
  const manualCredentialValid = useMemo(
    () => isManualCredential(manualCredential),
    [manualCredential]
  )
  const manualCredentialHasRefreshToken = useMemo(
    () => hasRefreshToken(manualCredential),
    [manualCredential]
  )

  const importCredential = useCallback(
    async (proofToken?: string): Promise<CodexLocalAuthImportResponse> => {
      try {
        if (props.channelId) {
          return await importCodexLocalAuthForChannel(
            props.channelId,
            proofToken
          )
        }
        if (!props.getCreatePayload) {
          throw new Error(t('Channel details are required before import.'))
        }
        const payload = await props.getCreatePayload()
        if (!payload) {
          throw new Error(
            t('Complete the required channel fields before import.')
          )
        }
        return await importCodexLocalAuthForNewChannel(payload, proofToken)
      } catch (error) {
        if (isVerificationRequiredError(error)) throw error
        if (isCodexLocalAuthConflict(error)) {
          throw new Error(t('Save failed, please retry'))
        }
        throw new Error(t('Failed to import local Codex credential'))
      }
    },
    [props, t]
  )

  const handleAutoImport = useCallback(async () => {
    if (!canAutoImport) return
    setIsImporting(true)
    setImportError('')
    try {
      const result = await withVerification(importCredential, {
        scope: 'channel.codex.local_import',
        preferredMethod: 'passkey',
        title: t('Verify to import local Codex credential'),
        description: t(
          'Use Passkey or 2FA before MyAPI reads and imports the local Codex login.'
        ),
      })
      if (result) {
        handleImportSuccess(result as CodexLocalAuthImportResponse)
      }
    } catch (error) {
      const message = getErrorMessage(
        error,
        t('Failed to import local Codex credential')
      )
      setImportError(message)
      toast.error(message)
    } finally {
      setIsImporting(false)
    }
  }, [
    canAutoImport,
    handleImportSuccess,
    importCredential,
    t,
    withVerification,
  ])

  const handleManualImport = useCallback(() => {
    if (!manualCredentialValid) {
      toast.error(
        t(
          'Codex credential must be a JSON object with access_token and account_id'
        )
      )
      return
    }
    props.onKeyImported(manualCredential.trim())
    clearManualCredential()
    toast.success(t('Codex credential filled into the channel form'))
    props.onOpenChange(false)
  }, [clearManualCredential, manualCredential, manualCredentialValid, props, t])

  const handleManualModeChange = useCallback((enabled: boolean) => {
    setManualCredential('')
    setManualMode(enabled)
  }, [])

  return (
    <>
      <Dialog open={props.open} onOpenChange={handleDialogOpenChange}>
        <DialogContent className='max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl'>
          <DialogHeader>
            <DialogTitle>{t('Import local Codex')}</DialogTitle>
            <DialogDescription>
              {t(
                'MyAPI checks the Codex login used by this MyAPI process. Imported credentials are saved on the server and are never returned to your browser.'
              )}
            </DialogDescription>
          </DialogHeader>

          <FieldGroup className='gap-4'>
            {isChecking && (
              <Alert>
                <HugeiconsIcon
                  icon={Loading03Icon}
                  className='animate-spin'
                  aria-hidden='true'
                />
                <AlertTitle>{t('Checking local Codex login')}</AlertTitle>
                <AlertDescription>
                  {t(
                    'Checking the local Codex login state for this MyAPI process.'
                  )}
                </AlertDescription>
              </Alert>
            )}

            {statusError && (
              <Alert variant='destructive'>
                <AlertTitle>
                  {t('Unable to check local Codex status')}
                </AlertTitle>
                <AlertDescription>{statusError}</AlertDescription>
              </Alert>
            )}

            {status && (
              <Alert
                variant={status.state === 'ready' ? 'default' : 'destructive'}
              >
                <AlertTitle>
                  {status.state === 'ready'
                    ? t('Local Codex account detected')
                    : t('Local Codex import is unavailable')}
                </AlertTitle>
                <AlertDescription>
                  <div className='grid gap-1'>
                    <p>{statusMessage(status, t)}</p>
                    {status.account_hint && (
                      <p>
                        {t('Connected account: {{account}}', {
                          account: status.account_hint,
                        })}
                      </p>
                    )}
                    {status.cli_version && (
                      <p>
                        {t('Codex CLI: {{version}}', {
                          version: status.cli_version,
                        })}
                      </p>
                    )}
                  </div>
                </AlertDescription>
              </Alert>
            )}

            {importError && (
              <Alert variant='destructive'>
                <AlertTitle>
                  {t('Failed to import local Codex credential')}
                </AlertTitle>
                <AlertDescription>{importError}</AlertDescription>
              </Alert>
            )}

            {!manualMode ? (
              <div className='flex flex-wrap gap-2'>
                <Button
                  type='button'
                  onClick={handleAutoImport}
                  disabled={!canAutoImport}
                >
                  {isImporting ? (
                    <HugeiconsIcon
                      icon={Loading03Icon}
                      data-icon='inline-start'
                      className='animate-spin'
                      aria-hidden='true'
                    />
                  ) : (
                    <HugeiconsIcon
                      icon={TerminalIcon}
                      data-icon='inline-start'
                      aria-hidden='true'
                    />
                  )}
                  {isImporting ? t('Importing...') : t('Import local Codex')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  onClick={() => void refreshStatus()}
                  disabled={isChecking || props.disabled}
                >
                  <HugeiconsIcon
                    icon={Refresh01Icon}
                    data-icon='inline-start'
                    aria-hidden='true'
                  />
                  {t('Check again')}
                </Button>
                {canManualImport && (
                  <Button
                    type='button'
                    variant='outline'
                    onClick={() => handleManualModeChange(true)}
                    disabled={props.disabled}
                  >
                    {t('Manual import')}
                  </Button>
                )}
              </div>
            ) : (
              <FieldGroup className='gap-4'>
                <Alert>
                  <AlertDescription>
                    {t(
                      'Run the matching command on the machine that owns the Codex login, then paste only its JSON output below. The pasted value stays in memory and is cleared after use.'
                    )}
                  </AlertDescription>
                </Alert>

                <CommandField
                  label={t('macOS / Linux')}
                  command={UNIX_EXPORT_COMMAND}
                  copiedText={copiedText}
                  onCopy={copyToClipboard}
                />
                <CommandField
                  label={t('Windows PowerShell')}
                  command={WINDOWS_EXPORT_COMMAND}
                  copiedText={copiedText}
                  onCopy={copyToClipboard}
                />

                <Field
                  data-invalid={
                    manualCredential.length > 0 && !manualCredentialValid
                  }
                >
                  <FieldLabel htmlFor='codex-local-auth-manual-json'>
                    {t('Codex credential JSON')}
                  </FieldLabel>
                  <Textarea
                    ref={manualCredentialRef}
                    id='codex-local-auth-manual-json'
                    value={manualCredential}
                    onChange={(event) =>
                      setManualCredential(event.target.value)
                    }
                    placeholder='{"access_token":"...","refresh_token":"...","account_id":"..."}'
                    autoComplete='off'
                    spellCheck={false}
                    aria-invalid={
                      manualCredential.length > 0 && !manualCredentialValid
                    }
                    className='min-h-32 font-mono text-xs'
                  />
                  <FieldDescription>
                    {manualCredentialHasRefreshToken || !manualCredentialValid
                      ? t(
                          'The JSON must include access_token and account_id. A refresh_token is recommended so MyAPI can maintain the channel.'
                        )
                      : t(
                          'A refresh_token is recommended so MyAPI can maintain the channel.'
                        )}
                  </FieldDescription>
                </Field>
              </FieldGroup>
            )}
          </FieldGroup>

          <DialogFooter>
            {manualMode && (
              <Button
                type='button'
                variant='outline'
                onClick={() => handleManualModeChange(false)}
              >
                {t('Back')}
              </Button>
            )}
            <Button
              type='button'
              variant='outline'
              onClick={() => handleDialogOpenChange(false)}
              disabled={isImporting}
            >
              {t('Cancel')}
            </Button>
            {manualMode && (
              <Button
                type='button'
                onClick={handleManualImport}
                disabled={!manualCredentialValid}
              >
                {t('Fill channel credential')}
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <SecureVerificationDialog
        open={verificationOpen}
        onOpenChange={(open) => {
          if (!open) cancelVerification()
        }}
        methods={verificationMethods}
        state={verificationState}
        onVerify={async (method, code) => {
          await executeVerification(method, code)
        }}
        onCancel={cancelVerification}
        onCodeChange={setVerificationCode}
        onMethodChange={switchVerificationMethod}
      />
    </>
  )
}

function CommandField(props: {
  label: string
  command: string
  copiedText: string | null
  onCopy: (text: string) => Promise<boolean>
}) {
  const { t } = useTranslation()
  const copied = props.copiedText === props.command
  return (
    <Field>
      <FieldLabel>{props.label}</FieldLabel>
      <div className='flex gap-2'>
        <Textarea
          readOnly
          value={props.command}
          className='min-h-24 font-mono text-xs'
        />
        <Button
          type='button'
          variant='outline'
          size='icon'
          className='shrink-0'
          onClick={() => void props.onCopy(props.command)}
          aria-label={t('Copy command')}
          title={t('Copy command')}
        >
          <HugeiconsIcon
            icon={copied ? CopyCheckIcon : Copy01Icon}
            className={copied ? 'text-success' : undefined}
            aria-hidden='true'
          />
        </Button>
      </div>
    </Field>
  )
}
