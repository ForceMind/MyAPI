/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */
import { act, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState, type AnchorHTMLAttributes } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import type { ConnectionSummary } from '@/features/connections/connection-state'
import { api } from '@/lib/api'

import type { FlowSelection } from '../../../lib/overview-flow'
import { FlowBoard, type FlowBoardConnections } from '../flow-board'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    params,
    search,
    state: _state,
    onClick,
    children,
    ...attributes
  }: AnchorHTMLAttributes<HTMLAnchorElement> & {
    to: string
    params?: Record<string, string>
    search?: Record<string, string>
    state?: unknown
  }) => {
    const path = to.replaceAll(/\$(\w+)/g, (_, name) => params?.[name] ?? '')
    const query = new URLSearchParams(search).toString()
    return (
      <a
        {...attributes}
        href={query ? `${path}?${query}` : path}
        onClick={(event) => {
          onClick?.(event)
          // Navigation is exercised with the real router in the page integration.
          event.preventDefault()
        }}
      >
        {children}
      </a>
    )
  },
}))

const longModel =
  'synthetic-model-with-a-complete-long-identifier-for-a-narrow-screen'
const connection = (
  id: number,
  overrides: Partial<ConnectionSummary> = {}
): ConnectionSummary => ({
  id,
  name: `Connection ${id}`,
  type: 1,
  status: 1,
  models: `alpha, ${longModel},alpha, `,
  priority: 0,
  weight: 0,
  ...overrides,
})

beforeEach(() => {
  vi.spyOn(api, 'get').mockRejectedValue(
    new Error('Unexpected saved-data query')
  )
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected write'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected write'))
})
afterEach(() => {
  vi.restoreAllMocks()
})

/** A real parent selection loop protects the node's select/cancel behavior. */
function TestBoard(props: {
  connections?: ConnectionSummary[]
  onSelect: (selection: FlowSelection | null) => void
  onOpenRouting?: (id: number) => void
}) {
  const [selection, setSelection] = useState<FlowSelection | null>(null)
  const connections: FlowBoardConnections = {
    items: props.connections ?? [connection(17)],
    total: props.connections?.length ?? 1,
    windows: new Map(),
    windowsError: false,
    onRetryWindows: vi.fn(),
    traffic: new Map(),
    state: 'ready',
    onRetry: vi.fn(),
    addAction: null,
    tools: props.onOpenRouting
      ? { onOpenRouting: props.onOpenRouting }
      : undefined,
  }
  return (
    <FlowBoard
      keys={{ nodes: [], state: 'ready', onRetry: vi.fn(), createAction: null }}
      connections={connections}
      core={<div>Gateway</div>}
      caption='Saved request sample'
      selection={selection}
      onSelect={(next) => {
        props.onSelect(next)
        setSelection(next)
      }}
    />
  )
}

