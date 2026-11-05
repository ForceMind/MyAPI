/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { QueryClient } from '@tanstack/react-query'
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import dayjs from '@/lib/dayjs'
import { useAuthStore } from '@/stores/auth-store'
import { useInspectorStore } from '@/stores/inspector-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import {
  KEY_21,
  mockKeysApi,
  newClient,
  renderKeysPage,
  setViewport,
  signIn,
} from './keys-page-fixture'

const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
let client: QueryClient

beforeEach(() => {
  localStorage.clear()
  client = newClient()
  signIn(100)
  setViewport(1440)
})

afterEach(() => {
  client.clear()
  useAuthStore.setState(originalAuth)
  useSystemConfigStore.setState(originalConfig)
  useInspectorStore.setState({ current: null })
  vi.restoreAllMocks()
  localStorage.clear()
})

test.each([1440, 390])(
  'at %ipx scope separates access from budgets and shares, with one identity and date set',
  async (width) => {
    setViewport(width)
    const longProfile = 'Assigned access profile '.repeat(8).trim()
    mockKeysApi({
      keys: [{ ...KEY_21, access_profile: { id: 'assigned', label: longProfile } }],
      logs: [],
      status: {},
    })
    await renderKeysPage(client, '/keys/21?tab=scope')
    const access = await screen.findByRole('region', { name: 'Access' })
    const budgets = screen.getByRole('region', { name: 'Budget and shares' })
    expect(access.parentElement).toBe(budgets.parentElement)
    // One column by default, two at desktop width; long values can wrap.
    expect(access.parentElement).toHaveClass('grid-cols-1', 'lg:grid-cols-2')
    expect(within(access).getByText(longProfile)).toHaveClass('wrap-anywhere')
    expect(within(access).getByText('Models')).toBeVisible()
    expect(within(access).getByText('IP Restriction')).toBeVisible()
    expect(within(access).queryByText('Quota')).not.toBeInTheDocument()
    expect(within(budgets).getByText('Quota')).toBeVisible()
    expect(
      within(budgets).getByRole('button', { name: 'Usage budgets' })
    ).toBeVisible()
    expect(
      within(budgets).getByRole('heading', { name: 'Window shares' })
    ).toBeVisible()
    expect(screen.getAllByText(/^Created:/)).toHaveLength(1)
    expect(screen.getAllByText(/^Expires:/)).toHaveLength(1)
    expect(screen.getAllByText(/^Last used:/)).toHaveLength(1)
    expect(screen.queryByText('sk-masked-value')).not.toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Scope and shares' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  }
)

test('personal scope hides financial figures while keeping the budget policy and share editor', async () => {
  mockKeysApi({
    keys: [KEY_21],
    logs: [],
    status: {
      user_funding_mode: 'disabled',
      user_funding_capabilities: { mode: 'disabled', epoch: 1, ready: true },
    },
  })
  await renderKeysPage(client, '/keys/21?tab=scope')
  const budgets = await screen.findByRole('region', { name: 'Budget and shares' })
  await waitFor(() =>
    expect(within(budgets).queryByText('Quota')).not.toBeInTheDocument()
  )
  expect(screen.queryByText('Remaining quota')).not.toBeInTheDocument()
  expect(
    within(budgets).getByRole('button', { name: 'Usage budgets' })
  ).toBeVisible()
  expect(
    within(budgets).getByRole('button', { name: 'Window share' })
  ).toBeVisible()
  expect(screen.getAllByText(/^Last used:/)).toHaveLength(1)
  await screen.findByText('No subscription share is configured for this Key.')
  const shareLabel = await screen.findByText('Window shares', { selector: 'dt' })
  const share = shareLabel.parentElement as HTMLElement
  expect(within(share).getByText('None')).toBeVisible()
})

test('an unreadable personal share is unknown instead of an empty allocation', async () => {
  mockKeysApi({
    keys: [KEY_21],
    logs: [],
    status: {
      user_funding_mode: 'disabled',
      user_funding_capabilities: { mode: 'disabled', epoch: 1, ready: true },
    },
    extraGet: (path) =>
      path === '/api/token/21/subscription-share'
        ? { data: { success: false } }
        : undefined,
  })
  await renderKeysPage(client, '/keys/21?tab=scope')
  await screen.findByText('Could not load subscription shares.')
  const shareLabel = await screen.findByText('Window share', { selector: 'dt' })
  const share = shareLabel.parentElement as HTMLElement
  expect(within(share).getByText('Unknown')).toBeVisible()
  expect(within(share).queryByText('None')).not.toBeInTheDocument()
})

test.each([
  [
    'https://gateway.synthetic.invalid/root///',
    'https://gateway.synthetic.invalid/root/v1',
    'From the server address setting',
  ],
  ['', `${window.location.origin}/v1`, 'Current access address'],
])(
  'delivery groups %s and its source with the masked Key without reading a credential',
  async (serverAddress, baseUrl, source) => {
    const mocks = mockKeysApi({
      keys: [KEY_21],
      logs: [],
      status: { server_address: serverAddress },
    })
    await renderKeysPage(client, '/keys/21?tab=delivery')
    const connection = await screen.findByRole('region', { name: 'Connection details' })
    expect(await within(connection).findByText(baseUrl)).toBeVisible()
    expect(within(connection).getByText(source)).toBeVisible()
    expect(
      within(connection).getByRole('button', { name: 'Copy Base URL' })
    ).toBeVisible()
    expect(
      within(connection).getByRole('button', { name: 'sk-masked-value' })
    ).toBeVisible()
    expect(screen.getAllByText('sk-masked-value')).toHaveLength(1)
    const maintenance = screen.getByRole('region', { name: 'Credential maintenance' })
    expect(
      within(maintenance).getByRole('button', { name: 'Credential maintenance' })
    ).toBeVisible()
    expect(
      within(connection).queryByRole('button', { name: 'Credential maintenance' })
    ).not.toBeInTheDocument()
    expect(mocks.post).not.toHaveBeenCalled()
    expect(mocks.put).not.toHaveBeenCalled()
  }
)

test('keyboard activation of the delivery mask reads the full Key only after that action', async () => {
  const user = userEvent.setup()
  const mocks = mockKeysApi({ keys: [KEY_21], logs: [], status: {} })
  await renderKeysPage(client, '/keys/21?tab=delivery')
  const reveal = await screen.findByRole('button', { name: 'sk-masked-value' })
  expect(mocks.post).not.toHaveBeenCalled()
  act(() => reveal.focus())
  await user.keyboard('{Enter}')
  const fullKey = await screen.findByRole('textbox')
  await waitFor(() => expect(fullKey).toHaveValue('sk-inert-existing-key'))
  expect(mocks.post).toHaveBeenCalledWith(
    '/api/token/21/key',
    undefined,
    expect.objectContaining({ skipAuthRefresh: true })
  )
  expect(mocks.put).not.toHaveBeenCalled()
})

test.each([1, 10])(
  'role %i sees delivery but no Root credential maintenance region',
  async (role) => {
    signIn(role)
    mockKeysApi({ keys: [KEY_21], logs: [], status: {} })
    await renderKeysPage(client, '/keys/21?tab=delivery')
    expect(
      await screen.findByRole('region', { name: 'Connection details' })
    ).toBeVisible()
    expect(
      screen.queryByRole('region', { name: 'Credential maintenance' })
    ).not.toBeInTheDocument()
  }
)

test('a saved share without a baseline keeps its unknown estimate beside its configured percentage', async () => {
  mockKeysApi({
    keys: [KEY_21],
    logs: [],
    status: {},
    extraGet: (path) =>
      path === '/api/token/21/subscription-share'
        ? {
            data: {
              success: true,
              data: {
                token_id: 21,
                pools: [
                  {
                    pool_id: 7,
                    snapshot_id: 70,
                    revision: 1,
                    channel_name: 'Synthetic window',
                    plan_type: 'plus',
                    window_slot: 'primary',
                    window_type: 'five_hour',
                    window_seconds: 18000,
                    reset_at: 1797003600,
                    observed_at: 1797000000,
                    max_age_seconds: 300,
                    sample_age_seconds: 30,
                    state: 'active',
                    allocated_bps: 7000,
                    share_bps: 2000,
                    estimated_used_micro: 0,
                    unattributed_micro: 0,
                    pending_attempts: 1,
                    unsampled_attempts: 0,
                    approximate: true,
                    baseline_time: 0,
                  },
                ],
              },
            },
          }
        : undefined,
  })
  await renderKeysPage(client, '/keys/21?tab=scope')
  const pool = await screen.findByRole('region', { name: /Synthetic window/ })
  await userEvent.click(within(pool).getByRole('button', { name: 'Observation details' }))
  expect(within(pool).getByText('20%')).toBeVisible()
  expect(within(pool).getByText('Unknown')).toBeVisible()
  expect(within(pool).queryByText('0%')).not.toBeInTheDocument()
  expect(
    within(pool).getByText('Usage is still awaiting attribution.')
  ).toBeVisible()
})

test('a failed detail refresh removes the previous identity and delivery fields', async () => {
  const mocks = mockKeysApi({ keys: [KEY_21], logs: [], status: {} })
  await renderKeysPage(client, '/keys/21?tab=delivery')
  await screen.findByRole('region', { name: 'Connection details' })
  const original = mocks.get.getMockImplementation()
  if (!original) throw new Error('fixture has no GET handler')
  mocks.get.mockImplementation(async (url, config) => {
    if (String(url) === '/api/token/21') throw new Error('unavailable')
    return original(url, config)
  })
  await act(async () => {
    await client.invalidateQueries({ queryKey: ['api-key'] })
  })
  expect(
    await screen.findByText('This Key is unavailable or belongs to another account.')
  ).toBeVisible()
  expect(
    screen.queryByRole('region', { name: 'Connection details' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('heading', { name: 'Desktop application' })
  ).not.toBeInTheDocument()
  expect(screen.queryByText('sk-masked-value')).not.toBeInTheDocument()
})

test('Delivery keeps one credential copy action while the other tabs keep their header shortcut', async () => {
  const mocks = mockKeysApi({ keys: [KEY_21], logs: [], status: {} })
  const user = userEvent.setup()
  await renderKeysPage(client, '/keys/21?tab=delivery')
  const delivery = await screen.findByRole('region', { name: 'Connection details' })
  expect(screen.queryByRole('button', { name: 'Copy Key' })).not.toBeInTheDocument()
  const copy = within(delivery).getByRole('button', { name: 'Copy API key' })
  expect(copy).toBeVisible()
  expect(mocks.post).not.toHaveBeenCalled()
  const clipboard = vi.spyOn(navigator.clipboard, 'writeText')
  await user.click(copy)
  await waitFor(() => expect(clipboard).toHaveBeenCalledWith('sk-inert-existing-key'))
  expect(mocks.post).toHaveBeenCalledExactlyOnceWith('/api/token/21/key', undefined, expect.objectContaining({ skipAuthRefresh: true }))
  await user.click(screen.getByRole('tab', { name: 'Requests' }))
  expect(await screen.findByRole('button', { name: 'Copy Key' })).toBeVisible()
})

test('a long credential at 320px stays shrinkable, completely selected and copyable only after an explicit reveal', async () => {
  setViewport(320)
  const mocks = mockKeysApi({ keys: [KEY_21], logs: [], status: {} })
  const fullKey = `sk-${'synthetic-long-secret-'.repeat(25)}`
  mocks.post.mockResolvedValue({ data: { success: true, data: { key: fullKey.slice(3) } } })
  const user = userEvent.setup()
  await renderKeysPage(client, '/keys/21?tab=delivery')
  const reveal = await screen.findByRole('button', { name: 'sk-masked-value' })
  expect(mocks.post).not.toHaveBeenCalled()
  await user.click(reveal)
  const input = await screen.findByRole('textbox', { name: 'Full API Key' })
  await waitFor(() => expect(input).toHaveValue(fullKey))
  expect(input).toHaveClass('min-w-0', 'max-w-full')
  expect(input.closest('[data-slot="popover-content"]')).toHaveClass('min-w-0', 'max-w-[min(90vw,28rem)]')
  await waitFor(() => expect(input).toHaveFocus())
  expect((input as HTMLInputElement).selectionStart).toBe(0)
  expect((input as HTMLInputElement).selectionEnd).toBe(fullKey.length)
  const clipboard = vi.spyOn(navigator.clipboard, 'writeText')
  await user.click(screen.getByRole('button', { name: 'Copy API key' }))
  expect(clipboard).toHaveBeenCalledWith(fullKey)
  expect(mocks.post).toHaveBeenCalledOnce()
})

test.each([1440, 390])('viewing an unknown expiry at %ipx preserves the list filters, page and origin focus on return', async (width) => {
  setViewport(width)
  signIn(1)
  const mocks = mockKeysApi({ keys: [{ ...KEY_21, expired_time: Number.MAX_SAFE_INTEGER }], total: 100, logs: [], status: {} })
  const user = userEvent.setup()
  const { router } = await renderKeysPage(client, '/keys?page=3&pageSize=20&filter=saved&status=%5B%221%22%5D')
  const opener = await screen.findByRole('button', { name: /Desktop application/ })
  await user.click(opener)
  expect(await screen.findByText('Expires: Unknown')).toBeVisible()
  expect(router.state.location.search).toMatchObject({ keyReturn: { page: 3, pageSize: 20, filter: 'saved', status: ['1'] } })
  await act(async () => useInspectorStore.getState().current?.close())
  await waitFor(() => expect(router.state.location.pathname).toBe('/keys'))
  expect(router.state.location.search).toMatchObject({ page: 3, pageSize: 20, filter: 'saved', status: ['1'] })
  const returned = await screen.findByRole('button', { name: /Desktop application/ })
  await waitFor(() => expect(returned).toHaveFocus())
  expect(mocks.post).not.toHaveBeenCalled()
  expect(mocks.put).not.toHaveBeenCalled()
})


test('an explicit epoch expiry displays its full date on the Key page without changing unused timestamps or reading a credential', async () => {
  const mocks = mockKeysApi({ keys: [{ ...KEY_21, expired_time: 0, accessed_time: 0 }], logs: [], status: {} })
  await renderKeysPage(client, '/keys/21')
  expect(await screen.findByText(`Expires: ${dayjs(0).format('YYYY-MM-DD HH:mm:ss')}`)).toBeVisible()
  expect(screen.getByText('Last used: Never')).toBeVisible()
  expect(mocks.post).not.toHaveBeenCalled()
  expect(mocks.put).not.toHaveBeenCalled()
})
