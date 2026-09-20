import type { Cell, Table } from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { ReactNode } from 'react'
import { describe, expect, test } from 'vitest'

import { LOG_TYPE_ENUM } from '../../constants'
import type { UsageLog } from '../../data/schema'
import { UsageLogsMobileList } from '../usage-logs-mobile-card'
import { UsageLogsProvider } from '../usage-logs-provider'

function makeCell(
  id: string,
  value: ReactNode,
  row: { original: UsageLog }
): Cell<UsageLog, unknown> {
  return {
    column: {
      id,
      columnDef: { cell: () => value },
    },
    getContext: () => ({}),
    row,
  } as unknown as Cell<UsageLog, unknown>
}

function makeTable({
  includeContent = true,
  rows = true,
} = {}): Table<UsageLog> {
  if (!rows) {
    return { getRowModel: () => ({ rows: [] }) } as unknown as Table<UsageLog>
  }

  const original: UsageLog = {
    id: 1,
    user_id: 2,
    created_at: 1_756_560_000,
    type: LOG_TYPE_ENUM.CONSUME,
    content: 'request payload',
    username: 'teammate',
    token_name: 'research-key',
    model_name: 'gpt-5',
    quota: 12,
    prompt_tokens: 10,
    completion_tokens: 8,
    use_time: 1,
    is_stream: false,
    channel: 3,
    channel_name: 'Codex',
    token_id: 4,
    group: 'default',
    ip: '',
    other: '',
    request_id: 'req-mobile',
    upstream_request_id: '',
  }
  const row = { original } as { original: UsageLog }
  const allCells = [
    makeCell('created_at', 'time', row),
    makeCell('model_name', 'gpt-5', row),
    makeCell('quota', '12', row),
    makeCell('channel', 'Codex', row),
    makeCell('user', 'teammate', row),
    makeCell('token_name', 'research-key', row),
    makeCell('use_time', '1s', row),
    makeCell('prompt_tokens', '10', row),
    makeCell('content', <button type='button'>View content</button>, row),
  ]
  const visibleCells = includeContent
    ? allCells
    : allCells.filter((cell) => cell.column.id !== 'content')
  return {
    getRowModel: () => ({
      rows: [
        {
          id: 'row-1',
          original,
          getVisibleCells: () => visibleCells,
          getAllCells: () => allCells,
        },
      ],
    }),
  } as unknown as Table<UsageLog>
}

function renderList(
  table: Table<UsageLog>,
  props: Record<string, unknown> = {}
) {
  return render(
    <UsageLogsProvider>
      <UsageLogsMobileList table={table} logCategory='common' {...props} />
    </UsageLogsProvider>
  )
}

describe('usage logs mobile list', () => {
  test('keeps the details action visible when desktop content column is hidden', () => {
    renderList(makeTable({ includeContent: false }))

    expect(screen.getByText('gpt-5')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'View content' })
    ).toBeInTheDocument()
  })

  test('renders loading and empty states without a table row', () => {
    const { rerender } = renderList(makeTable({ rows: false }), {
      isLoading: true,
    })
    expect(
      document.querySelectorAll('[data-slot="skeleton"]').length
    ).toBeGreaterThan(0)

    rerender(
      <UsageLogsProvider>
        <UsageLogsMobileList
          table={makeTable({ rows: false })}
          logCategory='common'
        />
      </UsageLogsProvider>
    )
    expect(screen.getByText('No Logs Found')).toBeInTheDocument()
  })
})
