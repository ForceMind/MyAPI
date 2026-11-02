import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { CreateDeploymentDrawer } from '../create-deployment-drawer'
import { ExtendDeploymentDialog } from '../extend-deployment-dialog'
import { RenameDeploymentDialog } from '../rename-deployment-dialog'
import { UpdateConfigDialog } from '../update-config-dialog'
import { ViewDetailsDialog } from '../view-details-dialog'
import { ViewLogsDialog } from '../view-logs-dialog'

let client: QueryClient

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/deployments/hardware-types') return { data: { success: true, data: { hardware_types: [] } } }
    if (url === '/api/deployments/17/containers') return { data: { success: true, data: { containers: [] } } }
    if (url === '/api/deployments/17') return { data: { success: true, data: { status: 'running', container_config: { image_url: 'saved-image:1' } } } }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected deployment write'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected deployment write'))
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected deployment write'))
})

afterEach(() => { cleanup(); client.clear(); vi.restoreAllMocks() })

const tasks: Array<[string, () => ReactNode]> = [
  ['Deployment details', () => <ViewDetailsDialog open onOpenChange={vi.fn()} deploymentId={17} />],
  ['Update configuration - 17', () => <UpdateConfigDialog open onOpenChange={vi.fn()} deploymentId={17} />],
  ['Extend deployment', () => <ExtendDeploymentDialog open onOpenChange={vi.fn()} deploymentId={17} />],
  ['Rename deployment', () => <RenameDeploymentDialog open onOpenChange={vi.fn()} deploymentId={17} />],
  ['Deployment logs', () => <ViewLogsDialog open onOpenChange={vi.fn()} deploymentId={17} />],
]

it.each(tasks)('keeps %s to one vertical task scroll area with reachable fixed actions', async (name, content) => {
  render(<QueryClientProvider client={client}>{content()}</QueryClientProvider>)
  const dialog = await screen.findByRole('dialog', { name })
  const scrollAreas = [...dialog.querySelectorAll<HTMLElement>('*')].filter((element) => element.classList.contains('overflow-y-auto'))
  expect(scrollAreas).toHaveLength(1)
  expect(dialog).toHaveClass('h-dvh', '[&_button]:min-h-11', '[&_button]:min-w-11')
  await waitFor(() => expect(dialog.querySelector('[data-slot=dialog-footer]')).not.toBeNull())
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})

it('moves creation section navigation to the existing heading while leaving all resource and advanced fields intact', async () => {
  const previous = Object.getOwnPropertyDescriptor(Element.prototype, 'scrollIntoView')
  const scroll = vi.fn()
  Object.defineProperty(Element.prototype, 'scrollIntoView', { configurable: true, value: scroll })
  try {
    render(<QueryClientProvider client={client}><CreateDeploymentDrawer open onOpenChange={vi.fn()} /></QueryClientProvider>)
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Resource Configuration' }))
    expect(screen.getByRole('heading', { name: 'Resource Configuration' })).toHaveFocus()
    expect(scroll).toHaveBeenCalledWith({ block: 'start' })
    expect(screen.getByLabelText('GPU count')).toBeInTheDocument()
    expect(screen.getByLabelText('Replica count')).toBeInTheDocument()
    expect(screen.getByLabelText('Duration (hours)')).toBeInTheDocument()
    expect(screen.getByLabelText('Registry secret')).toBeInTheDocument()
    expect(api.post).not.toHaveBeenCalled()
  } finally {
    if (previous) Object.defineProperty(Element.prototype, 'scrollIntoView', previous)
    else Reflect.deleteProperty(Element.prototype, 'scrollIntoView')
  }
})
