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
import i18next from 'i18next'
import type React from 'react'
import { beforeAll, describe, expect, test } from 'vitest'

import { formatLogQuota } from '@/lib/format'

import { LogCostDisplay } from '../log-cost-display'

function renderCost(
  props: React.ComponentProps<typeof LogCostDisplay>
): ReturnType<typeof render> {
  return render(<LogCostDisplay {...props} />)
}

function normalizedText(value: string | null): string {
  return (value ?? '').replaceAll(/\s/g, '')
}

describe('log cost display', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      Subscription: 'Subscription',
      'Deducted by subscription': 'Deducted by subscription',
      'Includes tool-call surcharge': 'Includes tool-call surcharge',
    })
  })

  test('shows an estimate marker beside a nonzero cost', () => {
    renderCost({ quota: 12500, other: { usage_accuracy: 'estimated' } })
    expect(screen.getByText('Estimated usage')).toBeInTheDocument()
  })

  test('shows unknown usage even when the recorded cost is zero', () => {
    renderCost({ quota: 0, other: { usage_accuracy: 'unknown' } })
    expect(screen.getByText('Usage unknown')).toBeInTheDocument()
  })

  test('does not treat old records without provenance as confirmed usage', () => {
    renderCost({ quota: 12500, other: null })
    expect(screen.getByText('Usage provenance unavailable')).toBeInTheDocument()
    expect(screen.queryByText('Reported usage')).not.toBeInTheDocument()
  })

  test('does not interpret unrecognized provenance as reported usage', () => {
    renderCost({ quota: 12500, other: { usage_accuracy: 'invalid-source' } })
    expect(screen.getByText('Usage provenance unavailable')).toBeInTheDocument()
  })

  test('explains reported usage through a keyboard-accessible tooltip', async () => {
    const user = userEvent.setup()
    renderCost({ quota: 12500, other: { usage_accuracy: 'reported' } })
    await user.tab()
    expect(screen.getByText('Reported usage')).toHaveFocus()
    expect(await screen.findByRole('tooltip')).toHaveTextContent('Token usage was reported by upstream; the provider bill has not been reconciled.')
  })

  test('keeps provenance out of non-consumption costs', () => {
    renderCost({ quota: 12500, other: null, showUsageAccuracy: false })
    expect(screen.queryByText('Usage provenance unavailable')).not.toBeInTheDocument()
  })

  test('keeps the regular cost visible and adds an accessible surcharge marker', () => {
    const rendered = renderCost({
      quota: 12500,
      other: {
        tool_surcharges: [{ name: 'lookup_customer', count: 1, price: 5 }],
      },
    })

    expect(
      normalizedText(rendered.container.textContent).includes(
        normalizedText(formatLogQuota(12500))
      )
    ).toBe(true)
    const marker = screen.getByRole('img', {
      name: 'Includes tool-call surcharge',
    })
    expect(marker).toHaveAttribute('data-tool-surcharge-indicator', 'true')
    expect(marker).toHaveAttribute('tabindex', '0')
  })

  test('preserves the subscription badge and adds the same legacy surcharge marker', () => {
    renderCost({
      quota: 5000,
      other: {
        billing_source: 'subscription',
        web_search: true,
        web_search_call_count: 1,
        web_search_price: 10,
      },
    })

    expect(screen.getByText('Subscription')).toBeInTheDocument()
    expect(
      screen.getByRole('img', { name: 'Includes tool-call surcharge' })
    ).toHaveAttribute('data-tool-surcharge-indicator', 'true')
  })
})
