/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import z from 'zod'

/** A Request ID as it appears in a page address. */
export const requestIdSchema = z.string().trim().min(1).max(128)

/**
 * Content tabs of the request page. Diagnosis is always shown beside them;
 * the saved body is the default and stays out of the URL.
 */
const REQUEST_PAGE_TABS = ['body', 'sse', 'raw'] as const
export type RequestPageTab = (typeof REQUEST_PAGE_TABS)[number]

export const requestPageSearchSchema = z.object({
  tab: z.enum(REQUEST_PAGE_TABS).optional().catch(undefined),
})

export type RequestPageSearch = z.infer<typeof requestPageSearchSchema>

/** `requestTab` of earlier inspector links. */
export const legacyRequestTabSchema = z
  .enum(['diagnosis', 'body', 'sse'])
  .optional()
  .catch(undefined)

/** The request page tab an earlier `requestTab` meant. */
export function legacyRequestTab(
  tab: z.infer<typeof legacyRequestTabSchema>
): RequestPageTab | undefined {
  return tab === 'body' || tab === 'sse' ? tab : undefined
}

/**
 * Where an earlier `/usage-logs/common?requestId=…&inspect=request` link
 * now points. `undefined` when the link only filters the call records.
 */
export function legacyRequestLinkTarget(search: {
  inspect?: 'request'
  requestId?: string
  requestTab?: z.infer<typeof legacyRequestTabSchema>
}): { requestId: string; search: RequestPageSearch } | undefined {
  if (search.inspect !== 'request') return undefined
  const requestId = requestIdSchema.safeParse(search.requestId)
  if (!requestId.success) return undefined
  return {
    requestId: requestId.data,
    search: { tab: legacyRequestTab(search.requestTab) },
  }
}
