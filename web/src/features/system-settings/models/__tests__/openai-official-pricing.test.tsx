import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { OpenAIOfficialPricingSection } from '../openai-official-pricing-section'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn() },
}))
const fixture = {
  source_url: 'https://developers.openai.com/api/docs/pricing.md',
  fetched_at: 1700000000,
  content_sha256: 'a'.repeat(64),
  currency: 'USD',
  unit_tokens: 1000000,
  service_tier: 'standard',
  scope: 'text-token-price-source-not-published',
  models: [
    {
      model: 'fixture-model',
      source_label: 'fixture-model (<272K context length)',
      short_context: {
        input_usd_per_million: '2.00',
        cached_input_usd_per_million: '0.00',
        cache_write_usd_per_million: null,
        output_usd_per_million: '10.00',
      },
      long_context: {
        input_usd_per_million: '4.00',
        cached_input_usd_per_million: '0.20',
        cache_write_usd_per_million: '5.00',
        output_usd_per_million: '15.00',
      },
    },
  ],
}
const originalUser = useAuthStore.getState().auth.user
afterEach(() => {
  act(() => useAuthStore.getState().auth.setUser(originalUser))
})
beforeEach(() => {
  vi.resetAllMocks()
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'fixture-root', role: 100 })
})
function renderSource() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <OpenAIOfficialPricingSection />
    </QueryClientProvider>
  )
}

test('manual source fetch preserves decimal prices and distinguishes unquoted values from zero without writes', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: fixture },
  })
  renderSource()
  expect(api.get).not.toHaveBeenCalled()
  await userEvent.click(
    screen.getByRole('button', { name: 'Fetch official pricing' })
  )
  expect(await screen.findByText('$2.00')).toBeInTheDocument()
  expect(screen.getByText('$0.00')).toBeInTheDocument()
  expect(screen.getByText('Not quoted')).toBeInTheDocument()
  expect(screen.getByText('$4.00')).toBeInTheDocument()
  expect(screen.getByText('Short context')).toBeInTheDocument()
  expect(screen.getByText('Long context')).toBeInTheDocument()
  expect(screen.getByText(fixture.content_sha256)).toBeInTheDocument()
  await userEvent.tab()
  await userEvent.tab()
  await userEvent.tab()
  await userEvent.tab()
  await userEvent.tab()
  expect(
    screen.getByRole('table', { name: 'OpenAI official pricing source' })
  ).toHaveFocus()
  expect(
    screen.getByText(
      'Fetching or saving a source does not change effective prices.'
    )
  ).toBeInTheDocument()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
})

test('failed refresh hides stale source prices and can be retried', async () => {
  vi.mocked(api.get)
    .mockResolvedValueOnce({ data: { success: true, data: fixture } })
    .mockRejectedValueOnce(new Error('synthetic source unavailable'))
    .mockResolvedValueOnce({ data: { success: true, data: fixture } })
  renderSource()
  const button = screen.getByRole('button', { name: 'Fetch official pricing' })
  await userEvent.click(button)
  await screen.findByText('$2.00')
  await userEvent.click(button)
  await screen.findByText(
    'Official source unavailable. Effective prices are unchanged.'
  )
  expect(screen.queryByText('$2.00')).not.toBeInTheDocument()
  await userEvent.click(button)
  expect(await screen.findByText('$2.00')).toBeInTheDocument()
})

test('non-Root cannot fetch or display the source', () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 2, username: 'fixture-admin', role: 10 })
  renderSource()
  expect(
    screen.getByText('Root access is required for this price source.')
  ).toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Fetch official pricing' })
  ).not.toBeInTheDocument()
  expect(api.get).not.toHaveBeenCalled()
})

test.each([
  { ...fixture, unit_tokens: 1000 },
  { ...fixture, currency: 'CNY' },
  { ...fixture, service_tier: 'batch' },
  { ...fixture, source_url: 'https://example.invalid/pricing' },
  { ...fixture, models: [] },
  { ...fixture, models: [fixture.models[0], fixture.models[0]] },
])(
  'malformed source metadata is rejected instead of displaying a plausible wrong price %#',
  async (source) => {
    vi.mocked(api.get).mockResolvedValue({
      data: { success: true, data: source },
    })
    renderSource()
    await userEvent.click(
      screen.getByRole('button', { name: 'Fetch official pricing' })
    )
    await screen.findByText(
      'Official source unavailable. Effective prices are unchanged.'
    )
    expect(screen.queryByText('$2.00')).not.toBeInTheDocument()
  }
)

