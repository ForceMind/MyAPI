/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'

import { getBuildRevision, installBuildMetadata } from '@/lib/build-metadata'

describe('runtime build metadata', () => {
  const buildId = 'a'.repeat(40)

  beforeAll(() => {
    vi.stubEnv('VITE_REACT_APP_VERSION', '9.8.7')
    vi.stubEnv('VITE_BUILD_ID', buildId)
  })

  afterAll(() => vi.unstubAllEnvs())

  it('returns the injected version and exact build identity', () => {
    const revision = getBuildRevision()

    expect(revision).toBe(`rv.9.8.7.${buildId}.2k6e8r7p`)
    expect(revision).toMatch(
      /^rv\.[0-9A-Za-z._-]+\.[0-9A-Za-z._-]+\.[0-9a-z]+$/
    )
    expect(revision).not.toMatch(/secret|token|cookie|jwt/i)
  })

  it('installs the revision in supported runtime inspection surfaces', () => {
    installBuildMetadata()
    const revision = getBuildRevision()

    expect(document.documentElement).toHaveAttribute('data-build-rev', revision)
    expect(document.documentElement).toHaveAttribute(
      'data-app-channel',
      '2k6e8r7p'
    )
    expect(document.querySelector('meta[name="build-id"]')).toHaveAttribute(
      'content',
      revision
    )
    expect(window.localStorage.getItem('app:rev')).toBe(revision)
    expect(window.__APP_BUILD__?.rev).toBe(revision)
  })
})
