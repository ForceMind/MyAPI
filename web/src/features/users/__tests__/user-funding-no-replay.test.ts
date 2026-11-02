import axios, { AxiosError, type AxiosAdapter, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AuthBundle } from '@/stores/auth-store'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))
// Keep both real Axios instances and their interceptors; replace only transport.
let transport: AxiosAdapter
const adapter: AxiosAdapter = (config) => transport(config)
const defaultAdapter = axios.defaults.adapter
axios.defaults.adapter = adapter
const { useAuthStore } = await import('@/stores/auth-store')
const users = await import('../api')
const subs = await import('@/features/subscriptions/api')
const wallet = await import('@/features/wallet/api')
const redemptions = await import('@/features/redemption-codes/api')
axios.defaults.adapter = defaultAdapter

const originalAuth = useAuthStore.getState()
let writes: InternalAxiosRequestConfig[]
let refreshes: InternalAxiosRequestConfig[]
let failureStatus: number
let target: string

function bundle(token: string): AuthBundle {
  return { access_token: token, token_type: 'Bearer', access_expires_at: 4_000_000_000,
    user: { id: 1, role: 100, username: 'synthetic-maintainer' },
    session: { sid: 'synthetic-same-session', current: true, login_method: 'password', ip: '127.0.0.1', user_agent: 'test', created_at: 1, last_active_at: 1, expires_at: 4_000_000_000 } }
}
function response(config: InternalAxiosRequestConfig, data: unknown): AxiosResponse {
  return { config, data, status: 200, statusText: 'OK', headers: {} }
}
const userPayload = { username: 'synthetic-user', display_name: 'Synthetic user', password: 'synthetic-not-a-real-password', role: 1 }
const planPayload = { plan: { title: 'Synthetic plan', price_amount: 10, total_amount: 0, enabled: false } }
const redemptionPayload = { name: 'Synthetic code', quota: 500000, expired_time: 0, count: 1 }
const cases = [
  { name: 'create user', method: 'post', url: '/api/user/', body: userPayload, run: () => users.createUser(userPayload) },
  { name: 'update user metadata', method: 'put', url: '/api/user/', body: { id: 22, username: 'synthetic-user', display_name: 'Synthetic user', group: 'default', remark: '' }, run: () => users.updateUser({ id: 22, username: 'synthetic-user', display_name: 'Synthetic user', group: 'default', remark: '' }) },
  { name: 'delete user', method: 'delete', url: '/api/user/22/', body: undefined, run: () => users.deleteUser(22) },
  { name: 'change user status', method: 'post', url: '/api/user/manage', body: { id: 22, action: 'disable' }, run: () => users.manageUser(22, 'disable') },
  { name: 'adjust user quota with original request identity', method: 'post', url: '/api/user/manage', body: { id: 22, action: 'add_quota', mode: 'override', value: 0, request_id: 'synthetic-stable-quota-attempt' }, run: () => users.adjustUserQuota({ id: 22, action: 'add_quota', mode: 'override', value: 0, request_id: 'synthetic-stable-quota-attempt' }) },
  { name: 'reset user Passkey', method: 'delete', url: '/api/user/22/reset_passkey', body: undefined, run: () => users.resetUserPasskey(22) },
  { name: 'reset user 2FA', method: 'delete', url: '/api/user/22/2fa', body: undefined, run: () => users.resetUserTwoFA(22) },
  { name: 'clear selected built-in binding', method: 'delete', url: '/api/user/22/bindings/email', body: undefined, run: () => users.adminClearUserBinding(22, 'email') },
  { name: 'unbind selected custom OAuth binding', method: 'delete', url: '/api/user/22/oauth/bindings/7', body: undefined, run: () => users.adminUnbindCustomOAuth(22, 7) },
  { name: 'create subscription plan', method: 'post', url: '/api/subscription/admin/plans', body: planPayload, run: () => subs.createPlan(planPayload) },
  { name: 'update subscription plan', method: 'put', url: '/api/subscription/admin/plans/17', body: planPayload, run: () => subs.updatePlan(17, planPayload) },
  { name: 'set plan explicit disabled', method: 'patch', url: '/api/subscription/admin/plans/17', body: { enabled: false }, run: () => subs.patchPlanStatus(17, false) },
  { name: 'grant user subscription', method: 'post', url: '/api/subscription/admin/users/22/subscriptions', body: { plan_id: 17 }, run: () => subs.createUserSubscription(22, { plan_id: 17 }) },
  { name: 'invalidate user subscription', method: 'post', url: '/api/subscription/admin/user_subscriptions/31/invalidate', body: undefined, run: () => subs.invalidateUserSubscription(31) },
  { name: 'delete user subscription', method: 'delete', url: '/api/subscription/admin/user_subscriptions/31', body: undefined, run: () => subs.deleteUserSubscription(31) },
  { name: 'reset selected user plan without advancing time', method: 'post', url: '/api/subscription/admin/users/22/subscriptions/reset', body: { plan_id: 17, advance_reset_time: false }, run: () => subs.resetUserSubscriptionsByPlan(22, { plan_id: 17, advance_reset_time: false }) },
  { name: 'reset selected plan subscriptions', method: 'post', url: '/api/subscription/admin/plans/17/subscriptions/reset', body: { advance_reset_time: true }, run: () => subs.resetPlanSubscriptions(17, { advance_reset_time: true }) },
  { name: 'pay Stripe subscription', method: 'post', url: '/api/subscription/stripe/pay', body: { plan_id: 17 }, run: () => subs.paySubscriptionStripe({ plan_id: 17 }) },
  { name: 'pay Creem subscription', method: 'post', url: '/api/subscription/creem/pay', body: { plan_id: 17 }, run: () => subs.paySubscriptionCreem({ plan_id: 17 }) },
  { name: 'pay Pancake subscription', method: 'post', url: '/api/subscription/waffo-pancake/pay', body: { plan_id: 17 }, run: () => subs.paySubscriptionWaffoPancake({ plan_id: 17 }) },
  { name: 'pay subscription from balance', method: 'post', url: '/api/subscription/balance/pay', body: { plan_id: 17 }, run: () => subs.paySubscriptionBalance({ plan_id: 17 }) },
  { name: 'pay Epay subscription', method: 'post', url: '/api/subscription/epay/pay', body: { plan_id: 17, payment_method: 'synthetic-method' }, run: () => subs.paySubscriptionEpay({ plan_id: 17, payment_method: 'synthetic-method' }) },
  { name: 'create external plan product', method: 'post', url: '/api/option/waffo-pancake/subscription-product', body: { name: 'Synthetic product', amount: '10.00' }, run: () => subs.createWaffoPancakeSubscriptionProduct({ name: 'Synthetic product', amount: '10.00' }) },
  { name: 'save billing preference', method: 'put', url: '/api/subscription/self/preference', body: { billing_preference: 'subscription_first' }, run: () => subs.updateBillingPreference('subscription_first') },
  { name: 'create redemption code', method: 'post', url: '/api/redemption/', body: redemptionPayload, run: () => redemptions.createRedemption(redemptionPayload) },
  { name: 'update redemption code', method: 'put', url: '/api/redemption/', body: { id: 18, ...redemptionPayload }, run: () => redemptions.updateRedemption({ id: 18, ...redemptionPayload }) },
  { name: 'set redemption explicit disabled', method: 'put', url: '/api/redemption/?status_only=true', body: { id: 18, status: 2 }, run: () => redemptions.updateRedemptionStatus(18, 2) },
  { name: 'delete selected redemption', method: 'delete', url: '/api/redemption/18/', body: undefined, run: () => redemptions.deleteRedemption(18) },
  { name: 'delete invalid redemptions', method: 'delete', url: '/api/redemption/invalid', body: undefined, run: () => redemptions.deleteInvalidRedemptions() },
  { name: 'redeem topup code', method: 'post', url: '/api/user/topup', body: { key: 'synthetic-non-redeemable-code' }, run: () => wallet.redeemTopupCode({ key: 'synthetic-non-redeemable-code' }) },
  { name: 'request regular topup', method: 'post', url: '/api/user/pay', body: { amount: 10, payment_method: 'synthetic-method' }, run: () => wallet.requestPayment({ amount: 10, payment_method: 'synthetic-method' }) },
  { name: 'request Stripe topup', method: 'post', url: '/api/user/stripe/pay', body: { amount: 10, payment_method: 'stripe' }, run: () => wallet.requestStripePayment({ amount: 10, payment_method: 'stripe' }) },
  { name: 'request Creem topup', method: 'post', url: '/api/user/creem/pay', body: { product_id: 'synthetic-product', payment_method: 'creem' }, run: () => wallet.requestCreemPayment({ product_id: 'synthetic-product', payment_method: 'creem' }) },
  { name: 'request Waffo topup', method: 'post', url: '/api/user/waffo/pay', body: { amount: 10, pay_method_index: 0 }, run: () => wallet.requestWaffoPayment({ amount: 10, pay_method_index: 0 }) },
  { name: 'request Pancake topup', method: 'post', url: '/api/user/waffo-pancake/pay', body: { amount: 10 }, run: () => wallet.requestWaffoPancakePayment({ amount: 10 }) },
  { name: 'transfer affiliate quota with original request identity', method: 'post', url: '/api/user/aff_transfer', body: { quota: 500000, request_id: 'synthetic-stable-transfer-attempt' }, run: () => wallet.transferAffiliateQuota({ quota: 500000, request_id: 'synthetic-stable-transfer-attempt' }) },
  { name: 'complete precisely selected order', method: 'post', url: '/api/user/topup/complete', body: { trade_no: 'synthetic-exact-order' }, run: () => wallet.completeOrder({ trade_no: 'synthetic-exact-order' }) },
]


