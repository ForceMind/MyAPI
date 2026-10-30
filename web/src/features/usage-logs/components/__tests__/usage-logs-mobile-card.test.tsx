/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { Cell, Table } from '@tanstack/react-table'
import { fireEvent, render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, test, vi } from 'vitest'

import { LOG_TYPE_ENUM } from '../../constants'
import type { UsageLog } from '../../data/schema'
import { UsageLogsMobileList } from '../usage-logs-mobile-card'
import { UsageLogsProvider } from '../usage-logs-provider'

vi.mock('@tanstack/react-router', () => ({
  Link: (props: {
    children?: ReactNode
    title?: string
    params: { requestId: string }
  }) => (
    <a
      href={`/usage-logs/request/${props.params.requestId}`}
      title={props.title}
    >
      {props.children}
    </a>
  ),
}))

const LOG: UsageLog = {
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

function makeCell<TData>(
  id: string,
  value: ReactNode,
  row: { original: TData }
): Cell<TData, unknown> {
  return {
    column: {
      id,
      columnDef: { cell: () => value },
    },
    getContext: () => ({}),
    row,
  } as unknown as Cell<TData, unknown>
}

/** A table whose rows carry these records; call records read the record. */
function makeTable<TData>(
  originals: TData[],
  cellsFor: (row: { original: TData }) => {
    visible: Cell<TData, unknown>[]
    all: Cell<TData, unknown>[]
  } = () => ({ visible: [], all: [] })
): Table<TData> {
  return {
    getRowModel: () => ({
      rows: originals.map((original, index) => {
        const row = { original }
        const cells = cellsFor(row)
        return {
          id: `row-${index + 1}`,
          original,
          getVisibleCells: () => cells.visible,
          getAllCells: () => cells.all,
        }
      }),
    }),
  } as unknown as Table<TData>
}

function renderList<TData>(
  table: Table<TData>,
  props: { isLoading?: boolean; logCategory?: 'common' | 'drawing' | 'task' } = {}
) {
  return render(
    <UsageLogsProvider>
      <UsageLogsMobileList
        table={table}
        logCategory={props.logCategory ?? 'common'}
        isLoading={props.isLoading}
      />
    </UsageLogsProvider>
  )
}

describe('usage logs mobile list', () => {
  test('a relayed request opens its request page from the whole row', () => {
    renderList(makeTable([LOG]))

    const row = screen.getByTitle('Click to view full details')
    expect(row).toHaveAttribute('href', '/usage-logs/request/req-mobile')
    expect(within(row).getByText('gpt-5')).toBeInTheDocument()
    expect(within(row).getByText('req-mobile')).toBeInTheDocument()
    expect(within(row).getByText('research-key')).toBeInTheDocument()
    expect(within(row).getByText('18 Tokens')).toBeInTheDocument()
  })

  test('HTTP success, missing usage and settlement review remain independent facts', () => {
    renderList(
      makeTable([{
        ...LOG,
        type: LOG_TYPE_ENUM.ERROR,
        quota: 0,
        prompt_tokens: 0,
        completion_tokens: 0,
        other: JSON.stringify({
          request_outcome: 'completed',
          http_status: 200,
          usage_outcome: 'missing',
          settlement_status: 'pending_review',
        }),
      }])
    )

    const row = screen.getByTitle('Click to view full details')
    expect(within(row).getByText('Request successful')).toBeInTheDocument()
    expect(within(row).getByText('HTTP 200')).toBeInTheDocument()
    expect(within(row).getByText('Usage not provided')).toBeInTheDocument()
    expect(within(row).getByText('Settlement pending review')).toBeInTheDocument()
    expect(screen.queryByText('0 Tokens')).not.toBeInTheDocument()
  })

  test('an absent HTTP status remains explicitly unknown', () => {
    renderList(makeTable([LOG]))

    expect(screen.getByText('HTTP status not recorded')).toBeInTheDocument()
    expect(screen.queryByText('HTTP 200')).not.toBeInTheDocument()
  })

  test('Request ID copying has a separate touch target beside the details link', () => {
    renderList(makeTable([LOG]))

    const copy = screen.getByRole('button', { name: 'Copy Request ID' })
    expect(copy).toHaveClass('min-h-11', 'min-w-11')
    expect(screen.getByTitle('Click to view full details')).not.toContainElement(copy)
  })

  test('a record without a Request ID keeps its details as a button', () => {
    renderList(makeTable([{ ...LOG, request_id: '' }]))

    const row = screen.getByTitle('Click to view full details')
    expect(row.tagName).toBe('BUTTON')
    expect(within(row).getByText('gpt-5')).toBeInTheDocument()
  })

  test('missing usage reads as not provided and never as zero Tokens', () => {
    renderList(
      makeTable([
        {
          ...LOG,
          type: LOG_TYPE_ENUM.ERROR,
          prompt_tokens: 0,
          completion_tokens: 0,
          other: JSON.stringify({
            request_outcome: 'completed',
            usage_outcome: 'missing',
          }),
        },
      ])
    )

    const row = screen.getByTitle('Click to view full details')
    expect(within(row).getByText('Usage not provided')).toBeInTheDocument()
    expect(within(row).queryByText('0 Tokens')).not.toBeInTheDocument()
  })

  test('a relayed request shows its timing beside the stream mode', () => {
    renderList(makeTable([LOG]))

    const timing = document.querySelector('[data-slot="mobile-log-timing"]')
    expect(timing?.children).toHaveLength(2)
    expect(timing).toHaveTextContent('Duration')
    expect(timing).toHaveTextContent('Non-stream')
  })

  test('a task keeps its result reachable when the PC view hid that column', () => {
    const task = { id: 9, task_id: 'task-9' }
    const table = makeTable([task], (row) => {
      const visible = [
        makeCell('task_id', 'task-9', row),
        makeCell('status', 'Succeeded', row),
        makeCell('submit_time', 'submitted', row),
      ]
      const result = makeCell(
        'fail_reason',
        <button type='button'>View reason</button>,
        row
      )
      return { visible, all: [...visible, result] }
    })

    renderList(table, { logCategory: 'task' })

    expect(screen.getByText('task-9')).toBeInTheDocument()
    expect(screen.getByText('Succeeded')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'View reason' })
    ).toBeInTheDocument()
  })

  test('a task retains hidden main fields and expands the remaining fields', () => {
    const task = { id: 9, task_id: 'task-hidden' }
    const table = makeTable([task], (row) => ({
      visible: [makeCell('submit_time', 'submitted', row)],
      all: [
        makeCell('task_id', 'task-hidden', row),
        makeCell('status', 'Unknown', row),
        makeCell('submit_time', 'submitted', row),
        makeCell('platform', 'kling', row),
        makeCell('quota', 'Metered amount', row),
        makeCell('user', 'task owner', row),
      ],
    }))

    renderList(table, { logCategory: 'task' })

    expect(screen.getByText('task-hidden')).toBeInTheDocument()
    expect(screen.getByText('Unknown')).toBeInTheDocument()
    expect(screen.getByText('kling')).toBeInTheDocument()
    expect(screen.getByText('Metered amount')).toBeInTheDocument()
    expect(screen.getByText('task owner')).not.toBeVisible()
    fireEvent.click(screen.getByText('More details'))
    expect(screen.getByText('task owner')).toBeVisible()
  })

  test('loading a replacement page hides old record values', () => {
    renderList(makeTable([LOG]), { isLoading: true })

    expect(screen.queryByText('req-mobile')).not.toBeInTheDocument()
    expect(screen.queryByText('18 Tokens')).not.toBeInTheDocument()
    expect(document.querySelector('[data-log-row]')).toBeNull()
  })

  test('renders loading and empty states without a record row', () => {
    const { rerender } = renderList(makeTable<UsageLog>([]), {
      isLoading: true,
    })
    expect(
      document.querySelectorAll('[data-slot="skeleton"]').length
    ).toBeGreaterThan(0)

    rerender(
      <UsageLogsProvider>
        <UsageLogsMobileList
          table={makeTable<UsageLog>([])}
          logCategory='common'
        />
      </UsageLogsProvider>
    )
    expect(screen.getByText('No Logs Found')).toBeInTheDocument()
    expect(document.querySelector('[data-log-row]')).toBeNull()
  })
})
