/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { useAuthStore } from '@/stores/auth-store'

import type {
  CredentialMaintenance,
  CredentialMigrationPreview,
} from '../../credential-maintenance-api'
import { CredentialMaintenanceDialog } from '../credential-maintenance-dialog'

const apiMocks = vi.hoisted(() => ({
  getMaintenance: vi.fn(),
  preview: vi.fn(),
  commit: vi.fn(),
  receipt: vi.fn(),
}))

vi.mock('../../credential-maintenance-api', async (original) => {
  const actual =
    await original<typeof import('../../credential-maintenance-api')>()
  return {
    ...actual,
    getCredentialMaintenance: apiMocks.getMaintenance,
    previewCredentialMigration: apiMocks.preview,
    commitCredentialMigration: apiMocks.commit,
    getCredentialMigrationReceipt: apiMocks.receipt,
    isUncertainCredentialCommit: () => true,
  }
})

vi.mock('@/features/auth/secure-verification', async () => {
  const React = await import('react')
  return {
    useSecureVerification: () => {
      const [callback, setCallback] = React.useState<
        ((proof?: string) => Promise<unknown>) | null
      >(null)
      return {
        open: Boolean(callback),
        methods: {
          has2FA: true,
          hasPasskey: false,
          passkeySupported: false,
        },
        state: {
          method: '2fa',
          loading: false,
          code: '123456',
        },
        startVerification: async (
          next: (proof?: string) => Promise<unknown>
        ) => {
          setCallback(() => next)
          return true
        },
        executeVerification: async () => callback?.('synthetic-proof'),
        cancel: () => setCallback(null),
        setCode: vi.fn(),
        switchMethod: vi.fn(),
      }
    },
    SecureVerificationDialog: (props: {
      open: boolean
      onVerify: (method: '2fa', code: string) => Promise<void>
    }) =>
      props.open ? (
        <button
          type='button'
          onClick={() => void props.onVerify('2fa', '123456')}
        >
          Complete verification
        </button>
      ) : null,
  }
})

const originalAuth = useAuthStore.getState()
const digest = 'a'.repeat(64)
const maintenance: CredentialMaintenance = {
  token: {
    id: 21,
    name: 'Desktop Key',
    masked_key: 'sk-…abcd',
    owner_user_id: 1,
  },
  permissions: { can_migrate: true, reason: '' },
  rotation: {
    commit_supported: false,
    status: 'blocked',
    blockers: [{ code: 'rotation_commit_not_supported', state: 'blocked' }],
  },
  migration: {
    status: 'ready',
    source: {
      channel_id: 7,
      channel_name: 'Current account',
      subject_hint: 'abcd…wxyz',
      windows: [
        {
          pool_id: 11,
          window_slot: 'primary',
          revision: 3,
          share_bps: 2500,
          observed_at: 1,
          reset_at: 2,
        },
      ],
    },
    candidates: [
      {
        channel_id: 8,
        channel_name: 'Target account',
        subject_hint: 'efgh…stuv',
        credential_mode: 'single',
        status: 'ready',
        blockers: [],
        windows: [
          {
            pool_id: 12,
            window_slot: 'primary',
            revision: 2,
            share_bps: 0,
            available_bps: 7500,
            observed_at: 1,
            reset_at: 2,
          },
        ],
      },
    ],
    blockers: [],
  },
}
const preview: CredentialMigrationPreview = {
  status: 'ready',
  preview_revision: digest,
  expires_at: 123,
  blockers: [],
  changes: {
    policy: {
      changed: true,
      before_revision: 2,
      after_revision: 3,
      source_channel_id: 7,
      target_channel_id: 8,
    },
    allocations: [
      {
        source_pool_id: 11,
        target_pool_id: 12,
        window_slot: 'primary',
        share_bps: 2500,
        target_existing: false,
      },
    ],
  },
}

let client: QueryClient

function setActor(id: number, role: number, sid: string) {
  useAuthStore.setState((state) => ({
    ...state,
    auth: {
      ...state.auth,
      user: { id, role, username: `user-${id}` },
      session: {
        sid,
        current: true,
        login_method: 'password',
        ip: '127.0.0.1',
        user_agent: 'test',
        created_at: 1,
        last_active_at: 1,
        expires_at: 4_000_000_000,
      },
    },
  }))
}

