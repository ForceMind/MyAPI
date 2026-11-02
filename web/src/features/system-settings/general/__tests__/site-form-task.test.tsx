import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError } from 'axios'
import { useState } from 'react'
import { toast, Toaster } from 'sonner'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { BasicAuthSection } from '../../auth/basic-auth-section'
import { SettingsPageProvider } from '../../components/settings-page-context'
import { SystemInfoSection } from '../system-info-section'
import { SiteSettings } from '../../site'

const originalAuth = useAuthStore.getState()
const siteDefaults = { SystemName: 'saved-site-name', ServerAddress: 'https://synthetic.invalid', Logo: '', Footer: '', About: '', HomePageContent: '', legal: { user_agreement: '', privacy_policy: '' } }
const authDefaults = { PasswordLoginEnabled: true, PasswordRegisterEnabled: true, EmailVerificationEnabled: false, RegisterEnabled: true, EmailDomainRestrictionEnabled: false, EmailAliasRestrictionEnabled: false, EmailDomainWhitelist: '' }
let client: QueryClient

beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.SUPER_ADMIN, username: 'synthetic-root' })
  vi.spyOn(api, 'get').mockRejectedValue(new Error('Unexpected setting read'))
})
afterEach(() => { act(() => toast.dismiss()); cleanup(); client.clear(); useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })

function SettingsFixture(props: { basic?: boolean }) {
  const [defaults, setDefaults] = useState(siteDefaults)
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  const [status, setStatus] = useState<HTMLSpanElement | null>(null)
  return <>
    <button type='button' onClick={() => setDefaults((previous) => ({ ...previous, SystemName: 'fresh-server-site-name' }))}>Receive saved server snapshot</button>
    <output aria-label='Saved server System Name'>{defaults.SystemName}</output>
    <span ref={setStatus} />
    <div ref={setActions} />
    <SettingsPageProvider actionsContainer={actions} titleStatusContainer={status} suppressSectionHeader={false}>
      {props.basic ? <BasicAuthSection defaultValues={authDefaults} /> : <SystemInfoSection defaultValues={defaults} />}
    </SettingsPageProvider>
    <Toaster />
  </>
}

async function renderSettings(basic = false) {
  await renderListRoute({ client, path: basic ? '/system-settings/auth/$section' : '/system-settings/site/$section', entry: basic ? '/system-settings/auth/basic-auth' : '/system-settings/site/system-info', element: <SettingsFixture basic={basic} /> })
  await screen.findByRole('button', { name: 'Save Changes' })
}

it('keeps a visible System Name draft when a newly received saved server snapshot changes the same field', async () => {
  const put = vi.spyOn(api, 'put')
  await renderSettings()
  fireEvent.change(screen.getByLabelText('System Name'), { target: { value: 'unsaved-site-name' } })
  await userEvent.setup().click(screen.getByRole('button', { name: 'Receive saved server snapshot' }))
  expect(screen.getByLabelText('Saved server System Name')).toHaveTextContent('fresh-server-site-name')
  expect(screen.getByLabelText('System Name')).toHaveValue('unsaved-site-name')
  expect(screen.getByText('Unsaved changes')).toBeVisible()
  expect(put).not.toHaveBeenCalled()
})

