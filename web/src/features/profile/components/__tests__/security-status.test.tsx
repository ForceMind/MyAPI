import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

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
})

describe('Two-factor authentication status', () => {
  it.each(statusFailures)(
    'keeps security actions unavailable after a $name',
    async ({ request }) => {
      vi.spyOn(console, 'error').mockImplementation(() => undefined)
      vi.spyOn(api, 'get').mockImplementation(request)

      render(<TwoFACard loading={false} />)

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

      render(<TwoFACard loading={false} />)

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
