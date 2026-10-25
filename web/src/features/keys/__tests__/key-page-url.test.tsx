/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { QueryClient } from '@tanstack/react-query'
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'
import { useInspectorStore } from '@/stores/inspector-store'

import {
  apiKeySearchSchema,
  keyPageSearchSchema,
  legacyKeyLinkTarget,
} from '../lib/keys-search'
import {
  KEY_21,
  KEY_22,
  mockKeysApi,
  newClient,
  renderKeysPage,
  requestLog,
  setViewport,
  signIn,
} from './keys-page-fixture'

const originalAuth = useAuthStore.getState()
let client: QueryClient
let mocks: ReturnType<typeof mockKeysApi>

beforeEach(() => {
  localStorage.clear()
  client = newClient()
  signIn()
  setViewport(1440)
  mocks = mockKeysApi({
    keys: [KEY_21, KEY_22],
    logs: [
      requestLog({ id: 3 }),
      requestLog({
        id: 2,
        type: 5,
        other: JSON.stringify({
          request_outcome: 'failed',
          usage_outcome: 'unknown',
        }),
        prompt_tokens: 0,
        completion_tokens: 0,
      }),
      requestLog({ id: 1, token_id: 22 }),
    ],
    status: {},
  })
})

afterEach(() => {
  client.clear()
  useAuthStore.setState(originalAuth)
  useInspectorStore.setState({ current: null })
  vi.restoreAllMocks()
  localStorage.clear()
})

/** What the breadcrumb and the mobile top bar's Back do. */
async function stepBack() {
  await waitFor(() =>
    expect(useInspectorStore.getState().current?.page).toBe(true)
  )
  act(() => useInspectorStore.getState().current?.close())
}

async function keyPage(name: string) {
  const page = await screen.findByTestId('key-page')
  await within(page).findByRole('heading', { level: 1, name })
  return page
}

