import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { UserAuthForm } from '../sign-in/components/user-auth-form'

const redirects = vi.hoisted(() => ({ handleLoginSuccess: vi.fn(), redirectTo2FA: vi.fn() }))
vi.mock('../hooks/use-auth-redirect', () => ({ useAuthRedirect: () => redirects }))
vi.mock('@/lib/passkey', () => ({ isPasskeySupported: async () => true, prepareCredentialRequestOptions: () => ({}), buildAssertionResult: () => ({}) }))
const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
const originalCredentials = Object.getOwnPropertyDescriptor(navigator, 'credentials')
const clients: QueryClient[] = []
const configured = { password_login_enabled: true, passkey_login: true, wechat_login: true, github_oauth: true, github_client_id: 'synthetic-client', user_agreement_enabled: true }

beforeEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.reset('idle')
  Object.defineProperty(navigator, 'credentials', { configurable: true, value: { get: vi.fn().mockResolvedValue(null) } })
  vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true, data: { require_2fa: true, flow_token: 'synthetic-flow' } } })
  redirects.handleLoginSuccess.mockClear()
  redirects.redirectTo2FA.mockClear()
})
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.setState(originalAuth)
  useSystemConfigStore.setState(originalConfig)
  if (originalCredentials) Object.defineProperty(navigator, 'credentials', originalCredentials)
  else Reflect.deleteProperty(navigator, 'credentials')
  localStorage.clear()
  vi.restoreAllMocks()
})

async function mount() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  clients.push(client)
  await renderListRoute({ client, path: '/sign-in/', element: <UserAuthForm /> })
  return client
}

async function enterCredentials() {
  const user = userEvent.setup()
  await user.type(screen.getByRole('textbox', { name: 'Username or Email' }), 'synthetic-user')
  await user.type(screen.getByLabelText('Password'), 'synthetic-password')
  return user
}

it.each([false, true])('pending sign-in settings (cached placeholder=%s) preserve fields and block every authentication entry and native submit', async (placeholder) => {
  if (placeholder) localStorage.setItem('status', JSON.stringify({ ...configured, user_agreement_enabled: false }))
  let release = () => {}
  const pending = new Promise<void>((resolve) => { release = resolve })
  vi.spyOn(api, 'get').mockImplementation(async () => { await pending; return { data: { success: true, data: configured } } })
  await mount()
  const user = await enterCredentials()
  expect(screen.getByRole('button', { name: 'Sign in' })).toBeDisabled()
  const passkey = screen.queryByRole('button', { name: 'Sign in with Passkey' })
  if (passkey) { expect(passkey).toBeDisabled(); fireEvent.click(passkey) }
  const wechat = screen.queryByRole('button', { name: 'Continue with WeChat' })
  if (wechat) { expect(wechat).toBeDisabled(); fireEvent.click(wechat) }
  const oauth = screen.queryByRole('button', { name: 'Continue with GitHub' })
  if (oauth) { expect(oauth).toBeDisabled(); fireEvent.click(oauth) }
  const pendingForm = screen.getByRole('button', { name: 'Sign in' }).closest('form')
  if (!pendingForm) throw new Error('Expected sign-in form while public settings are pending')
  await act(async () => { fireEvent.submit(pendingForm) })
  expect(api.post).not.toHaveBeenCalled()
  await act(async () => release())
  await waitFor(() => expect(screen.queryByText('Loading...')).not.toBeInTheDocument())
  expect(screen.getByRole('textbox', { name: 'Username or Email' })).toHaveValue('synthetic-user')
  expect(screen.getByLabelText('Password')).toHaveValue('synthetic-password')
  expect(screen.getByRole('button', { name: 'Sign in' })).toBeDisabled()
  const confirmedForm = screen.getByRole('button', { name: 'Sign in' }).closest('form')
  if (!confirmedForm) throw new Error('Expected sign-in form after public settings are confirmed')
  await act(async () => { fireEvent.submit(confirmedForm) })
  expect(api.post).not.toHaveBeenCalled()
  await user.click(screen.getByRole('checkbox'))
  await user.click(screen.getByRole('button', { name: 'Sign in' }))
  await waitFor(() => expect(redirects.redirectTo2FA).toHaveBeenCalledOnce())
  expect(api.post).toHaveBeenCalledExactlyOnceWith('/api/user/login?turnstile=', { username: 'synthetic-user', password: 'synthetic-password' }, { skipAuthRefresh: true })
})

it('a failed public settings read keeps credentials and retries only the status GET before requiring legal consent', async () => {
  const get = vi.spyOn(api, 'get').mockRejectedValue(new Error('Synthetic public settings unavailable'))
  await mount()
  await screen.findByRole('alert')
  const user = await enterCredentials()
  const failedForm = screen.getByRole('button', { name: 'Sign in' }).closest('form')
  if (!failedForm) throw new Error('Expected sign-in form after public settings fail')
  await act(async () => { fireEvent.submit(failedForm) })
  expect(api.post).not.toHaveBeenCalled()
  get.mockResolvedValue({ data: { success: true, data: configured } })
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument())
  expect(get.mock.calls.map(([url]) => url)).toEqual(['/api/status', '/api/status'])
  expect(screen.getByLabelText('Password')).toHaveValue('synthetic-password')
  expect(screen.getByRole('button', { name: 'Sign in' })).toBeDisabled()
  expect(api.post).not.toHaveBeenCalled()
})

it('an open WeChat task preserves its code when settings fail and retries only the public status read', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: configured } })
  const client = await mount()
  await screen.findByRole('checkbox')
  const user = await enterCredentials()
  await user.click(screen.getByRole('checkbox'))
  await user.click(screen.getByRole('button', { name: 'Continue with WeChat' }))
  const dialog = await screen.findByRole('dialog', { name: 'WeChat sign in' })
  const task = within(dialog)
  await user.type(task.getByRole('textbox', { name: 'Verification code' }), 'synthetic-wechat-code')
  get.mockRejectedValue(new Error('Synthetic public settings unavailable'))
  await act(async () => { await client.invalidateQueries({ queryKey: ['status'], exact: true }) })
  expect(task.getByRole('alert')).toHaveTextContent('Unable to load settings')
  expect(task.getByRole('button', { name: 'Confirm' })).toBeDisabled()
  expect(task.getByRole('textbox', { name: 'Verification code' })).toHaveValue('synthetic-wechat-code')
  fireEvent.click(task.getByRole('button', { name: 'Confirm' }))
  expect(api.post).not.toHaveBeenCalled()
  get.mockResolvedValue({ data: { success: true, data: configured } })
  await user.click(task.getByRole('button', { name: 'Retry' }))
  await waitFor(() => expect(task.queryByRole('alert')).not.toBeInTheDocument())
  expect(task.getByRole('button', { name: 'Confirm' })).toBeEnabled()
  expect(task.getByRole('textbox', { name: 'Verification code' })).toHaveValue('synthetic-wechat-code')
  expect(screen.getByLabelText('Password')).toHaveValue('synthetic-password')
  expect(get.mock.calls.map(([url]) => url)).toEqual(['/api/status', '/api/status', '/api/status'])
  expect(api.post).not.toHaveBeenCalled()
})
