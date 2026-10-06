/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import viLocale from '@/i18n/locales/vi.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zh from '@/i18n/locales/zh.json'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getChannelQuotaEvents } from '../../api'
import type { ChannelQuotaAlertDeliveryEventsResponse } from '../../types'
import { ChannelQuotaEvents } from '../channel-quota-events'

vi.mock('../../api', () => ({ getChannelQuotaEvents: vi.fn() }))
const events: ChannelQuotaAlertDeliveryEventsResponse = {
  success: true,
  data: {
    total: 1,
    page: 1,
    page_size: 10,
    items: [
      {
        id: 1,
        event_key: 'synthetic-event',
        channel_id: 17,
        snapshot_id: 2,
        series_ref: 'accountwindowseries',
        observed_at: 1000,
        created_at: 1000,
        updated_at: 1000,
        status: 'exhausted',
        kind: 'threshold',
        state: 'pending',
        attempt_count: 0,
        evidence: {
          version: 1,
          snapshot_id: 2,
          channel_id: 17,
          observed_at: 1000,
          available: 0,
          total: 100,
          unit: 'percent',
          metric_type: 'codex_rate_limit',
          window_type: 'five_hour',
          window_seconds: 18000,
          reset_at: 18000,
        },
      },
    ],
  },
}
function renderEvents() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ChannelQuotaEvents />
    </QueryClientProvider>
  )
}
describe('in-app quota events', () => {
  beforeEach(() => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
    vi.mocked(getChannelQuotaEvents).mockResolvedValue(events)
  })
  afterEach(() => useAuthStore.getState().auth.reset())
  test('loads only after opening and displays zero, account window and separate delivery state', async () => {
    renderEvents()
    expect(getChannelQuotaEvents).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Show events' }))
    expect(
      await screen.findByText('Observed remaining: 0 / 100 percent')
    ).toBeInTheDocument()
    expect(screen.getAllByText('Quota exhausted')).toHaveLength(2)
    expect(
      screen.getByText('Channel 17 · Account window accountwindo')
    ).toBeInTheDocument()
    expect(screen.getByText('External delivery pending')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Hide events' }))
    expect(
      screen.queryByText('Observed remaining: 0 / 100 percent')
    ).not.toBeInTheDocument()
  })
  test('failed refresh hides previously loaded account data and permits retry', async () => {
    renderEvents()
    fireEvent.click(screen.getByRole('button', { name: 'Show events' }))
    await screen.findByText('Observed remaining: 0 / 100 percent')
    vi.mocked(getChannelQuotaEvents).mockRejectedValueOnce(
      new Error('unavailable')
    )
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Unable to load quota events'
    )
    expect(
      screen.queryByText('Observed remaining: 0 / 100 percent')
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(
      await screen.findByText('Observed remaining: 0 / 100 percent')
    ).toBeInTheDocument()
  })
  test('role loss and account switching never retain the previous account rows', async () => {
    renderEvents()
    fireEvent.click(screen.getByRole('button', { name: 'Show events' }))
    await screen.findByText('Observed remaining: 0 / 100 percent')
    act(() =>
      useAuthStore
        .getState()
        .auth.setUser({ id: 2, username: 'ordinary', role: ROLE.USER })
    )
    expect(screen.queryByText('Quota events')).not.toBeInTheDocument()
    vi.mocked(getChannelQuotaEvents).mockResolvedValue({
      success: true,
      data: { items: [], total: 0, page: 1, page_size: 10 },
    })
    act(() =>
      useAuthStore
        .getState()
        .auth.setUser({ id: 3, username: 'other-root', role: ROLE.SUPER_ADMIN })
    )
    expect(
      screen.queryByText('Observed remaining: 0 / 100 percent')
    ).not.toBeInTheDocument()
    expect(
      await screen.findByText('No recorded quota events')
    ).toBeInTheDocument()
  })
  test('status filtering requests only the selected state and missing evidence stays unknown', async () => {
    vi.mocked(getChannelQuotaEvents).mockResolvedValue({
      ...events,
      data: {
        total: 1,
        page: 1,
        page_size: 10,
        items: (events.data?.items ?? []).map((event) => ({
          ...event,
          evidence: null,
        })),
      },
    })
    renderEvents()
    fireEvent.click(screen.getByRole('button', { name: 'Show events' }))
    expect(
      await screen.findByText('Original observation unavailable')
    ).toBeInTheDocument()
    expect(
      screen.queryByText('Observed remaining:', { exact: false })
    ).not.toBeInTheDocument()
    fireEvent.change(
      screen.getByRole('combobox', { name: 'Quota event status' }),
      { target: { value: 'exhausted' } }
    )
    await waitFor(() =>
      expect(getChannelQuotaEvents).toHaveBeenLastCalledWith(
        { p: 1, page_size: 10, status: 'exhausted' },
        expect.any(AbortSignal)
      )
    )
  })
})

const locales = { en, zh, 'zh-TW': zhTW, fr, ru, ja, vi: viLocale }
test.each(Object.entries(locales))(
  'renders actual %s quota event labels and numbers',
  async (language, resource) => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
    vi.mocked(getChannelQuotaEvents).mockResolvedValue(events)
    const i18n = createInstance()
    await i18n.init({
      lng: language,
      fallbackLng: false,
      resources: { [language]: resource },
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={client}>
          <ChannelQuotaEvents />
        </QueryClientProvider>
      </I18nextProvider>
    )
    fireEvent.click(
      screen.getByRole('button', { name: resource.translation['Show events'] })
    )
    expect(
      await screen.findByText(
        i18n.t('Observed remaining: {{remaining}} / {{total}} {{unit}}', {
          remaining: 0,
          total: 100,
          unit: 'percent',
        })
      )
    ).toBeInTheDocument()
    expect(
      screen.getByText(resource.translation['External delivery pending'])
    ).toBeInTheDocument()
    expect(resource.translation['Quota exhausted']).toBeTruthy()
    useAuthStore.getState().auth.reset()
  }
)
