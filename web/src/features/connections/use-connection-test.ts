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
  handleTestChannel,
} from '@/features/channels/lib/channel-actions'

import type { ConnectionSummary } from './connection-state'

export type ConnectionTestResult = {
  connectionId: number
  success: boolean
  /** Milliseconds, when the server reported one. */
  responseTime?: number
  error?: string
  errorCode?: string
}

/**
 * Runs the existing connectivity test for one connection. It contacts the
 * provider, so it only runs on an explicit click; the result is shown inline
 * and announced.
 */
export function useConnectionTest(identity: string) {
  const client = useQueryClient()
  const [running, setRunning] = useState<number | null>(null)
  const [result, setResult] = useState<ConnectionTestResult | null>(null)
  const run = async (item: Pick<ConnectionSummary, 'id' | 'name'>) => {
    if (running !== null) return
    setRunning(item.id)
    setResult(null)
    try {
      await handleTestChannel(
        item.id,
        { channelName: item.name, silent: true },
        (success, responseTime, error, errorCode) => {
          setResult({
            connectionId: item.id,
            success,
            responseTime,
            error,
            errorCode,
          })
        }
      )
      void client.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
    } finally {
      setRunning(null)
    }
  }
  return { running, result, run, identity }
}
