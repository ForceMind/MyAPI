import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { PricePublicationRequest } from '../price-publication-api'
import { PricePublicationPanel } from '../price-publication-panel'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

const source = 'a'.repeat(64)
const expectedDigest = 'b'.repeat(64)
const expression = 'v1:tier("short", p * 2 + c * 10 + cr * 0.1 + cc * 2.5)'
let published = false
let lastID = ''

function stateFixture() {
  return {
    expected_digest: expectedDigest,
    runtime_ready: true,
    snapshot: { state: { revision: published ? 1 : 0 } },
    receipts: published
      ? [
          {
            id: lastID,
            actor_id: 1,
            action: 'publish',
            created_at: 1700000000,
            revision: 1,
          },
        ]
      : [],
  }
}
function previewFixture() {
  return {
    source_sha256: source,
    expected_digest: expectedDigest,
    revision: published ? 1 : 0,
    rows: [
      {
        model: 'fixture',
        current_mode: 'tiered_expr',
        current_expression: 'p * 3',
        locked: published,
        eligible: !published,
        candidate: {
          model: 'fixture',
          expression,
          expression_sha256: 'c'.repeat(64),
          source_sha256: source,
        },
      },
      {
        model: 'locked-fixture',
        current_mode: 'tiered_expr',
        current_expression: 'p * 5',
        locked: true,
        eligible: false,
        candidate: null,
      },
      {
        model: 'unquoted-fixture',
        current_mode: 'ratio',
        current_expression: '',
        locked: false,
        eligible: false,
        candidate: null,
      },
    ],
  }
}
function successfulPost(_url: unknown, body: unknown) {
  const request = body as PricePublicationRequest
  lastID = request.id
  published = true
  return Promise.resolve({
    data: {
      success: true,
      data: {
        receipt: {
          id: request.id,
          actor_id: 1,
          action: request.action,
          created_at: 1700000000,
          revision: 1,
        },
        runtime_ready: true,
      },
    },
  })
}
beforeEach(() => {
  vi.resetAllMocks()
  published = false
  lastID = ''
  vi.mocked(api.get).mockImplementation((url) =>
    Promise.resolve({
      data: {
        success: true,
        data: url.endsWith('/publication-preview')
          ? previewFixture()
          : stateFixture(),
      },
    })
  )
  vi.mocked(api.post).mockImplementation(successfulPost)
})
function renderPanel(role = 100) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const view = render(
    <QueryClientProvider client={client}>
      <PricePublicationPanel
        key='root-1'
        digest={source}
        userID={1}
        role={role}
      />
    </QueryClientProvider>
  )
  return { ...view, client }
}
async function openPanel() {
  await userEvent.click(
    screen.getByRole('button', { name: 'Effective price publication' })
  )
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Review price change: fixture' })
    ).toBeEnabled()
  )
}

test('source opening is read-only and publishing requires reviewed diff and an explicit checkbox', async () => {
  renderPanel()
  expect(api.get).not.toHaveBeenCalled()
  await openPanel()
  expect(api.post).not.toHaveBeenCalled()
  expect(
    screen.getByRole('button', { name: 'Review price change: locked-fixture' })
  ).toBeDisabled()
  expect(
    screen.getByRole('button', {
      name: 'Review price change: unquoted-fixture',
    })
  ).toBeDisabled()
  await userEvent.click(
    screen.getByRole('button', { name: 'Review price change: fixture' })
  )
  expect(screen.getByText('Current configuration')).toBeInTheDocument()
  expect(screen.getByText('Proposed configuration')).toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))
  expect(await screen.findByText('Required')).toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
  await userEvent.click(
    screen.getByRole('checkbox', {
      name: 'I reviewed this price change and its reference-only scope.',
    })
  )
  await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))
  expect(await screen.findByRole('status')).toHaveTextContent(
    'Price operation saved.'
  )
  expect(api.post).toHaveBeenCalledTimes(1)
  const body = vi.mocked(api.post).mock.calls[0][1] as PricePublicationRequest
  expect(body).toEqual({
    id: expect.stringMatching(/^[0-9a-f]{64}$/),
    action: 'publish',
    expected_digest: expectedDigest,
    source_sha256: source,
    models: [{ model: 'fixture', locked: true }],
    confirmed: true,
  })
  expect(body).not.toHaveProperty('expression')
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Unlock: fixture' })
    ).toBeEnabled()
  )
})

