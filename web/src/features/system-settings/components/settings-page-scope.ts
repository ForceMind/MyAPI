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
import { createContext, useContext, useSyncExternalStore } from 'react'

import { useAuthStore } from '@/stores/auth-store'

type SettingsPageContextValue = {
  actionsContainer: HTMLDivElement | null
  titleStatusContainer: HTMLSpanElement | null
  suppressSectionHeader: boolean
  reloadScope: SettingsReloadScope
}

export type SettingsReloadScope = {
  subscribe: (listener: () => void) => () => void
  getSnapshot: () => number
  hasEditors: () => boolean
  acceptReload: () => void
  isActive: () => boolean
  setActive: (active: boolean) => void
}

export function createSettingsReloadScope(): SettingsReloadScope {
  let generation = 0
  let active = true
  const listeners = new Set<() => void>()
  return {
    subscribe: (listener) => { listeners.add(listener); return () => { listeners.delete(listener) } },
    getSnapshot: () => generation,
    hasEditors: () => listeners.size > 0,
    acceptReload: () => { if (!active) return; generation += 1; for (const listener of listeners) listener() },
    isActive: () => active,
    setActive: (value) => { active = value },
  }
}

export const SettingsPageContext = createContext<SettingsPageContextValue>({
  actionsContainer: null,
  titleStatusContainer: null,
  suppressSectionHeader: false,
  reloadScope: createSettingsReloadScope(),
})

export function useSettingsReloadScope() {
  return useContext(SettingsPageContext).reloadScope
}

export function isCurrentSettingsFrame(scope: SettingsReloadScope, identity: string): boolean {
  const { auth } = useAuthStore.getState()
  return scope.isActive() && identity === `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`
}

export function useSettingsReloadGeneration() {
  const scope = useSettingsReloadScope()
  return useSyncExternalStore(scope.subscribe, scope.getSnapshot, scope.getSnapshot)
}
