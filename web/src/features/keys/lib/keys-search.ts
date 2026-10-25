/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import z from 'zod'

import { connectionKeyContextSchema } from '@/features/connections/key-context'
import {
  legacyRequestTab,
  legacyRequestTabSchema,
  requestIdSchema,
  type RequestPageSearch,
} from '@/features/usage-logs/lib/request-page-search'

import { API_KEY_STATUS_OPTIONS } from '../constants'
import {
  keyRequestContextSchema,
  savedKeyIdSchema,
  type KeyRequestContext,
} from '../key-request-context'

/** Tabs of the Key page; requests is the default and stays out of the URL. */
const KEY_PAGE_TABS = ['requests', 'scope', 'delivery'] as const
export type KeyPageTab = (typeof KEY_PAGE_TABS)[number]

const savedRowIdSchema = z
  .number()
  .int()
  .positive()
  .max(Number.MAX_SAFE_INTEGER)

/**
 * `/keys/$keyId` search: the page tab, a legacy call record without a
 * Request ID opened over the page (`requestLog`, its row ID) and the list
 * state the page returns to (`keyReturn`).
 */
export const keyPageSearchSchema = z.object({
  tab: z.enum(KEY_PAGE_TABS).optional().catch(undefined),
  requestLog: savedRowIdSchema.optional().catch(undefined),
  keyReturn: keyRequestContextSchema.optional().catch(undefined),
})

export type KeyPageSearch = z.infer<typeof keyPageSearchSchema>

/**
 * `/keys` search: the list's filters and page, plus the inspector parameters
 * of earlier links (`inspect`, `keyId`, `tab`, `request`, `requestLog`,
 * `requestTab`), which the route redirects to the Key and request pages.
 */
export const apiKeySearchSchema = z.object({
  inspect: savedKeyIdSchema.optional().catch(undefined),
  tab: z.enum(['requests', 'access', 'shares']).optional().catch(undefined),
  request: requestIdSchema.optional().catch(undefined),
  requestLog: savedRowIdSchema.optional().catch(undefined),
  requestTab: legacyRequestTabSchema,
  keyId: savedKeyIdSchema.optional(),
  focusKeyId: savedKeyIdSchema.optional(),
  connectionContext: connectionKeyContextSchema.optional().catch(undefined),
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(undefined),
  status: z
    .array(z.enum(API_KEY_STATUS_OPTIONS.map((s) => s.value as `${number}`)))
    .optional()
    .catch([]),
  filter: z.string().optional().catch(''),
  token: z.string().optional().catch(''),
})

export type ApiKeySearch = z.infer<typeof apiKeySearchSchema>

/** The list state a Key page returns to; credential searches stay out. */
export function keyListContext(
  search: Pick<ApiKeySearch, 'page' | 'pageSize' | 'filter' | 'status'>
): KeyRequestContext | undefined {
  const context = keyRequestContextSchema.safeParse({
    page: search.page && search.page > 1 ? search.page : undefined,
    pageSize: search.pageSize,
    filter: search.filter || undefined,
    status: search.status?.length ? search.status : undefined,
  })
  if (!context.success) return undefined
  return Object.values(context.data).some((value) => value !== undefined)
    ? context.data
    : undefined
}

export type LegacyKeyLink =
  | { page: 'key'; keyId: number; search: KeyPageSearch }
  | {
      page: 'request'
      keyId: number
      requestId: string
      search: RequestPageSearch
    }

/**
 * Where an earlier `/keys` inspector link now points: `?inspect=` (or the
 * older `?keyId=`) opens the Key page on the matching tab, with `request`
 * the request page inside that Key. `undefined` when nothing to rewrite.
 */
export function legacyKeyLinkTarget(
  search: ApiKeySearch
): LegacyKeyLink | undefined {
  const keyId = search.inspect ?? search.keyId
  if (keyId === undefined) return undefined
  if (search.request !== undefined) {
    return {
      page: 'request',
      keyId,
      requestId: search.request,
      search: { tab: legacyRequestTab(search.requestTab) },
    }
  }
  return {
    page: 'key',
    keyId,
    search: {
      tab:
        search.tab === 'access' || search.tab === 'shares'
          ? 'scope'
          : undefined,
      requestLog: search.requestLog,
      keyReturn: keyListContext(search),
    },
  }
}
