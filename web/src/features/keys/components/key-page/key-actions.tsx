/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import {
  Copy01Icon,
  PauseIcon,
  PlayIcon,
  SourceCodeIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { createPlaygroundKeyHandoff } from '@/features/playground/lib/playground-key-handoff'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import { useAuthStore } from '@/stores/auth-store'

import { fetchTokenKey, updateApiKeyStatus } from '../../api'
import { API_KEY_STATUS } from '../../constants'
import { useKeyMaintenanceSession } from '../../hooks/use-key-maintenance-session'
import { getKeyCondition } from '../../lib/key-condition'
import type { ApiKey } from '../../types'
import { useApiKeys } from '../api-keys-provider'

/**
 * Pause/resume, copy and Playground act on this Key only after a click. An
 * unknown status write locks the toggle until the current state is reloaded.
 * `menu` (the Key's "more" menu) stays on the same line as the buttons.
 */
export function KeyActions(props: {
  apiKey: ApiKey
  onRefetch: () => unknown
  menu?: ReactNode
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isCurrentSession = useKeyMaintenanceSession()
  const { triggerRefresh } = useApiKeys()
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const showPlayground = useIsSidebarModuleVisible('/playground')
  const [copying, setCopying] = useState(false)
  const active = props.apiKey.status === API_KEY_STATUS.ENABLED
  const toggleable =
    props.apiKey.status === API_KEY_STATUS.ENABLED ||
    props.apiKey.status === API_KEY_STATUS.DISABLED
  const toggle = useMutation({
    mutationFn: async () => {
      if (!isCurrentSession()) throw new Error('Session changed')
      const response = await updateApiKeyStatus(
        props.apiKey.id,
        active ? API_KEY_STATUS.DISABLED : API_KEY_STATUS.ENABLED
      )
      if (!response.success) throw new Error('Status update failed')
    },
    retry: false,
    onSuccess: () => {
      if (isCurrentSession()) triggerRefresh()
    },
  })
  const copy = async () => {
    if (copying || !isCurrentSession()) return
    setCopying(true)
    try {
      const response = await fetchTokenKey(props.apiKey.id)
      if (!isCurrentSession()) return
      if (!response.success || !response.data?.key) {
        throw new Error('Key unavailable')
      }
      const copied = await copyToClipboard(`sk-${response.data.key}`)
      if (!isCurrentSession()) return
      if (copied) toast.success(t('Copied successfully'))
      else toast.error(t('Failed to copy'))
    } catch {
      if (isCurrentSession()) {
        toast.error(t('Could not copy this Key. Please try again.'))
      }
    } finally {
      setCopying(false)
    }
  }
  const playgroundReady =
    showPlayground &&
    user !== null &&
    sessionId !== null &&
    getKeyCondition(props.apiKey) === 'available'

  return (
    <div className='flex min-w-0 flex-col gap-2'>
      <div className='flex min-w-0 flex-wrap items-center gap-2'>
        {toggleable && (
          <Button
            variant='outline'
            disabled={toggle.isPending || toggle.isError}
            onClick={() => toggle.mutate()}
          >
            <HugeiconsIcon
              icon={active ? PauseIcon : PlayIcon}
              data-icon='inline-start'
              aria-hidden='true'
            />
            {active ? t('Pause Key') : t('Resume Key')}
          </Button>
        )}
        <Button variant='ghost' disabled={copying} onClick={() => void copy()}>
          <HugeiconsIcon
            icon={Copy01Icon}
            data-icon='inline-start'
            aria-hidden='true'
          />
          {t('Copy')}
        </Button>
        {showPlayground && (
          <Button
            variant='ghost'
            disabled={!playgroundReady}
            onClick={() => {
              if (!playgroundReady || !user || !isCurrentSession()) return
              createPlaygroundKeyHandoff({
                userId: user.id,
                sessionId,
                keyId: props.apiKey.id,
              })
              void navigate({ to: '/playground' })
            }}
          >
            <HugeiconsIcon
              icon={SourceCodeIcon}
              data-icon='inline-start'
              aria-hidden='true'
            />
            {t('Open Playground')}
          </Button>
        )}
        {props.menu}
      </div>
      {toggle.isError && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Failed to update API key status')}</AlertTitle>
          <AlertDescription>
            <Button
              variant='outline'
              size='sm'
              onClick={async () => {
                await props.onRefetch()
                if (isCurrentSession()) toggle.reset()
              }}
            >
              {t('Reload current state')}
            </Button>
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}
