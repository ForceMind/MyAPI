import axios, { AxiosError, type AxiosAdapter, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AuthBundle } from '@/stores/auth-store'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))
let transport: AxiosAdapter
const adapter: AxiosAdapter = (config) => transport(config)
const originalAdapter = axios.defaults.adapter
axios.defaults.adapter = adapter
const { useAuthStore } = await import('@/stores/auth-store')
const channel = await import('../api')
const learning = await import('@/features/prompt-learning/api')
const official = await import('@/features/system-settings/models/openai-official-pricing-api')
const check = await import('@/features/system-settings/models/openai-price-check-api')
const publication = await import('@/features/system-settings/models/price-publication-api')
const writer = await import('@/features/system-settings/quota-writer/api')
const affinity = await import('@/features/system-settings/general/channel-affinity/api')
const waffo = await import('@/features/system-settings/integrations/waffo-pancake-api')
const usagePolicy = await import('@/features/users/user-usage-policy-api')
const access = await import('@/features/access-policy/api')
const review = await import('@/features/usage-logs/usage-review-api')
const files = await import('@/features/full-content-logs/api')
axios.defaults.adapter = originalAdapter

const originalAuth = useAuthStore.getState()
let dispatched: InternalAxiosRequestConfig[]
let refreshes: InternalAxiosRequestConfig[]
let failureStatus: number
let activeCase: HttpCase

