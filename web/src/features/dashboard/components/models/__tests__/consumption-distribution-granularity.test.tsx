/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { ConsumptionDistributionChart } from '../consumption-distribution-chart'

vi.mock('@visactor/react-vchart', () => ({
  VChart: () => <div data-testid='chart' />,
}))

vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: vi.fn() },
}))

vi.mock('@/context/theme-provider', () => ({
  useTheme: () => ({ resolvedTheme: 'light' }),
}))

vi.mock('@/context/theme-customization-provider', () => ({
  useThemeCustomization: () => ({
    customization: { preset: 'default', radius: 'medium' },
  }),
}))

vi.mock('@/lib/theme-radius', () => ({
  useThemeRadiusPx: () => 8,
}))

describe('consumption distribution granularity', () => {
  it('shows the current bucket and reports direct minute/hour/day/week changes', async () => {
    const user = userEvent.setup()
    const onTimeGranularityChange = vi.fn()
    render(
      <ConsumptionDistributionChart
        data={[]}
        timeGranularity='minute'
        onTimeGranularityChange={onTimeGranularityChange}
      />
    )

    const minute = screen.getByRole('button', { name: 'Minute' })
    const hour = screen.getByRole('button', { name: 'Hour' })
    const day = screen.getByRole('button', { name: 'Day' })
    const week = screen.getByRole('button', { name: 'Week' })

    expect(minute).toHaveAttribute('aria-pressed', 'true')
    expect(hour).toHaveAttribute('aria-pressed', 'false')
    expect(day).toHaveAttribute('aria-pressed', 'false')
    expect(week).toHaveAttribute('aria-pressed', 'false')

    await user.click(day)
    expect(onTimeGranularityChange).toHaveBeenCalledWith('day')
  })
})
