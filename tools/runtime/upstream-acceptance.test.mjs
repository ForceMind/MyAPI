import test from 'node:test'
import assert from 'node:assert/strict'
import { acceptanceControl } from './upstream-acceptance.mjs'

test('rejects remote destinations and ambiguous run IDs before sending the session', async () => {
  let requests = 0
  const fetchImpl = async () => { requests++; throw new Error('must not send') }
  for (const baseURL of ['https://example.com', 'http://localhost:3000', 'http://127.0.0.1:3000/path', 'http://user:pass@127.0.0.1:3000']) {
    await assert.rejects(acceptanceControl({ baseURL, accessToken: 'synthetic-session', action: 'status', runID: 'run', fetchImpl }))
  }
  await assert.rejects(acceptanceControl({ baseURL: 'http://127.0.0.1:3000', accessToken: 'synthetic-session', action: 'stop', runID: '../other', fetchImpl }))
  assert.equal(requests, 0)
})

test('creates only with explicit budgets and disables redirects', async () => {
  const spec = { channel_id: 3, limits: { duration_seconds: 60, max_total: 2, max_sample: 2 } }
  const run = await acceptanceControl({ baseURL: 'http://127.0.0.1:3000', accessToken: 'synthetic-session', action: 'create', spec,
    fetchImpl: async (url, options) => {
      assert.equal(url.href, 'http://127.0.0.1:3000/api/upstream-acceptance/runs')
      assert.equal(options.method, 'POST')
      assert.equal(options.redirect, 'error')
      assert.equal(options.headers.Origin, 'http://127.0.0.1:3000')
      assert.deepEqual(JSON.parse(options.body), spec)
      return { ok: true, json: async () => ({ success: true, data: { id: 'synthetic-run', total: 0 } }) }
    } })
  assert.deepEqual(run, { id: 'synthetic-run', total: 0 })
})

test('does not misrepresent stopping as drained or retry unknown writes', async () => {
  let requests = 0
  const status = await acceptanceControl({ baseURL: 'http://127.0.0.1:3000', accessToken: 'synthetic-session', action: 'stop', runID: 'synthetic-run',
    fetchImpl: async (_url, options) => {
      requests++
      assert.equal(options.method, 'POST')
      return { ok: true, json: async () => ({ success: true, data: { run: { status: 'stopping' }, operations: 1 } }) }
    } })
  assert.equal(status.run.status, 'stopping')
  assert.equal(requests, 1)
  await assert.rejects(acceptanceControl({ baseURL: 'http://127.0.0.1:3000', accessToken: 'synthetic-session', action: 'stop', runID: 'synthetic-run',
    fetchImpl: async () => { requests++; throw new Error('synthetic network failure') } }))
  assert.equal(requests, 2)
})