type HttpCase = {
  name: string
  method: string
  url: string
  run: () => Promise<unknown>
  body?: unknown
  params?: unknown
  headers?: Record<string, string>
  signal?: AbortSignal
  response?: unknown
  expected?: unknown
}
const ok = { success: true }
const digest = 'a'.repeat(64)
const source = {
  source_url: 'https://developers.openai.com/api/docs/pricing.md', fetched_at: 1,
  content_sha256: digest, currency: 'USD', unit_tokens: 1000000,
  service_tier: 'standard', scope: 'text-token-price-source-not-published',
  models: [{ model: 'synthetic/model', source_label: 'Synthetic', short_context: {
    input_usd_per_million: '0', cached_input_usd_per_million: null,
    cache_write_usd_per_million: null, output_usd_per_million: '1.25',
  }, long_context: null }],
}
const policy = { subject: 'user' as const, subject_id: 17, owner_user_id: 17, revision: 2, assigned: true, enabled: false, public_models: null, upstream_models: null, channel_ids: null }
const candidate = { enabled: false, public_models: ['synthetic/model'], upstream_models: null, channel_ids: null }
const target = { subject: 'user' as const, id: 17, ownerUserId: 17 }
const userPolicy = { user_id: 17, no_balance: false, revision: 2, legacy_remaining_quota: 0 }
const reviewData = { request_id: 'synthetic/request', user_id: 17, token_id: 21, state: 'settled', reserved_quota: 0, actual_quota: 0 }
const manualVersion = { id: 31, content: 'Synthetic instructions', content_hash: digest, source: 'manual', created_at: 1 }
const publicationCommand = { id: digest, expected_digest: digest, action: 'lock' as const, models: [{ model: 'synthetic/model', locked: false }], confirmed: true }
const publicationResult = { receipt: { id: digest, actor_id: 1, action: 'lock', created_at: 1, revision: 2 }, runtime_ready: true }
const signal = new AbortController().signal
const createPayload = { mode: 'single' as const, channel: { name: 'Synthetic import', type: 57 } }
const cases: HttpCase[] = [
  { name: 'channel status', method: 'post', url: '/api/channel/17/status', body: { status: 0 }, run: () => channel.updateChannelStatus(17, 0) },
  { name: 'batch status', method: 'post', url: '/api/channel/status/batch', body: { ids: [17, 18], status: 0 }, run: () => channel.batchUpdateChannelStatus([17, 18], 0) },
  { name: 'delete channel', method: 'delete', url: '/api/channel/17', run: () => channel.deleteChannel(17) },
  { name: 'batch delete', method: 'post', url: '/api/channel/batch', body: { ids: [17, 18] }, run: () => channel.batchDeleteChannels({ ids: [17, 18] }) },
  { name: 'clear batch tag', method: 'post', url: '/api/channel/batch/tag', body: { ids: [17, 18], tag: null }, run: () => channel.batchSetChannelTag({ ids: [17, 18], tag: null }) },
  { name: 'copy stable operation', method: 'post', url: '/api/channel/copy/17', body: null, params: { suffix: '-synthetic', reset_balance: false }, headers: { 'Idempotency-Key': 'synthetic-stable-operation' }, run: () => channel.copyChannel(17, { suffix: '-synthetic', reset_balance: false, operation_key: 'synthetic-stable-operation' }) },
  { name: 'repair abilities', method: 'post', url: '/api/channel/fix', run: () => channel.fixChannelAbilities() },
  { name: 'delete disabled', method: 'delete', url: '/api/channel/disabled', run: () => channel.deleteDisabledChannels() },
  { name: 'enable tag', method: 'post', url: '/api/channel/tag/enabled', body: { tag: 'synthetic/tag' }, run: () => channel.enableTagChannels('synthetic/tag') },
  { name: 'disable tag', method: 'post', url: '/api/channel/tag/disabled', body: { tag: 'synthetic/tag' }, run: () => channel.disableTagChannels('synthetic/tag') },
  { name: 'edit tag with zero', method: 'put', url: '/api/channel/tag', body: { tag: 'synthetic/tag', weight: 0, priority: 0 }, run: () => channel.editTagChannels({ tag: 'synthetic/tag', weight: 0, priority: 0 }) },
  { name: 'save delivery configuration', method: 'put', url: '/api/channel/quota/alerts/delivery', body: { webhook_url: 'https://synthetic.invalid/hook', webhook_secret: 'synthetic-placeholder' }, run: () => channel.updateChannelQuotaAlertDeliverySettings({ webhook_url: 'https://synthetic.invalid/hook', webhook_secret: 'synthetic-placeholder' }) },
  { name: 'run delivery', method: 'post', url: '/api/channel/quota/alerts/delivery/run', run: () => channel.runChannelQuotaAlertDelivery() },
  { name: 'explicit model probe', method: 'post', url: '/api/channel/test/17', body: { model: 'synthetic/model', endpoint_type: 'chat', stream: false }, run: () => channel.testChannel(17, { model: 'synthetic/model', endpoint_type: 'chat', stream: false }) },
  { name: 'default model probe', method: 'get', url: '/api/channel/test/17', params: { stream: false }, run: () => channel.testChannel(17, { stream: false }) },
  { name: 'sample selected balance', method: 'get', url: '/api/channel/update_balance/17', run: () => channel.updateChannelBalance(17) },
  { name: 'fetch selected upstream models', method: 'get', url: '/api/channel/fetch_models/17', run: () => channel.fetchUpstreamModels(17) },
  { name: 'probe unsaved model configuration', method: 'post', url: '/api/channel/fetch_models', body: { base_url: 'https://synthetic.invalid', type: 1, key: 'synthetic-placeholder', proxy: '' }, run: () => channel.fetchModels({ base_url: 'https://synthetic.invalid', type: 1, key: 'synthetic-placeholder', proxy: '' }) },
  { name: 'probe all channels', method: 'get', url: '/api/channel/test', run: () => channel.testAllChannels() },
  { name: 'sample all balances', method: 'get', url: '/api/channel/update_balance', run: () => channel.updateAllChannelsBalance() },
  { name: 'refresh model discovery', method: 'post', url: '/api/channel/model-discovery/17', run: () => channel.refreshChannelModelDiscovery(17) },
  { name: 'sample online Codex usage', method: 'get', url: '/api/channel/17/codex/usage', run: () => channel.getCodexUsage(17) },
  { name: 'query online reset credits', method: 'get', url: '/api/channel/17/codex/usage/reset-credits', run: () => channel.getCodexResetCredits(17) },
  { name: 'reveal channel credential', method: 'post', url: '/api/channel/17/key', headers: { 'X-Security-Proof': 'synthetic-proof' }, run: () => channel.getChannelKey(17, 'synthetic-proof') },
  { name: 'refresh Codex credential', method: 'post', url: '/api/channel/17/codex/refresh', body: {}, run: () => channel.refreshCodexCredential(17) },
  { name: 'import new local credential', method: 'post', url: '/api/channel/codex/local-auth/import', body: createPayload, headers: { 'X-Security-Proof': 'synthetic-proof' }, run: () => channel.importCodexLocalAuthForNewChannel(createPayload, 'synthetic-proof') },
  { name: 'import selected local credential', method: 'post', url: '/api/channel/17/codex/local-auth/import', headers: { 'X-Security-Proof': 'synthetic-proof' }, run: () => channel.importCodexLocalAuthForChannel(17, 'synthetic-proof') },
  { name: 'consume reset credit', method: 'post', url: '/api/channel/17/codex/usage/reset', body: {}, run: () => channel.resetCodexUsage(17) },
  { name: 'save learning authorization', method: 'put', url: '/api/user/prompt-learning', body: { enabled: false }, response: { success: true, data: { scope: 'self', enabled: false, generation: 2 } }, expected: { scope: 'self', enabled: false, generation: 2 }, run: () => learning.updatePromptLearningPolicy(false) },
  { name: 'cancel selected learning run', method: 'post', url: '/api/user/prompt-learning/runs/17/cancel', response: { success: true, data: { cancelled: true } }, expected: undefined, run: () => learning.cancelPromptLearningRun(17) },
  { name: 'save immutable learning command', method: 'post', url: '/api/user/prompt-learning/versions', body: { command_id: 'synthetic-stable-command', content: 'Synthetic instructions', parent_id: 30 }, response: { success: true, data: { version: manualVersion, created: true } }, expected: { version: manualVersion, created: true }, run: () => learning.createPromptLearningVersion({ commandId: 'synthetic-stable-command', content: 'Synthetic instructions', parentId: 30 }) },
  { name: 'freeze official source', method: 'post', url: '/api/ratio_sync/openai/versions', signal, response: { success: true, data: source }, expected: source, run: () => official.saveOfficialOpenAIPriceSource(signal) },
  { name: 'queue official price check', method: 'post', url: '/api/ratio_sync/openai/check', body: {}, response: { success: true, data: { task: { task_id: 'synthetic-task', status: 'pending' }, created: true } }, expected: { task: { task_id: 'synthetic-task', status: 'pending' }, created: true }, run: () => check.startOpenAIPriceCheck() },
  { name: 'disable official price check', method: 'put', url: '/api/option/', body: { key: 'OpenAIOfficialPriceCheckEnabled', value: 'false' }, expected: undefined, run: () => check.setOpenAIPriceCheckEnabled(false) },
  { name: 'publish original price receipt', method: 'post', url: '/api/ratio_sync/openai/publications', body: publicationCommand, response: { success: true, data: publicationResult }, expected: publicationResult, run: () => publication.applyPricePublication(publicationCommand) },
  { name: 'apply writer epoch', method: 'post', url: '/api/quota-writer/apply', body: { target_mode: 'bridge', expected_epoch: 7, ack_note: 'Synthetic confirmation' }, run: () => writer.applyQuotaWriterTransition({ target_mode: 'bridge', expected_epoch: 7, ack_note: 'Synthetic confirmation' }) },
  { name: 'drive bounded drain', method: 'post', url: '/api/quota-writer/drain', body: null, params: { budget: 0 }, run: () => writer.driveQuotaWriterDrains(0) },
  { name: 'clear affinity cache', method: 'delete', url: '/api/option/channel_affinity_cache', params: { all: true }, run: () => affinity.clearAllCache() },
  { name: 'clear exact affinity rule', method: 'delete', url: '/api/option/channel_affinity_cache', params: { rule_name: 'synthetic/rule' }, run: () => affinity.clearRuleCache('synthetic/rule') },
  { name: 'create Waffo pair', method: 'post', url: '/api/option/waffo-pancake/pair', body: { merchant_id: 'synthetic-merchant', private_key: 'synthetic-placeholder', return_url: 'https://synthetic.invalid/return' }, run: () => waffo.createWaffoPancakePair({ merchantID: 'synthetic-merchant', privateKey: 'synthetic-placeholder', returnURL: 'https://synthetic.invalid/return' }) },
  { name: 'save Waffo selection', method: 'post', url: '/api/option/waffo-pancake/save', body: { merchant_id: 'synthetic-merchant', private_key: 'synthetic-placeholder', return_url: 'https://synthetic.invalid/return', store_id: 'synthetic-store', product_id: 'synthetic-product' }, run: () => waffo.saveWaffoPancakeConfig({ merchantID: 'synthetic-merchant', privateKey: 'synthetic-placeholder', returnURL: 'https://synthetic.invalid/return', storeID: 'synthetic-store', productID: 'synthetic-product' }) },
  { name: 'write original user policy ID', method: 'put', url: '/api/user/17/usage-policy', body: { id: digest, expected_revision: 1, no_balance: false, confirmed: true }, response: { success: true, data: userPolicy }, expected: userPolicy, run: () => usagePolicy.writeUserUsagePolicy(17, { id: digest, expected_revision: 1, no_balance: false, confirmed: true }) },
  { name: 'write user assigned access CAS', method: 'put', url: '/api/access-policy/user/17', body: { ...candidate, expected_revision: 1 }, signal, response: { success: true, data: policy }, expected: policy, run: () => access.saveAssignedPolicy(target, { ...candidate, expected_revision: 1 }, signal) },
  { name: 'remove user assigned access CAS', method: 'delete', url: '/api/access-policy/user/17', body: { expected_revision: 1 }, signal, response: { success: true, data: policy }, expected: policy, run: () => access.removeAssignedPolicy(target, 1, signal) },
  { name: 'reconcile exact dispatch request', method: 'post', url: '/api/usage-review/synthetic%2Frequest/reconcile', body: { actual_quota: 0, evidence_reference: 'synthetic-evidence', confirmed_reliable_evidence: true, actual_input_tokens: 0, actual_output_tokens: 0, actual_fee_usd: '0' }, response: { success: true, data: reviewData }, expected: reviewData, run: () => review.reconcileUsageReview('synthetic/request', 0, 'synthetic-evidence', { input: 0, output: 0, feeUSD: '0' }) },
  { name: 'recover exact dispatch request', method: 'post', url: '/api/usage-review/synthetic%2Frequest/recover-dispatch', body: { actual_quota: 0, evidence_reference: 'synthetic-evidence', confirmed_reliable_evidence: true, confirmed_request_finished: true }, response: { success: true, data: reviewData }, expected: reviewData, run: () => review.recoverTextDispatchUsage('synthetic/request', 0, 'synthetic-evidence') },
  { name: 'delete selected content file', method: 'delete', url: '/api/full-content-logs/files/synthetic%2Fcontent.jsonl', run: () => files.deleteFullContentLogFile('synthetic/content.jsonl') },
  { name: 'delete all content files', method: 'delete', url: '/api/full-content-logs/files', run: () => files.deleteAllFullContentLogFiles() },
]

