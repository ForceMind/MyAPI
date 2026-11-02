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
import { Copy01Icon, LinkSquare02Icon, Loading03Icon, Tick02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'

import { completeCodexOAuth, startCodexOAuth } from '../../api'
import type { AddChannelRequest } from '../../types'
import { useChannelTask } from '../../hooks/use-channel-task'

type CodexOAuthDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelId?: number
  getCreatePayload?: () => Promise<AddChannelRequest | null>
  onCredentialSaved?: () => void
  returnFocus?: () => HTMLElement | null
}

const INITIAL_STATE = {
  authorizeUrl: '',
  callbackUrl: '',
  isStarting: false,
  isCompleting: false,
}

export function CodexOAuthDialog(props: CodexOAuthDialogProps) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const [state, setState] = useState(INITIAL_STATE)
  const [actionError, setActionError] = useState<string | null>(null)
  const task = useChannelTask(props.open, props.channelId)

  useEffect(() => {
    setState(INITIAL_STATE)
    setActionError(null)
  }, [props.open, props.channelId, task.identity])

  const canComplete = useMemo(
    () => Boolean(state.callbackUrl.trim()) && !state.isCompleting && !state.isStarting,
    [state.callbackUrl, state.isCompleting, state.isStarting]
  )

  const handleStart = async () => {
    const operation = task.begin()
    if (!operation) return
    setActionError(null)
    setState((previous) => ({ ...previous, isStarting: true }))
    try {
      if (!props.channelId && !(await props.getCreatePayload?.())) {
        throw new Error(
          t('Complete the required channel fields before import.')
        )
      }
      if (!operation.isCurrent()) return
      const response = await startCodexOAuth(props.channelId)
      if (!operation.isCurrent()) return
      if (!response.success) {
        throw new Error(response.message || t('Failed to start Codex login'))
      }
      const authorizeUrl = response.data?.authorize_url?.trim() || ''
      if (!authorizeUrl) throw new Error(t('Missing authorization URL'))

      setState((previous) => ({ ...previous, authorizeUrl }))
      const opened = window.open(authorizeUrl, '_blank', 'noopener,noreferrer')
      if (opened) {
        toast.success(t('Opened ChatGPT authorization page'))
      } else {
        toast.info(t('Copy and open the authorization link'))
      }
    } catch (error) {
      if (!operation.isCurrent()) return
      setActionError(error instanceof Error ? error.message : t('Failed to start Codex login'))
      toast.error(
        error instanceof Error ? error.message : t('Codex login failed')
      )
    } finally {
      if (operation.isCurrent()) setState((previous) => ({ ...previous, isStarting: false }))
      operation.finish()
    }
  }

  const pasteCallbackFromClipboard = async () => {
    const operation = task.capture()
    try {
      const callbackUrl = await navigator.clipboard.readText()
      if (!operation.isCurrent()) return
      if (!callbackUrl.trim()) throw new Error(t('Clipboard is empty'))
      setState((previous) => ({
        ...previous,
        callbackUrl: callbackUrl.trim(),
      }))
    } catch (error) {
      if (!operation.isCurrent()) return
      toast.error(
        error instanceof Error ? error.message : t('Unable to read clipboard')
      )
    }
  }

  const handleComplete = async () => {
    const input = state.callbackUrl.trim()
    if (!input) return
    const operation = task.begin()
    if (!operation) return
    setActionError(null)
    setState((previous) => ({ ...previous, isCompleting: true }))
    try {
      const create = props.channelId
        ? undefined
        : await props.getCreatePayload?.()
      if (!props.channelId && !create) {
        throw new Error(
          t('Complete the required channel fields before import.')
        )
      }
      if (!operation.isCurrent()) return
      const response = await completeCodexOAuth(
        input,
        props.channelId,
        create ?? undefined
      )
      if (!operation.isCurrent()) return
      if (!response.success) {
        throw new Error(response.message || t('Codex authorization failed'))
      }

      props.onCredentialSaved?.()
      toast.success(t('Codex credential saved'))
      props.onOpenChange(false)
    } catch (error) {
      if (!operation.isCurrent()) return
      setActionError(error instanceof Error ? error.message : t('Codex authorization failed'))
      toast.error(
        error instanceof Error ? error.message : t('Codex authorization failed')
      )
    } finally {
      if (operation.isCurrent()) setState((previous) => ({ ...previous, isCompleting: false }))
      operation.finish()
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={(next) => { if (!task.isPending()) props.onOpenChange(next) }}>
      <DialogContent finalFocus={props.returnFocus} showCloseButton={!state.isStarting && !state.isCompleting} className='flex h-dvh max-h-dvh w-screen max-w-none flex-col overflow-hidden sm:h-auto sm:max-h-[calc(100dvh-2rem)] sm:max-w-2xl [&_button]:min-h-11 [&_button]:min-w-11 [&_input]:min-h-11'>
        <DialogHeader className='shrink-0 text-start'>
          <DialogTitle>{t('Sign in to Codex with ChatGPT')}</DialogTitle>
          <DialogDescription>
            {props.channelId
              ? t('The new credential will be saved directly to this channel.')
              : t('The channel will be created after login.')}
          </DialogDescription>
        </DialogHeader>

        <div className='min-h-0 space-y-4 overflow-y-auto overscroll-contain'>
          {actionError ? <div role='alert' className='text-destructive text-sm break-words'>{actionError}</div> : null}
          <Alert>
            <AlertDescription>
              {t(
                'Open the ChatGPT authorization page and finish signing in. The browser will redirect to localhost; if that page cannot load, this is expected. Copy the full URL from the address bar, return here, and paste it below.'
              )}
            </AlertDescription>
          </Alert>

          <div className='flex flex-wrap gap-2'>
            <Button
              type='button'
              onClick={handleStart}
              disabled={state.isStarting || state.isCompleting}
            >
              {state.isStarting ? (
                <HugeiconsIcon icon={Loading03Icon} aria-hidden='true' className='mr-2 h-4 w-4 animate-spin' />
              ) : (
                <HugeiconsIcon icon={LinkSquare02Icon} aria-hidden='true' className='mr-2 h-4 w-4' />
              )}
              {t('Open ChatGPT login')}
            </Button>
            <Button
              type='button'
              variant='outline'
              disabled={!state.authorizeUrl || state.isStarting}
              onClick={() => copyToClipboard(state.authorizeUrl)}
            >
              {copiedText === state.authorizeUrl ? (
                <HugeiconsIcon icon={Tick02Icon} aria-hidden='true' className='mr-2 h-4 w-4 text-green-600' />
              ) : (
                <HugeiconsIcon icon={Copy01Icon} aria-hidden='true' className='mr-2 h-4 w-4' />
              )}
              {t('Copy login link')}
            </Button>
          </div>

          <div className='space-y-2'>
            <Label htmlFor='codex-oauth-callback'>{t('Callback URL')}</Label>
            <div className='flex min-w-0 gap-2'>
              <Input
                id='codex-oauth-callback'
                disabled={state.isStarting || state.isCompleting}
                value={state.callbackUrl}
                onChange={(event) =>
                  setState((previous) => ({
                    ...previous,
                    callbackUrl: event.target.value,
                  }))
                }
                placeholder='http://localhost:1455/auth/callback?code=...&state=...'
                autoComplete='off'
                spellCheck={false}
              />
              <Button
                type='button'
                variant='outline'
                onClick={pasteCallbackFromClipboard}
                disabled={state.isStarting || state.isCompleting}
                aria-label={t('Paste from clipboard')}
                title={t('Paste from clipboard')}
              >
                <HugeiconsIcon icon={Copy01Icon} aria-hidden='true' className='h-4 w-4' />
              </Button>
            </div>
          </div>
        </div>

        <DialogFooter className='shrink-0 border-t pt-4'>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
            disabled={state.isStarting || state.isCompleting}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            onClick={handleComplete}
            disabled={!canComplete}
          >
            {state.isCompleting && (
              <HugeiconsIcon icon={Loading03Icon} aria-hidden='true' className='mr-2 h-4 w-4 animate-spin' />
            )}
            {state.isCompleting ? t('Authorizing...') : t('Complete login')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
