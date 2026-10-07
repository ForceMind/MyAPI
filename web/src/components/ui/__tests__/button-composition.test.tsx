import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { Button } from '../button'
import { DropdownMenu, DropdownMenuTrigger } from '../dropdown-menu'

describe('composed button presentation', () => {
  it('retains the touch and wrapping hook when a menu assigns its own slot', () => {
    render(
      <DropdownMenu>
        <DropdownMenuTrigger render={<Button />}>Open menu</DropdownMenuTrigger>
      </DropdownMenu>
    )
    const trigger = screen.getByRole('button', { name: 'Open menu' })
    expect(trigger).toHaveAttribute('data-slot', 'dropdown-menu-trigger')
    expect(trigger).toHaveAttribute('data-myapi-button')
  })
})
