/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQueryClient } from '@tanstack/react-query'
import { create } from 'zustand'

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

type ConnectionTestState = {
  /** The signed-in identity the running test and the result belong to. */
  identity: string | null
  running: number | null
  result: ConnectionTestResult | null
}

// Outside the page so a test started on the list ("Save and test") is still
// shown on the connection page it opens, and only for the same identity.
const useConnectionTestStore = create<ConnectionTestState>()(() => ({
  identity: null,
  running: null,
  result: null,
}))

/**
 * Runs the existing connectivity test for one connection. It contacts the
 * provider, so it only runs on an explicit click (or "Save and test"); the
 * result is shown inline and announced. One test runs at a time.
 */
export function useConnectionTest(identity: string) {
  const client = useQueryClient()
  const state = useConnectionTestStore()
  const own = state.identity === identity
  const run = async (item: Pick<ConnectionSummary, 'id' | 'name'>) => {
    const current = useConnectionTestStore.getState()
    if (current.running !== null && current.identity === identity) return
    useConnectionTestStore.setState({
      identity,
      running: item.id,
      result: null,
    })
    try {
      await handleTestChannel(
        item.id,
        { channelName: item.name, silent: true },
        (success, responseTime, error, errorCode) => {
          if (useConnectionTestStore.getState().identity !== identity) return
          useConnectionTestStore.setState({
            result: {
              connectionId: item.id,
              success,
              responseTime,
              error,
              errorCode,
            },
          })
        }
      )
      void client.invalidateQueries({ queryKey: channelsQueryKeys.all })
    } finally {
      if (useConnectionTestStore.getState().identity === identity) {
        useConnectionTestStore.setState({ running: null })
      }
    }
  }
  return {
    running: own ? state.running : null,
    result: own ? state.result : null,
    run,
  }
}
