/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */
import type { RowSelectionState } from '@tanstack/react-table'

import { useAuthStore } from '@/stores/auth-store'

// IDs only, bounded to twenty list history entries; no credentials or records.
const entries = new Map<string, RowSelectionState>()
function currentIdentity(): string {
  const auth = useAuthStore.getState().auth
  return `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`
}
let owner = currentIdentity()
useAuthStore.subscribe(() => {
  const next = currentIdentity()
  if (next !== owner) {
    entries.clear()
    owner = next
  }
})

export function readKeyListSelection(identity: string, entry: string | undefined): RowSelectionState {
  if (!entry || identity !== owner) return {}
  return { ...entries.get(entry) }
}

export function rememberKeyListSelection(identity: string, entry: string | undefined, rows: RowSelectionState): void {
  if (!entry || identity !== owner) return
  entries.delete(entry)
  entries.set(entry, { ...rows })
  while (entries.size > 20) {
    const first = entries.keys().next().value
    if (first === undefined) break
    entries.delete(first)
  }
}
