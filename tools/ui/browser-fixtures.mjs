// Synthetic contract fixtures for the production-build UI qualification only.
// No real credentials, accounts, provider calls, or financial acceptance evidence.
import { quotaFixtures } from '../quota/browser-fixtures.mjs'

export const FIXTURE_TIME = 1791352800000 // 2026-10-07T06:00:00Z
export const LONG_NAME = 'Synthetic operations workspace · International model and account administration · 浏览器回归'
export const SETTLEMENT_REVIEW_IDS = ['synthetic-settlement-pending', 'synthetic-settlement-journal', 'synthetic-settlement-manual']
export const SETTLEMENT_REVIEW_METADATA = '{"synthetic_frozen_pricing":"Root-only visible evidence, never a real invoice or credential"}'
export const SETTLEMENT_REVIEW_DIAGNOSTIC = 'synthetic-private-review-diagnostic'
const now = FIXTURE_TIME / 1000
const ok = data => ({ success: true, data })
const page = items => ({ items, total: items.length, page: 1, page_size: 10 })

// Keep the existing quota contracts, but do not inherit their broad /api/data*
// matcher. Only these exact GET routes may be delegated to the older fixture.
const sharedReads = new Set([
  '/api/channel/quota/status', '/api/channel/quota/changes',
  '/api/channel/1/quota/history', '/api/channel/1/codex/usage',
  '/api/channel/1/codex/usage/reset-credits', '/api/channel/ops',
  '/api/channel/model-discovery/1', '/api/channel/1', '/api/channel', '/api/channel/search',
  '/api/channel/models', '/api/group', '/api/prefill_group',
  '/api/user/models', '/api/user/self/groups', '/api/user/2fa/status', '/api/user/passkey',
  '/api/uptime/status', '/api/log', '/api/log/self', '/api/log/stat', '/api/log/self/stat',
  '/api/log/overview', '/api/log/self/overview',
  '/api/ratio_sync/openai', '/api/user/topup/info', '/api/user/topup/self', '/api/user/topup',
  '/api/user/aff', '/api/subscription/self',
])
const publicReads = new Set([
  '/api/setup', '/api/status', '/api/notice', '/api/home_page_content',
  '/api/about', '/api/pricing', '/api/rankings', '/api/user-agreement', '/api/privacy-policy',
])
const ownerReads = new Set([
  '/pg/keys', '/pg/models', '/api/user/self', '/api/user/models', '/api/user/self/groups', '/api/user/2fa/status',
  '/api/user/passkey', '/api/user/sessions', '/api/user/oauth/bindings',
  '/api/token', '/api/token/search', '/api/token/auto-groups', '/api/data/self', '/api/data/flow/self',
  '/api/log/self', '/api/log/self/stat', '/api/log/self/overview',
  '/api/mj/self', '/api/task/self', '/api/uptime/status',
  '/api/user/topup/info', '/api/user/topup/self', '/api/user/aff', '/api/subscription/self',
  '/api/user/prompt-learning', '/api/user/prompt-learning/versions', '/api/user/prompt-learning/runs',
])
const rootPrefixes = ['/api/option', '/api/system-info', '/api/system-task', '/api/quota-writer', '/api/ratio_sync', '/api/custom-oauth-provider', '/api/performance']

