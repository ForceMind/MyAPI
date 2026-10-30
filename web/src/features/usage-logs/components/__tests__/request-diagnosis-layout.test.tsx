/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type { UsageLog } from '../../data/schema'
import { DetailsDialog, LogDetailsContent } from '../dialogs/details-dialog'
import { UsageLogsProvider } from '../usage-logs-provider'

const clients: QueryClient[] = []
const originalAuth = useAuthStore.getState()
const publication = 'a'.repeat(64)
const sourceDigest = 'b'.repeat(64)
const richEvidence = {
  request_outcome: 'completed', usage_outcome: 'reported', usage_accuracy: 'reported',
  http_status: 200, request_path: '/v1/responses',
  request_conversion: ['OpenAI', 'Anthropic'],
  reasoning_effort: 'medium', is_system_prompt_overwritten: true,
  is_model_mapped: true, upstream_model_name: 'mapped-target',
  model_ratio: 1.5, completion_ratio: 2, group_ratio: 1,
  audio: true, audio_input: 42,
  po: ['override preserved_parameter'],
  official_price_source: {
    publication_id: publication, source_sha256: sourceDigest,
    expression_sha256: 'c'.repeat(64), scope: 'reference_tariff_not_upstream_invoice',
  },
  stream_status: {
    status: 'error', end_reason: 'saved-terminal-reason', error_count: 1,
    end_error: 'saved-terminal-error', errors: ['saved-soft-error'],
  },
  admin_info: {
    use_channel: [4, 5],
    model_route: {
      requested_model: 'public-model', upstream_model: 'administrator-target',
      endpoint: '/v1/messages', reason: 'explicit_exact', channel_id: 5,
      config_digest: 'saved-route-digest',
    },
    token_budget: {
      bound_source: 'openai_chat_context_window', input_tokens_bound: 1050000,
      max_output_tokens: 128000, reserved: 1050000,
      actual_input: 0, actual_output: 0,
    },
  },
}

function record(other: Record<string, unknown>, overrides: Partial<UsageLog> = {}): UsageLog {
  return {
    id: 1, user_id: 2, created_at: 1792470000, type: 2,
    content: 'saved request annotation', username: 'owner', token_name: 'identity-key',
    token_id: 8, model_name: 'public-model', channel: 4, channel_name: 'identity-connection',
    quota: 25, prompt_tokens: 0, completion_tokens: 0, use_time: 1,
    is_stream: true, group: 'saved-group', ip: '192.0.2.1',
    request_id: 'REQ-GROUPED', upstream_request_id: 'UPSTREAM-GROUPED',
    other: JSON.stringify(other), ...overrides,
  }
}

beforeEach(() => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
})

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.setState(originalAuth)
  vi.restoreAllMocks()
})

function renderEvidence(log: UsageLog, options: { isAdmin?: boolean; showCosts?: boolean; dialog?: boolean } = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  clients.push(client)
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <UsageLogsProvider showCosts={options.showCosts ?? true}>
          {options.dialog ? (
            <DetailsDialog log={log} isAdmin={options.isAdmin ?? true} open onOpenChange={() => {}} />
          ) : (
            <LogDetailsContent log={log} isAdmin={options.isAdmin ?? true} active embedded presentation='request' />
          )}
        </UsageLogsProvider>
      </TooltipProvider>
    </QueryClientProvider>
  )
}

