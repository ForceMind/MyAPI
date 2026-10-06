import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { SearchProvider, useSearch } from './search-provider'

vi.mock('@/components/command-menu', () => ({
  CommandMenu: () => null,
}))

function SearchState() {
  const { open } = useSearch()
  return <output>{String(open)}</output>
}

describe('SearchProvider', () => {
  test('toggles global search with the control-key shortcut', () => {
    render(
      <SearchProvider>
        <SearchState />
      </SearchProvider>
    )

    expect(screen.getByText('false')).toBeInTheDocument()

    fireEvent.keyDown(document, { key: 'k', ctrlKey: true })
    expect(screen.getByText('true')).toBeInTheDocument()

    fireEvent.keyDown(document, { key: 'k', ctrlKey: true })
    expect(screen.getByText('false')).toBeInTheDocument()
  })
})
