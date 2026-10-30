/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { FullContentLogPagination } from '../full-content-log-pagination'

function paginationProps() {
  return {
    page: 1,
    pageSize: 20 as const,
    total: 121,
    busy: false,
    onPageChange: vi.fn(),
    onPageSizeChange: vi.fn(),
  }
}

describe('full content log pagination controls', () => {
  test('first and last page buttons respect the list boundaries', async () => {
    const user = userEvent.setup()
    const props = paginationProps()
    const view = render(<FullContentLogPagination {...props} />)
    expect(screen.getByRole('button', { name: 'First page' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Last page' }))
    expect(props.onPageChange).toHaveBeenLastCalledWith(7)
    view.rerender(<FullContentLogPagination {...props} page={7} />)
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Last page' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Go to page 7' })).toHaveAttribute('aria-current', 'page')
    await user.click(screen.getByRole('button', { name: 'First page' }))
    expect(props.onPageChange).toHaveBeenLastCalledWith(1)
  })

  test.each(['0', '8', '-1', '1.5', '1e0', 'text', '9007199254740993'])(
    'an invalid direct page %s is rejected with an accessible error',
    async (value) => {
      const user = userEvent.setup()
      const props = paginationProps()
      render(<FullContentLogPagination {...props} />)
      await user.type(screen.getByRole('textbox', { name: 'Page number' }), value)
      await user.click(screen.getByRole('button', { name: 'Go to page' }))
      expect(props.onPageChange).not.toHaveBeenCalled()
      expect(screen.getByRole('textbox', { name: 'Page number' })).toHaveAttribute('aria-invalid', 'true')
      expect(screen.getByRole('alert')).toHaveTextContent('Enter a page between 1 and 7')
    }
  )

  test('a valid direct page can be submitted using the keyboard', async () => {
    const user = userEvent.setup()
    const props = paginationProps()
    render(<FullContentLogPagination {...props} />)
    await user.type(screen.getByRole('textbox', { name: 'Page number' }), '7{Enter}')
    expect(props.onPageChange).toHaveBeenCalledExactlyOnceWith(7)
  })

  test('only twenty, fifty and one hundred rows can be selected', async () => {
    const user = userEvent.setup()
    const props = paginationProps()
    render(<FullContentLogPagination {...props} />)
    expect(screen.getAllByRole('option').map((option) => option.getAttribute('value'))).toEqual(['20', '50', '100'])
    await user.selectOptions(screen.getByRole('combobox', { name: 'Rows per page' }), '100')
    expect(props.onPageSizeChange).toHaveBeenCalledExactlyOnceWith(100)
    fireEvent.change(screen.getByRole('combobox', { name: 'Rows per page' }), { target: { value: '500' } })
    expect(props.onPageSizeChange).toHaveBeenCalledTimes(1)
  })

  test.each([
    { busy: true, total: undefined, message: 'Loading request total' },
    { busy: false, total: undefined, message: 'Request total unavailable' },
  ])('unknown totals show $message and disable page navigation', (state) => {
    render(<FullContentLogPagination {...paginationProps()} {...state} />)
    expect(screen.getByRole('status')).toHaveTextContent(state.message)
    expect(screen.getByRole('status')).not.toHaveTextContent('0 requests')
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Last page' })).toBeDisabled()
    expect(screen.getByRole('textbox', { name: 'Page number' })).toBeDisabled()
  })

  test('pagination controls retain a forty four pixel touch target', () => {
    render(<FullContentLogPagination {...paginationProps()} />)
    for (const button of screen.getAllByRole('button')) {
      expect(button).toHaveClass('min-h-11', 'min-w-11')
    }
    expect(screen.getByRole('textbox', { name: 'Page number' })).toHaveClass('min-h-11')
    const sizeSelector = screen.getByRole('combobox', { name: 'Rows per page' })
    expect(sizeSelector.closest('[data-slot="native-select-wrapper"]')).toHaveClass(
      '[&_select]:min-h-11',
      '[&_select]:min-w-11'
    )
  })

  test('a confirmed empty response shows zero requests and cannot leave page one', () => {
    render(<FullContentLogPagination {...paginationProps()} total={0} />)
    expect(screen.getByRole('status')).toHaveTextContent('Page 1 of 1 · 0 requests')
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
  })
})
