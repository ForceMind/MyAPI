/*
Copyright (C) 2026 ForceMind

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
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

vi.mock('../../../api', () => ({
  fetchModels: vi.fn(),
  getAllModels: vi.fn(),
  getChannel: vi.fn(),
  getChannelKey: vi.fn(),
  getGroups: vi.fn(),
  getPrefillGroups: vi.fn(),
  refreshCodexCredential: vi.fn(),
  createChannel: vi.fn(),
  updateChannel: vi.fn(),
}))
vi.mock('../../../hooks/use-channel-upstream-updates', () => ({
  useChannelUpstreamUpdates: () => ({
    showModal: false,
    addModels: [],
    removeModels: [],
    preferredTab: 'add',
    applyLoading: false,
    openModal: vi.fn(),
    closeModal: vi.fn(),
    applyUpdates: vi.fn(),
  }),
}))
vi.mock('@/features/system-settings/components/form-navigation-guard', () => ({
  FormNavigationGuard: () => null,
}))
vi.mock('@/features/auth/secure-verification', () => ({
  SecureVerificationDialog: () => null,
  useSecureVerification: () => ({
    open: false,
    methods: [],
    state: {},
    executeVerification: vi.fn(),
    withVerification: vi.fn(),
    cancel: vi.fn(),
    setCode: vi.fn(),
    switchMethod: vi.fn(),
  }),
}))

const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { getAllModels, getGroups, getPrefillGroups } =
  await import('../../../api')
const { ChannelsProvider } = await import('../../channels-provider')
const { ChannelMutateDrawer } = await import('../channel-mutate-drawer')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

function DrawerHarness({ viaMenu = false }: { viaMenu?: boolean }) {
  const [open, setOpen] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  return (
    <>
      <button
        type='button'
        onClick={() => (viaMenu ? setMenuOpen(true) : setOpen(true))}
      >
        Open channel drawer
      </button>
      {menuOpen && (
        <div role='menu'>
          <button
            type='button'
            role='menuitem'
            onClick={() => {
              setMenuOpen(false)
              setOpen(true)
            }}
          >
            Edit from menu
          </button>
        </div>
      )}
      <ChannelMutateDrawer open={open} onOpenChange={setOpen} />
    </>
  )
}

function mount(viaMenu = false): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <ChannelsProvider>
          <DrawerHarness viaMenu={viaMenu} />
        </ChannelsProvider>
      </I18nextProvider>
    </QueryClientProvider>
  )
}

describe('ChannelMutateDrawer dirty close guard', () => {
  beforeEach(() => {
    vi.mocked(getGroups).mockResolvedValue({ success: true, data: ['default'] })
    vi.mocked(getAllModels).mockResolvedValue({ success: true, data: [] })
    vi.mocked(getPrefillGroups).mockResolvedValue({ success: true, data: [] })
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  test('restores the menu trigger after its edit item unmounts', async () => {
    mount(true)
    const opener = screen.getByRole('button', { name: 'Open channel drawer' })
    fireEvent.pointerDown(opener)
    fireEvent.click(opener)
    const item = screen.getByRole('menuitem', { name: 'Edit from menu' })
    fireEvent.pointerDown(item)
    fireEvent.click(item)
    await screen.findByLabelText('Name *')
    expect(item).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    await waitFor(() => expect(opener).toHaveFocus())
  })

  test('keeps an edited draft when Escape is pressed, then restores the opener focus after Leave', async () => {
    mount()
    const opener = screen.getByRole('button', {
      name: 'Open channel drawer',
    })
    fireEvent.pointerDown(opener)
    fireEvent.click(opener)
    const name = await screen.findByLabelText('Name *')
    fireEvent.change(name, { target: { value: 'Draft channel' } })

    fireEvent.keyDown(name, { key: 'Escape' })
    expect(await screen.findByText('Unsaved changes')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Stay' }))
    expect(screen.getByLabelText('Name *')).toHaveValue('Draft channel')

    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Leave' }))

    await waitFor(() =>
      expect(
        screen.queryByRole('heading', { name: 'Create Channel' })
      ).toBeNull()
    )
    expect(opener).toHaveFocus()
  })
})
