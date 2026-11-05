import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { CONNECTION, mountConnections } from './connections-page-fixture'

vi.mock('@/hooks/use-sidebar-config', () => ({ useIsSidebarModuleVisible: () => true }))
const originalAuth = useAuthStore.getState()
const originalAdapter = api.defaults.adapter
const originalMatchMedia = Object.getOwnPropertyDescriptor(window, 'matchMedia')
let teardown: (() => void) | undefined
afterEach(() => {
  teardown?.()
  api.defaults.adapter = originalAdapter
  useAuthStore.setState(originalAuth)
  if (originalMatchMedia) Object.defineProperty(window, 'matchMedia', originalMatchMedia)
  else Reflect.deleteProperty(window, 'matchMedia')
  vi.restoreAllMocks()
})

test.each(['/connections', '/connections/7?tab=routing'])('routing fields at %s edit directly while preserving the location and avoiding provider work', async (entry) => {
  const fixture = await mountConnections({ entry })
  teardown = fixture.teardown
  const group = await screen.findByRole('group', { name: 'Weight' })
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, data) => {
    fixture.state.items = [{ ...CONNECTION, ...(data as { weight: number }) }]
    return { data: { success: true } }
  })
  const location = fixture.router.state.location.href
  fireEvent.click(within(group).getByRole('button', { name: 'Increment' }))
  expect(put).not.toHaveBeenCalled()
  await act(async () => fireEvent.blur(within(group).getByRole('button', { name: '1' }), { relatedTarget: document.body }))
  await waitFor(() => expect(within(group).getByText('Saved')).toBeVisible())
  expect(put).toHaveBeenCalledExactlyOnceWith('/api/channel/', { id: 7, weight: 1 }, expect.objectContaining({ skipAuthRefresh: true }))
  expect(fixture.router.state.location.href).toBe(location)
  expect(fixture.requests.some((request) => request.method !== 'get' || /\/test\//.test(request.path))).toBe(false)
})

test.each(['/connections', '/connections/7?tab=routing'])('fresh read-only access at %s keeps routing values visible and prevents edits', async (entry) => {
  const fixture = await mountConnections({ entry, write: false })
  teardown = fixture.teardown
  const group = await screen.findByRole('group', { name: 'Weight' })
  expect(within(group).getByRole('button', { name: '0' })).toBeDisabled()
  expect(within(group).getByRole('button', { name: 'Increment' })).toBeDisabled()
  expect(fixture.requests.every((request) => request.method === 'get')).toBe(true)
})
