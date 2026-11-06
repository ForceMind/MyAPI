/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */
import type { QueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'

export type SavedContentActor = { id: number; role: number; sid: string | null }

export class SavedContentDeniedError extends Error {
  constructor(readonly status: number) {
    super('Request content is available to administrators only.')
    this.name = 'SavedContentDeniedError'
  }
}

export function savedContentStatus(error: unknown): number | undefined {
  if (error instanceof SavedContentDeniedError) return error.status
  return isAxiosError(error) ? error.response?.status : undefined
}

/** Clear both original and expanded snapshots; cancellation must not restore the old data. */
export async function denySavedContent(client: QueryClient, requestId: string, actor: SavedContentActor, status: number): Promise<void> {
  const queryKey = ['full-content-logs', 'detail', requestId, actor.id, actor.sid, actor.role]
  await client.cancelQueries({ queryKey }, { revert: false })
  const error = new SavedContentDeniedError(status)
  for (const query of client.getQueryCache().findAll({ queryKey })) {
    query.setState({ data: undefined, error, status: 'error', fetchStatus: 'idle', errorUpdatedAt: Date.now() })
  }
}
