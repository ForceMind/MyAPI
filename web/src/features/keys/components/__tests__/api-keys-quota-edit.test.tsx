import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { apiKeySchema, type ApiKey } from '../../types'
import { ApiKeysMutateDrawer } from '../api-keys-mutate-drawer'
import { ApiKeysProvider } from '../api-keys-provider'

const originalConfig = useSystemConfigStore.getState().config
const clients: QueryClient[] = []

function makeKey(overrides: Partial<ApiKey> = {}): ApiKey {
  return apiKeySchema.parse({
    id: 7,
    name: 'Accounting key',
    key: 'synthetic-masked',
    status: 1,
    remain_quota: 100,
    used_quota: 50,
    quota_version: '9007199254740993',
    metadata_fingerprint: 'a'.repeat(64),
    unlimited_quota: false,
    expired_time: -1,
    created_time: 0,
    accessed_time: 0,
    group: 'default',
    model_limits_enabled: false,
    model_limits: '',
    allow_ips: '',
    ...overrides,
  })
}

function installReadFixtures(keys: ApiKey[]) {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url.startsWith('/api/token/')) {
      const id = Number(url.slice('/api/token/'.length))
      const key = keys.find((candidate) => candidate.id === id)
      if (!key) throw new Error(`Unexpected synthetic key ${id}`)
      return { data: { success: true, data: key } }
    }
    if (url === '/api/status') {
      return {
        data: { success: true, data: { quota_display_type: 'TOKENS' } },
      }
    }
    if (url === '/api/user/models') {
      return { data: { success: true, data: [] } }
    }
    if (url === '/api/user/self/groups') {
      return { data: { success: true, data: { default: { ratio: 1 } } } }
    }
    if (url === '/api/token/auto-groups') {
      return { data: { success: true, data: { groups: [], max_count: 5 } } }
    }
    throw new Error(`Unexpected synthetic read ${url}`)
  })
}

async function renderDrawer(
  currentRow: ApiKey,
  onOpenChange = vi.fn()
) {
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'TOKENS' },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })

  const view = (open: boolean, row: ApiKey) => (
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <ApiKeysProvider>
          <ApiKeysMutateDrawer
            open={open}
            currentRow={row}
            onOpenChange={onOpenChange}
          />
        </ApiKeysProvider>
      </I18nextProvider>
    </QueryClientProvider>
  )
  const rendered = render(view(true, currentRow))
  await screen.findByDisplayValue(currentRow.name)

  return {
    client,
    rerender: async (open: boolean, row: ApiKey) => {
      rendered.rerender(view(open, row))
      if (open) await screen.findByDisplayValue(row.name)
    },
  }
}

async function changeQuota(value: string) {
  const quota = await screen.findByRole('spinbutton', {
    name: 'Key quota (internal units)',
  })
  fireEvent.change(quota, { target: { value } })
}

async function submit() {
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Save changes' }))
}

function getRequestId(call: readonly unknown[]): unknown {
  const payload = call[1]
  if (!payload || typeof payload !== 'object') return undefined
  return Reflect.get(payload, 'request_id')
}

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useSystemConfigStore.setState({ config: originalConfig })
})

it.each([false, true])(
  'preserves quota when editing metadata; explicit quota change=%s',
  async (changeQuotaValue) => {
    const key = makeKey({
      remain_quota: changeQuotaValue ? 100 : 4294967313,
      used_quota: 4294967321,
      unlimited_quota: !changeQuotaValue,
    })
    installReadFixtures([key])
    const update = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    useSystemConfigStore.getState().setConfig({
      currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'TOKENS' },
    })
    const { client } = await renderDrawer(key)
    fireEvent.change(screen.getByDisplayValue('Accounting key'), {
      target: { value: 'Renamed key' },
    })
    if (changeQuotaValue) {
      act(() =>
        client.setQueryData(['api-key', key.id], {
          success: true,
          data: {
            ...key,
            remain_quota: 75,
            quota_version: '9007199254740994',
            metadata_fingerprint: 'b'.repeat(64),
          },
        })
      )
      await changeQuota('120')
    }
    await submit()
    await waitFor(() => expect(update).toHaveBeenCalledOnce())
    const payload = update.mock.calls[0][1]
    expect(payload).toMatchObject({ id: 7, name: 'Renamed key' })
    if (changeQuotaValue) {
      expect(payload).toMatchObject({
        remain_quota: 120,
        quota_version: '9007199254740993',
        metadata_fingerprint: 'a'.repeat(64),
        request_id: expect.stringMatching(
          /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
        ),
      })
    } else {
      expect(payload).not.toHaveProperty('remain_quota')
      expect(payload).not.toHaveProperty('quota_version')
      expect(payload).not.toHaveProperty('metadata_fingerprint')
      expect(payload).not.toHaveProperty('request_id')
    }
  }
)

it.each(['response', 'network'] as const)(
  'reuses one request ID and the complete payload after a %s failure',
  async (failure) => {
    const key = makeKey()
    installReadFixtures([key])
    const update = vi.spyOn(api, 'put')
    if (failure === 'network') {
      update.mockRejectedValueOnce(new Error('synthetic response loss'))
    } else {
      update.mockResolvedValueOnce({
        data: { success: false, message: 'synthetic rejection' },
      })
    }
    update.mockResolvedValueOnce({ data: { success: true } })
    await renderDrawer(key)
    await changeQuota('120')

    await submit()
    await waitFor(() => expect(update).toHaveBeenCalledTimes(1))
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Save changes' })
      ).toBeEnabled()
    )
    await submit()
    await waitFor(() => expect(update).toHaveBeenCalledTimes(2))

    expect(update.mock.calls[1][1]).toEqual(update.mock.calls[0][1])
  }
)