describe('Full saved connection tools', () => {
  test('keyboard model disclosure expands the complete deduplicated saved list without filtering or network calls', async () => {
    const selected = vi.fn()
    const opened = vi.fn()
    const user = userEvent.setup()
    render(<TestBoard onSelect={selected} onOpenRouting={opened} />)
    const card = screen.getByRole('article', { name: 'Connection #17' })
    const filter = within(card).getByRole('button', { name: /Connection 17/ })
    const toggle = within(card).getByRole('button', { name: 'Models (2)' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(within(card).getByText(longModel)).not.toBeVisible()
    act(() => toggle.focus())
    await user.keyboard('{Enter}')
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(within(card).getByText(longModel)).toBeVisible()
    expect(within(card).getAllByText('alpha')).toHaveLength(1)
    expect(
      within(screen.getByTestId('flow-board')).getByText(
        'Models shown here are saved connection settings.'
      )
    ).toBeVisible()
    expect(filter).toHaveAttribute('aria-pressed', 'false')
    expect(filter.querySelector('button, a')).toBeNull()
    expect(within(card).getByRole('list')).toHaveClass(
      'min-w-0',
      'wrap-anywhere'
    )
    expect(toggle).toHaveClass('min-h-11')
    expect(card).toHaveClass('min-w-0')
    await user.keyboard(' ')
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(selected).not.toHaveBeenCalled()
    expect(opened).not.toHaveBeenCalled()
    expect(api.get).not.toHaveBeenCalled()
    expect(api.post).not.toHaveBeenCalled()
    expect(api.put).not.toHaveBeenCalled()
  })

  test('the filter keeps click, Enter, Space and select-again cancellation while the tools remain separate', async () => {
    const selected = vi.fn()
    const user = userEvent.setup()
    render(<TestBoard onSelect={selected} onOpenRouting={vi.fn()} />)
    const card = screen.getByRole('article', { name: 'Connection #17' })
    const filter = within(card).getByRole('button', { name: /Connection 17/ })
    await user.click(filter)
    expect(selected).toHaveBeenLastCalledWith({ kind: 'connection', id: 17 })
    expect(filter).toHaveAttribute('aria-pressed', 'true')
    await user.keyboard('{Enter}')
    expect(selected).toHaveBeenLastCalledWith(null)
    expect(filter).toHaveAttribute('aria-pressed', 'false')
    await user.keyboard(' ')
    expect(selected).toHaveBeenLastCalledWith({ kind: 'connection', id: 17 })
    await user.click(filter)
    expect(selected).toHaveBeenLastCalledWith(null)
    expect(filter).toHaveAttribute('aria-pressed', 'false')
  })

  test('routing links keep their own IDs and capture only plain same-tab clicks, never node selection', async () => {
    const opened = vi.fn()
    const selected = vi.fn()
    render(
      <TestBoard
        connections={[connection(17), connection(23)]}
        onSelect={selected}
        onOpenRouting={opened}
      />
    )
    const first = screen.getByRole('article', { name: 'Connection #17' })
    const second = screen.getByRole('article', { name: 'Connection #23' })
    expect(
      screen.getAllByText('Models shown here are saved connection settings.')
    ).toHaveLength(1)
    expect(
      within(first).queryByText(
        'Models shown here are saved connection settings.'
      )
    ).not.toBeInTheDocument()
    expect(
      within(second).queryByText(
        'Models shown here are saved connection settings.'
      )
    ).not.toBeInTheDocument()
    expect(
      within(first).getByRole('link', { name: 'Routing and models' })
    ).toHaveAttribute('href', '/connections/17?tab=routing')
    const link = within(second).getByRole('link', {
      name: 'Routing and models',
    })
    expect(link).toHaveAttribute('href', '/connections/23?tab=routing')
    expect(link).toHaveAttribute('data-overview-routing-link', '23')
    expect(link).toHaveClass('min-h-11', 'whitespace-normal')
    fireEvent.click(link, { ctrlKey: true })
    fireEvent.click(link, { metaKey: true })
    fireEvent.click(link, { shiftKey: true })
    fireEvent.click(link, { altKey: true })
    fireEvent.click(link, { button: 1 })
    link.setAttribute('target', '_blank')
    fireEvent.click(link)
    link.removeAttribute('target')
    const prevented = new MouseEvent('click', {
      bubbles: true,
      cancelable: true,
    })
    prevented.preventDefault()
    fireEvent(link, prevented)
    expect(opened).not.toHaveBeenCalled()
    await userEvent.click(link)
    expect(opened).toHaveBeenCalledTimes(1)
    expect(opened).toHaveBeenCalledWith(23)
    expect(selected).not.toHaveBeenCalled()
    expect(api.get).not.toHaveBeenCalled()
    expect(api.put).not.toHaveBeenCalled()
  })

  test('zero routing values remain exact and a saved empty model string is distinct from missing metadata', () => {
    render(
      <TestBoard
        connections={[
          connection(17, { models: null, priority: null, weight: undefined }),
          connection(23, { models: ' , , ' }),
        ]}
        onSelect={vi.fn()}
        onOpenRouting={vi.fn()}
      />
    )
    const unknown = screen.getByRole('article', { name: 'Connection #17' })
    const empty = screen.getByRole('article', { name: 'Connection #23' })
    expect(
      within(unknown).getByText('Priority').nextElementSibling
    ).toHaveTextContent('Unknown')
    expect(
      within(unknown).getByText('Weight').nextElementSibling
    ).toHaveTextContent('Unknown')
    expect(within(unknown).getByText('Models: Unknown')).toBeVisible()
    expect(within(unknown).queryByText('Models (0)')).not.toBeInTheDocument()
    expect(
      within(empty).getByText('Priority').nextElementSibling
    ).toHaveTextContent('0')
    expect(
      within(empty).getByText('Weight').nextElementSibling
    ).toHaveTextContent('0')
    expect(within(empty).getAllByText('Priority')).toHaveLength(1)
    expect(within(empty).queryByText('Priority 0')).not.toBeInTheDocument()
    expect(within(empty).getByText('Models (0)')).toBeVisible()
    expect(
      within(empty).getByText('No model IDs are saved for this connection.')
    ).toBeVisible()
    expect(
      within(empty).queryByRole('button', { name: 'Models (0)' })
    ).not.toBeInTheDocument()
  })

  test('omitting tools keeps the existing Lite node and its filter without models, weight or routing actions', async () => {
    const selected = vi.fn()
    render(<TestBoard onSelect={selected} />)
    expect(screen.queryByRole('article')).not.toBeInTheDocument()
    const filter = screen.getByRole('button', { name: /Connection 17/ })
    expect(within(filter).getByText('Priority 0')).toBeVisible()
    expect(
      screen.queryByRole('button', { name: /Models/ })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Routing and models' })
    ).not.toBeInTheDocument()
    expect(screen.queryByText('Weight')).not.toBeInTheDocument()
    await userEvent.click(filter)
    expect(selected).toHaveBeenCalledWith({ kind: 'connection', id: 17 })
  })
})
