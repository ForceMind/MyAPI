/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AxiosError } from 'axios'
import { createInstance } from 'i18next'
import { initReactI18next, I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { ChannelRoutingData } from '../types'

vi.mock('../api', () => ({
  getChannelRouting: vi.fn(),
  previewChannelRouting: vi.fn(),
  updateChannelRoutingPolicy: vi.fn(),
  updateRoutingChannel: vi.fn(),
}))

const { ChannelRoutingDialog, ChannelRoutingPanel } =
  await import('../channel-routing-dialog')
const {
  getChannelRouting,
  previewChannelRouting,
  updateChannelRoutingPolicy,
  updateRoutingChannel,
} = await import('../api')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const routingData: ChannelRoutingData = {
  policy: {
    enabled: true,
    sticky_enabled: true,
    session_ttl_seconds: 7200,
    quota_max_age_seconds: 600,
  },
  channels: [
    {
      id: 9,
      name: 'Primary',
      type: 1,
      status: 1,
      priority: 10,
      weight: 4,
      quota: { state: 'fresh', available: 42, unit: 'requests' },
    },
  ],
}

function setUser(role: number, canWriteChannels = false): void {
  useAuthStore.getState().auth.setBundle({
    access_token: 'test-token',
    token_type: 'Bearer',
    access_expires_at: Date.now() + 60_000,
    user: {
      id: 4,
      username: 'routing-test',
      role,
      permissions: canWriteChannels
        ? { admin_permissions: { channel: { write: true } } }
        : undefined,
    },
    session: {
      sid: 'routing-session',
      current: true,
      login_method: 'password',
      ip: '',
      user_agent: '',
      created_at: 1,
      last_active_at: 1,
      expires_at: Date.now() + 60_000,
    },
  })
}

function renderDialog(): void {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <ChannelRoutingDialog />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

async function openDialog(): Promise<void> {
  fireEvent.click(screen.getByRole('button', { name: 'Traffic Allocation' }))
  await screen.findByRole('heading', { name: 'Policy' })
}

describe('ChannelRoutingDialog', () => {
  beforeEach(() => {
    vi.mocked(getChannelRouting).mockResolvedValue({
      success: true,
      data: routingData,
    })
    vi.mocked(updateChannelRoutingPolicy).mockResolvedValue({ success: true })
    vi.mocked(updateRoutingChannel).mockResolvedValue({ success: true })
    vi.mocked(previewChannelRouting).mockResolvedValue({
      success: true,
      data: {
        workload: 'chat',
        reason: 'chat_lower_remaining_quota',
        candidates: [
          {
            id: 9,
            name: 'Primary',
            share: 1,
            reason: 'priority_weight',
          },
        ],
      },
    })
  })

  test('keeps an existing legacy order until an administrator chooses a preset', async () => {
    vi.mocked(getChannelRouting).mockResolvedValue({
      success: true,
      data: {
        ...routingData,
        channels: [
          {
            ...routingData.channels[0],
            priority: 1000000,
            legacy_priority: '9223372036854775807',
            weight: 1000000,
            legacy_weight: '1000042',
          },
        ],
      },
    })
    setUser(ROLE.SUPER_ADMIN)
    renderDialog()
    await openDialog()
    expect(
      screen.getByRole('combobox', { name: 'Routing order Primary' })
    ).toHaveValue('existing')
    expect(
      screen.getByText(
        'Existing order: 9223372036854775807. Choose a routing order to replace it.'
      )
    ).toBeInTheDocument()
    fireEvent.change(
      screen.getByRole('combobox', { name: 'Routing order Primary' }),
      { target: { value: '10' } }
    )
    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'Traffic share Primary' }),
      { target: { value: '6' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(updateRoutingChannel).toHaveBeenCalledWith(9, {
        priority: 10,
        weight: 6,
        expected_priority: 1000000,
        expected_weight: 1000000,
        expected_legacy_weight: '1000042',
        expected_legacy_priority: '9223372036854775807',
      })
    )
  })

  afterEach(() => {
    vi.clearAllMocks()
    useAuthStore.getState().auth.reset()
  })

  test('root administrator saves the four-field policy after changing its enabled state', async () => {
    setUser(ROLE.SUPER_ADMIN)
    renderDialog()
    await openDialog()

    fireEvent.click(
      screen.getByRole('switch', { name: 'Enable intelligent allocation' })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save policy' }))

    await waitFor(() =>
      expect(updateChannelRoutingPolicy).toHaveBeenCalledWith({
        enabled: false,
        sticky_enabled: true,
        session_ttl_seconds: 7200,
        quota_max_age_seconds: 600,
      })
    )
  })

  test('administrator can save a routing order and traffic share while policy controls stay root-only', async () => {
    setUser(ROLE.ADMIN, true)
    renderDialog()
    await openDialog()

    expect(
      screen.queryByRole('button', { name: 'Save policy' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Preferred' })).toHaveValue('10')
    expect(screen.getByRole('option', { name: 'Standard' })).toHaveValue('0')
    expect(screen.getByRole('option', { name: 'Backup' })).toHaveValue('-10')
    fireEvent.change(
      screen.getByRole('combobox', { name: 'Routing order Primary' }),
      {
        target: { value: '-10' },
      }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() =>
      expect(updateRoutingChannel).toHaveBeenCalledWith(9, {
        priority: -10,
        weight: 4,
        expected_priority: 10,
        expected_weight: 4,
      })
    )
  })

  test('keeps an edited policy visible when saving fails', async () => {
    vi.mocked(updateChannelRoutingPolicy).mockRejectedValueOnce(
      new Error('policy service unavailable')
    )
    setUser(ROLE.SUPER_ADMIN)
    renderDialog()
    await openDialog()

    fireEvent.click(
      screen.getByRole('switch', { name: 'Enable intelligent allocation' })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save policy' }))

    expect(
      await screen.findByText('policy service unavailable')
    ).toBeInTheDocument()
    expect(
      screen.getByRole('switch', { name: 'Enable intelligent allocation' })
    ).not.toBeChecked()
  })

  test('renders the panel without a dialog trigger and shows same-order traffic shares', async () => {
    vi.mocked(getChannelRouting).mockResolvedValueOnce({
      success: true,
      data: {
        ...routingData,
        channels: [
          { ...routingData.channels[0], weight: 3 },
          {
            id: 10,
            name: 'Secondary',
            type: 1,
            status: 1,
            priority: 10,
            weight: 1,
            quota: { state: 'fresh' },
          },
        ],
      },
    })
    setUser(ROLE.ADMIN, true)
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <ChannelRoutingPanel />
        </I18nextProvider>
      </QueryClientProvider>
    )

    await screen.findByRole('heading', { name: 'Channel allocation' })
    expect(
      screen.queryByRole('button', { name: 'Traffic Allocation' })
    ).not.toBeInTheDocument()
    expect(screen.getByText('Traffic share: 75%')).toBeInTheDocument()
    expect(screen.getByText('Traffic share: 25%')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Choose a routing order and set each channel’s traffic share. Channel status is managed in Channel Management.'
      )
    ).toBeInTheDocument()
  })

  test('blocks a newly entered zero traffic share', async () => {
    setUser(ROLE.ADMIN, true)
    renderDialog()
    await openDialog()

    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'Traffic share Primary' }),
      { target: { value: '0' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(
      await screen.findByText(
        'Choose a routing order and a positive traffic share within the displayed limits.'
      )
    ).toBeInTheDocument()
    expect(updateRoutingChannel).not.toHaveBeenCalled()
  })

  test('uses the selected preset path and supplied model and group for a routing preview', async () => {
    setUser(ROLE.ADMIN)
    renderDialog()
    await openDialog()

    fireEvent.change(screen.getByLabelText('Request type'), {
      target: { value: '/v1/responses' },
    })
    fireEvent.change(screen.getByLabelText('Model'), {
      target: { value: 'gpt-5' },
    })
    fireEvent.change(screen.getByLabelText('Group'), {
      target: { value: 'vip' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Preview routing' }))

    await waitFor(() =>
      expect(previewChannelRouting).toHaveBeenCalledWith({
        path: '/v1/responses',
        model: 'gpt-5',
        group: 'vip',
      })
    )
    expect(
      await screen.findByText(
        'Chat routing favors lower comparable remaining quota.'
      )
    ).toBeInTheDocument()
    expect(
      await screen.findByText(/Selected by routing order and traffic share/)
    ).toBeInTheDocument()
  })

  test('marks a saved-configuration preview stale when channel edits become a draft', async () => {
    setUser(ROLE.ADMIN, true)
    renderDialog()
    await openDialog()

    fireEvent.click(screen.getByRole('button', { name: 'Preview routing' }))
    await screen.findByText(
      'Chat routing favors lower comparable remaining quota.'
    )
    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'Traffic share Primary' }),
      { target: { value: '6' } }
    )

    expect(screen.getByText('Unsaved changes')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Configuration changed after this preview. Preview again to use the saved configuration.'
      )
    ).toBeInTheDocument()
  })

  test.each([
    [0.004, /0\.40%/],
    [0.000001, /<0\.01%/],
  ])(
    'keeps positive preview share %s and percent quota visible',
    async (share, label) => {
      vi.mocked(previewChannelRouting).mockResolvedValueOnce({
        success: true,
        data: {
          workload: 'work',
          reason: 'work_higher_remaining_quota',
          candidates: [
            {
              id: 9,
              name: 'Primary',
              share,
              reason: 'quota_higher_pool',
              available: 25,
              unit: 'percent',
            },
          ],
        },
      })
      setUser(ROLE.ADMIN)
      renderDialog()
      await openDialog()

      fireEvent.click(screen.getByRole('button', { name: 'Preview routing' }))

      expect(await screen.findByText(label)).toBeInTheDocument()
      expect(screen.getByText(/25%/)).toBeInTheDocument()
      expect(screen.getAllByText('Work')[1]).toBeInTheDocument()
    }
  )

  test.each([true, false])(
    'rebases exact legacy tokens after a conflict (fresh legacy: %s)',
    async (freshLegacy) => {
      const legacy = {
        priority: 1000000,
        weight: 1000000,
        legacy_priority: '1000042',
        legacy_weight: '1000042',
      }
      const fresh = freshLegacy
        ? { ...legacy, legacy_priority: '1000043', legacy_weight: '1000043' }
        : { priority: 12, weight: 4 }
      vi.mocked(getChannelRouting)
        .mockResolvedValueOnce({
          success: true,
          data: {
            ...routingData,
            channels: [{ ...routingData.channels[0], ...legacy }],
          },
        })
        .mockResolvedValue({
          success: true,
          data: {
            ...routingData,
            channels: [{ ...routingData.channels[0], ...fresh }],
          },
        })
      const error = new AxiosError(
        'Request failed with status code 409',
        undefined,
        undefined,
        undefined,
        {
          status: 409,
          statusText: 'Conflict',
          data: {
            code: 'channel_routing_conflict',
            message: 'Channel changed',
          },
          headers: {},
          config: {} as never,
        }
      )
      vi.mocked(updateRoutingChannel).mockRejectedValueOnce(error)
      setUser(ROLE.ADMIN, true)
      renderDialog()
      await openDialog()
      fireEvent.change(
        screen.getByRole('combobox', { name: 'Routing order Primary' }),
        { target: { value: '10' } }
      )

      fireEvent.change(
        screen.getByRole('spinbutton', { name: 'Traffic share Primary' }),
        {
          target: { value: '6' },
        }
      )
      fireEvent.click(screen.getByRole('button', { name: 'Save' }))

      expect(
        await screen.findByText(
          'This channel changed elsewhere. Current values were reloaded; your edits are still shown.'
        )
      ).toBeInTheDocument()
      expect(
        screen.getByRole('spinbutton', { name: 'Traffic share Primary' })
      ).toHaveValue(6)
      await waitFor(() => expect(getChannelRouting).toHaveBeenCalledTimes(2))
      fireEvent.click(screen.getByRole('button', { name: 'Save' }))
      await waitFor(() =>
        expect(updateRoutingChannel).toHaveBeenLastCalledWith(9, {
          priority: 10,
          weight: 6,
          expected_priority: fresh.priority,
          expected_weight: fresh.weight,
          ...(freshLegacy
            ? {
                expected_legacy_priority: '1000043',
                expected_legacy_weight: '1000043',
              }
            : {}),
        })
      )
    }
  )

  test('uses refreshed channel values as the baseline after reopening without an edit', async () => {
    vi.mocked(getChannelRouting)
      .mockResolvedValueOnce({ success: true, data: routingData })
      .mockResolvedValueOnce({
        success: true,
        data: {
          ...routingData,
          channels: [{ ...routingData.channels[0], priority: 20 }],
        },
      })
    setUser(ROLE.ADMIN, true)
    renderDialog()
    await openDialog()

    fireEvent.click(screen.getAllByRole('button', { name: 'Close' })[0])
    fireEvent.click(screen.getByRole('button', { name: 'Traffic Allocation' }))

    await waitFor(() =>
      expect(
        screen.getByRole('combobox', { name: 'Routing order Primary' })
      ).toHaveValue('existing')
    )
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  })

  test('keeps a dirty channel baseline when a refetch returns another administrator update', async () => {
    vi.mocked(getChannelRouting)
      .mockResolvedValueOnce({ success: true, data: routingData })
      .mockResolvedValueOnce({
        success: true,
        data: {
          ...routingData,
          channels: [{ ...routingData.channels[0], priority: 20 }],
        },
      })
    setUser(ROLE.ADMIN, true)
    renderDialog()
    await openDialog()

    fireEvent.change(
      screen.getByRole('combobox', { name: 'Routing order Primary' }),
      {
        target: { value: '-10' },
      }
    )
    fireEvent.click(screen.getAllByRole('button', { name: 'Close' })[0])
    fireEvent.click(screen.getByRole('button', { name: 'Traffic Allocation' }))
    await waitFor(() => expect(getChannelRouting).toHaveBeenCalledTimes(2))
    expect(
      screen.getByRole('combobox', { name: 'Routing order Primary' })
    ).toHaveValue('-10')

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(updateRoutingChannel).toHaveBeenCalledWith(9, {
        priority: -10,
        weight: 4,
        expected_priority: 10,
        expected_weight: 4,
      })
    )
  })

  test('keeps a root policy edit after saving a channel refetches routing data', async () => {
    setUser(ROLE.SUPER_ADMIN)
    renderDialog()
    await openDialog()

    fireEvent.click(
      screen.getByRole('switch', { name: 'Enable intelligent allocation' })
    )
    fireEvent.change(
      screen.getByRole('combobox', { name: 'Routing order Primary' }),
      {
        target: { value: '0' },
      }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(updateRoutingChannel).toHaveBeenCalled())
    expect(
      screen.getByRole('switch', { name: 'Enable intelligent allocation' })
    ).not.toBeChecked()
  })

  test('does not expose channel allocation controls to an administrator without channel write permission', async () => {
    setUser(ROLE.ADMIN)
    renderDialog()
    await openDialog()

    expect(
      screen.getByRole('combobox', { name: 'Routing order Primary' })
    ).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  })
  test('disabled channels do not dilute configured traffic shares', async () => {
    vi.mocked(getChannelRouting).mockResolvedValue({
      success: true,
      data: {
        ...routingData,
        channels: [
          routingData.channels[0],
          {
            ...routingData.channels[0],
            id: 10,
            name: 'Disabled peer',
            status: 2,
          },
        ],
      },
    })
    setUser(ROLE.ADMIN, true)
    renderDialog()
    await openDialog()
    expect(screen.getByText('Traffic share: 100%')).toBeInTheDocument()
    expect(screen.queryByText('Traffic share: 50%')).not.toBeInTheDocument()
  })

  test('changing order on an old zero-share channel requires a positive share', async () => {
    vi.mocked(getChannelRouting).mockResolvedValue({
      success: true,
      data: {
        ...routingData,
        channels: [{ ...routingData.channels[0], weight: 0 }],
      },
    })
    setUser(ROLE.ADMIN, true)
    renderDialog()
    await openDialog()
    fireEvent.change(
      screen.getByRole('combobox', { name: 'Routing order Primary' }),
      { target: { value: '-10' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(updateRoutingChannel).not.toHaveBeenCalled()
    expect(
      await screen.findByText(
        'Choose a routing order and a positive traffic share within the displayed limits.'
      )
    ).toBeInTheDocument()
  })

  test('editing a legacy channel share alone does not silently normalize its order', async () => {
    vi.mocked(getChannelRouting).mockResolvedValue({
      success: true,
      data: {
        ...routingData,
        channels: [
          {
            ...routingData.channels[0],
            priority: 1000000,
            legacy_priority: '9223372036854775807',
          },
        ],
      },
    })
    setUser(ROLE.ADMIN, true)
    renderDialog()
    await openDialog()
    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'Traffic share Primary' }),
      { target: { value: '8' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(updateRoutingChannel).not.toHaveBeenCalled()
  })
  test('initially loaded configuration is not reported as a successful save', async () => {
    setUser(ROLE.SUPER_ADMIN)
    renderDialog()
    await openDialog()
    expect(screen.queryByText('Saved successfully')).not.toBeInTheDocument()
    expect(screen.getAllByText('No changes').length).toBeGreaterThan(0)
  })

  test.each([
    ['Model', 'changed-model'],
    ['Group', 'changed-group'],
    ['Request type', '/v1/responses'],
  ])(
    'changing preview %s invalidates the previous result',
    async (label, value) => {
      setUser(ROLE.ADMIN)
      renderDialog()
      await openDialog()
      fireEvent.click(screen.getByRole('button', { name: 'Preview routing' }))
      await screen.findByText(
        'Chat routing favors lower comparable remaining quota.'
      )
      fireEvent.change(screen.getByLabelText(label), { target: { value } })
      expect(
        screen.getByText(
          'Configuration changed after this preview. Preview again to use the saved configuration.'
        )
      ).toBeInTheDocument()
    }
  )
})
