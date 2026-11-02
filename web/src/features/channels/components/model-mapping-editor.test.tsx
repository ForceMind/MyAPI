import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { ModelMappingEditor } from './model-mapping-editor'

afterEach(() => { cleanup(); vi.restoreAllMocks() })

function EditableMappings({ initial }: { initial: string }) {
  const [value, setValue] = useState(initial)
  return <ModelMappingEditor value={value} onChange={setValue} />
}

describe('ModelMappingEditor', () => {
  test('parses externally supplied JSON mappings into visual rows', async () => {
    render(
      <ModelMappingEditor
        value={JSON.stringify({ 'gpt-5': 'gpt-5-mini', o3: 'o4-mini' })}
        onChange={vi.fn()}
      />
    )

    expect(await screen.findByDisplayValue('gpt-5')).toBeInTheDocument()
    expect(screen.getByDisplayValue('gpt-5-mini')).toBeInTheDocument()
    expect(screen.getByDisplayValue('o3')).toBeInTheDocument()
    expect(screen.getByDisplayValue('o4-mini')).toBeInTheDocument()
  })

  test('labels each complete mapping and keeps row actions reachable without changing raw model values', async () => {
    const source = 'synthetic/source-with-an-extremely-long-complete-name-and-final-suffix'
    const target = 'synthetic/target-with-a-different-complete-name-and-final-suffix'
    render(<ModelMappingEditor value={JSON.stringify({ [source]: target })} onChange={vi.fn()} />)
    const row = await screen.findByRole('group', { name: 'Model mapping 1' })
    expect(within(row).getByRole('combobox', { name: 'Original Model' })).toHaveValue(source)
    expect(within(row).getByRole('combobox', { name: 'Replacement Model' })).toHaveValue(target)
    expect(within(row).getByRole('button', { name: 'Delete mapping' })).toHaveClass('min-h-11', 'min-w-11')
    expect(row).toHaveClass('grid-cols-1')
  })

  test('preserves the duplicate-source error rather than silently dropping one mapping', async () => {
    const change = vi.fn()
    render(<ModelMappingEditor value='{"first":"alpha","second":"beta"}' onChange={change} />)
    const row = await screen.findByRole('group', { name: 'Model mapping 2' })
    fireEvent.change(within(row).getByRole('combobox', { name: 'Original Model' }), { target: { value: 'first' } })
    expect(screen.getByText('Duplicate source model mappings are not allowed')).toBeVisible()
    expect(screen.getByDisplayValue('beta')).toBeInTheDocument()
    expect(change).toHaveBeenLastCalledWith('{ "duplicate_source_models": ')
  })

  test('preserves every raw model mapping through JSON and visual modes and deletes only the selected row', async () => {
    const user = userEvent.setup()
    render(<EditableMappings initial='{"first":"alpha","second":"beta"}' />)
    await screen.findByDisplayValue('alpha')
    await user.click(screen.getByRole('tab', { name: 'JSON' }))
    const json = await screen.findByRole('textbox', { name: 'Model Mapping' })
    expect(JSON.parse((json as HTMLTextAreaElement).value)).toEqual({ first: 'alpha', second: 'beta' })
    await user.click(screen.getByRole('tab', { name: 'Visual' }))
    const row = await screen.findByRole('group', { name: 'Model mapping 1' })
    await user.click(within(row).getByRole('button', { name: 'Delete mapping' }))
    expect(screen.queryByDisplayValue('first')).not.toBeInTheDocument()
    expect(screen.getByDisplayValue('second')).toBeVisible()
    expect(screen.getByDisplayValue('beta')).toBeVisible()
  })

  test('keeps disabled mappings readable while blocking edits and row mutations', async () => {
    const change = vi.fn()
    const user = userEvent.setup()
    render(<ModelMappingEditor value='{"first":"alpha"}' onChange={change} disabled />)
    const row = await screen.findByRole('group', { name: 'Model mapping 1' })
    expect(within(row).getByRole('combobox', { name: 'Original Model' })).toBeDisabled()
    expect(within(row).getByRole('combobox', { name: 'Replacement Model' })).toBeDisabled()
    await user.click(within(row).getByRole('button', { name: 'Delete mapping' }))
    await user.click(screen.getByRole('button', { name: 'Add Mapping' }))
    expect(change).not.toHaveBeenCalled()
    expect(screen.getByDisplayValue('first')).toBeVisible()
  })
})
