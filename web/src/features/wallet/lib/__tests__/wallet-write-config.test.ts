import { afterEach, expect, it, vi } from 'vitest'
import { api, type ApiRequestConfig } from '@/lib/api'
import * as wallet from '../../api'
import * as subscriptions from '@/features/subscriptions/api'

afterEach(() => vi.restoreAllMocks())

const commands = [
  { path: '/api/user/topup', body: { key: 'synthetic-code' }, regular: false, run: (config?: ApiRequestConfig) => wallet.redeemTopupCode({ key: 'synthetic-code' }, config) },
  { path: '/api/user/pay', body: { amount: 120, payment_method: 'alipay' }, regular: true, run: (config?: ApiRequestConfig) => wallet.requestPayment({ amount: 120, payment_method: 'alipay' }, config) },
  { path: '/api/user/stripe/pay', body: { amount: 120, payment_method: 'stripe' }, regular: true, run: (config?: ApiRequestConfig) => wallet.requestStripePayment({ amount: 120, payment_method: 'stripe' }, config) },
  { path: '/api/user/creem/pay', body: { product_id: 'synthetic-product', payment_method: 'creem' }, regular: true, run: (config?: ApiRequestConfig) => wallet.requestCreemPayment({ product_id: 'synthetic-product', payment_method: 'creem' }, config) },
  { path: '/api/user/waffo/pay', body: { amount: 120, pay_method_index: 3 }, regular: true, run: (config?: ApiRequestConfig) => wallet.requestWaffoPayment({ amount: 120, pay_method_index: 3 }, config) },
  { path: '/api/user/waffo-pancake/pay', body: { amount: 120 }, regular: true, run: (config?: ApiRequestConfig) => wallet.requestWaffoPancakePayment({ amount: 120 }, config) },
  { path: '/api/user/aff_transfer', body: { quota: 100, request_id: 'same-attempt-id' }, regular: false, run: (config?: ApiRequestConfig) => wallet.transferAffiliateQuota({ quota: 100, request_id: 'same-attempt-id' }, config) },
  { path: '/api/user/topup/complete', body: { trade_no: 'exact-order' }, regular: false, run: (config?: ApiRequestConfig) => wallet.completeOrder({ trade_no: 'exact-order' }, config) },
  ...['stripe', 'creem', 'waffo-pancake', 'balance'].map((kind) => {
    const methods = { stripe: subscriptions.paySubscriptionStripe, creem: subscriptions.paySubscriptionCreem, 'waffo-pancake': subscriptions.paySubscriptionWaffoPancake, balance: subscriptions.paySubscriptionBalance }
    const run = methods[kind as keyof typeof methods]
    return { path: `/api/subscription/${kind}/pay`, body: { plan_id: 31 }, regular: false, run: (config?: ApiRequestConfig) => run({ plan_id: 31 }, config) }
  }),
  { path: '/api/subscription/epay/pay', body: { plan_id: 31, payment_method: 'alipay' }, regular: false, run: (config?: ApiRequestConfig) => subscriptions.paySubscriptionEpay({ plan_id: 31, payment_method: 'alipay' }, config) },
]

it.each(commands)('$path keeps its original default command and cannot opt a write into auth replay', async (command) => {
  const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true } })
  await command.run()
  expect(post).toHaveBeenCalledExactlyOnceWith(command.path, command.body, command.regular ? { skipBusinessError: true, skipAuthRefresh: true } : { skipAuthRefresh: true })
  post.mockClear()
  await command.run({ skipAuthRefresh: false, skipErrorHandler: true, skipBusinessError: true })
  expect(post).toHaveBeenCalledExactlyOnceWith(command.path, command.body, { skipErrorHandler: true, skipBusinessError: true, skipAuthRefresh: true })
})

it('billing preference keeps its exact body and forced non-replay flag with optional quiet presentation', async () => {
  const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  await subscriptions.updateBillingPreference('wallet_only')
  expect(put).toHaveBeenCalledExactlyOnceWith('/api/subscription/self/preference', { billing_preference: 'wallet_only' }, { skipAuthRefresh: true })
  put.mockClear()
  await subscriptions.updateBillingPreference('wallet_only', { skipAuthRefresh: false, skipErrorHandler: true, skipBusinessError: true })
  expect(put).toHaveBeenCalledExactlyOnceWith('/api/subscription/self/preference', { billing_preference: 'wallet_only' }, { skipErrorHandler: true, skipBusinessError: true, skipAuthRefresh: true })
})

it('read wrapper defaults preserve their original endpoint and config shapes', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true } })
  await wallet.getTopupInfo()
  await wallet.getAffiliateCode()
  await wallet.getUserBillingHistory(3, 20, 'exact order')
  await wallet.getAllBillingHistory(2, 50)
  await subscriptions.getPublicPlans()
  await subscriptions.getSelfSubscriptionFull()
  expect(get.mock.calls).toEqual([
    ['/api/user/topup/info'], ['/api/user/aff'], ['/api/user/topup/self?p=3&page_size=20&keyword=exact+order'], ['/api/user/topup?p=2&page_size=50'], ['/api/subscription/plans'], ['/api/subscription/self', {}],
  ])
})

it.each([
  { path: '/api/user/amount', run: wallet.calculateAmount },
  { path: '/api/user/stripe/amount', run: wallet.calculateStripeAmount },
  { path: '/api/user/waffo/amount', run: wallet.calculateWaffoAmount },
  { path: '/api/user/waffo-pancake/amount', run: wallet.calculateWaffoPancakeAmount },
])('$path preserves the original pure quote wrapper default flags and amount body', async (quote) => {
  const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true, data: '0' } })
  await quote.run({ amount: 120 })
  expect(post).toHaveBeenCalledExactlyOnceWith(quote.path, { amount: 120 }, { skipBusinessError: true })
})
