import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { AccessPolicyDialog } from '../components/access-policy-dialog'
import { authenticate, envelope, queryWrapper } from './fixtures'

const originalAuth = useAuthStore.getState()
beforeEach(() => {
  authenticate(1)
  vi.spyOn(api, 'get').mockResolvedValue(envelope({ token_id: 7, assigned: true, user_revision: 1, token_revision: 2, models: [
    { model: 'synthetic-allowed/complete-long-public-model-id', allowed: true, reasons: [] },
    { model: 'synthetic-allowed-second', allowed: true, reasons: [] },
    { model: 'synthetic-denied', allowed: false, reasons: ['token_model_denied'] },
    { model: 'synthetic-unknown', allowed: false, reasons: [] },
  ] }))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected synthetic write'))
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected synthetic preview'))
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected synthetic delete'))
})
afterEach(() => { cleanup(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

it('keeps denied and unknown reasons visible while keyboard expansion reveals complete allowed names without additional requests', async () => {
  const wrapper = queryWrapper()
  render(<AccessPolicyDialog tokenId={7} onClose={vi.fn()} />, wrapper)
  const actions = userEvent.setup()
  const available = await screen.findByRole('button', { name: 'Available models (2)' })
  expect(available).toHaveAttribute('aria-expanded', 'false')
  expect(screen.getByText('synthetic-denied')).toBeVisible()
  expect(screen.getByText('The Key model limits deny this model.')).toBeVisible()
  expect(screen.getByText('synthetic-unknown')).toBeVisible()
  expect(screen.getByText('Unknown', { exact: true })).toBeVisible()
  expect(screen.getByText('Access is unavailable. Ask an administrator.')).toBeVisible()
  expect(screen.getByText('synthetic-allowed/complete-long-public-model-id')).not.toBeVisible()
  available.focus()
  await actions.keyboard('{Enter}')
  expect(screen.getByText('synthetic-allowed/complete-long-public-model-id')).toBeVisible()
  expect(screen.getByText('synthetic-allowed-second')).toBeVisible()
  await actions.click(available)
  expect(screen.getByText('synthetic-denied')).toBeVisible()
  expect(screen.getByText('Unknown', { exact: true })).toBeVisible()
  const requirements = screen.getByRole('button', { name: 'Request requirements' })
  expect(requirements).toHaveAttribute('aria-expanded', 'false')
  await actions.click(requirements)
  expect(screen.getByText('Assigned access requires an unambiguous JSON model and a final request body no larger than 1 MiB.')).toBeVisible()
  expect(api.get).toHaveBeenCalledTimes(1)
  expect(api.get).toHaveBeenCalledWith('/api/token/7/access', expect.objectContaining({ disableDuplicate: true, signal: expect.any(AbortSignal) }))
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
  expect(api.delete).not.toHaveBeenCalled()
})

it('keeps a real empty response explicit without an empty allowed-model disclosure or implied availability', async () => {
  vi.mocked(api.get).mockResolvedValue(envelope({ token_id: 7, assigned: true, user_revision: 1, token_revision: 2, models: [] }))
  const wrapper = queryWrapper()
  render(<AccessPolicyDialog tokenId={7} onClose={vi.fn()} />, wrapper)
  expect(await screen.findByText('No public models to display.')).toBeVisible()
  expect(screen.queryByRole('button', { name: /^Available models/ })).not.toBeInTheDocument()
  expect(screen.queryByText('Available', { exact: true })).not.toBeInTheDocument()
  expect(api.get).toHaveBeenCalledTimes(1)
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
})
