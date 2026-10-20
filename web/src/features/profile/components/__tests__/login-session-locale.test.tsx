/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { act, render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { afterEach, expect, test, vi } from 'vitest'

import frenchLocale from '@/i18n/locales/fr.json'
import type { LoginSession } from '@/stores/auth-store'

import { LoginSessionItem } from '../login-session-item'

afterEach(async () => {
  await i18next.changeLanguage('en')
  i18next.removeResourceBundle('fr', 'translation')
  vi.useRealTimers()
})

test('session recency follows the current language and returns to English without changing its timestamps', async () => {
  i18next.addResourceBundle('fr', 'translation', frenchLocale.translation)
  await i18next.changeLanguage('en')
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-10-07T06:00:00Z'))
  const now = Math.floor(Date.now() / 1000)
  const session: LoginSession = {
    sid: 'synthetic-session',
    current: true,
    login_method: 'password',
    ip: '127.0.0.1',
    user_agent: 'synthetic-browser',
    created_at: now - 600,
    last_active_at: now - 30,
    expires_at: now + 3600,
  }
  render(<LoginSessionItem session={session} onRevoke={vi.fn()} />)
  expect(screen.getByText(/a few seconds ago/)).toBeInTheDocument()
  await act(async () => {
    await i18next.changeLanguage('fr')
  })
  expect(screen.getByText(/il y a quelques secondes/)).toBeInTheDocument()
  await act(async () => {
    await i18next.changeLanguage('en')
  })
  expect(screen.getByText(/a few seconds ago/)).toBeInTheDocument()
})

test.each([
  {
    language: 'zhCN',
    resource: () => import('@/i18n/locales/zh.json'),
    relative: '几秒前',
  },
  {
    language: 'zhTW',
    resource: () => import('@/i18n/locales/zh-TW.json'),
    relative: '幾秒前',
  },
  {
    language: 'ja',
    resource: () => import('@/i18n/locales/ja.json'),
    relative: '数秒前',
  },
  {
    language: 'ru',
    resource: () => import('@/i18n/locales/ru.json'),
    relative: 'несколько секунд назад',
  },
  {
    language: 'vi',
    resource: () => import('@/i18n/locales/vi.json'),
    relative: 'vài giây trước',
  },
])(
  'session recency supports the $language interface alias',
  async (fixture) => {
    const resource = await fixture.resource()
    i18next.addResourceBundle(
      fixture.language,
      'translation',
      resource.default.translation
    )
    await i18next.changeLanguage(fixture.language)
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-07T06:00:00Z'))
    const now = Math.floor(Date.now() / 1000)
    render(
      <LoginSessionItem
        session={{
          sid: 'synthetic-session',
          current: true,
          login_method: 'password',
          ip: '127.0.0.1',
          user_agent: 'synthetic-browser',
          created_at: now - 600,
          last_active_at: now - 30,
          expires_at: now + 3600,
        }}
        onRevoke={vi.fn()}
      />
    )
    expect(screen.getByText(new RegExp(fixture.relative))).toBeInTheDocument()
  }
)
