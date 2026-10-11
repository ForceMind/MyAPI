// API-boundary schemas validate each component. Null means only that their
// derived sum cannot be represented as a safe JavaScript integer.
export function checkedAddSafeIntegers(
  left: number,
  right: number
): number | null {
  if (!Number.isSafeInteger(left) || !Number.isSafeInteger(right)) {
    return null
  }
  if (
    (right > 0 && left > Number.MAX_SAFE_INTEGER - right) ||
    (right < 0 && left < Number.MIN_SAFE_INTEGER - right)
  ) {
    return null
  }
  return left + right
}