function bundle(token: string): AuthBundle {
  return {
    access_token: token, token_type: 'Bearer', access_expires_at: 4000000000,
    user: { id: 1, role: 100, username: 'synthetic-maintainer' },
    session: { sid: 'synthetic-session', current: true, login_method: 'password', ip: '192.0.2.1', user_agent: 'test', created_at: 1, last_active_at: 1, expires_at: 4000000000 },
  }
}
function response(config: InternalAxiosRequestConfig, data: unknown): AxiosResponse {
  return { config, data, status: 200, statusText: 'OK', headers: {} }
}
function requestBody(config: InternalAxiosRequestConfig) {
  return typeof config.data === 'string' ? JSON.parse(config.data) : config.data
}
function expectOriginalRequest(testCase: HttpCase) {
  const config = dispatched[0]
  if (!config) throw new Error('Expected original HTTP dispatch')
  expect(config.method).toBe(testCase.method)
  expect(config.url).toBe(testCase.url)
  expect(requestBody(config)).toEqual(testCase.body)
  expect(config.params).toEqual(testCase.params)
  if (testCase.signal) expect(config.signal).toBe(testCase.signal)
  for (const [name, value] of Object.entries(testCase.headers ?? {})) {
    expect(config.headers.get(name)).toBe(value)
  }
}

