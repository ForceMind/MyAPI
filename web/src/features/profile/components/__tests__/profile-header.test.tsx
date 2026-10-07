import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import type { UserProfile } from '../../types'
import { ProfileHeader } from '../profile-header'

const profile: UserProfile = {
  id: 42,
  username: 'operator',
  display_name: 'Operations',
  role: 10,
  email: 'operator@example.test',
  group: 'default',
  quota: 5000000,
  used_quota: 1000000,
  request_count: 42,
  status: 1,
  aff_count: 0,
  aff_quota: 0,
  aff_history_quota: 0,
  created_time: 0,
}

describe('profile identity and evidence', () => {
  it('wraps complete identity details instead of hiding long names or email addresses', () => {
    const longName = 'OperatorWithAnExceptionallyLongUnbrokenDisplayName'
    const email = 'operator-with-a-long-account-identifier@example.test'
    render(
      <ProfileHeader
        profile={{ ...profile, display_name: longName, email }}
        loading={false}
      />
    )
    expect(
      screen.getByRole('heading', { name: longName, level: 2 })
    ).toHaveClass('wrap-anywhere')
    expect(screen.getByText(email)).toHaveClass('wrap-anywhere')
    expect(screen.getByText(`User ID ${profile.id}`)).toBeInTheDocument()
  })

  it('stacks labeled quota metrics on narrow screens and keeps their explanations visible', () => {
    render(<ProfileHeader profile={profile} loading={false} />)
    const balance = screen.getByText('Current Balance')
    const metrics = balance.closest('dl')
    expect(metrics).toHaveClass('grid-cols-1', 'sm:grid-cols-3')
    expect(balance).not.toHaveClass('truncate')
    expect(screen.getByText('Remaining quota')).not.toHaveClass('hidden')
    expect(screen.getAllByRole('term')).toHaveLength(3)
    expect(screen.getAllByRole('definition')).toHaveLength(6)
  })

  it('announces loading without turning unavailable profile evidence into zero balances', () => {
    const view = render(<ProfileHeader profile={null} loading />)
    expect(screen.getByRole('status')).toHaveTextContent('Loading')
    expect(screen.queryByText('Current Balance')).not.toBeInTheDocument()
    view.rerender(<ProfileHeader profile={null} loading={false} />)
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(screen.queryByText('Current Balance')).not.toBeInTheDocument()
  })
})