export function createUIFixture({ role = 100, language = 'en', setupComplete = true, sidebar = false, longText = false, playgroundDisabled = false, pricingEnabled = false, settlementReviews = false } = {}) {
  // quotaFixtures accepts time through Date.now. Override only during its
  // synchronous construction and restore immediately; no timers are faked here.
  const originalNow = Date.now
  let quota
  try { Date.now = () => FIXTURE_TIME; quota = quotaFixtures() }
  finally { Date.now = originalNow }
  const user = {
    id: role === 1 ? 2 : 1, username: role === 1 ? 'synthetic-owner' : 'synthetic-operator',
    display_name: longText ? LONG_NAME : 'Synthetic UI operator', role, status: 1,
    group: 'default', account_tier_id: 'standard', quota: 1000000, used_quota: 12500,
    request_count: 1, language, email: '', setting: {},
    sidebar_modules: sidebar ? JSON.stringify({ console: { enabled: true, token: false } }) : '',
    permissions: {
      sidebar_settings: role !== 100,
      admin_permissions: role >= 10 ? { channel: { read: true, operate: true, write: true, sensitive_write: true, secret_view: false } } : {},
    },
  }
  const session = { sid: `synthetic-ui-${role}`, current: true, login_method: 'synthetic', ip: '127.0.0.1', user_agent: 'Synthetic Chromium', created_at: now - 600, last_active_at: now, expires_at: now + 3600 }
  const funding = { mode: 'disabled', epoch: 1, ready: true, can_top_up: false, can_redeem: false, can_transfer_affiliate_rewards: false, can_purchase_subscription: false, can_view_funding_history: true }
  const key = { id: 21, user_id: user.id, name: longText ? LONG_NAME : 'synthetic-key', key: 'masked', status: 1, remain_quota: 1000, used_quota: 0, unlimited_quota: false, expired_time: -1, created_time: now - 600, accessed_time: now, group: 'default', model_limits_enabled: false, model_limits: '', allow_ips: '' }
  const writerState = { id: 1, schema_version: 1, mode: 'legacy', epoch: 1, lock_version: 1, updated_at: now }
  const writerAudit = { can_enable: false, state: writerState, writers: [], all_writers_migrated: false, batch_queue_empty: true, projection_pending: 0, balance_drain_pending: 0, balance_drain_inflight_zero: true, maintenance_backfill_done: false, redis_epoch_consistent: false, cluster_drain_ack: false, inflight_sessions: 0, inflight_zero: true, missing_or_failed_checks: ['all_writers_migrated'] }
  const options = [
    { key: 'SystemName', value: 'MyAPI' },
    { key: 'Notice', value: '' },
    { key: 'user_funding_setting.mode', value: 'disabled' },
  ]
  // Defaults preserve earlier journeys; new journeys explicitly opt into each
  // populated, confirmed-empty, or unavailable contract before navigation.
  const state = { keys: 'populated', options: 'ready', analytics: 'empty', performance: 'empty', modelPerformance: 'empty', deploymentSettings: 'disabled', promptLearning: 'ready', pricing: 'empty' }
  const analyticsRows = [
    { id: 31, user_id: user.id, username: user.username, model_name: 'synthetic-text-model', created_at: now - 3600, token_used: 2400, count: 12, quota: 6000 },
    { id: 32, user_id: user.id, username: user.username, model_name: 'synthetic-text-model', created_at: now, token_used: 1600, count: 8, quota: 4000 },
  ]
  const flowRows = [{ user_id: user.id, username: user.username, node_name: 'synthetic-node', use_group: 'default', token_id: 21, token_name: 'synthetic-key', channel_id: 1, channel_name: 'synthetic-channel', model_name: 'synthetic-text-model', token_used: 4000, count: 20, quota: 10000 }]
  // Match raw journal states rather than falsely declaring finalization done.
  // The opt-in is session-local and cannot alter the older quota/playground suite.
  const settlementDetails = new Map(SETTLEMENT_REVIEW_IDS.map((requestId, index) => [`/api/usage-review/${requestId}`, {
    request_id: requestId, user_id: 2, token_id: 21, channel_id: 1,
    model_name: 'synthetic-text-model', writer: index === 1 ? 'legacy' : 'authoritative',
    state: index === 1 ? 'prepared' : 'usage_unknown',
    reserved_quota: index === 1 ? 0 : 100, actual_quota: index === 1 ? 60 : null,
    text_dispatch_pending: index === 0, can_recover_text_dispatch: false,
    can_reconcile_usage: index === 2,
    settlement_status: ['pending', 'applied_journal_pending', 'none'][index],
    recovery_block_reason: ['automatic_settlement_pending', 'automatic_settlement_applied', ''][index],
    // Existing GET includes frozen pricing for the owner too; the UI must hide
    // it. Do not invent a stronger response-redaction contract in this fixture.
    review_metadata: SETTLEMENT_REVIEW_METADATA, reason: SETTLEMENT_REVIEW_DIAGNOSTIC,
  }]))
  const unavailable = message => ({ status: 503, body: { success: false, message } })
  return {
    state,
    user,
    response(url, method = 'GET') {
      const path = url.pathname.replace(/\/$/, '')
      if (path === '/api/user/auth/refresh' && method === 'POST') {
        return { status: role === 0 ? 401 : 200, body: role === 0 ? { success: false, message: 'Synthetic anonymous visitor' } : ok({ access_token: 'synthetic-ui-session-not-a-credential', token_type: 'Bearer', access_expires_at: now + 3600, user, session }) }
      }
      if (method !== 'GET') return { violation: `Unexpected mutation: ${method} ${path}` }
      if (path === '/api/perf-metrics') {
        if (!pricingEnabled || url.searchParams.get('model') !== 'synthetic-text-model' || url.searchParams.get('hours') !== '24' || [...url.searchParams.keys()].some(key => !['model', 'hours'].includes(key))) return { violation: `Unconfigured performance read: ${method} ${path}${url.search}` }
        if (state.modelPerformance === 'error') return unavailable('Synthetic model performance unavailable')
        return { status: 200, body: ok({ model_name: 'synthetic-text-model', groups: [] }) }
      }
      // The real API exposes this exact summary to public pricing. This
      // opt-in never grants ordinary dashboard sessions a summary read.
      if (pricingEnabled && path === '/api/perf-metrics/summary') {
        if (url.searchParams.get('hours') !== '24' || [...url.searchParams.keys()].some(key => key !== 'hours')) return { violation: `Unconfigured pricing summary read: ${method} ${path}${url.search}` }
        if (state.performance === 'error') return unavailable('Synthetic performance unavailable')
        return { status: 200, body: ok({ models: [] }) }
      }
      if (role === 0 && !publicReads.has(path)) return { violation: `Anonymous private request: ${method} ${path}` }
      if (role === 1 && !publicReads.has(path) && !ownerReads.has(path) && !(settlementReviews && settlementDetails.has(path))) return { violation: `Ordinary user requested admin data: ${method} ${path}` }
      if (role !== 100 && rootPrefixes.some(prefix => path === prefix || path.startsWith(`${prefix}/`))) return { violation: `Non-Root requested Root data: ${method} ${path}` }
      if (settlementReviews && settlementDetails.has(path)) {
        if (url.search) return { violation: `Unconfigured settlement detail query: ${method} ${path}${url.search}` }
        if (role !== 100 && role !== 1) return { violation: `Non-owner requested settlement detail: ${method} ${path}` }
        return { status: 200, body: ok({ ...settlementDetails.get(path) }) }
      }
      let body
      switch (path) {
        case '/pg/keys': body = ok(page([{ id: key.id, name: key.name, status: key.status, group: key.group, remain_quota: key.remain_quota, used_quota: key.used_quota, unlimited_quota: key.unlimited_quota, expired_time: key.expired_time, model_limits_enabled: key.model_limits_enabled }])); break
        case '/pg/models': body = { success: true, object: 'list', data: [{ id: 'synthetic-text-model', object: 'model', owned_by: 'synthetic-provider' }] }; break
        case '/api/setup': body = ok({ status: setupComplete, root_init: setupComplete, database_type: 'sqlite' }); break
        case '/api/user/self': body = ok(user); break
        case '/api/status': body = ok({
          system_name: longText ? LONG_NAME : 'MyAPI', version: 'synthetic-ui-fixture', start_time: now - 600,
          password_login_enabled: true, password_register_enabled: true, register_enabled: true,
          self_use_mode_enabled: false, email_verification: false, turnstile_check: false, passkey_login: false,
          api_info_enabled: false, announcements_enabled: false, faq_enabled: false, uptime_kuma_enabled: false,
          quota_per_unit: 500000, display_in_currency: false,
          user_funding_mode: 'disabled', user_funding_capabilities: funding,
          SidebarModulesAdmin: sidebar || playgroundDisabled ? JSON.stringify({ ...(sidebar ? { console: { enabled: true, log: false } } : {}), ...(playgroundDisabled ? { chat: { enabled: true, playground: false } } : {}) }) : '',
          ...(pricingEnabled ? { HeaderNavModules: JSON.stringify({ pricing: { enabled: true, requireAuth: false } }) } : {}),
        }); break
        case '/api/notice': case '/api/home_page_content': body = ok(''); break
        case '/api/about': body = ok('# About\n\nSynthetic UI qualification. MyAPI retains its source notices and existing contracts.'); break
        case '/api/user-agreement': case '/api/privacy-policy': body = ok('Synthetic policy content for layout qualification only.'); break
        case '/api/option':
          if (state.options === 'error') return { status: 503, body: { success: false, message: 'Synthetic settings unavailable' } }
          body = ok(options); break
        case '/api/option/typed-bulk/revision': body = ok({ revision: 1 }); break
        case '/api/token': case '/api/token/search': body = ok(page(state.keys === 'empty' ? [] : [key])); break
        case '/api/token/auto-groups': body = ok({ groups: ['default'], max_count: 5 }); break
        case '/api/log': case '/api/log/self':
          if (!settlementReviews) { body = quota.response(url); break }
          body = ok({ ...page([...settlementDetails.values()].map((review, index) => ({
            id: 70 + index, user_id: review.user_id, token_id: review.token_id,
            request_id: review.request_id, created_at: now - index * 60, type: 5,
            content: `Synthetic status ${review.request_id}`, username: 'synthetic-owner',
            token_name: 'synthetic-key', model_name: review.model_name,
            quota: 0, prompt_tokens: 0, completion_tokens: 0, use_time: 1, is_stream: true,
            ...(role === 100 ? { channel: 1, channel_name: 'synthetic-channel' } : {}),
            group: 'default', ip: '',
            other: JSON.stringify({ usage_accuracy: 'unknown', settlement_status: 'pending_review', reserved_quota: review.reserved_quota, actual_quota: null }),
          }))), page_size: 20 }); break
        case '/api/channel/models_enabled': body = ok(['synthetic-text-model']); break
        case '/api/channel/quota/alerts/delivery': body = ok({ policy_enabled: false, configured: false, https_only: true, redirects_allowed: false, timeout_ms: 5000, max_attempts: 3 }); break
        case '/api/channel/quota/alerts': case '/api/channel/quota/events': body = ok(page([])); break
        case '/api/deployments/settings':
          if (state.deploymentSettings === 'error') return unavailable('Synthetic deployment settings unavailable')
          body = ok({ enabled: false }); break
        case '/api/data': case '/api/data/self': case '/api/data/users':
          if (state.analytics === 'error') return unavailable('Synthetic analytics unavailable')
          body = ok(state.analytics === 'populated' ? analyticsRows : []); break
        case '/api/data/flow': case '/api/data/flow/self':
          if (state.analytics === 'error') return unavailable('Synthetic analytics unavailable')
          body = ok(state.analytics === 'populated' ? flowRows : []); break
        case '/api/perf-metrics/summary':
          if (state.performance === 'error') return unavailable('Synthetic performance unavailable')
          body = ok({ models: [] }); break
        case '/api/user/prompt-learning':
          if (state.promptLearning === 'error') return unavailable('Synthetic learning settings unavailable')
          body = ok({ scope: 'self', enabled: false, generation: 0 }); break
        case '/api/user/prompt-learning/versions': case '/api/user/prompt-learning/runs':
          if (url.searchParams.get('p') !== '1' || url.searchParams.get('page_size') !== '20' || [...url.searchParams.keys()].some(key => !['p', 'page_size'].includes(key))) return { violation: `Unconfigured learning history query: ${method} ${path}${url.search}` }
          if (state.promptLearning === 'error') return unavailable('Synthetic learning settings unavailable')
          body = ok({ items: [], total: 0, page: 1, page_size: 20 }); break
        case '/api/user': case '/api/user/search': body = ok(page([{ ...user, id: 2, username: 'synthetic-managed-user', role: 1 }])); break
        case '/api/authz/catalog': body = ok({ resources: [], roles: [] }); break
        case '/api/user/sessions': body = ok([session]); break
        case '/api/user/oauth/bindings': case '/api/custom-oauth-provider': body = ok([]); break
        case '/api/models': case '/api/models/search': body = ok({ ...page([{ id: 1, model_name: 'synthetic-text-model', description: LONG_NAME, status: 1, sync_official: 0, created_time: now - 600, updated_time: now, name_rule: 0, bound_channels: [], enable_groups: ['default'] }]), vendor_counts: {} }); break
        case '/api/vendors': case '/api/vendors/search': body = ok(page([])); break
        case '/api/models/missing': body = ok([]); break
        case '/api/system-info/instances': body = ok([{ node_name: 'synthetic-node', status: 'online', stale_after_seconds: 60, started_at: now - 600, last_seen_at: now, info: { schema_version: 1, node: { name: 'synthetic-node' }, runtime: { version: 'synthetic-ui-fixture', goos: 'linux', goarch: 'amd64', started_at: now - 600 } } }]); break
        case '/api/system-task/list': body = ok([]); break
        case '/api/system-task/current': body = ok(null); break
        case '/api/quota-writer/status': body = ok({ state: writerState, audit: writerAudit, inflight_sessions: 0 }); break
        case '/api/quota-writer/transitions': body = ok(page([])); break
        case '/api/quota-writer/plan': body = ok({ current: writerState, target_mode: 'bridge', proposed_epoch: 2, audit: writerAudit, ready: false, validation: ['Synthetic evidence only'] }); break
        case '/api/option/channel_affinity_cache': body = ok({ enabled: false, total: 0, unknown: 0, by_rule_name: {}, cache_capacity: 1000, cache_algo: 'lru' }); break
        case '/api/performance/logs': body = ok({ enabled: false, log_dir: '', file_count: 0, total_size: 0 }); break
        case '/api/performance/stats': body = ok({ config: { is_running_in_container: true } }); break
        case '/api/ratio_sync/channels': body = ok([]); break
        case '/api/ratio_sync/openai/check': body = ok({ enabled: false, interval_seconds: 86400, stale_after_seconds: 259200, stale: true, last_attempt_at: 0, last_attempt_status: '', last_task_id: '', last_error_code: '', last_success_at: 0, next_check_at: 0, source_sha256: '', source_fetched_at: 0, pending_source_sha256: '', expected_digest: '0'.repeat(64), revision: 0, diff_total: 0, diff_truncated: false, diff_review_required: false, diff_counts: { addition: 0, change: 0, removal: 0, unqualified: 0, unchanged: 0 }, diff: [] }); break
        case '/api/subscription/admin/plans': case '/api/subscription/plans': body = ok([]); break
        case '/api/redemption': case '/api/redemption/search': body = ok(page([])); break
        case '/api/mj': case '/api/mj/self': case '/api/task': case '/api/task/self': body = ok(page([])); break
        case '/api/full-content-logs': body = ok({ ...page([]), enabled: false, files: { count: 0, total_size: 0, files: [] }, facets: { models: [], tokens: [] } }); break
        case '/api/full-content-logs/files': body = ok({ count: 0, total_size: 0, files: [] }); break
        case '/api/usage-reviews/pending':
          if (!settlementReviews) { body = ok(page([])); break }
          if (role !== 100) return { violation: `Non-Root requested pending settlement reviews: ${method} ${path}` }
          if (!['authoritative', 'legacy'].includes(url.searchParams.get('writer')) || url.searchParams.get('after') !== '0' || [...url.searchParams.keys()].some(key => !['writer', 'after'].includes(key)) || url.searchParams.getAll('writer').length !== 1 || url.searchParams.getAll('after').length !== 1) return { violation: `Unconfigured pending settlement query: ${method} ${path}${url.search}` }
          body = ok({ items: [...settlementDetails.values()].filter(review => review.writer === url.searchParams.get('writer')).map(review => ({ ...review })), next_after: '' }); break
        case '/api/pricing':
          if (state.pricing === 'error') return unavailable('Synthetic pricing unavailable')
          body = { success: true, data: state.pricing === 'populated' ? [{ id: 41, model_name: 'synthetic-text-model', quota_type: 0, model_ratio: 1, completion_ratio: 2, enable_groups: ['default'] }] : [], vendors: [], group_ratio: { default: 1 }, usable_group: { default: { desc: 'Synthetic', ratio: 1 } }, supported_endpoint: {}, auto_groups: [] }; break
        case '/api/rankings': body = ok({ models: [], vendors: [], top_movers: [], top_droppers: [], models_history: { points: [], models: [], buckets: 0 }, vendor_share_history: { points: [], vendors: [], buckets: 0 } }); break
        default: if (sharedReads.has(path)) body = quota.response(url)
      }
      if (!body) return { violation: `Unconfigured API: ${method} ${path}` }
      return { status: 200, body }
    },
  }
}
