import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen } from '@testing-library/react'
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

function renderDrawer(
  role: number,
  open: boolean,
  currentRow?: Parameters<typeof ModelMutateDrawer>[0]['currentRow']
) {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'synthetic-admin', role })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  return render(
    <QueryClientProvider client={client}>
      <ModelMutateDrawer
        open={open}
        onOpenChange={() => {}}
        currentRow={currentRow}
      />
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

  it('preserves metadata and pricing drafts across a failed background read and retry', async () => {
    let unavailable = false
    const model = {
      id: 7,
      model_name: 'saved-model',
      description: 'saved description',
      status: 1,
      sync_official: 1,
      created_time: 0,
      updated_time: 0,
      name_rule: 0,
    }
    vi.spyOn(api, 'get').mockImplementation(async (path) => {
      if (path === '/api/models/7') {
        return { data: { success: true, data: model } }
      }
      if (path === '/api/option/') {
        if (unavailable) throw new Error('offline')
        return {
          data: {
            success: true,
            data: [{ key: 'ModelRatio', value: '{"saved-model":0.5}' }],
          },
        }
      }
      return { data: { success: true, data: { items: [] } } }
    })
    renderDrawer(ROLE.SUPER_ADMIN, true, model)
    await screen.findByDisplayValue('saved-model')
    await screen.findByDisplayValue('0.5')
    fireEvent.change(screen.getByLabelText('Model Name *'), {
      target: { value: 'draft-model' },
    })
    fireEvent.change(screen.getByLabelText('Description'), {
      target: { value: 'unsaved description' },
    })
    fireEvent.change(screen.getByLabelText('Model ratio'), {
      target: { value: '1.25' },
    })
    unavailable = true
    await act(async () => {
      await clients[0].invalidateQueries({ queryKey: ['system-options'] })
    })
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Unable to load settings'
    )
    expect(screen.getByLabelText('Model Name *')).toHaveValue('draft-model')
    expect(screen.getByLabelText('Description')).toHaveValue(
      'unsaved description'
    )
    expect(screen.getByRole('button', { name: 'Update Model' })).toBeDisabled()
    unavailable = false
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByLabelText('Model ratio')).toHaveValue('1.25')
    expect(screen.getByLabelText('Model Name *')).toHaveValue('draft-model')
    expect(screen.getByRole('button', { name: 'Update Model' })).toBeEnabled()
  })

  it('fills first available pricing without replacing metadata typed during an initial failure', async () => {
    let unavailable = true
    vi.spyOn(api, 'get').mockImplementation(async (path) => {
      if (path === '/api/option/') {
        if (unavailable) throw new Error('offline')
        return { data: { success: true, data: [] } }
      }
      return { data: { success: true, data: { items: [] } } }
    })
    renderDrawer(ROLE.SUPER_ADMIN, true)
    await screen.findByRole('alert')
    fireEvent.change(screen.getByLabelText('Model Name *'), {
      target: { value: 'first-draft' },
    })
    fireEvent.change(screen.getByLabelText('Description'), {
      target: { value: 'typed while unavailable' },
    })
    unavailable = false
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await screen.findByText('Pricing Configuration')
    expect(screen.getByLabelText('Model Name *')).toHaveValue('first-draft')
    expect(screen.getByLabelText('Description')).toHaveValue(
      'typed while unavailable'
    )
  })

  it('starts a clean form after closing and reopening instead of retaining the previous draft', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (path) => ({
      data: {
        success: true,
        data: path === '/api/option/' ? [] : { items: [] },
      },
    }))
    const view = renderDrawer(ROLE.SUPER_ADMIN, true)
    await screen.findByText('Pricing Configuration')
    fireEvent.change(screen.getByLabelText('Model Name *'), {
      target: { value: 'discarded-draft' },
    })
    fireEvent.change(screen.getByLabelText('Model ratio'), {
      target: { value: '1.25' },
    })
    view.rerender(
      <QueryClientProvider client={clients[0]}>
        <ModelMutateDrawer open={false} onOpenChange={() => {}} />
      </QueryClientProvider>
    )
    view.rerender(
      <QueryClientProvider client={clients[0]}>
        <ModelMutateDrawer open onOpenChange={() => {}} />
      </QueryClientProvider>
    )
    expect(await screen.findByLabelText('Model Name *')).toHaveValue('')
    expect(screen.getByLabelText('Model ratio')).toHaveValue('')
  })
})
