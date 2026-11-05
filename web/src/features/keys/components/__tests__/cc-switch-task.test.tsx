import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { CCSwitchDialog } from '../dialogs/cc-switch-dialog'

const originalAuth = useAuthStore.getState()
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
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })

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

it('a selected Key only offers its own candidates and does not choose the first one', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get).mockResolvedValue({ data: { success: true, data: ['allowed', 'other'] } })
  render(<QueryClientProvider client={client}><CCSwitchDialog open tokenKey='synthetic-credential' apiKey={{ model_limits_enabled: true, model_limits: 'allowed' }} onOpenChange={() => {}} /></QueryClientProvider>)
  const primary = screen.getByRole('combobox', { name: 'Primary Model *' })
  await user.click(primary)
  expect(await screen.findByRole('option', { name: 'allowed' })).toBeVisible()
  expect(screen.queryByRole('option', { name: 'other' })).not.toBeInTheDocument()
  expect(primary).toHaveValue('')
  expect(screen.getByText('Candidates follow this Key’s restrictions. Protocol support and upstream availability have not been tested.')).toBeVisible()
  expect(window.open).not.toHaveBeenCalled()
})

it('a versioned server setting keeps the existing Codex import endpoint contract without a duplicate suffix', async () => {
  localStorage.setItem('status', JSON.stringify({ server_address: ' https://synthetic.gateway.invalid/root/v1/// ' }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={client}><CCSwitchDialog open tokenKey='synthetic-credential' onOpenChange={() => {}} /></QueryClientProvider>)
  await user.click(screen.getByRole('radio', { name: 'Codex' }))
  await user.click(screen.getByRole('combobox', { name: 'Primary Model *' }))
  await screen.findByRole('option', { name: modelName })
  await user.keyboard('{ArrowDown}{Enter}')
  await user.click(screen.getByRole('button', { name: 'Open CC Switch' }))
  const target = vi.mocked(window.open).mock.calls[0]?.[0]
  if (typeof target !== 'string') throw new Error('Expected a synthetic CC Switch URL')
  expect(new URL(target).searchParams.get('endpoint')).toBe('https://synthetic.gateway.invalid/root/v1')
  expect(new URL(target).searchParams.get('homepage')).toBe('https://synthetic.gateway.invalid/root')
})


it.each(['', null])('an enabled empty or null Key scope %s never falls back to the full user model directory', async (modelLimits) => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={client}><CCSwitchDialog open tokenKey='synthetic-credential' apiKey={{ model_limits_enabled: true, model_limits: modelLimits }} onOpenChange={() => {}} /></QueryClientProvider>)
  await waitFor(() => expect(api.get).toHaveBeenCalledWith('/api/user/models', expect.objectContaining({ signal: expect.any(AbortSignal), disableDuplicate: true, skipErrorHandler: true, skipBusinessError: true, isCurrentScope: expect.any(Function) })))
  const primary = screen.getByRole('combobox', { name: 'Primary Model *' })
  await user.click(primary)
  expect(screen.queryByRole('option', { name: modelName })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Open CC Switch' }))
  expect(window.open).not.toHaveBeenCalled()
})

it('changing actor closes the old secret-bearing import and a fresh actor reads a separate directory', async () => {
  useAuthStore.setState((state) => ({ ...state, auth: { ...state.auth, user: { id: 1, role: 1, username: 'first' }, session: null } }))
  const renderDialog = (identity: string, secret: string) => <QueryClientProvider client={client}><CCSwitchDialog key={identity} open tokenKey={secret} onOpenChange={() => {}} /></QueryClientProvider>
  const view = render(renderDialog('first', 'previous-secret'))
  const user = userEvent.setup()
  await user.click(screen.getByRole('combobox', { name: 'Primary Model *' }))
  await screen.findByRole('option', { name: modelName })
  await user.keyboard('{ArrowDown}{Enter}')
  act(() => useAuthStore.setState((state) => ({ ...state, auth: { ...state.auth, user: { id: 2, role: 1, username: 'next' } } })))
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  vi.mocked(api.get).mockResolvedValue({ data: { success: true, data: ['next-actor-model'] } })
  view.rerender(renderDialog('next', 'next-secret'))
  await user.click(screen.getByRole('combobox', { name: 'Primary Model *' }))
  expect(await screen.findByRole('option', { name: 'next-actor-model' })).toBeVisible()
  expect(screen.queryByRole('option', { name: modelName })).not.toBeInTheDocument()
  expect(screen.getByRole('combobox', { name: 'Primary Model *' })).toHaveValue('')
  expect(window.open).not.toHaveBeenCalled()
})

it.each([undefined, null, 'not-a-list', { models: [] }, ['valid-model', 42]])('a successful response with malformed directory %j is an error rather than a fabricated empty catalog', async (data) => {
  vi.mocked(api.get).mockResolvedValue({ data: { success: true, data } })
  render(<QueryClientProvider client={client}><CCSwitchDialog open tokenKey='synthetic-credential' onOpenChange={() => {}} /></QueryClientProvider>)
  expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load available models')
  expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
  expect(window.open).not.toHaveBeenCalled()
})

it('an explicit manual model ID can still be imported when the catalog fails without testing an upstream', async () => {
  vi.mocked(api.get).mockRejectedValue(new Error('Synthetic catalog unavailable'))
  const user = userEvent.setup()
  render(<QueryClientProvider client={client}><CCSwitchDialog open tokenKey='synthetic-credential' onOpenChange={() => {}} /></QueryClientProvider>)
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic catalog unavailable')
  const submit = screen.getByRole('button', { name: 'Open CC Switch' })
  expect(submit).toBeDisabled()
  await user.type(screen.getByRole('combobox', { name: 'Primary Model *' }), 'manual-model-id')
  expect(submit).toBeEnabled()
  await user.click(submit)
  const target = vi.mocked(window.open).mock.calls[0]?.[0]
  if (typeof target !== 'string') throw new Error('Expected a synthetic CC Switch URL')
  expect(new URL(target).searchParams.get('model')).toBe('manual-model-id')
  expect(api.get).toHaveBeenCalledExactlyOnceWith('/api/user/models', expect.objectContaining({ signal: expect.any(AbortSignal), disableDuplicate: true, skipErrorHandler: true, skipBusinessError: true, isCurrentScope: expect.any(Function) }))
})

it('a manual model ID outside the Key restriction cannot be imported while an allowed uncatalogued ID can', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={client}><CCSwitchDialog open tokenKey='synthetic-credential' apiKey={{ model_limits_enabled: true, model_limits: 'allowed-manual-model' }} onOpenChange={() => {}} /></QueryClientProvider>)
  const primary = screen.getByRole('combobox', { name: 'Primary Model *' })
  await user.type(primary, 'outside-model')
  expect(screen.getByRole('button', { name: 'Open CC Switch' })).toBeDisabled()
  expect(screen.getByText('One or more model IDs are outside this Key’s restrictions.')).toBeVisible()
  expect(window.open).not.toHaveBeenCalled()
  await user.clear(primary)
  await user.type(primary, 'allowed-manual-model')
  expect(screen.getByRole('button', { name: 'Open CC Switch' })).toBeEnabled()
  await user.click(screen.getByRole('button', { name: 'Open CC Switch' }))
  expect(window.open).toHaveBeenCalledOnce()
})
