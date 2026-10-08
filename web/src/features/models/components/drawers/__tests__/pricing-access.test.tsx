import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { ModelMutateDrawer } from '../model-mutate-drawer'

const clients: QueryClient[] = []
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.getState().auth.setUser(null)
  vi.restoreAllMocks()
})

function renderDrawer(role: number, open: boolean) {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'synthetic-admin', role })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  return render(
    <QueryClientProvider client={client}>
      <ModelMutateDrawer open={open} onOpenChange={() => {}} />
    </QueryClientProvider>
  )
}

describe('model drawer pricing access', () => {
  it('does not fetch system options for a closed administrator drawer', () => {
    const get = vi
      .spyOn(api, 'get')
      .mockResolvedValue({ data: { success: true, data: [] } })
    renderDrawer(ROLE.ADMIN, false)
    expect(get).not.toHaveBeenCalled()
  })

  it('does not offer Root pricing controls to an administrator editing metadata', async () => {
    const get = vi
      .spyOn(api, 'get')
      .mockResolvedValue({ data: { success: true, data: { items: [] } } })
    renderDrawer(ROLE.ADMIN, true)
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(screen.queryByText('Pricing Configuration')).not.toBeInTheDocument()
    expect(get.mock.calls.some(([path]) => path === '/api/option/')).toBe(false)
  })

  it('shows an unavailable state rather than editable pricing defaults when Root cannot load options', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path) => {
      if (path === '/api/option/') throw new Error('offline')
      return { data: { success: true, data: { items: [] } } }
    })
    renderDrawer(ROLE.SUPER_ADMIN, true)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Unable to load settings'
    )
    expect(screen.queryByText('Pricing mode')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
  })

  it('retains pricing controls for Root after a successful options read', async () => {
    const get = vi.spyOn(api, 'get').mockImplementation(async (path) => ({
      data: {
        success: true,
        data: path === '/api/option/' ? [] : { items: [] },
      },
    }))
    renderDrawer(ROLE.SUPER_ADMIN, true)
    expect(await screen.findByText('Pricing Configuration')).toBeInTheDocument()
    expect(screen.getByText('Pricing mode')).toBeInTheDocument()
    expect(
      get.mock.calls.filter(([path]) => path === '/api/option/')
    ).toHaveLength(1)
  })
})
