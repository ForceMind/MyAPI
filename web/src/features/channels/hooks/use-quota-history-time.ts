/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useState } from 'react'

import type { ChannelQuotaHistoryRange } from '../types'

export interface QuotaCustomRange {
  start: string
  end: string
}

export function useQuotaHistoryTime(initial: ChannelQuotaHistoryRange = '24h') {
  const [range, setRange] = useState<ChannelQuotaHistoryRange>(initial)
  const [customRange, setCustomRange] = useState<QuotaCustomRange>(() => {
    const now = Date.now()
    return {
      start: new Date(now - 86400000).toISOString(),
      end: new Date(now).toISOString(),
    }
  })
  return {
    range,
    setRange,
    customRange,
    setCustomRange,
    params: {
      range,
      start: range === 'custom' ? customRange.start : undefined,
      end: range === 'custom' ? customRange.end : undefined,
    },
  }
}
