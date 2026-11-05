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
import { api, type ApiRequestConfig } from '@/lib/api'

import type {
  RedemptionRequest,
  PaymentRequest,
  AmountRequest,
  AffiliateTransferRequest,
  ApiResponse,
  TopupInfoResponse,
  RedemptionResponse,
  AmountResponse,
  PaymentResponse,
  StripePaymentResponse,
  AffiliateCodeResponse,
  AffiliateTransferResponse,
  BillingHistoryResponse,
  CompleteOrderRequest,
  CreemPaymentRequest,
  CreemPaymentResponse,
  WaffoPaymentRequest,
  WaffoPaymentResponse,
  WaffoPancakePaymentRequest,
  WaffoPancakePaymentResponse,
} from './types'

// ============================================================================
// Wallet API Functions
// ============================================================================

/**
 * Check if API response is successful
 */
export function isApiSuccess(response: ApiResponse): boolean {
  return response.success === true || response.message === 'success'
}

/**
 * Get topup configuration info
 */
export async function getTopupInfo(config?: ApiRequestConfig): Promise<TopupInfoResponse> {
  const res = config ? await api.get('/api/user/topup/info', config) : await api.get('/api/user/topup/info')
  return res.data
}

/**
 * Redeem a topup code
 */
export async function redeemTopupCode(
  request: RedemptionRequest,
  config?: ApiRequestConfig
): Promise<RedemptionResponse> {
  const res = await api.post('/api/user/topup', request, { ...config, skipAuthRefresh: true })
  return res.data
}

/**
 * Calculate payment amount for regular payment
 */
export async function calculateAmount(
  request: AmountRequest,
  config?: ApiRequestConfig
): Promise<AmountResponse> {
  const res = await api.post('/api/user/amount', request, {
    skipBusinessError: true,
    ...config,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Calculate payment amount for Stripe payment
 */
export async function calculateStripeAmount(
  request: AmountRequest,
  config?: ApiRequestConfig
): Promise<AmountResponse> {
  const res = await api.post('/api/user/stripe/amount', request, {
    skipBusinessError: true,
    ...config,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Calculate payment amount for Waffo payment
 */
export async function calculateWaffoAmount(
  request: AmountRequest,
  config?: ApiRequestConfig
): Promise<AmountResponse> {
  const res = await api.post('/api/user/waffo/amount', request, {
    skipBusinessError: true,
    ...config,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Request regular payment
 */
export async function requestPayment(
  request: PaymentRequest,
  config?: ApiRequestConfig
): Promise<PaymentResponse> {
  const res = await api.post('/api/user/pay', request, {
    skipBusinessError: true,
    ...config,
    skipAuthRefresh: true,
  } as Record<string, unknown>)
  return {
    ...res.data,
    url: res.data.url || (res as unknown as { url?: string }).url,
  }
}

/**
 * Request Stripe payment
 */
export async function requestStripePayment(
  request: PaymentRequest,
  config?: ApiRequestConfig
): Promise<StripePaymentResponse> {
  const res = await api.post('/api/user/stripe/pay', request, {
    skipBusinessError: true,
    ...config,
    skipAuthRefresh: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Request Creem payment
 */
export async function requestCreemPayment(
  request: CreemPaymentRequest,
  config?: ApiRequestConfig
): Promise<CreemPaymentResponse> {
  const res = await api.post('/api/user/creem/pay', request, {
    skipBusinessError: true,
    ...config,
    skipAuthRefresh: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Request Waffo payment
 */
export async function requestWaffoPayment(
  request: WaffoPaymentRequest,
  config?: ApiRequestConfig
): Promise<WaffoPaymentResponse> {
  const res = await api.post('/api/user/waffo/pay', request, {
    skipBusinessError: true,
    ...config,
    skipAuthRefresh: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Calculate payment amount for Waffo Pancake payment
 */
export async function calculateWaffoPancakeAmount(
  request: AmountRequest,
  config?: ApiRequestConfig
): Promise<AmountResponse> {
  const res = await api.post('/api/user/waffo-pancake/amount', request, {
    skipBusinessError: true,
    ...config,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Request Waffo Pancake payment
 */
export async function requestWaffoPancakePayment(
  request: WaffoPancakePaymentRequest,
  config?: ApiRequestConfig
): Promise<WaffoPancakePaymentResponse> {
  const res = await api.post('/api/user/waffo-pancake/pay', request, {
    skipBusinessError: true,
    ...config,
    skipAuthRefresh: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Get affiliate code
 */
export async function getAffiliateCode(config?: ApiRequestConfig): Promise<AffiliateCodeResponse> {
  const res = config ? await api.get('/api/user/aff', config) : await api.get('/api/user/aff')
  return res.data
}

/**
 * Transfer affiliate quota to balance
 */
export async function transferAffiliateQuota(
  request: AffiliateTransferRequest,
  config?: ApiRequestConfig
): Promise<AffiliateTransferResponse> {
  const res = await api.post('/api/user/aff_transfer', request, { ...config, skipAuthRefresh: true })
  return res.data
}

/**
 * Get billing history for current user
 */
export async function getUserBillingHistory(
  page: number,
  pageSize: number,
  keyword?: string,
  config?: ApiRequestConfig
): Promise<ApiResponse<BillingHistoryResponse>> {
  const params = new URLSearchParams({
    p: page.toString(),
    page_size: pageSize.toString(),
  })
  if (keyword) {
    params.append('keyword', keyword)
  }
  const res = config ? await api.get(`/api/user/topup/self?${params.toString()}`, config) : await api.get(`/api/user/topup/self?${params.toString()}`)
  return res.data
}

/**
 * Get billing history for all users (admin only)
 */
export async function getAllBillingHistory(
  page: number,
  pageSize: number,
  keyword?: string,
  config?: ApiRequestConfig
): Promise<ApiResponse<BillingHistoryResponse>> {
  const params = new URLSearchParams({
    p: page.toString(),
    page_size: pageSize.toString(),
  })
  if (keyword) {
    params.append('keyword', keyword)
  }
  const res = config ? await api.get(`/api/user/topup?${params.toString()}`, config) : await api.get(`/api/user/topup?${params.toString()}`)
  return res.data
}

/**
 * Complete a pending order (admin only)
 */
export async function completeOrder(
  request: CompleteOrderRequest,
  config?: ApiRequestConfig
): Promise<ApiResponse> {
  const res = await api.post('/api/user/topup/complete', request, { ...config, skipAuthRefresh: true })
  return res.data
}


/** The wallet reads only its current account; default options match the existing self read. */
export async function getWalletAccount(config?: ApiRequestConfig): Promise<ApiResponse> {
  const res = await api.get('/api/user/self', { skipErrorHandler: true, ...config })
  return res.data
}
