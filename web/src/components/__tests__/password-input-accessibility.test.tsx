import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it } from 'vitest'

import { PasswordInput } from '../password-input'

afterEach(() => cleanup())
it('the password visibility action reports its state and preserves the editable input value', async () => {
  const user = userEvent.setup()
  render(<PasswordInput aria-label='Synthetic password field' defaultValue='SYNTHETIC_ONLY_PASSWORD' />)
  const field = screen.getByLabelText('Synthetic password field')
  expect(field).toHaveAttribute('type', 'password')
  const toggle = screen.getByRole('button', { name: 'Toggle password visibility' })
  expect(toggle).toHaveAttribute('aria-pressed', 'false')
  expect(toggle).toHaveClass('min-h-11', 'min-w-11')
  await user.click(toggle)
  expect(field).toHaveAttribute('type', 'text')
  expect(field).toHaveValue('SYNTHETIC_ONLY_PASSWORD')
  expect(toggle).toHaveAttribute('aria-pressed', 'true')
})
