import { describe, expect, test } from 'vitest'

import type { PricingModel } from '../types'
import { buildRateLimits, formatRateLimit } from './mock-stats'

const baseModel: PricingModel = {
  id: 1,
  model_name: 'gpt-5',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 1,
  enable_groups: ['zeta', 'auto', 'alpha'],
}

describe('mock rate limits', () => {
  test('sorts enabled groups and remains deterministic for a model', () => {
    const first = buildRateLimits(baseModel)

    expect(first.map((limit) => limit.group)).toEqual(['alpha', 'zeta'])
    expect(buildRateLimits(baseModel)).toEqual(first)
  })

  test('keeps rate-limit category defaults and compact formatting boundaries', () => {
    expect(buildRateLimits({ ...baseModel, model_name: 'image-gen' })).toEqual(
      expect.arrayContaining([expect.objectContaining({ tpm: 0 })])
    )
    expect(formatRateLimit(9_999)).toBe('10.0K')
    expect(formatRateLimit(10_000)).toBe('10K')
  })
})
