/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  copyChannel,
  createChannel,
  getChannelQuotaChanges,
  getChannelQuotaAlertDeliveryEvents,
  getChannelQuotaAlertDeliveryStatus,
  getChannelQuotaHistory,
  getChannelRoutingPreview,
  getChannelQuotaSamplingStatus,
  getCodexLocalAuthStatus,
  importCodexLocalAuthForChannel,
  importCodexLocalAuthForNewChannel,
  runChannelQuotaAlertDelivery,
  updateChannelQuotaAlertDeliverySettings,
} from './api'

vi.mock('@/lib/api', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
  },
}))

describe('channel quota API authentication behavior', () => {
  test('forwards independent quota analysis parameters to history', async () => {
    vi.mocked(api.get).mockResolvedValueOnce({
      data: { success: true },
    } as never)

    await getChannelQuotaHistory(12, {
      range: '24h',
      granularity: 'hour',
      rate_window: 3600,
      ewma_half_life: 1800,
    })

    expect(api.get).toHaveBeenLastCalledWith(
      '/api/channel/12/quota/history',
      expect.objectContaining({
        params: expect.objectContaining({
          range: '24h',
          granularity: 'hour',
          rate_window: 3600,
          ewma_half_life: 1800,
        }),
      })
    )
  })

  test('forwards persistent operation keys for create and copy retries', async () => {
    vi.mocked(api.post).mockResolvedValue({ data: { success: true } } as never)

    await createChannel(
      { mode: 'single', channel: { name: 'A', type: 1, key: 'secret' } },
      'create-operation-key'
    )
    await copyChannel(12, {
      suffix: '-copy',
      reset_balance: true,
      operation_key: 'copy-operation-key',
    })

    expect(api.post).toHaveBeenNthCalledWith(
      1,
      '/api/channel',
      { mode: 'single', channel: { name: 'A', type: 1, key: 'secret' } },
      expect.objectContaining({
        headers: { 'Idempotency-Key': 'create-operation-key' },
      })
    )
    expect(api.post).toHaveBeenNthCalledWith(
      2,
      '/api/channel/copy/12',
      null,
      expect.objectContaining({
        params: { suffix: '-copy', reset_balance: true },
        headers: { 'Idempotency-Key': 'copy-operation-key' },
      })
    )
  })

  test('forwards explicit routing preview parameters', async () => {
    vi.mocked(api.post).mockClear()
    vi.mocked(api.get).mockResolvedValueOnce({
      data: { success: true, data: { tiers: [] } },
    } as never)

    await getChannelRoutingPreview({
      group: 'default',
      model: 'gpt-test',
      request_path: '/v1/responses',
    })

    expect(api.get).toHaveBeenLastCalledWith('/api/channel/routing-preview', {
      skipBusinessError: true,
      skipErrorHandler: true,
      params: {
        group: 'default',
        model: 'gpt-test',
        request_path: '/v1/responses',
      },
    })
    expect(api.post).not.toHaveBeenCalled()
  })

  test('keeps the standard auth refresh path for quota changes', async () => {
    vi.mocked(api.get).mockResolvedValueOnce({
      data: { success: true, data: { items: [] } },
    } as never)

    await getChannelQuotaChanges({ range: '24h', limit: 5 })

    const config = vi.mocked(api.get).mock.calls[0]?.[1] as Record<
      string,
      unknown
    >
    expect(config.skipAuthRefresh).toBeUndefined()
    expect(config.skipErrorHandler).toBe(true)
    expect(config.skipBusinessError).toBe(true)
  })

  test('keeps the standard auth refresh path for sampler status', async () => {
    vi.mocked(api.get).mockResolvedValueOnce({
      data: { success: true, data: { enabled: false } },
    } as never)

    await getChannelQuotaSamplingStatus()

    const config = vi.mocked(api.get).mock.calls[0]?.[1] as Record<
      string,
      unknown
    >
    expect(config.skipAuthRefresh).toBeUndefined()
    expect(config.skipErrorHandler).toBe(true)
    expect(config.skipBusinessError).toBe(true)
  })

  test('uses protected quota alert delivery endpoints for status, history, configuration, and manual runs', async () => {
    vi.mocked(api.get).mockResolvedValue({ data: { success: true } } as never)
    vi.mocked(api.put).mockResolvedValue({ data: { success: true } } as never)
    vi.mocked(api.post).mockResolvedValue({ data: { success: true } } as never)

    await getChannelQuotaAlertDeliveryStatus()
    await getChannelQuotaAlertDeliveryEvents({
      p: 2,
      page_size: 10,
      state: 'retryable',
    })
    await updateChannelQuotaAlertDeliverySettings({
      webhook_url: 'https://alerts.example.com/quota',
      webhook_secret: '0123456789abcdef',
    })
    await runChannelQuotaAlertDelivery()

    expect(api.get).toHaveBeenNthCalledWith(
      1,
      '/api/channel/quota/alerts/delivery',
      expect.objectContaining({
        skipBusinessError: true,
        skipErrorHandler: true,
      })
    )
    expect(api.get).toHaveBeenNthCalledWith(
      2,
      '/api/channel/quota/alerts',
      expect.objectContaining({
        params: { p: 2, page_size: 10, state: 'retryable' },
      })
    )
    expect(api.put).toHaveBeenCalledWith(
      '/api/channel/quota/alerts/delivery',
      {
        webhook_url: 'https://alerts.example.com/quota',
        webhook_secret: '0123456789abcdef',
      },
      expect.objectContaining({
        skipBusinessError: true,
        skipErrorHandler: true,
      })
    )
    expect(api.post).toHaveBeenCalledWith(
      '/api/channel/quota/alerts/delivery/run',
      undefined,
      expect.objectContaining({
        skipBusinessError: true,
        skipErrorHandler: true,
      })
    )
  })

  test('uses protected local-auth endpoints and forwards a security proof only for imports', async () => {
    vi.mocked(api.get).mockResolvedValueOnce({
      data: { success: true },
    } as never)
    vi.mocked(api.post).mockResolvedValue({ data: { success: true } } as never)

    await getCodexLocalAuthStatus()
    await importCodexLocalAuthForNewChannel(
      { mode: 'single', channel: { name: 'Codex', type: 57, key: '' } },
      'proof-token'
    )
    await importCodexLocalAuthForChannel(12, 'proof-token')

    expect(api.get).toHaveBeenLastCalledWith(
      '/api/channel/codex/local-auth/status',
      expect.objectContaining({
        disableDuplicate: true,
        skipBusinessError: true,
        skipErrorHandler: true,
      })
    )
    expect(api.post).toHaveBeenNthCalledWith(
      1,
      '/api/channel/codex/local-auth/import',
      { mode: 'single', channel: { name: 'Codex', type: 57, key: '' } },
      expect.objectContaining({
        headers: { 'X-Security-Proof': 'proof-token' },
      })
    )
    expect(api.post).toHaveBeenNthCalledWith(
      2,
      '/api/channel/12/codex/local-auth/import',
      undefined,
      expect.objectContaining({
        headers: { 'X-Security-Proof': 'proof-token' },
      })
    )
  })
})
