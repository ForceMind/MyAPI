/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { UsageLog } from '../data/schema'
import type { LogOtherData } from '../types'

/** Request completion and usage evidence are independent facts. */
export function getPersonalCallEvidence(log: UsageLog) {
  let other: LogOtherData | null = null
  try {
    const parsed: unknown = JSON.parse(log.other || 'null')
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      other = parsed as LogOtherData
    }
  } catch {
    // Historical malformed metadata cannot establish usage or completion.
  }
  const completed =
    other?.request_outcome === 'completed' ||
    (log.type === 2 && !other?.request_outcome)
  const validTokens =
    Number.isSafeInteger(log.prompt_tokens) &&
    log.prompt_tokens >= 0 &&
    Number.isSafeInteger(log.completion_tokens) &&
    log.completion_tokens >= 0
  const usageKnown =
    validTokens &&
    (other?.usage_outcome === 'reported' ||
      (!other?.usage_outcome &&
        log.type === 2 &&
        (log.prompt_tokens > 0 || log.completion_tokens > 0)))
  return { completed, usageKnown, other }
}
