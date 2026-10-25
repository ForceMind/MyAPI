/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'

import { apiKeySchema } from '../../types'
import { ApiKeysProvider } from '../api-keys-provider'
import { DataTableRowActions } from '../data-table-row-actions'

const apiKey = apiKeySchema.parse({
  id: 21,
  name: 'Row actions fixture',
  key: 'masked',
  status: 1,
  remain_quota: 100,
  used_quota: 0,
  unlimited_quota: false,
  expired_time: -1,
  created_time: 1,
  accessed_time: 1,
  group: 'default',
  model_limits_enabled: false,
})

test('the View Key requests action grows with a wrapped label and keeps words whole', () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <TooltipProvider>
        <ApiKeysProvider viewRequests={vi.fn()}>
          <DataTableRowActions apiKey={apiKey} />
        </ApiKeysProvider>
      </TooltipProvider>
    </QueryClientProvider>
  )
  const button = screen.getByRole('button', { name: 'View Key requests' })
  // A fixed height let a second line spill over the next row, and the
  // console's inherited `overflow-wrap: anywhere` let the column squeeze the
  // button below its longest word ("reques ts"). Real text bounds are
  // checked in Chromium by tools/ui/key-request-browser.mjs.
  expect(button).toHaveClass(
    'h-auto',
    'whitespace-normal',
    'wrap-break-word',
    'break-keep'
  )
  expect(button).not.toHaveClass('min-w-0')
  expect(button).not.toHaveClass('truncate')
})
