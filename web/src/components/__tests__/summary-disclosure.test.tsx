import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { FormEvent } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { SummaryDisclosure } from '../summary-disclosure'

afterEach(cleanup)

test('keyboard disclosure preserves the in-progress field and never submits its parent form', async () => {
  const submit = vi.fn((event: FormEvent<HTMLFormElement>) => event.preventDefault())
  const user = userEvent.setup()
  render(<form onSubmit={submit}>
    <SummaryDisclosure summary='Saved entries (1)'>
      <label>Existing draft<input defaultValue='Original' /></label>
    </SummaryDisclosure>
  </form>)
  const trigger = screen.getByRole('button', { name: 'Saved entries (1)' })
  expect(trigger).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByRole('textbox', { name: 'Existing draft' })).not.toBeInTheDocument()
  trigger.focus()
  await user.keyboard('{Enter}')
  const field = await screen.findByRole('textbox', { name: 'Existing draft' })
  await user.clear(field)
  await user.type(field, 'Edited but not saved')
  trigger.focus()
  await user.keyboard(' ')
  expect(trigger).toHaveAttribute('aria-expanded', 'false')
  expect(field).not.toBeVisible()
  await user.keyboard('{Enter}')
  expect(await screen.findByRole('textbox', { name: 'Existing draft' })).toBe(field)
  expect(field).toHaveValue('Edited but not saved')
  expect(submit).not.toHaveBeenCalled()
})
