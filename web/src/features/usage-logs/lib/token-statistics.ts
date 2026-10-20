/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { z } from 'zod'

const count = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER)
const tokens = z.string().regex(/^(0|[1-9]\d*)$/)

export const tokenStatisticsSchema = z
  .object({
    records: count,
    known_records: count,
    unknown_records: count,
    input_tokens: tokens.nullable(),
    output_tokens: tokens.nullable(),
    total_tokens: tokens.nullable(),
    known_input_tokens: tokens,
    known_output_tokens: tokens,
    known_total_tokens: tokens,
    cache_known_records: count,
    cache_unknown_records: count,
    cache_read_tokens: tokens,
    cache_input_tokens: tokens,
    cache_hit_rate: z.number().min(0).max(1).nullable(),
  })
  .refine((value) => {
    const input = BigInt(value.known_input_tokens)
    const output = BigInt(value.known_output_tokens)
    const cacheInput = BigInt(value.cache_input_tokens)
    const cached = BigInt(value.cache_read_tokens)
    const complete = value.unknown_records === 0
    return (
      value.known_records + value.unknown_records === value.records &&
      value.cache_known_records + value.cache_unknown_records ===
        value.records &&
      value.cache_known_records <= value.known_records &&
      input + output === BigInt(value.known_total_tokens) &&
      cached <= cacheInput &&
      cacheInput <= input &&
      (cacheInput === 0n
        ? value.cache_hit_rate === null
        : value.cache_hit_rate !== null) &&
      (complete
        ? value.input_tokens === value.known_input_tokens &&
          value.output_tokens === value.known_output_tokens &&
          value.total_tokens === value.known_total_tokens
        : value.input_tokens === null &&
          value.output_tokens === null &&
          value.total_tokens === null)
    )
  }, 'Inconsistent Token statistics')

export type TokenUsageStatistics = z.infer<typeof tokenStatisticsSchema>
