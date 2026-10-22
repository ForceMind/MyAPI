import assert from 'node:assert/strict'
import test from 'node:test'
import { connectionLocation, createConnectionKeyFixture } from './connection-key-browser-fixtures.mjs'

const url = path => new URL(path, 'http://synthetic.invalid')

test('connection return ignores query ordering without dropping identity, filters or duplicate values', () => {
  const origin = 'http://synthetic.invalid'
  const expected = connectionLocation('/connections?filter=Synthetic&page=2&status=enabled&connection=1', origin)
  assert.deepEqual(connectionLocation('/connections?page=2&filter=Synthetic&status=enabled&connection=1', origin), expected)
  for (const path of [
    '/connections?filter=Synthetic&page=2&status=enabled&connection=2',
    '/connections?filter=Other&page=2&status=enabled&connection=1',
    '/connections?filter=Synthetic&page=3&status=enabled&connection=1',
    '/connections?filter=Synthetic&page=2&status=disabled&connection=1',
    '/connections?filter=Synthetic&page=2&status=enabled&connection=1&connection=2',
    '/connections?filter=Synthetic&page=2&status=enabled&connection=1#different',
    '/keys?filter=Synthetic&page=2&status=enabled&connection=1',
    'https://other.invalid/connections?filter=Synthetic&page=2&status=enabled&connection=1',
  ]) assert.notDeepEqual(connectionLocation(path, origin), expected, path)
})

test('association pages retain channel and pool identities rather than display names', () => {
  const fixture = createConnectionKeyFixture()
  const first = fixture.response(url('/api/subscription-share/channels/1/pools?page=1')).body.data
  const second = fixture.response(url('/api/subscription-share/channels/1/pools?page=2')).body.data
  assert.equal(first.channel_id, 1)
  assert.equal(first.has_more, true)
  assert.deepEqual(first.items.map(row => row.pool_id), Array.from({ length: 20 }, (_, index) => 27 - index))
  assert.equal(second.has_more, false)
  assert.equal(second.items[0].pool_id, 7)
  const usage = fixture.response(url('/api/subscription-share/pools/7/usage')).body.data
  assert.equal(usage.allocations[0].token_name, usage.allocations[1].token_name)
  assert.deepEqual(usage.allocations.map(row => row.token_id), [21, 22])
  assert.equal(usage.allocations.reduce((sum, row) => sum + row.share_bps, 0), usage.allocated_bps)
  assert.equal(usage.allocations.reduce((sum, row) => sum + row.estimated_used_micro, 0), usage.estimated_used_micro)
})

test('association fixtures reject writes, scope drift and ambiguous query parameters', () => {
  const fixture = createConnectionKeyFixture()
  for (const [method, path] of [
    ['PUT', '/api/token/21/subscription-share'],
    ['POST', '/api/subscription-share/pools/7/close'],
    ['GET', '/api/subscription-share/channels/2/pools?page=1'],
    ['GET', '/api/subscription-share/channels/1/pools?page=1&page=2'],
    ['GET', '/api/subscription-share/channels/1/pools?page=1&token_id=21'],
    ['GET', '/api/subscription-share/pools/8/usage'],
    ['GET', '/api/subscription-share/pools/7/usage?channel=2'],
  ]) assert(fixture.response(url(path), method).violation, `${method} ${path}`)
  assert.equal(fixture.response(url('/api/token/21/key'), 'POST'), null)
})

test('explicit empty, failure and historical states do not invent current allocation access', () => {
  const fixture = createConnectionKeyFixture()
  fixture.state.pools = 'empty'
  assert.deepEqual(fixture.response(url('/api/subscription-share/channels/1/pools?page=1')).body.data.items, [])
  fixture.state.pools = 'error'
  assert.equal(fixture.response(url('/api/subscription-share/channels/1/pools?page=1')).status, 503)
  fixture.state.usage = 'error'
  assert.equal(fixture.response(url('/api/subscription-share/pools/7/usage')).status, 503)
  fixture.state.usage = 'ready'
  fixture.state.closed = true
  fixture.state.deleted = true
  const view = fixture.response(url('/api/subscription-share/pools/7/usage')).body.data
  assert.equal(view.state, 'closed')
  assert.equal(view.allocations[0].token_state, 'deleted')
  assert.equal(view.allocations[0].estimated_used_micro, 5000000)
})
