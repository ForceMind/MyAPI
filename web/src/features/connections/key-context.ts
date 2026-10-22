/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import z from 'zod'

import { connectionsSearchSchema } from './connection-state'

const positiveId = z.number().int().positive().max(Number.MAX_SAFE_INTEGER)
export const connectionKeyContextSchema = z.object({
  connection: connectionsSearchSchema.extend({ connection: positiveId }),
  pool: positiveId.optional(),
  poolPage: positiveId.max(1000000).optional(),
  key: positiveId.optional(),
})
export type ConnectionKeyContext = z.infer<typeof connectionKeyContextSchema>