test('request groups retain protocol, upstream, dispatch, usage and price evidence while the page owns identity and four results', () => {
  renderEvidence(record(richEvidence))
  const identity = screen.getByRole('region', { name: 'Request identity and protocol' })
  expect(within(identity).getByText('UPSTREAM-GROUPED')).toBeVisible()
  expect(within(identity).getByText('saved-group')).toBeVisible()
  expect(within(identity).getByText('192.0.2.1')).toBeVisible()
  expect(within(identity).getByText('/v1/responses')).toBeVisible()
  expect(within(identity).getByText('OpenAI -> Anthropic')).toBeVisible()
  expect(within(identity).getByText('Response Time')).toBeVisible()
  expect(within(identity).getByText('medium')).toBeVisible()
  expect(within(identity).getByText('Overwritten')).toBeVisible()
  const routing = screen.getByRole('region', { name: 'Routing and dispatch evidence' })
  expect(within(routing).getByText('administrator-target')).toBeVisible()
  expect(within(routing).getByText('saved-route-digest')).toBeVisible()
  expect(within(routing).getByText('mapped-target')).toBeVisible()
  expect(within(routing).getByText('saved-terminal-reason')).toBeVisible()
  expect(within(routing).getByText('saved-terminal-error')).toBeVisible()
  expect(within(routing).getByText('saved-soft-error')).toBeVisible()
  const budget = within(routing).getByRole('region', { name: 'Reservation and actual usage' })
  expect(within(budget).getAllByText('0')).toHaveLength(3)
  const usage = screen.getByRole('region', { name: 'Usage and settlement evidence' })
  expect(within(usage).getByText(publication)).toBeVisible()
  expect(within(usage).getByText(sourceDigest)).toBeVisible()
  expect(within(usage).getByText('Billing Details')).toBeVisible()
  expect(within(usage).getByText('Audio Input')).toBeVisible()
  expect(within(usage).getAllByText('0')).toHaveLength(2)
  for (const duplicate of ['REQ-GROUPED', 'identity-key', 'HTTP status', 'Request succeeded']) {
    expect(screen.queryByText(duplicate)).not.toBeInTheDocument()
  }
  expect(screen.queryByText(/identity-connection/)).not.toBeInTheDocument()
})

test('low-frequency saved content and parameter overrides remain reachable in one collapsed section', async () => {
  const user = userEvent.setup()
  renderEvidence(record(richEvidence))
  const additional = screen.getByRole('region', { name: 'Additional saved evidence' })
  const summary = within(additional).getByText('Additional saved evidence', { selector: 'summary' })
  expect(summary).toHaveClass('min-h-11')
  expect(summary.closest('details')).not.toHaveAttribute('open')
  expect(within(additional).getByText('preserved_parameter')).not.toBeVisible()
  expect(within(additional).getByText('saved request annotation')).not.toBeVisible()

  await user.click(summary)

  expect(summary.closest('details')).toHaveAttribute('open')
  expect(within(additional).getByText('preserved_parameter')).toBeVisible()
  expect(within(additional).getByText('saved request annotation')).toBeVisible()
})

test('a saved failure message stays visible without expanding additional evidence', () => {
  renderEvidence(record({ request_outcome: 'failed', http_status: 429 }, {
    type: 5, content: 'Upstream rate limited',
  }))

  const routing = screen.getByRole('region', { name: 'Routing and dispatch evidence' })
  expect(within(routing).getByText('Upstream rate limited')).toBeVisible()
  expect(screen.queryByRole('region', { name: 'Additional saved evidence' })).not.toBeInTheDocument()
})

test('administrator-only route, budget and conversion evidence stay inaccessible to an ordinary user', () => {
  useAuthStore.getState().auth.setUser({ id: 2, username: 'owner', role: 1 })
  renderEvidence(record(richEvidence), { isAdmin: false })

  for (const adminOnly of ['administrator-target', 'saved-route-digest', '/v1/messages', 'Request Conversion']) {
    expect(screen.queryByText(adminOnly)).not.toBeInTheDocument()
  }
  expect(screen.queryByRole('region', { name: 'Reservation and actual usage' })).not.toBeInTheDocument()
  expect(screen.getByText('UPSTREAM-GROUPED')).toBeVisible()
  expect(screen.getByText('Token Breakdown')).toBeVisible()
})

