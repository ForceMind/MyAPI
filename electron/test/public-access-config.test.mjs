import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import vm from 'node:vm'
import config from '../public-access-config.js'

test('HTTPS root declarations normalize only their origin and generate one client suffix without verification claims', () => {
  for (const [input, origin] of [
    ['https://PUBLIC.example.test/', 'https://public.example.test'],
    [' https://public.example.test:8443 ', 'https://public.example.test:8443'],
    ['https://example.com', 'https://example.com'],
    ['https://203.0.113.8', 'https://203.0.113.8'],
  ]) {
    assert.deepEqual(config.validatePublicEntry(input), { ok: true, code: 'valid', origin, baseURL: `${origin}/v1` })
  }
})

test('credentials, paths, query markers and non-HTTPS declarations are rejected rather than silently removed', () => {
  const cases = [
    [undefined, 'invalid'], ['', 'empty'], ['a'.repeat(2049), 'too-long'],
    ['https://public.example.test\n', 'control-character'], ['https:\\public.example.test', 'backslash'],
    ['http://public.example.test', 'scheme'], ['javascript:alert(1)', 'scheme'], ['file:///tmp/config', 'scheme'],
    ['https://@public.example.test', 'credentials'], ['https://user:secret@public.example.test', 'credentials'],
    ['https://public.example.test?', 'query-fragment'], ['https://public.example.test#', 'query-fragment'],
    ['https://public.example.test?key=secret', 'query-fragment'],
    ['https://public.example.test/v1', 'path'], ['https://public.example.test/prefix', 'path'],
    ['https://public.example.test/.', 'path'], ['https://public.example.test/%2f', 'path'],
  ]
  for (const [input, code] of cases) {
    assert.deepEqual(config.validatePublicEntry(input), { ok: false, code }, String(input))
  }
})

test('known loopback and private names return to the local or LAN branch without DNS lookup', () => {
  for (const host of ['localhost', 'localhost.', 'panel.localhost.', 'myapi.local', 'myapi.local.', '127.1', '10.0.0.7', '172.16.4.2', '192.168.1.2', '169.254.1.2', '[::1]', '[fd00::1]', '[::ffff:192.168.1.2]']) {
    assert.deepEqual(config.validatePublicEntry(`https://${host}`), { ok: false, code: 'local-or-lan' }, host)
  }
  assert.deepEqual(config.validatePublicEntry('http://127.0.0.1:3000'), { ok: false, code: 'local-or-lan' })
})

test('IDN normalization must remain within the saved-origin limit rather than expand beyond copy and reload bounds', () => {
  const value = `https://${Array(260).fill('é').join('.')}.test`
  assert.ok(value.length < config.MAX_PUBLIC_ORIGIN_LENGTH)
  assert.ok(new URL(value).origin.length > config.MAX_PUBLIC_ORIGIN_LENGTH)
  assert.deepEqual(config.validatePublicEntry(value), { ok: false, code: 'too-long' })
})

test('proxy targets are exact local root URLs, distinct from public entries and client Base URLs', () => {
  for (const root of ['http://127.0.0.1:3000', 'http://192.168.1.20:4317', 'https://localhost:8443']) {
    assert.equal(config.validateProxyTarget(root), true, root)
  }
  for (const root of ['http://127.0.0.1:3000/v1', 'http://127.0.0.1:3000/', 'https://public.example.test', 'http://user@127.0.0.1:3000', 'http://127.0.0.1:3000?', 'file:///tmp/data']) {
    assert.equal(config.validateProxyTarget(root), false, root)
  }
})

test('the same permission-free browser source exposes the exact CJS validation contract', async () => {
  const source = await readFile(new URL('../public-access-config.js', import.meta.url), 'utf8')
  const window = {}
  vm.runInNewContext(source, { window, URL })
  assert.deepEqual(Object.keys(window.MyAPIPublicAccess).sort(), Object.keys(config).sort())
  for (const value of ['https://public.example.test', 'https://user@public.example.test', 'https://localhost.', 'https://public.example.test/v1']) {
    assert.equal(JSON.stringify(window.MyAPIPublicAccess.validatePublicEntry(value)), JSON.stringify(config.validatePublicEntry(value)))
  }
})