test('in-flight fetch is disabled and an account change cannot display prior-session source data', async () => {
  let resolve = (_value: unknown) => {}
  vi.mocked(api.get).mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r
      }) as never
  )
  renderSource()
  await userEvent.click(
    screen.getByRole('button', { name: 'Fetch official pricing' })
  )
  expect(screen.getByRole('button', { name: 'Loading...' })).toBeDisabled()
  await act(async () => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 2, username: 'fixture-admin', role: 10 })
    resolve({ data: { success: true, data: fixture } })
  })
  await waitFor(() =>
    expect(screen.queryByText('$2.00')).not.toBeInTheDocument()
  )
  expect(
    screen.queryByRole('button', { name: 'Fetch official pricing' })
  ).not.toBeInTheDocument()
})

test('explicit save sends no client prices and displays the returned frozen source without applying it', async () => {
  const frozen = { ...fixture, content_sha256: 'b'.repeat(64) }
  vi.mocked(api.post).mockResolvedValue({
    data: { success: true, data: frozen },
  })
  renderSource()
  expect(api.post).not.toHaveBeenCalled()
  await userEvent.click(
    screen.getByRole('button', { name: 'Save official source version' })
  )
  expect(await screen.findByText(frozen.content_sha256)).toBeInTheDocument()
  expect(
    screen.getByText(
      'Frozen source loaded. Publishing requires a separate confirmation.'
    )
  ).toBeInTheDocument()
  expect(api.post).toHaveBeenCalledWith(
    '/api/ratio_sync/openai/versions',
    undefined,
    expect.objectContaining({ skipErrorHandler: true })
  )
  expect(api.put).not.toHaveBeenCalled()
})

test('saved digest validates before request and a mismatched returned version is never displayed', async () => {
  renderSource()
  const input = screen.getByRole('textbox', { name: 'Saved source SHA256' })
  await userEvent.type(input, 'bad')
  await userEvent.click(
    screen.getByRole('button', { name: 'Read saved source version' })
  )
  expect(
    await screen.findByText('Enter a 64-character lowercase SHA256.')
  ).toBeInTheDocument()
  expect(input).toHaveAttribute('aria-invalid', 'true')
  expect(api.get).not.toHaveBeenCalled()
  await userEvent.clear(input)
  await userEvent.type(input, 'b'.repeat(64))
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: fixture },
  })
  await userEvent.click(
    screen.getByRole('button', { name: 'Read saved source version' })
  )
  await screen.findByText(
    'Official source unavailable. Effective prices are unchanged.'
  )
  expect(screen.queryByText('$2.00')).not.toBeInTheDocument()
  expect(api.get).toHaveBeenCalledWith(
    `/api/ratio_sync/openai/versions/${'b'.repeat(64)}`,
    expect.anything()
  )
})

test('explicit saved version read uses its own prices and can retry a failed version without latest fallback', async () => {
  vi.mocked(api.get)
    .mockRejectedValueOnce(new Error('version missing'))
    .mockResolvedValueOnce({ data: { success: true, data: fixture } })
  renderSource()
  await userEvent.type(
    screen.getByRole('textbox', { name: 'Saved source SHA256' }),
    fixture.content_sha256
  )
  const read = screen.getByRole('button', { name: 'Read saved source version' })
  await userEvent.click(read)
  await screen.findByText(
    'Official source unavailable. Effective prices are unchanged.'
  )
  expect(screen.queryByText('$2.00')).not.toBeInTheDocument()
  await userEvent.click(read)
  expect(await screen.findByText('$2.00')).toBeInTheDocument()
  expect(
    vi
      .mocked(api.get)
      .mock.calls.every(
        ([url]) =>
          url === `/api/ratio_sync/openai/versions/${fixture.content_sha256}`
      )
  ).toBe(true)
})

test('in-flight save is disabled and does not expose old-session results after an account change', async () => {
  let resolve = (_value: unknown) => {}
  vi.mocked(api.post).mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r
      }) as never
  )
  renderSource()
  await userEvent.click(
    screen.getByRole('button', { name: 'Save official source version' })
  )
  expect(screen.getByRole('button', { name: 'Saving...' })).toBeDisabled()
  expect(
    screen.getByRole('textbox', { name: 'Saved source SHA256' })
  ).toBeDisabled()
  await act(async () => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 3, username: 'second-root', role: 100 })
    resolve({ data: { success: true, data: fixture } })
  })
  expect(screen.queryByText('$2.00')).not.toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Save official source version' })
  ).toBeEnabled()
})
