import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { apiKeySchema } from '../../types'
import { ApiKeysMutateDrawer } from '../api-keys-mutate-drawer'
import { ApiKeysProvider } from '../api-keys-provider'
import { CredentialMaintenanceDialog } from '../credential-maintenance-dialog'
import { CredentialRotationDialog } from '../credential-rotation-dialog'
import { ModelScopeDialog } from '../model-scope-dialog'
import { TokenBudgetDialog } from '../token-budget-dialog'

const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
const modelName = 'synthetic/complete-model-id-with-a-very-long-final-suffix'
const keyName = 'Synthetic complete Key name with a long final suffix'
const key = apiKeySchema.parse({ id: 21, name: keyName, key: 'synthetic-masked', status: 1, remain_quota: 100, used_quota: 0, unlimited_quota: false, expired_time: -1, created_time: 1, accessed_time: 1, group: 'default', model_limits_enabled: true, model_limits: modelName, allow_ips: '', quota_version: '1', metadata_fingerprint: 'a'.repeat(64) })
let client: QueryClient
beforeEach(() => {
  sessionStorage.clear()
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.setState((state) => ({ ...state, auth: { ...state.auth, user: { id: 1, role: 100, username: 'synthetic-root' }, session: null } }))
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/token/21/budget') return { data: { success: true, data: { policy: { token_id: 21, user_id: 1, enabled: false, account_threshold_enabled: false, account_min_remaining_bps: 2000, account_max_age_seconds: 300, fee_enabled: false, fee_limit_usd: '0', fee_used_usd: '0', fee_reserved_usd: '0', limit: 100, used: 0, reserved: 0, pending_request_id: '', revision: 0 }, pending: null } } }
    if (url === '/api/token/21/model-limits') return { data: { success: true, data: { token_id: 21, token_name: keyName, model_limits_enabled: true, models: [modelName], revision: 'a'.repeat(64) } } }
    if (url === '/api/token/21/maintenance') return { data: { success: true, data: { token: { id: 21, name: keyName, masked_key: 'sk-…synthetic', owner_user_id: 1 }, permissions: { can_migrate: false, reason: 'Synthetic blocked migration' }, rotation: { commit_supported: false, status: 'blocked', blockers: [{ code: 'rotation_commit_not_supported', state: 'blocked' }] }, migration: { status: 'blocked', source: null, candidates: [], blockers: [{ code: 'no_source', state: 'blocked' }] } } } }
    if (url === '/api/token/21') return { data: { success: true, data: key } }
    if (url === '/api/status') return { data: { success: true, data: { default_use_auto_group: false, quota_display_type: 'TOKENS' } } }
    if (url === '/api/user/models') return { data: { success: true, data: [modelName, 'synthetic/catalog-model-with-complete-long-suffix'] } }
    if (url === '/api/user/self/groups') return { data: { success: true, data: { default: { desc: 'Standard', ratio: 1 } } } }
    if (url === '/api/token/auto-groups') return { data: { success: true, data: { groups: ['default'], max_count: 5 } } }
    if (url === '/api/user/2fa/status' || url === '/api/user/passkey') return { data: { success: true, data: { enabled: false } } }
    throw new Error(`Unexpected synthetic GET ${url}`)
  })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected synthetic write'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected synthetic write'))
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); useSystemConfigStore.setState(originalConfig); sessionStorage.clear(); localStorage.clear(); vi.restoreAllMocks() })

function renderTask(element: ReactNode) {
  return render(<QueryClientProvider client={client}><TooltipProvider><ApiKeysProvider>{element}</ApiKeysProvider></TooltipProvider></QueryClientProvider>)
}

