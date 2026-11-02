import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { CreateDeploymentDrawer } from '../create-deployment-drawer'

let client: QueryClient

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected deployment write'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected deployment write'))
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected deployment write'))
})

afterEach(() => { cleanup(); client.clear(); vi.restoreAllMocks() })

it('shows an unavailable quote before any hardware or region has been selected without inventing a zero cost', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { hardware_types: [] } } })
  render(<QueryClientProvider client={client}><CreateDeploymentDrawer open onOpenChange={vi.fn()} /></QueryClientProvider>)
  expect(await screen.findByLabelText('Estimated cost')).toHaveTextContent('Not available')
  expect(api.post).not.toHaveBeenCalled()
})

it('shows a hardware read failure while keeping all deployment configuration sections reachable', async () => {
  vi.spyOn(api, 'get').mockRejectedValue(new Error('Synthetic hardware read failed'))
  render(<QueryClientProvider client={client}><CreateDeploymentDrawer open onOpenChange={vi.fn()} /></QueryClientProvider>)
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic hardware read failed')
  expect(screen.getByLabelText('Container name')).toBeVisible()
  expect(screen.getByText('Hardware type')).toBeVisible()
  expect(screen.getByText('Deployment location')).toBeVisible()
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(api.post).not.toHaveBeenCalled()
})
