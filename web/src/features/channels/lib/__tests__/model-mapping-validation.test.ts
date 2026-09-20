import { describe, expect, test } from 'vitest'

import {
  extractMappingSourceModels,
  extractRedirectModels,
  findMissingModelsInMapping,
  formatModelsArray,
  validateModelMappingJson,
} from '../model-mapping-validation'

describe('model mapping validation', () => {
  test('preserves the first model order while deduplicating mapping models', () => {
    const mapping = JSON.stringify({
      ' gpt-5 ': 'o3',
      'gpt-4.1': 'o3',
      'gpt-5-mini': 'gpt-5',
    })

    expect(formatModelsArray(['gpt-5', 'gpt-4.1', 'gpt-5'])).toBe(
      'gpt-5,gpt-4.1'
    )
    expect(extractMappingSourceModels(mapping)).toEqual([
      'gpt-5',
      'gpt-4.1',
      'gpt-5-mini',
    ])
    expect(extractRedirectModels(mapping)).toEqual(['o3', 'gpt-5'])
    expect(
      findMissingModelsInMapping(mapping, ['gpt-5', 'gpt-5-mini'])
    ).toEqual(['gpt-4.1'])
  })

  test('keeps invalid mapping classifications distinct', () => {
    expect(validateModelMappingJson('[]')).toEqual({
      valid: false,
      error: 'Model mapping must be a valid JSON object',
    })
    expect(validateModelMappingJson('{"gpt-5": 1}')).toEqual({
      valid: false,
      error: 'Model mapping values must be strings',
    })
  })
})
