import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { ChannelSelectorDialog } from '../channel-selector-dialog'
import { ConflictConfirmDialog } from '../conflict-confirm-dialog'

afterEach(() => { cleanup(); vi.restoreAllMocks() })
it('channel selection keeps the original selected IDs and complete source URL with pagination outside the results region', async () => {
  const url = 'https://synthetic.invalid/upstream-with-a-complete-long-source-path-and-final-suffix'
  const confirm = vi.fn()
  const user = userEvent.setup()
  render(<ChannelSelectorDialog open onOpenChange={() => {}} channels={[{ id: 7, name: 'Synthetic long source name', base_url: url, status: 1 }]} selectedChannelIds={[7]} onSelectedChannelIdsChange={() => {}} channelEndpoints={{ 7: '/api/ratio_config' }} onChannelEndpointsChange={() => {}} onConfirm={confirm} />)
  const body = screen.getByRole('region', { name: 'Ratio sync channels' })
  expect(screen.getByText(url)).not.toHaveClass('truncate')
  expect(body).toHaveClass('min-h-0', 'overflow-y-auto')
  expect(body).not.toContainElement(screen.getByRole('button', { name: 'Confirm Selection' }))
  await user.click(screen.getByRole('button', { name: 'Confirm Selection' }))
  expect(confirm).toHaveBeenCalledWith([7])
})
it('billing conflicts keep complete before and after values within one scroll body and confirm only on explicit action', async () => {
  const confirm = vi.fn()
  const user = userEvent.setup()
  render(<ConflictConfirmDialog open onOpenChange={() => {}} conflicts={[{ channel: 'Synthetic complete channel name', model: 'synthetic/model-with-a-long-complete-id', current: 'ratio: 0', newVal: 'fixed: 12.3400000001' }]} onConfirm={confirm} />)
  const body = screen.getByRole('region', { name: 'Billing conflict details' })
  expect(body).toHaveClass('min-h-0', 'overflow-y-auto')
  expect(screen.getByText('ratio: 0')).toBeVisible()
  expect(screen.getByText('fixed: 12.3400000001')).toBeVisible()
  expect(body).not.toContainElement(screen.getByRole('button', { name: 'Confirm Changes' }))
  expect(confirm).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Confirm Changes' }))
  expect(confirm).toHaveBeenCalledTimes(1)
})
