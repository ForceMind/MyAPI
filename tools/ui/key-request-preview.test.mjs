import assert from 'node:assert/strict'
import test from 'node:test'
import { createKeyRequestFixture } from './key-request-preview.mjs'

const url = path => new URL(path, 'http://synthetic.invalid')

test('the preview keeps same-name Keys and unidentified history out of a stable Key request scope', () => {
  const fixture = createKeyRequestFixture()
  const list = fixture.response(url('/api/log?token_id=21&page_size=20')).body.data
  assert.deepEqual(list.items.map(row => row.request_id), ['SYNTHETIC-CONTEXT-ERROR', 'SYNTHETIC-COMPLETED-NO-USAGE', 'SYNTHETIC-NOT-RECORDED', 'SYNTHETIC-PREFLIGHT-REJECTED'])
  assert.equal(list.total, 4)
  assert.equal(list.items.every(row => row.token_id === 21), true)
  const stat = fixture.response(url('/api/log/stat?token_id=21')).body.data.token_usage
  assert.equal(stat.records, 4)
  assert.equal(stat.unknown_records, 3)
  assert.equal(stat.total_tokens, null)
  assert.equal(stat.known_total_tokens, '120')
  const second = fixture.response(url('/api/log?token_id=21&p=2&page_size=1')).body.data
  assert.equal(second.items[0].request_id, 'SYNTHETIC-COMPLETED-NO-USAGE')
})

test('the preview exposes saved stream errors and missing-body responses without replaying requests', () => {
  const fixture = createKeyRequestFixture()
  const saved = fixture.response(url('/api/full-content-logs/SYNTHETIC-CONTEXT-ERROR'))
  assert.equal(saved.status, 200)
  assert.equal(saved.body.data.status, 200)
  assert.match(saved.body.data.response_body, /context_length_exceeded/)
  assert.equal(fixture.response(url('/api/full-content-logs/SYNTHETIC-NOT-RECORDED')).status, 404)
  for (const path of ['/api/token/21', '/api/usage-review/SYNTHETIC-CONTEXT-ERROR/reconcile', '/v1/chat/completions', '/pg/chat/completions']) {
    assert.equal(fixture.response(url(path), 'POST').status, 405)
  }
})

test('the preview rejects invalid or duplicate Key IDs instead of returning unfiltered requests', () => {
  const fixture = createKeyRequestFixture()
  for (const query of ['token_id=0', 'token_id=-1', 'token_id=2147483648', 'token_id=21&token_id=22']) {
    assert.equal(fixture.response(url('/api/log?' + query)).status, 400)
  }
})
