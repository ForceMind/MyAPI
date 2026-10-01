/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { act, renderHook, waitFor } from '@testing-library/react'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { getHomePageContent } from '../../api'
import { useHomePageContent } from '../use-home-page-content'

vi.mock('../../api', () => ({ getHomePageContent: vi.fn() }))
vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

const originalStorage = Object.getOwnPropertyDescriptor(window, 'localStorage')

beforeEach(() => {
  vi.mocked(getHomePageContent).mockReset()
  const values = new Map<string, string>()
  Object.defineProperty(window, 'localStorage', {
    configurable: true,
    value: {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => {
        values.set(key, value)
      },
      removeItem: (key: string) => {
        values.delete(key)
      },
      clear: () => values.clear(),
      key: (index: number) => [...values.keys()][index] ?? null,
      get length() {
        return values.size
      },
    },
  })
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.clearAllMocks()
  if (originalStorage) {
    Object.defineProperty(window, 'localStorage', originalStorage)
  }
})

test('shows the default home after a successful empty response when cache reads are blocked', async () => {
  vi.spyOn(window.localStorage, 'getItem').mockImplementation(() => {
    throw new DOMException('storage blocked', 'SecurityError')
  })
  expect(() => window.localStorage.getItem('home_page_content')).toThrow(
    'storage blocked'
  )
  vi.mocked(getHomePageContent).mockResolvedValue({ success: true, data: '' })

  const { result } = renderHook(() => useHomePageContent())

  await waitFor(() => expect(result.current.isLoaded).toBe(true))
  expect(result.current.content).toBe('')
  expect(getHomePageContent).toHaveBeenCalledOnce()
  expect(toast.error).not.toHaveBeenCalled()
})

test('keeps the server custom home without an error toast when cache writes are blocked', async () => {
  vi.spyOn(window.localStorage, 'setItem').mockImplementation(() => {
    throw new DOMException('storage blocked', 'SecurityError')
  })
  expect(() =>
    window.localStorage.setItem('home_page_content', 'cached')
  ).toThrow('storage blocked')
  vi.mocked(getHomePageContent).mockResolvedValue({
    success: true,
    data: '# Instance home',
  })

  const { result } = renderHook(() => useHomePageContent())

  await waitFor(() => expect(result.current.isLoaded).toBe(true))
  expect(result.current.content).toBe('# Instance home')
  expect(toast.error).not.toHaveBeenCalled()
})

test('returns to the default home when the server clears custom content but cache removal is blocked', async () => {
  window.localStorage.setItem('home_page_content', '# Stale home')
  vi.spyOn(window.localStorage, 'removeItem').mockImplementation(() => {
    throw new DOMException('storage blocked', 'SecurityError')
  })
  vi.mocked(getHomePageContent).mockResolvedValue({ success: true, data: '' })

  const { result } = renderHook(() => useHomePageContent())

  await waitFor(() => expect(result.current.isLoaded).toBe(true))
  expect(result.current.content).toBe('')
  expect(toast.error).not.toHaveBeenCalled()
})

test('failed server read shows a retryable fallback without stale administrator content or toast', async () => {
  window.localStorage.setItem('home_page_content', '# Stale instance home')
  vi.mocked(getHomePageContent)
    .mockRejectedValueOnce(new Error('offline'))
    .mockResolvedValueOnce({ success: true, data: '# Current instance home' })

  const { result } = renderHook(() => useHomePageContent())

  await waitFor(() => expect(result.current.isLoaded).toBe(true))
  expect(result.current.content).toBe('')
  expect(window.localStorage.getItem('home_page_content')).toBeNull()
  expect(result.current.failed).toBe(true)
  expect(toast.error).not.toHaveBeenCalled()

  act(() => result.current.retry())
  await waitFor(() =>
    expect(result.current.content).toBe('# Current instance home')
  )
  expect(result.current.failed).toBe(false)
  expect(toast.error).not.toHaveBeenCalled()
})

test('business failure is not mistaken for an intentionally empty custom home', async () => {
  vi.mocked(getHomePageContent).mockResolvedValue({
    success: false,
    message: 'unavailable',
    data: '',
  })

  const { result } = renderHook(() => useHomePageContent())

  await waitFor(() => expect(result.current.isLoaded).toBe(true))
  expect(result.current.content).toBe('')
  expect(result.current.failed).toBe(true)
  expect(toast.error).not.toHaveBeenCalled()
})

test('successful response missing its required string is not treated as configured empty content', async () => {
  vi.mocked(getHomePageContent).mockResolvedValue({ success: true })

  const { result } = renderHook(() => useHomePageContent())

  await waitFor(() => expect(result.current.isLoaded).toBe(true))
  expect(result.current.content).toBe('')
  expect(result.current.failed).toBe(true)
})
