import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterContextProvider,
} from '@tanstack/react-router'
import {
  act,
  render,
  renderHook,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { getDeploymentSettings, testDeploymentConnection } from '../../api'
import { DeploymentAccessGuard } from '../../components/deployment-access-guard'
import {
  clearConnectionCache,
  useModelDeploymentSettings,
} from '../use-model-deployment-settings'

vi.mock('../../api', () => ({
  getDeploymentSettings: vi.fn(),
  testDeploymentConnection: vi.fn(),
}))

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function DeploymentAvailability(props: { active: boolean }) {
  const deployment = useModelDeploymentSettings(props.active)
  if (!props.active) return <p>Metadata</p>
  return (
    <DeploymentAccessGuard
      loading={deployment.loading}
      loadingPhase={deployment.loadingPhase}
      isEnabled={deployment.isIoNetEnabled}
      connectionLoading={deployment.connectionLoading}
      connectionOk={deployment.connectionOk}
      connectionError={deployment.connectionError}
      settingsError={deployment.settingsError}
      onRetry={
        deployment.settingsError
          ? deployment.refresh
          : deployment.testConnection
      }
    >
      <p>Deployment list</p>
    </DeploymentAccessGuard>
  )
}

beforeEach(() => {
  vi.resetAllMocks()
  clearConnectionCache()
})

describe('deployment availability evidence', () => {
  it.each(['response', 'network'] as const)(
    'preserves a cached %s connection failure after navigation and allows retry',
    async (failure) => {
      const user = userEvent.setup()
      const router = createRouter({
        routeTree: createRootRoute(),
        history: createMemoryHistory({ initialEntries: ['/'] }),
      })
      vi.mocked(getDeploymentSettings).mockResolvedValue({
        success: true,
        data: { enabled: true },
      })
      if (failure === 'network') {
        vi.mocked(testDeploymentConnection).mockRejectedValueOnce(
          new Error('Provider unavailable')
        )
      } else {
        vi.mocked(testDeploymentConnection).mockResolvedValueOnce({
          success: false,
          message: 'Provider unavailable',
        })
      }
      vi.mocked(testDeploymentConnection).mockResolvedValue({ success: true })
      const { rerender } = render(
        <RouterContextProvider router={router}>
          <DeploymentAvailability active />
        </RouterContextProvider>
      )
      expect(await screen.findByText('Provider unavailable')).toBeVisible()

      rerender(
        <RouterContextProvider router={router}>
          <DeploymentAvailability active={false} />
        </RouterContextProvider>
      )
      rerender(
        <RouterContextProvider router={router}>
          <DeploymentAvailability active />
        </RouterContextProvider>
      )

      expect(
        await screen.findByRole('heading', { name: 'Connection failed' })
      ).toBeVisible()
      expect(screen.getByText('Provider unavailable')).toBeVisible()
      expect(screen.queryByText('Deployment list')).not.toBeInTheDocument()
      expect(testDeploymentConnection).toHaveBeenCalledOnce()
      const retry = screen.getByRole('button', { name: 'Retry' })
      expect(retry).toBeEnabled()
      retry.focus()
      await user.keyboard('{Enter}')
      expect(await screen.findByText('Deployment list')).toBeVisible()
      expect(
        screen.queryByRole('button', { name: 'Retry' })
      ).not.toBeInTheDocument()
      expect(testDeploymentConnection).toHaveBeenCalledTimes(2)
    }
  )

  it('keeps unavailable settings distinct from a confirmed disabled service', async () => {
    vi.mocked(getDeploymentSettings).mockRejectedValue(new Error('offline'))
    const { result } = renderHook(() => useModelDeploymentSettings())
    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.settingsError).toBe('Unable to load settings')
    expect(result.current.isIoNetEnabled).toBe(false)
    expect(testDeploymentConnection).not.toHaveBeenCalled()
    vi.mocked(getDeploymentSettings).mockResolvedValue({
      success: true,
      data: { enabled: false },
    })
    await act(async () => {
      await result.current.refresh()
    })
    expect(result.current.settingsError).toBeNull()
    expect(result.current.isIoNetEnabled).toBe(false)
    expect(testDeploymentConnection).not.toHaveBeenCalled()
  })

  it('does not query deployment services on the metadata tab', async () => {
    vi.mocked(getDeploymentSettings).mockResolvedValue({
      success: true,
      data: { enabled: false },
    })
    const { rerender, result } = renderHook(
      ({ active }) => useModelDeploymentSettings(active),
      { initialProps: { active: false } }
    )
    expect(getDeploymentSettings).not.toHaveBeenCalled()
    rerender({ active: true })
    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(getDeploymentSettings).toHaveBeenCalledOnce()
    expect(result.current.settingsError).toBeNull()
    expect(result.current.isIoNetEnabled).toBe(false)
    expect(testDeploymentConnection).not.toHaveBeenCalled()
  })

  it('keeps the latest disabled settings when an earlier deployment visit finishes late', async () => {
    const firstSettings =
      deferred<Awaited<ReturnType<typeof getDeploymentSettings>>>()
    vi.mocked(getDeploymentSettings)
      .mockReturnValueOnce(firstSettings.promise)
      .mockResolvedValue({ success: true, data: { enabled: false } })
    vi.mocked(testDeploymentConnection).mockResolvedValue({ success: true })
    const { rerender, result } = renderHook(
      ({ active }) => useModelDeploymentSettings(active),
      { initialProps: { active: true } }
    )

    rerender({ active: false })
    rerender({ active: true })
    await waitFor(() => expect(result.current.loading).toBe(false))
    await act(async () => {
      firstSettings.resolve({ success: true, data: { enabled: true } })
      await firstSettings.promise
    })

    expect(result.current.isIoNetEnabled).toBe(false)
    expect(result.current.connectionOk).toBeNull()
    expect(result.current.settingsError).toBeNull()
    expect(getDeploymentSettings).toHaveBeenCalledTimes(2)
    expect(testDeploymentConnection).not.toHaveBeenCalled()
  })

  it('does not start a connection check when settings finish after leaving deployments', async () => {
    const settings =
      deferred<Awaited<ReturnType<typeof getDeploymentSettings>>>()
    vi.mocked(getDeploymentSettings).mockReturnValue(settings.promise)
    vi.mocked(testDeploymentConnection).mockResolvedValue({ success: true })
    const { rerender, result } = renderHook(
      ({ active }) => useModelDeploymentSettings(active),
      { initialProps: { active: true } }
    )

    rerender({ active: false })
    await act(async () => {
      settings.resolve({ success: true, data: { enabled: true } })
      await settings.promise
    })

    expect(result.current.isIoNetEnabled).toBe(false)
    expect(testDeploymentConnection).not.toHaveBeenCalled()
  })

  it('keeps the current load pending when settings from a previous visit fail', async () => {
    const firstSettings =
      deferred<Awaited<ReturnType<typeof getDeploymentSettings>>>()
    const currentSettings =
      deferred<Awaited<ReturnType<typeof getDeploymentSettings>>>()
    vi.mocked(getDeploymentSettings)
      .mockReturnValueOnce(firstSettings.promise)
      .mockReturnValueOnce(currentSettings.promise)
    const { rerender, result } = renderHook(
      ({ active }) => useModelDeploymentSettings(active),
      { initialProps: { active: true } }
    )

    rerender({ active: false })
    rerender({ active: true })
    await act(async () => {
      firstSettings.reject(new Error('offline'))
      await firstSettings.promise.catch(() => undefined)
    })

    expect(result.current.loading).toBe(true)
    expect(result.current.loadingPhase).toBe('settings')
    expect(result.current.settingsError).toBeNull()
    await act(async () => {
      currentSettings.resolve({ success: true, data: { enabled: false } })
      await currentSettings.promise
    })
    expect(result.current.loading).toBe(false)
    expect(result.current.isIoNetEnabled).toBe(false)
    expect(testDeploymentConnection).not.toHaveBeenCalled()
  })

  it('ignores a late connection result and does not reuse it on the next visit', async () => {
    const firstConnection =
      deferred<Awaited<ReturnType<typeof testDeploymentConnection>>>()
    vi.mocked(getDeploymentSettings)
      .mockResolvedValueOnce({ success: true, data: { enabled: true } })
      .mockResolvedValueOnce({ success: true, data: { enabled: false } })
      .mockResolvedValue({ success: true, data: { enabled: true } })
    vi.mocked(testDeploymentConnection)
      .mockReturnValueOnce(firstConnection.promise)
      .mockResolvedValue({
        success: false,
        message: 'Current connection failed',
      })
    const { rerender, result } = renderHook(
      ({ active }) => useModelDeploymentSettings(active),
      { initialProps: { active: true } }
    )
    await waitFor(() => expect(result.current.connectionLoading).toBe(true))

    rerender({ active: false })
    rerender({ active: true })
    await waitFor(() => expect(result.current.loading).toBe(false))
    await act(async () => {
      firstConnection.resolve({ success: true })
      await firstConnection.promise
    })
    expect(result.current.isIoNetEnabled).toBe(false)
    expect(result.current.connectionOk).toBeNull()

    rerender({ active: false })
    rerender({ active: true })
    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.connectionOk).toBe(false)
    expect(result.current.connectionError).toBe('Current connection failed')
    expect(testDeploymentConnection).toHaveBeenCalledTimes(2)
  })

  it('keeps a successful retry when an older retry fails later', async () => {
    const firstRetry =
      deferred<Awaited<ReturnType<typeof testDeploymentConnection>>>()
    vi.mocked(getDeploymentSettings).mockResolvedValue({
      success: true,
      data: { enabled: true },
    })
    vi.mocked(testDeploymentConnection)
      .mockResolvedValueOnce({ success: false, message: 'Connection failed' })
      .mockReturnValueOnce(firstRetry.promise)
      .mockResolvedValue({ success: true })
    const { result } = renderHook(() => useModelDeploymentSettings())
    await waitFor(() => expect(result.current.loading).toBe(false))

    let pendingRetry!: Promise<void>
    act(() => {
      pendingRetry = result.current.testConnection()
    })
    await act(async () => {
      await result.current.testConnection()
    })
    await act(async () => {
      firstRetry.reject(new Error('Old failure'))
      await pendingRetry
    })

    expect(result.current.connectionOk).toBe(true)
    expect(result.current.connectionError).toBeNull()
    expect(result.current.connectionLoading).toBe(false)
    expect(testDeploymentConnection).toHaveBeenCalledTimes(3)
  })

  it('keeps current connection checking active when a retry from a previous visit succeeds', async () => {
    const oldRetry =
      deferred<Awaited<ReturnType<typeof testDeploymentConnection>>>()
    const currentConnection =
      deferred<Awaited<ReturnType<typeof testDeploymentConnection>>>()
    vi.mocked(getDeploymentSettings).mockResolvedValue({
      success: true,
      data: { enabled: true },
    })
    vi.mocked(testDeploymentConnection)
      .mockResolvedValueOnce({ success: false, message: 'Connection failed' })
      .mockReturnValueOnce(oldRetry.promise)
      .mockReturnValueOnce(currentConnection.promise)
    const { rerender, result } = renderHook(
      ({ active }) => useModelDeploymentSettings(active),
      { initialProps: { active: true } }
    )
    await waitFor(() => expect(result.current.loading).toBe(false))
    let pendingRetry!: Promise<void>
    act(() => {
      pendingRetry = result.current.testConnection()
    })

    rerender({ active: false })
    rerender({ active: true })
    await waitFor(() => expect(result.current.loadingPhase).toBe('connection'))
    await act(async () => {
      oldRetry.resolve({ success: true })
      await pendingRetry
    })

    expect(result.current.loading).toBe(true)
    expect(result.current.loadingPhase).toBe('connection')
    expect(result.current.connectionLoading).toBe(true)
    expect(result.current.connectionOk).toBeNull()
    await act(async () => {
      currentConnection.resolve({ success: false, message: 'Current failure' })
      await currentConnection.promise
    })
    expect(result.current.loading).toBe(false)
    expect(result.current.connectionError).toBe('Current failure')
    expect(testDeploymentConnection).toHaveBeenCalledTimes(3)
  })

  it('keeps a focused refresh authoritative when the previous connection check fails', async () => {
    const previousConnection =
      deferred<Awaited<ReturnType<typeof testDeploymentConnection>>>()
    const focusedSettings =
      deferred<Awaited<ReturnType<typeof getDeploymentSettings>>>()
    vi.mocked(getDeploymentSettings)
      .mockResolvedValueOnce({ success: true, data: { enabled: true } })
      .mockReturnValueOnce(focusedSettings.promise)
    vi.mocked(testDeploymentConnection).mockReturnValue(
      previousConnection.promise
    )
    const { result } = renderHook(() => useModelDeploymentSettings())
    await waitFor(() => expect(result.current.connectionLoading).toBe(true))

    act(() => {
      window.dispatchEvent(new Event('focus'))
    })
    await act(async () => {
      previousConnection.reject(new Error('Outdated connection failure'))
      await previousConnection.promise.catch(() => undefined)
    })

    expect(result.current.loading).toBe(true)
    expect(result.current.loadingPhase).toBe('settings')
    expect(result.current.connectionError).toBeNull()
    await act(async () => {
      focusedSettings.resolve({ success: true, data: { enabled: false } })
      await focusedSettings.promise
    })
    expect(result.current.loading).toBe(false)
    expect(result.current.isIoNetEnabled).toBe(false)
    expect(result.current.connectionLoading).toBe(false)
    expect(getDeploymentSettings).toHaveBeenCalledTimes(2)
    expect(testDeploymentConnection).toHaveBeenCalledOnce()
  })

  it('does not start a connection check when settings finish after unmount', async () => {
    const settings =
      deferred<Awaited<ReturnType<typeof getDeploymentSettings>>>()
    vi.mocked(getDeploymentSettings).mockReturnValue(settings.promise)
    vi.mocked(testDeploymentConnection).mockResolvedValue({ success: true })
    const { unmount } = renderHook(() => useModelDeploymentSettings())

    unmount()
    await act(async () => {
      settings.resolve({ success: true, data: { enabled: true } })
      await settings.promise
    })

    expect(testDeploymentConnection).not.toHaveBeenCalled()
  })
})
