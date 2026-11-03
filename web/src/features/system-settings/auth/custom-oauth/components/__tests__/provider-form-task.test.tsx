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
  await user.click(screen.getByRole('button', { name: 'Delete' }))
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


it('keeps provider advanced drafts and raw policy while local section expansion neither discovers nor saves', async () => {
  const get = vi.spyOn(api, 'get').mockRejectedValue(new Error('Unexpected provider read'))
  const post = vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected provider creation'))
  const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: false, message: 'Synthetic provider save held' } })
  await renderListRoute({ client, path: '/system-settings/auth/custom-oauth', element: <ProviderFormDialog open provider={provider} serverAddress='https://synthetic.invalid' onOpenChange={vi.fn()} /> })
  await screen.findByDisplayValue(provider.name)
  expect(screen.getByText('Login policy: custom rules configured')).toBeVisible()
  for (const label of ['Endpoints', 'Field Mapping', 'Advanced']) {
    expect(screen.getByRole('button', { name: label })).toHaveAttribute('aria-expanded', 'false')
  }
  expect(screen.getByLabelText('Client ID')).toBeVisible()
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Advanced' }))
  const policy = screen.getByLabelText('Access Policy (JSON)')
  const raw = '{ "custom": [null, 0, false], "opaque": "retained" }'
  fireEvent.input(policy, { target: { value: raw } })
  await user.click(screen.getByRole('button', { name: 'Advanced' }))
  expect(policy).not.toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Advanced' }))
  expect(policy).toHaveValue(raw)
  const endpoints = screen.getByRole('button', { name: 'Endpoints' })
  endpoints.focus()
  await user.keyboard('{Enter}')
  expect(screen.getByLabelText('Authorization Endpoint')).toHaveValue(provider.authorization_endpoint)
  expect(get).not.toHaveBeenCalled()
  expect(post).not.toHaveBeenCalled()
  expect(put).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Update Provider' }))
  await screen.findByText('Synthetic provider save held')
  expect(put).toHaveBeenCalledTimes(1)
  expect(put.mock.calls[0]?.[1]).toMatchObject({ access_policy: raw, enabled: false, auth_style: 0, authorization_endpoint: provider.authorization_endpoint, user_id_field: 'sub' })
})

it('opens and focuses an invalid endpoint without sending the provider form or hiding its original validation message', async () => {
  const put = vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected invalid provider write'))
  const post = vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected invalid provider creation'))
  const invalid = { ...provider, authorization_endpoint: '' }
  await renderListRoute({ client, path: '/system-settings/auth/custom-oauth', element: <ProviderFormDialog open provider={invalid} serverAddress='https://synthetic.invalid' onOpenChange={vi.fn()} /> })
  await screen.findByDisplayValue(provider.name)
  expect(screen.getByRole('button', { name: 'Endpoints' })).toHaveAttribute('aria-expanded', 'false')
  await userEvent.setup().click(screen.getByRole('button', { name: 'Update Provider' }))
  expect(await screen.findByText('Authorization endpoint is required')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Endpoints' })).toHaveAttribute('aria-expanded', 'true')
  await waitFor(() => expect(screen.getByLabelText('Authorization Endpoint')).toHaveFocus())
  expect(put).not.toHaveBeenCalled()
  expect(post).not.toHaveBeenCalled()
})


it('keeps malformed raw policy visible and invalid without revealing its client secret in section summaries or changing save validation', async () => {
  const get = vi.spyOn(api, 'get').mockRejectedValue(new Error('Unexpected discovery'))
  const post = vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected provider creation'))
  const put = vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected provider write'))
  const secret = 'synthetic-secret-not-for-a-summary'
  const value = { ...provider, access_policy: '{broken', client_secret: secret }
  await renderListRoute({ client, path: '/system-settings/auth/custom-oauth', element: <ProviderFormDialog open provider={value} serverAddress='https://synthetic.invalid' onOpenChange={vi.fn()} /> })
  await screen.findByDisplayValue(provider.name)
  const advanced = screen.getByRole('button', { name: 'Advanced' })
  expect(advanced).toHaveAttribute('aria-expanded', 'true')
  const policy = screen.getByLabelText('Access Policy (JSON)')
  expect(policy).toBeVisible()
  expect(policy).toHaveValue('{broken')
  await waitFor(() => expect(policy).toHaveAttribute('aria-invalid', 'true'))
  expect(screen.getByText('Login policy JSON is invalid. Review the original draft.')).toBeVisible()
  const user = userEvent.setup()
  await user.click(advanced)
  expect(advanced).toHaveAttribute('aria-expanded', 'true')
  fireEvent.input(policy, { target: { value: '{ "opaque": 0 }' } })
  await waitFor(() => expect(policy).not.toHaveAttribute('aria-invalid'))
  expect(advanced).toHaveAttribute('aria-expanded', 'true')
  await user.click(advanced)
  expect(policy).not.toBeVisible()
  for (const name of ['Endpoints', 'Field Mapping', 'Advanced']) expect(screen.getByRole('button', { name })).not.toHaveTextContent(secret)
  expect(screen.getByLabelText('Client Secret')).toHaveAttribute('type', 'password')
  expect(get).not.toHaveBeenCalled()
  expect(post).not.toHaveBeenCalled()
  expect(put).not.toHaveBeenCalled()
})