describe('a Key opens as its own page', () => {
  test('clicking a Key opens its page on the requests tab, with only its own requests', async () => {
    const user = userEvent.setup()
    const { router } = await renderKeysPage(client)
    await user.click(
      await screen.findByRole('button', { name: /Desktop application/ })
    )
    await waitFor(() => expect(router.state.location.pathname).toBe('/keys/21'))
    const page = await keyPage('Desktop application')
    expect(within(page).getByRole('tab', { name: 'Requests' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    // Only this Key's requests, matched by its saved ID.
    expect(
      await within(page).findAllByRole('button', { name: /synthetic-model/ })
    ).toHaveLength(2)
    expect(useInspectorStore.getState().current?.title).toBe(
      'Desktop application'
    )
  })

  test('a click anywhere on a row opens the Key, but the row’s own controls do not', async () => {
    const user = userEvent.setup()
    const { router } = await renderKeysPage(client)
    await screen.findByRole('button', { name: /Shared device/ })
    const row = screen
      .getByRole('button', { name: /Shared device/ })
      .closest('tr') as HTMLElement
    await user.click(within(row).getByRole('checkbox'))
    expect(router.state.location.pathname).toBe('/keys')
    await user.click(
      row.querySelector('td[data-column-id="status"]') as HTMLElement
    )
    await waitFor(() => expect(router.state.location.pathname).toBe('/keys/22'))
  })

  test('the tab is kept in the URL, so a refresh or shared link reopens it', async () => {
    const user = userEvent.setup()
    const { router } = await renderKeysPage(client, '/keys/21')
    const page = await keyPage('Desktop application')
    await user.click(
      within(page).getByRole('tab', { name: 'Scope and shares' })
    )
    expect(router.state.location.search).toMatchObject({ tab: 'scope' })
    expect(await within(page).findByText('Access profile')).toBeVisible()
  })

  test('Back returns to the list with its filter, and focus on the Key’s row', async () => {
    const user = userEvent.setup()
    const { router } = await renderKeysPage(client, '/keys?filter=Shared')
    await user.click(
      await screen.findByRole('button', { name: /Shared device/ })
    )
    await keyPage('Shared device')
    expect(router.state.location.search).toMatchObject({
      keyReturn: { filter: 'Shared' },
    })
    await stepBack()
    await waitFor(() => expect(router.state.location.pathname).toBe('/keys'))
    expect(router.state.location.search).toMatchObject({ filter: 'Shared' })
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: /Shared device/ })
      ).toHaveFocus()
    )
  })

  test('Back from a shared Key link still returns to its list and row', async () => {
    const { router } = await renderKeysPage(
      client,
      '/keys/22?keyReturn=%7B%22filter%22%3A%22Shared%22%7D'
    )
    await keyPage('Shared device')
    await stepBack()
    await waitFor(() => expect(router.state.location.pathname).toBe('/keys'))
    expect(router.state.location.search).toMatchObject({
      filter: 'Shared',
      focusKeyId: 22,
    })
  })

  test('opening a request and pressing Back returns to the Key with focus on that request', async () => {
    const user = userEvent.setup()
    const { router } = await renderKeysPage(client)
    await user.click(
      await screen.findByRole('button', { name: /Desktop application/ })
    )
    const page = await keyPage('Desktop application')
    const first = (
      await within(page).findAllByRole('button', { name: /synthetic-model/ })
    )[0]
    await user.click(first)
    await waitFor(() =>
      expect(router.state.location.pathname).toBe('/keys/21/requests/REQ-3')
    )
    expect(await screen.findByTestId('request-page')).toBeVisible()
    await stepBack()
    await waitFor(() => expect(router.state.location.pathname).toBe('/keys/21'))
    await waitFor(() =>
      expect(
        within(screen.getByTestId('key-page')).getAllByRole('button', {
          name: /synthetic-model/,
        })[0]
      ).toHaveFocus()
    )
  })

  test('a request opened from a Key sits under it in the breadcrumb', async () => {
    await renderKeysPage(client, '/keys/21/requests/REQ-3')
    expect(await screen.findByTestId('request-page')).toBeVisible()
    await waitFor(() =>
      expect(useInspectorStore.getState().current?.parents).toEqual([
        { title: 'Desktop application', open: expect.any(Function) },
      ])
    )
    expect(useInspectorStore.getState().current?.title).toBe('Request REQ-3')
  })

  test('a request of another Key opened under this Key says so', async () => {
    await renderKeysPage(client, '/keys/21/requests/REQ-1')
    expect(
      await screen.findByText('This request was made with a different Key.')
    ).toBeVisible()
  })

  test('below 1024px the Key page fills the screen and the top bar takes Back', async () => {
    setViewport(390)
    const user = userEvent.setup()
    const { router } = await renderKeysPage(client)
    await user.click(
      await screen.findByRole('button', { name: /Desktop application/ })
    )
    await keyPage('Desktop application')
    expect(
      screen.queryByRole('button', { name: /Shared device/ })
    ).not.toBeInTheDocument()
    expect(useInspectorStore.getState().current?.title).toBe(
      'Desktop application'
    )
    await stepBack()
    await waitFor(() => expect(router.state.location.pathname).toBe('/keys'))
    expect(
      await screen.findByRole('button', { name: /Shared device/ })
    ).toBeVisible()
  })
})

describe('phone first screen', () => {
  test('the requests tab opens straight onto the Key’s request rows, with no text filters', async () => {
    setViewport(390)
    await renderKeysPage(client, '/keys/21')
    const page = await keyPage('Desktop application')
    const panel = await within(page).findByRole('tabpanel')
    await within(panel).findAllByRole('button', { name: /synthetic-model/ })
    expect(within(page).getByRole('tab', { name: 'Requests' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    const firstBlock = panel.firstElementChild
    expect(firstBlock?.querySelector('[data-request-row]')).not.toBeNull()
    expect(within(panel).queryByRole('textbox')).not.toBeInTheDocument()
  })
})

describe('returning from the call records', () => {
  test('on a phone the Key the records were opened from takes focus', async () => {
    setViewport(390)
    await renderKeysPage(client, '/keys?focusKeyId=21')
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: /Desktop application/ })
      ).toHaveFocus()
    )
  })
})

