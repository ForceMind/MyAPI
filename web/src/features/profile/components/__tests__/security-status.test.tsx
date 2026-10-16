/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { act, render, renderHook, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { useProfilePasskey } from '../../hooks/use-profile-passkey'
import { useTwoFA } from '../../hooks/use-two-fa'
import { PasskeyCard } from '../passkey-card'
import { TwoFACard } from '../two-fa-card'

vi.mock('@/features/auth/secure-verification', () => ({
  SecureVerificationDialog: () => null,
  useSecureVerification: () => ({
    open: false,
    setOpen: vi.fn(),
    methods: {
      has2FA: false,
      hasPasskey: false,
      passkeySupported: false,
    },
    state: {
      method: null,
      loading: false,
      code: '',
    },
    startVerification: vi.fn(),
    executeVerification: vi.fn(),
    cancel: vi.fn(),
    setCode: vi.fn(),
    switchMethod: vi.fn(),
    fetchVerificationMethods: vi.fn(),
  }),
}))

type StatusFailure = {
  name: string
  request: () => Promise<unknown>
}

const statusFailures: StatusFailure[] = [
  {
    name: 'request rejection',
    request: () => Promise.reject(new Error('Synthetic status outage')),
  },
  {
    name: 'business failure',
    request: () =>
      Promise.resolve({
        data: { success: false, message: 'Synthetic business failure' },
      }),
  },
  {
    name: 'malformed success payload',
    request: () =>
      Promise.resolve({
        data: {
          success: true,
          data: { enabled: 'yes', locked: false, backup_codes_remaining: 8 },
        },
      }),
  },
]

afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('Two-factor authentication status', () => {
  it.each(statusFailures)(
    'keeps security actions unavailable after a $name',
    async ({ request }) => {
      vi.spyOn(console, 'error').mockImplementation(() => undefined)
      vi.spyOn(api, 'get').mockImplementation(request)

      render(<TwoFACard loading={false} securityScopeKey='user-1:session-1' />)

      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Unable to load settings'
      )
      expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
      expect(screen.queryByText('Disabled')).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Enable' })
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Disable 2FA' })
      ).not.toBeInTheDocument()
    }
  )

  it.each([
    { enabled: true, expectedStatus: 'Enabled', expectedAction: 'Disable 2FA' },
    { enabled: false, expectedStatus: 'Disabled', expectedAction: 'Enable' },
  ])(
    'shows the real $expectedStatus state after retry succeeds',
    async ({ enabled, expectedStatus, expectedAction }) => {
      const user = userEvent.setup()
      vi.spyOn(console, 'error').mockImplementation(() => undefined)
      vi.spyOn(api, 'get')
        .mockRejectedValueOnce(new Error('Synthetic status outage'))
        .mockResolvedValueOnce({
          data: {
            success: true,
            data: { enabled, locked: false, backup_codes_remaining: 8 },
          },
        })

      render(<TwoFACard loading={false} securityScopeKey='user-1:session-1' />)

      await user.click(await screen.findByRole('button', { name: 'Retry' }))

      expect(await screen.findByText(expectedStatus)).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: expectedAction })
      ).toBeInTheDocument()
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    }
  )
})

describe('Passkey status', () => {
  it.each(statusFailures)(
    'keeps security actions unavailable after a $name',
    async ({ request }) => {
      vi.spyOn(console, 'error').mockImplementation(() => undefined)
      vi.spyOn(api, 'get').mockImplementation(request)

      render(<PasskeyCard loading={false} />)

      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Failed to load Passkey status'
      )
      expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
      expect(screen.queryByText('Disabled')).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Enable Passkey' })
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Remove Passkey' })
      ).not.toBeInTheDocument()
    }
  )

  it.each([
    {
      enabled: true,
      expectedStatus: 'Enabled',
      expectedAction: 'Remove Passkey',
    },
    {
      enabled: false,
      expectedStatus: 'Disabled',
      expectedAction: 'Enable Passkey',
    },
  ])(
    'shows the real $expectedStatus state after retry succeeds',
    async ({ enabled, expectedStatus, expectedAction }) => {
      const user = userEvent.setup()
      vi.spyOn(console, 'error').mockImplementation(() => undefined)
      vi.spyOn(api, 'get')
        .mockRejectedValueOnce(new Error('Synthetic status outage'))
        .mockResolvedValueOnce({
          data: { success: true, data: { enabled, last_used_at: null } },
        })

      render(<PasskeyCard loading={false} />)

      await user.click(await screen.findByRole('button', { name: 'Retry' }))

      expect(await screen.findByText(expectedStatus)).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: expectedAction })
      ).toBeInTheDocument()
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    }
  )
})

