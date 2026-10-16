/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { ModelScope, ModelScopeCommand } from '../model-scope-api'

export type ModelScopeMode = 'unlimited' | 'restricted' | 'blocked'

export type ModelScopeDraft = {
  mode: ModelScopeMode
  models: string[]
}

export type ModelScopeValidationError =
  | 'empty-restricted-scope'
  | 'invalid-new-model'
  | 'duplicate-model'
  | 'scope-too-large'
  | null

export function modelScopeDraft(scope: ModelScope): ModelScopeDraft {
  if (!scope.model_limits_enabled) {
    return { mode: 'unlimited', models: [...scope.models] }
  }
  if (scope.models.length === 0) {
    return { mode: 'blocked', models: [] }
  }
  return { mode: 'restricted', models: [...scope.models] }
}

export function modelScopeCommand(
  scope: ModelScope,
  draft: ModelScopeDraft
): ModelScopeCommand {
  return {
    expected_revision: scope.revision,
    model_limits_enabled: draft.mode !== 'unlimited',
    models: draft.mode === 'blocked' ? [] : [...draft.models],
  }
}

export function sameModelScopeDraft(
  left: ModelScopeDraft,
  right: ModelScopeDraft
): boolean {
  return (
    left.mode === right.mode &&
    left.models.length === right.models.length &&
    left.models.every((model, index) => model === right.models[index])
  )
}

export function modelScopeMatchesCommand(
  scope: ModelScope,
  command: ModelScopeCommand
): boolean {
  return (
    scope.model_limits_enabled === command.model_limits_enabled &&
    scope.models.length === command.models.length &&
    scope.models.every((model, index) => model === command.models[index])
  )
}

export function isValidNewModel(model: string): boolean {
  const hasDisallowedCharacter = [...model].some((character) => {
    const codePoint = character.codePointAt(0) ?? 0
    return (
      /\s/u.test(character) ||
      codePoint <= 0x1f ||
      (codePoint >= 0x7f && codePoint <= 0x9f) ||
      codePoint === 0xfeff
    )
  })
  return model.length > 0 && !model.includes(',') && !hasDisallowedCharacter
}

export function modelScopeListItems(models: string[]) {
  const occurrences = new Map<string, number>()
  return models.map((model) => {
    const occurrence = occurrences.get(model) ?? 0
    occurrences.set(model, occurrence + 1)
    return { model, key: JSON.stringify([model, occurrence]) }
  })
}

export function validateModelScopeDraft(
  draft: ModelScopeDraft,
  originalModels: string[]
): ModelScopeValidationError {
  if (draft.mode === 'restricted' && draft.models.length === 0) {
    return 'empty-restricted-scope'
  }

  const savedModels = draft.mode === 'blocked' ? [] : draft.models
  if (new Set(savedModels).size !== savedModels.length) {
    return 'duplicate-model'
  }

  const originalCounts = new Map<string, number>()
  for (const model of originalModels) {
    originalCounts.set(model, (originalCounts.get(model) ?? 0) + 1)
  }
  for (const model of draft.models) {
    const remaining = originalCounts.get(model) ?? 0
    if (remaining > 0) {
      originalCounts.set(model, remaining - 1)
      continue
    }
    if (!isValidNewModel(model)) return 'invalid-new-model'
  }

  const encoded = new TextEncoder().encode(savedModels.join(','))
  if (encoded.byteLength > 65535) return 'scope-too-large'
  return null
}

export function modelScopeCatalog(
  catalog: string[],
  selected: string[],
  search: string
): string[] {
  const selectedSet = new Set(selected)
  const normalizedSearch = search.trim().toLocaleLowerCase()
  const result: string[] = []
  const seen = new Set<string>()
  for (const model of catalog) {
    if (
      seen.has(model) ||
      selectedSet.has(model) ||
      !isValidNewModel(model) ||
      (normalizedSearch &&
        !model.toLocaleLowerCase().includes(normalizedSearch))
    ) {
      continue
    }
    seen.add(model)
    result.push(model)
  }
  return result
}
