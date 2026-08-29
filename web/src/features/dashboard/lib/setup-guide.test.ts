import { describe, expect, test } from 'vitest'

import { resolveSetupGuideExpanded } from './setup-guide'

describe('setup guide visibility', () => {
  test('stays hidden until setup status is ready', () => {
    expect(resolveSetupGuideExpanded(false, false, null)).toBe(false)
  })

  test('expands by default while setup is incomplete', () => {
    expect(resolveSetupGuideExpanded(true, false, null)).toBe(true)
  })

  test('honors a user collapse preference while incomplete', () => {
    expect(resolveSetupGuideExpanded(true, false, false)).toBe(false)
    expect(resolveSetupGuideExpanded(true, false, true)).toBe(true)
  })

  test('always hides after setup completes, including stale expanded state', () => {
    expect(resolveSetupGuideExpanded(true, true, true)).toBe(false)
    expect(resolveSetupGuideExpanded(true, true, false)).toBe(false)
  })
})
