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
const HANDOFF_MAX_AGE_MS = 30_000

type PendingPlaygroundKeyHandoff = {
  requestId: number
  userId: number
  sessionId: string | null
  keyId: number
  createdAt: number
}

export type PlaygroundKeyHandoff =
  | { status: 'none' }
  | { status: 'rejected'; requestId: number }
  | { status: 'accepted'; requestId: number; keyId: number }

let pendingHandoff: PendingPlaygroundKeyHandoff | null = null
let nextRequestId = 1

export function createPlaygroundKeyHandoff(input: {
  userId: number
  sessionId: string | null
  keyId: number
  now?: number
}): void {
  if (
    !Number.isSafeInteger(input.userId) ||
    input.userId <= 0 ||
    !Number.isSafeInteger(input.keyId) ||
    input.keyId <= 0
  ) {
    pendingHandoff = null
    return
  }

  pendingHandoff = {
    requestId: nextRequestId++,
    userId: input.userId,
    sessionId: input.sessionId,
    keyId: input.keyId,
    createdAt: input.now ?? Date.now(),
  }
}

export function readPlaygroundKeyHandoff(
  userId: number,
  sessionId: string | null,
  now = Date.now()
): PlaygroundKeyHandoff {
  if (!pendingHandoff) return { status: 'none' }

  if (
    now - pendingHandoff.createdAt > HANDOFF_MAX_AGE_MS ||
    pendingHandoff.userId !== userId ||
    pendingHandoff.sessionId !== sessionId
  ) {
    return { status: 'rejected', requestId: pendingHandoff.requestId }
  }

  return {
    status: 'accepted',
    requestId: pendingHandoff.requestId,
    keyId: pendingHandoff.keyId,
  }
}

export function completePlaygroundKeyHandoff(requestId: number): void {
  if (pendingHandoff?.requestId === requestId) pendingHandoff = null
}
