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
import i18next from 'i18next'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useInspectorStore } from '@/stores/inspector-store'

import {
  KEY_21,
  mockKeysApi,
  newClient,
  renderKeysPage,
  requestLog,
  setViewport,
  signIn,
} from './keys-page-fixture'

const originalAuth = useAuthStore.getState()
const completedWithoutUsage = requestLog({
  id: 11,
  type: 5,
  content: 'completed without usage',
  prompt_tokens: 0,
  completion_tokens: 0,
  request_id: 'req-complete',
  other: JSON.stringify({
    request_outcome: 'completed',
    usage_outcome: 'missing',
    settlement_status: 'pending_review',
  }),
})
const failed = requestLog({
  id: 10,
  type: 5,
  content: 'Upstream rate limited',
  prompt_tokens: 0,
  completion_tokens: 0,
  request_id: 'req-failed',
  other: JSON.stringify({
    request_outcome: 'failed',
    usage_outcome: 'missing',
  }),
})
let client: QueryClient
let logs: ReturnType<typeof requestLog>[]

beforeEach(() => {
  localStorage.clear()
  client = newClient()
  signIn(1)
  setViewport(1440)
  logs = [completedWithoutUsage, failed]
})

afterEach(() => {
  client.clear()
  useAuthStore.setState(originalAuth)
  useInspectorStore.setState({ current: null })
  vi.restoreAllMocks()
  localStorage.clear()
})

async function openKey() {
  const mocks = mockKeysApi({ keys: [KEY_21], logs, status: {} })
  const result = await renderKeysPage(client, '/keys/21')
  const inspector = await screen.findByTestId('key-page')
  return { ...result, mocks, inspector }
}

/** What the breadcrumb and the mobile top bar's Back do. */
async function stepBack() {
  await waitFor(() =>
    expect(useInspectorStore.getState().current?.page).toBe(true)
  )
  act(() => useInspectorStore.getState().current?.close())
}

test('a completed request without a usage report is successful and shows no invented tokens', async () => {
  const user = userEvent.setup()
  const { inspector, mocks } = await openKey()
  const rows = await within(inspector).findAllByRole('button', {
    name: /synthetic-model/,
  })
  expect(within(rows[0]).getByText('Request successful')).toBeVisible()
  expect(within(rows[0]).getByText('Usage not provided')).toBeVisible()
  expect(within(rows[0]).queryByText(/0 Tokens/)).not.toBeInTheDocument()
  await user.click(rows[0])
  const detail = await screen.findByTestId('request-page')
  expect(
    (await within(detail).findAllByText('Usage not provided')).length
  ).toBeGreaterThan(0)
  expect(
    within(detail).getByRole('button', { name: 'View this Key' })
  ).toHaveAttribute('href', '/keys/21')
  expect(screen.queryByText('Current Balance')).not.toBeInTheDocument()
  // An ordinary user reads only its own call records for this Key.
  const logCall = mocks.get.mock.calls
    .map(([url]) => String(url))
    .find((url) => url.startsWith('/api/log'))
  expect(logCall).toContain('/api/log/self?')
  expect(logCall).toContain('token_id=21')
})

test('a failed request opens by keyboard and Back returns focus to it', async () => {
  const user = userEvent.setup()
  const { inspector } = await openKey()
  const rows = await within(inspector).findAllByRole('button', {
    name: /synthetic-model/,
  })
  rows[1].focus()
  await user.keyboard('{Enter}')
  const detail = await screen.findByTestId('request-page')
  expect(
    (await within(detail).findAllByText('Request failed')).length
  ).toBeGreaterThan(0)
  await user.click(within(detail).getByRole('tab', { name: 'Diagnosis' }))
  expect(within(detail).getByText('Upstream rate limited')).toBeVisible()
  await stepBack()
  await waitFor(() =>
    expect(
      within(screen.getByTestId('key-page')).getAllByRole('button', {
        name: /synthetic-model/,
      })[1]
    ).toHaveFocus()
  )
})

test('a row for another user in the response is never shown as this Key’s request', async () => {
  logs = [{ ...completedWithoutUsage, user_id: 99 }]
  const { inspector } = await openKey()
  expect(
    await within(inspector).findByText('No requests from this Key yet')
  ).toBeVisible()
  expect(
    within(inspector).queryByRole('button', { name: /synthetic-model/ })
  ).not.toBeInTheDocument()
})

test('a failed read offers a retry and an empty answer then shows the empty state', async () => {
  const mocks = mockKeysApi({ keys: [KEY_21], logs: [], status: {} })
  const original = mocks.get.getMockImplementation()
  if (!original) throw new Error('fixture has no GET handler')
  let failures = 1
  mocks.get.mockImplementation(async (url, config) => {
    // The Key's latest requests (not its 24-hour totals) fail once.
    if (
      String(url).startsWith('/api/log') &&
      String(url).includes('page_size=20') &&
      failures > 0
    ) {
      failures--
      throw new Error('Synthetic unavailable')
    }
    return original(url, config)
  })
  const user = userEvent.setup()
  await renderKeysPage(client, '/keys/21')
  const inspector = await screen.findByTestId('key-page')
  expect(
    await within(inspector).findByText('Could not load requests for this Key')
  ).toBeVisible()
  await user.click(within(inspector).getByRole('button', { name: 'Retry' }))
  expect(
    await within(inspector).findByText('No requests from this Key yet')
  ).toBeVisible()
})

test.each(['zhCN', 'zhTW'])(
  'renders request times in the %s locale without breaking',
  async (language) => {
    await i18next.changeLanguage(language)
    try {
      const { inspector } = await openKey()
      const times = await within(inspector).findAllByText(
        (_, element) => element?.tagName === 'TIME'
      )
      expect(times.length).toBeGreaterThan(0)
      for (const time of times) {
        expect(
          Number.isNaN(Date.parse(time.getAttribute('datetime') ?? ''))
        ).toBe(false)
      }
    } finally {
      await i18next.changeLanguage('en')
    }
  }
)

test('the full list for the Key opens in call records with a way back to the list', async () => {
  const { inspector } = await openKey()
  const link = (
    await within(inspector).findByText('View all in call records')
  ).closest('a')
  const href = link?.getAttribute('href') ?? ''
  expect(href).toContain('/usage-logs/common')
  expect(href).toContain('tokenId=21')
  expect(decodeURIComponent(href)).toContain('"focusKeyId":21')
  expect(api.post).not.toHaveBeenCalled()
})