test('an uncertain response preserves operation identity for a safe explicit retry', async () => {
  vi.mocked(api.post).mockRejectedValueOnce(new Error('connection lost'))
  renderPanel()
  await openPanel()
  await userEvent.click(
    screen.getByRole('button', { name: 'Review price change: fixture' })
  )
  await userEvent.click(screen.getByRole('checkbox'))
  await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))
  expect(
    await screen.findByText(
      'Price operation failed. Retry the same operation or refresh its status.'
    )
  ).toBeInTheDocument()
  const first = vi.mocked(api.post).mock.calls[0][1]
  await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))
  await screen.findByRole('status')
  expect(vi.mocked(api.post).mock.calls[1][1]).toEqual(first)
})

test('canceling unlock has no side effect and confirmed unlock does not publish source prices', async () => {
  renderPanel()
  await openPanel()
  await userEvent.click(
    screen.getByRole('button', { name: 'Unlock: locked-fixture' })
  )
  await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(api.post).not.toHaveBeenCalled()
  await userEvent.click(
    screen.getByRole('button', { name: 'Unlock: locked-fixture' })
  )
  await userEvent.click(screen.getByRole('checkbox'))
  await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))
  await screen.findByRole('status')
  const request = vi.mocked(api.post).mock
    .calls[0][1] as PricePublicationRequest
  expect(request.action).toBe('lock')
  expect(request.models).toEqual([{ model: 'locked-fixture', locked: false }])
  expect(request.source_sha256).toBeUndefined()
})

test('non-root sessions cannot open publication controls or trigger reads', () => {
  renderPanel(10)
  expect(screen.queryByRole('button')).not.toBeInTheDocument()
  expect(api.get).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
})

test('pending confirmation disables duplicate submission and old-session completion stays isolated', async () => {
  let finish: (() => void) | undefined
  vi.mocked(api.post).mockImplementation(
    (url, body) =>
      new Promise((resolve) => {
        finish = () => {
          void successfulPost(url, body).then(resolve)
        }
      })
  )
  const view = renderPanel()
  await openPanel()
  await userEvent.click(
    screen.getByRole('button', { name: 'Review price change: fixture' })
  )
  await userEvent.click(screen.getByRole('checkbox'))
  await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))
  expect(screen.getByRole('button', { name: 'Saving...' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  view.rerender(
    <QueryClientProvider client={view.client}>
      <PricePublicationPanel
        key='root-2'
        digest={source}
        userID={2}
        role={100}
      />
    </QueryClientProvider>
  )
  await act(async () => {
    finish?.()
  })
  expect(screen.queryByRole('status')).not.toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Effective price publication' })
  ).toBeEnabled()
  expect(api.post).toHaveBeenCalledTimes(1)
})

test('rollback uses the current revision digest and cannot be submitted without confirmation', async () => {
  published = true
  lastID = 'd'.repeat(64)
  renderPanel()
  await userEvent.click(
    screen.getByRole('button', { name: 'Effective price publication' })
  )
  const rollback = await screen.findByRole('button', {
    name: `Rollback: ${lastID}`,
  })
  await waitFor(() => expect(rollback).toBeEnabled())
  await userEvent.click(rollback)
  await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))
  expect(api.post).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('checkbox'))
  const target = lastID
  await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))
  await screen.findByRole('status')
  expect(vi.mocked(api.post).mock.calls[0][1]).toEqual(
    expect.objectContaining({
      action: 'rollback',
      rollback_of: target,
      expected_digest: expectedDigest,
      confirmed: true,
    })
  )
})
