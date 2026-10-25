/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { createFileRoute, redirect } from '@tanstack/react-router'

import {
  requestIdSchema,
  requestPageSearchSchema,
} from '@/features/usage-logs/lib/request-page-search'
import { CallRecordRequestPage } from '@/features/usage-logs/request-page'
import { USAGE_LOGS_DEFAULT_SECTION } from '@/features/usage-logs/section-registry'

export const Route = createFileRoute(
  '/_authenticated/usage-logs/request/$requestId'
)({
  validateSearch: requestPageSearchSchema,
  beforeLoad: ({ params }) => {
    if (!requestIdSchema.safeParse(params.requestId).success) {
      throw redirect({
        to: '/usage-logs/$section',
        params: { section: USAGE_LOGS_DEFAULT_SECTION },
        replace: true,
      })
    }
  },
  component: CallRecordRequestPage,
})
