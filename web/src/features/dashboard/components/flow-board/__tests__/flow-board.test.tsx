/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AnchorHTMLAttributes } from 'react'
import { describe, expect, test, vi } from 'vitest'

import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import type { ConnectionSummary } from '@/features/connections/connection-state'

import type { FlowKeyNode, FlowSelection } from '../../../lib/overview-flow'
import {
  FlowBoard,
  type FlowBoardConnections,
  type FlowBoardKeys,
} from '../flow-board'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    children,
    ...attributes
  }: AnchorHTMLAttributes<HTMLAnchorElement> & { to: string }) => (
    <a {...attributes} href={to}>
      {children}
    </a>
  ),
}))

const keyNode = (id: number, overrides: Partial<FlowKeyNode> = {}) => ({
  id,
  name: `Key ${id}`,
  status: 1,
  own: true,
  requests: 0,
  failures: 0,
  ...overrides,
})

function keysColumn(overrides: Partial<FlowBoardKeys> = {}): FlowBoardKeys {
  return {
    nodes: [keyNode(21, { requests: 3, failures: 1 })],
    state: 'ready',
    onRetry: vi.fn(),
    createAction: null,
    ...overrides,
  }
}

const connection = (
  id: number,
  overrides: Partial<ConnectionSummary> = {}
): ConnectionSummary => ({
  id,
  type: 1,
  name: `Connection ${id}`,
  status: 1,
  priority: 1,
  ...overrides,
})

function connectionsColumn(
  overrides: Partial<FlowBoardConnections> = {}
): FlowBoardConnections {
  return {
    items: [connection(1)],
    total: 1,
    windows: new Map(),
    windowsError: false,
    onRetryWindows: vi.fn(),
    traffic: new Map(),
    state: 'ready',
    onRetry: vi.fn(),
    addAction: null,
    ...overrides,
  }
}

function renderBoard(props: {
  keys?: FlowBoardKeys
  connections?: FlowBoardConnections | null
  selection?: FlowSelection | null
  onSelect?: (selection: FlowSelection | null) => void
}) {
  const onSelect = props.onSelect ?? vi.fn()
  render(
    <FlowBoard
      keys={props.keys ?? keysColumn()}
      connections={
        props.connections === undefined
          ? connectionsColumn()
          : props.connections
      }
      core={<div>core card</div>}
      caption='Node counts'
      selection={props.selection ?? null}
      onSelect={onSelect}
    />
  )
  return onSelect
}

const keyButton = (id: number) =>
  screen
    .getAllByRole('button')
    .find((button) => button.dataset.flowNode === `key-${id}`)

describe('selecting nodes', () => {
  test('clicking a Key node selects it by its stable ID', async () => {
    const onSelect = renderBoard({})
    await userEvent.setup().click(keyButton(21) as HTMLElement)
    expect(onSelect).toHaveBeenCalledWith({ kind: 'key', id: 21 })
  })

  test('clicking the selected node again clears the selection', async () => {
    const onSelect = renderBoard({ selection: { kind: 'key', id: 21 } })
    const node = keyButton(21) as HTMLElement
    expect(node).toHaveAttribute('aria-pressed', 'true')
    await userEvent.setup().click(node)
    expect(onSelect).toHaveBeenCalledWith(null)
  })

  test.each(['{Enter}', ' '])(
    'the keyboard selects a focused node with %s',
    async (key) => {
      const onSelect = renderBoard({})
      const user = userEvent.setup()
      await user.tab()
      expect(keyButton(21)).toHaveFocus()
      await user.keyboard(key)
      expect(onSelect).toHaveBeenCalledWith({ kind: 'key', id: 21 })
    }
  )

  test('a connection node selects the connection', async () => {
    const onSelect = renderBoard({})
    await userEvent
      .setup()
      .click(screen.getByRole('button', { name: /Connection 1/ }))
    expect(onSelect).toHaveBeenCalledWith({ kind: 'connection', id: 1 })
  })
})

