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
import { useEffect, useRef } from 'react'
import type { DefaultValues, FieldValues, UseFormReturn } from 'react-hook-form'

import { useAuthStore } from '@/stores/auth-store'
import { useSettingsReloadGeneration } from '../components/settings-page-context'

function hasDirtySettingsValue(value: unknown): boolean {
  if (value === true) return true
  if (!value || typeof value !== 'object') return false
  return Object.values(value).some(hasDirtySettingsValue)
}

/** Overlay edited values without normalizing or serializing the saved values. */
export function mergeDirtySettingsValues<T extends FieldValues>(defaults: T, current: T, dirty: unknown): T {
  if (!hasDirtySettingsValue(dirty)) return defaults
  if (dirty === true || Array.isArray(current)) return current
  const saved = defaults as Record<string, unknown> | undefined
  const values = current as Record<string, unknown> | undefined
  const result: Record<string, unknown> = { ...saved }
  for (const [key, marker] of Object.entries(dirty as Record<string, unknown>)) {
    if (!hasDirtySettingsValue(marker)) continue
    result[key] = mergeDirtySettingsValues(saved?.[key] as FieldValues, values?.[key] as FieldValues, marker)
  }
  return result as T
}

function changedSettingsValues(before: unknown, after: unknown): unknown {
  if (Object.is(before, after)) return false
  if (Array.isArray(before) || Array.isArray(after)) return JSON.stringify(before) !== JSON.stringify(after)
  if (before && after && typeof before === 'object' && typeof after === 'object') {
    const left = before as Record<string, unknown>
    const right = after as Record<string, unknown>
    const changed: Record<string, unknown> = {}
    for (const key of new Set([...Object.keys(left), ...Object.keys(right)])) changed[key] = changedSettingsValues(left[key], right[key])
    return changed
  }
  return true
}

/** Preserve only edits made after this particular save collected its values. */
export function preserveSettingsEdits<T extends FieldValues>(saved: T, submitted: T, current: T): T {
  return mergeDirtySettingsValues(saved, current, changedSettingsValues(submitted, current))
}

/**
 * Reset a react-hook-form instance whenever the provided default values change.
 * Guards against naively resetting on every render by tracking the last
 * serialized snapshot of the defaults.
 */
export function useResetForm<TFieldValues extends FieldValues, TContext = unknown, TTransformedValues = TFieldValues>(
  form: UseFormReturn<TFieldValues, TContext, TTransformedValues>,
  values: DefaultValues<TFieldValues> | undefined
) {
  const lastSerializedDefaults = useRef<string | null>(null)
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  const actor = useRef(identity)
  const reloadGeneration = useSettingsReloadGeneration()
  const lastReload = useRef(reloadGeneration)
  const dirtyFields = form.formState.dirtyFields

  useEffect(() => {
    if (!values) return

    const serializedDefaults = JSON.stringify(values)
    const actorChanged = actor.current !== identity
    const explicitReload = reloadGeneration !== lastReload.current
    if (!actorChanged && !explicitReload && serializedDefaults === lastSerializedDefaults.current) {
      return
    }

    const current = form.getValues()
    const resetTask = actorChanged || explicitReload
    const next = resetTask ? values : mergeDirtySettingsValues(values as TFieldValues, current, dirtyFields)
    form.reset(values)
    if (!resetTask) form.reset(next as TFieldValues, { keepDefaultValues: true })
    lastSerializedDefaults.current = serializedDefaults
    actor.current = identity
    lastReload.current = reloadGeneration
  }, [values, form, identity, dirtyFields, reloadGeneration])
}
