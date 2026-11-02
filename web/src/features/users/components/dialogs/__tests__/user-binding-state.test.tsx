import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { UserBindingDialog } from '../user-binding-dialog'

const originalAuth = useAuthStore.getState()
afterEach(() => { cleanup(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

it('retains the independent email unbind confirmation after the unbind request fails', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.SUPER_ADMIN, username: 'synthetic-root' })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/user/22') return { data: { success: true, data: { id: 22, username: 'synthetic-user', email: 'synthetic@example.invalid' } } }
    if (url === '/api/user/22/oauth/bindings') return { data: { success: true, data: [] } }
    if (url === '/api/status') return { data: { success: true, data: {} } }
    throw new Error(`Unexpected GET ${url}`)
  })
  const remove = vi.spyOn(api, 'delete').mockRejectedValue(new Error('Synthetic unbind failure'))
  render(<UserBindingDialog open onOpenChange={vi.fn()} userId={22} />)
  await screen.findByText('synthetic@example.invalid')
  const button = within(screen.getByRole('dialog')).getAllByRole('button').filter((item) => item.textContent?.trim() === '' && item.getAttribute('data-slot') !== 'dialog-close').at(-1)
  if (!button) throw new Error('Existing binding removal action is missing')
  const user = userEvent.setup()
  await user.click(button)
  const confirm = await screen.findByRole('alertdialog')
  await user.click(within(confirm).getByRole('button', { name: 'Confirm Unbind' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Unbind failed')
  expect(screen.getByRole('alertdialog')).toHaveTextContent('Email')
  expect(remove).toHaveBeenCalledTimes(1)
})
