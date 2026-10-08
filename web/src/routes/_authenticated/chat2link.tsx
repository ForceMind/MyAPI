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
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { Loader2 } from 'lucide-react'
import { useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Main } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { useActiveChatKey } from '@/features/chat/hooks/use-active-chat-key'
import { useChatPresets } from '@/features/chat/hooks/use-chat-presets'
import {
  chatLinkRequiresApiKey,
  resolveChatUrl,
} from '@/features/chat/lib/chat-links'
import { useStatus } from '@/hooks/use-status'

export const Route = createFileRoute('/_authenticated/chat2link')({
  component: Chat2LinkPage,
})

function Chat2LinkPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { chatPresets, serverAddress } = useChatPresets()
  const { loading, error, confirmed } = useStatus()

  const firstWebPreset = useMemo(
    () => chatPresets.find((p) => p.type === 'web'),
    [chatPresets]
  )
  const requiresActiveKey = Boolean(
    firstWebPreset && chatLinkRequiresApiKey(firstWebPreset.url)
  )

  const { data: activeKey, error: keyError } = useActiveChatKey(
    Boolean(requiresActiveKey && confirmed)
  )

  useEffect(() => {
    if (!confirmed) return
    if (!firstWebPreset) {
      return
    }

    if (requiresActiveKey && activeKey === undefined && !keyError) return

    if (requiresActiveKey && (keyError || !activeKey)) {
      const message =
        keyError instanceof Error
          ? keyError.message
          : t('No enabled tokens available')
      toast.error(message)
      navigate({ to: '/keys' })
      return
    }

    const url = resolveChatUrl({
      template: firstWebPreset.url,
      apiKey: requiresActiveKey ? activeKey : undefined,
      serverAddress,
    })

    if (url) {
      window.location.href = url
    }
  }, [
    firstWebPreset,
    activeKey,
    keyError,
    serverAddress,
    chatPresets.length,
    confirmed,
    requiresActiveKey,
    navigate,
    t,
  ])

  if (error || (!loading && confirmed && !firstWebPreset)) {
    return (
      <Main className='items-center justify-center p-6'>
        <section
          role={error ? 'alert' : 'status'}
          className='bg-card flex w-full max-w-xl flex-col items-center gap-4 rounded-xl border p-6 text-center'
        >
          <h1 className='text-xl font-semibold'>
            {error ? t('Unable to open chat') : t('Chat preset not found')}
          </h1>
          <p className='text-muted-foreground'>
            {error
              ? t('Please try again later.')
              : t('No available Web chat links')}
          </p>
          <Button render={<Link to='/dashboard' />}>
            {t('Return to dashboard')}
          </Button>
        </section>
      </Main>
    )
  }

  return (
    <Main className='items-center justify-center gap-3'>
      <Loader2 className='text-muted-foreground h-8 w-8 animate-spin' />
      <p className='text-muted-foreground text-sm'>
        {t('Redirecting to chat page...')}
      </p>
    </Main>
  )
}
