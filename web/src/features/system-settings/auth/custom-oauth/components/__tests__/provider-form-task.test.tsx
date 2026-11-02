import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect, useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { CustomOAuthProvider } from '../../types'
import { ProviderFormDialog } from '../provider-form-dialog'
import { ProviderTable } from '../provider-table'
import { CustomOAuthSection } from '../../custom-oauth-section'

const provider: CustomOAuthProvider = { id: 7, name: 'Synthetic provider', slug: 'synthetic', icon: '', enabled: false, client_id: 'synthetic-client', client_secret: '', authorization_endpoint: 'https://synthetic.invalid/authorize', token_endpoint: 'https://synthetic.invalid/token', user_info_endpoint: 'https://synthetic.invalid/userinfo', scopes: 'openid', user_id_field: 'sub', username_field: '', display_name_field: '', email_field: '', well_known: '', auth_style: 0, access_policy: '{"enabled":false,"count":0}', access_denied_message: '' }
const originalAuth = useAuthStore.getState()
let client: QueryClient
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.SUPER_ADMIN, username: 'synthetic-root' })
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

it('preserves a provider draft when a refreshed same-provider object changes its saved name', async () => {
  let refresh: ((value: CustomOAuthProvider) => void) | undefined
  function RefreshedProvider() {
    const [value, setValue] = useState(provider)
    useEffect(() => { refresh = setValue; return () => { refresh = undefined } }, [])
    return <ProviderFormDialog open provider={value} serverAddress='https://synthetic.invalid' onOpenChange={vi.fn()} />
  }
  await renderListRoute({ client, path: '/system-settings/auth/custom-oauth', element: <RefreshedProvider /> })
  await screen.findByDisplayValue(provider.name)
  fireEvent.change(screen.getByLabelText('Provider Name'), { target: { value: 'unsaved-provider-name' } })
  await act(async () => { refresh?.({ ...provider, name: 'fresh-saved-provider-name' }) })
  expect(screen.getByLabelText('Provider Name')).toHaveValue('unsaved-provider-name')
})

function ControlledProvider() {
  const [open, setOpen] = useState(true)
  return <ProviderFormDialog open={open} onOpenChange={setOpen} provider={provider} serverAddress='https://synthetic.invalid' />
}

it('keeps the provider task open during update and preserves disabled, auto-auth zero and raw policy values in the original payload', async () => {
  let complete: ((value: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { complete = resolve })
  const put = vi.spyOn(api, 'put').mockReturnValue(pending)
  await renderListRoute({ client, path: '/system-settings/auth/custom-oauth', element: <ControlledProvider /> })
  await screen.findByDisplayValue(provider.name)
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Update Provider' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  try {
    await user.keyboard('{Escape}')
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(put.mock.calls[0]?.slice(0, 2)).toEqual(['/api/custom-oauth-provider/7', {
      name: provider.name, slug: provider.slug, icon: provider.icon, enabled: false,
      client_id: provider.client_id, client_secret: '', authorization_endpoint: provider.authorization_endpoint,
      token_endpoint: provider.token_endpoint, user_info_endpoint: provider.user_info_endpoint, scopes: provider.scopes,
      user_id_field: provider.user_id_field, username_field: '', display_name_field: '', email_field: '', well_known: '',
      auth_style: 0, access_policy: provider.access_policy, access_denied_message: '',
    }])
  } finally {
    await act(async () => { complete?.({ data: { success: true } }); await pending })
  }
})


it('keeps an independent provider deletion confirmation open with its error after business rejection', async () => {
  const remove = vi.spyOn(api, 'delete').mockResolvedValue({ data: { success: false, message: 'Synthetic provider deletion rejected' } })
  await renderListRoute({ client, path: '/system-settings/auth/custom-oauth', element: <ProviderTable providers={[provider]} onCreate={vi.fn()} onEdit={vi.fn()} /> })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Open menu' }))
  await user.click(screen.getByRole('menuitem', { name: 'Delete' }))
  await user.click(screen.getByRole('button', { name: 'Delete', exact: true }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic provider deletion rejected')
  expect(screen.getByRole('alertdialog')).toBeInTheDocument()
  expect(remove).toHaveBeenCalledTimes(1)
})

it('shows a failed provider read as an error instead of a confirmed empty provider list', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: false, message: 'Synthetic provider read rejected' } })
  await renderListRoute({ client, path: '/system-settings/auth/custom-oauth', element: <CustomOAuthSection serverAddress='https://synthetic.invalid' /> })
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic provider read rejected')
  expect(screen.queryByText('No custom OAuth providers configured yet.')).not.toBeInTheDocument()
})
