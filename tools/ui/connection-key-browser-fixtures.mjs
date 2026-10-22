// Saved association fixtures only. Never return a real credential or accept
// business writes, provider requests, ambiguous identities or broad searches.
import { FIXTURE_TIME } from './browser-fixtures.mjs'

export const ASSOCIATED_KEY_NAME = 'Same display name · 同名但不同稳定身份 · long-associated-key-name-for-responsive-layout'

export function createConnectionKeyFixture() {
  const state = { pools: 'ready', usage: 'ready', closed: false, deleted: false }
  const now = FIXTURE_TIME / 1000
  const ok = data => ({ status: 200, body: { success: true, data } })
  const unavailable = () => ({ status: 503, body: { success: false } })
  const poolUsage = () => ({
    pool_id: 7, snapshot_id: 70, revision: 2, channel_id: 1,
    channel_name: 'Synthetic subscription connection', window_slot: 'primary',
    window_type: 'five_hour', window_seconds: 18000, plan_type: 'plus', generation: 1,
    state: state.closed ? 'closed' : 'active', reason: state.closed ? 'pool_closed' : 'eligible',
    observed_at: now - 30, reset_at: now + 3600, max_age_seconds: 300, sample_age_seconds: 30,
    allocated_bps: 6000, approximate: true, baseline_time: now - 60,
    baseline_used_micro: 40000000, initial_baseline_time: now - 600,
    initial_baseline_used_micro: 35000000, observed_used_micro: 55000000,
    estimated_used_micro: 15000000, unattributed_micro: 40000000,
    unattributed_growth_micro: 5000000, decomposition_state: 'available',
    pending_attempts: 0, unsampled_attempts: 0, uncertain_attempts: 0,
    allocations: [
      { token_id: 21, token_name: ASSOCIATED_KEY_NAME, token_state: state.deleted ? 'deleted' : 'enabled', share_bps: 2000, estimated_used_micro: 5000000, pending_attempts: 0, unsampled_attempts: 0, uncertain_attempts: 0 },
      { token_id: 22, token_name: ASSOCIATED_KEY_NAME, token_state: 'enabled', share_bps: 4000, estimated_used_micro: 10000000, pending_attempts: 0, unsampled_attempts: 0, uncertain_attempts: 0 },
    ],
  })
  return {
    state,
    response(url, method = 'GET') {
      const path = url.pathname
      if (!path.startsWith('/api/subscription-share/') && !/^\/api\/token\/(21|22)\/subscription-share$/.test(path)) return null
      if (method !== 'GET') return { violation: `Unexpected association mutation: ${method} ${path}` }
      if (path === '/api/subscription-share/channels/1/pools') {
        const page = url.searchParams.get('page')
        if (!['1', '2'].includes(page) || [...url.searchParams.keys()].length !== 1) return { violation: `Unconfigured association page: ${url.search}` }
        if (state.pools === 'error') return unavailable()
        const ids = page === '1' ? Array.from({ length: 20 }, (_, index) => 27 - index) : [7]
        return ok({ channel_id: 1, has_more: state.pools !== 'empty' && page === '1', items: state.pools === 'empty' ? [] : ids.map(poolId => (
          { pool_id: poolId, channel_id: 1, window_source: 'codex_wham_usage_primary', window_type: 'five_hour', enabled: page === '2' && !state.closed }
        )) })
      }
      if (url.search) return { violation: `Unconfigured association query: ${path}${url.search}` }
      if (path === '/api/subscription-share/pools/7/usage') return state.usage === 'error' ? unavailable() : ok(poolUsage())
      if (/^\/api\/token\/(21|22)\/subscription-share$/.test(path)) {
        const tokenId = Number(path.split('/')[3])
        return ok({ token_id: tokenId, pools: [{ ...poolUsage(), window_source: 'codex_wham_usage_primary', share_bps: tokenId === 21 ? 2000 : 4000 }] })
      }
      return { violation: `Unconfigured association identity: ${path}` }
    },
  }
}
