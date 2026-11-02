import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { CCSwitchDialog } from '../dialogs/cc-switch-dialog'

let client: QueryClient
const modelName = 'synthetic/model-with-a-complete-long-identifier-and-final-suffix'
beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('status', JSON.stringify({ server_address: 'https://synthetic.gateway.invalid' }))
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url !== '/api/user/models') throw new Error(`Unexpected synthetic GET ${url}`)
    return { data: { success: true, data: [modelName] } }
  })
  vi.spyOn(window, 'open').mockReturnValue(null)
})
afterEach(() => { cleanup(); client.clear(); localStorage.clear(); vi.restoreAllMocks() })

it.each(['transport', 'business'] as const)('a %s catalog read failure has an explicit retry and preserves the CC Switch draft', async (failure) => {
  const get = vi.mocked(api.get)
  if (failure === 'transport') get.mockRejectedValueOnce(new Error('Synthetic catalog failed'))
  else get.mockResolvedValueOnce({ data: { success: false, message: 'Synthetic catalog failed' } })
  const user = userEvent.setup()
  render(<QueryClientProvider client={client}><CCSwitchDialog open tokenKey='synthetic-credential' onOpenChange={() => {}} /></QueryClientProvider>)
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic catalog failed')
  await user.click(screen.getByRole('radio', { name: 'Codex' }))
  const name = screen.getByRole('combobox', { name: 'Name' })
  await user.clear(name)
  await user.type(name, 'My saved draft')
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument())
  expect(screen.getByRole('radio', { name: 'Codex' })).toBeChecked()
  expect(name).toHaveValue('My saved draft')
  await user.click(screen.getByRole('combobox', { name: 'Primary Model *' }))
  expect(await screen.findByRole('option', { name: modelName })).toBeVisible()
  expect(get).toHaveBeenCalledTimes(2)
  expect(window.open).not.toHaveBeenCalled()
})

it.each(['claude', 'codex', 'gemini'] as const)('the %s form preserves every application field and exact external import parameters', async (app) => {
  const user = userEvent.setup()
  const onOpenChange = vi.fn()
  render(<QueryClientProvider client={client}><CCSwitchDialog open tokenKey={app === 'codex' ? 'sk-synthetic-credential' : 'synthetic-credential'} onOpenChange={onOpenChange} /></QueryClientProvider>)
  if (app !== 'claude') await user.click(screen.getByRole('radio', { name: app === 'codex' ? 'Codex' : 'Gemini' }))
  if (app === 'gemini') {
    const name = screen.getByRole('combobox', { name: 'Name' })
    await user.clear(name)
    await user.type(name, 'Synthetic custom import name')
  }
  const fields = app === 'claude' ? ['Primary Model *', 'Haiku Model', 'Sonnet Model', 'Opus Model'] : ['Primary Model *']
  for (const field of fields) {
    const input = screen.getByRole('combobox', { name: field })
    await user.click(input)
    await screen.findByRole('option', { name: modelName })
    await user.keyboard('{ArrowDown}{Enter}')
    expect(input).toHaveValue(modelName)
  }
  expect(window.open).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Open CC Switch' }))
  expect(window.open).toHaveBeenCalledTimes(1)
  const target = vi.mocked(window.open).mock.calls[0]?.[0]
  if (typeof target !== 'string') throw new Error('Expected a synthetic CC Switch URL')
  const url = new URL(target)
  expect(url.protocol).toBe('ccswitch:')
  expect(url.host).toBe('v1')
  expect(url.pathname).toBe('/import')
  expect(url.searchParams.get('resource')).toBe('provider')
  expect(url.searchParams.get('app')).toBe(app)
  const expectedNames = { claude: 'My Claude', codex: 'My Codex', gemini: 'Synthetic custom import name' }
  expect(url.searchParams.get('name')).toBe(expectedNames[app])
  expect(url.searchParams.get('endpoint')).toBe(app === 'codex' ? 'https://synthetic.gateway.invalid/v1' : 'https://synthetic.gateway.invalid')
  expect(url.searchParams.get('apiKey')).toBe('sk-synthetic-credential')
  expect(url.searchParams.get('model')).toBe(modelName)
  expect(url.searchParams.get('homepage')).toBe('https://synthetic.gateway.invalid')
  expect(url.searchParams.get('enabled')).toBe('true')
  if (app === 'claude') {
    expect(url.searchParams.get('haikuModel')).toBe(modelName)
    expect(url.searchParams.get('sonnetModel')).toBe(modelName)
    expect(url.searchParams.get('opusModel')).toBe(modelName)
  } else {
    expect(url.searchParams.has('haikuModel')).toBe(false)
    expect(url.searchParams.has('sonnetModel')).toBe(false)
    expect(url.searchParams.has('opusModel')).toBe(false)
  }
  expect(onOpenChange).toHaveBeenCalledWith(false)
})

it('CC Switch uses linked labels and one scrolling task body while Cancel never opens the external URL', async () => {
  const user = userEvent.setup()
  const onOpenChange = vi.fn()
  render(<QueryClientProvider client={client}><CCSwitchDialog open tokenKey='sk-synthetic-credential' onOpenChange={onOpenChange} /></QueryClientProvider>)
  const body = screen.getByRole('region', { name: 'CC Switch import task' })
  expect(body).toHaveClass('min-h-0', 'overflow-y-auto')
  expect(within(body).getByRole('combobox', { name: 'Name' })).toHaveClass('min-h-11')
  expect(body).not.toContainElement(screen.getByRole('heading', { name: 'Import to CC Switch' }))
  const cancel = screen.getByRole('button', { name: 'Cancel' })
  expect(cancel).toHaveClass('min-h-11')
  expect(body).not.toContainElement(cancel)
  await user.click(cancel)
  expect(window.open).not.toHaveBeenCalled()
  expect(onOpenChange).toHaveBeenCalledWith(false)
})
