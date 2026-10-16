/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { apiKeySchema } from '../../types'
import { ApiKeysProvider } from '../api-keys-provider'
import { ModelScopeDialog } from '../model-scope-dialog'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))

const revisionA = 'a'.repeat(64)
const revisionB = 'b'.repeat(64)
const key = apiKeySchema.parse({
  id: 21,
  name: 'Desktop application',
  key: 'masked',
  status: 1,
  remain_quota: 100,
  used_quota: 0,
  unlimited_quota: true,
  expired_time: -1,
  created_time: 1,
  accessed_time: 1,
  group: 'default',
  model_limits_enabled: false,
})
const originalAuth = useAuthStore.getState()
let client: QueryClient

const scopeEnvelope = (
  model_limits_enabled: boolean,
  models: string[],
  revision = revisionA
) => ({
  data: {
    success: true,
    data: {
      token_id: 21,
      token_name: 'Desktop application',
      model_limits_enabled,
      models,
      revision,
    },
  },
})

function renderDialog(onClose = vi.fn()) {
  render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <ApiKeysProvider>
          <ModelScopeDialog apiKey={key} onClose={onClose} />
        </ApiKeysProvider>
      </TooltipProvider>
    </QueryClientProvider>
  )
  return onClose
}

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useAuthStore.setState((state) => ({
    ...state,
    auth: {
      ...state.auth,
      user: { id: 1, role: 100, username: 'owner' },
      session: null,
    },
  }))
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState(originalAuth)
  vi.restoreAllMocks()
})

test('an existing empty enabled scope is shown as explicit block-all state', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/token/21/model-limits') return scopeEnvelope(true, [])
    if (url === '/api/user/models') {
      return { data: { success: true, data: [] } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  const put = vi.spyOn(api, 'put')
  renderDialog()
  expect(
    await screen.findByRole('radio', { name: /Block all models/ })
  ).toBeChecked()
  expect(
    screen.getByRole('button', { name: 'Save model scope' })
  ).toBeDisabled()
  expect(put).not.toHaveBeenCalled()
})

test('a failed catalog keeps unavailable and long saved model IDs visible', async () => {
  const user = userEvent.setup()
  const longModel = `retired-${'x'.repeat(180)}`
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/token/21/model-limits') {
      return scopeEnvelope(true, ['', longModel])
    }
    if (url === '/api/user/models') throw new Error('catalog unavailable')
    throw new Error(`Unexpected GET ${url}`)
  })
  const put = vi.spyOn(api, 'put')
  const onClose = renderDialog()
  expect(await screen.findByText(longModel)).toBeVisible()
  expect(screen.getByText('Empty model ID')).toBeVisible()
  expect(screen.getAllByText('Preserved model ID')).toHaveLength(2)
  expect(
    screen.getByText('Could not load the current model catalog')
  ).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(onClose).toHaveBeenCalledOnce()
  expect(put).not.toHaveBeenCalled()
  expect(get).toHaveBeenCalledWith(
    '/api/user/models',
    expect.objectContaining({ params: { group: 'default' } })
  )
})

