/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import z from 'zod'

export const connectionsSearchSchema = z.object({
  page: z.number().int().positive().optional().catch(undefined),
  filter: z.string().trim().max(200).optional().catch(undefined),
  status: z.enum(['enabled', 'disabled']).optional().catch(undefined),
  connection: z.number().int().positive().optional().catch(undefined),
})
export type ConnectionsSearch = z.infer<typeof connectionsSearchSchema>
export const CONNECTION_PAGE_SIZE = 20

// Deliberately project configuration metadata before it reaches query caches.
// Credentials, opaque settings and provider responses never belong in this view.
const connectionSchema = z.object({
  id: z.number().int().positive(),
  type: z.number().int(),
  name: z.string(),
  status: z.number().int(),
  models: z.string().nullish(),
  group: z.string().nullish(),
  priority: z.number().int().nullish(),
  weight: z.number().int().nullish(),
})
export type ConnectionSummary = z.infer<typeof connectionSchema>

export function parseConnections(payload: unknown) {
  const result = z
    .object({
      success: z.literal(true),
      data: z.object({
        total: z.number().int().nonnegative(),
        items: z.array(connectionSchema),
      }),
    })
    .parse(payload).data
  if (
    result.total < result.items.length ||
    new Set(result.items.map((item) => item.id)).size !== result.items.length
  ) {
    throw new Error('Invalid connection list')
  }
  return result
}

export function parseConnection(payload: unknown, expectedId: number) {
  const item = z
    .object({ success: z.literal(true), data: connectionSchema })
    .parse(payload).data
  if (item.id !== expectedId) throw new Error('Connection identity changed')
  return item
}

export function connectionValues(value: string | null | undefined): string[] {
  return [
    ...new Set(
      value
        ?.split(',')
        .map((item) => item.trim())
        .filter(Boolean) ?? []
    ),
  ]
}
