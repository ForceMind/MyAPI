/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { z } from 'zod'

export const modelRouteSchema = z.object({
  public_model: z
    .string()
    .min(1)
    .max(255)
    .refine((value) => value.trim() === value && !/[\r\n\t,]/.test(value)),
  upstream_model: z
    .string()
    .min(1)
    .max(255)
    .refine((value) => value.trim() === value && !/[\r\n\t,]/.test(value)),
  endpoint: z.enum(['', '/v1/chat/completions', '/v1/responses']),
  match: z.enum(['exact', 'prefix']),
  priority: z.number().int(),
})
export type ModelRoute = z.infer<typeof modelRouteSchema>

export function validateModelRoutes(
  settings: string | undefined,
  type: number
): string | null {
  let parsed: { model_routes?: unknown }
  try {
    parsed = JSON.parse(settings || '{}')
  } catch {
    return null
  }
  if (!parsed || parsed.model_routes === undefined) return null
  const result = z
    .array(modelRouteSchema)
    .max(256)
    .safeParse(parsed.model_routes)
  if (!result.success) {
    return 'Each route requires a public model, upstream model, endpoint, match mode, and integer priority.'
  }
  if (result.data.length && type !== 1 && type !== 57) {
    return 'Model routes support only OpenAI and Codex channels.'
  }
  for (const [index, route] of result.data.entries()) {
    if (type === 57 && route.endpoint === '/v1/chat/completions') {
      return 'Codex routes require Responses.'
    }
    if (
      result.data
        .slice(0, index)
        .some(
          (other) =>
            other.public_model === route.public_model &&
            other.match === route.match &&
            other.endpoint === route.endpoint &&
            other.priority === route.priority &&
            other.upstream_model !== route.upstream_model
        )
    ) {
      return 'Conflicting model routes have the same match, endpoint, and priority.'
    }
  }
  return null
}
