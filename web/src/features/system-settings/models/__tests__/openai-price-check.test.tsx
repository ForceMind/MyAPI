import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { OpenAIOfficialPricingSection } from '../openai-official-pricing-section'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn() },
}))
const source = 'a'.repeat(64)
const fixture = {
  enabled: false,
  interval_seconds: 86400,
  stale_after_seconds: 259200,
  stale: true,
  last_attempt_at: 1791000100,
  last_attempt_status: 'failed',
  last_task_id: 'synthetic-check',
  last_error_code: 'fetch_failed',
  last_success_at: 1790000000,
  next_check_at: 0,
  source_sha256: source,
  source_fetched_at: 1789000000,
  pending_source_sha256: source,
  expected_digest: 'b'.repeat(64),
  revision: 3,
  diff_total: 1,
  diff_truncated: false,
  diff_review_required: true,
  diff_counts: {
    addition: 0,
    change: 1,
    removal: 0,
    unqualified: 0,
    unchanged: 0,
  },
  diff: [
    {
      model: 'gpt-6.1-sol',
      model_name_truncated: false,
      change: 'change',
      current_mode: 'tiered_expr',
      current_expression_sha256: 'c'.repeat(64),
      candidate: {
        model: 'gpt-6.1-sol',
        expression: 'synthetic',
        expression_sha256: 'd'.repeat(64),
        source_sha256: source,
      },
      locked: true,
      eligible: false,
      pending_review: true,
    },
  ],
}
const originalUser = useAuthStore.getState().auth.user
beforeEach(() => {
  vi.resetAllMocks()
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'synthetic-root', role: 100 })
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: fixture },
  })
})
afterEach(() => act(() => useAuthStore.getState().auth.setUser(originalUser)))
function renderChecks() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <OpenAIOfficialPricingSection />
    </QueryClientProvider>
  )
}
async function openChecks() {
  renderChecks()
  await userEvent.click(
    screen.getByRole('button', { name: 'Official source checks' })
  )
  return screen.findByRole('region', { name: 'Official source checks' })
}

test('opening status reads saved evidence and distinguishes failed freshness from pending locked publication', async () => {
  renderChecks()
  expect(api.get).not.toHaveBeenCalled()
  await userEvent.click(
    screen.getByRole('button', { name: 'Official source checks' })
  )
  expect(await screen.findByText('Stale source evidence')).toBeInTheDocument()
  expect(screen.getByText(source)).toBeInTheDocument()
  expect(
    screen.getByText(
      'The last source check failed. The last good source and effective prices are retained.'
    )
  ).toBeInTheDocument()
  const diff = screen.getByRole('table', { name: 'Source check differences' })
  expect(within(diff).getByText('Locked')).toBeInTheDocument()
  expect(
    within(diff).queryByText('Source not qualified')
  ).not.toBeInTheDocument()
  expect(within(diff).getByText('Pending review')).toBeInTheDocument()
  expect(within(diff).getByText('gpt-6.1-sol')).toBeInTheDocument()
  expect(api.get).toHaveBeenCalledWith(
    '/api/ratio_sync/openai/check',
    expect.objectContaining({ signal: expect.any(AbortSignal) })
  )
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})

test('explicit schedule enable saves only the option and can be disabled without checking or publishing', async () => {
  let enabled = false
  vi.mocked(api.get).mockImplementation(async () => ({
    data: { success: true, data: { ...fixture, enabled } },
  }))
  vi.mocked(api.put).mockImplementation(async (_url, body) => {
    enabled = (body as { value: string }).value === 'true'
    return { data: { success: true } }
  })
  await openChecks()
  await userEvent.click(
    await screen.findByRole('button', { name: 'Enable daily source checks' })
  )
  await userEvent.click(
    await screen.findByRole('button', { name: 'Disable daily source checks' })
  )
  await screen.findByRole('button', { name: 'Enable daily source checks' })
  expect(vi.mocked(api.put).mock.calls.map((call) => call.slice(0, 2))).toEqual(
    [
      [
        '/api/option/',
        { key: 'OpenAIOfficialPriceCheckEnabled', value: 'true' },
      ],
      [
        '/api/option/',
        { key: 'OpenAIOfficialPriceCheckEnabled', value: 'false' },
      ],
    ]
  )
  expect(api.post).not.toHaveBeenCalled()
})

test('repeated manual clicks enqueue once and show a reused task without publishing', async () => {
  let resolve: (value: unknown) => void = () => {}
  vi.mocked(api.post).mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done
      }) as never
  )
  await openChecks()
  const check = await screen.findByRole('button', {
    name: 'Check official source now',
  })
  await waitFor(() => expect(check).toBeEnabled())
  await userEvent.dblClick(check)
  expect(check).toBeDisabled()
  expect(api.post).toHaveBeenCalledTimes(1)
  await act(async () =>
    resolve({
      data: {
        success: true,
        data: {
          created: false,
          task: { task_id: 'synthetic-check', status: 'running' },
        },
      },
    })
  )
  expect(
    await screen.findByText('An existing source check was reused.')
  ).toBeInTheDocument()
  expect(api.post).toHaveBeenCalledWith(
    '/api/ratio_sync/openai/check',
    {},
    expect.any(Object)
  )
})

