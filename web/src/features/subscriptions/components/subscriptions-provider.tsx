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

import {
  getOptionValue,
  useSystemOptions,
} from '@/features/system-settings/hooks/use-system-options'
import useDialogState from '@/hooks/use-dialog'

import type { PlanRecord, SubscriptionsDialogType } from '../types'

const CURRENT_COMPLIANCE_TERMS_VERSION = 'v1'

type SubscriptionsContextType = {
  returnFocus: () => HTMLElement | null
  open: SubscriptionsDialogType | null
  setOpen: (str: SubscriptionsDialogType | null) => void
  currentRow: PlanRecord | null
  setCurrentRow: React.Dispatch<React.SetStateAction<PlanRecord | null>>
  refreshTrigger: number
  triggerRefresh: () => void
  complianceConfirmed: boolean
}

const SubscriptionsContext =
  React.createContext<SubscriptionsContextType | null>(null)

export function SubscriptionsProvider({ children }: { children: React.ReactNode }) {
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  return <SubscriptionsSessionProvider key={identity} identity={identity}>{children}</SubscriptionsSessionProvider>
}

function SubscriptionsSessionProvider({ children, identity }: { children: React.ReactNode; identity: string }) {
  const [open, setOpenState] = useDialogState<SubscriptionsDialogType>(null)
  const opener = useRef<{ node: HTMLElement | null; id: string | null; action: string | null } | null>(null)
  const setOpen = useCallback((next: SubscriptionsDialogType | null) => {
    if (next !== null) {
      const focused = document.activeElement instanceof HTMLElement ? document.activeElement : null
      const node = focused?.getAttribute('role') === 'menuitem' ? document.querySelector<HTMLElement>('[data-plan-actions] button[data-popup-open]') : focused
      const container = node?.closest<HTMLElement>('[data-plan-actions]')
      opener.current = { node, id: container?.dataset.planActions ?? null, action: node?.dataset.planAction ?? null }
    }
    setOpenState(next)
  }, [setOpenState])
  const returnFocus = useCallback(() => {
    const { auth } = useAuthStore.getState()
    if (identity !== `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`) return null
    const target = opener.current
    if (target?.node?.isConnected) return target.node
    if (!target?.id) return document.querySelector<HTMLElement>('[data-plan-create]:not(:disabled)') ?? document.querySelector<HTMLElement>('[data-plan-list-heading]')
    const container = [...document.querySelectorAll<HTMLElement>('[data-plan-actions]')].find((node) => node.dataset.planActions === target.id)
    return [...(container?.querySelectorAll<HTMLElement>('[data-plan-action]') ?? [])].find((node) => node.dataset.planAction === target.action) ?? container?.querySelector<HTMLElement>('button[aria-haspopup="menu"]') ?? document.querySelector<HTMLElement>('[data-plan-list-heading]')
  }, [identity])
  const [currentRow, setCurrentRow] = useState<PlanRecord | null>(null)
  const [refreshTrigger, setRefreshTrigger] = useState(0)
  const { data } = useSystemOptions()
  const complianceOptions = getOptionValue(data?.data, {
    'payment_setting.compliance_confirmed': false,
    'payment_setting.compliance_terms_version': '',
  })
  const complianceConfirmed =
    complianceOptions['payment_setting.compliance_confirmed'] &&
    complianceOptions['payment_setting.compliance_terms_version'] ===
      CURRENT_COMPLIANCE_TERMS_VERSION

  const triggerRefresh = () => setRefreshTrigger((prev) => prev + 1)

  return (
    <SubscriptionsContext
      value={{
        returnFocus,
        open,
        setOpen,
        currentRow,
        setCurrentRow,
        refreshTrigger,
        triggerRefresh,
        complianceConfirmed,
      }}
    >
      {children}
    </SubscriptionsContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useSubscriptions = () => {
  const ctx = React.useContext(SubscriptionsContext)
  if (!ctx) {
    throw new Error(
      'useSubscriptions has to be used within <SubscriptionsProvider>'
    )
  }
  return ctx
}
