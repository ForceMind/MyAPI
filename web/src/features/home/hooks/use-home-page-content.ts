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
import { useCallback, useEffect, useState } from 'react'

import { isHttpUrl } from '@/lib/content-format'

import { getHomePageContent } from '../api'
import type { HomePageContentResult } from '../types'

const STORAGE_KEY = 'home_page_content'

/**
 * Hook to load and manage custom home page content
 * Supports both Markdown/HTML content and iframe URLs
 */
export function useHomePageContent(): HomePageContentResult {
  const [content, setContent] = useState<string>('')
  const [isLoaded, setIsLoaded] = useState(false)
  const [failed, setFailed] = useState(false)
  const [retrying, setRetrying] = useState(false)
  const [requestVersion, setRequestVersion] = useState(0)
  const retry = useCallback(
    () => setRequestVersion((version) => version + 1),
    []
  )

  useEffect(() => {
    let mounted = true

    const loadContent = async () => {
      if (requestVersion > 0) setRetrying(true)
      // Browser storage is an optional cache, not a prerequisite for the
      // configured home page or the default self-hosted entry point.
      let cached: string | null = null
      if (requestVersion === 0) {
        try {
          cached = localStorage.getItem(STORAGE_KEY)
        } catch {
          // Private or restricted browsers may deny access to localStorage.
        }
        if (cached && mounted) {
          setContent(cached)
        }
      }

      try {
        const response = await getHomePageContent()
        if (!mounted) return

        if (
          response?.success &&
          typeof response.data === 'string' &&
          response.data
        ) {
          setContent(response.data)
          setFailed(false)
          try {
            localStorage.setItem(STORAGE_KEY, response.data)
          } catch {
            // The server result remains valid without a browser cache.
          }
        } else {
          // Missing or malformed server responses are not an intentional
          // empty custom home. Never publish stale cached administrator HTML.
          setContent('')
          setFailed(!(response?.success && response.data === ''))
          try {
            localStorage.removeItem(STORAGE_KEY)
          } catch {
            // Cache cleanup failure must not replace the default home.
          }
        }
      } catch {
        if (!mounted) return
        // A failed authoritative read must not publish an old administrator
        // URL or HTML page from this browser's optional cache.
        setContent('')
        setFailed(true)
        try {
          localStorage.removeItem(STORAGE_KEY)
        } catch {
          // Restricted storage must not prevent the safe default home.
        }
      } finally {
        if (mounted) {
          setIsLoaded(true)
          setRetrying(false)
        }
      }
    }

    void loadContent()

    return () => {
      mounted = false
    }
  }, [requestVersion])

  const isUrl = isHttpUrl(content)

  return { content, isLoaded, isUrl, failed, retrying, retry }
}
