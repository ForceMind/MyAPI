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
 * Request evidence tabs. The saved body remains the default and stays out
 * of the URL; diagnosis has its own explicit tab.
 */
const REQUEST_PAGE_TABS = ['detail', 'body', 'sse', 'raw'] as const
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
  if (tab === 'diagnosis') return 'detail'
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
