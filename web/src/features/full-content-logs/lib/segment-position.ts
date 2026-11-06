/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */
import { useAuthStore } from '@/stores/auth-store'

type Position = { offset: string; snapshot: string }
const positions = new Map<string, Position>()

// No body, headers, URLs or credentials enter this bounded return-position cache.
export const savedSegmentPositions = {
  read(key: string) { return positions.get(key) },
  write(key: string, point: Position) {
    positions.delete(key)
    positions.set(key, { ...point })
    if (positions.size > 20) {
      const oldest = positions.keys().next().value
      if (oldest !== undefined) positions.delete(oldest)
    }
  },
  remove(key: string) { positions.delete(key) },
}

useAuthStore.subscribe((state, previous) => {
  if (state.auth.user?.id !== previous.auth.user?.id || state.auth.user?.role !== previous.auth.user?.role ||
    (state.auth.session?.sid ?? null) !== (previous.auth.session?.sid ?? null)) positions.clear()
})
