/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { DateTimeRangePicker } from '@/components/date-time-range-picker'

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllEnvs()
})

describe('date and time range selection', () => {
  test('selects start and end in one calendar across months and commits inclusive local days', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <DateTimeRangePicker
        start={new Date(2026, 9, 1)}
        onChange={onChange}
      />
    )

    await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
    expect(screen.getAllByRole('grid')).toHaveLength(1)
    await user.click(screen.getByRole('button', { name: /October 29/ }))
    await user.click(screen.getByRole('button', { name: 'Next month' }))
    await user.click(screen.getByRole('button', { name: /November 2nd/ }))
    expect(onChange).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Apply range' }))

    expect(onChange).toHaveBeenCalledExactlyOnceWith({
      start: new Date(2026, 9, 29, 0, 0, 0),
      end: new Date(2026, 10, 2, 23, 59, 59),
    })
  })

  test('opens from the keyboard and selects the end with calendar arrow keys', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <DateTimeRangePicker
        start={new Date(2026, 9, 1)}
        onChange={onChange}
      />
    )
    const trigger = screen.getByRole('button', { name: /^Date Range:/ })
    trigger.focus()
    await user.keyboard('{Enter}')
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    await user.click(screen.getByRole('button', { name: /October 12th/ }))
    await user.keyboard('{ArrowRight}{Enter}')
    await user.click(screen.getByRole('button', { name: 'Apply range' }))

    expect(onChange).toHaveBeenCalledExactlyOnceWith({
      start: new Date(2026, 9, 12),
      end: new Date(2026, 9, 13, 23, 59, 59),
    })
  })

  test('keeps custom seconds on the same day and identifies the local timezone', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <DateTimeRangePicker
        start={new Date(2026, 9, 1)}
        onChange={onChange}
      />
    )
    await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
    await user.click(screen.getByRole('button', { name: /October 12th/ }))
    await user.click(screen.getByRole('button', { name: /October 12th/ }))
    fireEvent.change(screen.getByLabelText('Start Time'), { target: { value: '10:23:45' } })
    fireEvent.change(screen.getByLabelText('End Time'), { target: { value: '11:34:56' } })
    expect(screen.getByText(`Local time: ${Intl.DateTimeFormat().resolvedOptions().timeZone}`)).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Apply range' }))

    expect(onChange).toHaveBeenCalledExactlyOnceWith({
      start: new Date(2026, 9, 12, 10, 23, 45),
      end: new Date(2026, 9, 12, 11, 34, 56),
    })
  })

  test('discards a preset on Cancel and restores the controlled range on reopening', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <DateTimeRangePicker
        start={new Date(2026, 9, 12, 10)}
        end={new Date(2026, 9, 13, 11)}
        onChange={onChange}
      />
    )
    await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
    await user.click(screen.getByRole('button', { name: 'Today' }))
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onChange).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: /^Date Range:/ }))

    expect(screen.getByLabelText('Start Time')).toHaveValue('10:00:00')
    expect(screen.getByLabelText('End Time')).toHaveValue('11:00:00')
    expect(screen.getByText('2026-10-12')).toBeVisible()
    await user.keyboard('{Escape}')
    expect(onChange).not.toHaveBeenCalled()
  })

  test.each([
    ['Today', new Date(2026, 9, 30), new Date(2026, 9, 30, 23, 59, 59)],
    ['Last 7 days', new Date(2026, 9, 24), new Date(2026, 9, 30, 23, 59, 59)],
    ['Last 30 days', new Date(2026, 9, 1), new Date(2026, 9, 30, 23, 59, 59)],
    ['This month', new Date(2026, 9, 1), new Date(2026, 9, 31, 23, 59, 59)],
  ])('stages %s until Apply and preserves the end of the final day', async (preset, start, end) => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date(2026, 9, 30, 14))
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<DateTimeRangePicker onChange={onChange} />)
    await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
    await user.click(screen.getByRole('button', { name: preset }))
    expect(onChange).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Apply range' }))
    expect(onChange).toHaveBeenCalledExactlyOnceWith({ start, end })
  })

  test('requires the second calendar click before applying an unfinished selection', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <DateTimeRangePicker
        start={new Date(2026, 9, 1)}
        onChange={onChange}
      />
    )
    await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
    await user.click(screen.getByRole('button', { name: /October 12th/ }))
    await user.click(screen.getByRole('button', { name: 'Apply range' }))

    expect(screen.getByRole('alert')).toHaveTextContent('Select an end date.')
    expect(onChange).not.toHaveBeenCalled()
  })

  test('rejects a reversed calendar range and allows correcting it', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <DateTimeRangePicker
        start={new Date(2026, 9, 1)}
        onChange={onChange}
      />
    )
    await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
    await user.click(screen.getByRole('button', { name: /October 13th/ }))
    await user.click(screen.getByRole('button', { name: /October 12th/ }))
    await user.click(screen.getByRole('button', { name: 'Apply range' }))
    expect(screen.getByRole('alert')).toHaveTextContent('End time must not be before start time.')
    expect(onChange).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: /October 12th/ }))
    await user.click(screen.getByRole('button', { name: /October 13th/ }))
    await user.click(screen.getByRole('button', { name: 'Apply range' }))
    expect(onChange).toHaveBeenCalledExactlyOnceWith({
      start: new Date(2026, 9, 12),
      end: new Date(2026, 9, 13, 23, 59, 59),
    })
  })

  test.each(['', '24:00', '10:60'])('rejects invalid time %s rather than normalizing or clearing the date', async (time) => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <DateTimeRangePicker
        start={new Date(2026, 9, 12)}
        end={new Date(2026, 9, 13)}
        onChange={onChange}
      />
    )
    await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
    fireEvent.change(screen.getByLabelText('Start Time'), { target: { value: time } })
    await user.click(screen.getByRole('button', { name: 'Apply range' }))

    expect(screen.getByRole('alert')).toHaveTextContent('Enter a valid time.')
    expect(screen.getByLabelText('Start Time')).toHaveAttribute('aria-invalid', 'true')
    expect(onChange).not.toHaveBeenCalled()
  })

  test('rejects invalid controlled dates without rendering them into the calendar', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <DateTimeRangePicker
        start={new Date('invalid')}
        onChange={onChange}
      />
    )
    await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
    await user.click(screen.getByRole('button', { name: 'Apply range' }))

    expect(screen.getByRole('alert')).toHaveTextContent('Select a valid date range.')
    expect(onChange).not.toHaveBeenCalled()
  })

  test('rejects reversed times on the same day rather than sending a negative range', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <DateTimeRangePicker
        start={new Date(2026, 9, 12, 12)}
        end={new Date(2026, 9, 12, 11)}
        onChange={onChange}
      />
    )
    await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
    await user.click(screen.getByRole('button', { name: 'Apply range' }))
    expect(screen.getByRole('alert')).toHaveTextContent('End time must not be before start time.')
    expect(onChange).not.toHaveBeenCalled()
  })

  test('rejects local times skipped by daylight saving instead of moving the selected hour', async () => {
    vi.stubEnv('TZ', 'America/New_York')
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <DateTimeRangePicker
        start={new Date(2026, 2, 8)}
        end={new Date(2026, 2, 8, 23, 59, 59)}
        onChange={onChange}
      />
    )
    await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
    fireEvent.change(screen.getByLabelText('Start Time'), { target: { value: '02:30:00' } })
    await user.click(screen.getByRole('button', { name: 'Apply range' }))
    expect(screen.getByRole('alert')).toHaveTextContent('Enter a valid time.')
    expect(onChange).not.toHaveBeenCalled()
  })

  test('supports explicitly clearing one boundary and applying a fully cleared range', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <DateTimeRangePicker
        start={new Date(2026, 9, 12)}
        end={new Date(2026, 9, 13, 23, 59, 59)}
        onChange={onChange}
      />
    )
    await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
    await user.click(screen.getByRole('button', { name: 'Clear start date' }))
    expect(screen.getByLabelText('Start Time')).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Apply range' }))
    expect(onChange).toHaveBeenCalledExactlyOnceWith({ start: undefined, end: new Date(2026, 9, 13, 23, 59, 59) })

    await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
    await user.click(screen.getByRole('button', { name: 'Clear range' }))
    await user.click(screen.getByRole('button', { name: 'Apply range' }))
    expect(onChange).toHaveBeenLastCalledWith({ start: undefined, end: undefined })
  })
})