beforeEach(() => {
  localStorage.clear()
  writes = []
  refreshes = []
  failureStatus = 401
  useAuthStore.getState().auth.setBundle(bundle('synthetic-original-access'))
  transport = async (config) => {
    if (config.url === '/api/user/auth/refresh') {
      refreshes.push(config)
      return response(config, { success: true, data: bundle('synthetic-refreshed-access') })
    }
    if (config.url !== target || config.method === 'get') throw new Error(`Unexpected request ${config.method} ${config.url}`)
    writes.push(config)
    if (writes.length === 1 && failureStatus !== 0) {
      throw new AxiosError('Synthetic rejected write', AxiosError.ERR_BAD_REQUEST, config, undefined, { config, data: { success: false }, status: failureStatus, statusText: 'Rejected', headers: {} })
    }
    return response(config, { success: true })
  }
})
afterEach(() => { useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })

describe('user and funding writes do not replay', () => {
  it.each(cases)('$name rejects 401 after one dispatch without authentication refresh or write replay', async (testCase) => {
    target = testCase.url
    await expect(testCase.run()).rejects.toMatchObject({ response: { status: 401 } })
    expect(writes).toHaveLength(1)
    expect(refreshes).toHaveLength(0)
    expect(writes[0].method).toBe(testCase.method)
    expect(writes[0].data === undefined ? undefined : JSON.parse(writes[0].data)).toEqual(testCase.body)
  })
  it.each(cases)('$name rejects 403 once without replay', async (testCase) => {
    target = testCase.url
    failureStatus = 403
    await expect(testCase.run()).rejects.toMatchObject({ response: { status: 403 } })
    expect(writes).toHaveLength(1)
    expect(refreshes).toHaveLength(0)
  })
  it.each(cases)('$name preserves successful method and exact payload', async (testCase) => {
    target = testCase.url
    failureStatus = 0
    await expect(testCase.run()).resolves.toMatchObject({ success: true })
    expect(writes).toHaveLength(1)
    expect(writes[0].method).toBe(testCase.method)
    expect(writes[0].data === undefined ? undefined : JSON.parse(writes[0].data)).toEqual(testCase.body)
    expect(refreshes).toHaveLength(0)
  })
})

it.each([
  { url: '/api/user/amount', run: () => wallet.calculateAmount({ amount: 10 }) },
  { url: '/api/user/stripe/amount', run: () => wallet.calculateStripeAmount({ amount: 10 }) },
  { url: '/api/user/waffo/amount', run: () => wallet.calculateWaffoAmount({ amount: 10 }) },
  { url: '/api/user/waffo-pancake/amount', run: () => wallet.calculateWaffoPancakeAmount({ amount: 10 }) },
])('preserves authentication refresh for the read-only estimate at $url', async (testCase) => {
  target = testCase.url
  await expect(testCase.run()).resolves.toMatchObject({ success: true })
  expect(writes).toHaveLength(2)
  expect(refreshes).toHaveLength(1)
  expect(writes.map((config) => JSON.parse(config.data))).toEqual([{ amount: 10 }, { amount: 10 }])
})
