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
/* eslint-disable react-refresh/only-export-components */
import { useQueryClient } from '@tanstack/react-query'
import React, {
  createContext,
  useContext,
  useState,
  useCallback,
  useMemo,
  useRef,
} from 'react'

import { useAuthStore } from '@/stores/auth-store'

import { useChannelUpstreamUpdates } from '../hooks/use-channel-upstream-updates'
import { channelsQueryKeys } from '../lib'
import type { Channel } from '../types'

// ============================================================================
// Types
// ============================================================================

type DialogType =
  | 'create-channel'
  | 'update-channel'
  | 'test-channel'
  | 'balance-query'
  | 'fetch-models'
  | 'ollama-models'
  | 'multi-key-manage'
  | 'tag-batch-edit'
  | 'edit-tag'
  | 'copy-channel'
  | null

type UpstreamUpdateState = ReturnType<typeof useChannelUpstreamUpdates>

type ChannelsContextType = {
  open: DialogType
  setOpen: (open: DialogType) => void
  currentRow: Channel | null
  setCurrentRow: (row: Channel | null) => void
  currentTag: string | null
  setCurrentTag: (tag: string | null) => void
  enableTagMode: boolean
  setEnableTagMode: (enabled: boolean) => void
  idSort: boolean
  setIdSort: (enabled: boolean) => void
  batchMode: boolean
  setBatchMode: (enabled: boolean) => void
  sensitiveVisible: boolean
  setSensitiveVisible: (visible: boolean) => void
  upstream: UpstreamUpdateState
  returnFocus: () => HTMLElement | null
}

// ============================================================================
// Context
// ============================================================================

const ChannelsContext = createContext<ChannelsContextType | undefined>(
  undefined
)

// ============================================================================
// Provider
// ============================================================================

export function ChannelsProvider({ children }: { children: React.ReactNode }) {
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  return <ChannelsSessionProvider key={identity} identity={identity}>{children}</ChannelsSessionProvider>
}

function ChannelsSessionProvider({ children, identity }: { children: React.ReactNode; identity: string }) {
  const [open, setOpenState] = useState<DialogType>(null)
  const opener = useRef<{ node: HTMLElement | null; id: string | null; edit: boolean } | null>(null)
  const setOpen = useCallback((next: DialogType) => {
    if (next !== null) {
      const focused = document.activeElement instanceof HTMLElement ? document.activeElement : null
      const popupTrigger = document.querySelector<HTMLElement>('[data-channel-actions] button[data-popup-open]')
      const node = focused?.getAttribute('role') === 'menuitem' ? popupTrigger : focused
      const container = node?.closest<HTMLElement>('[data-channel-actions]')
      opener.current = { node, id: container?.dataset.channelActions ?? null, edit: Boolean(node?.hasAttribute('data-channel-edit')) }
    }
    setOpenState(next)
  }, [])
  const returnFocus = useCallback(() => {
    const { auth } = useAuthStore.getState()
    if (identity !== `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`) return null
    const action = opener.current
    if (action?.node?.isConnected && !action.node.closest('[hidden]')) return action.node
    if (!action?.id) return document.querySelector<HTMLElement>('[data-channel-workspace-return]')
    const container = [...document.querySelectorAll<HTMLElement>('[data-channel-actions]')].find((element) => element.dataset.channelActions === action.id)
    const candidate = container?.querySelector<HTMLElement>(action.edit ? '[data-channel-edit]' : 'button[aria-haspopup]')
    if (candidate && !candidate.closest('[hidden]')) return candidate
    return document.querySelector<HTMLElement>('[data-channel-workspace-return]')
  }, [identity])
  const [currentRow, setCurrentRow] = useState<Channel | null>(null)
  const [currentTag, setCurrentTag] = useState<string | null>(null)
  const [enableTagMode, setEnableTagMode] = useState(() => {
    return localStorage.getItem('enable-tag-mode') === 'true'
  })
  const [idSort, setIdSort] = useState(() => {
    return localStorage.getItem('channels-id-sort') === 'true'
  })
  const [batchMode, setBatchMode] = useState(false)
  const [sensitiveVisible, setSensitiveVisible] = useState(true)

  const queryClient = useQueryClient()
  const refreshChannels = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: channelsQueryKeys.all })
  }, [queryClient])
  const upstream = useChannelUpstreamUpdates(refreshChannels)

  // useState setters are stable, so the context value only needs to change when
  // an actual state value changes. Memoizing avoids handing every consumer
  // (including all channel cards/cells) a brand-new object on each render.
  const value = useMemo<ChannelsContextType>(
    () => ({
      open,
      setOpen,
      currentRow,
      setCurrentRow,
      currentTag,
      setCurrentTag,
      enableTagMode,
      setEnableTagMode,
      idSort,
      setIdSort,
      batchMode,
      setBatchMode,
      sensitiveVisible,
      setSensitiveVisible,
      upstream,
      returnFocus,
    }),
    [
      open,
      currentRow,
      currentTag,
      enableTagMode,
      idSort,
      batchMode,
      sensitiveVisible,
      upstream,
      setOpen,
      returnFocus,
    ]
  )

  return (
    <ChannelsContext.Provider value={value}>
      {children}
    </ChannelsContext.Provider>
  )
}

// ============================================================================
// Hook
// ============================================================================

export function useChannels() {
  const context = useContext(ChannelsContext)
  if (!context) {
    throw new Error('useChannels must be used within ChannelsProvider')
  }
  return context
}