describe('folding', () => {
  test('six Keys are all shown without a view-all link', () => {
    renderBoard({
      keys: keysColumn({
        nodes: [1, 2, 3, 4, 5, 6].map((id) => keyNode(id)),
      }),
    })
    expect(
      screen
        .getAllByRole('button')
        .filter((button) => button.dataset.flowNode?.startsWith('key-'))
    ).toHaveLength(6)
    expect(
      screen.queryByRole('link', { name: /View all/ })
    ).not.toBeInTheDocument()
  })

  test('seven Keys fold to six with a link to the Key list', () => {
    renderBoard({
      keys: keysColumn({
        nodes: [1, 2, 3, 4, 5, 6, 7].map((id) => keyNode(id)),
      }),
    })
    expect(
      screen
        .getAllByRole('button')
        .filter((button) => button.dataset.flowNode?.startsWith('key-'))
    ).toHaveLength(6)
    expect(screen.getByRole('link', { name: 'View all 7' })).toHaveAttribute(
      'href',
      '/keys'
    )
  })

  test('a selected seventh Key stays visible', () => {
    renderBoard({
      keys: keysColumn({
        nodes: [1, 2, 3, 4, 5, 6, 7].map((id) => keyNode(id)),
      }),
      selection: { kind: 'key', id: 7 },
    })
    expect(keyButton(7)).toHaveAttribute('aria-pressed', 'true')
  })

  test('more connections than shown link to the connection list', () => {
    renderBoard({
      connections: connectionsColumn({
        items: [1, 2, 3, 4, 5, 6].map((id) => connection(id)),
        total: 9,
      }),
    })
    expect(screen.getByRole('link', { name: 'View all 9' })).toHaveAttribute(
      'href',
      '/connections'
    )
  })
})

describe('node content', () => {
  test('a subscription window ring always writes its value', () => {
    const window: ChannelQuotaChangeItem = {
      channel_id: 1,
      name: 'Account',
      metric_type: 'codex_rate_limit',
      unit: 'percent',
      status: 'success',
      window_type: 'five_hour',
      window_seconds: 18000,
      current_available: 18,
      observed_at: Math.floor(Date.now() / 1000),
    }
    renderBoard({
      connections: connectionsColumn({
        items: [connection(1, { type: 57 })],
        windows: new Map([[1, window]]),
      }),
    })
    const ring = screen.getByRole('img', {
      name: '5 hours: 82% used, 18% left',
    })
    expect(ring).toHaveTextContent('82%')
    expect(screen.getByText('5 hours · 18% left')).toBeVisible()
  })

  test('a metered connection says so instead of drawing a ring', () => {
    renderBoard({})
    const node = screen.getByRole('button', { name: /Connection 1/ })
    expect(within(node).getByText('Metered')).toBeVisible()
    expect(within(node).queryByRole('img')).not.toBeInTheDocument()
  })

  test('unknown request counts read as unknown, never as zero', () => {
    renderBoard({
      keys: keysColumn({
        nodes: [keyNode(21, { requests: null, failures: null })],
      }),
    })
    const node = keyButton(21) as HTMLElement
    expect(within(node).getByText('Unknown')).toBeVisible()
    expect(within(node).queryByText(/0 requests/)).not.toBeInTheDocument()
  })

  test('failures among the sampled requests are shown on the Key', () => {
    renderBoard({})
    expect(
      within(keyButton(21) as HTMLElement).getByText('Failed 1')
    ).toBeVisible()
  })

  test('without connection permission there is no upstream column', () => {
    renderBoard({ connections: null })
    expect(
      screen.queryByRole('heading', { name: /Upstream/ })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /API Key/ })).toBeVisible()
  })
})

describe('column states', () => {
  test('no Keys shows the create action', () => {
    renderBoard({
      keys: keysColumn({
        nodes: [],
        createAction: <a href='/keys'>Create API Key</a>,
      }),
    })
    expect(screen.getByText('No API Keys yet')).toBeVisible()
    expect(screen.getByRole('link', { name: 'Create API Key' })).toBeVisible()
  })

  test('a failed connection read offers a retry and shows no count', async () => {
    const onRetry = vi.fn()
    renderBoard({
      connections: connectionsColumn({
        items: [],
        total: 0,
        state: 'error',
        onRetry,
      }),
    })
    expect(screen.getByText('Connections could not be loaded.')).toBeVisible()
    expect(screen.getByRole('heading', { name: 'Upstream' })).toBeVisible()
    await userEvent.setup().click(screen.getByRole('button', { name: 'Retry' }))
    expect(onRetry).toHaveBeenCalled()
  })

  test('unreadable account windows are said to be missing and can be retried', async () => {
    const onRetryWindows = vi.fn()
    renderBoard({
      connections: connectionsColumn({
        items: [connection(1, { type: 57 })],
        windowsError: true,
        onRetryWindows,
      }),
    })
    const notice = screen.getByTestId('accounts-error-state')
    expect(notice).toHaveTextContent('Saved account windows could not be read.')
    await userEvent
      .setup()
      .click(within(notice).getByRole('button', { name: 'Retry' }))
    expect(onRetryWindows).toHaveBeenCalled()
  })

  test('loading columns are marked busy', () => {
    renderBoard({ keys: keysColumn({ nodes: [], state: 'loading' }) })
    expect(
      screen
        .getByRole('heading', { name: 'API Key' })
        .parentElement?.querySelector('[aria-busy="true"]')
    ).not.toBeNull()
  })
})
