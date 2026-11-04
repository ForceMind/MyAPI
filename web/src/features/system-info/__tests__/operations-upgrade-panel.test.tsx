import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError, type AxiosAdapter, type InternalAxiosRequestConfig } from 'axios'
import { StrictMode } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { OperationsUpgradePanel } from '../components/operations-upgrade-panel'
import { operationsProofScope, type OperationsPlan } from '../operations-api'
import { now, operationResponse, operationsSignIn, plan } from './operations-fixtures'

const previousAuth = useAuthStore.getState()
const previousAdapter = api.defaults.adapter
let client: QueryClient
beforeEach(() => {
  operationsSignIn()
  vi.spyOn(Date, 'now').mockReturnValue(now * 1000)
  client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } } })
})
afterEach(() => { cleanup(); client.clear(); api.defaults.adapter = previousAdapter; useAuthStore.setState(previousAuth); vi.restoreAllMocks() })

function renderPanel(adapter: AxiosAdapter) {
  api.defaults.adapter = adapter
  render(<QueryClientProvider client={client}><OperationsUpgradePanel /></QueryClientProvider>)
  return userEvent.setup()
}

function defaultResponse(config: InternalAxiosRequestConfig, plans: OperationsPlan[] = [plan]) {
  if (config.url === '/api/operations/plans') return operationResponse(config, plans)
  if (config.url === '/api/operations/instances') return operationResponse(config, [{ id: plan.instance_id, state: 'idle' }])
  if (config.url === `/api/operations/plans/${plan.id}`) return operationResponse(config, plan)
  if (config.url === '/api/user/2fa/status') return operationResponse(config, { enabled: true })
  if (config.url === '/api/user/passkey') return operationResponse(config, { enabled: false })
  if (config.url === '/api/verify') return operationResponse(config, { proof_token: 'synthetic-proof', expires_at: now + 60, method: '2fa', scope: operationsProofScope(plan) })
  if (config.url === `/api/operations/plans/${plan.id}/approve`) return operationResponse(config, { ...plan, state: 'approved', approved_at: now })
  throw new Error(`Unexpected request ${config.method} ${config.url}`)
}

async function openReview(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('button', { name: 'Review upgrade approval' }))
  return screen.findByRole('dialog', { name: 'Review upgrade approval' })
}
async function openVerification(user: ReturnType<typeof userEvent.setup>) {
  const review = await openReview(user)
  await user.click(within(review).getByRole('checkbox', { name: 'I confirm this exact upgrade target and plan digest.' }))
  await user.click(within(review).getByRole('button', { name: 'Verify and approve this plan' }))
  const verification = await screen.findByRole('dialog', { name: 'Verify upgrade approval' })
  await within(verification).findByLabelText('Authenticator or backup code')
  return verification
}

it.each([ROLE.ADMIN, ROLE.USER, ROLE.GUEST])('hides upgrade approvals and performs no operations calls for role %s', (role) => {
  operationsSignIn(role)
  const adapter = vi.fn<AxiosAdapter>()
  renderPanel(adapter)
  expect(screen.queryByText('Upgrade approvals')).not.toBeInTheDocument()
  expect(adapter).not.toHaveBeenCalled()
})

it('hides upgrade approvals for a Root identity without a session', () => {
  useAuthStore.getState().auth.reset()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'owner', role: ROLE.SUPER_ADMIN })
  const adapter = vi.fn<AxiosAdapter>()
  renderPanel(adapter)
  expect(adapter).not.toHaveBeenCalled()
  expect(screen.queryByText('Upgrade approvals')).not.toBeInTheDocument()
})

