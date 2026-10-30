/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { formatLogQuota } from '@/lib/format'

import type { UsageLog } from '../../data/schema'
import { useColumnsByCategory } from '../../lib/columns'
import type { LogCategory, MidjourneyLog, TaskLog } from '../../types'
import { UsageLogsProvider } from '../usage-logs-provider'

const CALL: UsageLog = {
  id: 1,
  user_id: 2,
  created_at: 1_756_560_000,
  type: 5,
  content: 'usage_pending_review',
  username: '',
  token_name: '',
  model_name: 'gpt-5',
  quota: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  use_time: 1,
  is_stream: false,
  channel: 3,
  channel_name: '',
  token_id: 4,
  group: 'default',
  ip: '',
  request_id: 'req-review',
  upstream_request_id: '',
  other: JSON.stringify({
    request_outcome: 'completed',
    http_status: 200,
    usage_outcome: 'missing',
    settlement_status: 'pending_review',
  }),
}
const DRAWING: MidjourneyLog = {
  id: 1,
  user_id: 2,
  channel_id: 3,
  code: 1,
  mj_id: 'mj-result',
  action: 'IMAGINE',
  submit_time: 1_756_560_000_000,
  progress: '100%',
  prompt: 'A mountain',
  status: 'FAILURE',
}
const TASK: TaskLog = {
  id: 1,
  user_id: 2,
  platform: 'kling',
  task_id: 'video-result',
  action: 'GENERATE',
  channel_id: 3,
  submit_time: 1_756_560_000,
  status: 'UNKNOWN',
}

type ColumnsViewProps = {
  category: LogCategory
  record: UsageLog | MidjourneyLog | TaskLog
  columnIds: string[]
  isAdmin?: boolean
  showCosts?: boolean
}

function ColumnsView(props: ColumnsViewProps) {
  const columns = useColumnsByCategory(props.category, props.isAdmin ?? false)
  const table = useReactTable({
    data: [props.record],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
  return (
    <div>
      {table.getRowModel().rows[0]
        .getAllCells()
        .filter((cell) => props.columnIds.includes(cell.column.id))
        .map((cell) => (
          <div key={cell.id}>
            {flexRender(cell.column.columnDef.cell, cell.getContext())}
          </div>
        ))}
    </div>
  )
}

function renderColumns(props: ColumnsViewProps) {
  return render(
    <UsageLogsProvider showCosts={props.showCosts}>
      <ColumnsView {...props} />
    </UsageLogsProvider>
  )
}

describe('usage log column evidence', () => {
  test('a completed HTTP request with missing usage keeps settlement pending', () => {
    renderColumns({
      category: 'common',
      record: CALL,
      columnIds: ['request_result', 'prompt_tokens', 'settlement'],
    })

    expect(screen.getByText('Request successful')).toBeInTheDocument()
    expect(screen.getByText('HTTP 200')).toBeInTheDocument()
    expect(screen.getByText('Usage not provided')).toBeInTheDocument()
    expect(screen.getByText('Settlement pending review')).toBeInTheDocument()
    expect(screen.queryByText('0 / 0')).not.toBeInTheDocument()
  })

  test('a drawing exposes business status to its owner without the admin submit result', () => {
    renderColumns({
      category: 'drawing',
      record: DRAWING,
      columnIds: ['status', 'code'],
    })

    expect(screen.getByText('Failed')).toBeInTheDocument()
    expect(screen.queryByText('Submitted')).not.toBeInTheDocument()
  })

  test.each(['drawing', 'task'] as const)(
    '%s missing quota stays unknown instead of becoming a zero charge',
    (category) => {
      renderColumns({
        category,
        record: category === 'drawing' ? DRAWING : TASK,
        columnIds: ['quota'],
      })

      expect(screen.getByText('Unknown')).toBeInTheDocument()
      expect(screen.queryByText(formatLogQuota(0))).not.toBeInTheDocument()
    }
  )

  test.each(['drawing', 'task'] as const)(
    '%s recorded quota respects the page money visibility',
    (category) => {
      const record = {
        ...(category === 'drawing' ? DRAWING : TASK),
        quota: 12000,
      }
      const view = renderColumns({ category, record, columnIds: ['quota'] })
      expect(view.container.textContent?.replace(/\s/g, '')).toBe(
        formatLogQuota(12000).replace(/\s/g, '')
      )

      view.rerender(
        <UsageLogsProvider showCosts={false}>
          <ColumnsView category={category} record={record} columnIds={['quota']} />
        </UsageLogsProvider>
      )
      expect(view.container.textContent).toBe('')
    }
  )

  test('a task with missing status remains unknown instead of claiming submission', () => {
    renderColumns({
      category: 'task',
      record: { ...TASK, status: '' },
      columnIds: ['status'],
    })

    expect(screen.getByText('Unknown')).toBeInTheDocument()
    expect(screen.queryByText('Submitting')).not.toBeInTheDocument()
  })

  test('a successful video task keeps its read-only result preview', () => {
    renderColumns({
      category: 'task',
      record: {
        ...TASK,
        status: 'SUCCESS',
        fail_reason: 'https://provider.example/video.mp4',
      },
      columnIds: ['fail_reason'],
    })

    expect(
      screen.getByRole('link', { name: 'Click to preview video' })
    ).toHaveAttribute('href', '/v1/videos/video-result/content')
  })
})
