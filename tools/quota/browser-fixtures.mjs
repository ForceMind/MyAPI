// Explicit synthetic fixtures for browser regression only. No production data,
// usable credentials, or upstream calls are used by this harness.
export function quotaFixtures({ latestError = false } = {}) {
  const now = Math.floor(Date.now() / 1000)
  const start = now - 600
  const series = {
    channel_id: 1, name: 'Codex · 浏览器测试', account_label: 'Codex',
    metric_type: 'codex_rate_limit', window_type: 'weekly',
    source: 'codex_wham_usage_primary', plan_type: 'pro',
    window_seconds: 604800, unit: 'percent', currency: '',
  }
  const used = [10, 11, 13, 13, 14, 15]
  const points = used.map((value, index) => ({
    timestamp: start + index * 60, observed_at: start + index * 60,
    status: 'success', used: value, available: 100 - value, total: 100,
    used_source: 'reported', reset_at: now + 86400,
    sample_count: 1, success_count: 1, failed_count: 0,
    ...(index ? { consumption: value - used[index - 1], rate_per_minute: value - used[index - 1],
      peak_rate_per_minute: value - used[index - 1], observed_seconds: 60, interval_count: 1 } : {}),
  }))
  if (latestError) points.push({
    timestamp: start + 360, observed_at: start + 360, status: 'error',
    error_code: 'upstream_transport', event_source: 'codex_wham_usage',
    sample_count: 1, success_count: 0, failed_count: 1, continuity_break: true,
  })
  const consumption = {
    observed: 5, basis: 'used', pair_count: 5, observed_seconds: 300,
    average_rate_per_minute: 1, peak_rate_per_minute: 2,
    peak_rate_observed_at: start + 120, allocation: 'interval_end',
    unit: 'percent', reset_boundaries: 0, recovery_count: 0,
    interrupted_count: latestError ? 1 : 0, gap_count: 0, baseline_change_count: 0,
  }
  const dataQuality = {
    sample_count: points.length, success_count: used.length,
    error_count: latestError ? 1 : 0, invalid_count: 0,
    unsupported_count: 0, reset_boundaries: 0, span_seconds: 300,
    observed_span_seconds: 300,
  }
  const current = points.at(-1)
  const eta = {
    outcome: latestError ? 'insufficient_data' : 'depletes_before_reset',
    ...(latestError ? {} : {
      seconds_to_depletion: 5100,
      estimated_depletion_at: now + 4800,
      reset_at: now + 86400,
      seconds_until_reset: 86700,
    }),
  }
  const rateMethod = (rate, observedSeconds, intervalCount, coverage) => ({
    rate_per_minute: rate,
    rate_per_hour: rate * 60,
    coverage,
    observed_seconds: observedSeconds,
    interval_count: intervalCount,
    observed_at: start + 300,
    eta,
  })
  const analysis = {
    default_method: 'observed_window',
    rate_window_seconds: 3600,
    window_start: now - 3600,
    window_end: now,
    as_of: now,
    ewma_half_life_seconds: 1800,
    complete: true,
    methods: {
      latest_interval: rateMethod(1, 60, 1, 1 / 60),
      observed_window: rateMethod(1, 300, 5, 1 / 12),
      ewma: rateMethod(1.1, 300, 5, 1 / 12),
    },
  }
  const overviewPoints = points.map((point) => ({
    timestamp: point.observed_at,
    available: point.available ?? null,
    continuity_break: Boolean(point.continuity_break),
  }))
  const item = {
    ...series, status: current.status, observed_at: current.observed_at,
    current_available: current.available, current_total: current.total,
    previous_available: 86, previous_observed_at: start + 240,
    change_per_minute: latestError ? null : -1,
    abs_change_per_minute: latestError ? null : 1,
    sample_span_seconds: latestError ? null : 60,
    direction: latestError ? 'unknown' : 'decrease',
    consumption, analysis, data_quality: dataQuality,
    overview_points: overviewPoints,
    peak_abs_change_per_minute: 2, peak_drop_per_minute: 2, peak_increase_per_minute: 0,
  }
  const history = {
    ...series, start, end: now, series_id: 'synthetic-weekly', limit: 5000,
    granularity: 'minute', timezone_offset: -480, points, current,
    raw_observations: points.length, available_points: points.length,
    returned_points: points.length, source_complete: true, points_complete: true,
    complete: true, truncated: false, data_quality: dataQuality, analysis,
    summary: {
      start_available: 90, end_available: 85, change: -5, change_percent: -5.56,
      minimum: 85, maximum: 90, consumption, data_quality: dataQuality,
      used: { start: 10, end: 15, change: 5, minimum: 10, maximum: 15, samples: 6 },
      available: { start: 90, end: 85, change: -5, minimum: 85, maximum: 90, samples: 6 },
      total: { start: 100, end: 100, change: 0, minimum: 100, maximum: 100, samples: 6 },
    },
  }
  const channel = {
    id: 1, name: series.name, type: 57, status: 1, key: '',
    created_time: start, test_time: now, response_time: 100,
    balance: 0, balance_updated_time: now, used_quota: 0,
    models: 'gpt-5-codex', group: 'default', other: '', other_info: '',
    remark: '', settings: '{}', priority: 0,
    channel_info: { is_multi_key: false, multi_key_size: 0, multi_key_polling_index: 0, multi_key_mode: 'random' },
  }
  const ordinary = process.env.MYAPI_BROWSER_ORDINARY === '1'
  const user = { id: ordinary ? 2 : 1, username: ordinary ? 'ordinary-browser-fixture' : 'browser-fixture', display_name: '浏览器回归', role: ordinary ? 1 : 100, status: 1, group: 'default', language: 'zhCN', quota: 1000, used_quota: 0, request_count: 1 }
  const ok = (data) => ({ success: true, data })
  const fundingMode = process.env.MYAPI_BROWSER_FUNDING_MODE || 'disabled'
  const funding = { mode: fundingMode, epoch: 1, ready: true,
    can_top_up: fundingMode === 'enabled', can_redeem: fundingMode === 'enabled',
    can_transfer_affiliate_rewards: fundingMode === 'enabled', can_purchase_subscription: fundingMode === 'enabled',
    can_view_funding_history: true }
  return { item, history, response(url) {
    const path = url.pathname.replace(/\/$/, '')
    if (path === '/api/user/auth/refresh' && process.env.MYAPI_BROWSER_ANONYMOUS === '1') return { success: false, message: 'Synthetic visitor' }
    if (path === '/api/user/auth/refresh') return ok({
      access_token: 'synthetic-browser-session-not-a-credential', token_type: 'Bearer', access_expires_at: now + 3600, user,
      session: { sid: 'synthetic-browser-session', current: true, login_method: 'test', ip: '127.0.0.1', user_agent: 'browser regression', created_at: start, last_active_at: now, expires_at: now + 3600 },
    })
    if (path === '/api/user/self') return ok(user)
    if (path === '/api/setup') return ok({ status: true })
    if (path === '/api/option') return ok([])
    if (path === '/api/ratio_sync/openai/versions') return this.response(new URL('/api/ratio_sync/openai', url))
    if (path.startsWith('/api/ratio_sync/openai/versions/')) {
      if (path.endsWith('/' + 'f'.repeat(64))) return this.response(new URL('/api/ratio_sync/openai', url))
      return { fixture_http_status: 404, success: false, message: 'Synthetic version missing' }
    }
    if (path === '/api/ratio_sync/openai') return ok({
      source_url: 'https://developers.openai.com/api/docs/pricing.md', fetched_at: now,
      content_sha256: 'f'.repeat(64), currency: 'USD', unit_tokens: 1000000,
      service_tier: 'standard', scope: 'text-token-price-source-not-published',
      models: [
        { model: 'fixture-cached-model', source_label: 'fixture-cached-model (<272K context length)',
          short_context: { input_usd_per_million: '2.00', cached_input_usd_per_million: '0.00', cache_write_usd_per_million: '2.50', output_usd_per_million: '10.00' },
          long_context: { input_usd_per_million: '4.00', cached_input_usd_per_million: '0.20', cache_write_usd_per_million: '5.00', output_usd_per_million: '15.00' } },
        { model: 'fixture-no-cache', source_label: 'fixture-no-cache',
          short_context: { input_usd_per_million: '0.005', cached_input_usd_per_million: null, cache_write_usd_per_million: null, output_usd_per_million: '0.25' }, long_context: null },
      ],
    })
    if (path === '/api/home_page_content') return ok('')
    if (path === '/api/status') return ok({ system_name: 'MyAPI', version: 'browser-fixture', start_time: start, api_info_enabled: false, announcements_enabled: false, faq_enabled: false, uptime_kuma_enabled: false, quota_per_unit: 500000, display_in_currency: false, user_funding_mode: fundingMode, user_funding_capabilities: funding })
    if (path === '/api/user/topup/info') return ok({ user_funding_mode: fundingMode, user_funding_capabilities: funding, min_topup: 1, payment_types: [], payment_compliance_confirmed: false })
    if (path === '/api/user/aff') return ok('synthetic-history-no-new-referral')
    if (path === '/api/subscription/self') return ok({ subscriptions: [], expired_subscriptions: [], billing_preference: 'wallet_only' })
    if (path === '/api/user/topup/self' || path === '/api/user/topup') return ok({ items: [{ id:1, user_id:1, trade_no:'synthetic-old-order', amount:1, money:1, payment_method:'stripe', create_time:now-40*86400, complete_time:now-40*86400, status:'success' }], total:1, page:1, page_size:10 })
    if (path === '/api/channel/quota/status') return ok({ enabled: true, interval_seconds: 60, max_channels: 2 })
    if (path === '/api/channel/codex/local-auth/status') return ok({
      state: 'ready', platform: 'darwin', environment: 'native',
      codex_installed: true, auth_file_exists: true, auth_readable: true,
      logged_in: true, auto_import_available: true, manual_import_available: true,
      account_hint: 'acct…1234', email_hint: 'b***@example.com',
      last_refresh: new Date((now - 300) * 1000).toISOString(), can_refresh: true,
    })
    if (path === '/api/channel/quota/changes') {
      const requestedOverviewPoints = Number(url.searchParams.get('overview_points') || 0)
      const overviewItem = {
        ...item,
        ...(requestedOverviewPoints > 0 ? {} : { overview_points: undefined }),
      }
      const multiSeries = process.env.MYAPI_BROWSER_MULTISERIES === '1'
      return ok({
        items: multiSeries ? [
          { ...overviewItem, name: 'Shared Codex channel', account_label: 'Shared Codex channel', series_id: 'a'.repeat(64) },
          { ...overviewItem, name: 'Shared Codex channel', account_label: 'Shared Codex channel', series_id: 'b'.repeat(64), current_available: 58,
            overview_points: overviewItem.overview_points?.map((point) => ({ ...point, available: point.available == null ? null : point.available - 27 })) },
          { ...overviewItem, channel_id: 2, name: 'Other provider channel', account_label: 'Other account', series_id: 'c'.repeat(64), current_available: 41,
            overview_points: overviewItem.overview_points?.map((point) => ({ ...point, available: point.available == null ? null : point.available - 44 })) },
        ] : [overviewItem],
        range: url.searchParams.get('range') || '24h',
        start: now - 24 * 60 * 60,
        end: now,
        generated_at: now,
        rate_window_seconds: Number(url.searchParams.get('rate_window') || 86400),
        ewma_half_life_seconds: Number(url.searchParams.get('ewma_half_life') || 43200),
        data_quality: dataQuality,
      })
    }
    if (path === '/api/channel/1/quota/history') return ok({ ...history, granularity: url.searchParams.get('granularity') || 'auto' })
    if (path === '/api/channel/1/codex/usage') return ok({ plan_type: 'pro', rate_limit: { allowed: true, limit_reached: false, primary_window: { used_percent: 15, reset_at: now + 86400, limit_window_seconds: 604800 } } })
    if (path === '/api/channel/1/codex/usage/reset-credits') return ok({ credits: [], available_count: 0 })
    if (path === '/api/channel/ops') return ok({ retry_times: 0 })
    if (path === '/api/channel/1') return ok(channel)
    if (path === '/api/channel' || path === '/api/channel/search') return ok({ items: [channel], total: 1, page: 1, page_size: 10, type_counts: { 57: 1 } })
    if (path === '/api/group') return ok(['default'])
    if (path === '/api/user/self/groups') return ok({ default: { ratio: 1, desc: '测试分组' } })
    if (path === '/api/user/models') return ok(['gpt-5-codex'])
    if (path === '/api/channel/models') return ok(['gpt-5-codex'])
    if (path === '/api/prefill_group') return ok([])
    if (path === '/api/user/2fa/status') return ok({ enabled: false })
    if (path === '/api/user/passkey') return ok({ enabled: false, credentials: [] })
    if (path === '/api/token' || path === '/api/token/search') return ok({ items: [], total: 0, page: 1, page_size: 10 })
    if (path.startsWith('/api/data') || path === '/api/uptime/status') return ok([])
    if (path === '/api/log' || path === '/api/log/self') return ok({
      items: ['reported', 'estimated', 'unknown', null].map((accuracy, index) => ({
        id: 10 + index, user_id: 1, created_at: now - index * 60,
        type: 2, content: 'Synthetic usage provenance fixture', username: 'browser-fixture',
        token_name: 'fixture-key', model_name: `fixture-${accuracy || 'legacy'}`,
        quota: accuracy === 'unknown' ? 0 : 12500,
        prompt_tokens: accuracy === 'unknown' ? 0 : 100,
        completion_tokens: accuracy === 'unknown' ? 0 : 10,
        use_time: 1, is_stream: true, channel: 1, channel_name: 'Codex Fixture',
        token_id: 1, group: 'default', ip: '',
        other: JSON.stringify({ ...(accuracy ? { usage_accuracy: accuracy } : {}),
          cache_tokens: accuracy === 'unknown' ? 0 : 40, reasoning_tokens: accuracy === 'unknown' ? 0 : 4,
          model_ratio: 1, completion_ratio: 2, cache_ratio: 0.1, group_ratio: 1 }),
      })), total: 4, page: 1, page_size: 20,
    })
    if (path === '/api/log/stat' || path === '/api/log/self/stat') return ok({ quota: 0, rpm: 0, tpm: 0, count: 0 })
    if (path === '/api/log/overview') return ok({
      requests: [{ id: 1, created_at: now - 120, model_name: 'gpt-5-codex', username: 'browser-fixture' }],
      errors: [{ id: 2, created_at: now - 60, model_name: 'failed-model', username: 'browser-fixture' }],
    })
    if (path === '/api/log/self/overview') return ok({
      requests: [{ id: 1, created_at: now - 120, model_name: 'gpt-5-codex' }],
      errors: [{ id: 2, created_at: now - 60, model_name: 'failed-model' }],
    })
    if (path === '/api/perf-metrics/summary') return ok({ models: [] })
    if (path === '/api/notice') return ok('')
    return null
  } }
}
