import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import {
  UptimeSparkline,
  UptimeStatusRow,
} from './model-details-uptime-sparkline'

describe('uptime status presentation', () => {
  test('shows the empty label and the degraded status branch', () => {
    render(
      <>
        <UptimeSparkline series={[]} emptyLabel='No uptime data' />
        <UptimeStatusRow
          series={[
            {
              date: '2026-09-01',
              uptime_pct: 96.5,
              incidents: 1,
              outage_minutes: 50,
            },
          ]}
        />
      </>
    )

    expect(screen.getByText('No uptime data')).toBeInTheDocument()
    expect(
      screen.getByText('Degraded performance recently')
    ).toBeInTheDocument()
  })
})
