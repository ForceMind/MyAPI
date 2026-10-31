/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import z from 'zod'

/** Tabs of the first-version connection inspector, kept for earlier links. */
const LEGACY_INSPECTOR_TABS = ['overview', 'windows', 'routing'] as const

/**
 * `/connections` search: the card grid's filters and page, and the card to
 * focus on arrival. An earlier link
 * that opened a connection in the inspector (`connection`, `tab`) is
 * redirected to the connection's own page.
 */
export const connectionsSearchSchema = z.object({
  page: z.number().int().positive().max(Number.MAX_SAFE_INTEGER).optional().catch(undefined),
  pageSize: z.number().int().positive().max(100).optional().catch(undefined),
  filter: z.string().trim().optional().catch(undefined),
  status: z.union([z.enum(['enabled', 'disabled']), z.array(z.string())]).optional().catch(undefined),
  type: z.array(z.string()).optional().catch(undefined),
  group: z.array(z.string()).optional().catch(undefined),
  model: z.string().optional().catch(undefined),
  view: z.literal('table').optional(),
  quotaChannelId: z.number().int().positive().optional().catch(undefined),
  /** The card to focus on arrival (Back from a page opened by a link). */
  focusConnectionId: z.number().int().positive().optional().catch(undefined),
  connection: z.number().int().positive().optional().catch(undefined),
  tab: z.enum(LEGACY_INSPECTOR_TABS).optional().catch(undefined),
})
export type ConnectionsSearch = z.infer<typeof connectionsSearchSchema>
export const CONNECTION_PAGE_SIZE = 20

/** The list state a connection page returns to. */
const connectionReturnSchema = connectionsSearchSchema.pick({
  page: true,
  filter: true,
  status: true,
  pageSize: true,
  type: true,
  group: true,
  model: true,
  view: true,
  quotaChannelId: true,
})
export type ConnectionReturn = z.infer<typeof connectionReturnSchema>

/** Tabs of the connection page; windows is the default and stays out of the URL. */
export const CONNECTION_PAGE_TABS = [
  'windows',
  'routing',
  'requests',
  'history',
] as const
export type ConnectionPageTab = (typeof CONNECTION_PAGE_TABS)[number]

/** `/connections/$connectionId` search: the tab and the list to return to. */
export const connectionPageSearchSchema = z.object({
  tab: z
    .enum(CONNECTION_PAGE_TABS)
    .exclude(['windows'])
    .optional()
    .catch(undefined),
  connectionReturn: connectionReturnSchema.optional().catch(undefined),
})
export type ConnectionPageSearch = z.infer<typeof connectionPageSearchSchema>

export const connectionIdSchema = z
  .union([z.number(), z.string().regex(/^\d+$/).transform(Number)])
  .pipe(z.number().int().positive().max(2147483647))

/** The list's filters and page, when any differ from the plain first page. */
export function connectionListContext(
  search: ConnectionReturn
): ConnectionReturn | undefined {
  const context: ConnectionReturn = {
    page: search.page && search.page > 1 ? search.page : undefined,
    filter: search.filter || undefined,
    status: search.status,
    pageSize: search.pageSize,
    type: search.type,
    group: search.group,
    model: search.model || undefined,
    view: search.view,
    quotaChannelId: search.quotaChannelId,
  }
  return Object.values(context).some((value) => value !== undefined)
    ? context
    : undefined
}

/** The backend supports one value per facet; keep full URL arrays for round trips. */
export function connectionFilterValue(value: string | string[] | undefined): string | undefined {
  if (Array.isArray(value) && value.includes('all')) return undefined
  const first = Array.isArray(value) ? value[0] : value
  return first && first !== 'all' ? first : undefined
}

/** Card and table URLs retain their existing status representations. */
export function connectionsToTableSearch(search: ConnectionReturn) {
  return {
    page: search.page,
    pageSize: search.pageSize ?? CONNECTION_PAGE_SIZE,
    filter: search.filter,
    status: typeof search.status === 'string' ? [search.status] : (search.status ?? []),
    type: search.type ?? [],
    group: search.group ?? [],
    model: search.model,
    quotaChannelId: search.quotaChannelId,
  }
}

export function tableToConnectionsSearch(search: {
  page?: number
  pageSize?: number
  filter?: string
  status?: string[]
  type?: string[]
  group?: string[]
  model?: string
  quotaChannelId?: number
}): ConnectionsSearch {
  const status = search.status
  const single = status?.length === 1 ? status[0] : undefined
  return {
    page: search.page,
    pageSize: search.pageSize,
    filter: search.filter,
    status: single === 'enabled' || single === 'disabled' ? single : status,
    type: search.type,
    group: search.group,
    model: search.model,
    quotaChannelId: search.quotaChannelId,
  }
}

/**
 * Where an earlier `/connections?connection=<id>[&tab=]` inspector link now
 * points: the connection page on the matching tab, returning to the same
 * list. `undefined` when there is nothing to rewrite.
 */
export function legacyConnectionLinkTarget(
  search: ConnectionsSearch
): { connectionId: number; search: ConnectionPageSearch } | undefined {
  if (search.connection === undefined) return undefined
  return {
    connectionId: search.connection,
    search: {
      tab: search.tab === 'routing' ? 'routing' : undefined,
      connectionReturn: connectionListContext(search),
    },
  }
}

/** ChatGPT Subscription (Codex): an account with usage windows. */
export const SUBSCRIPTION_CONNECTION_TYPE = 57
/** Ollama and Xinference serve models from the operator's own machine. */
const LOCAL_CONNECTION_TYPES = new Set([4, 47])

export type ConnectionKind = 'subscription' | 'api' | 'local'

/**
 * How a connection is billed, for its badge: a subscription account (it
 * reports usage windows), a model server on the operator's machine, or an
 * API Key billed per use.
 */
export function connectionKind(
  connection: Pick<ConnectionSummary, 'type'>,
  hasWindows: boolean
): ConnectionKind {
  if (hasWindows || connection.type === SUBSCRIPTION_CONNECTION_TYPE) {
    return 'subscription'
  }
  return LOCAL_CONNECTION_TYPES.has(connection.type) ? 'local' : 'api'
}

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
  // Last saved connectivity test; 0 means never tested.
  test_time: z.number().int().nullish(),
  response_time: z.number().int().nullish(),
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
