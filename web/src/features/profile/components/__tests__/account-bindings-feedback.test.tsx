import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import type { UserProfile } from '../../types'
import { AccountBindingsTab } from '../tabs/account-bindings-tab'

const profile: UserProfile = { id: 42, username: 'synthetic-owner', display_name: 'Synthetic Owner', role: 1, group: 'default', quota: 0, used_quota: 0, request_count: 0, status: 1, aff_count: 0, aff_quota: 0, aff_history_quota: 0, created_time: 1 }
const providerName = 'Synthetic provider with a complete long name / final suffix'
const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
let client: QueryClient
beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 42, role: 1, username: 'synthetic-owner' })
  vi.spyOn(window, 'open').mockReturnValue(null)
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); useSystemConfigStore.setState(originalConfig); localStorage.clear(); vi.restoreAllMocks() })

it.each(['transport', 'business'] as const)('a %s custom binding read failure stays Unknown and retries explicitly before Bind is available', async (kind) => {
  let reads = 0
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') return { data: { success: true, data: { custom_oauth_providers: [{ id: 7, slug: 'synthetic-provider', name: providerName, client_id: 'synthetic-client', authorization_endpoint: 'https://synthetic.invalid/authorize' }] } } }
    if (url === '/api/user/oauth/bindings') {
      reads += 1
      if (reads === 1) {
        if (kind === 'transport') throw new Error('Synthetic binding read failed')
        return { data: { success: false, message: 'Synthetic binding read failed' } }
      }
      return { data: { success: true, data: [] } }
    }
    throw new Error(`Unexpected synthetic GET ${url}`)
  })
  const user = userEvent.setup()
  render(<QueryClientProvider client={client}><AccountBindingsTab profile={profile} onUpdate={() => {}} securityScopeKey='synthetic-owner-bindings' /></QueryClientProvider>)
  expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load OAuth bindings')
  const provider = screen.getByRole('group', { name: providerName })
  expect(within(provider).getByText('Unknown')).toBeVisible()
  expect(within(provider).queryByText('Not bound')).not.toBeInTheDocument()
  expect(within(provider).getByRole('button', { name: `Bind ${providerName}` })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await within(provider).findByText('Not bound')).toBeVisible()
  expect(within(provider).getByRole('button', { name: `Bind ${providerName}` })).toBeEnabled()
  expect(reads).toBe(2)
  expect(window.open).not.toHaveBeenCalled()
})
