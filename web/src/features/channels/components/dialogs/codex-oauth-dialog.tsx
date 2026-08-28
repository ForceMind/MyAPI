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
import {
  Check,
  ClipboardPaste,
  Copy,
  ExternalLink,
  Loader2,
} from 'lucide-react'
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
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { tryPrettyJson } from '@/lib/utils'

import { completeCodexOAuth, startCodexOAuth } from '../../api'

type CodexOAuthDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelId?: number
  onKeyGenerated: (key: string) => void
  onCredentialSaved?: () => void
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

  useEffect(() => {
    if (!props.open) setState(INITIAL_STATE)
  }, [props.open])

  const canComplete = useMemo(
    () => Boolean(state.callbackUrl.trim()) && !state.isCompleting,
    [state.callbackUrl, state.isCompleting]
  )

  const handleStart = async () => {
    setState((previous) => ({ ...previous, isStarting: true }))
    try {
      const response = await startCodexOAuth(props.channelId)
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
      toast.error(
        error instanceof Error ? error.message : t('Codex login failed')
      )
    } finally {
      setState((previous) => ({ ...previous, isStarting: false }))
    }
  }

  const pasteCallbackFromClipboard = async () => {
    try {
      const callbackUrl = await navigator.clipboard.readText()
      if (!callbackUrl.trim()) throw new Error(t('Clipboard is empty'))
      setState((previous) => ({
        ...previous,
        callbackUrl: callbackUrl.trim(),
      }))
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Unable to read clipboard')
      )
    }
  }

  const handleComplete = async () => {
    const input = state.callbackUrl.trim()
    if (!input) return
    setState((previous) => ({ ...previous, isCompleting: true }))
    try {
      const response = await completeCodexOAuth(input, props.channelId)
      if (!response.success) {
        throw new Error(response.message || t('Codex authorization failed'))
      }

      if (props.channelId) {
        props.onCredentialSaved?.()
        toast.success(t('Codex credential saved'))
      } else {
        const rawKey = response.data?.key || ''
        if (!rawKey) throw new Error(t('Missing generated credential'))
        props.onKeyGenerated(tryPrettyJson(rawKey))
        toast.success(t('Codex credential filled into the channel form'))
      }
      props.onOpenChange(false)
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Codex authorization failed')
      )
    } finally {
      setState((previous) => ({ ...previous, isCompleting: false }))
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('Sign in to Codex with ChatGPT')}</DialogTitle>
          <DialogDescription>
            {props.channelId
              ? t('The new credential will be saved directly to this channel.')
              : t(
                  'The generated credential will be filled into the channel form.'
                )}
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-4'>
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
              disabled={state.isStarting}
            >
              {state.isStarting ? (
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
              ) : (
                <ExternalLink className='mr-2 h-4 w-4' />
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
                <Check className='mr-2 h-4 w-4 text-green-600' />
              ) : (
                <Copy className='mr-2 h-4 w-4' />
              )}
              {t('Copy login link')}
            </Button>
          </div>

          <div className='space-y-2'>
            <div className='text-sm font-medium'>{t('Callback URL')}</div>
            <div className='flex gap-2'>
              <Input
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
                title={t('Paste from clipboard')}
              >
                <ClipboardPaste className='h-4 w-4' />
              </Button>
            </div>
          </div>
        </div>

        <DialogFooter>
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
              <Loader2 className='mr-2 h-4 w-4 animate-spin' />
            )}
            {state.isCompleting ? t('Authorizing...') : t('Complete login')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