describe('security status request lifecycle', () => {
  it('turns a stalled 2FA read into a recoverable failure after the deadline', async () => {
    vi.useFakeTimers()
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    vi.spyOn(api, 'get').mockReturnValue(new Promise<never>(() => undefined))

    const { result } = renderHook(() => useTwoFA())

    await act(() => vi.advanceTimersByTimeAsync(15000))

    expect(result.current.loading).toBe(false)
    expect(result.current.error).toBe(true)
    expect(result.current.status).toBeNull()
  })

  it('keeps the retried 2FA snapshot when the cancelled read resolves late', async () => {
    type StatusResponse = Awaited<ReturnType<typeof api.get>>
    let resolveOldRead!: (response: StatusResponse) => void
    const oldRead = new Promise<StatusResponse>((resolve) => {
      resolveOldRead = resolve
    })
    const enabledStatus = {
      enabled: true,
      locked: false,
      backup_codes_remaining: 8,
    }
    vi.spyOn(api, 'get')
      .mockReturnValueOnce(oldRead)
      .mockResolvedValueOnce({
        data: { success: true, data: enabledStatus },
      })

    const { result } = renderHook(() => useTwoFA())
    await act(() => result.current.refetch())

    await act(async () => {
      resolveOldRead({
        data: {
          success: true,
          data: {
            enabled: false,
            locked: false,
          },
        },
      } as StatusResponse)
      await oldRead
    })

    expect(result.current.status).toEqual(enabledStatus)
    expect(result.current.error).toBe(false)
  })

  it('aborts the 2FA read on unmount and ignores its late response', async () => {
    type StatusResponse = Awaited<ReturnType<typeof api.get>>
    let resolveRead!: (response: StatusResponse) => void
    const pendingRead = new Promise<StatusResponse>((resolve) => {
      resolveRead = resolve
    })
    const get = vi.spyOn(api, 'get').mockReturnValue(pendingRead)
    const { unmount } = renderHook(() => useTwoFA())

    unmount()

    expect(get.mock.calls[0]?.[1]?.signal?.aborted).toBe(true)
    await act(async () => {
      resolveRead({
        data: {
          success: true,
          data: { enabled: false, locked: false },
        },
      } as StatusResponse)
      await pendingRead
    })
  })

  it('turns a stalled Passkey read into a recoverable failure after the deadline', async () => {
    vi.useFakeTimers()
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    vi.spyOn(api, 'get').mockReturnValue(new Promise<never>(() => undefined))

    const { result } = renderHook(() => useProfilePasskey('user-1:session-1'))

    await act(() => vi.advanceTimersByTimeAsync(15000))

    expect(result.current.loading).toBe(false)
    expect(result.current.error).toBe(true)
    expect(result.current.status).toBeNull()
  })

  it('keeps the retried Passkey snapshot when the cancelled read resolves late', async () => {
    type StatusResponse = Awaited<ReturnType<typeof api.get>>
    let resolveOldRead!: (response: StatusResponse) => void
    const oldRead = new Promise<StatusResponse>((resolve) => {
      resolveOldRead = resolve
    })
    const enabledStatus = { enabled: true, last_used_at: null }
    vi.spyOn(api, 'get')
      .mockReturnValueOnce(oldRead)
      .mockResolvedValueOnce({
        data: { success: true, data: enabledStatus },
      })

    const { result } = renderHook(() => useProfilePasskey('user-1:session-1'))
    await act(() => result.current.fetchStatus())

    await act(async () => {
      resolveOldRead({
        data: { success: true, data: { enabled: false } },
      } as StatusResponse)
      await oldRead
    })

    expect(result.current.status).toEqual(enabledStatus)
    expect(result.current.error).toBe(false)
  })

  it('aborts the Passkey read on unmount and ignores its late response', async () => {
    type StatusResponse = Awaited<ReturnType<typeof api.get>>
    let resolveRead!: (response: StatusResponse) => void
    const pendingRead = new Promise<StatusResponse>((resolve) => {
      resolveRead = resolve
    })
    const get = vi.spyOn(api, 'get').mockReturnValue(pendingRead)
    const { unmount } = renderHook(() => useProfilePasskey('user-1:session-1'))

    unmount()

    expect(get.mock.calls[0]?.[1]?.signal?.aborted).toBe(true)
    await act(async () => {
      resolveRead({
        data: { success: true, data: { enabled: false } },
      } as StatusResponse)
      await pendingRead
    })
  })
})
