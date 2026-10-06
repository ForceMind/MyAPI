import { expect, test } from 'vitest'

import { parseHeaderNavModules } from '../nav-modules'

test('fresh navigation omits model-square and rankings promotions by default', () => {
  const modules = parseHeaderNavModules('')

  expect(modules.home).toBe(true)
  expect(modules.console).toBe(true)
  expect(modules.docs).toBe(true)
  expect(modules.pricing.enabled).toBe(false)
  expect(modules.rankings.enabled).toBe(false)
})

test('an explicit administrator configuration can still enable both destinations', () => {
  const modules = parseHeaderNavModules(
    JSON.stringify({
      pricing: { enabled: true, requireAuth: true },
      rankings: { enabled: true, requireAuth: false },
    })
  )

  expect(modules.pricing).toEqual({ enabled: true, requireAuth: true })
  expect(modules.rankings).toEqual({ enabled: true, requireAuth: false })
})
