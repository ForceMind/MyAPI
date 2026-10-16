/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { beforeEach, expect, test } from 'vitest'

import {
  clearPendingCredentialMaintenanceOperation,
  readPendingCredentialMaintenanceOperation,
  savePendingCredentialMaintenanceOperation,
} from '../credential-maintenance-pending-operation'

beforeEach(() => {
  window.sessionStorage.clear()
})

test('pending operations are isolated by actor and token', () => {
  const actorOneTokenTwentyOne = 'a'.repeat(64)
  const actorOneTokenTwentyTwo = 'b'.repeat(64)
  const actorTwoTokenTwentyOne = 'c'.repeat(64)

  savePendingCredentialMaintenanceOperation(1, 21, actorOneTokenTwentyOne)
  savePendingCredentialMaintenanceOperation(1, 22, actorOneTokenTwentyTwo)
  savePendingCredentialMaintenanceOperation(2, 21, actorTwoTokenTwentyOne)

  expect(readPendingCredentialMaintenanceOperation(1, 21)).toBe(
    actorOneTokenTwentyOne
  )
  expect(readPendingCredentialMaintenanceOperation(1, 22)).toBe(
    actorOneTokenTwentyTwo
  )
  expect(readPendingCredentialMaintenanceOperation(2, 21)).toBe(
    actorTwoTokenTwentyOne
  )
})

test('an old instance cannot overwrite or clear a different operation', () => {
  const currentOperation = 'd'.repeat(64)
  const staleOperation = 'e'.repeat(64)
  savePendingCredentialMaintenanceOperation(1, 21, currentOperation)

  expect(() =>
    savePendingCredentialMaintenanceOperation(1, 21, staleOperation)
  ).toThrow('Another credential maintenance operation is pending')
  expect(() =>
    clearPendingCredentialMaintenanceOperation(1, 21, staleOperation)
  ).toThrow('Pending credential maintenance operation changed')
  expect(readPendingCredentialMaintenanceOperation(1, 21)).toBe(
    currentOperation
  )

  clearPendingCredentialMaintenanceOperation(1, 21, currentOperation)
  expect(readPendingCredentialMaintenanceOperation(1, 21)).toBeNull()
})