beforeEach(() => {
  localStorage.clear()
  dispatched = []
  refreshes = []
  failureStatus = 401
  useAuthStore.getState().auth.setBundle(bundle('synthetic-original-access'))
  transport = async (config) => {
    if (config.url === '/api/user/auth/refresh') {
      refreshes.push(config)
      return response(config, { success: true, data: bundle('synthetic-refreshed-access') })
    }
    if (config.url !== activeCase.url) throw new Error(`Unexpected synthetic request ${config.method} ${config.url}`)
    dispatched.push(config)
    if (dispatched.length === 1 && failureStatus !== 0) {
      throw new AxiosError('Synthetic rejected action', AxiosError.ERR_BAD_REQUEST, config, undefined, {
        config, data: { success: false }, status: failureStatus, statusText: 'Rejected', headers: {},
      })
    }
    return response(config, activeCase.response ?? ok)
  }
})
afterEach(() => {
  useAuthStore.setState(originalAuth)
  localStorage.clear()
  vi.restoreAllMocks()
})

describe('remaining reachable UI API actions never replay', () => {
  it.each(cases)('$name rejects 401 once without refresh or replay', async (testCase) => {
    activeCase = testCase
    await expect(testCase.run()).rejects.toMatchObject({ response: { status: 401 } })
    expect(dispatched).toHaveLength(1)
    expect(refreshes).toHaveLength(0)
    expectOriginalRequest(testCase)
  })
  it.each(cases)('$name rejects 403 once without refresh or replay', async (testCase) => {
    activeCase = testCase
    failureStatus = 403
    await expect(testCase.run()).rejects.toMatchObject({ response: { status: 403 } })
    expect(dispatched).toHaveLength(1)
    expect(refreshes).toHaveLength(0)
    expectOriginalRequest(testCase)
  })
  it.each(cases)('$name preserves successful original fields, IDs, headers and parameters', async (testCase) => {
    activeCase = testCase
    failureStatus = 0
    const result = await testCase.run()
    expect(result).toEqual('expected' in testCase ? testCase.expected : ok)
    expect(dispatched).toHaveLength(1)
    expect(refreshes).toHaveLength(0)
    expectOriginalRequest(testCase)
  })
})

