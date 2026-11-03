import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type { CredentialMaintenance } from '../../credential-maintenance-api'
import { savePendingCredentialMaintenanceOperation } from '../../credential-maintenance-pending-operation'
import { CredentialMaintenanceDialog } from '../credential-maintenance-dialog'

const originalAuth = useAuthStore.getState()
const operationId = 'c'.repeat(64)
const maintenance: CredentialMaintenance = {
  token: { id: 21, name: 'Synthetic maintenance Key', masked_key: 'sk-…abcd', owner_user_id: 1 },
  permissions: { can_migrate: true, reason: '' },
  rotation: { commit_supported: false, status: 'blocked', blockers: [{ code: 'rotation_commit_not_supported', state: 'blocked' }] },
  migration: {
    status: 'ready',
    source: { channel_id: 7, channel_name: 'Synthetic current account', subject_hint: 'abcd…wxyz', windows: [{ pool_id: 11, window_slot: 'primary', revision: 3, share_bps: 2500, observed_at: 1, reset_at: 2 }] },
    candidates: [{ channel_id: 8, channel_name: 'Synthetic target account', subject_hint: 'efgh…stuv', credential_mode: 'single', status: 'ready', blockers: [], windows: [{ pool_id: 12, window_slot: 'primary', revision: 2, share_bps: 0, available_bps: 7500, observed_at: 1, reset_at: 2 }] }],
    blockers: [],
  },
}
let client: QueryClient
beforeEach(() => {
  sessionStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: 100, username: 'synthetic-root' })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/token/21/maintenance') return { data: { success: true, data: maintenance } }
    if (url === `/api/token/21/maintenance/operations/${operationId}`) throw new Error('Synthetic unknown receipt')
    if (url === '/api/user/2fa/status' || url === '/api/user/passkey') return { data: { success: true, data: { enabled: false } } }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockResolvedValue({ data: { success: false, message: 'Synthetic preview unavailable' } })
})
afterEach(() => { cleanup(); client.clear(); sessionStorage.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })
function fixture() {
  return <QueryClientProvider client={client}><TooltipProvider><CredentialMaintenanceDialog tokenId={21} onClose={vi.fn()} /></TooltipProvider></QueryClientProvider>
}

it('preserves idle account and pool selection through collapse without requests and forces configuration open after an explicit failed preview', async () => {
  render(fixture())
  const actions = userEvent.setup()
  const trigger = await screen.findByRole('button', { name: 'Migration configuration' })
  expect(trigger).toHaveAttribute('aria-expanded', 'false')
  expect(screen.getByText('Credential rotation cannot be committed yet.')).toBeVisible()
  const reads = vi.mocked(api.get).mock.calls.length
  trigger.focus()
  await actions.keyboard('{Enter}')
  await actions.click(screen.getByRole('combobox', { name: 'Target Codex account' }))
  await actions.click(screen.getByRole('option', { name: 'Synthetic target account · efgh…stuv' }))
  expect(screen.getByRole('combobox', { name: 'Primary window' })).toHaveTextContent('Pool #12')
  expect(screen.getByText('This migration keeps the current 25% share.')).toBeVisible()
  await actions.click(trigger)
  expect(trigger).toHaveAttribute('aria-expanded', 'false')
  expect(screen.getByText('Credential rotation cannot be committed yet.')).toBeVisible()
  await actions.click(trigger)
  expect(screen.getByRole('combobox', { name: 'Target Codex account' })).toHaveTextContent('Synthetic target account')
  expect(screen.getByRole('combobox', { name: 'Primary window' })).toHaveTextContent('Pool #12')
  expect(api.get).toHaveBeenCalledTimes(reads)
  expect(api.post).not.toHaveBeenCalled()
  await actions.click(screen.getByRole('button', { name: 'Preview migration' }))
  expect(await screen.findByText('Migration preview failed')).toBeVisible()
  expect(trigger).toHaveAttribute('aria-expanded', 'true')
  expect(trigger).toBeDisabled()
  expect(api.post).toHaveBeenCalledExactlyOnceWith('/api/token/21/account-migration/preview', {
    target_channel_id: 8, target_shares: [{ source_pool_id: 11, target_pool_id: 12, share_bps: 2500 }],
  }, { signal: expect.any(AbortSignal), timeout: 15000, disableDuplicate: true, skipAuthRefresh: true, skipBusinessError: true, skipErrorHandler: true })
})

it('keeps an unknown migration and its exact receipt recovery outside collapsed configuration without repeating any POST', async () => {
  savePendingCredentialMaintenanceOperation(1, 21, operationId)
  render(fixture())
  const actions = userEvent.setup()
  expect(await screen.findByText('Migration result is unknown')).toBeVisible()
  const configuration = await screen.findByRole('button', { name: 'Migration configuration' })
  expect(configuration).toHaveAttribute('aria-expanded', 'true')
  expect(configuration).toBeDisabled()
  expect(screen.getByText('Credential rotation cannot be committed yet.')).toBeVisible()
  await actions.click(screen.getByRole('button', { name: 'Check receipt again' }))
  await waitFor(() => expect(vi.mocked(api.get).mock.calls.filter(([url]) => url === `/api/token/21/maintenance/operations/${operationId}`)).toHaveLength(2))
  expect(screen.getByText('Migration result is unknown')).toBeVisible()
  expect(api.post).not.toHaveBeenCalled()
})
