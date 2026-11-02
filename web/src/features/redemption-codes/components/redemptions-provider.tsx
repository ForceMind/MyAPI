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
import React, { useCallback, useRef, useState } from 'react'

import { useAuthStore } from '@/stores/auth-store'

import useDialogState from '@/hooks/use-dialog'

import type { Redemption, RedemptionsDialogType } from '../types'

type RedemptionsContextType = {
  returnFocus: () => HTMLElement | null
  open: RedemptionsDialogType | null
  setOpen: (str: RedemptionsDialogType | null) => void
  currentRow: Redemption | null
  setCurrentRow: React.Dispatch<React.SetStateAction<Redemption | null>>
  refreshTrigger: number
  triggerRefresh: () => void
}

const RedemptionsContext = React.createContext<RedemptionsContextType | null>(
  null
)

export function RedemptionsProvider({ children }: { children: React.ReactNode }) {
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  return <RedemptionsSessionProvider key={identity} identity={identity}>{children}</RedemptionsSessionProvider>
}

function RedemptionsSessionProvider({ children, identity }: { children: React.ReactNode; identity: string }) {
  const [open, setOpenState] = useDialogState<RedemptionsDialogType>(null)
  const opener = useRef<{ node: HTMLElement | null; id: string | null; action: string | null } | null>(null)
  const setOpen = useCallback((next: RedemptionsDialogType | null) => {
    if (next !== null) {
      const focused = document.activeElement instanceof HTMLElement ? document.activeElement : null
      const node = focused?.getAttribute('role') === 'menuitem' ? document.querySelector<HTMLElement>('[data-redemption-actions] button[data-popup-open]') : focused
      const container = node?.closest<HTMLElement>('[data-redemption-actions]')
      opener.current = { node, id: container?.dataset.redemptionActions ?? null, action: node?.dataset.redemptionAction ?? null }
    }
    setOpenState(next)
  }, [setOpenState])
  const returnFocus = useCallback(() => {
    const { auth } = useAuthStore.getState()
    if (identity !== `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`) return null
    const target = opener.current
    if (target?.node?.isConnected) return target.node
    if (!target?.id) return document.querySelector<HTMLElement>('[data-redemption-create]:not(:disabled)') ?? document.querySelector<HTMLElement>('[data-redemption-list-heading]')
    const container = [...document.querySelectorAll<HTMLElement>('[data-redemption-actions]')].find((node) => node.dataset.redemptionActions === target.id)
    return [...(container?.querySelectorAll<HTMLElement>('[data-redemption-action]') ?? [])].find((node) => node.dataset.redemptionAction === target.action) ?? container?.querySelector<HTMLElement>('button[aria-haspopup="menu"]') ?? document.querySelector<HTMLElement>('[data-redemption-list-heading]')
  }, [identity])
  const [currentRow, setCurrentRow] = useState<Redemption | null>(null)
  const [refreshTrigger, setRefreshTrigger] = useState(0)

  const triggerRefresh = () => setRefreshTrigger((prev) => prev + 1)

  return (
    <RedemptionsContext
      value={{
        returnFocus,
        open,
        setOpen,
        currentRow,
        setCurrentRow,
        refreshTrigger,
        triggerRefresh,
      }}
    >
      {children}
    </RedemptionsContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useRedemptions = () => {
  const redemptionsContext = React.useContext(RedemptionsContext)

  if (!redemptionsContext) {
    throw new Error(
      'useRedemptions has to be used within <RedemptionsProvider>'
    )
  }

  return redemptionsContext
}
