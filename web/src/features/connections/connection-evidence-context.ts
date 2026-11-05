/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */
import { createContext } from 'react'

import type { ChannelQuotaChangeItem } from '@/features/channels/types'

/** One actor-scoped saved read supplies all real rows, including mobile name cells. */
export const ConnectionEvidenceContext = createContext<{
  identity: string
  data: ChannelQuotaChangeItem[] | undefined
  known: boolean
  loading: boolean
  now: number
} | null>(null)
