/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { createFileRoute, redirect } from '@tanstack/react-router'

import { KeyPage } from '@/features/keys/key-page'
import { savedKeyIdSchema } from '@/features/keys/key-request-context'
import { keyPageSearchSchema } from '@/features/keys/lib/keys-search'

export const Route = createFileRoute('/_authenticated/keys/$keyId/')({
  validateSearch: keyPageSearchSchema,
  beforeLoad: ({ params }) => {
    if (!savedKeyIdSchema.safeParse(params.keyId).success) {
      throw redirect({ to: '/keys', replace: true })
    }
  },
  component: KeyPage,
})
