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
import { afterEach, describe, expect, test } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { STORAGE_KEYS } from '../../../constants'
import { createUserMessage } from '../../message/message-utils'
import { loadConfig, loadMessages, saveConfig, saveMessages } from '../storage'

afterEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.reset()
})

function signIn(id: number) {
  useAuthStore.getState().auth.setUser({ id, username: `user-${id}`, role: 1 })
}

describe('account-scoped playground storage', () => {
  test('account B does not load account A chat text or selected API key', () => {
    signIn(101)
    saveMessages([createUserMessage('private A text')])
    saveConfig({ keyId: 11, model: 'A-model' })
    signIn(202)
    expect(loadMessages()).toBeNull()
    expect(loadConfig()).toEqual({})
  })

  test('returning to the same account restores its own history and configuration', () => {
    signIn(101)
    saveMessages([createUserMessage('private A text')])
    saveConfig({ keyId: 11 })
    signIn(202)
    saveMessages([createUserMessage('private B text')])
    saveConfig({ keyId: 22 })
    signIn(101)
    expect(loadMessages()?.[0].versions[0].content).toBe('private A text')
    expect(loadConfig().keyId).toBe(11)
  })

  test('ownerless legacy history is left untouched and is never automatically claimed', () => {
    const legacy = JSON.stringify([
      {
        key: 'legacy',
        from: 'user',
        versions: [{ id: 'v', content: 'unknown owner' }],
      },
    ])
    localStorage.setItem(STORAGE_KEYS.MESSAGES, legacy)
    localStorage.setItem(STORAGE_KEYS.CONFIG, JSON.stringify({ keyId: 33 }))
    signIn(101)
    expect(loadMessages()).toBeNull()
    expect(loadConfig()).toEqual({})
    expect(localStorage.getItem(STORAGE_KEYS.MESSAGES)).toBe(legacy)
  })

  test('signed-out reads do not expose a former account history', () => {
    signIn(101)
    saveMessages([createUserMessage('private A text')])
    saveConfig({ keyId: 11 })
    useAuthStore.getState().auth.reset()
    expect(loadMessages()).toBeNull()
    expect(loadConfig()).toEqual({})
  })
})
