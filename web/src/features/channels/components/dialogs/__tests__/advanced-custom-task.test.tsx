import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { AdvancedCustomEditorDialog } from '../advanced-custom-editor-dialog'

const fixture = { advanced_routes: [
  { incoming_path: '/v1/chat/completions', upstream_path: '/synthetic/upstream-path-with-a-complete-long-final-suffix', converter: 'none', models: ['synthetic/model-with-a-complete-long-final-suffix'], auth: { type: 'header', name: 'X-Synthetic-Key', value: '{api_key}' } },
  { incoming_path: '/v1/models', upstream_path: '/synthetic/models', converter: 'none' },
  { incoming_path: '/v1/dashboard/billing/credit_grants', upstream_path: '/synthetic/balance', converter: 'none' },
] }
afterEach(() => { cleanup(); vi.restoreAllMocks() })

it('all four advanced modes remain reachable in one scroll body and save the original supported route fields', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn()
  render(<AdvancedCustomEditorDialog open value={JSON.stringify(fixture)} onSave={onSave} onOpenChange={() => {}} />)
  const body = screen.getByRole('region', { name: 'Advanced routing task' })
  expect(body).toHaveClass('min-h-0', 'overflow-y-auto')
  expect(body).toHaveAttribute('tabindex', '0')
  expect(body).not.toContainElement(screen.getByRole('heading', { name: 'Advanced Custom Routes' }))
  expect(body).not.toContainElement(screen.getByRole('button', { name: 'Save changes' }))
  await user.click(screen.getByRole('tab', { name: /^Model List/ }))
  expect(screen.getByDisplayValue('/synthetic/models')).toBeVisible()
  await user.click(screen.getByRole('tab', { name: /^Balance Query/ }))
  expect(screen.getByDisplayValue('/synthetic/balance')).toBeVisible()
  await user.click(screen.getByRole('tab', { name: 'Full JSON' }))
  expect(JSON.parse((screen.getByRole('textbox', { name: 'Advanced text editing' }) as HTMLTextAreaElement).value)).toEqual(fixture)
  await user.click(screen.getByRole('tab', { name: /^Forwarding Routes/ }))
  expect(screen.getByRole('tab', { name: /^Forwarding Routes/ })).toHaveAttribute('aria-selected', 'true')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  expect(onSave).toHaveBeenCalledTimes(1)
  expect(JSON.parse(onSave.mock.calls[0]?.[0])).toEqual(fixture)
})

it('invalid advanced JSON keeps its draft and active mode and cancelling never saves it', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn()
  const onOpenChange = vi.fn()
  render(<AdvancedCustomEditorDialog open value={JSON.stringify(fixture)} onSave={onSave} onOpenChange={onOpenChange} />)
  await user.click(screen.getByRole('tab', { name: 'Full JSON' }))
  const editor = screen.getByRole('textbox', { name: 'Advanced text editing' })
  fireEvent.input(editor, { target: { value: '{"advanced_routes":' } })
  await user.click(screen.getByRole('tab', { name: /^Model List/ }))
  expect(screen.getByRole('tab', { name: 'Full JSON' })).toHaveAttribute('aria-selected', 'true')
  expect(editor).toHaveValue('{"advanced_routes":')
  expect(editor).toHaveAttribute('aria-invalid', 'true')
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(onOpenChange).toHaveBeenCalledWith(false)
  expect(onSave).not.toHaveBeenCalled()
})

it('cancelling template replacement leaves forwarding and both management routes unchanged', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn()
  render(<AdvancedCustomEditorDialog open value={JSON.stringify(fixture)} onSave={onSave} onOpenChange={() => {}} />)
  await user.click(screen.getByRole('combobox', { name: 'Add template' }))
  await user.click(await screen.findByRole('option', { name: 'OpenAI only' }))
  const confirmation = await screen.findByRole('alertdialog', { name: 'Replace forwarding routes?' })
  expect(within(confirmation).getByText(/Model list and balance routes will be preserved/)).toBeVisible()
  await user.click(within(confirmation).getByRole('button', { name: 'Cancel' }))
  expect(onSave).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  expect(JSON.parse(onSave.mock.calls[0]?.[0])).toEqual(fixture)
})

it('confirming a template replacement preserves the original model-list and balance route payloads', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn()
  render(<AdvancedCustomEditorDialog open value={JSON.stringify(fixture)} onSave={onSave} onOpenChange={() => {}} />)
  await user.click(screen.getByRole('combobox', { name: 'Add template' }))
  await user.click(await screen.findByRole('option', { name: 'OpenAI only' }))
  const confirmation = await screen.findByRole('alertdialog', { name: 'Replace forwarding routes?' })
  await user.click(within(confirmation).getByRole('button', { name: 'Replace' }))
  expect(onSave).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  const saved = JSON.parse(onSave.mock.calls[0]?.[0]) as typeof fixture
  expect(saved.advanced_routes).toContainEqual(fixture.advanced_routes[1])
  expect(saved.advanced_routes).toContainEqual(fixture.advanced_routes[2])
  expect(saved.advanced_routes.some((route) => route.upstream_path === '/synthetic/upstream-path-with-a-complete-long-final-suffix')).toBe(false)
})
