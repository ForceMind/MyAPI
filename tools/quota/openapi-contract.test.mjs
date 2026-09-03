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
  const changes = new Map(api.paths[paths[1]].get.parameters.map((p) => [p.name, p]))
  assert.equal(changes.get('limit').schema.maximum, undefined, 'large changes limits are clamped, not rejected')
  assert.match(changes.get('limit').description, /2000/)
  assert.deepEqual(history.get('range').schema.enum, ['1h', '6h', '24h', '1d', '7d', '30d', '90d', 'custom'])
})
