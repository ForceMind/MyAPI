import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { EmptyState } from '../empty-state'
import { ErrorState } from '../error-state'
import { LoadingState } from '../loading-state'

describe('task feedback', () => {
  it('announces pending data without presenting an empty result', () => {
    render(<LoadingState message='Loading account evidence' />)
    expect(screen.getByRole('status')).toHaveTextContent(
      'Loading account evidence'
    )
    expect(screen.getByRole('status')).toHaveAttribute('aria-busy', 'true')
    expect(screen.queryByText('No Data')).not.toBeInTheDocument()
  })

  it('announces an inline operation even when no custom message is supplied', () => {
    render(<LoadingState inline />)
    expect(screen.getByRole('status')).toHaveTextContent('Loading...')
  })

  it('keeps a failed operation distinct and exposes a keyboard retry', async () => {
    const retry = vi.fn()
    const user = userEvent.setup()
    render(<ErrorState title='Account evidence unavailable' onRetry={retry} />)
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Account evidence unavailable'
    )
    await user.tab()
    await user.keyboard('{Enter}')
    expect(retry).toHaveBeenCalledOnce()
  })

  it('announces a confirmed empty result and retains its next action', () => {
    render(
      <EmptyState
        title='No matching requests'
        action={<button type='button'>Clear filters</button>}
      />
    )
    expect(screen.getByRole('status')).toHaveTextContent('No matching requests')
    expect(screen.getByRole('button', { name: 'Clear filters' })).toBeEnabled()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
