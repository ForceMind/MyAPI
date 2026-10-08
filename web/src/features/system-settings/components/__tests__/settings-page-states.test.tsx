import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import type { SystemOption } from '../../types'
import { SettingsPage } from '../settings-page'

async function renderSettings(
  resolveSettings?: (
    settings: { ExampleSetting: string },
    raw: SystemOption[] | undefined
  ) => { ExampleSetting: string }
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const root = createRootRoute()
  const route = createRoute({
    getParentRoute: () => root,
    path: '/settings/$section',
    component: () => (
      <SettingsPage
        routePath='/settings/$section'
        defaultSettings={{ ExampleSetting: 'default' }}
        defaultSection='example'
        resolveSettings={resolveSettings}
        getSectionMeta={() => ({ titleKey: 'System Settings' })}
        getSectionContent={(_section, settings) => (
          <button type='button'>Save {settings.ExampleSetting}</button>
        )}
      />
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: ['/settings/example'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return client
}

describe('settings task states', () => {
  it.each([false, true])(
    'does not parse a malformed options payload when success is %s',
    async (success) => {
      vi.spyOn(api, 'get').mockResolvedValue({ data: { success, data: {} } })
      const resolve = vi.fn(
        (
          _settings: { ExampleSetting: string },
          raw: SystemOption[] | undefined
        ) => ({
          ExampleSetting:
            raw?.map((option) => option.value).join(',') ?? 'default',
        })
      )
      await renderSettings(resolve)
      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Unable to load settings'
      )
      expect(resolve).not.toHaveBeenCalled()
      expect(
        screen.queryByRole('button', { name: /Save/ })
      ).not.toBeInTheDocument()
    }
  )

  it('does not show editable defaults after an unsuccessful server response', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: false, message: 'unavailable' },
    })
    await renderSettings()
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Unable to load settings'
    )
    expect(
      screen.queryByRole('button', { name: /Save/ })
    ).not.toBeInTheDocument()
  })

  it('retries a failed request and only then exposes the saved values', async () => {
    const user = userEvent.setup()
    vi.spyOn(api, 'get')
      .mockRejectedValueOnce(new Error('network unavailable'))
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: [{ key: 'ExampleSetting', value: 'saved-value' }],
        },
      })
    await renderSettings()
    await screen.findByRole('alert')
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(
      await screen.findByRole('button', { name: 'Save saved-value' })
    ).toBeEnabled()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('removes stale writable settings when refresh fails', async () => {
    const request = vi.spyOn(api, 'get').mockResolvedValueOnce({
      data: {
        success: true,
        data: [{ key: 'ExampleSetting', value: 'saved-value' }],
      },
    })
    const client = await renderSettings()
    await screen.findByRole('button', { name: 'Save saved-value' })
    request.mockRejectedValueOnce(new Error('refresh failed'))
    await client.invalidateQueries({ queryKey: ['system-options'] })
    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument())
    expect(
      screen.queryByRole('button', { name: /Save/ })
    ).not.toBeInTheDocument()
  })
})
