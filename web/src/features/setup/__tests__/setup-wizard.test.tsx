import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { SetupWizard } from '../setup-wizard'

const initialConfig = useSystemConfigStore.getState()
const clients: QueryClient[] = []

async function renderWizard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const route = createRootRoute({ component: SetupWizard })
  const router = createRouter({
    routeTree: route,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  useSystemConfigStore.setState({ loading: false })
})

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useSystemConfigStore.setState(initialConfig)
  localStorage.clear()
})

describe('setup task flow', () => {
  it('keeps failed setup status recoverable without exposing initialization controls', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: false, message: 'Status unavailable' },
    })
    await renderWizard()
    expect(await screen.findByRole('button', { name: 'Retry' })).toBeEnabled()
    expect(
      screen.queryByRole('button', { name: 'Next' })
    ).not.toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  })

  it('announces the current step and moves keyboard focus to its heading after Next and Back', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { status: false, root_init: false, database_type: 'sqlite' },
      },
    })
    const user = userEvent.setup()
    await renderWizard()
    const next = await screen.findByRole('button', { name: 'Next' })
    next.focus()
    await user.keyboard('{Enter}')
    const heading = screen.getByRole('heading', {
      name: 'Administrator account',
      level: 2,
    })
    expect(heading).toHaveFocus()
    const progress = screen.getByRole('list', { name: 'System setup wizard' })
    const active = within(progress)
      .getByText('Administrator account')
      .closest('li')
    expect(active).toHaveAttribute('aria-current', 'step')
    await user.click(screen.getByRole('button', { name: 'Back' }))
    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'Database check', level: 2 })
      ).toHaveFocus()
    )
  })

  it('keeps invalid administrator details on their step with the existing field error', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { status: false, root_init: false, database_type: 'postgres' },
      },
    })
    const user = userEvent.setup()
    await renderWizard()
    await user.click(await screen.findByRole('button', { name: 'Next' }))
    await user.click(screen.getByRole('button', { name: 'Next' }))
    expect(
      screen.getByRole('textbox', { name: 'Administrator username' })
    ).toHaveAttribute('aria-invalid', 'true')
    expect(
      screen.getByRole('heading', { name: 'Administrator account', level: 2 })
    ).toBeInTheDocument()
  })

  it('recovers a failed status read before allowing the user to continue', async () => {
    vi.spyOn(api, 'get')
      .mockResolvedValueOnce({ data: { success: false } })
      .mockResolvedValue({
        data: {
          success: true,
          data: { status: false, root_init: false, database_type: 'sqlite' },
        },
      })
    const user = userEvent.setup()
    await renderWizard()
    await user.click(await screen.findByRole('button', { name: 'Retry' }))
    expect(await screen.findByRole('button', { name: 'Next' })).toBeEnabled()
    expect(
      screen.queryByRole('button', { name: 'Retry' })
    ).not.toBeInTheDocument()
    expect(screen.getByText('Persist your data file')).toBeInTheDocument()
  })

  it('preserves existing administrator credentials through review and a failed initialization', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          status: false,
          root_init: true,
          database_type: 'postgres',
          SelfUseModeEnabled: true,
        },
      },
    })
    let finishRequest!: (value: { data: { success: boolean } }) => void
    const post = vi.spyOn(api, 'post').mockImplementation(
      () =>
        new Promise((resolve) => {
          finishRequest = resolve
        })
    )
    const user = userEvent.setup()
    await renderWizard()
    await user.click(await screen.findByRole('button', { name: 'Next' }))
    expect(
      screen.getByText(
        'The administrator account is already initialized. You can keep your existing credentials and continue to the next step.'
      )
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('textbox', { name: 'Administrator username' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Next' }))
    await user.click(screen.getByRole('button', { name: 'Next' }))
    await user.click(screen.getByRole('button', { name: 'Initialize system' }))
    expect(post).toHaveBeenCalledExactlyOnceWith('/api/setup', {
      SelfUseModeEnabled: true,
      DemoSiteEnabled: false,
    })
    expect(screen.getByRole('button', { name: 'Initializing…' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Back' })).toBeDisabled()
    await act(async () => finishRequest({ data: { success: false } }))
    expect(
      await screen.findByRole('button', { name: 'Initialize system' })
    ).toBeEnabled()
    expect(
      screen.getByRole('heading', { name: 'Review & initialize', level: 2 })
    ).toBeInTheDocument()
  })
})