test('failed manual request requires a status refresh before retry and retains last good source', async () => {
  vi.mocked(api.post).mockRejectedValue(new Error('synthetic conflict'))
  await openChecks()
  const check = await screen.findByRole('button', {
    name: 'Check official source now',
  })
  await waitFor(() => expect(check).toBeEnabled())
  await userEvent.click(check)
  await screen.findByText(
    'Source check request failed. Refresh status before retrying; saved evidence and effective prices are unchanged.'
  )
  expect(check).toBeDisabled()
  expect(screen.getByText(source)).toBeInTheDocument()
  await userEvent.click(
    screen.getByRole('button', { name: 'Refresh check status' })
  )
  await waitFor(() => expect(check).toBeEnabled())
})

test('failed status refresh hides obsolete actions and supports a read-only retry', async () => {
  await openChecks()
  await screen.findByText(source)
  vi.mocked(api.get).mockRejectedValueOnce(new Error('synthetic status error'))
  await userEvent.click(
    screen.getByRole('button', { name: 'Refresh check status' })
  )
  await screen.findByText(
    'Could not load source check status. Refresh before making changes.'
  )
  expect(screen.queryByText(source)).not.toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Check official source now' })
  ).toBeDisabled()
  await userEvent.click(
    screen.getByRole('button', { name: 'Refresh check status' })
  )
  expect(await screen.findByText(source)).toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})

test('late previous-session status never leaks source data or source controls to an administrator', async () => {
  let resolve: (value: unknown) => void = () => {}
  vi.mocked(api.get).mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done
      }) as never
  )
  await openChecks()
  await act(async () => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 2, username: 'synthetic-admin', role: 10 })
    resolve({ data: { success: true, data: fixture } })
  })
  expect(screen.queryByText(source)).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Official source checks' })
  ).not.toBeInTheDocument()
  expect(
    screen.getByText('Root access is required for this price source.')
  ).toBeInTheDocument()
})

test('bounded and truncated diff preview warns that omitted models are not reviewed', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...fixture,
        diff_total: 129,
        diff_truncated: true,
        diff: [
          {
            ...fixture.diff[0],
            model: 'long-model-'.repeat(30),
            model_name_truncated: true,
          },
        ],
      },
    },
  })
  await openChecks()
  expect(
    await screen.findByText(
      'This bounded preview is incomplete. Counts cover displayed rows only; review all source models before publication.'
    )
  ).toBeInTheDocument()
  expect(
    screen.getByRole('table', { name: 'Source check differences' })
  ).toHaveAttribute('tabindex', '0')
})

test('review saved source loads the exact frozen version through the existing publication entrypoint without writing', async () => {
  const rates = {
    input_usd_per_million: '2.5',
    cached_input_usd_per_million: '0.25',
    cache_write_usd_per_million: '2.5',
    output_usd_per_million: '15',
  }
  vi.mocked(api.get).mockImplementation(async (url) => ({
    data: {
      success: true,
      data:
        url === '/api/ratio_sync/openai/check'
          ? fixture
          : {
              source_url: 'https://developers.openai.com/api/docs/pricing.md',
              fetched_at: 1789000000,
              content_sha256: source,
              currency: 'USD',
              unit_tokens: 1000000,
              service_tier: 'standard',
              scope: 'text-token-price-source-not-published',
              models: [
                {
                  model: 'gpt-6.1-sol',
                  source_label: 'gpt-6.1-sol',
                  short_context: rates,
                  long_context: { ...rates, input_usd_per_million: '5' },
                },
              ],
            },
    },
  }))
  await openChecks()
  await userEvent.click(
    await screen.findByRole('button', { name: 'Review saved source' })
  )
  const prices = await screen.findByRole('table', {
    name: 'OpenAI official pricing source',
  })
  expect(within(prices).getByText('$5')).toBeInTheDocument()
  expect(
    screen.getByText(
      'Frozen source loaded. Publishing requires a separate confirmation.'
    )
  ).toBeInTheDocument()
  expect(api.get).toHaveBeenCalledWith(
    `/api/ratio_sync/openai/versions/${source}`,
    expect.any(Object)
  )
  expect(api.post).not.toHaveBeenCalled()
})

test('never-checked status has no fabricated source or successful-check timestamp', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...fixture,
        last_attempt_at: 0,
        last_attempt_status: '',
        last_task_id: '',
        last_error_code: '',
        last_success_at: 0,
        source_fetched_at: 0,
        source_sha256: '',
        pending_source_sha256: '',
        diff_total: 0,
        diff_review_required: false,
        diff_counts: {
          addition: 0,
          change: 0,
          removal: 0,
          unqualified: 0,
          unchanged: 0,
        },
        diff: [],
      },
    },
  })
  await openChecks()
  expect(await screen.findByText('Stale source evidence')).toBeInTheDocument()
  expect(
    screen.queryByRole('table', { name: 'Source check differences' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Review saved source' })
  ).not.toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Enable daily source checks' })
  ).toBeEnabled()
  expect(api.post).not.toHaveBeenCalled()
})
