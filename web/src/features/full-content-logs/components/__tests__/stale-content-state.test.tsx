/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { useMediaQuery } from '@/hooks'

import { FullContentLogs } from '../../index'
import { FullContentLogDetailsDialog } from '../full-content-log-details-dialog'

vi.mock('@tanstack/react-query', () => ({
  useQuery: vi.fn(),
  useMutation: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useQueryClient: vi.fn(() => ({ invalidateQueries: vi.fn() })),
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

vi.mock('@/hooks', () => ({
  useMediaQuery: vi.fn(() => true),
}))

vi.mock('@/hooks/use-media-query', () => ({
  useMediaQuery: vi.fn(() => true),
}))

vi.mock('@/stores/auth-store', () => ({
  useAuthStore: (selector: (state: unknown) => unknown) =>
    selector({ auth: { user: null, session: null } }),
}))

vi.mock('@/components/layout', () => {
  const Slot = ({ children }: { children?: ReactNode }) => <>{children}</>
  const Layout = ({ children }: { children?: ReactNode }) => (
    <main>{children}</main>
  )
  Layout.Title = Slot
  Layout.Actions = Slot
  Layout.Content = Slot
  return { SectionPageLayout: Layout }
})

vi.mock('@/components/ui/button', () => ({
  Button: (props: { children?: ReactNode; onClick?: () => void }) => (
    <button type='button' onClick={props.onClick}>
      {props.children}
    </button>
  ),
}))

vi.mock('@/components/ui/skeleton', () => ({
  Skeleton: () => <div />,
}))

vi.mock('@/components/ui/table', () => ({
  Table: ({ children }: { children?: ReactNode }) => <table>{children}</table>,
  TableBody: ({ children }: { children?: ReactNode }) => (
    <tbody>{children}</tbody>
  ),
  TableCell: ({ children }: { children?: ReactNode }) => <td>{children}</td>,
  TableHead: ({ children }: { children?: ReactNode }) => <th>{children}</th>,
  TableHeader: ({ children }: { children?: ReactNode }) => (
    <thead>{children}</thead>
  ),
  TableRow: ({ children }: { children?: ReactNode }) => <tr>{children}</tr>,
}))

vi.mock('@/components/ui/badge', () => ({
  Badge: ({ children }: { children?: ReactNode }) => <span>{children}</span>,
}))

vi.mock('@/components/ui/tabs', () => ({
  Tabs: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  TabsContent: ({ children }: { children?: ReactNode }) => (
    <div>{children}</div>
  ),
  TabsList: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  TabsTrigger: ({ children }: { children?: ReactNode }) => (
    <button>{children}</button>
  ),
}))

vi.mock('@/components/dialog', () => ({
  Dialog: ({ children }: { children?: ReactNode }) => (
    <section>{children}</section>
  ),
}))

vi.mock('@/hooks/use-copy-to-clipboard', () => ({
  useCopyToClipboard: () => ({ copiedText: null, copyToClipboard: vi.fn() }),
}))

vi.mock('@/lib/utils', () => ({
  cn: (...values: unknown[]) => values.filter(Boolean).join(' '),
}))

vi.mock('../../api', () => ({
  getFullContentLogs: vi.fn(),
  getFullContentLogDetail: vi.fn(),
  getFullContentLogFiles: vi.fn(),
  downloadFullContentLogFile: vi.fn(),
  deleteFullContentLogFile: vi.fn(),
  deleteAllFullContentLogFiles: vi.fn(),
}))

vi.mock('../full-content-log-filter-bar', () => ({
  FullContentLogFilterBar: () => null,
}))

vi.mock('../full-content-log-files-dialog', () => ({
  FullContentLogFilesDialog: () => null,
}))

vi.mock('../full-content-log-view-mode-toggle', () => ({
  FullContentLogViewModeToggle: () => null,
}))

vi.mock('../full-content-log-row', () => ({
  FullContentLogMobileCard: ({ log }: { log: { request_id: string } }) => (
    <article data-testid='mobile-log'>{log.request_id}</article>
  ),
  FullContentLogRow: ({ log }: { log: { request_id: string } }) => (
    <tr data-testid='desktop-log'>{log.request_id}</tr>
  ),
}))

function queryState(overrides: Record<string, unknown> = {}) {
  return {
    data: undefined,
    error: null,
    isError: false,
    isLoading: false,
    isFetching: false,
    refetch: vi.fn(),
    ...overrides,
  }
}

const LOG_LIST = {
  enabled: true,
  page: 1,
  page_size: 20,
  total: 1,
  items: [
    {
      timestamp: '2026-08-28T01:00:00Z',
      request_id: 'stale-request-1',
      method: 'POST',
      path: '/v1/responses',
      model: 'gpt-5.6-luna',
      status: 200,
      duration_ms: 10,
      request_bytes: 10,
      response_bytes: 20,
      chunk_count: 1,
    },
  ],
  files: { count: 0, total_size: 0, files: [] },
  facets: { models: [], tokens: [] },
}

const LOG_DETAIL = {
  ...LOG_LIST.items[0],
  request_body: 'sensitive request body',
  response_body: 'sensitive response body',
  request_headers: {},
  response_headers: {},
  query: {},
}

describe('full content logs stale error state', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useMediaQuery).mockReturnValue(true)
  })

  test('hides previously loaded mobile rows when list refetch fails', () => {
    vi.mocked(useQuery).mockReturnValue(
      queryState({
        data: LOG_LIST,
        error: new Error('forbidden'),
        isError: true,
      }) as never
    )

    render(<FullContentLogs />)

    expect(screen.getAllByText('forbidden').length).toBeGreaterThan(0)
    expect(screen.queryByTestId('mobile-log')).not.toBeInTheDocument()
  })

  test('hides previously loaded desktop rows when list refetch fails', () => {
    vi.mocked(useMediaQuery).mockReturnValue(false)
    vi.mocked(useQuery).mockReturnValue(
      queryState({
        data: LOG_LIST,
        error: new Error('forbidden'),
        isError: true,
      }) as never
    )

    render(<FullContentLogs />)

    expect(screen.queryByTestId('desktop-log')).not.toBeInTheDocument()
  })

  test('hides previously loaded detail body when detail refetch fails', () => {
    vi.mocked(useMediaQuery).mockReturnValue(false)
    const refetch = vi.fn()
    vi.mocked(useQuery).mockReturnValue(
      queryState({
        data: LOG_DETAIL,
        error: new Error('forbidden'),
        isError: true,
        refetch,
      }) as never
    )

    render(
      <FullContentLogDetailsDialog
        requestId='stale-request-1'
        open
        onOpenChange={() => undefined}
      />
    )

    expect(screen.getByText('forbidden')).toBeInTheDocument()
    expect(screen.queryByText('sensitive request body')).not.toBeInTheDocument()
    expect(
      screen.queryByText('sensitive response body')
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(refetch).toHaveBeenCalledTimes(1)
  })
})
