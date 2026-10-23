export const INPUT_TOKEN_LIMITS_ERROR =
  'Input limits must be a JSON object with non-empty model names and integer values from 1 to 2147483647.'

export function parseInputTokenLimits(value?: string): Record<string, number> {
  if (!value?.trim()) return {}
  const parsed: unknown = JSON.parse(value)
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error(INPUT_TOKEN_LIMITS_ERROR)
  }
  for (const [model, limit] of Object.entries(parsed)) {
    if (
      !model.trim() ||
      model !== model.trim() ||
      typeof limit !== 'number' ||
      !Number.isSafeInteger(limit) ||
      limit < 1 ||
      limit > 2147483647
    ) {
      throw new Error(INPUT_TOKEN_LIMITS_ERROR)
    }
  }
  return parsed as Record<string, number>
}

export function isValidInputTokenLimits(value?: string): boolean {
  try {
    parseInputTokenLimits(value)
    return true
  } catch {
    return false
  }
}
