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
import React, { useCallback, useMemo, useRef, useState } from 'react'

import useDialogState from '@/hooks/use-dialog'
import { useAuthStore } from '@/stores/auth-store'

import type { User, UsersDialogType } from '../types'

type UsersContextType = {
  open: UsersDialogType | null
  setOpen: (str: UsersDialogType | null) => void
  currentRow: User | null
  setCurrentRow: React.Dispatch<React.SetStateAction<User | null>>
  refreshTrigger: number
  triggerRefresh: () => void
  returnFocus: () => HTMLElement | null
}

const UsersContext = React.createContext<UsersContextType | null>(null)

export function UsersProvider({ children }: { children: React.ReactNode }) {
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  return <UsersSessionProvider key={identity} identity={identity}>{children}</UsersSessionProvider>
}

function UsersSessionProvider({ children, identity }: { children: React.ReactNode; identity: string }) {
  const [open, setOpenState] = useDialogState<UsersDialogType>(null)
  const opener = useRef<{ node: HTMLElement | null; id: string | null; edit: boolean } | null>(null)
  const setOpen = useCallback((next: UsersDialogType | null) => {
    if (next !== null) {
      const focused = document.activeElement instanceof HTMLElement ? document.activeElement : null
      const menu = document.querySelector<HTMLElement>('[data-user-actions] button[data-popup-open]')
      const node = focused?.getAttribute('role') === 'menuitem' ? menu : focused
      const container = node?.closest<HTMLElement>('[data-user-actions]')
      opener.current = { node, id: container?.dataset.userActions ?? null, edit: Boolean(node?.hasAttribute('data-user-edit')) }
    }
    setOpenState(next)
  }, [setOpenState])
  const returnFocus = useCallback(() => {
    const { auth } = useAuthStore.getState()
    if (identity !== `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`) return null
    if (opener.current?.node?.isConnected) return opener.current.node
    const id = opener.current?.id
    if (!id) return null
    const container = [...document.querySelectorAll<HTMLElement>('[data-user-actions]')].find((element) => element.dataset.userActions === id)
    return container?.querySelector<HTMLElement>(opener.current?.edit ? '[data-user-edit]' : 'button[aria-haspopup]') ?? document.querySelector<HTMLElement>('[data-users-heading]')
  }, [identity])
  const [currentRow, setCurrentRow] = useState<User | null>(null)
  const [refreshTrigger, setRefreshTrigger] = useState(0)

  const triggerRefresh = useCallback(() => setRefreshTrigger((prev) => prev + 1), [])
  const value = useMemo(() => ({ open, setOpen, currentRow, setCurrentRow, refreshTrigger, triggerRefresh, returnFocus }), [open, setOpen, currentRow, refreshTrigger, triggerRefresh, returnFocus])

  return (
    <UsersContext
      value={value}
    >
      {children}
    </UsersContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useUsers = () => {
  const usersContext = React.useContext(UsersContext)

  if (!usersContext) {
    throw new Error('useUsers has to be used within <UsersContext>')
  }

  return usersContext
}
