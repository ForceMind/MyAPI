/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useCallback, useEffect, useRef, useState } from 'react'

import { getDeploymentSettings, testDeploymentConnection } from '../api'

interface ConnectionState {
  loading: boolean
  ok: boolean | null
  error: string | null
}

// Connection cache (5 minutes TTL)
const CONNECTION_CACHE_TTL = 5 * 60 * 1000
interface CachedConnection {
  ok: boolean
  error: string | null
  timestamp: number
}
let connectionCache: CachedConnection | null = null

function getCachedConnection(): CachedConnection | null {
  if (!connectionCache) return null
  if (Date.now() - connectionCache.timestamp > CONNECTION_CACHE_TTL) {
    connectionCache = null
    return null
  }
  return connectionCache
}

function setCachedConnection(ok: boolean, error: string | null = null) {
  connectionCache = { ok, error, timestamp: Date.now() }
}

export function clearConnectionCache() {
  connectionCache = null
}

type LoadingPhase = 'idle' | 'settings' | 'connection' | 'done'

export function useModelDeploymentSettings(active = true) {
  const [loading, setLoading] = useState(true)
  const [loadingPhase, setLoadingPhase] = useState<LoadingPhase>('settings')
  const [settingsError, setSettingsError] = useState<string | null>(null)
  const [settings, setSettings] = useState<Record<string, unknown>>({
    'model_deployment.ionet.enabled': false,
  })
  const [connectionState, setConnectionState] = useState<ConnectionState>({
    loading: false,
    ok: null,
    error: null,
  })
  const activeRef = useRef(false)
  const requestGenerationRef = useRef(0)

  // Load settings before checking the connection when enabled.
  const fetchAll = useCallback(async (useCache = true) => {
    if (!activeRef.current) return
    const generation = ++requestGenerationRef.current
    setLoading(true)
    setSettingsError(null)
    setLoadingPhase('settings')

    try {
      // Step 1: Fetch settings first (usually fast)
      const response = await getDeploymentSettings()
      if (generation !== requestGenerationRef.current) return
      if (!response?.success) throw new Error('Unable to load settings')
      const isEnabled = response?.success && response?.data?.enabled === true

      setSettings({
        'model_deployment.ionet.enabled': isEnabled,
      })

      if (!isEnabled) {
        // Not enabled, done
        setConnectionState({ loading: false, ok: null, error: null })
        setLoadingPhase('done')
        setLoading(false)
        return
      }

      // Step 2: Check connection (check cache first)
      if (useCache) {
        const cached = getCachedConnection()
        if (cached !== null) {
          setConnectionState({
            loading: false,
            ok: cached.ok,
            error: cached.error,
          })
          setLoadingPhase('done')
          setLoading(false)
          return
        }
      }

      // Test connection
      setLoadingPhase('connection')
      setConnectionState({ loading: true, ok: null, error: null })

      try {
        const connResponse = await testDeploymentConnection()
        if (generation !== requestGenerationRef.current) return
        if (connResponse?.success) {
          setCachedConnection(true)
          setConnectionState({ loading: false, ok: true, error: null })
        } else {
          const message = connResponse?.message || 'Connection failed'
          setCachedConnection(false, message)
          setConnectionState({ loading: false, ok: false, error: message })
        }
      } catch (error: unknown) {
        if (generation !== requestGenerationRef.current) return
        const errMsg =
          error instanceof Error ? error.message : 'Connection failed'
        setCachedConnection(false, errMsg)
        setConnectionState({ loading: false, ok: false, error: errMsg })
      }
    } catch {
      if (generation !== requestGenerationRef.current) return
      setSettingsError('Unable to load settings')
      setSettings({ 'model_deployment.ionet.enabled': false })
      setConnectionState({ loading: false, ok: null, error: null })
    } finally {
      if (generation === requestGenerationRef.current) {
        setLoadingPhase('done')
        setLoading(false)
      }
    }
  }, [])

  // Each visit owns its requests; leaving also invalidates pending retries.
  useEffect(() => {
    activeRef.current = active
    if (active) void fetchAll(true)

    return () => {
      activeRef.current = false
      requestGenerationRef.current += 1
    }
  }, [active, fetchAll])

  const isIoNetEnabled = Boolean(settings['model_deployment.ionet.enabled'])

  // Manual retry (skip cache)
  const testConnection = useCallback(async () => {
    if (!activeRef.current) return
    const generation = ++requestGenerationRef.current
    clearConnectionCache()
    setConnectionState({ loading: true, ok: null, error: null })
    setLoadingPhase('connection')

    try {
      const response = await testDeploymentConnection()
      if (generation !== requestGenerationRef.current) return
      if (response?.success) {
        setCachedConnection(true)
        setConnectionState({ loading: false, ok: true, error: null })
        return
      }
      const message = response?.message || 'Connection failed'
      setCachedConnection(false, message)
      setConnectionState({ loading: false, ok: false, error: message })
    } catch (error: unknown) {
      if (generation !== requestGenerationRef.current) return
      const errMsg =
        error instanceof Error ? error.message : 'Connection failed'
      setCachedConnection(false, errMsg)
      setConnectionState({ loading: false, ok: false, error: errMsg })
    } finally {
      if (generation === requestGenerationRef.current) {
        setLoadingPhase('done')
        setLoading(false)
      }
    }
  }, [])

  // Refresh all (skip cache)
  const refresh = useCallback(() => {
    if (!activeRef.current) return
    clearConnectionCache()
    return fetchAll(false)
  }, [fetchAll])

  // Refresh on window focus (useful after saving settings in another page)
  useEffect(() => {
    if (!active) return
    const handler = () => {
      // Use cache on focus to avoid unnecessary requests
      fetchAll(true)
    }
    window.addEventListener('focus', handler)
    return () => window.removeEventListener('focus', handler)
  }, [active, fetchAll])

  return {
    loading,
    loadingPhase,
    settingsError,
    settings,
    isIoNetEnabled,
    refresh,
    connectionLoading: connectionState.loading,
    connectionOk: connectionState.ok,
    connectionError: connectionState.error,
    testConnection,
  }
}
