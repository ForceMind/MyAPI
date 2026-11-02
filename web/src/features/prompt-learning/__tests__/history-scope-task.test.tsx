import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { PromptLearning } from '../index'

const clients: QueryClient[] = []
const policy = { scope: 'self', enabled: false, generation: 0 }
const emptyPage = { page: 1, page_size: 20, total: 0, items: [] }

function response(data: unknown) {
  return { data: { success: true, data } }
}

function setActor(id: number, sid = 'synthetic-session') {
  useAuthStore.setState((state) => ({
    auth: {
      ...state.auth,
      user: { id, username: `synthetic-${id}`, role: 1 },
      session: {
        sid, current: true, login_method: 'password', ip: '192.0.2.1',
        user_agent: 'Synthetic browser', created_at: 1, last_active_at: 1,
        expires_at: 9999999999,
      },
    },
  }))
}

function renderWorkspace() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  clients.push(client)
  render(<QueryClientProvider client={client}><PromptLearning /></QueryClientProvider>)
}

beforeEach(() => {
  setActor(71)
  vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    if (url.endsWith('prompt-learning')) return response(policy)
    const page = Number(config?.params?.p ?? 1)
    const pageSize = Number(config?.params?.page_size ?? 20)
    return response({ ...emptyPage, page, page_size: pageSize, total: 120 })
  })
  vi.spyOn(api, 'post').mockResolvedValue(response({ cancelled: true }))
  vi.spyOn(api, 'put').mockResolvedValue(response(policy))
})
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
})

describe('prompt history task boundaries', () => {
  it('paginates run and version history independently through existing HTTP page parameters', async () => {
    const user = userEvent.setup()
    renderWorkspace()
    await screen.findByText('No instruction versions yet.')
    const runs = screen.getByRole('navigation', { name: 'Learning run pagination' })
    const versions = screen.getByRole('navigation', { name: 'Instruction version pagination' })
    await waitFor(() => expect(within(runs).getByRole('button', { name: 'Last page' })).toBeEnabled())
    await user.click(within(runs).getByRole('button', { name: 'Last page' }))
    await waitFor(() => expect(api.get).toHaveBeenCalledWith(
      '/api/user/prompt-learning/runs', { params: { p: 6, page_size: 20 } }
    ))
    expect(within(versions).getByRole('button', { name: 'Go to page 1' })).toHaveAttribute('aria-current', 'page')
    await user.selectOptions(within(versions).getByRole('combobox', { name: 'Rows per page' }), '50')
    await waitFor(() => expect(api.get).toHaveBeenCalledWith(
      '/api/user/prompt-learning/versions', { params: { p: 1, page_size: 50 } }
    ))
    await waitFor(() => expect(within(versions).getByRole('button', { name: 'Go to page 2' })).toBeEnabled())
    await user.click(within(versions).getByRole('button', { name: 'Go to page 2' }))
    await waitFor(() => expect(api.get).toHaveBeenCalledWith(
      '/api/user/prompt-learning/versions', { params: { p: 2, page_size: 50 } }
    ))
    expect(api.post).not.toHaveBeenCalled()
    expect(api.put).not.toHaveBeenCalled()
  })

  it('keeps a failed page unknown and explicitly retries without claiming empty history', async () => {
    vi.mocked(api.get).mockImplementation(async (url) => {
      if (url.endsWith('runs')) throw new Error('synthetic offline')
      return response(url.endsWith('prompt-learning') ? policy : emptyPage)
    })
    const user = userEvent.setup()
    renderWorkspace()
    expect(await screen.findByRole('alert')).toHaveTextContent('Failed to load learning runs')
    expect(screen.queryByText('No learning runs yet.')).not.toBeInTheDocument()
    const runs = screen.getByRole('navigation', { name: 'Learning run pagination' })
    expect(within(runs).getByText('Record total unavailable')).toBeVisible()
    expect(within(runs).getByRole('button', { name: 'Last page' })).toBeDisabled()
    vi.mocked(api.get).mockImplementation(async (url) => response(url.endsWith('prompt-learning') ? policy : emptyPage))
    await user.click(within(screen.getByRole('alert')).getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('No learning runs yet.')).toBeVisible()
    expect(within(runs).getByText('Page 1 of 1 · 0 records')).toBeVisible()
  })

  it('does not reuse another session history or draft, including same-user session replacement', async () => {
    vi.mocked(api.get).mockImplementation(async (url) => {
      if (url.endsWith('versions')) return response({ ...emptyPage, total: 1, items: [{
        id: 7, content: 'Old session instructions', content_hash: 'a'.repeat(64), source: 'manual', created_at: 1,
      }] })
      return response(url.endsWith('prompt-learning') ? policy : emptyPage)
    })
    const user = userEvent.setup()
    renderWorkspace()
    await screen.findByText('Old session instructions')
    await user.type(screen.getByLabelText('Instruction content'), 'Old draft')
    vi.mocked(api.get).mockImplementation(async () => new Promise<ReturnType<typeof response>>(() => undefined))
    act(() => setActor(71, 'replacement-session'))
    expect(screen.getByLabelText('Instruction content')).toHaveValue('')
    expect(screen.queryByText('Old session instructions')).not.toBeInTheDocument()
    expect(screen.getByText('Loading instruction versions…')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Save version' })).toBeDisabled()
  })

  it('does not dispatch a queued old-actor save after an identity change in the same event', async () => {
    const user = userEvent.setup()
    renderWorkspace()
    await screen.findByText('Learning is disabled')
    await user.type(screen.getByLabelText('Instruction content'), 'Synthetic draft')
    const save = screen.getByRole('button', { name: 'Save version' })
    await waitFor(() => expect(save).toBeEnabled())
    await act(async () => {
      fireEvent.click(save)
      setActor(72)
    })
    expect(api.post).not.toHaveBeenCalled()
    expect(screen.getByLabelText('Instruction content')).toHaveValue('')
    expect(api.put).not.toHaveBeenCalled()
  })

  it('saves exactly one manual command with the original HTTP fields and no automatic model work', async () => {
    const user = userEvent.setup()
    vi.mocked(api.post).mockResolvedValue(response({
      created: true,
      version: { id: 81, content: 'Synthetic instructions', content_hash: 'a'.repeat(64), source: 'manual', created_at: 1 },
    }))
    renderWorkspace()
    await screen.findByText('Learning is disabled')
    await user.type(screen.getByLabelText('Instruction content'), 'Synthetic instructions')
    await waitFor(() => expect(screen.getByRole('button', { name: 'Save version' })).toBeEnabled())
    await user.click(screen.getByRole('button', { name: 'Save version' }))
    await waitFor(() => expect(api.post).toHaveBeenCalledExactlyOnceWith(
      '/api/user/prompt-learning/versions',
      {
        command_id: expect.stringMatching(/^[0-9a-f-]{36}$/i),
        content: 'Synthetic instructions',
        parent_id: undefined,
      },
      { skipAuthRefresh: true }
    ))
    await waitFor(() => expect(screen.getByLabelText('Instruction content')).toHaveValue(''))
    expect(api.put).not.toHaveBeenCalled()
    expect(api.get).toHaveBeenCalledWith('/api/user/prompt-learning')
  })

})
