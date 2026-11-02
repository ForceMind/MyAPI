import { QueryClient } from '@tanstack/react-query'
import { cleanup, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { ModelSettings } from '..'
import { ModelRatioVisualEditor } from '../model-ratio-visual-editor'
import { ModelPricingEditorPanel } from '../model-pricing-sheet'

const originalAuth = useAuthStore.getState()
let client: QueryClient
beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: 100, username: 'synthetic-root' })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

it('malformed stored model JSON remains the original editable text and cannot masquerade as an empty blacklist', async () => {
  const raw = '  {synthetic-invalid-json  '
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/option/') return { data: { success: true, data: [{ key: 'global.thinking_model_blacklist', value: raw }] } }
    if (url === '/api/option/typed-bulk/revision') return { data: { success: true, data: { revision: 0 } } }
    throw new Error(`Unexpected synthetic settings GET ${url}`)
  })
  const put = vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected synthetic settings write'))
  const user = userEvent.setup()
  await renderListRoute({ client, path: '/system-settings/models/$section', entry: '/system-settings/models/global', element: <ModelSettings /> })
  const editor = await screen.findByRole('textbox', { name: 'Models that skip thinking suffix processing' })
  expect(editor).toHaveValue(raw)
  expect(editor).toHaveAttribute('aria-invalid', 'true')
  await user.click(screen.getByRole('switch', { name: 'Enable Request Passthrough' }))
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  expect(await screen.findByText('Invalid JSON format')).toBeVisible()
  expect(editor).toHaveValue(raw)
  expect(put).not.toHaveBeenCalled()
})

it('a medium viewport opens the existing pricing sheet rather than hiding the selected editor in an undersized second column', async () => {
  vi.stubGlobal('innerWidth', 1024)
  const original = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({ ...original(query), matches: query === '(max-width: 1279px)' }))
  const empty = '{}'
  const ratio = '{"synthetic-long-model-name-for-medium-pricing-workspace":1}'
  const user = userEvent.setup()
  await renderListRoute({ client, path: '/system-settings/billing/model-pricing', element: <ModelRatioVisualEditor savedModelPrice={empty} savedModelRatio={ratio} savedCacheRatio={empty} savedCreateCacheRatio={empty} savedCompletionRatio={empty} savedImageRatio={empty} savedAudioRatio={empty} savedAudioCompletionRatio={empty} savedBillingMode={empty} savedBillingExpr={empty} modelPrice={empty} modelRatio={ratio} cacheRatio={empty} createCacheRatio={empty} completionRatio={empty} imageRatio={empty} audioRatio={empty} audioCompletionRatio={empty} billingMode={empty} billingExpr={empty} onChange={() => {}} onSave={() => {}} isSaving={false} /> })
  await user.click(screen.getByRole('button', { name: 'Edit' }))
  const dialog = await screen.findByRole('dialog', { name: 'Edit model pricing' })
  expect(dialog).toBeVisible()
  expect(within(dialog).getByDisplayValue('synthetic-long-model-name-for-medium-pricing-workspace')).toBeVisible()
})

it('the pricing editor exposes a complete long model name and keeps its existing save action outside one details region', async () => {
  const modelName = 'synthetic/model-with-a-complete-long-name-and-final-suffix'
  await renderListRoute({ client, path: '/system-settings/billing/model-pricing', element: <ModelPricingEditorPanel editData={{ name: modelName, ratio: '1', billingMode: 'per-token' }} onSave={() => {}} /> })
  const body = screen.getByRole('region', { name: 'Model pricing details' })
  expect(body).toHaveClass('min-h-0', 'overflow-y-auto')
  expect(screen.getByDisplayValue(modelName)).toBeVisible()
  expect(body).not.toContainElement(screen.getByRole('button', { name: 'Save model prices' }))
})
