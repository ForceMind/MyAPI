/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { describe, expect, test } from 'vitest'

import type { ModelScope } from '../../model-scope-api'
import {
  isValidNewModel,
  modelScopeCatalog,
  modelScopeCommand,
  modelScopeDraft,
  modelScopeListItems,
  modelScopeMatchesCommand,
  validateModelScopeDraft,
} from '../model-scope'

const revision = 'a'.repeat(64)
const view = (overrides: Partial<ModelScope> = {}): ModelScope => ({
  token_id: 21,
  token_name: 'Desktop application',
  model_limits_enabled: false,
  models: ['legacy-model'],
  revision,
  ...overrides,
})

describe('model scope semantics', () => {
  test('distinguishes no extra limit from the explicit empty deny-all state', () => {
    expect(modelScopeDraft(view())).toEqual({
      mode: 'unlimited',
      models: ['legacy-model'],
    })
    expect(
      modelScopeDraft(view({ model_limits_enabled: true, models: [] }))
    ).toEqual({ mode: 'blocked', models: [] })
  })

  test('turning off limits keeps the persisted list in the narrow payload', () => {
    expect(
      modelScopeCommand(view({ model_limits_enabled: true }), {
        mode: 'unlimited',
        models: ['legacy-model'],
      })
    ).toEqual({
      expected_revision: revision,
      model_limits_enabled: false,
      models: ['legacy-model'],
    })
  })

  test('requires a nonempty ordinary restricted scope and treats deny-all separately', () => {
    expect(
      validateModelScopeDraft({ mode: 'restricted', models: [] }, [
        'legacy-model',
      ])
    ).toBe('empty-restricted-scope')
    expect(
      validateModelScopeDraft({ mode: 'blocked', models: ['legacy-model'] }, [
        'legacy-model',
      ])
    ).toBeNull()
  })

  test('preserves invalid legacy IDs but rejects the same shape as a new selection', () => {
    const legacy = 'legacy model'
    expect(
      validateModelScopeDraft(
        { mode: 'restricted', models: [legacy, 'current-model'] },
        [legacy]
      )
    ).toBeNull()
    expect(
      validateModelScopeDraft({ mode: 'restricted', models: [legacy] }, [])
    ).toBe('invalid-new-model')
    expect(isValidNewModel('model,alias')).toBe(false)
    expect(isValidNewModel('model\0alias')).toBe(false)
    expect(isValidNewModel('model\u0001alias')).toBe(false)
    expect(isValidNewModel('model\u0085alias')).toBe(false)
    expect(isValidNewModel('model\ufeffalias')).toBe(false)
  })

  test('requires the user to remove duplicate persisted IDs instead of silently deduplicating', () => {
    expect(
      validateModelScopeDraft(
        { mode: 'restricted', models: ['same', 'same'] },
        ['same', 'same']
      )
    ).toBe('duplicate-model')
    expect(
      modelScopeListItems(['same', 'same']).map((item) => item.key)
    ).toEqual(['["same",0]', '["same",1]'])
  })

  test('catalog suggestions exclude invalid, duplicate, selected, and nonmatching values', () => {
    expect(
      modelScopeCatalog(
        ['alpha-model', 'beta-model', 'beta-model', 'bad model'],
        ['alpha-model'],
        'BETA'
      )
    ).toEqual(['beta-model'])
  })

  test('readback matching includes the enabled bit and exact preserved model sequence', () => {
    const command = modelScopeCommand(view(), {
      mode: 'restricted',
      models: ['alpha', 'beta'],
    })
    expect(
      modelScopeMatchesCommand(
        view({ model_limits_enabled: true, models: ['alpha', 'beta'] }),
        command
      )
    ).toBe(true)
    expect(
      modelScopeMatchesCommand(
        view({ model_limits_enabled: true, models: ['beta', 'alpha'] }),
        command
      )
    ).toBe(false)
  })
})
