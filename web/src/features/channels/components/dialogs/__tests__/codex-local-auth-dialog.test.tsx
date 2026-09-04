/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import {
  getCodexLocalAuthStatus,
  importCodexLocalAuthForChannel,
  importCodexLocalAuthForNewChannel,
} from '../../../api'
import { CodexLocalAuthDialog } from '../codex-local-auth-dialog'

const secureVerification = vi.hoisted(() => ({
  cancel: vi.fn(),
  executeVerification: vi.fn(),
  setCode: vi.fn(),
  switchMethod: vi.fn(),
  withVerification: vi.fn(),
}))

const toast = vi.hoisted(() => ({
  error: vi.fn(),
  success: vi.fn(),
}))

vi.mock('../../../api', () => ({
  getCodexLocalAuthStatus: vi.fn(),
  importCodexLocalAuthForChannel: vi.fn(),
  importCodexLocalAuthForNewChannel: vi.fn(),
}))

vi.mock('@/features/auth/secure-verification', () => ({
  SecureVerificationDialog: () => null,
  useSecureVerification: () => ({
    open: false,
    methods: { has2FA: false, hasPasskey: false, passkeySupported: false },
    state: { method: null, loading: false, code: '' },
    executeVerification: secureVerification.executeVerification,
    withVerification: secureVerification.withVerification,
    cancel: secureVerification.cancel,
    setCode: secureVerification.setCode,
    switchMethod: secureVerification.switchMethod,
  }),
}))

vi.mock('sonner', () => ({ toast }))

const readyStatus = {
  state: 'ready' as const,
  platform: 'darwin' as const,
  environment: 'native' as const,
  codex_installed: true,
  cli_version: '1.2.3',
  auth_file_exists: true,
  auth_readable: true,
  logged_in: true,
  auto_import_available: true,
  manual_import_available: true,
  account_hint: 'acc…1234',
  can_refresh: true,
}

function renderDialog(
  props?: Partial<React.ComponentProps<typeof CodexLocalAuthDialog>>
) {
  const defaults = {
    open: true,
    onOpenChange: vi.fn(),
    onImportSuccess: vi.fn(),
    onKeyImported: vi.fn(),
  }
  return {
    ...defaults,
    ...props,
    ...render(<CodexLocalAuthDialog {...defaults} {...props} />),
  }
}

