/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import z from 'zod'

import { connectionKeyContextSchema } from '@/features/connections/key-context'
import { requestInspectorTabSchema } from '@/features/usage-logs/lib/request-inspector-tabs'

import { API_KEY_STATUS_OPTIONS } from '../constants'
import { savedKeyIdSchema } from '../key-request-context'

export const KEY_INSPECTOR_TABS = ['requests', 'access', 'shares'] as const
export type KeyInspectorTab = (typeof KEY_INSPECTOR_TABS)[number]

/**
 * `/keys` search. The inspector state lives here so refresh, Back and shared
 * links reopen the same Key, tab and request: `inspect` is the Key, `tab`
 * its page (recent requests by default), `request` a Request ID inside it
 * (`requestLog` a row ID for legacy records without one) and `requestTab`
 * the request's diagnosis/body/SSE tab. `keyId` is the older link form and
 * is redirected to `inspect`.
 */
export const apiKeySearchSchema = z.object({
  inspect: savedKeyIdSchema.optional().catch(undefined),
  tab: z.enum(KEY_INSPECTOR_TABS).optional().catch(undefined),
  request: z.string().trim().min(1).max(128).optional().catch(undefined),
  requestLog: z
    .number()
    .int()
    .positive()
    .max(Number.MAX_SAFE_INTEGER)
    .optional()
    .catch(undefined),
  requestTab: requestInspectorTabSchema,
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

/**
 * The search an older `?keyId=` link now means: the same Key open in the
 * inspector on its recent requests. `undefined` when nothing to rewrite.
 */
export function legacyKeyLinkTarget(
  search: ApiKeySearch
): ApiKeySearch | undefined {
  if (search.keyId === undefined) return undefined
  return { ...search, keyId: undefined, inspect: search.keyId }
}
