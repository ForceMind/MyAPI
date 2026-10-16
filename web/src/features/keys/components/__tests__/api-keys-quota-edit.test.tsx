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

function installReadFixtures(
  keys: ApiKey[],
  readKey: (key: ApiKey, readCount: number) => ApiKey | Promise<ApiKey> = (
    key
  ) => key
) {
  let detailReadCount = 0
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url.startsWith('/api/token/')) {
      const id = Number(url.slice('/api/token/'.length))
      const key = keys.find((candidate) => candidate.id === id)
      if (!key) throw new Error(`Unexpected synthetic key ${id}`)
      const data = await readKey(key, ++detailReadCount)
      return { data: { success: true, data } }
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

async function waitForDrawerReady(currentRow: ApiKey) {
  await screen.findByDisplayValue(currentRow.name)
  await waitFor(() => {
    expect(document.getElementById('api-key-form')).toHaveAttribute(
      'aria-busy',
      'false'
    )
    expect(document.getElementById('api-key-form')).not.toHaveAttribute('inert')
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled()
  })
}

async function renderDrawer(currentRow: ApiKey, onOpenChange = vi.fn()) {
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
  await waitForDrawerReady(currentRow)

  return {
    client,
    rerender: async (
      open: boolean,
      row: ApiKey,
      { waitUntilReady = true }: { waitUntilReady?: boolean } = {}
    ) => {
      rendered.rerender(view(open, row))
      if (open && waitUntilReady) await waitForDrawerReady(row)
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

it('omits model scope from ordinary metadata edits so deny-all cannot be disabled', async () => {
  const key = makeKey({
    model_limits_enabled: true,
    model_limits: '',
    unlimited_quota: true,
  })
  installReadFixtures([key])
  const update = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  await renderDrawer(key)
  fireEvent.change(screen.getByDisplayValue('Accounting key'), {
    target: { value: 'Renamed deny-all key' },
  })
  await submit()
  await waitFor(() => expect(update).toHaveBeenCalledOnce())
  const payload = update.mock.calls[0][1]
  expect(payload).toMatchObject({ id: 7, name: 'Renamed deny-all key' })
  expect(payload).not.toHaveProperty('model_limits_enabled')
  expect(payload).not.toHaveProperty('model_limits')
})

it('preserves the opened raw deny-all scope in a quota CAS payload', async () => {
  const key = makeKey({
    model_limits_enabled: true,
    model_limits: '',
  })
  installReadFixtures([key])
  const update = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  await renderDrawer(key)
  await changeQuota('120')
  await submit()
  await waitFor(() => expect(update).toHaveBeenCalledOnce())
  expect(update.mock.calls[0][1]).toMatchObject({
    id: 7,
    remain_quota: 120,
    model_limits_enabled: true,
    model_limits: '',
    metadata_fingerprint: 'a'.repeat(64),
  })
})

it.each([
  {
    name: 'unlimited',
    scope: { model_limits_enabled: false, model_limits: '' },
    expectedMode: 'No additional model limit',
    expectedModels: null,
  },
  {
    name: 'deny all',
    scope: { model_limits_enabled: true, model_limits: '' },
    expectedMode: 'Block all models',
    expectedModels: null,
  },
  {
    name: 'selected models',
    scope: {
      model_limits_enabled: true,
      model_limits: 'retired/very-long-model,retired/very-long-model',
    },
    expectedMode: 'Only selected models',
    expectedModels: 'retired/very-long-model,retired/very-long-model',
  },
])(
  'shows the complete $name model scope as read-only in the generic editor',
  async ({ scope, expectedMode, expectedModels }) => {
    const key = makeKey(scope)
    installReadFixtures([key])
    await renderDrawer(key)

    await userEvent
      .setup()
      .click(screen.getByRole('button', { name: /Advanced Settings/ }))

    expect(screen.getByText(expectedMode)).toBeVisible()
    expect(
      screen.getByText(
        'Close this editor, then use Model scope from the Key actions to change this setting.'
      )
    ).toBeVisible()
    if (expectedModels) {
      expect(screen.getByText(expectedModels)).toBeVisible()
    } else {
      expect(screen.queryByText('Stored model IDs')).not.toBeInTheDocument()
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
      expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled()
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
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled()
  )

  await changeQuota('100')
  fireEvent.change(screen.getByDisplayValue('Accounting key'), {
    target: { value: 'Metadata-only retry boundary' },
  })
  await submit()
  await waitFor(() => expect(update).toHaveBeenCalledTimes(2))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled()
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
      expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled()
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
  let resolveReopenedDetail!: (key: ApiKey) => void
  let reopenedDetailRequested = false
  const reopenedDetail = new Promise<ApiKey>((resolve) => {
    resolveReopenedDetail = resolve
  })
  installReadFixtures([key], (candidate, readCount) => {
    if (candidate.id === key.id && readCount === 2) {
      reopenedDetailRequested = true
      return reopenedDetail
    }
    return candidate
  })
  const update = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: false } })
  const drawer = await renderDrawer(key)
  await changeQuota('120')
  await submit()
  await waitFor(() => expect(update).toHaveBeenCalledTimes(1))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled()
  )

  await drawer.rerender(false, key)
  await drawer.rerender(true, key, { waitUntilReady: false })
  await waitFor(() => expect(reopenedDetailRequested).toBe(true))
  await waitFor(() => {
    expect(document.getElementById('api-key-form')).toHaveAttribute(
      'aria-busy',
      'true'
    )
    expect(document.getElementById('api-key-form')).toHaveAttribute('inert')
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  })

  await act(async () => {
    resolveReopenedDetail(key)
    await reopenedDetail
  })
  await waitForDrawerReady(key)
  expect(
    screen.getByRole('spinbutton', {
      name: 'Key quota (internal units)',
    })
  ).toHaveValue(100)
  await changeQuota('120')
  await submit()
  await waitFor(() => expect(update).toHaveBeenCalledTimes(2))

  const firstPayload = update.mock.calls[0][1]
  const secondPayload = update.mock.calls[1][1]
  if (!firstPayload || typeof firstPayload !== 'object') {
    throw new TypeError(
      'Expected the first quota update payload to be an object'
    )
  }
  const firstRequestId = getRequestId(update.mock.calls[0])
  const secondRequestId = getRequestId(update.mock.calls[1])
  const expectedQuotaIntent = {
    id: key.id,
    name: key.name,
    remain_quota: 120,
    quota_version: key.quota_version,
    metadata_fingerprint: key.metadata_fingerprint,
  }
  expect(firstPayload).toMatchObject(expectedQuotaIntent)
  expect(secondPayload).toMatchObject(expectedQuotaIntent)
  expect(firstRequestId).toEqual(expect.any(String))
  expect(secondRequestId).toEqual(expect.any(String))
  expect(secondRequestId).not.toBe(firstRequestId)
  expect(secondPayload).toEqual({
    ...firstPayload,
    request_id: secondRequestId,
  })
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
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled()
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
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled()
  )
  await submit()
  await waitFor(() => expect(update).toHaveBeenCalledTimes(2))

  const firstRequestId = getRequestId(update.mock.calls[0])
  const secondRequestId = getRequestId(update.mock.calls[1])
  expect(firstRequestId).toEqual(expect.any(String))
  expect(secondRequestId).toEqual(expect.any(String))
  expect(secondRequestId).not.toBe(firstRequestId)
})
