import { QueryClient } from '@tanstack/react-query'
import { cleanup, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import type { Model } from '../../types'
import { ModelsProvider } from '../models-provider'
import { ModelsTable } from '../models-table'

const item: Model = { id: 24, model_name: 'synthetic-metadata-model', description: 'Saved complete model description', status: 1, sync_official: 0, created_time: 1, updated_time: 2, name_rule: 0, tags: 'tag-one,tag-two', endpoints: '{"openai":{"path":"/v1/chat/completions","method":"POST"}}', bound_channels: [{ name: 'Saved connection', type: 1 }], enable_groups: ['saved-group'], quota_types: [0] }
const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
let client: QueryClient

beforeEach(() => {
  localStorage.clear()
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: 100, username: 'synthetic-admin' })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') return { data: { success: true, data: {} } }
    if (url === '/api/vendors/') return { data: { success: true, data: { items: [], total: 0 } } }
    if (url === '/api/models/' || url === '/api/models/search') return { data: { success: true, data: { items: [item], total: 1, vendor_counts: { all: 1 } } } }
    throw new Error(`Unexpected GET ${url}`)
  })
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); useSystemConfigStore.setState(originalConfig); localStorage.clear(); vi.restoreAllMocks() })

it('exposes complete metadata and runtime fields through a reachable phone disclosure', async () => {
  const original = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({ ...original(query), matches: query === '(max-width: 640px)' }))
  await renderListRoute({ client, path: '/models/$section', entry: '/models/metadata', element: <ModelsProvider><ModelsTable /></ModelsProvider> })
  await screen.findByText(item.model_name)
  const user = userEvent.setup()
  await user.click(screen.getByText('More details'))
  expect(screen.getByText('ID')).toBeVisible()
  expect(screen.getByText('Bound Channels')).toBeVisible()
  expect(screen.getByText('Quota Types')).toBeVisible()
  expect(screen.getByText('Saved complete model description')).toBeVisible()
  expect(screen.getByText('saved-group')).toBeVisible()
  expect(screen.getByText('Updated')).toBeVisible()
})
