/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { QuotaRing } from '../quota-ring'

describe('quota ring', () => {
  test('shows the used percentage as text inside the ring, not only as a fill', () => {
    render(<QuotaRing used={82} label='5 hours window: 82% used' />)
    const ring = screen.getByRole('img', { name: '5 hours window: 82% used' })
    expect(ring).toHaveTextContent('82%')
  })

  test.each([
    [12, 'normal'],
    [60, 'warning'],
    [80, 'critical'],
  ])('%i%% used is drawn with the %s meter colour', (used, severity) => {
    render(<QuotaRing used={used} label='window' />)
    expect(screen.getByRole('img', { name: 'window' })).toHaveAttribute(
      'data-severity',
      severity
    )
  })

  test('values outside 0-100 are clamped for drawing but the text stays honest', () => {
    render(<QuotaRing used={100} label='full' />)
    expect(screen.getByRole('img', { name: 'full' })).toHaveTextContent('100%')
  })
})
