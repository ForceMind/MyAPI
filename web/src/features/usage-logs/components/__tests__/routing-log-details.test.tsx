/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { RoutingLogDetails } from '../dialogs/routing-log-details'

describe('routing log visibility', () => {
  test('does not expose routing metadata to ordinary users', () => {
    const { container } = render(
      <RoutingLogDetails
        isAdmin={false}
        routing={{
          channel_id: 701,
          switch_count: 2,
          switch_reason: 'channel_failure',
        }}
      />
    )
    expect(container).toBeEmptyDOMElement()
  })

  test('shows recorded channel and switching information to administrators', () => {
    render(
      <RoutingLogDetails
        isAdmin
        routing={{
          channel_id: 701,
          group: 'special-group',
          sticky: true,
          switch_count: 2,
          switch_reason: 'channel_failure',
        }}
      />
    )
    expect(screen.getByText('701')).toBeInTheDocument()
    expect(screen.getByText('special-group')).toBeInTheDocument()
    expect(screen.getByText('channel_failure')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
  })
})