describe('CodexLocalAuthDialog', () => {
  beforeEach(() => {
    vi.mocked(getCodexLocalAuthStatus).mockResolvedValue({
      success: true,
      data: readyStatus,
    })
    secureVerification.withVerification.mockImplementation(
      (callback: (proofToken?: string) => Promise<unknown>) => callback()
    )
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.clearAllMocks()
  })

  test('imports an existing ready local Codex account without exposing its credential', async () => {
    vi.mocked(importCodexLocalAuthForChannel).mockResolvedValue({
      success: true,
      data: { channel_id: 7 },
    })
    const view = renderDialog({ channelId: 7 })

    expect(
      await screen.findByText('Local Codex account detected')
    ).toBeInTheDocument()
    expect(screen.getByText('Connected account: acc…1234')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Import local Codex' }))

    await waitFor(() => {
      expect(importCodexLocalAuthForChannel).toHaveBeenCalledWith(7, undefined)
    })
    expect(view.onImportSuccess).toHaveBeenCalledWith({
      success: true,
      data: { channel_id: 7 },
    })
    expect(screen.queryByText('access_token')).not.toBeInTheDocument()
  })

  test('uses the validated new-channel payload when importing a new channel', async () => {
    const payload = {
      mode: 'single' as const,
      channel: { name: 'Local Codex', type: 57, key: '' },
    }
    vi.mocked(importCodexLocalAuthForNewChannel).mockResolvedValue({
      success: true,
      data: { channel_id: 8 },
    })
    const getCreatePayload = vi.fn().mockResolvedValue(payload)
    renderDialog({ getCreatePayload })

    await screen.findByText('Local Codex account detected')
    fireEvent.click(screen.getByRole('button', { name: 'Import local Codex' }))

    await waitFor(() => {
      expect(getCreatePayload).toHaveBeenCalledOnce()
      expect(importCodexLocalAuthForNewChannel).toHaveBeenCalledWith(
        payload,
        undefined
      )
    })
  })

  test('passes the security-proof scope and proof token through the automatic import', async () => {
    vi.mocked(importCodexLocalAuthForChannel).mockResolvedValue({
      success: true,
      data: { channel_id: 7 },
    })
    secureVerification.withVerification.mockImplementation(
      (callback: (proofToken?: string) => Promise<unknown>) =>
        callback('proof-token')
    )
    renderDialog({ channelId: 7 })

    await screen.findByText('Local Codex account detected')
    fireEvent.click(screen.getByRole('button', { name: 'Import local Codex' }))

    await waitFor(() => {
      expect(secureVerification.withVerification).toHaveBeenCalledWith(
        expect.any(Function),
        expect.objectContaining({
          scope: 'channel.codex.local_import',
          preferredMethod: 'passkey',
        })
      )
      expect(importCodexLocalAuthForChannel).toHaveBeenCalledWith(
        7,
        'proof-token'
      )
    })
  })

  test('keeps the dialog open and shows a safe error when automatic import fails', async () => {
    vi.mocked(importCodexLocalAuthForChannel).mockRejectedValue(
      new Error('network failure')
    )
    const view = renderDialog({ channelId: 7 })

    await screen.findByText('Local Codex account detected')
    fireEvent.click(screen.getByRole('button', { name: 'Import local Codex' }))

    expect(
      await screen.findAllByText('Failed to import local Codex credential')
    ).not.toHaveLength(0)
    expect(view.onOpenChange).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledWith(
      'Failed to import local Codex credential'
    )
  })

  test('keeps the dialog open and offers a safe retry message for import conflicts', async () => {
    vi.mocked(importCodexLocalAuthForChannel).mockRejectedValue({
      response: { data: { code: 'CODEX_LOCAL_AUTH_CONFLICT' } },
    })
    const view = renderDialog({ channelId: 7 })

    await screen.findByText('Local Codex account detected')
    fireEvent.click(screen.getByRole('button', { name: 'Import local Codex' }))

    expect(
      await screen.findByText('Save failed, please retry')
    ).toBeInTheDocument()
    expect(view.onOpenChange).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledWith('Save failed, please retry')
  })

  test('keeps a missing local login in the manual fallback path', async () => {
    vi.mocked(getCodexLocalAuthStatus).mockResolvedValue({
      success: true,
      data: {
        ...readyStatus,
        state: 'not_found',
        auth_file_exists: false,
        auth_readable: false,
        logged_in: false,
        auto_import_available: false,
      },
    })
    const view = renderDialog()

    expect(
      await screen.findByText(
        'No Codex login file was found for the MyAPI process user.'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Import local Codex' })
    ).toBeDisabled()

    fireEvent.click(screen.getByRole('button', { name: 'Manual import' }))
    expect(
      screen.getAllByRole('button', { name: 'Copy command' })
    ).toHaveLength(2)
    const credentialInput = screen.getByLabelText('Codex credential JSON')
    expect(credentialInput).toHaveFocus()
    fireEvent.change(credentialInput, { target: { value: '{not-json}' } })
    expect(
      screen.getByRole('button', { name: 'Fill channel credential' })
    ).toBeDisabled()

    const credential =
      '{"access_token":"access","refresh_token":"refresh","account_id":"account"}'
    fireEvent.change(credentialInput, { target: { value: credential } })
    fireEvent.click(
      screen.getByRole('button', { name: 'Fill channel credential' })
    )

    expect(view.onKeyImported).toHaveBeenCalledWith(credential)
    expect(view.onOpenChange).toHaveBeenCalledWith(false)
  })

  test('warns when a valid manual credential lacks a refresh token', async () => {
    renderDialog()

    await screen.findByText('Local Codex account detected')
    fireEvent.click(screen.getByRole('button', { name: 'Manual import' }))
    fireEvent.change(screen.getByLabelText('Codex credential JSON'), {
      target: { value: '{"access_token":"access","account_id":"account"}' },
    })

    expect(
      screen.getByText(
        'A refresh_token is recommended so MyAPI can maintain the channel.'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Fill channel credential' })
    ).toBeEnabled()
  })

  test('reports container access as unavailable instead of a missing login', async () => {
    vi.mocked(getCodexLocalAuthStatus).mockResolvedValue({
      success: true,
      data: {
        ...readyStatus,
        state: 'container_host_unavailable',
        environment: 'container',
        auth_file_exists: false,
        auth_readable: false,
        logged_in: false,
        auto_import_available: false,
      },
    })
    renderDialog()

    expect(
      await screen.findByText(
        'MyAPI is running in a container and cannot access the host Codex login.'
      )
    ).toBeInTheDocument()
    expect(
      screen.queryByText(
        'No Codex login file was found for the MyAPI process user.'
      )
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Manual import' })).toBeEnabled()
  })

  test('keeps explicit manual configuration available when LAN blocks local scanning', async () => {
    vi.mocked(getCodexLocalAuthStatus).mockRejectedValue({
      response: { data: { code: 'MYAPI_LAN_ROUTE_DISABLED' } },
    })
    renderDialog()

    expect(
      await screen.findByText('Local Codex import is unavailable in LAN Lite.')
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Import local Codex' })
    ).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Manual import' })).toBeEnabled()
  })

  test('clears manual credentials when switching modes or closing the dialog', async () => {
    const view = renderDialog()

    await screen.findByText('Local Codex account detected')
    fireEvent.click(screen.getByRole('button', { name: 'Manual import' }))
    fireEvent.change(screen.getByLabelText('Codex credential JSON'), {
      target: { value: '{"access_token":"access","account_id":"account"}' },
    })

    fireEvent.click(screen.getByRole('button', { name: 'Back' }))
    fireEvent.click(screen.getByRole('button', { name: 'Manual import' }))
    expect(screen.getByLabelText('Codex credential JSON')).toHaveValue('')

    fireEvent.change(screen.getByLabelText('Codex credential JSON'), {
      target: { value: '{"access_token":"access","account_id":"account"}' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(view.onOpenChange).toHaveBeenCalledWith(false)

    view.rerender(<CodexLocalAuthDialog {...view} open={false} />)
    view.rerender(<CodexLocalAuthDialog {...view} open />)
    await screen.findByText('Local Codex account detected')
    fireEvent.click(screen.getByRole('button', { name: 'Manual import' }))
    expect(screen.getByLabelText('Codex credential JSON')).toHaveValue('')
  })

  test('never persists a manually pasted credential in browser storage', async () => {
    const storageGetItem = vi.spyOn(Storage.prototype, 'getItem')
    const storageSetItem = vi.spyOn(Storage.prototype, 'setItem')
    renderDialog()

    await screen.findByText('Local Codex account detected')
    fireEvent.click(screen.getByRole('button', { name: 'Manual import' }))
    fireEvent.change(screen.getByLabelText('Codex credential JSON'), {
      target: {
        value:
          '{"access_token":"access-secret","refresh_token":"refresh-secret","account_id":"account"}',
      },
    })

    expect(storageGetItem).not.toHaveBeenCalled()
    expect(storageSetItem).not.toHaveBeenCalled()
  })
})
