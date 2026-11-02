import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { ParamOverrideEditorDialog } from '../param-override-editor-dialog'

const fixture = { operations: [
  { description: 'Synthetic first rule with a complete long description suffix', mode: 'set', path: 'generation.synthetic_path_with_a_complete_long_suffix', value: 0, keep_origin: true, conditions: [{ path: 'stream', mode: 'full', value: false, invert: true, pass_missing_key: true }], logic: 'AND' },
  { mode: 'sync_fields', from: 'header:session_id', to: 'json:prompt_cache_key' },
] }
afterEach(() => { cleanup(); vi.restoreAllMocks() })

it('rule selection is keyboard reachable in one body and saves zero false and all sync endpoint fields unchanged', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn()
  render(<ParamOverrideEditorDialog open value={JSON.stringify(fixture)} onSave={onSave} onOpenChange={() => {}} />)
  const body = screen.getByRole('region', { name: 'Parameter override task' })
  expect(body).toHaveClass('min-h-0', 'overflow-y-auto')
  expect(body).toHaveAttribute('tabindex', '0')
  expect(body).not.toContainElement(screen.getByRole('heading', { name: 'Parameter Override' }))
  expect(body).not.toContainElement(screen.getByRole('button', { name: 'Save' }))
  expect(screen.getByText('Synthetic first rule with a complete long description suffix')).not.toHaveClass('line-clamp-2')
  const second = screen.getByRole('button', { name: 'Edit rule 2' })
  second.focus()
  await user.keyboard('[Space]')
  expect(second).toHaveAttribute('aria-pressed', 'true')
  expect(await screen.findByText('Sync Endpoints')).toBeVisible()
  expect(screen.getByDisplayValue('session_id')).toBeVisible()
  expect(screen.getByDisplayValue('prompt_cache_key')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Save' }))
  expect(onSave).toHaveBeenCalledTimes(1)
  expect(JSON.parse(onSave.mock.calls[0]?.[0])).toEqual(fixture)
})

it('legacy and JSON modes preserve arbitrary supported legacy extensions and explicit zero or false', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn()
  const original = { temperature: 0, stream: false, extension: { unknown: { count: 0, enabled: false } } }
  render(<ParamOverrideEditorDialog open value={JSON.stringify(original)} onSave={onSave} onOpenChange={() => {}} />)
  await user.click(screen.getByRole('button', { name: 'JSON Text' }))
  const editor = screen.getByRole('textbox', { name: 'Advanced text editing' })
  expect(JSON.parse((editor as HTMLTextAreaElement).value)).toEqual(original)
  const edited = { ...original, extension: { ...original.extension, retained: 'synthetic-value' } }
  fireEvent.input(editor, { target: { value: JSON.stringify(edited) } })
  await user.click(screen.getByRole('button', { name: 'Visual' }))
  expect(screen.getByRole('button', { name: 'Visual' })).toHaveAttribute('aria-pressed', 'true')
  expect(JSON.parse((screen.getByRole('textbox', { name: 'Legacy Format (JSON Object)' }) as HTMLTextAreaElement).value)).toEqual(edited)
  await user.click(screen.getByRole('button', { name: 'Save' }))
  expect(JSON.parse(onSave.mock.calls[0]?.[0])).toEqual(edited)
})

it('invalid raw JSON remains in JSON mode and Cancel never applies the draft', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn()
  const onOpenChange = vi.fn()
  render(<ParamOverrideEditorDialog open value={JSON.stringify(fixture)} onSave={onSave} onOpenChange={onOpenChange} />)
  await user.click(screen.getByRole('button', { name: 'JSON Text' }))
  const editor = screen.getByRole('textbox', { name: 'Advanced text editing' })
  fireEvent.input(editor, { target: { value: '{"operations":' } })
  await user.click(screen.getByRole('button', { name: 'Visual' }))
  expect(screen.getByRole('button', { name: 'JSON Text' })).toHaveAttribute('aria-pressed', 'true')
  expect(editor).toHaveAttribute('aria-invalid', 'true')
  expect(editor).toHaveValue('{"operations":')
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(onOpenChange).toHaveBeenCalledWith(false)
  expect(onSave).not.toHaveBeenCalled()
})

it('Fill Template retains its existing direct draft replacement and requires Save to apply it', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn()
  render(<ParamOverrideEditorDialog open value={JSON.stringify(fixture)} onSave={onSave} onOpenChange={() => {}} />)
  await user.click(screen.getByRole('button', { name: 'Fill Template' }))
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  expect(onSave).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Save' }))
  expect(JSON.parse(onSave.mock.calls[0]?.[0])).toEqual({ operations: [{ description: 'Set default temperature for openai/* models.', path: 'temperature', mode: 'set', value: 0.7, conditions: [{ path: 'model', mode: 'prefix', value: 'openai/' }], logic: 'AND' }] })
})
