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
import { useAuthStore } from '@/stores/auth-store'
import React, { createContext, useCallback, useContext, useRef, useState } from 'react'
import type { LogPageSize } from '@/lib/log-pagination'

import { findModelAction, type ModelActionTarget } from '../model-action-focus'

import type {
  Model,
  ModelTabCategory,
  Vendor,
  SyncDiffData,
  SyncLocale,
  SyncSource,
} from '../types'

// ============================================================================
// Types
// ============================================================================

type DialogType =
  | 'create-model'
  | 'update-model'
  | 'create-vendor'
  | 'update-vendor'
  | 'missing-models'
  | 'sync-wizard'
  | 'upstream-conflict'
  | 'prefill-groups'
  | 'description'
  | null

type MissingModelsView = { search: string; page: number; pageSize: LogPageSize }
type MissingModelReturnTarget = Extract<ModelActionTarget, { kind: 'missing-configure' }>

type ModelsContextType = {
  rememberOpener: (target: ModelActionTarget) => void
  returnFocus: () => HTMLElement | null
  open: DialogType
  setOpen: (open: DialogType) => void
  currentRow: Model | null
  setCurrentRow: (model: Model | null) => void
  missingView: MissingModelsView
  setMissingView: React.Dispatch<React.SetStateAction<MissingModelsView>>
  missingReturnTarget: MissingModelReturnTarget | null
  setMissingReturnTarget: (target: MissingModelReturnTarget | null) => void
  currentVendor: Vendor | null
  setCurrentVendor: (vendor: Vendor | null) => void
  selectedVendor: string | null
  setSelectedVendor: (vendor: string | null) => void
  descriptionData: { modelName: string; description: string } | null
  setDescriptionData: (
    data: { modelName: string; description: string } | null
  ) => void
  upstreamConflicts: SyncDiffData['conflicts']
  setUpstreamConflicts: (conflicts: SyncDiffData['conflicts']) => void
  syncWizardOptions: { locale: SyncLocale; source: SyncSource }
  setSyncWizardOptions: React.Dispatch<
    React.SetStateAction<{ locale: SyncLocale; source: SyncSource }>
  >
  tabCategory: ModelTabCategory
  setTabCategory: (category: ModelTabCategory) => void
}

// ============================================================================
// Context
// ============================================================================

const ModelsContext = createContext<ModelsContextType | undefined>(undefined)

// ============================================================================
// Provider
// ============================================================================

export function ModelsProvider({ children }: { children: React.ReactNode }) {
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  return <ModelsSessionProvider key={identity}>{children}</ModelsSessionProvider>
}

function ModelsSessionProvider({ children }: { children: React.ReactNode }) {
  const opener = useRef<{ element: HTMLElement | null; target: ModelActionTarget } | null>(null)
  const rememberOpener = useCallback((target: ModelActionTarget) => {
    const active = document.activeElement instanceof HTMLElement && document.activeElement !== document.body ? document.activeElement : null
    opener.current = { element: target.kind === 'tools' ? findModelAction(target) : active, target }
  }, [])
  const returnFocus = useCallback(() => {
    const remembered = opener.current
    if (remembered?.element?.isConnected) return remembered.element
    return (remembered ? findModelAction(remembered.target) : null) ?? document.querySelector<HTMLElement>('[data-model-list-heading]')
  }, [])
  const [open, setOpen] = useState<DialogType>(null)
  const [currentRow, setCurrentRow] = useState<Model | null>(null)
  const [missingView, setMissingView] = useState<MissingModelsView>({ search: '', page: 1, pageSize: 20 })
  const [missingReturnTarget, setMissingReturnTarget] = useState<MissingModelReturnTarget | null>(null)
  const [currentVendor, setCurrentVendor] = useState<Vendor | null>(null)
  const [selectedVendor, setSelectedVendor] = useState<string | null>(null)
  const [descriptionData, setDescriptionData] = useState<{
    modelName: string
    description: string
  } | null>(null)
  const [upstreamConflicts, setUpstreamConflicts] = useState<
    SyncDiffData['conflicts']
  >([])
  const [syncWizardOptions, setSyncWizardOptions] = useState<{
    locale: SyncLocale
    source: SyncSource
  }>({
    locale: 'zh',
    source: 'official',
  })
  const [tabCategory, setTabCategory] = useState<ModelTabCategory>('metadata')

  return (
    <ModelsContext.Provider
      value={{
        rememberOpener,
        returnFocus,
        open,
        setOpen,
        currentRow,
        setCurrentRow,
        missingView,
        setMissingView,
        missingReturnTarget,
        setMissingReturnTarget,
        currentVendor,
        setCurrentVendor,
        selectedVendor,
        setSelectedVendor,
        descriptionData,
        setDescriptionData,
        upstreamConflicts,
        setUpstreamConflicts,
        syncWizardOptions,
        setSyncWizardOptions,
        tabCategory,
        setTabCategory,
      }}
    >
      {children}
    </ModelsContext.Provider>
  )
}

// ============================================================================
// Hook
// ============================================================================

export function useModels() {
  const context = useContext(ModelsContext)
  if (!context) {
    throw new Error('useModels must be used within ModelsProvider')
  }
  return context
}
