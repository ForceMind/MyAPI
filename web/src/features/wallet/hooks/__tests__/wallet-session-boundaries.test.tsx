import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { useAffiliate } from '../use-affiliate'
import { useCreemPayment } from '../use-creem-payment'
import { usePayment } from '../use-payment'
import { useRedemption } from '../use-redemption'
import { useWaffoPayment } from '../use-waffo-payment'
import { useWaffoPancakePayment } from '../use-waffo-pancake-payment'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))
const originalAuth = useAuthStore.getState()
let sequence = 0
function signIn(id = 1, sid = `wallet-original-${++sequence}`) {
  useAuthStore.setState((state) => ({ auth: { ...state.auth, user: { id, role: 100, username: `user-${id}` }, session: { sid, current: true, login_method: 'password', ip: '', user_agent: '', created_at: 1, last_active_at: 1, expires_at: 4000000000 } } }))
}
function deferred() {
  let resolve!: (value: { data: unknown }) => void
  const promise = new Promise<{ data: unknown }>((accept) => { resolve = accept })
  return { promise, resolve }
}
beforeEach(() => {
  signIn()
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: 'synthetic-affiliate' } })
  vi.spyOn(window, 'open').mockReturnValue(null)
  vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(() => {})
  vi.mocked(toast.success).mockClear()
  vi.mocked(toast.error).mockClear()
})
afterEach(() => { cleanup(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

const flows = [
  { name: 'Stripe', useFlow: () => { const flow = usePayment(); return () => flow.processPayment(120, 'stripe') }, data: { success: true, data: { pay_link: 'https://pay.synthetic.invalid' } } },
  { name: 'form payment', useFlow: () => { const flow = usePayment(); return () => flow.processPayment(120, 'alipay') }, data: { success: true, url: 'https://pay.synthetic.invalid', data: { amount: '120' } } },
  { name: 'Creem', useFlow: () => { const flow = useCreemPayment(); return () => flow.processCreemPayment('synthetic-product') }, data: { success: true, data: { checkout_url: 'https://pay.synthetic.invalid' } } },
  { name: 'Waffo', useFlow: () => { const flow = useWaffoPayment(); return () => flow.processWaffoPayment(120, 3) }, data: { success: true, data: { payment_url: 'https://pay.synthetic.invalid' } } },
  { name: 'Pancake', useFlow: () => { const flow = useWaffoPancakePayment(); return () => flow.processWaffoPancakePayment(120) }, data: { success: true, data: { checkout_url: 'https://pay.synthetic.invalid' } } },
  { name: 'redemption', useFlow: () => { const flow = useRedemption(); return () => flow.redeemCode('synthetic-code') }, data: { success: true, data: 500000 } },
  { name: 'affiliate transfer', useFlow: () => { const flow = useAffiliate(); return () => flow.transferQuota(100) }, data: { success: true } },
]
for (const flow of flows) {
  it.each(['actor', 'SID', 'unmount'] as const)(`${flow.name} ignores a late successful response after %s without redirecting, notifying, rereading or repeating the write`, async (change) => {
    const pending = deferred()
    const post = vi.spyOn(api, 'post').mockReturnValue(pending.promise)
    const view = renderHook(flow.useFlow)
    let result!: Promise<boolean>
    act(() => { result = view.result.current() })
    await waitFor(() => expect(post).toHaveBeenCalledOnce())
    const reads = vi.mocked(api.get).mock.calls.length
    const location = window.location.href
    act(() => { if (change === 'unmount') view.unmount(); else signIn(change === 'actor' ? 2 : 1, 'wallet-next') })
    await act(async () => { pending.resolve({ data: flow.data }); await result })
    expect(window.open).not.toHaveBeenCalled()
    expect(HTMLFormElement.prototype.submit).not.toHaveBeenCalled()
    expect(window.location.href).toBe(location)
    expect(toast.success).not.toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()
    expect(api.get).toHaveBeenCalledTimes(reads)
    expect(post).toHaveBeenCalledOnce()
    expect(await result).toBe(false)
  })
}

it('failed or absent quote data stays unknown while an explicitly quoted zero stays valid', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValueOnce({ data: { success: false, data: '0' } }).mockResolvedValueOnce({ data: { success: true, data: '0' } })
  const view = renderHook(usePayment)
  await act(async () => { expect(await view.result.current.calculatePaymentAmount(120, 'alipay')).toBeUndefined() })
  expect(view.result.current.quoteKnown).toBe(false)
  expect(view.result.current.quoteError).toBeTruthy()
  await act(async () => { expect(await view.result.current.calculatePaymentAmount(120, 'alipay')).toBe(0) })
  expect(view.result.current.quoteKnown).toBe(true)
  expect(view.result.current.quoteError).toBeNull()
  expect(post).toHaveBeenCalledTimes(2)
})

it('a superseded amount quote cannot overwrite the current quote or authorize its old confirmation', async () => {
  const oldQuote = deferred()
  vi.spyOn(api, 'post').mockReturnValueOnce(oldQuote.promise).mockResolvedValueOnce({ data: { success: true, data: '20' } })
  const view = renderHook(usePayment)
  let first!: Promise<number | undefined>
  act(() => { first = view.result.current.calculatePaymentAmount(100, 'alipay') })
  await act(async () => { expect(await view.result.current.calculatePaymentAmount(200, 'stripe')).toBe(20) })
  await act(async () => { oldQuote.resolve({ data: { success: true, data: '10' } }); expect(await first).toBeUndefined() })
  expect(view.result.current.isQuoteCurrent(100, 'alipay')).toBe(false)
  expect(view.result.current.isQuoteCurrent(200, 'stripe')).toBe(true)
  expect(view.result.current.amount).toBe(20)
  expect(view.result.current.quoteKnown).toBe(true)
})

for (const flow of flows) {
  it(`${flow.name} has one synchronous flight even when activated twice before React updates`, async () => {
    const pending = deferred()
    const post = vi.spyOn(api, 'post').mockReturnValue(pending.promise)
    const view = renderHook(flow.useFlow)
    let first!: Promise<boolean>
    let second!: Promise<boolean>
    act(() => { first = view.result.current(); second = view.result.current() })
    expect(await second).toBe(false)
    expect(post).toHaveBeenCalledOnce()
    await act(async () => { pending.resolve({ data: { success: false, message: 'Synthetic rejected command' } }); await first })
    expect(post).toHaveBeenCalledOnce()
  })
}

it.each(['redemption', 'affiliate transfer'])('an acknowledged %s remains successful when its following read fails, so a read error cannot invite another write', async (name) => {
  vi.mocked(api.get).mockImplementation(async (url) => {
    if (url === '/api/user/aff') return { data: { success: true, data: 'synthetic-affiliate' } }
    throw new Error('Synthetic snapshot read failed after commit')
  })
  const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true, data: 500000 } })
  const flow = flows.find((candidate) => candidate.name === name)
  if (!flow) throw new Error('Expected the acknowledged funding flow')
  const view = renderHook(flow.useFlow)
  await act(async () => { expect(await view.result.current()).toBe(true) })
  expect(toast.success).toHaveBeenCalledOnce()
  expect(toast.error).not.toHaveBeenCalled()
  expect(post).toHaveBeenCalledOnce()
})

