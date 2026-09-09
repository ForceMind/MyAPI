/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useEffect, useState } from 'react'

import { computeTimeRange } from '@/lib/time'

export function useRollingDayWindow() {
  const [range, setRange] = useState(() => computeTimeRange(1))
  useEffect(() => {
    const interval = window.setInterval(
      () => setRange(computeTimeRange(1)),
      60000
    )
    return () => window.clearInterval(interval)
  }, [])
  return range
}
