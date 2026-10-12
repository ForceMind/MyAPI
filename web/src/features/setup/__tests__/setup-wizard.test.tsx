import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { SetupWizard } from '../setup-wizard'
import type { SetupResponse } from '../types'

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}))

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

async function reachReviewStep(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('button', { name: 'Next' }))
  await user.type(
    screen.getByRole('textbox', { name: 'Administrator username' }),
    'setup-admin'
  )
  await user.type(screen.getByLabelText('Password'), 'safe-password')
  await user.type(screen.getByLabelText('Confirm password'), 'safe-password')
  await user.click(screen.getByRole('button', { name: 'Next' }))
  await user.click(screen.getByRole('button', { name: 'Next' }))
  expect(
    screen.getByRole('heading', { name: 'Review & initialize', level: 2 })
  ).toBeInTheDocument()
}

beforeEach(() => {
  useSystemConfigStore.setState({ loading: false })
})

afterEach(() => {
  vi.useRealTimers()
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
    expect(post).toHaveBeenCalledExactlyOnceWith(
      '/api/setup',
      {
        SelfUseModeEnabled: true,
        DemoSiteEnabled: false,
      },
      expect.objectContaining({ skipAuthRefresh: true })
    )
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

  it('keeps confirmed success when the timed-out submission rejects after setup status is initialized', async () => {
    vi.spyOn(api, 'get')
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: { status: false, root_init: false, database_type: 'sqlite' },
        },
      })
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: { status: true, root_init: true, database_type: 'sqlite' },
        },
      })
    let rejectPost!: (error: Error) => void
    const post = vi.spyOn(api, 'post').mockImplementation(
      () =>
        new Promise((_resolve, reject) => {
          rejectPost = reject
        })
    )
    const user = userEvent.setup()
    await renderWizard()
    await reachReviewStep(user)

    vi.useFakeTimers()
    fireEvent.click(screen.getByRole('button', { name: 'Initialize system' }))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(15000)
    })

    expect(toast.success).toHaveBeenCalledWith(
      'System initialized successfully! Redirecting…'
    )
    expect(post).toHaveBeenCalledOnce()
    expect(api.get).toHaveBeenCalledTimes(2)
    expect(
      clients[0].getQueryData<SetupResponse>(['setup-status'])?.data?.status
    ).toBe(true)

    await act(async () => rejectPost(new Error('late adapter failure')))
    expect(toast.success).toHaveBeenCalledOnce()
    expect(toast.error).not.toHaveBeenCalledWith('Failed to initialize system')
  })

  it('keeps an unknown submission locked and retries only the status check when setup is still incomplete', async () => {
    vi.spyOn(api, 'get')
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: { status: false, root_init: false, database_type: 'sqlite' },
        },
      })
      .mockRejectedValueOnce(new Error('status unavailable'))
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: { status: false, root_init: false, database_type: 'sqlite' },
        },
      })
    const post = vi
      .spyOn(api, 'post')
      .mockRejectedValue(new Error('connection lost'))
    const user = userEvent.setup()
    await renderWizard()
    await reachReviewStep(user)

    await user.click(screen.getByRole('button', { name: 'Initialize system' }))
    const retryStatus = await screen.findByRole('button', {
      name: 'Check installation status',
    })
    expect(post).toHaveBeenCalledOnce()
    expect(screen.getByText('setup-admin')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Initializing…' })
    ).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Back' })).toBeDisabled()

    await user.click(retryStatus)

    expect(
      await screen.findByRole('button', { name: 'Check installation status' })
    ).toBeEnabled()
    expect(
      screen.getByRole('button', { name: 'Initializing…' })
    ).toBeDisabled()
    expect(screen.getByText('setup-admin')).toBeInTheDocument()
    expect(post).toHaveBeenCalledOnce()
    expect(api.get).toHaveBeenCalledTimes(3)
  })
})
