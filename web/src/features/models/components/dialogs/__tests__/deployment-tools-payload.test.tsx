import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { ExtendDeploymentDialog } from '../extend-deployment-dialog'
import { RenameDeploymentDialog } from '../rename-deployment-dialog'
import { UpdateConfigDialog } from '../update-config-dialog'

let client: QueryClient

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true } })
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected deployment delete'))
})

afterEach(() => { cleanup(); client.clear(); vi.restoreAllMocks() })

function fixture(children: ReactNode) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

it('preserves the configuration payload including string conversion of zero and false environment values', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { container_config: { image_url: 'saved-image:1', traffic_port: 11434, entrypoint: ['bash', '-lc'], env_variables: { ZERO: 0, FALSE: false, TEXT: 'unchanged' } } } } })
  const close = vi.fn()
  render(fixture(<UpdateConfigDialog open onOpenChange={close} deploymentId={17} />))
  await screen.findByDisplayValue('saved-image:1')
  fireEvent.change(screen.getByLabelText('Image'), { target: { value: ' updated-image:2 ' } })
  fireEvent.change(screen.getByLabelText('Port'), { target: { value: '8080' } })
  fireEvent.change(screen.getByLabelText('Args (space separated)'), { target: { value: '--alpha  two' } })
  fireEvent.change(screen.getByLabelText('Command'), { target: { value: '  run-command  ' } })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Registry (optional)' }))
  fireEvent.change(screen.getByLabelText('Registry username'), { target: { value: ' synthetic-user ' } })
  fireEvent.change(screen.getByLabelText('Registry secret'), { target: { value: ' synthetic-secret ' } })
  await user.click(screen.getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(vi.mocked(api.put).mock.calls[0]?.slice(0, 2)).toEqual(['/api/deployments/17', {
    image_url: 'updated-image:2', traffic_port: 8080, registry_username: 'synthetic-user', registry_secret: 'synthetic-secret', command: 'run-command', entrypoint: ['bash', '-lc'], args: ['--alpha', 'two'], env_variables: { ZERO: '0', FALSE: 'false', TEXT: 'unchanged' },
  }])
  await waitFor(() => expect(close).toHaveBeenCalledWith(false))
  expect(api.post).not.toHaveBeenCalled()
})

it('renames exactly the selected deployment with the existing trimmed name payload', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { available: true } } })
  render(fixture(<RenameDeploymentDialog open onOpenChange={vi.fn()} deploymentId='deployment-17' currentName='saved-name' />))
  fireEvent.change(screen.getByPlaceholderText('Enter a new name'), { target: { value: ' new-name ' } })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Rename' }))
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(vi.mocked(api.put).mock.calls[0]?.slice(0, 2)).toEqual(['/api/deployments/deployment-17/name', { name: 'new-name' }])
  expect(api.post).not.toHaveBeenCalled()
})

it('extends only the selected deployment with the existing duration payload when a quote is unavailable', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: {} } })
  render(fixture(<ExtendDeploymentDialog open onOpenChange={vi.fn()} deploymentId={17} />))
  await screen.findByText('Unable to estimate price for this deployment.')
  fireEvent.change(screen.getByLabelText('Duration (hours)'), { target: { value: '7' } })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Extend' }))
  await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1))
  expect(vi.mocked(api.post).mock.calls[0]?.slice(0, 2)).toEqual(['/api/deployments/17/extend', { duration_hours: 7 }])
  expect(api.put).not.toHaveBeenCalled()
})
