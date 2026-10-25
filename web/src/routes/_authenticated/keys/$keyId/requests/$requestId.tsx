/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { createFileRoute, redirect } from '@tanstack/react-router'

import { savedKeyIdSchema } from '@/features/keys/key-request-context'
import {
  requestIdSchema,
  requestPageSearchSchema,
} from '@/features/usage-logs/lib/request-page-search'
import { KeyRequestPage } from '@/features/usage-logs/request-page'

export const Route = createFileRoute(
  '/_authenticated/keys/$keyId/requests/$requestId'
)({
  validateSearch: requestPageSearchSchema,
  beforeLoad: ({ params }) => {
    if (!savedKeyIdSchema.safeParse(params.keyId).success) {
      throw redirect({ to: '/keys', replace: true })
    }
    if (!requestIdSchema.safeParse(params.requestId).success) {
      throw redirect({
        to: '/keys/$keyId',
        params: { keyId: params.keyId },
        replace: true,
      })
    }
  },
  component: KeyRequestPage,
})
