/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

const STORAGE_PREFIX = 'myapi.credential-maintenance.pending.v1'
const operationIdPattern = /^[0-9a-f]{64}$/

type PendingCredentialMaintenanceOperation = {
  version: 1
  actor_id: number
  token_id: number
  operation_id: string
}

function storageKey(actorId: number, tokenId: number): string {
  if (!Number.isSafeInteger(actorId) || actorId <= 0) {
    throw new Error('Invalid credential maintenance actor')
  }
  if (!Number.isSafeInteger(tokenId) || tokenId <= 0) {
    throw new Error('Invalid credential maintenance token')
  }
  return `${STORAGE_PREFIX}.${actorId}.${tokenId}`
}

function sessionStore(): Storage {
  if (typeof window === 'undefined') {
    throw new Error('Credential maintenance session storage is unavailable')
  }
  return window.sessionStorage
}

export function readPendingCredentialMaintenanceOperation(
  actorId: number,
  tokenId: number
): string | null {
  const raw = sessionStore().getItem(storageKey(actorId, tokenId))
  if (raw === null) return null
  const parsed: unknown = JSON.parse(raw)
  if (
    !parsed ||
    typeof parsed !== 'object' ||
    Array.isArray(parsed) ||
    Object.keys(parsed).length !== 4
  ) {
    throw new Error('Invalid pending credential maintenance operation')
  }
  const record = parsed as Partial<PendingCredentialMaintenanceOperation>
  if (
    record.version !== 1 ||
    record.actor_id !== actorId ||
    record.token_id !== tokenId ||
    typeof record.operation_id !== 'string' ||
    !operationIdPattern.test(record.operation_id)
  ) {
    throw new Error('Invalid pending credential maintenance operation')
  }
  return record.operation_id
}

export function savePendingCredentialMaintenanceOperation(
  actorId: number,
  tokenId: number,
  operationId: string
): void {
  if (!operationIdPattern.test(operationId)) {
    throw new Error('Invalid pending credential maintenance operation')
  }
  const key = storageKey(actorId, tokenId)
  const existing = readPendingCredentialMaintenanceOperation(actorId, tokenId)
  if (existing && existing !== operationId) {
    throw new Error('Another credential maintenance operation is pending')
  }
  const value = JSON.stringify({
    version: 1,
    actor_id: actorId,
    token_id: tokenId,
    operation_id: operationId,
  } satisfies PendingCredentialMaintenanceOperation)
  const storage = sessionStore()
  storage.setItem(key, value)
  if (storage.getItem(key) !== value) {
    throw new Error('Pending credential maintenance operation was not saved')
  }
}

export function clearPendingCredentialMaintenanceOperation(
  actorId: number,
  tokenId: number,
  expectedOperationId: string
): void {
  if (!operationIdPattern.test(expectedOperationId)) {
    throw new Error('Invalid pending credential maintenance operation')
  }
  const key = storageKey(actorId, tokenId)
  const storage = sessionStore()
  const existing = readPendingCredentialMaintenanceOperation(actorId, tokenId)
  if (existing === null) return
  if (existing !== expectedOperationId) {
    throw new Error('Pending credential maintenance operation changed')
  }
  storage.removeItem(key)
  if (storage.getItem(key) !== null) {
    throw new Error('Pending credential maintenance operation was not cleared')
  }
}
