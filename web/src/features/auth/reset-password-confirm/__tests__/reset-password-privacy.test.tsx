import { QueryClient } from '@tanstack/react-query'
import { cleanup, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { toast } from 'sonner'
import { afterEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'

import { ResetPasswordConfirm } from '..'

let client: QueryClient | undefined
afterEach(() => { cleanup(); client?.clear(); toast.dismiss(); vi.restoreAllMocks() })

it('reset and manual copy retain the generated one-time password but never put it in toast notifications', async () => {
  const password = 'SYNTHETIC_ONLY_RESET_PASSWORD'
  const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true, data: password } })
  const notifications = vi.spyOn(toast, 'success')
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  const user = userEvent.setup()
  const clipboard = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  await renderListRoute({ client, path: '/reset', element: <ResetPasswordConfirm email='synthetic@example.invalid' token='synthetic-reset-link-token' /> })
  await user.click(screen.getByRole('button', { name: i18next.t('auth.resetPasswordConfirm.confirm') }))
  expect(await screen.findByRole('textbox', { name: i18next.t('New password') })).toHaveValue(password)
  await waitFor(() => expect(clipboard).toHaveBeenCalledWith(password))
  await waitFor(() => expect(notifications).toHaveBeenCalled())
  expect(notifications.mock.calls.map(([message]) => String(message)).join(' ')).not.toContain(password)
  await user.click(screen.getByRole('button', { name: i18next.t('Copy password') }))
  expect(clipboard).toHaveBeenCalledTimes(2)
  expect(notifications.mock.calls.map(([message]) => String(message)).join(' ')).not.toContain(password)
  expect(post).toHaveBeenCalledTimes(1)
  expect(post.mock.calls[0]?.[0]).toBe('/api/user/reset')
  expect(post.mock.calls[0]?.[1]).toEqual({ email: 'synthetic@example.invalid', token: 'synthetic-reset-link-token' })
  expect(post.mock.calls[0]?.[2]).toMatchObject({ skipBusinessError: true, skipAuthRefresh: true })
})