it('shows unconfigured operations without inventing a zero count or starting verification', async () => {
  const requests: string[] = []
  renderPanel(async (config) => {
    requests.push(config.url ?? '')
    throw new AxiosError('private service secret', AxiosError.ERR_BAD_RESPONSE, config, undefined, { config, data: { code: 'OPERATIONS_UNAVAILABLE', message: 'private service secret' }, status: 503, statusText: 'Unavailable', headers: {} })
  })
  expect(await screen.findByText('Operations service not configured')).toBeInTheDocument()
  expect(screen.queryByText('No upgrade plans returned.')).not.toBeInTheDocument()
  expect(screen.queryByText(/private service secret/)).not.toBeInTheDocument()
  expect(requests.sort()).toEqual(['/api/operations/instances', '/api/operations/plans'])
})

it('keeps full image IDs and digest behind explicit details, then shows the exact confirmation target', async () => {
  const requests: string[] = []
  const user = renderPanel(async (config) => { requests.push(config.url ?? ''); return defaultResponse(config) })
  await screen.findByText(`${plan.source_version} → ${plan.version} · ${plan.instance_id}`)
  expect(screen.getByText(plan.target_image_id)).not.toBeVisible()
  expect(requests.some((url) => url.includes('/api/user/'))).toBe(false)
  const review = await openReview(user)
  expect(within(review).getByText(plan.target_image_id)).toBeInTheDocument()
  expect(within(review).getByText(plan.source_image_id)).toBeInTheDocument()
  expect(within(review).getByText(plan.digest)).toBeInTheDocument()
  expect(within(review).getByText(plan.source_sha)).toBeInTheDocument()
  expect(within(review).getByText(operationsProofScope(plan))).toBeInTheDocument()
  expect(within(review).getByText('9007199254740993')).toBeInTheDocument()
  expect(within(review).getByText('The retained old container will have automatic restart disabled. The new container will keep the current restart policy.')).toBeVisible()
  expect(within(review).queryByText('Source mutation attempted')).not.toBeInTheDocument()
  expect(within(review).queryByText('Source auto-restart disabled')).not.toBeInTheDocument()
  expect(within(review).getByRole('button', { name: 'Verify and approve this plan' })).toBeDisabled()
})

it('obtains plan-bound proof before exactly one approval POST, rechecking the original plan before each boundary', async () => {
  const requests: InternalAxiosRequestConfig[] = []
  const user = renderPanel(async (config) => { requests.push(config); return defaultResponse(config) })
  const verification = await openVerification(user)
  expect(requests.filter((request) => request.method === 'post')).toHaveLength(0)
  await user.type(within(verification).getByLabelText('Authenticator or backup code'), '123456')
  await user.click(within(verification).getByRole('button', { name: 'Verify' }))
  expect(await screen.findByText('Upgrade approved. A trusted operations tool must request execution separately.')).toBeInTheDocument()
  const writes = requests.filter((request) => request.method === 'post')
  expect(writes.map((request) => request.url)).toEqual(['/api/verify', `/api/operations/plans/${plan.id}/approve`])
  expect(JSON.parse(writes[0]?.data as string).scope).toBe(operationsProofScope(plan))
  expect(writes[1]?.headers.get('X-Security-Proof')).toBe('synthetic-proof')
  expect(JSON.parse(writes[1]?.data as string)).toEqual({ digest: plan.digest })
  expect(requests.filter((request) => request.url === `/api/operations/plans/${plan.id}`)).toHaveLength(2)
  expect(requests.some((request) => /execute|model|diag/.test(request.url ?? ''))).toBe(false)
})

it('blocks an expired prepared plan without requesting proof or approval', async () => {
  const requests: InternalAxiosRequestConfig[] = []
  const user = renderPanel(async (config) => { requests.push(config); return defaultResponse(config, [{ ...plan, expires_at: now }]) })
  const review = await openReview(user)
  expect(within(review).getByText('This upgrade plan has expired. Prepare a new plan before approving.')).toBeInTheDocument()
  expect(within(review).getByRole('button', { name: 'Verify and approve this plan' })).toBeDisabled()
  expect(requests.some((request) => request.method === 'post')).toBe(false)
})

