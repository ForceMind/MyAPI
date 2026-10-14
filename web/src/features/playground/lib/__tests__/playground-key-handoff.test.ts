/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { afterEach, describe, expect, test } from 'vitest'

import {
  completePlaygroundKeyHandoff,
  createPlaygroundKeyHandoff,
  readPlaygroundKeyHandoff,
} from '../playground-key-handoff'

afterEach(() => {
  const handoff = readPlaygroundKeyHandoff(7, 'session-a')
  if (handoff.status !== 'none') {
    completePlaygroundKeyHandoff(handoff.requestId)
  }
})

describe('playground Key handoff', () => {
  test('accepts the selected Key only for the same user and session', () => {
    createPlaygroundKeyHandoff({
      userId: 7,
      sessionId: 'session-a',
      keyId: 23,
      now: 1_000,
    })

    expect(readPlaygroundKeyHandoff(7, 'session-a', 2_000)).toMatchObject({
      status: 'accepted',
      keyId: 23,
    })
  })

  test.each<[string, number, string]>([
    ['another user', 8, 'session-a'],
    ['another session', 7, 'session-b'],
  ])(
    'rejects %s without exposing the selected Key',
    (_name, userId, sessionId) => {
      createPlaygroundKeyHandoff({
        userId: 7,
        sessionId: 'session-a',
        keyId: 23,
        now: 1_000,
      })

      expect(readPlaygroundKeyHandoff(userId, sessionId, 2_000)).toEqual({
        status: 'rejected',
        requestId: expect.any(Number),
      })
    }
  )

  test('rejects an expired handoff and removes it only after explicit completion', () => {
    createPlaygroundKeyHandoff({
      userId: 7,
      sessionId: 'session-a',
      keyId: 23,
      now: 1_000,
    })

    const expired = readPlaygroundKeyHandoff(7, 'session-a', 32_001)
    expect(expired).toEqual({
      status: 'rejected',
      requestId: expect.any(Number),
    })
    expect(readPlaygroundKeyHandoff(7, 'session-a', 32_001)).toEqual(expired)
    if (expired.status !== 'rejected') throw new Error('Expected rejection')

    completePlaygroundKeyHandoff(expired.requestId)
    expect(readPlaygroundKeyHandoff(7, 'session-a', 32_001)).toEqual({
      status: 'none',
    })
  })

  test('does not let an older mount complete a newer handoff', () => {
    createPlaygroundKeyHandoff({
      userId: 7,
      sessionId: 'session-a',
      keyId: 22,
      now: 1_000,
    })
    const older = readPlaygroundKeyHandoff(7, 'session-a', 2_000)
    if (older.status !== 'accepted') throw new Error('Expected acceptance')
    createPlaygroundKeyHandoff({
      userId: 7,
      sessionId: 'session-a',
      keyId: 23,
      now: 2_000,
    })

    completePlaygroundKeyHandoff(older.requestId)
    expect(readPlaygroundKeyHandoff(7, 'session-a', 2_001)).toMatchObject({
      status: 'accepted',
      keyId: 23,
    })
  })

  test('an invalid new selection cannot leave an older handoff pending', () => {
    createPlaygroundKeyHandoff({
      userId: 7,
      sessionId: 'session-a',
      keyId: 23,
      now: 1_000,
    })

    createPlaygroundKeyHandoff({
      userId: 7,
      sessionId: 'session-a',
      keyId: 0,
      now: 2_000,
    })

    expect(readPlaygroundKeyHandoff(7, 'session-a', 2_001)).toEqual({
      status: 'none',
    })
  })
})
