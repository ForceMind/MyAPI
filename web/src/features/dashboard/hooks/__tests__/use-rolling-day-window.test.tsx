/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { act, renderHook } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { useRollingDayWindow } from '../use-rolling-day-window'

afterEach(() => vi.useRealTimers())

test('requests the last 24 hours ending now and advances the window each minute', () => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-09-09T09:00:00Z'))
  const now = Math.floor(Date.now() / 1000)
  const { result, unmount } = renderHook(() => useRollingDayWindow())
  expect(result.current).toEqual({
    start_timestamp: now - 86400,
    end_timestamp: now,
  })
  act(() => vi.advanceTimersByTime(60000))
  expect(result.current).toEqual({
    start_timestamp: now + 60 - 86400,
    end_timestamp: now + 60,
  })
  unmount()
})