const readCases: HttpCase[] = [
  { name: 'multi-key status POST', method: 'post', url: '/api/channel/multi_key/manage', body: { channel_id: 17, action: 'get_key_status', page: 2, page_size: 50 }, run: () => channel.manageMultiKeys({ channel_id: 17, action: 'get_key_status', page: 2, page_size: 50 }) },
  { name: 'user access preview POST', method: 'post', url: '/api/access-policy/user/17/preview', body: candidate, signal, response: { success: true, data: { policy, models: [] } }, expected: { policy, models: [] }, run: () => access.previewAssignedPolicy(target, candidate, signal) },
  { name: 'persisted discovery GET', method: 'get', url: '/api/channel/model-discovery/17', run: () => channel.getChannelModelDiscovery(17) },
]
it.each(readCases)('$name retains the original positive authentication refresh behavior', async (testCase) => {
  activeCase = testCase
  const result = await testCase.run()
  expect(result).toEqual('expected' in testCase ? testCase.expected : ok)
  expect(refreshes).toHaveLength(1)
  expect(dispatched).toHaveLength(2)
  expectOriginalRequest(testCase)
  const replay = dispatched[1]
  if (!replay) throw new Error('Expected permitted read replay')
  expect(replay.method).toBe(testCase.method)
  expect(requestBody(replay)).toEqual(testCase.body)
  expect(replay.params).toEqual(testCase.params)
})
