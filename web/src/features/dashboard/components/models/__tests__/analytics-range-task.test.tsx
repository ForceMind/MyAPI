import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, expect, it, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import type { DashboardChartPreferences } from '../../../types'
import { ModelsFilter } from '../models-filter-dialog'

const originalAuth = useAuthStore.getState()
const originalAnimations = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'getAnimations')
beforeAll(() => {
  if (!originalAnimations) Object.defineProperty(HTMLElement.prototype, 'getAnimations', { configurable: true, value: () => [] })
})
afterAll(() => {
  if (originalAnimations) Object.defineProperty(HTMLElement.prototype, 'getAnimations', originalAnimations)
  else Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
})
const preferences: DashboardChartPreferences = { consumptionDistributionChart: 'bar', modelAnalyticsChart: 'trend', defaultTimeRangeDays: 7, defaultTimeGranularity: 'day' }
afterEach(() => { cleanup(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

it('commits a cross-month single-calendar range to the filter draft before the original analytics Apply', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, role: 100, username: 'synthetic-admin' })
  const apply = vi.fn()
  const user = userEvent.setup()
  render(<ModelsFilter preferences={preferences} currentFilters={{ start_timestamp: new Date(2026, 9, 1), end_timestamp: new Date(2026, 9, 7), time_granularity: 'day', username: 'synthetic-user' }} onFilterChange={apply} onReset={vi.fn()} />)
  await user.click(screen.getByRole('button', { name: 'Filter' }))
  await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
  expect(screen.getAllByRole('grid')).toHaveLength(1)
  await user.click(screen.getByRole('button', { name: /October 29/ }))
  await user.click(screen.getByRole('button', { name: 'Next month' }))
  await user.click(screen.getByRole('button', { name: /November 2nd/ }))
  await user.click(screen.getByRole('button', { name: 'Apply range' }))
  expect(apply).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Apply Filters' }))
  expect(apply).toHaveBeenCalledExactlyOnceWith({ start_timestamp: new Date(2026, 9, 29), end_timestamp: new Date(2026, 10, 2, 23, 59, 59), time_granularity: 'day', username: 'synthetic-user' })
})

it('discards cancelled calendar and filter drafts while reopening from the applied dates', async () => {
  const apply = vi.fn()
  const user = userEvent.setup()
  render(<ModelsFilter preferences={preferences} currentFilters={{ start_timestamp: new Date(2026, 9, 1), end_timestamp: new Date(2026, 9, 7), time_granularity: 'day' }} onFilterChange={apply} onReset={vi.fn()} />)
  await user.click(screen.getByRole('button', { name: 'Filter' }))
  await user.click(screen.getByRole('button', { name: /^Date Range:/ }))
  await user.click(screen.getByRole('button', { name: /October 12th/ }))
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(screen.getByRole('button', { name: /^Date Range:/ })).toHaveAccessibleName(/2026-10-01.*2026-10-07/)
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Model Analytics Filters' })).not.toBeInTheDocument())
  expect(apply).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Filter' }))
  expect(screen.getByRole('button', { name: /^Date Range:/ })).toHaveAccessibleName(/2026-10-01.*2026-10-07/)
})
