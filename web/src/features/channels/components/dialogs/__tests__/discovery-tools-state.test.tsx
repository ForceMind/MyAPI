import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'

import { ChannelsProvider } from '../../channels-provider'
import { FetchModelsDialog } from '../fetch-models-dialog'
import { UpstreamUpdateDialog } from '../upstream-update-dialog'

let client: QueryClient
beforeEach(() => { client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } }) })
afterEach(() => { cleanup(); client.clear(); vi.restoreAllMocks() })
function fixture(children: ReactNode) {
  return <QueryClientProvider client={client}><TooltipProvider><ChannelsProvider>{children}</ChannelsProvider></TooltipProvider></QueryClientProvider>
}

it('keeps the new fetch task selection when an older source finishes after the current source', async () => {
  let finishOld: ((models: string[]) => void) | undefined
  const oldRead = new Promise<string[]>((resolve) => { finishOld = resolve })
  const first = vi.fn(() => oldRead)
  const second = vi.fn().mockResolvedValue(['synthetic-new-source'])
  const selected = vi.fn()
  const view = render(fixture(<FetchModelsDialog open onOpenChange={vi.fn()} customFetcher={first} onModelsSelected={selected} existingModelsOverride={[]} />))
  await waitFor(() => expect(first).toHaveBeenCalledTimes(1))
  view.rerender(fixture(<FetchModelsDialog open onOpenChange={vi.fn()} customFetcher={second} onModelsSelected={selected} existingModelsOverride={[]} />))
  expect(second).not.toHaveBeenCalled()
  expect(await screen.findByRole('alert')).toHaveTextContent('The model source changed. Fetch models again.')
  await userEvent.setup().click(screen.getByRole('button', { name: 'Fetch Models' }))
  const checkbox = await screen.findByRole('checkbox', { name: 'synthetic-new-source' })
  await userEvent.setup().click(checkbox)
  expect(checkbox).toBeChecked()
  await act(async () => { finishOld?.(['synthetic-old-source']); await oldRead })
  expect(screen.getByRole('checkbox', { name: 'synthetic-new-source' })).toBeChecked()
  expect(screen.queryByText('synthetic-old-source')).not.toBeInTheDocument()
  expect(selected).not.toHaveBeenCalled()
})

it('shows a failed automatic model fetch as an error instead of a successful empty model list', async () => {
  const fetcher = vi.fn().mockRejectedValue(new Error('Synthetic upstream model read failed'))
  render(fixture(<FetchModelsDialog open onOpenChange={vi.fn()} customFetcher={fetcher} onModelsSelected={vi.fn()} existingModelsOverride={[]} />))
  expect(await screen.findByText('Synthetic upstream model read failed')).toBeVisible()
  expect(fetcher).toHaveBeenCalledTimes(1)
})

it('blocks Escape and cancellation while the original upstream model update is pending', async () => {
  const cancel = vi.fn()
  render(<UpstreamUpdateDialog open addModels={['synthetic-add']} removeModels={[]} preferredTab='add' confirmLoading onConfirm={vi.fn()} onCancel={cancel} />)
  await userEvent.setup().keyboard('{Escape}')
  expect(cancel).not.toHaveBeenCalled()
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
})

it('admits one upstream update callback while its original asynchronous update is unresolved', async () => {
  let complete: (() => void) | undefined
  const pending = new Promise<void>((resolve) => { complete = resolve })
  const confirm = vi.fn(() => pending)
  render(<UpstreamUpdateDialog open addModels={['synthetic-add']} removeModels={[]} preferredTab='add' confirmLoading={false} onConfirm={confirm} onCancel={vi.fn()} />)
  const button = screen.getByRole('button', { name: 'Confirm' })
  fireEvent.click(button)
  fireEvent.click(button)
  try {
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(confirm).toHaveBeenCalledWith({ addModels: ['synthetic-add'], removeModels: [] })
  } finally {
    await act(async () => { complete?.(); await pending })
  }
})


it('clears the old selectable directory before a failed retry and never offers stale results for application', async () => {
  const fetcher = vi.fn().mockResolvedValueOnce(['synthetic-first']).mockRejectedValueOnce(new Error('Synthetic retry failed'))
  const selected = vi.fn()
  const firstScope = {}
  const view = render(fixture(<FetchModelsDialog open onOpenChange={vi.fn()} sourceScope={firstScope} customFetcher={fetcher} onModelsSelected={selected} existingModelsOverride={['synthetic-saved']} />))
  expect(await screen.findByRole('checkbox', { name: 'synthetic-first' })).toBeVisible()
  view.rerender(fixture(<FetchModelsDialog open onOpenChange={vi.fn()} sourceScope={{}} customFetcher={fetcher} onModelsSelected={selected} existingModelsOverride={['synthetic-saved']} />))
  expect(fetcher).toHaveBeenCalledTimes(1)
  expect(screen.queryByRole('button', { name: 'Save Models' })).not.toBeInTheDocument()
  await userEvent.setup().click(screen.getByRole('button', { name: 'Fetch Models' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic retry failed')
  expect(screen.queryByRole('checkbox', { name: 'synthetic-first' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Save Models' })).not.toBeInTheDocument()
  expect(selected).not.toHaveBeenCalled()
  expect(fetcher).toHaveBeenCalledTimes(2)
})
