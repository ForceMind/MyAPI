import { render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { ModelMappingEditor } from './model-mapping-editor'

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
})