it.each(['before-proof', 'after-proof'] as const)('does not approve when the digest changes %s and never replaces the confirmed target', async (changeAt) => {
  let reads = 0
  const writes: InternalAxiosRequestConfig[] = []
  const user = renderPanel(async (config) => {
    if (config.method === 'post') writes.push(config)
    if (config.url === `/api/operations/plans/${plan.id}`) {
      reads += 1
      if (changeAt === 'before-proof' || reads === 2) return operationResponse(config, { ...plan, digest: 'c'.repeat(64), target_image_id: `sha256:${'7'.repeat(64)}` })
    }
    return defaultResponse(config)
  })
  if (changeAt === 'before-proof') {
    const review = await openReview(user)
    await user.click(within(review).getByRole('checkbox'))
    await user.click(within(review).getByRole('button', { name: 'Verify and approve this plan' }))
  } else {
    const verification = await openVerification(user)
    await user.type(within(verification).getByLabelText('Authenticator or backup code'), '123456')
    await user.click(within(verification).getByRole('button', { name: 'Verify' }))
  }
  expect(await screen.findByText('This plan changed. Close this review and inspect the current plan; the confirmed target was not replaced.')).toBeInTheDocument()
  const review = screen.getByRole('dialog', { name: 'Review upgrade approval' })
  expect(within(review).getByText(plan.digest)).toBeInTheDocument()
  expect(within(review).getByText(plan.target_image_id)).toBeInTheDocument()
  expect(writes.some((request) => request.url?.endsWith('/approve'))).toBe(false)
})

it('requires Root to configure 2FA or Passkey when neither verification method is available', async () => {
  const writes: InternalAxiosRequestConfig[] = []
  const user = renderPanel(async (config) => {
    if (config.method === 'post') writes.push(config)
    if (config.url?.startsWith('/api/user/')) return operationResponse(config, { enabled: false })
    return defaultResponse(config)
  })
  const review = await openReview(user)
  await user.click(within(review).getByRole('checkbox'))
  await user.click(within(review).getByRole('button', { name: 'Verify and approve this plan' }))
  const verification = await screen.findByRole('dialog', { name: 'Verify upgrade approval' })
  expect(await within(verification).findByText('Enable Two-factor Authentication or Passkey in your profile to unlock sensitive operations.')).toBeInTheDocument()
  expect(within(verification).getByRole('button', { name: 'Verify' })).toBeDisabled()
  expect(writes).toHaveLength(0)
})