it('keeps text entered after a site save started and leaves it dirty after that save is acknowledged', async () => {
  let complete: ((value: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { complete = resolve })
  const put = vi.spyOn(api, 'put').mockReturnValue(pending)
  await renderSettings()
  fireEvent.change(screen.getByLabelText('System Name'), { target: { value: 'submitted-site-name' } })
  await userEvent.setup().click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  fireEvent.change(screen.getByLabelText('System Name'), { target: { value: 'edited-while-saving' } })
  await act(async () => { complete?.({ data: { success: true } }); await pending })
  expect(screen.getByLabelText('System Name')).toHaveValue('edited-while-saving')
  expect(screen.getByText('Unsaved changes')).toBeVisible()
  expect(put.mock.calls[0]?.slice(0, 2)).toEqual(['/api/option/', { key: 'SystemName', value: 'submitted-site-name' }])
})

it('clears the former actor site draft when a new login identity starts editing the same saved settings', async () => {
  await renderSettings()
  fireEvent.change(screen.getByLabelText('System Name'), { target: { value: 'previous-actor-secret-draft' } })
  await act(async () => { useAuthStore.getState().auth.setUser({ id: 2, role: ROLE.SUPER_ADMIN, username: 'synthetic-new-root' }) })
  expect(screen.getByLabelText('System Name')).toHaveValue(siteDefaults.SystemName)
  expect(screen.queryByText('Unsaved changes')).not.toBeInTheDocument()
})

it('does not acknowledge a business-rejected site save and keeps its draft dirty with a readable error', async () => {
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: false, message: 'Synthetic option rejected' } })
  await renderSettings()
  fireEvent.change(screen.getByLabelText('System Name'), { target: { value: 'rejected-site-name' } })
  await userEvent.setup().click(screen.getByRole('button', { name: 'Save Changes' }))
  await screen.findByText('Synthetic option rejected')
  await screen.findByRole('button', { name: 'Save Changes' })
  expect(screen.getByLabelText('System Name')).toHaveValue('rejected-site-name')
  expect(screen.getByText('Unsaved changes')).toBeVisible()
})

it.each(['page Save', 'native submit'])('reports a basic-auth HTTP failure from %s without losing the visible draft or leaking an unhandled rejection', async (path) => {
  const put = vi.spyOn(api, 'put').mockRejectedValue(new AxiosError('Synthetic HTTP option failure', 'ERR_NETWORK'))
  await renderSettings(true)
  const input = screen.getByLabelText('Email Domain Whitelist')
  fireEvent.change(input, { target: { value: 'synthetic.invalid' } })
  if (path === 'page Save') await userEvent.setup().click(screen.getByRole('button', { name: 'Save Changes' }))
  else {
    const form = input.closest('form')
    if (!form) throw new Error('Actual settings form is missing')
    fireEvent.submit(form)
  }
  await waitFor(() => expect(screen.getByText('Synthetic HTTP option failure')).toBeVisible())
  expect(input).toHaveValue('synthetic.invalid')
  expect(put).toHaveBeenCalledTimes(1)
})

it('stops an old actor multi-key save before its next write and lets the newly mounted settings task save explicitly', async () => {
  const saved = { SystemName: siteDefaults.SystemName, Footer: 'saved-footer' }
  vi.mocked(api.get).mockImplementation(async (url) => {
    if (url === '/api/option/') return { data: { success: true, data: Object.entries(saved).map(([key, value]) => ({ key, value })) } }
    if (url === '/api/status') return { data: { success: true, data: {} } }
    throw new Error(`Unexpected GET ${url}`)
  })
  let complete: ((value: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { complete = resolve })
  const put = vi.spyOn(api, 'put').mockReturnValueOnce(pending).mockImplementation(async (_url, request) => {
    const update = request as { key: keyof typeof saved; value: string }
    saved[update.key] = update.value
    return { data: { success: true } }
  })
  await renderListRoute({ client, path: '/system-settings/site/$section', entry: '/system-settings/site/system-info', element: <SiteSettings /> })
  await screen.findByDisplayValue(siteDefaults.SystemName)
  fireEvent.change(screen.getByLabelText('System Name'), { target: { value: 'submitted-site-name' } })
  fireEvent.change(screen.getByLabelText('Footer'), { target: { value: 'old-actor-unsent-footer' } })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  await act(async () => {
    useAuthStore.getState().auth.setUser({ id: 2, role: ROLE.SUPER_ADMIN, username: 'synthetic-new-root' })
    saved.SystemName = 'submitted-site-name'
    complete?.({ data: { success: true } })
    await pending
  })
  await waitFor(() => expect(screen.getByLabelText('Footer')).toHaveValue('saved-footer'))
  expect(put).toHaveBeenCalledTimes(1)
  fireEvent.change(screen.getByLabelText('Footer'), { target: { value: 'new-actor-footer' } })
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(2))
  expect(put.mock.calls.map((call) => call[1])).toEqual([
    { key: 'SystemName', value: 'submitted-site-name' },
    { key: 'Footer', value: 'new-actor-footer' },
  ])
})
