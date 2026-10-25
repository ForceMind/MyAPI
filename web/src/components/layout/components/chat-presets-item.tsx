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
import { LinkSquare02Icon, Loading03Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link, useLocation } from '@tanstack/react-router'
import { useCallback, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
} from '@/components/ui/dropdown-menu'
import { fetchActiveChatKey } from '@/features/chat/hooks/use-active-chat-key'
import { useChatPresets } from '@/features/chat/hooks/use-chat-presets'
import {
  chatLinkRequiresApiKey,
  resolveChatUrl,
  type ChatPreset,
} from '@/features/chat/lib/chat-links'

import { normalizeHref } from '../lib/url-utils'
import type { NavChatPresets } from '../types'

/**
 * Chat presets offered by the administrator. Web presets open the in-app
 * chat page; external presets resolve their link (with the active Key when
 * the template needs one) and open in a new tab.
 */
function useChatPresetEntries() {
  const { t } = useTranslation()
  const { chatPresets, serverAddress } = useChatPresets()
  const [loadingPresetId, setLoadingPresetId] = useState<string | null>(null)
  const loadingPresetIdRef = useRef<string | null>(null)

  const presets = useMemo(
    () => chatPresets.filter((preset) => preset.type !== 'fluent'),
    [chatPresets]
  )

  const openExternal = useCallback(
    async (preset: ChatPreset) => {
      if (preset.type === 'web') return

      const needsKey = chatLinkRequiresApiKey(preset.url)
      let activeKey: string | undefined

      if (needsKey && loadingPresetIdRef.current) {
        toast.info(t('Preparing your chat link, please try again in a moment.'))
        return
      }

      if (needsKey) {
        loadingPresetIdRef.current = preset.id
        setLoadingPresetId(preset.id)
        try {
          activeKey = await fetchActiveChatKey()
        } catch (error) {
          const message =
            error instanceof Error
              ? error.message
              : t(
                  'Unable to prepare chat link. Please ensure you have an enabled API key.'
                )
          toast.error(message)
          return
        } finally {
          loadingPresetIdRef.current = null
          setLoadingPresetId(null)
        }
      }

      const url = resolveChatUrl({
        template: preset.url,
        apiKey: needsKey ? activeKey : undefined,
        serverAddress,
      })

      if (!url) {
        toast.error(t('Invalid chat link. Please contact the administrator.'))
        return
      }

      if (typeof window === 'undefined') return

      window.open(url, '_blank', 'noopener')
    },
    [serverAddress, t]
  )

  return { presets, loadingPresetId, openExternal }
}

function PresetStatusIcon(props: { loading: boolean }) {
  return (
    <HugeiconsIcon
      icon={props.loading ? Loading03Icon : LinkSquare02Icon}
      className={props.loading ? 'ms-auto animate-spin' : 'ms-auto'}
      aria-hidden='true'
    />
  )
}

/** Chat presets as a labelled group of the top bar's More menu. */
export function ChatPresetMenuItems(props: { item: NavChatPresets }) {
  const entries = useChatPresetEntries()
  if (entries.presets.length === 0) return null

  return (
    <DropdownMenuGroup>
      <DropdownMenuLabel>{props.item.title}</DropdownMenuLabel>
      {entries.presets.map((preset) => {
        if (preset.type === 'web') {
          return (
            <DropdownMenuItem
              key={preset.id}
              render={
                <Link to='/chat/$chatId' params={{ chatId: preset.id }} />
              }
            >
              {preset.name}
            </DropdownMenuItem>
          )
        }
        const loading = entries.loadingPresetId === preset.id
        return (
          <DropdownMenuItem
            key={preset.id}
            disabled={loading}
            onClick={() => {
              if (!loading) void entries.openExternal(preset)
            }}
          >
            {preset.name}
            <PresetStatusIcon loading={loading} />
          </DropdownMenuItem>
        )
      })}
    </DropdownMenuGroup>
  )
}

/** Chat presets as entries of the mobile More sheet. */
export function ChatPresetLinks(props: {
  item: NavChatPresets
  onNavigate?: () => void
}) {
  const entries = useChatPresetEntries()
  const href = useLocation({ select: (location) => location.href })
  if (entries.presets.length === 0) return null
  const current = normalizeHref(href)

  return entries.presets.map((preset) => {
    if (preset.type === 'web') {
      return (
        <Link
          key={preset.id}
          to='/chat/$chatId'
          params={{ chatId: preset.id }}
          className='pc-nav-link'
          preload={false}
          aria-current={current === `/chat/${preset.id}` ? 'page' : undefined}
          onClick={props.onNavigate}
        >
          <span>{preset.name}</span>
        </Link>
      )
    }
    const loading = entries.loadingPresetId === preset.id
    return (
      <button
        key={preset.id}
        type='button'
        className='pc-nav-link'
        aria-disabled={loading ? 'true' : undefined}
        onClick={() => {
          if (loading) return
          props.onNavigate?.()
          void entries.openExternal(preset)
        }}
      >
        <span>{preset.name}</span>
        <PresetStatusIcon loading={loading} />
      </button>
    )
  })
}