function renderDialog() {
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <CredentialMaintenanceDialog tokenId={21} onClose={vi.fn()} />
      </TooltipProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  setActor(1, 100, 'session-a')
  apiMocks.getMaintenance.mockResolvedValue(maintenance)
  apiMocks.preview.mockResolvedValue(preview)
  apiMocks.commit.mockRejectedValue(new Error('Synthetic network loss'))
  apiMocks.receipt.mockRejectedValue(new Error('Synthetic missing receipt'))
})

afterEach(() => {
  vi.clearAllMocks()
  useAuthStore.setState(originalAuth)
})

test('a non-Root session never reads credential maintenance', async () => {
  setActor(1, 1, 'session-owner')
  renderDialog()

  expect(screen.getByText('Root access is required')).toBeInTheDocument()
  expect(apiMocks.getMaintenance).not.toHaveBeenCalled()
})

test('a Root session cannot render maintenance returned for another owner', async () => {
  apiMocks.getMaintenance.mockResolvedValue({
    ...maintenance,
    token: { ...maintenance.token, owner_user_id: 2 },
  })
  renderDialog()

  expect(
    await screen.findByText('Could not load credential maintenance')
  ).toBeInTheDocument()
  expect(screen.queryByText('Desktop Key')).not.toBeInTheDocument()
})

test('unexpected full subject and Key values are defensively redacted', async () => {
  const source = maintenance.migration.source
  if (!source) throw new Error('This fixture requires a source account')
  apiMocks.getMaintenance.mockResolvedValue({
    ...maintenance,
    token: {
      ...maintenance.token,
      masked_key: 'sk-this-must-never-render-in-full',
    },
    migration: {
      ...maintenance.migration,
      source: {
        ...source,
        subject_hint: 'acct-this-must-never-render-in-full',
      },
    },
  })
  renderDialog()

  await screen.findByText('Desktop Key')
  expect(
    screen.queryByText('sk-this-must-never-render-in-full')
  ).not.toBeInTheDocument()
  expect(
    screen.queryByText('acct-this-must-never-render-in-full')
  ).not.toBeInTheDocument()
})

test('changing identity remounts the dialog and removes prior owner data', async () => {
  apiMocks.getMaintenance
    .mockResolvedValueOnce(maintenance)
    .mockImplementationOnce(() => new Promise(() => {}))
  renderDialog()
  expect(await screen.findByText('Desktop Key')).toBeInTheDocument()

  act(() => setActor(2, 100, 'session-b'))

  await waitFor(() => {
    expect(screen.queryByText('Desktop Key')).not.toBeInTheDocument()
  })
  expect(apiMocks.getMaintenance).toHaveBeenCalledTimes(2)
})

test('an uncertain commit reads the receipt and never repeats the POST', async () => {
  const user = userEvent.setup()
  renderDialog()
  await screen.findByText('Desktop Key')

  await user.click(
    screen.getByRole('combobox', { name: 'Target Codex account' })
  )
  await user.click(screen.getByText('Target account · efgh…stuv'))
  await user.click(screen.getByRole('button', { name: 'Preview migration' }))
  await screen.findByText('Confirm migration')
  await user.click(
    screen.getByLabelText(
      'I understand this keeps the Key identity, secret, and request history.'
    )
  )
  await user.click(
    screen.getByLabelText(
      'I understand usage estimates remain approximate and the old pools are not closed.'
    )
  )
  await user.click(
    screen.getByRole('button', { name: 'Verify and move account' })
  )
  await user.click(
    screen.getByRole('button', { name: 'Complete verification' })
  )

  expect(
    await screen.findByText('Migration result is unknown')
  ).toBeInTheDocument()
  expect(apiMocks.commit).toHaveBeenCalledTimes(1)
  expect(apiMocks.receipt).toHaveBeenCalledTimes(1)
  const command = apiMocks.commit.mock.calls[0]?.[1] as {
    operation_id: string
  }
  expect(command).toMatchObject({
    target_channel_id: 8,
    target_shares: [
      { source_pool_id: 11, target_pool_id: 12, share_bps: 2500 },
    ],
    preview_revision: digest,
    confirmed: true,
  })
  expect(apiMocks.receipt).toHaveBeenCalledWith(
    21,
    command.operation_id,
    expect.any(AbortSignal)
  )

  await user.click(screen.getByRole('button', { name: 'Check receipt again' }))
  await waitFor(() => expect(apiMocks.receipt).toHaveBeenCalledTimes(2))
  expect(apiMocks.commit).toHaveBeenCalledTimes(1)
})