it('recovers a network-unknown approval only with GET and keeps the plan locked against replay', async () => {
  const requests: InternalAxiosRequestConfig[] = []
  let unknownWrite = false
  const user = renderPanel(async (config) => {
    requests.push(config)
    if (config.url?.endsWith('/approve')) { unknownWrite = true; throw new AxiosError('private transport secret', AxiosError.ERR_NETWORK, config) }
    if (unknownWrite && config.url === `/api/operations/plans/${plan.id}`) return operationResponse(config, { ...plan, state: 'approved' })
    return defaultResponse(config)
  })
  const verification = await openVerification(user)
  await user.type(within(verification).getByLabelText('Authenticator or backup code'), '123456')
  await user.click(within(verification).getByRole('button', { name: 'Verify' }))
  const review = await screen.findByRole('dialog', { name: 'Review upgrade approval' })
  await waitFor(() => expect(within(review).getAllByRole('button', { name: 'Close' })[0]).toBeEnabled())
  await user.click(within(review).getAllByRole('button', { name: 'Close' })[0])
  expect(await screen.findByText('Approval result unknown')).toBeInTheDocument()
  const sameSessionAdapter = api.defaults.adapter as AxiosAdapter
  cleanup()
  renderPanel(sameSessionAdapter)
  expect(await screen.findByText('Approval result unknown')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Check approval status' }))
  expect(await screen.findByText('Plan status checked. No approval or execution request was replayed.')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Review upgrade approval' })).toBeDisabled()
  expect(requests.filter((request) => request.url?.endsWith('/approve'))).toHaveLength(1)
  expect(screen.queryByText(/private transport secret/)).not.toBeInTheDocument()
})

it.each(['session', 'actor', 'permissions'] as const)('discards a late verification response after the %s snapshot changes', async (change) => {
  let finishProof: (() => void) | undefined
  const requests: InternalAxiosRequestConfig[] = []
  const user = renderPanel(async (config) => {
    requests.push(config)
    if (config.url === '/api/verify') {
      await new Promise<void>((resolve) => { finishProof = resolve })
      return defaultResponse(config)
    }
    if (config.headers.Authorization === 'Bearer session-replacement' && config.url === '/api/operations/plans') return operationResponse(config, [])
    return defaultResponse(config)
  })
  const verification = await openVerification(user)
  await user.type(within(verification).getByLabelText('Authenticator or backup code'), '123456')
  await user.click(within(verification).getByRole('button', { name: 'Verify' }))
  await waitFor(() => expect(finishProof).toBeTypeOf('function'))
  act(() => {
    if (change === 'session') operationsSignIn(ROLE.SUPER_ADMIN, 'replacement')
    else if (change === 'actor') operationsSignIn(ROLE.SUPER_ADMIN, useAuthStore.getState().auth.session?.sid, 2)
    else useAuthStore.getState().auth.setUser({ id: 1, username: 'owner', role: ROLE.SUPER_ADMIN, permissions: { admin_permissions: { channel: { read: false } } } })
  })
  await act(async () => { finishProof?.() })
  if (change === 'session') expect(await screen.findByText('No upgrade plans returned.')).toBeInTheDocument()
  else expect(await screen.findByRole('button', { name: 'Review upgrade approval' })).toBeInTheDocument()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(requests.some((request) => request.url?.endsWith('/approve'))).toBe(false)
})

it('keeps recovery-required source mutation risk visible before target start and beyond the ten latest completed plans', async () => {
  const completed = Array.from({ length: 11 }, (_, index) => ({ ...plan, id: (index + 1).toString(16).padStart(32, '0'), state: 'succeeded' as const, created_at: now - index }))
  const recovery = { ...plan, id: 'f'.repeat(32), state: 'recovery_required' as const, created_at: now - 100, backup_created: true, target_start_attempted: false, source_mutation_attempted: true, source_restart_disabled: true, code: 'OPERATIONS_HEALTH_FAILED' }
  const user = renderPanel(async (config) => defaultResponse(config, [...completed, recovery]))
  expect(await screen.findByText('Upgrade recovery needs attention')).toBeInTheDocument()
  const list = screen.getByRole('list', { name: 'Upgrade plan list' })
  expect(within(list).getAllByRole('listitem')).toHaveLength(11)
  const recoveryItem = within(list).getAllByRole('listitem').find((item) => within(item).queryByText('recovery_required'))
  expect(recoveryItem).toBeDefined()
  if (!recoveryItem) throw new Error('Expected recovery plan in the returned plan list')
  await user.click(within(recoveryItem).getByRole('button', { name: 'Upgrade plan details' }))
  expect(within(recoveryItem).getByText('OPERATIONS_HEALTH_FAILED')).toBeInTheDocument()
  expect(within(recoveryItem).getByText('Backup created')).toBeInTheDocument()
  expect(within(recoveryItem).getByText('Target start attempted')).toBeInTheDocument()
  expect(within(recoveryItem).getByText('Source mutation attempted')).toBeInTheDocument()
  expect(within(recoveryItem).getByText('Source auto-restart disabled')).toBeInTheDocument()
})


it('can approve through the real proof dialog after StrictMode replays its mount effects', async () => {
  api.defaults.adapter = async (config) => defaultResponse(config)
  render(<StrictMode><QueryClientProvider client={client}><OperationsUpgradePanel /></QueryClientProvider></StrictMode>)
  const user = userEvent.setup()
  const verification = await openVerification(user)
  await user.type(within(verification).getByLabelText('Authenticator or backup code'), '123456')
  await user.click(within(verification).getByRole('button', { name: 'Verify' }))
  expect(await screen.findByText('Upgrade approved. A trusted operations tool must request execution separately.')).toBeInTheDocument()
})
