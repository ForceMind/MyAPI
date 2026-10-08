import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { PromptLearning } from '../index'

const policy = { scope: 'self', enabled: false, generation: 0 }
const emptyPage = { page: 1, page_size: 20, total: 0, items: [] }
const pendingRunPage = {
  ...emptyPage,
  total: 1,
  items: [
    {
      id: 8,
      state: 'pending',
      sample_count: 4,
      model_ref: 'synthetic-model',
      template_version: 'prompt-learning-v1',
      created_at: 1,
      updated_at: 2,
    },
  ],
}
const clients: QueryClient[] = []

function renderWorkspace() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <PromptLearning />
    </QueryClientProvider>
  )
  return client
}

function successfulResponse(data: unknown) {
  return { data: { success: true, data } }
}

describe('prompt learning failure recovery', () => {
  beforeEach(() => {
    vi.spyOn(api, 'get').mockImplementation(async (url) =>
      successfulResponse(url.endsWith('prompt-learning') ? policy : emptyPage)
    )
    vi.spyOn(api, 'put').mockResolvedValue(successfulResponse(policy))
    vi.spyOn(api, 'post').mockResolvedValue(
      successfulResponse({ cancelled: true })
    )
  })

  afterEach(() => {
    clients.splice(0).forEach((client) => client.clear())
  })

  it('does not claim learning is disabled or allow writes while policy is loading', async () => {
    let resolvePolicy!: (value: ReturnType<typeof successfulResponse>) => void
    const request = new Promise<ReturnType<typeof successfulResponse>>(
      (resolve) => {
        resolvePolicy = resolve
      }
    )
    vi.mocked(api.get).mockImplementation(async (url) =>
      url.endsWith('prompt-learning') ? request : successfulResponse(emptyPage)
    )
    const user = userEvent.setup()
    renderWorkspace()

    expect(screen.getByText('Loading learning settings…')).toBeInTheDocument()
    expect(screen.queryByText('Learning is disabled')).not.toBeInTheDocument()
    expect(
      screen.getByRole('switch', { name: 'Enable learning' })
    ).toHaveAttribute('aria-disabled', 'true')
    await user.type(
      screen.getByLabelText('Instruction content'),
      'Draft instructions'
    )
    expect(screen.getByRole('button', { name: 'Save version' })).toBeDisabled()

    resolvePolicy(successfulResponse(policy))
    expect(await screen.findByText('Learning is disabled')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save version' })).toBeEnabled()
  })

  it.each([
    ['runs', 'Loading learning runs…', 'No learning runs yet.'],
    [
      'versions',
      'Loading instruction versions…',
      'No instruction versions yet.',
    ],
  ])(
    'keeps %s loading separate from successful empty histories',
    async (endpoint, loading, empty) => {
      let resolvePage!: (value: ReturnType<typeof successfulResponse>) => void
      const request = new Promise<ReturnType<typeof successfulResponse>>(
        (resolve) => {
          resolvePage = resolve
        }
      )
      vi.mocked(api.get).mockImplementation(async (url) => {
        if (url.endsWith(endpoint)) return request
        return successfulResponse(
          url.endsWith('prompt-learning') ? policy : emptyPage
        )
      })
      renderWorkspace()

      expect(screen.getByText(loading)).toHaveAttribute('role', 'status')
      expect(screen.queryByText(empty)).not.toBeInTheDocument()
      expect(
        await screen.findByText('Learning is disabled')
      ).toBeInTheDocument()
      resolvePage(successfulResponse(emptyPage))
      expect(await screen.findByText(empty)).toBeInTheDocument()
      expect(screen.queryByText(loading)).not.toBeInTheDocument()
    }
  )

  it.each(['transport', 'unsuccessful', 'missing'])(
    'keeps writes disabled when policy response is %s instead of displaying disabled authorization',
    async (failure) => {
      vi.mocked(api.get).mockImplementation(async (url) => {
        if (url.endsWith('prompt-learning')) {
          if (failure === 'transport') throw new Error('unavailable')
          if (failure === 'unsuccessful') return { data: { success: false } }
          return successfulResponse(null)
        }
        return successfulResponse(
          url.endsWith('runs') ? pendingRunPage : emptyPage
        )
      })
      const user = userEvent.setup()
      renderWorkspace()

      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Failed to load learning policy'
      )
      expect(screen.queryByText('Learning is disabled')).not.toBeInTheDocument()
      expect(
        screen.getByRole('switch', { name: 'Enable learning' })
      ).toHaveAttribute('aria-disabled', 'true')
      expect(
        await screen.findByRole('button', { name: 'Cancel' })
      ).toBeDisabled()
      await user.type(
        screen.getByLabelText('Instruction content'),
        'Draft instructions'
      )
      expect(
        screen.getByRole('button', { name: 'Save version' })
      ).toBeDisabled()
      await user.click(screen.getByRole('switch', { name: 'Enable learning' }))
      await user.click(screen.getByRole('button', { name: 'Cancel' }))
      await user.click(screen.getByRole('button', { name: 'Save version' }))
      expect(api.put).not.toHaveBeenCalled()
      expect(api.post).not.toHaveBeenCalled()
    }
  )

  it('retries policy reads without writing and only enables the switch after recovery', async () => {
    let fail = true
    let resolvePolicy!: (value: ReturnType<typeof successfulResponse>) => void
    const recovery = new Promise<ReturnType<typeof successfulResponse>>(
      (resolve) => {
        resolvePolicy = resolve
      }
    )
    vi.mocked(api.get).mockImplementation(async (url) => {
      if (!url.endsWith('prompt-learning')) return successfulResponse(emptyPage)
      if (fail) throw new Error('unavailable')
      return recovery
    })
    const user = userEvent.setup()
    renderWorkspace()
    const alert = await screen.findByRole('alert')
    fail = false
    const retry = within(alert).getByRole('button', { name: 'Retry' })
    await user.dblClick(retry)
    expect(
      screen.getByRole('switch', { name: 'Enable learning' })
    ).toHaveAttribute('aria-disabled', 'true')
    expect(api.put).not.toHaveBeenCalled()
    expect(api.post).not.toHaveBeenCalled()
    resolvePolicy(successfulResponse({ ...policy, enabled: true }))

    expect(await screen.findByText('Learning is enabled')).toBeInTheDocument()
    expect(
      screen.getByRole('switch', { name: 'Enable learning' })
    ).not.toHaveAttribute('aria-disabled', 'true')
    expect(
      screen.getByRole('switch', { name: 'Enable learning' })
    ).toBeChecked()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('preserves a manual draft but blocks saving after a version refresh fails until retry succeeds', async () => {
    const user = userEvent.setup()
    const client = renderWorkspace()
    await screen.findByText('No instruction versions yet.')
    await user.type(
      screen.getByLabelText('Instruction content'),
      'Preserved draft'
    )
    expect(screen.getByRole('button', { name: 'Save version' })).toBeEnabled()
    vi.mocked(api.get).mockRejectedValueOnce(new Error('refresh failed'))
    await act(async () =>
      client.invalidateQueries({ queryKey: ['prompt-learning', 'versions'] })
    )

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('Failed to load instruction versions')
    expect(
      screen.queryByText('No instruction versions yet.')
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save version' })).toBeDisabled()
    expect(screen.getByLabelText('Instruction content')).toHaveValue(
      'Preserved draft'
    )
    await user.click(within(alert).getByRole('button', { name: 'Retry' }))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save version' })).toBeEnabled()
    )
    expect(screen.getByLabelText('Instruction content')).toHaveValue(
      'Preserved draft'
    )
    expect(api.post).not.toHaveBeenCalled()
  })

  it('reports a run failure independently and retries it to a successful empty history', async () => {
    let fail = true
    vi.mocked(api.get).mockImplementation(async (url) => {
      if (url.endsWith('runs') && fail) throw new Error('unavailable')
      return successfulResponse(
        url.endsWith('prompt-learning') ? policy : emptyPage
      )
    })
    const user = userEvent.setup()
    renderWorkspace()

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('Failed to load learning runs')
    expect(screen.queryByText('No learning runs yet.')).not.toBeInTheDocument()
    expect(screen.getByText('No instruction versions yet.')).toBeInTheDocument()
    expect(
      screen.getByRole('switch', { name: 'Enable learning' })
    ).not.toHaveAttribute('aria-disabled', 'true')
    fail = false
    await user.click(within(alert).getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('No learning runs yet.')).toBeInTheDocument()
    expect(api.post).not.toHaveBeenCalled()
  })

  it('removes stale cancellation actions when run refresh fails', async () => {
    vi.mocked(api.get).mockImplementation(async (url) => {
      if (url.endsWith('runs')) return successfulResponse(pendingRunPage)
      return successfulResponse(
        url.endsWith('prompt-learning') ? policy : emptyPage
      )
    })
    const client = renderWorkspace()
    expect(await screen.findByRole('button', { name: 'Cancel' })).toBeEnabled()
    vi.mocked(api.get).mockRejectedValueOnce(new Error('refresh failed'))
    await act(async () =>
      client.invalidateQueries({ queryKey: ['prompt-learning', 'runs'] })
    )

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load learning runs'
    )
    expect(
      screen.queryByRole('button', { name: 'Cancel' })
    ).not.toBeInTheDocument()
    expect(api.post).not.toHaveBeenCalled()
  })
})
