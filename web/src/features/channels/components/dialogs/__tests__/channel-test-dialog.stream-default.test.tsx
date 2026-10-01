import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { Channel } from '@/features/channels/types'

import { useChannels } from '../../channels-provider'
import { ChannelTestDialog } from '../channel-test-dialog'

vi.mock('../../channels-provider', () => ({ useChannels: vi.fn() }))

afterEach(() => vi.clearAllMocks())

function renderChannelTestDialog(channelType: number) {
  vi.mocked(useChannels).mockReturnValue({
    currentRow: {
      id: channelType,
      type: channelType,
      name: channelType === 57 ? 'Codex test' : 'OpenAI test',
      models: channelType === 57 ? 'gpt-5-codex' : 'gpt-4o-mini',
    } as Channel,
  } as ReturnType<typeof useChannels>)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ChannelTestDialog open onOpenChange={vi.fn()} />
    </QueryClientProvider>
  )
}

test('Codex detailed connection test starts with streaming enabled', () => {
  renderChannelTestDialog(57)
  expect(screen.getByRole('switch', { name: 'Stream Mode' })).toHaveAttribute(
    'aria-checked',
    'true'
  )
})

test('non-Codex detailed connection test keeps streaming optional', () => {
  renderChannelTestDialog(1)
  expect(screen.getByRole('switch', { name: 'Stream Mode' })).toHaveAttribute(
    'aria-checked',
    'false'
  )
})
