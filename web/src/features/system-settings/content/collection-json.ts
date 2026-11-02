export type ContentEntry = { id: number }

/** Local row identities only; callers keep their existing serializers. */
export function parseContentEntries<T extends ContentEntry>(source: string): { entries: T[]; incompatible: boolean } {
  try {
    const parsed: unknown = JSON.parse(source || '[]')
    if (!Array.isArray(parsed)) return { entries: [], incompatible: true }
    const seen = new Set<number>()
    const entries: T[] = []
    for (const [index, value] of parsed.entries()) {
      if (!value || typeof value !== 'object' || Array.isArray(value)) return { entries: [], incompatible: true }
      const record = value as Record<string, unknown>
      const id = record.id ?? index + 1
      if (typeof id !== 'number' || !Number.isFinite(id) || seen.has(id)) return { entries: [], incompatible: true }
      seen.add(id)
      entries.push({ ...record, id } as T)
    }
    return { entries, incompatible: false }
  } catch {
    return { entries: [], incompatible: true }
  }
}