it('closing a transfer task cannot apply its late response to a reopened transfer under the same account', async () => {
  const pending = deferred()
  const post = vi.spyOn(api, 'post').mockReturnValue(pending.promise)
  const view = renderHook(useAffiliate)
  let current = true
  let request!: Promise<boolean>
  act(() => { request = view.result.current.transferQuota(100, () => current) })
  const reads = vi.mocked(api.get).mock.calls.length
  current = false
  await act(async () => { pending.resolve({ data: { success: true } }); expect(await request).toBe(false) })
  expect(post).toHaveBeenCalledOnce()
  expect(api.get).toHaveBeenCalledTimes(reads)
  expect(toast.success).not.toHaveBeenCalled()
  expect(view.result.current.outcomeUnknown).toBe(true)
})

it('affiliate recovery repeats only the same preserved quota command and request_id instead of minting a second transfer', async () => {
  const post = vi.spyOn(api, 'post').mockRejectedValueOnce(new Error('Synthetic transfer response lost')).mockResolvedValue({ data: { success: true } })
  const view = renderHook(useAffiliate)
  await act(async () => { expect(await view.result.current.transferQuota(100)).toBe(false) })
  expect(view.result.current.outcomeUnknown).toBe(true)
  const original = post.mock.calls[0]?.[1]
  await act(async () => { expect(await view.result.current.transferQuota(200)).toBe(false) })
  expect(post).toHaveBeenCalledOnce()
  await act(async () => { expect(await view.result.current.transferQuota(100)).toBe(true) })
  expect(post).toHaveBeenCalledTimes(2)
  expect(post.mock.calls[1]?.[1]).toEqual(original)
  expect(original).toMatchObject({ quota: 100, request_id: expect.any(String) })
  expect(view.result.current.outcomeUnknown).toBe(false)
})
