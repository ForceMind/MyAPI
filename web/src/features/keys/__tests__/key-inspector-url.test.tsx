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

import { apiKeySearchSchema, legacyKeyLinkTarget } from '../lib/keys-search'
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

beforeEach(() => {
  localStorage.clear()
  client = newClient()
  signIn()
  setViewport(1440)
  mockKeysApi({
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

describe('Key inspector state lives in the URL', () => {
  test('clicking a Key opens its recent requests beside the list and writes inspect to the URL', async () => {
    const user = userEvent.setup()
    const { router } = await renderKeysPage(client)
    const row = await screen.findByRole('button', {
      name: /Desktop application/,
    })
    await user.click(row)
    await waitFor(() =>
      expect(router.state.location.search).toMatchObject({ inspect: 21 })
    )
    const inspector = await screen.findByRole('complementary', {
      name: 'API Key #21',
    })
    expect(
      within(inspector).getByRole('tab', { name: 'Recent requests' })
    ).toHaveAttribute('aria-selected', 'true')
    expect(row).toHaveAttribute('aria-expanded', 'true')
    // Only this Key's requests, matched by its saved ID.
    expect(
      await within(inspector).findAllByRole('button', {
        name: /synthetic-model/,
      })
    ).toHaveLength(2)
  })

  test('a shared link restores the Key, the request and its body tab', async () => {
    await renderKeysPage(
      client,
      '/keys?inspect=21&request=REQ-2&requestTab=body'
    )
    // The inspector is named after the open request inside the Key.
    const inspector = await screen.findByRole('complementary', {
      name: 'Request REQ-2',
    })
    expect(
      await within(inspector).findByRole('button', {
        name: /Back to Desktop application/,
      })
    ).toBeVisible()
    expect(
      await within(inspector).findByRole('tab', { name: 'Body' })
    ).toHaveAttribute('aria-selected', 'true')
    expect(within(inspector).getByText('REQ-2')).toBeVisible()
    expect(within(inspector).getByText('Request failed')).toBeVisible()
    // A missing usage report is "not provided", never zero tokens.
    expect(within(inspector).getByText('Usage not provided')).toBeVisible()
  })

  test('opening a request and pressing Back returns to the Key with focus on that request', async () => {
    const user = userEvent.setup()
    const { router } = await renderKeysPage(client)
    await user.click(
      await screen.findByRole('button', { name: /Desktop application/ })
    )
    const inspector = await screen.findByRole('complementary', {
      name: 'API Key #21',
    })
    const first = (
      await within(inspector).findAllByRole('button', {
        name: /synthetic-model/,
      })
    )[0]
    await user.click(first)
    await waitFor(() =>
      expect(router.state.location.search).toMatchObject({
        inspect: 21,
        request: 'REQ-3',
      })
    )
    expect(await screen.findByTestId('request-inspector')).toBeVisible()
    await user.click(
      screen.getByRole('button', { name: /Back to Desktop application/ })
    )
    await waitFor(() =>
      expect(router.state.location.search).not.toHaveProperty('request')
    )
    await waitFor(() =>
      expect(
        within(
          screen.getByRole('complementary', { name: 'API Key #21' })
        ).getAllByRole('button', { name: /synthetic-model/ })[0]
      ).toHaveFocus()
    )
  })

  test('closing the inspector returns focus to the Key row it was opened from', async () => {
    const user = userEvent.setup()
    const { router } = await renderKeysPage(client)
    const row = await screen.findByRole('button', { name: /Shared device/ })
    await user.click(row)
    await screen.findByRole('complementary', { name: 'API Key #22' })
    await user.click(screen.getByRole('button', { name: 'Close details' }))
    await waitFor(() =>
      expect(router.state.location.search).not.toHaveProperty('inspect')
    )
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: /Shared device/ })
      ).toHaveFocus()
    )
  })

  test('below 1024px the inspector replaces the list and the top bar takes Back', async () => {
    setViewport(390)
    const user = userEvent.setup()
    const { router } = await renderKeysPage(client)
    await user.click(
      await screen.findByRole('button', { name: /Desktop application/ })
    )
    const page = await screen.findByRole('region', { name: 'API Key #21' })
    expect(page).toHaveAttribute('data-inspector', 'page')
    // The list stays mounted (filters and scroll survive) but is hidden.
    expect(
      screen.getByRole('button', { name: /Shared device/, hidden: true })
    ).not.toBeVisible()
    expect(useInspectorStore.getState().current?.title).toBe('API Key #21')
    act(() => useInspectorStore.getState().current?.close())
    await waitFor(() =>
      expect(router.state.location.search).not.toHaveProperty('inspect')
    )
    expect(
      await screen.findByRole('button', { name: /Shared device/ })
    ).toBeVisible()
  })
})

describe('phone first screen', () => {
  test('a Key opens straight onto its request rows, with no filters or totals above them', async () => {
    setViewport(390)
    await renderKeysPage(client, '/keys?inspect=21')
    const page = await screen.findByRole('region', { name: 'API Key #21' })
    const panel = await within(page).findByRole('tabpanel')
    await within(panel).findAllByRole('button', { name: /synthetic-model/ })
    // The default tab is the requests, and its first block is the list.
    expect(
      within(page).getByRole('tab', { name: 'Recent requests' })
    ).toHaveAttribute('aria-selected', 'true')
    const firstBlock = panel.firstElementChild?.firstElementChild
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

describe('phone Back steps out one level at a time', () => {
  test('from a request the top bar returns to its Key, then to the list', async () => {
    setViewport(390)
    const { router } = await renderKeysPage(
      client,
      '/keys?inspect=21&request=REQ-3'
    )
    expect(await screen.findByTestId('request-inspector')).toBeVisible()
    // On a phone the top bar carries Back; the panel repeats none.
    expect(
      screen.queryByRole('button', { name: /Back to Desktop application/ })
    ).not.toBeInTheDocument()
    await waitFor(() =>
      expect(useInspectorStore.getState().current?.title).toBe('Request REQ-3')
    )
    act(() => useInspectorStore.getState().current?.close())
    await waitFor(() =>
      expect(router.state.location.search).toEqual({ inspect: 21 })
    )
    await waitFor(() =>
      expect(useInspectorStore.getState().current?.title).toBe('API Key #21')
    )
    act(() => useInspectorStore.getState().current?.close())
    await waitFor(() =>
      expect(router.state.location.search).not.toHaveProperty('inspect')
    )
  })
})

describe('Key search parameters', () => {
  test('an older keyId link becomes the same Key inspector', () => {
    const search = apiKeySearchSchema.parse({ keyId: '21', filter: 'same' })
    expect(legacyKeyLinkTarget(search)).toMatchObject({
      inspect: 21,
      keyId: undefined,
      filter: 'same',
    })
    expect(legacyKeyLinkTarget(apiKeySearchSchema.parse({}))).toBeUndefined()
  })

  test.each([
    [{ inspect: 0 }, {}],
    [{ inspect: 'abc' }, {}],
    [{ tab: 'billing' }, {}],
    [{ requestTab: 'raw' }, {}],
    [{ request: 'x'.repeat(200) }, {}],
  ])(
    'rejects an invalid inspector value %o instead of guessing',
    (input, expected) => {
      const parsed = apiKeySearchSchema.parse(input)
      expect(parsed).toMatchObject(expected)
      expect(parsed.inspect).toBeUndefined()
      expect(parsed.tab).toBeUndefined()
      expect(parsed.requestTab).toBeUndefined()
      expect(parsed.request).toBeUndefined()
    }
  )
})