it('mints a new quota request ID after a metadata-only submission interrupts a failed attempt', async () => {
  const key = makeKey()
  installReadFixtures([key])
  const update = vi
    .spyOn(api, 'put')
    .mockRejectedValueOnce(new Error('synthetic quota response loss'))
    .mockRejectedValueOnce(new Error('synthetic metadata response loss'))
    .mockResolvedValueOnce({ data: { success: true } })
  await renderDrawer(key)

  await changeQuota('120')
  await submit()
  await waitFor(() => expect(update).toHaveBeenCalledTimes(1))
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Save changes' })
    ).toBeEnabled()
  )

  await changeQuota('100')
  fireEvent.change(screen.getByDisplayValue('Accounting key'), {
    target: { value: 'Metadata-only retry boundary' },
  })
  await submit()
  await waitFor(() => expect(update).toHaveBeenCalledTimes(2))
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Save changes' })
    ).toBeEnabled()
  )

  const metadataPayload = update.mock.calls[1][1]
  expect(metadataPayload).not.toHaveProperty('remain_quota')
  expect(metadataPayload).not.toHaveProperty('quota_version')
  expect(metadataPayload).not.toHaveProperty('metadata_fingerprint')
  expect(metadataPayload).not.toHaveProperty('request_id')

  fireEvent.change(screen.getByDisplayValue('Metadata-only retry boundary'), {
    target: { value: 'Accounting key' },
  })
  await changeQuota('120')
  await submit()
  await waitFor(() => expect(update).toHaveBeenCalledTimes(3))

  const firstRequestId = getRequestId(update.mock.calls[0])
  const thirdRequestId = getRequestId(update.mock.calls[2])
  expect(firstRequestId).toEqual(expect.any(String))
  expect(thirdRequestId).toEqual(expect.any(String))
  expect(thirdRequestId).not.toBe(firstRequestId)
})

it.each(['name', 'quota'] as const)(
  'mints a new request ID when the failed submission changes its %s intent',
  async (changedField) => {
    const key = makeKey()
    installReadFixtures([key])
    const update = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: false } })
    await renderDrawer(key)
    await changeQuota('120')
    await submit()
    await waitFor(() => expect(update).toHaveBeenCalledTimes(1))
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Save changes' })
      ).toBeEnabled()
    )

    if (changedField === 'name') {
      fireEvent.change(screen.getByDisplayValue('Accounting key'), {
        target: { value: 'Changed metadata' },
      })
    } else {
      await changeQuota('121')
    }
    await submit()
    await waitFor(() => expect(update).toHaveBeenCalledTimes(2))

    const firstRequestId = getRequestId(update.mock.calls[0])
    const secondRequestId = getRequestId(update.mock.calls[1])
    expect(firstRequestId).toEqual(expect.any(String))
    expect(secondRequestId).toEqual(expect.any(String))
    expect(secondRequestId).not.toBe(firstRequestId)
  }
)

it('clears the failed attempt when the drawer closes and reopens', async () => {
  const key = makeKey()
  installReadFixtures([key])
  const update = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: false } })
  const drawer = await renderDrawer(key)
  await changeQuota('120')
  await submit()
  await waitFor(() => expect(update).toHaveBeenCalledTimes(1))
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Save changes' })
    ).toBeEnabled()
  )

  await drawer.rerender(false, key)
  await drawer.rerender(true, key)
  await changeQuota('120')
  await submit()
  await waitFor(() => expect(update).toHaveBeenCalledTimes(2))

  const firstRequestId = getRequestId(update.mock.calls[0])
  const secondRequestId = getRequestId(update.mock.calls[1])
  expect(firstRequestId).toEqual(expect.any(String))
  expect(secondRequestId).toEqual(expect.any(String))
  expect(secondRequestId).not.toBe(firstRequestId)
})

it('clears the failed attempt and captures a fresh snapshot when switching keys', async () => {
  const first = makeKey()
  const second = makeKey({
    id: 8,
    name: 'Second key',
    remain_quota: 200,
    quota_version: '9007199254740995',
    metadata_fingerprint: 'c'.repeat(64),
  })
  installReadFixtures([first, second])
  const update = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: false } })
  const drawer = await renderDrawer(first)
  await changeQuota('120')
  await submit()
  await waitFor(() => expect(update).toHaveBeenCalledTimes(1))
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Save changes' })
    ).toBeEnabled()
  )

  await drawer.rerender(true, second)
  await changeQuota('220')
  await submit()
  await waitFor(() => expect(update).toHaveBeenCalledTimes(2))

  expect(update.mock.calls[1][1]).toMatchObject({
    id: 8,
    remain_quota: 220,
    quota_version: '9007199254740995',
    metadata_fingerprint: 'c'.repeat(64),
  })
  const firstRequestId = getRequestId(update.mock.calls[0])
  const secondRequestId = getRequestId(update.mock.calls[1])
  expect(firstRequestId).toEqual(expect.any(String))
  expect(secondRequestId).toEqual(expect.any(String))
  expect(secondRequestId).not.toBe(firstRequestId)
})

it('clears the attempt after success before another identical submission', async () => {
  const key = makeKey()
  installReadFixtures([key])
  const update = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  await renderDrawer(key)
  await changeQuota('120')
  await submit()
  await waitFor(() => expect(update).toHaveBeenCalledTimes(1))
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Save changes' })
    ).toBeEnabled()
  )
  await submit()
  await waitFor(() => expect(update).toHaveBeenCalledTimes(2))

  const firstRequestId = getRequestId(update.mock.calls[0])
  const secondRequestId = getRequestId(update.mock.calls[1])
  expect(firstRequestId).toEqual(expect.any(String))
  expect(secondRequestId).toEqual(expect.any(String))
  expect(secondRequestId).not.toBe(firstRequestId)
})