test('hidden monetary details keep their existing gate while explicit zero Tokens remain confirmed', () => {
  renderEvidence(record(richEvidence), { showCosts: false })

  expect(screen.queryByText('Billing Details')).not.toBeInTheDocument()
  expect(screen.queryByText('Price publication ID')).not.toBeInTheDocument()
  expect(screen.queryByText(publication)).not.toBeInTheDocument()
  expect(screen.queryByText(sourceDigest)).not.toBeInTheDocument()
  const usage = screen.getByRole('region', { name: 'Usage and settlement evidence' })
  expect(within(usage).getByText('Input Tokens')).toBeVisible()
  expect(within(usage).getByText('Output Tokens')).toBeVisible()
  expect(within(usage).getAllByText('0')).toHaveLength(2)
})

test('unknown upstream usage stays unknown rather than becoming explicit zero Tokens', () => {
  renderEvidence(record({ request_outcome: 'completed', usage_outcome: 'missing' }, {
    type: 5, content: '', quota: 0,
  }))
  const usage = screen.getByRole('region', { name: 'Usage and settlement evidence' })
  expect(within(usage).getAllByText('Unknown')).toHaveLength(2)
  expect(within(usage).queryByText('0')).not.toBeInTheDocument()
})

test.each([
  { type: 1, other: { admin_info: { payment_method: 'saved-payment-method', version: 'saved-system-version' } }, expected: 'saved-payment-method' },
  { type: 6, other: { task_id: 'saved-refund-task', reason: 'saved-refund-reason' }, expected: 'saved-refund-reason' },
  { type: 3, other: { admin_info: { admin_username: 'saved-admin', admin_id: 99 }, audit_info: { method: 'PATCH', route: '/api/synthetic', status: 200, success: true } }, expected: 'PATCH /api/synthetic' },
])('non-request Dialog type $type retains the original identity and audit/refund content', ({ type, other, expected }) => {
  renderEvidence(record(other, { type }), { dialog: true })

  expect(screen.getByText('REQ-GROUPED')).toBeVisible()
  expect(screen.getByText('identity-key')).toBeVisible()
  expect(screen.getByText(/identity-connection/)).toBeVisible()
  expect(screen.getByText(expected)).toBeVisible()
  expect(screen.queryByRole('region', { name: 'Request identity and protocol' })).not.toBeInTheDocument()
  expect(screen.queryByText('Additional saved evidence')).not.toBeInTheDocument()
})

test('opening request diagnosis, expanding evidence and editing reconciliation never submit without the existing explicit confirmation', async () => {
  const user = userEvent.setup()
  const review = {
    request_id: 'REQ-GROUPED', user_id: 2, token_id: 8,
    state: 'usage_unknown', reserved_quota: 100, actual_quota: null,
  }
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: review } })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { ...review, state: 'settled', actual_quota: 120 } },
  })
  renderEvidence(record({
    request_outcome: 'completed', usage_outcome: 'missing', settlement_status: 'pending_review',
  }, { type: 5 }))
  await screen.findByRole('region', { name: 'Usage pending review' })
  expect(post).not.toHaveBeenCalled()
  await user.click(screen.getByText('Additional saved evidence', { selector: 'summary' }))
  expect(post).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Advanced: manual reconciliation' }))
  await user.type(screen.getByLabelText('Confirmed quota (internal units)'), '120')
  await user.type(screen.getByLabelText('Evidence reference'), 'synthetic invoice reference')
  const submit = screen.getByRole('button', { name: 'Confirm reconciliation' })
  await user.click(submit)
  expect(post).not.toHaveBeenCalled()
  await user.click(screen.getByRole('checkbox', { name: 'I verified the evidence and frozen pricing.' }))
  await user.click(submit)

  expect(await screen.findByText('Reconciled')).toBeVisible()
  expect(post).toHaveBeenCalledExactlyOnceWith(
    '/api/usage-review/REQ-GROUPED/reconcile',
    { actual_quota: 120, evidence_reference: 'synthetic invoice reference', confirmed_reliable_evidence: true },
    { skipErrorHandler: true }
  )
})
