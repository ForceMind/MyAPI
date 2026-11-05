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
import i18next from 'i18next'
import { useState, useEffect, useCallback, useRef, useLayoutEffect } from 'react'
import { toast } from 'sonner'

import { useIsAdmin } from '@/hooks/use-admin'
import { useDebounce } from '@/hooks/use-debounce'

import {
  getUserBillingHistory,
  getAllBillingHistory,
  completeOrder,
  isApiSuccess,
} from '../api'
import type { TopupRecord } from '../types'
import { useWalletSession, walletRequestConfig, walletWriteIsUnknown, walletWriteErrorMessage } from './use-wallet-session'

// ============================================================================
// Billing History Hook
// ============================================================================

interface UseBillingHistoryOptions {
  enabled?: boolean
  /** Initial page number */
  initialPage?: number
  /** Initial page size */
  initialPageSize?: number
}

export function useBillingHistory(options: UseBillingHistoryOptions = {}) {
  const { initialPage = 1, initialPageSize = 10 } = options
  const isAdmin = useIsAdmin()
  const isCurrent = useWalletSession()
  const enabled = options.enabled !== false
  const readActive = useRef(enabled)
  const opening = useRef<object>({})

  const [records, setRecords] = useState<TopupRecord[]>([])
  const [total, setTotal] = useState<number | undefined>(undefined)
  const [page, setPage] = useState(initialPage)
  const [pageSize, setPageSize] = useState(initialPageSize)
  const [keyword, setKeyword] = useState('')
  const debouncedKeyword = useDebounce(keyword)
  const requestIdRef = useRef(0)
  const completionFlight = useRef(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [completing, setCompleting] = useState(false)
  const [completionUnknown, setCompletionUnknown] = useState(false)
  const [completionFailure, setCompletionFailure] = useState<string | null>(null)

  useLayoutEffect(() => {
    opening.current = {}
    readActive.current = enabled
    if (!enabled) requestIdRef.current += 1
  }, [enabled])

  /**
   * Fetch billing history
   */
  const fetchBillingHistory = useCallback(async () => {
    if (!isCurrent() || !enabled || !readActive.current) return
    const requestId = ++requestIdRef.current
    setLoading(true)
    setError(null)
    try {
      const response = isAdmin
        ? await getAllBillingHistory(page, pageSize, debouncedKeyword, walletRequestConfig(() => isCurrent() && readActive.current && requestId === requestIdRef.current))
        : await getUserBillingHistory(page, pageSize, debouncedKeyword, walletRequestConfig(() => isCurrent() && readActive.current && requestId === requestIdRef.current))

      if (!isCurrent() || !readActive.current || requestId !== requestIdRef.current) return

      if (isApiSuccess(response) && response.data) {
        const items = response.data.items === null && response.data.total === 0
          ? []
          : response.data.items
        if (!Array.isArray(items)) {
          throw new Error(i18next.t('Invalid billing history response'))
        }
        const count = response.data.total
        setRecords(items)
        setTotal(
          typeof count === 'number' && Number.isSafeInteger(count) && count >= 0
            ? count
            : undefined
        )
      } else {
        toast.error(
          response.message || i18next.t('Failed to load billing history')
        )
        setRecords([])
        setTotal(undefined)
        setError(response.message || i18next.t('Failed to load billing history'))
      }
    } catch (error) {
      if (!isCurrent() || !readActive.current || requestId !== requestIdRef.current) return

      // eslint-disable-next-line no-console
      console.error('Failed to fetch billing history:', error)
      toast.error(i18next.t('Failed to load billing history'))
      setRecords([])
      setTotal(undefined)
      setError(error instanceof Error ? error.message : i18next.t('Failed to load billing history'))
    } finally {
      if (isCurrent() && readActive.current && requestId === requestIdRef.current) {
        setLoading(false)
      }
    }
  }, [debouncedKeyword, enabled, isAdmin, isCurrent, page, pageSize])

  /**
   * Complete a pending order (admin only)
   */
  const handleCompleteOrder = useCallback(
    async (tradeNo: string) => {
      if (!isCurrent() || !enabled || !readActive.current || completionFlight.current) return false
      if (!isAdmin) {
        toast.error(i18next.t('Admin access required'))
        return false
      }

      const operation = opening.current
      completionFlight.current = true
      setCompletionUnknown(false)
      setCompletionFailure(null)
      setCompleting(true)
      try {
        const response = await completeOrder({ trade_no: tradeNo }, walletRequestConfig(() => isCurrent() && readActive.current && operation === opening.current))
        if (!isCurrent() || !readActive.current || operation !== opening.current) return false
        if (isApiSuccess(response)) {
          toast.success(i18next.t('Order completed successfully'))
          // Refresh the list
          await fetchBillingHistory()
          return isCurrent() && readActive.current
        } else {
          const message = response.message || i18next.t('Failed to complete order')
          setCompletionFailure(message)
          toast.error(message)
          return false
        }
      } catch (error) {
        if (!isCurrent() || !readActive.current || operation !== opening.current) return false
        const unknown = walletWriteIsUnknown(error)
        const fallback = unknown ? i18next.t('Payment request outcome is unknown. Check billing history before trying again.') : i18next.t('Failed to complete order')
        const message = walletWriteErrorMessage(error, fallback)
        setCompletionUnknown(unknown)
        setCompletionFailure(message)
        toast.error(message)
        return false
      } finally {
        completionFlight.current = false
        if (isCurrent()) {
          setCompleting(false)
          if (operation !== opening.current) setCompletionUnknown(true)
        }
      }
    },
    [enabled, isAdmin, fetchBillingHistory, isCurrent]
  )

  /**
   * Change page
   */
  const handlePageChange = useCallback((newPage: number) => {
    setPage(newPage)
  }, [])

  /**
   * Change page size
   */
  const handlePageSizeChange = useCallback((newPageSize: number) => {
    setPageSize(newPageSize)
    setPage(1) // Reset to first page when changing page size
  }, [])

  /**
   * Search by keyword
   */
  const handleSearch = useCallback((newKeyword: string) => {
    requestIdRef.current += 1
    setKeyword(newKeyword)
    setPage(1) // Reset to first page when searching
  }, [])

  // Fetch data after the search draft has settled.
  useEffect(() => {
    if (!enabled || keyword !== debouncedKeyword) return

    fetchBillingHistory()
  }, [debouncedKeyword, enabled, fetchBillingHistory, keyword])

  return {
    records,
    total,
    page,
    pageSize,
    keyword,
    loading,
    error,
    completing,
    completionUnknown,
    completionFailure,
    isAdmin,
    handlePageChange,
    handlePageSizeChange,
    handleSearch,
    handleCompleteOrder,
    refresh: fetchBillingHistory,
  }
}
