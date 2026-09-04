import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const api = JSON.parse(readFileSync(new URL('../../docs/openapi/api.json', import.meta.url), 'utf8'))
const paths = [
  '/api/channel/quota/status',
  '/api/channel/quota/changes',
  '/api/channel/{id}/quota/history',
]

test('quota OpenAPI documents authenticated reads and business failure envelopes', () => {
  for (const path of paths) {
    const operation = api.paths[path].get
    assert.deepEqual(operation.security, [{ AccessToken1: [] }])
    assert.match(operation.description, /channel\.read/)
    const variants = operation.responses['200'].content['application/json'].schema.oneOf
    assert.equal(variants.length, 2)
    assert.deepEqual(variants[0].properties.success.enum, [true])
    assert.deepEqual(variants[0].required, ['success', 'data'])
    assert.equal(variants[1].$ref, '#/components/schemas/QuotaBusinessError')
  }
  assert.deepEqual(api.components.schemas.QuotaBusinessError.properties.success.enum, [false])
})

test('quota schemas resolve references without confusing omitted values and null summaries', () => {
  const pending = paths.map((path) => api.paths[path])
  const visited = new Set()
  while (pending.length) {
    const value = pending.pop()
    if (!value || typeof value !== 'object') continue
    if (value.$ref) {
      const prefix = '#/components/schemas/'
      assert.ok(value.$ref.startsWith(prefix))
      const name = value.$ref.slice(prefix.length)
      assert.ok(api.components.schemas[name], `missing schema ${name}`)
      if (!visited.has(name)) {
        visited.add(name)
        pending.push(api.components.schemas[name])
      }
    }
    pending.push(...Object.values(value))
  }
  assert.ok(visited.has('QuotaHistoryPoint'))
  const point = api.components.schemas.QuotaHistoryPoint
  assert.ok(!point.required.includes('consumption'))
  assert.notEqual(point.properties.consumption.nullable, true)
  assert.equal(api.components.schemas.QuotaChangesSummary.properties.max_drop_per_minute.nullable, true)
  const history = api.components.schemas.QuotaHistoryData
  for (const name of ['source_complete', 'points_complete', 'complete', 'truncated']) {
    assert.ok(history.required.includes(name))
  }
})

test('quota query contract retains granularities, precision and distinct limit policies', () => {
  const history = new Map(api.paths[paths[2]].get.parameters.map((p) => [p.name, p]))
  assert.deepEqual(history.get('granularity').schema.enum, ['raw', 'minute', '5m', '15m', 'hour', 'day', 'week', 'auto'])
  assert.equal(history.get('limit').schema.minimum, 1)
  assert.equal(history.get('limit').schema.maximum, 5000)
  assert.equal(history.get('timezone_offset').schema.minimum, -840)
  assert.equal(history.get('timezone_offset').schema.maximum, 840)
  assert.equal(history.get('id').in, 'path')
  assert.equal(history.get('id').required, true)
  for (const name of ['metric_type', 'window_type', 'source', 'plan_type', 'unit', 'currency', 'window_seconds']) {
    assert.equal(history.get(name).in, 'query')
  }
  for (const name of ['rate_window', 'ewma_half_life']) {
    assert.equal(history.get(name).in, 'query')
    assert.match(history.get(name).description, /seconds|preset/i)
    const numeric = history.get(name).schema.oneOf.find((variant) => variant.type === 'integer')
    assert.equal(numeric.minimum, 1)
    assert.equal(numeric.maximum, 15552000)
  }
  const changes = new Map(api.paths[paths[1]].get.parameters.map((p) => [p.name, p]))
  assert.equal(changes.get('limit').schema.maximum, undefined, 'large changes limits are clamped, not rejected')
  assert.match(changes.get('limit').description, /2000/)
  assert.equal(changes.get('overview_points').schema.minimum, 1)
  assert.equal(changes.get('overview_points').schema.maximum, 120)
  for (const name of ['rate_window', 'ewma_half_life']) {
    assert.equal(changes.get(name).in, 'query')
    const numeric = changes.get(name).schema.oneOf.find((variant) => variant.type === 'integer')
    assert.equal(numeric.minimum, 1)
    assert.equal(numeric.maximum, 15552000)
  }
  assert.deepEqual(history.get('range').schema.enum, ['1h', '6h', '24h', '1d', '7d', '30d', '90d', 'custom'])
})

test('quota analysis exposes three complete rate methods and reset-aware ETA outcomes', () => {
  const analysis = api.components.schemas.QuotaAnalysis
  assert.deepEqual(analysis.required, [
    'default_method',
    'rate_window_seconds',
    'window_start',
    'window_end',
    'as_of',
    'ewma_half_life_seconds',
    'complete',
    'methods',
  ])
  assert.deepEqual(analysis.properties.default_method.enum, ['observed_window'])
  const methods = api.components.schemas.QuotaRateMethods
  assert.deepEqual(methods.required, ['latest_interval', 'observed_window', 'ewma'])
  for (const name of methods.required) {
    assert.equal(methods.properties[name].$ref, '#/components/schemas/QuotaRateMethodAnalysis')
  }
  const method = api.components.schemas.QuotaRateMethodAnalysis
  for (const name of ['rate_per_minute', 'rate_per_hour', 'coverage', 'observed_seconds', 'interval_count', 'observed_at', 'eta']) {
    assert.ok(method.required.includes(name))
  }
  assert.equal(method.properties.rate_per_minute.nullable, true)
  assert.equal(method.properties.rate_per_hour.nullable, true)
  assert.equal(method.properties.observed_at.nullable, true)
  assert.match(analysis.description, /latest_interval.*60.*consumption.*observed_seconds/i)
  assert.match(analysis.description, /observed_window.*sum.*consumption.*sum.*observed_seconds/i)
  assert.match(analysis.description, /lambda=ln\(2\).*integral/i)
  const outcomes = api.components.schemas.QuotaETA.properties.outcome.enum
  assert.deepEqual(outcomes, [
    'depletes_before_reset',
    'reset_before_depletion',
    'stable_or_no_observed_consumption',
    'insufficient_data',
  ])
  assert.match(api.components.schemas.QuotaETA.description, /仅 depletes_before_reset.*reset_before_depletion/)
  assert.match(api.components.schemas.QuotaETA.description, /同刻.*reset/i)
  assert.equal(api.components.schemas.QuotaHistoryData.properties.analysis.$ref, '#/components/schemas/QuotaAnalysis')
  assert.equal(api.components.schemas.QuotaChangeItem.properties.analysis.$ref, '#/components/schemas/QuotaAnalysis')
  assert.ok(!api.components.schemas.QuotaChangeItem.required.includes('overview_points'))
  assert.equal(api.components.schemas.QuotaChangeItem.properties.overview_points.items.$ref, '#/components/schemas/QuotaOverviewPoint')
  assert.deepEqual(api.components.schemas.QuotaOverviewPoint.required, [
    'timestamp',
    'available',
    'continuity_break',
  ])
  assert.equal(api.components.schemas.QuotaOverviewPoint.properties.available.nullable, true)
  assert.equal(api.components.schemas.QuotaHistorySummary.properties.drop_rate_per_day.deprecated, true)
  assert.equal(api.components.schemas.QuotaHistorySummary.properties.forecast_zero_at.deprecated, true)
  assert.equal(api.components.schemas.QuotaHistorySummary.properties.forecast_confidence.deprecated, true)
  assert.match(api.components.schemas.QuotaChangesData.description, /limit.*analysis/i)
})
