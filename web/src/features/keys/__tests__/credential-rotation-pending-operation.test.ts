/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { beforeEach, expect, test } from 'vitest'

import {
  clearPendingCredentialRotation,
  readPendingCredentialRotation,
  savePendingCredentialRotation,
} from '../credential-rotation-pending-operation'

beforeEach(() => window.sessionStorage.clear())

test('rotation operations are isolated by actor and token', () => {
  const first = 'a'.repeat(64)
  const second = 'b'.repeat(64)
  const third = 'c'.repeat(64)

  savePendingCredentialRotation(1, 21, first)
  savePendingCredentialRotation(1, 22, second)
  savePendingCredentialRotation(2, 21, third)

  expect(readPendingCredentialRotation(1, 21)).toBe(first)
  expect(readPendingCredentialRotation(1, 22)).toBe(second)
  expect(readPendingCredentialRotation(2, 21)).toBe(third)
})

test('a stale instance cannot overwrite or clear another rotation operation', () => {
  const current = 'd'.repeat(64)
  const stale = 'e'.repeat(64)
  savePendingCredentialRotation(1, 21, current)

  expect(() => savePendingCredentialRotation(1, 21, stale)).toThrow(
    'Another credential rotation is pending'
  )
  expect(() => clearPendingCredentialRotation(1, 21, stale)).toThrow(
    'Pending credential rotation changed'
  )
  expect(readPendingCredentialRotation(1, 21)).toBe(current)

  clearPendingCredentialRotation(1, 21, current)
  expect(readPendingCredentialRotation(1, 21)).toBeNull()
})
