/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError } from 'axios'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import {
  getChannelModelDiscovery,
  refreshChannelModelDiscovery,
} from '../../api'
import type { ChannelModelDiscoveryResponse } from '../../types'
import { ChannelModelDiscovery } from '../channel-model-discovery'

vi.mock('../../api', () => ({
  getChannelModelDiscovery: vi.fn(),
  refreshChannelModelDiscovery: vi.fn(),
}))
function renderDiscovery(channelId: number | null = 1) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      <ChannelModelDiscovery channelId={channelId} />
    </QueryClientProvider>
  )
}
const evidence = {
  models: ['upstream-one'],
  source: 'openai_models' as const,
  status: 'success' as const,
  fetched_at: 1728000000,
  checked_at: 1728000000,
  stale: false,
}
describe('model discovery evidence', () => {
  beforeEach(() =>
    useAuthStore.getState().auth.setUser({ id: 1, username: 'test', role: 100 })
  )
  afterEach(() => useAuthStore.getState().auth.reset())
  test('unsaved channel explains manual models without fetching', () => {
    renderDiscovery(null)
    expect(
      screen.getByText(
        'Save the channel to discover models. Manually entered models are unverified.'
      )
    ).toBeInTheDocument()
    expect(getChannelModelDiscovery).not.toHaveBeenCalled()
  })
  test('loading disables refresh until discovery is available', async () => {
    vi.mocked(getChannelModelDiscovery).mockReturnValue(new Promise(() => {}))
    renderDiscovery()
    expect(screen.getByRole('status')).toHaveTextContent('Loading...')
    expect(
      screen.getByRole('button', { name: 'Refresh model discovery' })
    ).toBeDisabled()
  })
  test('empty discovery is explicit rather than inferred as success', async () => {
    vi.mocked(getChannelModelDiscovery).mockResolvedValue({
      success: true,
      data: { ...evidence, models: [], status: 'empty' },
    })
    renderDiscovery()
    expect(
      await screen.findByText('Upstream returned no models')
    ).toBeInTheDocument()
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })
  test('failed refresh retains stale models and successful timestamp', async () => {
    vi.mocked(getChannelModelDiscovery).mockResolvedValue({
      success: true,
      data: evidence,
    })
    vi.mocked(refreshChannelModelDiscovery).mockResolvedValue({
      success: false,
      data: {
        ...evidence,
        status: 'failed',
        stale: true,
        checked_at: 1729000000,
      },
    })
    renderDiscovery()
    await screen.findByText('upstream-one')
    await userEvent.click(
      screen.getByRole('button', { name: 'Refresh model discovery' })
    )
    expect(
      await screen.findByText(
        'Model discovery failed; previous evidence is retained'
      )
    ).toBeInTheDocument()
    expect(screen.getByText('upstream-one')).toBeInTheDocument()
    expect(screen.getByText('Stale discovery evidence')).toBeInTheDocument()
  })
  test('transport failure offers retry with explicit failure state', async () => {
    vi.mocked(getChannelModelDiscovery).mockRejectedValue(new Error('network'))
    renderDiscovery()
    expect(
      await screen.findByText(
        'Model discovery failed; previous evidence is retained'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Refresh model discovery' })
    ).toBeEnabled()
  })
  test('permission failure is explicit and disables unauthorized refresh', async () => {
    const error = new AxiosError('forbidden')
    error.response = { status: 403 } as typeof error.response
    vi.mocked(getChannelModelDiscovery).mockRejectedValue(error)
    renderDiscovery()
    expect(
      await screen.findByText("You don't have necessary permission")
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Refresh model discovery' })
    ).toBeDisabled()
  })

  test('authorization loss hides previously cached discovery evidence', async () => {
    vi.mocked(getChannelModelDiscovery).mockResolvedValue({
      success: true,
      data: evidence,
    })
    const error = new AxiosError('forbidden')
    error.response = { status: 403 } as typeof error.response
    vi.mocked(refreshChannelModelDiscovery).mockRejectedValue(error)
    renderDiscovery()
    await screen.findByText('upstream-one')
    await userEvent.click(
      screen.getByRole('button', { name: 'Refresh model discovery' })
    )
    await screen.findByText("You don't have necessary permission")
    expect(screen.queryByText('upstream-one')).not.toBeInTheDocument()
    expect(screen.queryByText('openai_models')).not.toBeInTheDocument()
  })
  test('switching users never reuses the previous user discovery cache', async () => {
    vi.mocked(getChannelModelDiscovery)
      .mockResolvedValueOnce({ success: true, data: evidence })
      .mockReturnValue(new Promise(() => {}))
    renderDiscovery()
    await screen.findByText('upstream-one')
    act(() =>
      useAuthStore
        .getState()
        .auth.setUser({ id: 2, username: 'other', role: 100 })
    )
    expect(screen.queryByText('upstream-one')).not.toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('Loading...')
  })
  test('a refresh completing after user switch cannot populate the new scope', async () => {
    let resolveRefresh:
      | ((response: ChannelModelDiscoveryResponse) => void)
      | undefined
    vi.mocked(getChannelModelDiscovery)
      .mockResolvedValueOnce({ success: true, data: evidence })
      .mockReturnValue(new Promise(() => {}))
    vi.mocked(refreshChannelModelDiscovery).mockReturnValue(
      new Promise((resolve) => {
        resolveRefresh = resolve
      })
    )
    renderDiscovery()
    await screen.findByText('upstream-one')
    await userEvent.click(
      screen.getByRole('button', { name: 'Refresh model discovery' })
    )
    act(() =>
      useAuthStore
        .getState()
        .auth.setUser({ id: 2, username: 'other', role: 100 })
    )
    await act(async () => {
      resolveRefresh?.({
        success: true,
        data: { ...evidence, models: ['private-late-result'] },
      })
    })
    expect(screen.queryByText('private-late-result')).not.toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('Loading...')
  })
})
