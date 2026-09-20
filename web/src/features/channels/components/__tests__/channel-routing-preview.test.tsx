import { readFileSync } from 'node:fs'
import path from 'node:path'

/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { getChannelRoutingPreview } from '../../api'
import { channelsQueryKeys } from '../../lib'
import { ChannelRoutingPreview } from '../channel-routing-preview'

vi.mock('../../api', () => ({
  getChannelRoutingPreview: vi.fn(),
}))

function renderPreview() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return {
    client,
    ...render(
      <QueryClientProvider client={client}>
        <ChannelRoutingPreview />
      </QueryClientProvider>
    ),
  }
}

describe('channel routing preview', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })
  test('submits an explicit route and renders effective shares', async () => {
    vi.mocked(getChannelRoutingPreview).mockResolvedValue({
      success: true,
      data: {
        group: 'default',
        model: 'gpt-test',
        request_path: '/v1/responses',
        source: 'database',
        generation: 1,
        data_generation: 1,
        published_generation: 1,
        cluster_committed_epoch: 1,
        local_published_epoch: 1,
        cache_enabled: false,
        cache_pending: false,
        tiers: [
          {
            priority: 100,
            fallback_index: 0,
            channels: [
              {
                id: 1,
                name: 'Primary channel',
                type: 1,
                weight: 20,
                effective_weight: 20,
                expected_share: 1,
                key: 'must-not-render',
                base_url: 'https://must-not-render.example',
              },
              {
                id: 2,
                name: 'Zero channel',
                type: 1,
                weight: 0,
                effective_weight: 0,
                expected_share: 0,
              },
            ],
          },
        ],
        affinity: {
          evaluated: false,
          precedence: 'before_priority_weight',
          explanation_code: 'routing_preview_affinity_not_evaluated',
        },
      },
    } as never)
    const user = userEvent.setup()
    renderPreview()

    await user.type(screen.getByLabelText('Routing preview model'), 'gpt-test')
    await user.type(
      screen.getByLabelText('Routing preview request path'),
      '/v1/responses'
    )
    await user.click(screen.getByRole('button', { name: 'Preview routing' }))

    expect(getChannelRoutingPreview).toHaveBeenCalledWith({
      group: 'default',
      model: 'gpt-test',
      request_path: '/v1/responses',
    })
    expect(await screen.findByText('Primary channel')).toBeInTheDocument()
    expect(screen.getByText('Zero channel')).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: 'Priority 100' })
    ).toBeInTheDocument()
    expect(screen.getByText('Fallback tier 1')).toBeInTheDocument()
    expect(screen.getByText('Weight: 20')).toBeInTheDocument()
    expect(screen.getByText('Effective weight: 20')).toBeInTheDocument()
    expect(screen.getByText('Expected share: 100%')).toBeInTheDocument()
    expect(screen.getByText('Expected share: 0%')).toBeInTheDocument()
    expect(screen.queryByText('must-not-render')).not.toBeInTheDocument()
    expect(
      screen.queryByText('https://must-not-render.example')
    ).not.toBeInTheDocument()
  })

  test('shows a no-match result after a successful preview response', async () => {
    vi.mocked(getChannelRoutingPreview).mockResolvedValue({
      success: true,
      data: {
        group: 'default',
        model: 'missing-model',
        request_path: '',
        source: 'database',
        generation: 1,
        data_generation: 1,
        published_generation: 1,
        cluster_committed_epoch: 1,
        local_published_epoch: 1,
        cache_enabled: false,
        cache_pending: false,
        tiers: [],
        affinity: {
          evaluated: false,
          precedence: 'before_priority_weight',
          explanation_code: 'routing_preview_affinity_not_evaluated',
        },
      },
    })
    const user = userEvent.setup()
    renderPreview()

    await user.type(
      screen.getByLabelText('Routing preview model'),
      'missing-model'
    )
    await user.click(screen.getByRole('button', { name: 'Preview routing' }))

    expect(
      await screen.findByText(
        'No matching enabled channels were found for this routing preview.'
      )
    ).toBeInTheDocument()
  })

  test('shows a business error instead of an empty result', async () => {
    vi.mocked(getChannelRoutingPreview).mockResolvedValue({
      success: false,
      code: 'routing_preview_auto_group_unsupported',
      message: 'invalid parameters',
    })
    const user = userEvent.setup()
    renderPreview()

    await user.clear(screen.getByLabelText('Routing preview group'))
    await user.type(screen.getByLabelText('Routing preview group'), 'auto')
    await user.type(screen.getByLabelText('Routing preview model'), 'gpt-test')
    await user.click(screen.getByRole('button', { name: 'Preview routing' }))

    expect(
      await screen.findByText(
        'Routing preview does not support the auto group.'
      )
    ).toBeInTheDocument()
    expect(
      screen.queryByText(
        'No matching enabled channels were found for this routing preview.'
      )
    ).not.toBeInTheDocument()
  })

  test('hides a prior successful preview after a refresh fails', async () => {
    vi.mocked(getChannelRoutingPreview)
      .mockResolvedValueOnce({
        success: true,
        data: {
          group: 'default',
          model: 'gpt-test',
          request_path: '',
          source: 'database',
          generation: 1,
          data_generation: 1,
          published_generation: 1,
          cluster_committed_epoch: 1,
          local_published_epoch: 1,
          cache_enabled: false,
          cache_pending: false,
          tiers: [
            {
              priority: 100,
              fallback_index: 0,
              channels: [
                {
                  id: 1,
                  name: 'Previously current channel',
                  type: 1,
                  weight: 1,
                  effective_weight: 1,
                  expected_share: 1,
                },
              ],
            },
          ],
          affinity: {
            evaluated: false,
            precedence: 'before_priority_weight',
            explanation_code: 'routing_preview_affinity_not_evaluated',
          },
        },
      })
      .mockRejectedValueOnce({ isAxiosError: true, message: 'Network Error' })
    const user = userEvent.setup()
    renderPreview()
    await user.type(screen.getByLabelText('Routing preview model'), 'gpt-test')
    const submit = screen.getByRole('button', { name: 'Preview routing' })

    await user.click(submit)
    expect(
      await screen.findByText('Previously current channel')
    ).toBeInTheDocument()

    await user.click(submit)
    expect(
      await screen.findByText('Unable to reach the server. Try again.')
    ).toBeInTheDocument()
    expect(
      screen.queryByText('Previously current channel')
    ).not.toBeInTheDocument()
  })

  test.each([
    {
      name: 'rejected 400',
      error: {
        isAxiosError: true,
        response: {
          status: 400,
          data: { code: 'routing_preview_invalid_params' },
        },
      },
      expected: 'Group and model are required.',
    },
    {
      name: 'rejected 500',
      error: {
        isAxiosError: true,
        response: {
          status: 500,
          data: { code: 'routing_preview_database_error' },
        },
      },
      expected: 'Routing preview is temporarily unavailable.',
    },
    {
      name: 'rejected 403',
      error: {
        isAxiosError: true,
        response: { status: 403 },
      },
      expected: "You don't have necessary permission",
    },
    {
      name: 'network rejection',
      error: { isAxiosError: true, message: 'Network Error' },
      expected: 'Unable to reach the server. Try again.',
    },
  ])('renders one inline error for $name', async ({ error, expected }) => {
    vi.mocked(getChannelRoutingPreview).mockRejectedValue(error)
    const user = userEvent.setup()
    renderPreview()
    await user.type(screen.getByLabelText('Routing preview model'), 'gpt-test')

    await user.click(screen.getByRole('button', { name: 'Preview routing' }))

    expect(await screen.findByText(expected)).toBeInTheDocument()
    expect(screen.getAllByRole('alert')).toHaveLength(1)
  })

  test('refreshes on repeated submits and channel-list invalidation', async () => {
    vi.mocked(getChannelRoutingPreview).mockResolvedValue({
      success: true,
      data: {
        group: 'default',
        model: 'gpt-test',
        request_path: '',
        source: 'database',
        generation: 1,
        data_generation: 1,
        published_generation: 1,
        cluster_committed_epoch: 1,
        local_published_epoch: 1,
        cache_enabled: false,
        cache_pending: false,
        tiers: [],
        affinity: {
          evaluated: false,
          precedence: 'before_priority_weight',
          explanation_code: 'routing_preview_affinity_not_evaluated',
        },
      },
    })
    const user = userEvent.setup()
    const { client } = renderPreview()
    await user.type(screen.getByLabelText('Routing preview model'), 'gpt-test')
    const submit = screen.getByRole('button', { name: 'Preview routing' })

    await user.click(submit)
    await waitFor(() =>
      expect(getChannelRoutingPreview).toHaveBeenCalledTimes(1)
    )
    await user.click(submit)
    await waitFor(() =>
      expect(getChannelRoutingPreview).toHaveBeenCalledTimes(2)
    )

    await client.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
    await waitFor(() =>
      expect(getChannelRoutingPreview).toHaveBeenCalledTimes(3)
    )
  })

  test('uses headings for static guidance and routing tiers without announcing guidance as an alert', async () => {
    vi.mocked(getChannelRoutingPreview).mockResolvedValue({
      success: true,
      data: {
        group: 'default',
        model: 'gpt-test',
        request_path: '',
        source: 'database',
        generation: 1,
        data_generation: 1,
        published_generation: 1,
        cluster_committed_epoch: 1,
        local_published_epoch: 1,
        cache_enabled: false,
        cache_pending: false,
        tiers: [
          {
            priority: 100,
            fallback_index: 0,
            channels: [],
          },
        ],
        affinity: {
          evaluated: false,
          precedence: 'before_priority_weight',
          explanation_code: 'routing_preview_affinity_not_evaluated',
        },
      },
    })
    const user = userEvent.setup()
    renderPreview()
    const contractHeading = screen.getByRole('heading', {
      name: 'Runtime routing contract',
    })
    expect(contractHeading.closest('[role="alert"]')).toBeNull()
    const previewForm = screen.getByRole('form', { name: 'Routing Preview' })
    expect(previewForm).toHaveClass('grid-cols-1')
    expect(previewForm.className).toContain('md:grid-cols-')
    await user.type(screen.getByLabelText('Routing preview model'), 'gpt-test')
    await user.click(screen.getByRole('button', { name: 'Preview routing' }))

    const tierHeading = await screen.findByRole('heading', {
      name: 'Priority 100',
    })
    const tierSection = tierHeading.closest('section')
    expect(tierSection).toHaveAttribute(
      'aria-labelledby',
      tierHeading.getAttribute('id')
    )
  })

  test('clears the old preview when an input changes without refetching old parameters', async () => {
    vi.mocked(getChannelRoutingPreview).mockResolvedValue({
      success: true,
      data: {
        group: 'default',
        model: 'gpt-test',
        request_path: '',
        source: 'cache',
        generation: 4,
        data_generation: 5,
        published_generation: 4,
        cluster_committed_epoch: 5,
        local_published_epoch: 4,
        cache_enabled: true,
        cache_pending: true,
        tiers: [
          {
            priority: 100,
            fallback_index: 0,
            channels: [],
          },
        ],
        affinity: {
          evaluated: false,
          precedence: 'before_priority_weight',
          explanation_code: 'routing_preview_affinity_not_evaluated',
        },
      },
    })
    const user = userEvent.setup()
    const { client } = renderPreview()
    const modelInput = screen.getByLabelText('Routing preview model')
    await user.type(modelInput, 'gpt-test')
    await user.click(screen.getByRole('button', { name: 'Preview routing' }))

    expect(
      await screen.findByText('Runtime cache snapshot')
    ).toBeInTheDocument()
    expect(screen.getByText('Snapshot epoch 4')).toBeInTheDocument()
    expect(
      screen.getByText(/runtime cache is pending publication/i)
    ).toBeInTheDocument()

    await user.type(modelInput, '-changed')

    expect(screen.queryByText('Runtime cache snapshot')).not.toBeInTheDocument()
    expect(screen.queryByText('Snapshot epoch 4')).not.toBeInTheDocument()
    await client.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
    expect(getChannelRoutingPreview).toHaveBeenCalledTimes(1)
  })

  test('states that list sorting does not control runtime routing', () => {
    renderPreview()

    expect(
      screen.getByText(
        'Channel list sorting is display-only and does not change runtime routing.'
      )
    ).toBeInTheDocument()
  })

  test('keeps every routing preview translation key in every locale', () => {
    const componentPath = path.resolve(
      process.cwd(),
      'src/features/channels/components/channel-routing-preview.tsx'
    )
    const source = readFileSync(componentPath, 'utf8')
    const keyPattern = /\bt\(\s*(['"])(.*?)\1/gms
    const keys = [
      ...new Set([...source.matchAll(keyPattern)].map((match) => match[2])),
    ]

    for (const locale of ['en', 'zh', 'zh-TW', 'fr', 'ja', 'ru', 'vi']) {
      const localePath = path.resolve(
        process.cwd(),
        `src/i18n/locales/${locale}.json`
      )
      const translations = JSON.parse(readFileSync(localePath, 'utf8'))
        .translation as Record<string, string>
      for (const key of keys) {
        expect(translations).toHaveProperty(key)
      }
    }
  })
})
