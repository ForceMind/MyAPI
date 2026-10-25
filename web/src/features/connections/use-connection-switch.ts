/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import {
  channelsQueryKeys,
  handleDisableChannel,
  handleEnableChannel,
} from '@/features/channels/lib/channel-actions'

/**
 * Turns a connection on or off with the existing status action (it reports
 * success or failure as a toast). The saved list and the connection are read
 * again afterwards, so the switch always shows the stored state.
 */
export function useConnectionSwitch() {
  const client = useQueryClient()
  const [switching, setSwitching] = useState<number | null>(null)
  const toggle = async (connectionId: number, enabled: boolean) => {
    if (switching !== null) return
    setSwitching(connectionId)
    try {
      const refresh = () =>
        void client.invalidateQueries({ queryKey: channelsQueryKeys.all })
      if (enabled) await handleEnableChannel(connectionId, client, refresh)
      else await handleDisableChannel(connectionId, client, refresh)
    } finally {
      setSwitching(null)
    }
  }
  return { switching, toggle }
}
