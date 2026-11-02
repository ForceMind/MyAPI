import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { LogSettingsSection } from '../log-settings-section'
import { PerformanceSection } from '../performance-section'

const originalAuth = useAuthStore.getState()
const defaults = { 'performance_setting.disk_cache_enabled': false, 'performance_setting.disk_cache_threshold_mb': 10, 'performance_setting.disk_cache_max_size_mb': 1024, 'performance_setting.disk_cache_path': '', 'performance_setting.monitor_enabled': false, 'performance_setting.monitor_cpu_threshold': 90, 'performance_setting.monitor_memory_threshold': 90, 'performance_setting.monitor_disk_threshold': 95 }
let client: QueryClient
beforeEach(() => { client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } }); useAuthStore.getState().auth.setUser({ id: 1, role: 100, username: 'synthetic-root' }); vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined) })
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

it('a performance read failure is visible and retries explicitly without inventing zero statistics', async () => {
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url !== '/api/performance/stats') throw new Error(`Unexpected synthetic maintenance GET ${url}`)
    return { data: { success: false, message: 'Synthetic statistics unavailable' } }
  })
  const user = userEvent.setup()
  await renderListRoute({ client, path: '/system-settings/operations/performance', element: <PerformanceSection defaultValues={defaults} /> })
  const failure = await screen.findByRole('alert')
  expect(failure).toHaveTextContent('Synthetic statistics unavailable')
  await user.click(within(failure).getByRole('button', { name: 'Retry' }))
  expect(get).toHaveBeenCalledTimes(2)
  expect(screen.queryByText('0 B')).not.toBeInTheDocument()
})

it('cache cleanup preserves the original ten-minute confirmation and pending lock and leaves a failed target open without automatic retry', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: false, message: 'Synthetic statistics unavailable' } })
  let complete: ((value: { data: { success: boolean; message: string } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean; message: string } }>((resolve) => { complete = resolve })
  const remove = vi.spyOn(api, 'delete').mockReturnValue(pending)
  const user = userEvent.setup()
  await renderListRoute({ client, path: '/system-settings/operations/performance', element: <PerformanceSection defaultValues={defaults} /> })
  await user.click(screen.getByRole('button', { name: 'Clean up inactive cache' }))
  const confirmation = await screen.findByRole('alertdialog', { name: 'Confirm cleanup of inactive disk cache?' })
  expect(confirmation).toHaveTextContent('more than 10 minutes')
  await user.click(within(confirmation).getByRole('button', { name: 'Confirm' }))
  expect(within(confirmation).getByRole('button', { name: 'Cancel' })).toBeDisabled()
  await user.keyboard('{Escape}')
  expect(confirmation).toBeVisible()
  await act(async () => { complete?.({ data: { success: false, message: 'Synthetic cache cleanup failed' } }); await pending })
  expect(await within(confirmation).findByRole('alert')).toHaveTextContent('Synthetic cache cleanup failed')
  expect(remove).toHaveBeenCalledTimes(1)
  expect(remove).toHaveBeenCalledWith('/api/performance/disk_cache', { skipAuthRefresh: true })
  expect(remove.mock.calls[0]?.[0]).toBe('/api/performance/disk_cache')
})

it('unreadable server log storage metadata is an error rather than a disabled or empty log directory', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/performance/logs') return { data: { success: false, message: 'Synthetic log metadata unavailable' } }
    if (url === '/api/system-task/current') return { data: { success: true, data: null } }
    throw new Error(`Unexpected synthetic maintenance GET ${url}`)
  })
  const remove = vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected synthetic log deletion'))
  await renderListRoute({ client, path: '/system-settings/operations/logs', element: <LogSettingsSection defaultEnabled /> })
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic log metadata unavailable')
  expect(remove).not.toHaveBeenCalled()
})

it('missing optional cache statistics remain unavailable rather than displaying invented zero bytes or hit counts', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url !== '/api/performance/stats') throw new Error(`Unexpected synthetic maintenance GET ${url}`)
    return { data: { success: true, data: {} } }
  })
  await renderListRoute({ client, path: '/system-settings/operations/performance', element: <PerformanceSection defaultValues={defaults} /> })
  expect(await screen.findByText('Cache statistics unavailable')).toBeVisible()
  expect(screen.queryByText('0 Bytes')).not.toBeInTheDocument()
})