test('a failed scope read can be retried while an empty catalog preserves saved IDs', async () => {
  const user = userEvent.setup()
  let scopeReads = 0
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/token/21/model-limits') {
      scopeReads += 1
      if (scopeReads === 1) throw new Error('scope unavailable')
      return scopeEnvelope(true, ['retired-model'])
    }
    if (url === '/api/user/models') {
      return { data: { success: true, data: [] } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  renderDialog()
  expect(await screen.findByText('Could not load model scope')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Retry' }))

  expect(await screen.findByText('retired-model')).toBeVisible()
  expect(
    screen.getByText('No additional catalog models are available')
  ).toBeVisible()
  expect(scopeReads).toBe(2)
})

test('saving selected models sends only the narrow command and closes after matching readback', async () => {
  const user = userEvent.setup()
  let current = scopeEnvelope(false, ['retained-while-off'])
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/token/21/model-limits') return current
    if (url === '/api/user/models') {
      return { data: { success: true, data: ['gpt-5-codex'] } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => {
    const command = body as {
      model_limits_enabled: boolean
      models: string[]
    }
    current = scopeEnvelope(
      command.model_limits_enabled,
      command.models,
      revisionB
    )
    return current
  })
  const onClose = renderDialog()
  await user.click(
    await screen.findByRole('radio', { name: /Only selected models/ })
  )
  await user.click(screen.getByRole('button', { name: /gpt-5-codex/ }))
  await user.click(screen.getByRole('button', { name: 'Save model scope' }))
  await waitFor(() => expect(onClose).toHaveBeenCalledOnce())
  expect(put).toHaveBeenCalledWith(
    '/api/token/21/model-limits',
    {
      expected_revision: revisionA,
      model_limits_enabled: true,
      models: ['retained-while-off', 'gpt-5-codex'],
    },
    expect.objectContaining({ skipAuthRefresh: true })
  )
  expect(
    get.mock.calls.filter(([url]) => url === '/api/token/21/model-limits')
  ).toHaveLength(2)
})

test('a lost write response is confirmed by GET without replaying the write', async () => {
  const user = userEvent.setup()
  let current = scopeEnvelope(false, ['retained-while-off'])
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/token/21/model-limits') return current
    if (url === '/api/user/models') {
      return { data: { success: true, data: ['gpt-5-codex'] } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => {
    const command = body as {
      model_limits_enabled: boolean
      models: string[]
    }
    current = scopeEnvelope(
      command.model_limits_enabled,
      command.models,
      revisionB
    )
    throw new Error('synthetic response loss after commit')
  })
  const onClose = renderDialog()
  await user.click(
    await screen.findByRole('radio', { name: /Only selected models/ })
  )
  await user.click(screen.getByRole('button', { name: /gpt-5-codex/ }))
  await user.click(screen.getByRole('button', { name: 'Save model scope' }))

  await waitFor(() => expect(onClose).toHaveBeenCalledOnce())
  expect(put).toHaveBeenCalledOnce()
  expect(
    get.mock.calls.filter(([url]) => url === '/api/token/21/model-limits')
  ).toHaveLength(2)
})

test('an unconfirmed save stays write-locked through edits until a fresh read succeeds', async () => {
  const user = userEvent.setup()
  let scopeReads = 0
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/token/21/model-limits') {
      scopeReads += 1
      if (scopeReads === 2) throw new Error('synthetic readback failure')
      return scopeEnvelope(false, ['retained-while-off'], revisionB)
    }
    if (url === '/api/user/models') {
      return { data: { success: true, data: ['gpt-5-codex'] } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue(
      scopeEnvelope(true, ['retained-while-off', 'gpt-5-codex'], revisionB)
    )
  renderDialog()
  await user.click(
    await screen.findByRole('radio', { name: /Only selected models/ })
  )
  await user.click(screen.getByRole('button', { name: /gpt-5-codex/ }))
  await user.click(screen.getByRole('button', { name: 'Save model scope' }))

  expect(
    await screen.findByText('The save result could not be confirmed')
  ).toBeVisible()
  const save = screen.getByRole('button', { name: 'Save model scope' })
  expect(save).toBeDisabled()
  await user.click(screen.getByRole('radio', { name: /Block all models/ }))
  await user.click(
    screen.getByRole('checkbox', {
      name: /I understand that this Key will be unable/,
    })
  )
  expect(save).toBeDisabled()
  expect(put).toHaveBeenCalledOnce()

  await user.click(screen.getByRole('button', { name: 'Reload current scope' }))
  await waitFor(() =>
    expect(
      screen.queryByText('The save result could not be confirmed')
    ).not.toBeInTheDocument()
  )
  expect(scopeReads).toBe(3)
  await user.click(screen.getByRole('radio', { name: /Only selected models/ }))
  expect(save).toBeEnabled()
  expect(put).toHaveBeenCalledOnce()
})

test('block-all requires an explicit confirmation and sends an empty model list', async () => {
  const user = userEvent.setup()
  let current = scopeEnvelope(false, ['preserved-model'])
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/token/21/model-limits') return current
    if (url === '/api/user/models') {
      return { data: { success: true, data: [] } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  const put = vi.spyOn(api, 'put').mockImplementation(async (_url, body) => {
    const command = body as {
      model_limits_enabled: boolean
      models: string[]
    }
    current = scopeEnvelope(
      command.model_limits_enabled,
      command.models,
      revisionB
    )
    return current
  })
  const onClose = renderDialog()
  await user.click(
    await screen.findByRole('radio', { name: /Block all models/ })
  )
  const save = screen.getByRole('button', { name: 'Save model scope' })
  expect(save).toBeDisabled()
  await user.click(
    screen.getByRole('checkbox', {
      name: /I understand that this Key will be unable/,
    })
  )
  expect(save).toBeEnabled()
  await user.click(save)
  await waitFor(() => expect(onClose).toHaveBeenCalledOnce())
  expect(put.mock.calls[0][1]).toEqual({
    expected_revision: revisionA,
    model_limits_enabled: true,
    models: [],
  })
})

test('a conflict reloads and displays the current server scope without retrying the write', async () => {
  const user = userEvent.setup()
  let reads = 0
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/token/21/model-limits') {
      reads += 1
      return reads === 1
        ? scopeEnvelope(false, [])
        : scopeEnvelope(true, ['server-current'], revisionB)
    }
    if (url === '/api/user/models') {
      return { data: { success: true, data: ['client-choice'] } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  const put = vi.spyOn(api, 'put').mockRejectedValue(
    Object.assign(new Error('conflict'), {
      isAxiosError: true,
      response: { status: 409 },
    })
  )
  renderDialog()
  await user.click(
    await screen.findByRole('radio', { name: /Only selected models/ })
  )
  await user.click(screen.getByRole('button', { name: /client-choice/ }))
  await user.click(screen.getByRole('button', { name: 'Save model scope' }))
  expect(
    await screen.findByText('The model scope changed in another operation')
  ).toBeVisible()
  expect(screen.getByText('server-current')).toBeVisible()
  expect(put).toHaveBeenCalledOnce()
})

test('changing the signed-in identity never shows the previous session model scope', async () => {
  let resolveSecond!: (value: ReturnType<typeof scopeEnvelope>) => void
  const second = new Promise<ReturnType<typeof scopeEnvelope>>((resolve) => {
    resolveSecond = resolve
  })
  let scopeReads = 0
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/token/21/model-limits') {
      scopeReads += 1
      if (scopeReads === 1) return scopeEnvelope(true, ['first-user-private'])
      return second
    }
    if (url === '/api/user/models') {
      return { data: { success: true, data: [] } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  renderDialog()
  expect(await screen.findByText('first-user-private')).toBeVisible()
  act(() => {
    useAuthStore.setState((state) => ({
      ...state,
      auth: {
        ...state.auth,
        user: { id: 2, role: 100, username: 'other-owner' },
        session: null,
      },
    }))
  })
  await waitFor(() =>
    expect(screen.queryByText('first-user-private')).not.toBeInTheDocument()
  )
  expect(screen.getByText('Loading model scope...')).toBeInTheDocument()
  await act(async () => {
    resolveSecond(scopeEnvelope(true, ['second-user-model'], revisionB))
  })
  expect(await screen.findByText('second-user-model')).toBeVisible()
})
