import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { toast, Toaster } from 'sonner'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { PerformanceSection } from '../performance-section'

const originalAuth = useAuthStore.getState()
const defaults = { 'performance_setting.disk_cache_enabled': true, 'performance_setting.disk_cache_threshold_mb': 10, 'performance_setting.disk_cache_max_size_mb': 1024, 'performance_setting.disk_cache_path': '/synthetic/saved', 'performance_setting.monitor_enabled': false, 'performance_setting.monitor_cpu_threshold': 90, 'performance_setting.monitor_memory_threshold': 90, 'performance_setting.monitor_disk_threshold': 95 }
let client: QueryClient
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: 100, username: 'synthetic-root' })
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: {} } })
})
afterEach(() => { act(() => toast.dismiss()); cleanup(); client.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })
function PerformanceFrame() {
  const actor = useAuthStore((state) => `${state.auth.user?.id}:${state.auth.user?.role}:${state.auth.session?.sid}`)
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  return <><div ref={setActions} /><SettingsPageProvider key={actor} actionsContainer={actions} suppressSectionHeader={false}><PerformanceSection defaultValues={defaults} /></SettingsPageProvider><Toaster /></>
}
async function renderPerformance() {
  await renderListRoute({ client, path: '/system-settings/operations/performance', element: <PerformanceFrame /> })
  await screen.findByLabelText('Cache Directory')
}
it('preserves missing, null and non-finite cache fields as unavailable while keeping explicit zero bytes and counts', async () => {
  vi.mocked(api.get).mockResolvedValue({ data: { success: true, data: { cache_stats: { current_disk_usage_bytes: null, disk_cache_max_bytes: 0, active_disk_files: 0, disk_cache_hits: Number.NaN, current_memory_usage_bytes: 0 } } } })
  await renderPerformance()
  await screen.findAllByText('Not available')
  const disk = screen.getByText('Request Body Disk Cache').parentElement
  const memory = screen.getByText('Request Body Memory Cache').parentElement
  if (!disk || !memory) throw new Error('Cache statistics containers are missing')
  expect(disk).toHaveTextContent(/Not available\s*\/\s*0 Bytes/)
  expect(disk).toHaveTextContent(/Active Files:\s*0/)
  expect(disk).toHaveTextContent(/Disk Hits:\s*Not available/)
  expect(memory).toHaveTextContent(/Current Cache Size:\s*0 Bytes/)
  expect(memory).toHaveTextContent(/Active Cache Count:\s*Not available/)
  expect(memory).toHaveTextContent(/Memory Hits:\s*Not available/)
})
it('keeps a Cache Directory edit entered during save and sends it only after a second explicit save with the original option body', async () => {
  let finish: ((value: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { finish = resolve })
  const put = vi.spyOn(api, 'put').mockReturnValueOnce(pending).mockResolvedValue({ data: { success: true } })
  await renderPerformance()
  const input = screen.getByLabelText('Cache Directory')
  fireEvent.change(input, { target: { value: '/synthetic/submitted' } })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  fireEvent.change(input, { target: { value: '/synthetic/later-draft' } })
  await act(async () => { finish?.({ data: { success: true } }); await pending })
  expect(input).toHaveValue('/synthetic/later-draft')
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(2))
  expect(put.mock.calls.map((call) => call.slice(0, 2))).toEqual([
    ['/api/option/', { key: 'performance_setting.disk_cache_path', value: '/synthetic/submitted' }],
    ['/api/option/', { key: 'performance_setting.disk_cache_path', value: '/synthetic/later-draft' }],
  ])
})
it('does not issue a follow-up statistics GET or toast from an old cleanup after the settings actor frame was replaced', async () => {
  let finish: ((value: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { finish = resolve })
  const remove = vi.spyOn(api, 'delete').mockReturnValue(pending)
  await renderPerformance()
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Clean up inactive cache' }))
  await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Confirm' }))
  await waitFor(() => expect(remove).toHaveBeenCalledTimes(1))
  await act(async () => { useAuthStore.getState().auth.setUser({ id: 2, role: 100, username: 'new-synthetic-root' }) })
  await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2))
  await act(async () => { finish?.({ data: { success: true } }); await pending })
  expect(api.get).toHaveBeenCalledTimes(2)
  expect(screen.queryByText('Disk cache cleared')).not.toBeInTheDocument()
  expect(remove).toHaveBeenCalledTimes(1)
})
