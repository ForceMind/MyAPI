/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'

import { DateTimeRangePicker } from '@/components/date-time-range-picker'

test('keeps date range controls touch-sized and the popup scrollable within the viewport', async () => {
  const user = userEvent.setup()
  render(<DateTimeRangePicker onChange={() => undefined} />)
  const trigger = screen.getByRole('button', { name: /^Date Range:/ })
  expect(trigger).toHaveClass('min-h-11', 'min-w-0')
  await user.click(trigger)

  const dialog = screen.getByRole('dialog', { name: 'Date Range' })
  expect(dialog).toHaveClass(
    'overflow-y-auto',
    'w-[min(380px,calc(100vw-2px))]',
    'max-h-[min(46rem,calc(100dvh-2rem))]',
    'p-1',
    'min-[360px]:p-3'
  )
  const calendar = screen.getByRole('grid').closest('[data-slot="calendar"]')
  expect(calendar).toHaveClass('[--cell-size:2.75rem]')
  expect(screen.getByRole('button', { name: 'Today' })).toHaveClass('min-h-11')
  expect(screen.getByRole('button', { name: 'Cancel' })).toHaveClass('min-h-11')
  expect(screen.getByRole('button', { name: 'Apply range' })).toHaveClass('min-h-11')
})
