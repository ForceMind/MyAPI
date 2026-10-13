import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { updateUserSettings } from '../../api'
import type { UserProfile } from '../../types'
import { NotificationTab } from '../tabs/notification-tab'

vi.mock('../../api', () => ({
  updateUserSettings: vi.fn(),
}))

const profile: UserProfile = {
  id: 42,
  username: 'operator',
  display_name: 'Operations',
  role: 1,
  group: 'default',
  quota: 5000000,
  used_quota: 1000000,
  request_count: 42,
  status: 1,
  aff_count: 0,
  aff_quota: 0,
  aff_history_quota: 0,
  created_time: 0,
  setting: JSON.stringify({
    notify_type: 'email',
    quota_warning_threshold: 50,
    notification_email: 'alerts@example.test',
    accept_unset_model_ratio_model: true,
    record_ip_log: true,
    language: 'ja',
    sidebar_modules: '{"dashboard":{"visible":false}}',
    billing_preference: 'subscription_first',
  }),
}

beforeEach(() => {
  vi.mocked(updateUserSettings).mockResolvedValue({ success: true })
})

afterEach(() => {
  vi.resetAllMocks()
})

test('saving notification settings submits owned fields and refreshes the profile', async () => {
  const onUpdate = vi.fn()
  const user = userEvent.setup()
  render(<NotificationTab profile={profile} onUpdate={onUpdate} />)

  await user.click(screen.getByRole('button', { name: 'Save Settings' }))

  await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1))
  expect(updateUserSettings).toHaveBeenCalledWith({
    notify_type: 'email',
    quota_warning_threshold: 50,
    notification_email: 'alerts@example.test',
    webhook_url: '',
    webhook_secret: '',
    bark_url: '',
    gotify_url: '',
    gotify_token: '',
    gotify_priority: 5,
    accept_unset_model_ratio_model: true,
    record_ip_log: true,
    upstream_model_update_notify_enabled: false,
  })
  await waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1))
})
