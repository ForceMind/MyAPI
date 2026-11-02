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
import { RotateCcw, Save } from 'lucide-react'
import {
  createContext,
  useContext,
  useLayoutEffect,
  useState,
  useSyncExternalStore,
  type ComponentProps,
  type ReactNode,
  type RefObject,
} from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { isReportedOptionFailure } from '../hooks/use-update-option'
import { useAuthStore } from '@/stores/auth-store'

type SettingsPageContextValue = {
  actionsContainer: HTMLDivElement | null
  titleStatusContainer: HTMLSpanElement | null
  suppressSectionHeader: boolean
  reloadScope: SettingsReloadScope
}

type SettingsReloadScope = {
  subscribe: (listener: () => void) => () => void
  getSnapshot: () => number
  hasEditors: () => boolean
  acceptReload: () => void
  isActive: () => boolean
  setActive: (active: boolean) => void
}

function createSettingsReloadScope(): SettingsReloadScope {
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

const SettingsPageContext = createContext<SettingsPageContextValue>({
  actionsContainer: null,
  titleStatusContainer: null,
  suppressSectionHeader: false,
  reloadScope: createSettingsReloadScope(),
})

type SettingsPageProviderProps = {
  actionsContainer: HTMLDivElement | null
  titleStatusContainer?: HTMLSpanElement | null
  children: ReactNode
  suppressSectionHeader?: boolean
}

export function SettingsPageProvider(props: SettingsPageProviderProps) {
  const [reloadScope] = useState(createSettingsReloadScope)
  useLayoutEffect(() => { reloadScope.setActive(true); return () => { reloadScope.setActive(false) } }, [reloadScope])
  return (
    <SettingsPageContext.Provider
      value={{
        actionsContainer: props.actionsContainer,
        titleStatusContainer: props.titleStatusContainer ?? null,
        suppressSectionHeader: props.suppressSectionHeader ?? true,
        reloadScope,
      }}
    >
      {props.children}
    </SettingsPageContext.Provider>
  )
}

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

export function useSuppressSettingsSectionHeader() {
  return useContext(SettingsPageContext).suppressSectionHeader
}

type SettingsPageTitleStatusPortalProps = {
  children: ReactNode
}

export function SettingsPageTitleStatusPortal(
  props: SettingsPageTitleStatusPortalProps
) {
  const { titleStatusContainer } = useContext(SettingsPageContext)

  if (!titleStatusContainer) return null

  return createPortal(props.children, titleStatusContainer)
}

type SettingsPageActionsPortalProps = {
  children: ReactNode
}

export function SettingsPageActionsPortal(
  props: SettingsPageActionsPortalProps
) {
  const { actionsContainer } = useContext(SettingsPageContext)

  if (!actionsContainer) return null

  return createPortal(
    <div className='flex flex-wrap items-center justify-end gap-2'>
      {props.children}
    </div>,
    actionsContainer
  )
}

type SettingsPageFormActionsProps = {
  onSave: () => void
  onReset?: () => void
  isSaving?: boolean
  isSaveDisabled?: boolean
  isResetDisabled?: boolean
  saveLabel?: string
  savingLabel?: string
  resetLabel?: string
  resetVariant?: ComponentProps<typeof Button>['variant']
  saveButtonRef?: RefObject<HTMLButtonElement | null>
}

export function SettingsPageFormActions(props: SettingsPageFormActionsProps) {
  const { t } = useTranslation()
  const saveLabel = props.isSaving
    ? (props.savingLabel ?? 'Saving...')
    : (props.saveLabel ?? 'Save Changes')

  return (
    <SettingsPageActionsPortal>
      {props.onReset && (
        <Button
          type='button'
          size='sm'
          variant={props.resetVariant ?? 'outline'}
          onClick={props.onReset}
          disabled={props.isResetDisabled || props.isSaving}
        >
          <RotateCcw data-icon='inline-start' />
          <span>{t(props.resetLabel ?? 'Reset')}</span>
        </Button>
      )}
      <Button
        ref={props.saveButtonRef}
        type='button'
        size='sm'
        onClick={() => {
          try {
            void Promise.resolve(props.onSave()).catch((error: unknown) => {
              if (!isReportedOptionFailure(error)) throw error
            })
          } catch (error) {
            if (!isReportedOptionFailure(error)) throw error
          }
        }}
        disabled={props.isSaving || props.isSaveDisabled}
      >
        <Save data-icon='inline-start' />
        <span>{t(saveLabel)}</span>
      </Button>
    </SettingsPageActionsPortal>
  )
}
