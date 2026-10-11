import { QueryClient } from '@tanstack/react-query'
import { cleanup, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { formatQuota } from '@/lib/format'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { apiKeySchema, type ApiKey } from '../../types'
import { ApiKeysProvider } from '../api-keys-provider'
import { ApiKeysTable } from '../api-keys-table'

const originalConfig = useSystemConfigStore.getState().config
const originalInnerWidth = Object.getOwnPropertyDescriptor(
  window,
  'innerWidth'
)
let client: QueryClient

function makeKey(usedQuota: number, remainQuota: number): ApiKey {
  return apiKeySchema.parse({
    id: 21,
    name: 'wide-key',
    key: 'masked',
    status: 1,
    remain_quota: remainQuota,
    used_quota: usedQuota,
    unlimited_quota: false,
    expired_time: -1,
    created_time: 1,
    accessed_time: 1,
    group: 'default',
    model_limits_enabled: false,
  })
}

function setViewport(layout: 'desktop' | 'mobile') {
  const width = layout === 'mobile' ? 320 : 1280
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    value: width,
  })
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    matches: query.includes('max-width') && width < 768,
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  }))
}

async function renderTable(item: ApiKey) {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') {
      return {
        data: {
          success: true,
          data: { quota_display_type: 'TOKENS' },
        },
      }
    }
    if (url === '/api/user/self/groups') {
      return {
        data: {
          success: true,
          data: { default: { desc: 'Default', ratio: 1 } },
        },
      }
    }
    if (url.startsWith('/api/token/?')) {
      return {
        data: {
          success: true,
          data: { items: [item], total: 1, page: 1, page_size: 20 },
        },
      }
    }
    throw new Error(`Unexpected GET ${url}`)
  })

  return renderListRoute({
    client,
    path: '/keys/',
    entry: '/keys/',
    element: (
      <ApiKeysProvider>
        <ApiKeysTable />
      </ApiKeysProvider>
    ),
  })
}

beforeEach(() => {
  localStorage.clear()
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'TOKENS' },
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  useSystemConfigStore.setState({ config: originalConfig })
  if (originalInnerWidth) {
    Object.defineProperty(window, 'innerWidth', originalInnerWidth)
  }
  localStorage.clear()
  vi.restoreAllMocks()
})

test.each(['desktop', 'mobile'] as const)(
  'keeps the normal %s quota display and progress at the exact safe boundary',
  async (layout) => {
    setViewport(layout)
    await renderTable(makeKey(Number.MAX_SAFE_INTEGER, 0))

    expect(await screen.findByText('wide-key')).toBeVisible()
    expect(screen.queryByText('Unavailable')).not.toBeInTheDocument()
    if (layout === 'desktop') {
      expect(screen.getByRole('progressbar')).toBeVisible()
    }
  }
)

test.each(['desktop', 'mobile'] as const)(
  'shows both exact components and no derived progress when the %s total exceeds the safe boundary',
  async (layout) => {
    setViewport(layout)
    const expectedUsed = formatQuota(Number.MAX_SAFE_INTEGER)
    const expectedRemaining = formatQuota(1)
    await renderTable(makeKey(Number.MAX_SAFE_INTEGER, 1))

    expect(await screen.findByText('wide-key')).toBeVisible()
    const labelSuffix = layout === 'mobile' ? ':' : ''
    const used = screen.getByText(`Used quota${labelSuffix}`)
    const remaining = screen.getByText(`Remaining quota${labelSuffix}`)
    expect(used).toBeVisible()
    expect(remaining).toBeVisible()
    expect(used.parentElement).toHaveTextContent(expectedUsed)
    expect(remaining.parentElement).toHaveTextContent(expectedRemaining)
    expect(screen.getByText(`Total quota${labelSuffix}`)).toBeVisible()
    expect(screen.getByText('Unavailable')).toBeVisible()
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
  }
)
