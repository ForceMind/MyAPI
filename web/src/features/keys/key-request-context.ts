/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import z from 'zod'

import { API_KEY_STATUS_OPTIONS } from './constants'

export const savedKeyIdSchema = z
  .union([z.number(), z.string().regex(/^\d+$/).transform(Number)])
  .pipe(z.number().int().positive().max(2147483647))

// Deliberately excludes the credential search parameter and arbitrary URLs.
export const keyRequestContextSchema = z.object({
  page: z.number().int().positive().optional(),
  pageSize: z.number().int().positive().max(1000).optional(),
  filter: z.string().max(512).optional(),
  status: z
    .array(z.enum(API_KEY_STATUS_OPTIONS.map((s) => s.value as `${number}`)))
    .optional(),
  keyId: savedKeyIdSchema.optional(),
  focusKeyId: savedKeyIdSchema.optional(),
})

export type KeyRequestContext = z.infer<typeof keyRequestContextSchema>
