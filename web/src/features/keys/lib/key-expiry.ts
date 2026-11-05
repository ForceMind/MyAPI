/** Only the explicit -1 sentinel is perpetual; malformed observations stay unknown. */
export function keyExpiry(value: unknown):
  | { kind: 'never' }
  | { kind: 'unknown' }
  | { kind: 'dated'; timestamp: number } {
  if (value === -1) return { kind: 'never' }
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value <= 0 ||
    !Number.isFinite(new Date(value * 1000).getTime())) return { kind: 'unknown' }
  return { kind: 'dated', timestamp: value }
}
