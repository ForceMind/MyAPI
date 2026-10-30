/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test } from 'vitest'

import type { FullContentLogDetail } from '../../types'
import { SavedStreamEvents } from '../saved-stream-events'

const DETAIL: FullContentLogDetail = {
  timestamp: '2026-10-30T01:02:03Z', request_id: 'saved-events',
  method: 'POST', path: '/v1/responses', status: 200, duration_ms: 10,
  request_bytes: 1, response_bytes: 1, chunk_count: 3,
  request_body: '', response_body: '', request_headers: {},
  response_headers: {}, query: {},
}

describe('saved SSE event preview', () => {
  test('missing content type preserves observed SSE fields and completion/error sources independently', () => {
    render(<SavedStreamEvents detail={{
      ...DETAIL,
      response_body: 'data: [DONE]\n\nevent: error\ndata: {"error":{"message":"later failure"}}\n\n',
    }} />)

    expect(screen.getByText('SSE fields were observed in the saved body; the saved content type does not confirm streaming.')).toBeInTheDocument()
    const list = screen.getByRole('list', { name: 'Saved SSE event preview' })
    expect(within(list).getByText(/Observed completion marker.*data: \[DONE\]/)).toBeInTheDocument()
    expect(within(list).getByText(/Observed error marker.*event: error/)).toBeInTheDocument()
    expect(screen.queryByText('This response was not streamed; the raw body is shown.')).not.toBeInTheDocument()
  })

  test('event 501 is searchable across the full preview and its saved data remains expandable', async () => {
    const user = userEvent.setup()
    const body = Array.from({ length: 501 }, (_, index) => `data: {"text":"saved-event-${index + 1}"}\n\n`).join('')
    render(<SavedStreamEvents detail={{ ...DETAIL, response_body: body }} />)

    const list = screen.getByRole('list', { name: 'Saved SSE event preview' })
    expect(within(list).getAllByRole('listitem')).toHaveLength(50)
    expect(within(list).queryByText('#501')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Show more saved events' })).toBeEnabled()
    await user.type(screen.getByRole('searchbox', { name: 'Search saved event preview' }), 'saved-event-501')

    expect(within(list).getAllByRole('listitem')).toHaveLength(1)
    const sequence = within(list).getByText('#501')
    await user.click(sequence)
    const data = within(list).getByRole('region', { name: 'Saved event data' })
    expect(data).toBeVisible()
    expect(data.querySelector('pre')?.textContent).toBe('{"text":"saved-event-501"}')
    expect(screen.queryByRole('button', { name: 'Show more saved events' })).not.toBeInTheDocument()
  })

  test('loading another local batch reveals previously unrendered saved events', async () => {
    const user = userEvent.setup()
    const body = Array.from({ length: 51 }, (_, index) => `data: event-${index + 1}\n\n`).join('')
    render(<SavedStreamEvents detail={{ ...DETAIL, response_body: body }} />)

    const list = screen.getByRole('list', { name: 'Saved SSE event preview' })
    await user.click(screen.getByRole('button', { name: 'Show more saved events' }))
    expect(within(list).getByText('#51')).toBeInTheDocument()
    expect(within(list).getAllByRole('listitem')).toHaveLength(51)
  })
})