describe('earlier inspector links', () => {
  test('a shared request link opens the request page with its body tab', async () => {
    const { router } = await renderKeysPage(
      client,
      '/keys?inspect=21&request=REQ-2&requestTab=body'
    )
    const page = await screen.findByTestId('request-page')
    expect(router.state.location.pathname).toBe('/keys/21/requests/REQ-2')
    expect(
      await within(page).findByRole('tab', { name: 'Body' })
    ).toHaveAttribute('aria-selected', 'true')
    expect(within(page).getAllByText('REQ-2')[0]).toBeVisible()
    const grid = within(page).getByTestId('request-result-grid')
    expect(grid).toHaveTextContent('Request failed')
    // A missing usage report is "not provided", never zero tokens.
    expect(grid).toHaveTextContent('Usage not provided')
  })

  test('the phone top bar steps out of a shared request one level at a time', async () => {
    setViewport(390)
    const { router } = await renderKeysPage(
      client,
      '/keys?inspect=21&request=REQ-3'
    )
    expect(await screen.findByTestId('request-page')).toBeVisible()
    await waitFor(() =>
      expect(useInspectorStore.getState().current?.title).toBe('Request REQ-3')
    )
    await stepBack()
    await waitFor(() => expect(router.state.location.pathname).toBe('/keys/21'))
    await waitFor(() =>
      expect(useInspectorStore.getState().current?.title).toBe(
        'Desktop application'
      )
    )
    await stepBack()
    await waitFor(() => expect(router.state.location.pathname).toBe('/keys'))
  })

  test.each([
    ['/keys?inspect=21', '/keys/21', {}],
    [
      '/keys?keyId=21&filter=same',
      '/keys/21',
      { keyReturn: { filter: 'same' } },
    ],
    ['/keys?inspect=21&tab=access', '/keys/21', { tab: 'scope' }],
    ['/keys?inspect=21&tab=shares', '/keys/21', { tab: 'scope' }],
    [
      '/keys?inspect=21&request=REQ-3&requestTab=sse',
      '/keys/21/requests/REQ-3',
      { tab: 'sse' },
    ],
    [
      '/keys?inspect=21&request=REQ-3&requestTab=diagnosis',
      '/keys/21/requests/REQ-3',
      {},
    ],
  ])('%s now opens %s', async (entry, pathname, search) => {
    const { router } = await renderKeysPage(client, entry)
    await waitFor(() => expect(router.state.location.pathname).toBe(pathname))
    expect(router.state.location.search).toEqual(search)
  })

  test('a legacy record without a Request ID opens over its Key page', async () => {
    mocks.get.mockRestore()
    mockKeysApi({
      keys: [KEY_21],
      logs: [requestLog({ id: 9, request_id: '' })],
      status: {},
    })
    const user = userEvent.setup()
    const { router } = await renderKeysPage(client, '/keys/21')
    const page = await keyPage('Desktop application')
    await user.click(
      await within(page).findByRole('button', { name: /synthetic-model/ })
    )
    await waitFor(() =>
      expect(router.state.location.search).toMatchObject({ requestLog: 9 })
    )
    const dialog = await screen.findByRole('dialog', { name: /Log Details/ })
    expect(dialog).toBeVisible()
    await user.click(within(dialog).getByRole('button', { name: 'Close' }))
    await waitFor(() =>
      expect(router.state.location.search).not.toHaveProperty('requestLog')
    )
  })

  test.each(['/keys/abc', '/keys/0'])(
    'an invalid Key address %s returns to the list',
    async (entry) => {
      const { router } = await renderKeysPage(client, entry)
      await waitFor(() => expect(router.state.location.pathname).toBe('/keys'))
    }
  )
})

describe('Key search parameters', () => {
  test('an older keyId link becomes the same Key page with the list context', () => {
    const search = apiKeySearchSchema.parse({ keyId: '21', filter: 'same' })
    expect(legacyKeyLinkTarget(search)).toEqual({
      page: 'key',
      keyId: 21,
      search: {
        tab: undefined,
        requestLog: undefined,
        keyReturn: { filter: 'same' },
      },
    })
    expect(legacyKeyLinkTarget(apiKeySearchSchema.parse({}))).toBeUndefined()
  })

  test.each([
    [{ inspect: 0 }],
    [{ inspect: 'abc' }],
    [{ tab: 'billing' }],
    [{ requestTab: 'raw' }],
    [{ request: 'x'.repeat(200) }],
  ])(
    'rejects an invalid earlier inspector value %o instead of guessing',
    (input) => {
      const parsed = apiKeySearchSchema.parse(input)
      expect(parsed.inspect).toBeUndefined()
      expect(parsed.tab).toBeUndefined()
      expect(parsed.requestTab).toBeUndefined()
      expect(parsed.request).toBeUndefined()
      expect(legacyKeyLinkTarget(parsed)).toBeUndefined()
    }
  )

  test('the Key page ignores unknown tabs and invalid record IDs', () => {
    expect(
      keyPageSearchSchema.parse({ tab: 'access', requestLog: -1 })
    ).toEqual({ tab: undefined, requestLog: undefined, keyReturn: undefined })
  })
})