it('budget guidance and forms share one body while title and refresh or close remain outside it', async () => {
  renderTask(<TokenBudgetDialog tokenId={21} onClose={() => {}} />)
  await screen.findByRole('button', { name: 'Save' })
  const body = screen.getByRole('region', { name: 'Usage budget task' })
  body.focus()
  expect(body).toHaveFocus()
  expect(body).toHaveClass('min-h-0', 'overflow-y-auto')
  expect(within(body).getByRole('checkbox', { name: 'Enable strict Token budget' })).toBeVisible()
  expect(body).not.toContainElement(screen.getByRole('heading', { name: 'API Key usage budgets' }))
  const refresh = screen.getByRole('button', { name: 'Refresh' })
  const close = within(screen.getByRole('group', { name: 'Actions' })).getByRole('button', { name: 'Close' })
  expect(body).not.toContainElement(refresh)
  expect(body).not.toContainElement(close)
  expect(refresh).toHaveClass('min-h-11')
  expect(close).toHaveClass('min-h-11')
  expect(api.put).not.toHaveBeenCalled()
})

it('model scope keeps complete stored and catalog IDs in its single scroll body with 44px candidate actions', async () => {
  renderTask(<ModelScopeDialog apiKey={key} onClose={() => {}} />)
  await screen.findByText(modelName)
  const body = screen.getByRole('region', { name: 'Model scope task' })
  body.focus()
  expect(body).toHaveFocus()
  expect(body).toHaveClass('min-h-0', 'overflow-y-auto')
  const candidate = await screen.findByRole('button', { name: /synthetic\/catalog-model-with-complete-long-suffix/ })
  expect(candidate).toHaveClass('min-h-11')
  expect(candidate.parentElement).not.toHaveClass('overflow-y-auto')
  expect(body).not.toContainElement(screen.getByRole('button', { name: 'Save model scope' }))
  expect(body).not.toContainElement(screen.getByRole('heading', { name: 'Model scope' }))
  expect(api.put).not.toHaveBeenCalled()
})

it('credential maintenance keeps the complete Key identity in its body while the original Close stays fixed', async () => {
  renderTask(<CredentialMaintenanceDialog tokenId={21} onClose={() => {}} />)
  const body = screen.getByRole('region', { name: 'Credential maintenance task' })
  expect(await within(body).findByText(keyName)).toBeVisible()
  body.focus()
  expect(body).toHaveFocus()
  expect(body).toHaveClass('min-h-0', 'overflow-y-auto')
  expect(body).not.toContainElement(screen.getByRole('heading', { name: 'Credential maintenance' }))
  const close = within(screen.getByRole('group', { name: 'Actions' })).getByRole('button', { name: 'Close' })
  expect(close).toHaveClass('min-h-11')
  expect(body).not.toContainElement(close)
  expect(api.post).not.toHaveBeenCalled()
})

it('rotation starts without a write or secret and keeps the eligibility action in one scroll body', async () => {
  renderTask(<CredentialRotationDialog open tokenId={21} tokenName={keyName} maskedKey='sk-…synthetic' userId={1} onOpenChange={() => {}} onWriteLockChange={() => {}} onCommitted={() => {}} />)
  const body = screen.getByRole('region', { name: 'Credential rotation task' })
  expect(within(body).getByText(keyName)).toBeVisible()
  body.focus()
  expect(body).toHaveFocus()
  expect(body).toHaveClass('min-h-0', 'overflow-y-auto')
  expect(within(body).getByRole('button', { name: 'Check rotation eligibility' })).toHaveClass('min-h-11')
  expect(body).not.toContainElement(screen.getByRole('heading', { name: 'Credential rotation' }))
  expect(body).not.toContainElement(within(screen.getByRole('group', { name: 'Actions' })).getByRole('button', { name: 'Close' }))
  expect(screen.queryByRole('button', { name: /reveal/i })).not.toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})

it('the existing Key editor exposes complete stored model IDs in its existing single form scroll body', async () => {
  const user = userEvent.setup()
  renderTask(<ApiKeysMutateDrawer open currentRow={key} onOpenChange={() => {}} />)
  const save = await screen.findByRole('button', { name: 'Save changes' })
  await waitFor(() => expect(save).toBeEnabled())
  expect(save).toHaveClass('min-h-11')
  await user.click(screen.getByRole('button', { name: /Advanced Settings/ }))
  const stored = await screen.findByText(modelName)
  expect(stored).toBeVisible()
  expect(stored.parentElement).not.toHaveClass('overflow-y-auto')
  expect(stored.closest('form')).toHaveClass('min-h-0', 'overflow-y-auto')
  expect(api.put).not.toHaveBeenCalled()
})
