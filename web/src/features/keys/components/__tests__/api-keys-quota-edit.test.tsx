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

import { apiKeySchema } from '../../types'
import { ApiKeysMutateDrawer } from '../api-keys-mutate-drawer'
import { ApiKeysProvider } from '../api-keys-provider'

const originalConfig = useSystemConfigStore.getState().config
const clients: QueryClient[] = []

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useSystemConfigStore.setState({ config: originalConfig })
})

it.each([false, true])(
  'preserves quota when editing metadata; explicit quota change=%s',
  async (changeQuota) => {
    const key = apiKeySchema.parse({
      id: 7,
      name: 'Accounting key',
      key: 'synthetic-masked',
      status: 1,
      remain_quota: changeQuota ? 100 : 4294967313,
      used_quota: 4294967321,
      quota_version: '9007199254740993',
      metadata_fingerprint: 'a'.repeat(64),
      unlimited_quota: !changeQuota,
      expired_time: -1,
      created_time: 0,
      accessed_time: 0,
      group: 'default',
      model_limits_enabled: false,
      model_limits: '',
      allow_ips: '',
    })
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/token/7') return { data: { success: true, data: key } }
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
    const update = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    useSystemConfigStore.getState().setConfig({
      currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'TOKENS' },
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    clients.push(client)
    const i18n = createInstance()
    await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
    render(
      <QueryClientProvider client={client}>
        <I18nextProvider i18n={i18n}>
          <ApiKeysProvider>
            <ApiKeysMutateDrawer
              open
              currentRow={key}
              onOpenChange={() => undefined}
            />
          </ApiKeysProvider>
        </I18nextProvider>
      </QueryClientProvider>
    )
    const user = userEvent.setup()
    const name = await screen.findByDisplayValue('Accounting key')
    fireEvent.change(name, { target: { value: 'Renamed key' } })
    if (changeQuota) {
      // A refetch during editing must not silently adopt the new balance version.
      act(() =>
        client.setQueryData(['api-key', key.id], {
          success: true,
          data: { ...key, remain_quota: 75, quota_version: '9007199254740994', metadata_fingerprint: 'b'.repeat(64) },
        })
      )
      const quota = await screen.findByRole('spinbutton', {
        name: 'Key quota (internal units)',
      })
      fireEvent.change(quota, { target: { value: '120' } })
    }
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(update).toHaveBeenCalledOnce())
    const payload = update.mock.calls[0][1]
    expect(payload).toMatchObject({ id: 7, name: 'Renamed key' })
    if (changeQuota) {
      expect(payload).toMatchObject({
        remain_quota: 120,
        quota_version: '9007199254740993',
        metadata_fingerprint: 'a'.repeat(64),
      })
    } else {
      expect(payload).not.toHaveProperty('remain_quota')
      expect(payload).not.toHaveProperty('quota_version')
      expect(payload).not.toHaveProperty('metadata_fingerprint')
    }
  }
)
