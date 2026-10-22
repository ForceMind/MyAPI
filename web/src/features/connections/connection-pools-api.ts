/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import z from 'zod'

import { api } from '@/lib/api'

const positiveId = z.number().int().positive().max(Number.MAX_SAFE_INTEGER)
const connectionPoolsSchema = z.object({
  success: z.literal(true),
  data: z.object({
    channel_id: positiveId,
    has_more: z.boolean(),
    items: z
      .array(
        z.object({
          pool_id: positiveId,
          channel_id: positiveId,
          window_source: z.string(),
          window_type: z.string(),
          enabled: z.boolean(),
        })
      )
      .max(20),
  }),
})

export async function getConnectionPools(
  channelId: number,
  page: number,
  signal?: AbortSignal
) {
  const response = await api.get(
    `/api/subscription-share/channels/${channelId}/pools`,
    {
      params: { page },
      signal,
      disableDuplicate: true,
      skipAuthRefresh: true,
      skipBusinessError: true,
      skipErrorHandler: true,
    }
  )
  const view = connectionPoolsSchema.parse(response.data).data
  if (
    view.channel_id !== channelId ||
    view.items.some((item) => item.channel_id !== channelId) ||
    new Set(view.items.map((item) => item.pool_id)).size !== view.items.length
  ) {
    throw new Error('Connection pool identity mismatch')
  }
  return view
}
