import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { RenameDeploymentDialog } from '../rename-deployment-dialog'
import { UpdateConfigDialog } from '../update-config-dialog'
import { ViewDetailsDialog } from '../view-details-dialog'
import { ViewLogsDialog } from '../view-logs-dialog'

let client: QueryClient

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected deployment write'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected deployment write'))
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected deployment write'))
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
})

function fixture(children: ReactNode) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

it('keeps a container-list failure unknown instead of displaying a zero container count', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/deployments/17') return { data: { success: true, data: { id: 17, status: 'running', hardware_name: 'Synthetic GPU' } } }
    if (url === '/api/deployments/17/containers') throw new Error('Synthetic container read failed')
    throw new Error(`Unexpected GET ${url}`)
  })
  render(fixture(<ViewDetailsDialog open onOpenChange={vi.fn()} deploymentId={17} />))
  await screen.findByText('running')
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic container read failed')
  expect(screen.queryByText('0')).not.toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})

it('shows a failed log read as an error rather than an empty successful log', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/deployments/17/containers') return { data: { success: true, data: { containers: [{ container_id: 'synthetic-container' }] } } }
    if (url === '/api/deployments/17/logs') throw new Error('Synthetic log read failed')
    throw new Error(`Unexpected GET ${url}`)
  })
  render(fixture(<ViewLogsDialog open onOpenChange={vi.fn()} deploymentId={17} />))
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic log read failed')
  expect(screen.queryByText('No logs')).not.toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})

it('keeps an unsaved image when deployment details are reread while the same configuration task is open', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { container_config: { image_url: 'saved-image:1', traffic_port: 11434 } } } })
  render(fixture(<UpdateConfigDialog open onOpenChange={vi.fn()} deploymentId={17} />))
  const image = await screen.findByDisplayValue('saved-image:1')
  const user = userEvent.setup()
  await user.clear(image)
  await user.type(image, 'unsaved-image:2')
  get.mockResolvedValue({ data: { success: true, data: { container_config: { image_url: 'new-server-image:3', traffic_port: 8080 } } } })
  await act(async () => { await client.refetchQueries({ queryKey: ['deployment-details-for-update', 17] }) })
  await waitFor(() => expect(get).toHaveBeenCalledTimes(2))
  expect(screen.getByLabelText('Image')).toHaveValue('unsaved-image:2')
  expect(api.put).not.toHaveBeenCalled()
})

it('keeps an edited deployment name when the same object receives a refreshed saved name', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { available: true } } })
  const view = render(fixture(<RenameDeploymentDialog open onOpenChange={vi.fn()} deploymentId={17} currentName='saved-name' />))
  const user = userEvent.setup()
  const name = screen.getByPlaceholderText('Enter a new name')
  await user.clear(name)
  await user.type(name, 'unsaved-name')
  view.rerender(fixture(<RenameDeploymentDialog open onOpenChange={vi.fn()} deploymentId={17} currentName='new-server-name' />))
  expect(screen.getByPlaceholderText('Enter a new name')).toHaveValue('unsaved-name')
  expect(api.put).not.toHaveBeenCalled()
})
