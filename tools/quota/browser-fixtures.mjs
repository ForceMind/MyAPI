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
  const user = { id: 1, username: 'browser-fixture', display_name: '浏览器回归', role: 100, status: 1, group: 'default', language: 'zhCN', quota: 1000, used_quota: 0, request_count: 1 }
  const ok = (data) => ({ success: true, data })
  return { item, history, response(url) {
    const path = url.pathname.replace(/\/$/, '')
    if (path === '/api/user/auth/refresh') return ok({
      access_token: 'synthetic-browser-session-not-a-credential', token_type: 'Bearer', access_expires_at: now + 3600, user,
      session: { sid: 'synthetic-browser-session', current: true, login_method: 'test', ip: '127.0.0.1', user_agent: 'browser regression', created_at: start, last_active_at: now, expires_at: now + 3600 },
    })
    if (path === '/api/user/self') return ok(user)
    if (path === '/api/setup') return ok({ status: true })
    if (path === '/api/status') return ok({ system_name: 'MyAPI', version: 'browser-fixture', start_time: start, api_info_enabled: false, announcements_enabled: false, faq_enabled: false, uptime_kuma_enabled: false, quota_per_unit: 500000, display_in_currency: false })
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
      return ok({
        items: [{
          ...item,
          ...(requestedOverviewPoints > 0 ? {} : { overview_points: undefined }),
        }],
        range: url.searchParams.get('range') || '24h',
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
    if (path === '/api/log/stat' || path === '/api/log/self/stat') return ok({ quota: 0, rpm: 0, tpm: 0, count: 0 })
    if (path === '/api/perf-metrics/summary') return ok({ models: [] })
    if (path === '/api/notice') return ok('')
    return null
  } }
}
