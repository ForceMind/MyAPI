/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import type { UsageLog } from '../../data/schema'
import { DetailsDialog } from '../dialogs/details-dialog'

const log: UsageLog = {
  id: 1,
  user_id: 2,
  created_at: 1756560000,
  type: 5,
  content: '',
  username: 'user',
  token_name: 'key',
  model_name: 'public',
  quota: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  use_time: 1,
  is_stream: false,
  channel: 3,
  channel_name: 'Test',
  token_id: 4,
  group: 'default',
  ip: '',
  request_id: '',
  upstream_request_id: '',
  other: JSON.stringify({
    admin_info: {
      model_route: {
        requested_model: 'public',
        upstream_model: 'private-target',
        endpoint: '/v1/responses',
        reason: 'explicit_exact',
        channel_id: 3,
        config_digest: 'config-evidence',
      },
    },
  }),
}
describe('actual model route evidence', () => {
  test.each([2, 5])(
    'administrator sees recorded target endpoint reason and configuration for log type %s',
    (type) => {
      render(
        <DetailsDialog
          log={{ ...log, type }}
          isAdmin
          open
          onOpenChange={() => {}}
        />
      )
      expect(screen.getByText('Actual route evidence')).toBeInTheDocument()
      expect(screen.getByText('private-target')).toBeInTheDocument()
      expect(screen.getByText('/v1/responses')).toBeInTheDocument()
      expect(screen.getByText('explicit_exact')).toBeInTheDocument()
      expect(screen.getByText('config-evidence')).toBeInTheDocument()
    }
  )
  test('nonadministrator never sees admin route evidence even if response contains it', () => {
    render(
      <DetailsDialog log={log} isAdmin={false} open onOpenChange={() => {}} />
    )
    expect(screen.queryByText('Actual route evidence')).not.toBeInTheDocument()
    expect(screen.queryByText('private-target')).not.toBeInTheDocument()
    expect(screen.queryByText('config-evidence')).not.toBeInTheDocument()
  })
})
