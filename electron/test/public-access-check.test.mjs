import assert from 'node:assert/strict'
import { EventEmitter } from 'node:events'
import { execFileSync } from 'node:child_process'
import { mkdtemp, chmod, readFile, rm, writeFile } from 'node:fs/promises'
import http from 'node:http'
import https from 'node:https'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'
import checker from '../public-access-check.js'

const origin = 'https://public.example.test'
const privateText = 'SYNTHETIC_PRIVATE_RESPONSE_ERROR'
const now = Date.parse('2030-06-01T12:00:00Z')
const statusBody = { success: true, data: { version: '0.2.0-beta.53', product_edition: 'lite', ignored: privateText } }
const certificate = { subject: { CN: 'public.example.test' }, subjectaltname: 'DNS:public.example.test',
  valid_from: 'Jan 1 00:00:00 2020 GMT', valid_to: 'Jan 1 00:00:00 2040 GMT' }

function deferred() {
  let resolve
  const promise = new Promise(done => { resolve = done })
  return { promise, resolve }
}

function fixture(settings = {}) {
  const records = settings.records ?? [{ address: '8.8.8.8', family: 4 }]
  const lookupStarted = deferred(), socketStarted = deferred(), responseStarted = deferred()
  const requests = [], lookups = [], waiting = new Map(), delays = []
  let timerId = 0
  const timers = {
    setTimeout(callback, delay) { const id = timerId++; waiting.set(id, callback); delays.push(delay); return id },
    clearTimeout(id) { waiting.delete(id) },
  }
  const lookup = (hostname, options) => {
    lookups.push({ hostname, options }); lookupStarted.resolve()
    if (settings.dnsError) return Promise.reject(Object.assign(new Error(privateText), { code: 'ENOTFOUND' }))
    return settings.lookup ? settings.lookup() : Promise.resolve(records)
  }
  const request = (options, callback) => {
    const outgoing = new EventEmitter(), response = new EventEmitter(), socket = new EventEmitter()
    Object.assign(socket, { encrypted: true, authorized: settings.authorized ?? true,
      authorizationError: settings.authorizationError, getPeerCertificate: () => structuredClone(settings.certificate ?? certificate) })
    Object.assign(response, { statusCode: settings.status ?? 200, complete: false, destroyed: false,
      headers: { location: 'https://127.0.0.1/private?' + privateText } })
    const record = { options, outgoing, response, socket, ends: 0, destroyed: false }
    requests.push(record)
    response.destroy = () => { response.destroyed = true; response.emit('close') }
    outgoing.destroy = () => { record.destroyed = true }
    outgoing.end = (...args) => {
      assert.deepEqual(args, [], 'the fixed GET must not send a body')
      record.ends++
      queueMicrotask(() => {
        if (record.destroyed) return
        callback(response); responseStarted.resolve()
        if (response.destroyed || settings.hangResponse) return
        const bytes = settings.body === undefined ? Buffer.from(JSON.stringify(statusBody)) : Buffer.from(settings.body)
        response.emit('data', bytes)
        if (!response.destroyed) { response.complete = settings.complete ?? true; response.emit('end') }
      })
    }
    queueMicrotask(() => {
      if (record.destroyed) return
      if (settings.requestError) { outgoing.emit('error', Object.assign(new Error(privateText), { code: settings.requestError })); return }
      options.lookup(options.hostname, { family: options.family, all: true }, (error, addresses) => {
        if (record.destroyed) return
        if (error) { outgoing.emit('error', error); return }
        record.connectedAddresses = addresses
        outgoing.emit('socket', socket); socketStarted.resolve()
        if (!settings.hangTLS && !record.destroyed) socket.emit('secureConnect')
      })
    })
    return outgoing
  }
  const instance = checker.createPublicAccessCheck({ lookup, request, timers, now: () => now })
  return { instance, records, requests, lookups, lookupStarted, socketStarted, responseStarted, waiting, delays,
    deadline() { for (const callback of [...waiting.values()]) callback() } }
}

function limitedClaims(result) {
  assert.equal(result.source, 'this-computer')
  assert.equal(result.externalVerification, 'unknown')
  assert.equal(result.sameInstanceVerified, false)
  assert.equal(result.modelVerified, false)
  assert.equal(result.quotaVerified, false)
  assert.equal(JSON.stringify(result).includes(privateText), false)
}

test('construction is inert and only an accepted explicit root entry can begin network work', async () => {
  const f = fixture()
  assert.deepEqual(f.lookups, [])
  assert.deepEqual(f.requests, [])
  for (const dependencies of [{ allowPrivate: true }, { ca: 'caller-trust' }, { rejectUnauthorized: false }, { lookup: null }, { request: false }, { now: 0 }, { timers: null }]) {
    assert.throws(() => checker.createPublicAccessCheck(dependencies), /Invalid public entry check dependencies/)
  }
  for (const entry of [undefined, '', 'http://public.example.test', 'https://user:secret@public.example.test', 'https://public.example.test/v1', 'https://localhost']) {
    const result = await f.instance.check(entry)
    assert.equal(result.code, 'invalid-entry')
    assert.equal(result.origin, null)
    limitedClaims(result)
  }
  for (const options of [null, [], { signal: {} }, { onProgress: 1 }, { method: 'POST' }, { headers: { Authorization: privateText } }, { allowPrivate: true }]) {
    assert.equal((await f.instance.check(origin, options)).code, 'invalid-options')
  }
  assert.deepEqual(f.lookups, [])
  assert.deepEqual(f.requests, [])
})

test('one validated destination, trusted original hostname and fixed credential-free GET produce only safe metadata', async () => {
  const f = fixture(), progress = []
  const result = await f.instance.check(origin, { onProgress: item => { progress.push(structuredClone(item)); item.origin = 'changed'; item.externalVerification = 'verified' } })
  assert.equal(result.outcome, 'passed')
  assert.equal(result.code, 'ok')
  assert.equal(result.origin, origin)
  assert.equal(result.checkedAt, now)
  assert.deepEqual(result.status, { state: 'passed', version: '0.2.0-beta.53', productEdition: 'lite' })
  assert.equal(f.lookups.length, 1)
  assert.deepEqual(f.lookups[0], { hostname: 'public.example.test', options: { all: true, verbatim: true } })
  assert.equal(f.requests.length, 1)
  const options = f.requests[0].options
  assert.equal(options.hostname, 'public.example.test')
  assert.equal(options.servername, 'public.example.test')
  assert.equal(options.method, 'GET')
  assert.equal(options.path, '/api/status')
  assert.equal(options.agent, false)
  assert.equal(options.rejectUnauthorized, true)
  assert.deepEqual(options.headers, { Accept: 'application/json' })
  for (const name of ['auth', 'ca', 'cert', 'key', 'pfx', 'proxy']) assert.equal(Object.hasOwn(options, name), false)
  assert.equal(f.requests[0].ends, 1)
  assert.equal(f.requests[0].destroyed, true)
  assert.equal(f.waiting.size, 0)
  assert.equal(progress.every(item => item.outcome === 'unknown' && item.code === 'checking' && item.checkedAt === null), true)
  assert.equal(progress.some(item => item.dns.state === 'checking' && item.tls.state === 'not-run'), true)
  assert.equal(progress.some(item => item.tls.state === 'checking' && item.status.state === 'not-run'), true)
  assert.equal(progress.some(item => item.status.state === 'checking'), true)
  limitedClaims(result)
  const second = await f.instance.check(origin)
  assert.equal(second.outcome, 'passed')
  assert.equal(f.lookups.length, 2, 'an explicit subsequent check must not use a DNS/result cache')
  assert.equal(f.requests.length, 2)
})

test('all DNS answers must be ordinary public addresses, including mixed answers and IPv6 special-purpose ranges', async () => {
  const blocked = [
    ['0.0.0.1', 4], ['10.1.2.3', 4], ['100.64.0.1', 4], ['127.0.0.1', 4], ['169.254.169.254', 4],
    ['172.31.1.1', 4], ['192.0.0.9', 4], ['192.0.2.1', 4], ['192.31.196.1', 4], ['192.52.193.1', 4],
    ['192.88.99.1', 4], ['192.168.1.2', 4], ['192.175.48.1', 4], ['198.18.0.1', 4], ['198.51.100.1', 4],
    ['203.0.113.1', 4], ['224.0.0.1', 4], ['255.255.255.255', 4],
    ['::', 6], ['::1', 6], ['::ffff:8.8.8.8', 6], ['::ffff:808:808', 6], ['64:ff9b::808:808', 6],
    ['100:0:0:1::1', 6], ['2001::1', 6], ['2001:20::1', 6], ['2001:db8::1', 6], ['2002:808:808::1', 6],
    ['2620:4f:8000::1', 6], ['3fff::1', 6], ['5f00::1', 6], ['fc00::1', 6], ['fe80::1', 6], ['ff02::1', 6],
  ]
  for (const [address, family] of blocked) {
    const f = fixture({ records: [{ address: '8.8.8.8', family: 4 }, { address, family }] })
    const result = await f.instance.check(origin)
    assert.equal(result.code, 'address-type-unsupported', address)
    assert.equal(result.dns.state, 'failed')
    assert.equal(result.tls.state, 'not-run')
    assert.equal(f.requests.length, 0)
    limitedClaims(result)
  }
  const ordinary = fixture({ records: [{ address: '2001:4860:4860::8888', family: 6 }, { address: '8.8.8.8', family: 4 }] })
  assert.equal((await ordinary.instance.check(origin)).outcome, 'passed')
  assert.equal(ordinary.requests[0].options.family, 6)
})

test('empty, malformed and failed DNS observations stay unknown without invented answers', async () => {
  for (const records of [[], null, [{ address: 'not-an-address', family: 4 }], [{ address: '8.8.8.8', family: 6 }], [{ address: '8.8.8.8', family: '4' }]]) {
    const f = fixture({ lookup: () => Promise.resolve(records) })
    const result = await f.instance.check(origin)
    assert.equal(result.outcome, 'unknown')
    assert.equal(result.code, 'dns-unavailable')
    assert.equal(f.requests.length, 0)
  }
  const f = fixture({ dnsError: true })
  const failed = await f.instance.check(origin)
  assert.equal(failed.code, 'dns-unavailable')
  limitedClaims(failed)
})

test('a mutable DNS result cannot rebind the pinned lookup and explicit IP entries do not pretend to query DNS', async () => {
  const f = fixture()
  const result = await f.instance.check(origin, { onProgress: report => { if (report.dns.state === 'passed') f.records[0].address = '10.0.0.1' } })
  assert.equal(result.outcome, 'passed')
  assert.deepEqual(f.requests[0].connectedAddresses, [{ address: '8.8.8.8', family: 4 }])
  let pinned
  f.requests[0].options.lookup('public.example.test', { all: true }, error => { assert.ok(error) }) // Finished requests cannot reconnect.
  const open = fixture({ hangResponse: true })
  const pending = open.instance.check(origin)
  await open.responseStarted.promise
  open.requests[0].options.lookup('public.example.test', { all: true }, (error, records) => { assert.equal(error, null); pinned = records })
  assert.deepEqual(pinned, [{ address: '8.8.8.8', family: 4 }])
  open.requests[0].options.lookup('changed.example.test', {}, error => { assert.ok(error) })
  assert.equal(open.lookups.length, 1)
  open.deadline(); await pending
  const literal = fixture({ certificate: { ...certificate, subjectaltname: 'IP Address:8.8.8.8' } })
  const literalResult = await literal.instance.check('https://8.8.8.8')
  assert.equal(literalResult.outcome, 'passed')
  assert.equal(literalResult.dns.source, 'literal')
  assert.equal(literal.lookups.length, 0)
  assert.equal(literal.requests[0].options.servername, '')
  const special = fixture()
  const specialResult = await special.instance.check('https://[::ffff:8.8.8.8]')
  assert.equal(specialResult.dns.source, 'literal')
  assert.equal(specialResult.code, 'address-type-unsupported')
  assert.equal(special.requests.length, 0)
})

test('hostname, untrusted certificates and invalid validity windows reject before sending HTTP', async () => {
  const cases = [
    [{ certificate: { ...certificate, subjectaltname: 'DNS:other.example.test' } }, 'tls-hostname'],
    [{ authorized: false, authorizationError: 'DEPTH_ZERO_SELF_SIGNED_CERT' }, 'tls-untrusted'],
    [{ certificate: { ...certificate, valid_to: 'Jan 1 00:00:00 2029 GMT' } }, 'tls-validity'],
    [{ certificate: { ...certificate, valid_from: 'Jan 1 00:00:00 2031 GMT' } }, 'tls-validity'],
    [{ certificate: { ...certificate, valid_to: undefined } }, 'tls-unverified'],
    [{ requestError: 'ERR_TLS_CERT_ALTNAME_INVALID' }, 'tls-hostname'],
    [{ requestError: 'CERT_HAS_EXPIRED' }, 'tls-validity'],
    [{ requestError: 'SELF_SIGNED_CERT_IN_CHAIN' }, 'tls-untrusted'],
  ]
  for (const [settings, code] of cases) {
    const f = fixture(settings), result = await f.instance.check(origin)
    assert.equal(result.code, code)
    assert.notEqual(result.outcome, 'passed')
    assert.equal(f.requests[0].ends, 0)
    assert.equal(f.requests[0].destroyed, true)
    limitedClaims(result)
  }
})

test('redirects and auth barriers are distinct failures without follow-up requests or leaked headers', async () => {
  for (const [status, code] of [[301, 'http-redirect'], [307, 'http-redirect'], [401, 'http-auth-required'], [403, 'http-auth-required'], [404, 'http-status']]) {
    const f = fixture({ status }), result = await f.instance.check(origin)
    assert.equal(result.code, code)
    assert.equal(result.tls.state, 'passed')
    assert.equal(f.requests.length, 1)
    assert.equal(f.lookups.length, 1)
    assert.deepEqual(result.status, { state: 'failed', version: null, productEdition: null })
    limitedClaims(result)
  }
})

test('fake 200, malformed JSON and missing/null/zero fields never acquire default metadata', async () => {
  const cases = [
    '<html>login</html>', '{', Buffer.from([0xff]), JSON.stringify({ ...statusBody, success: false }),
    JSON.stringify({ success: true }), JSON.stringify({ success: true, data: { product_edition: 'lite' } }),
    JSON.stringify({ success: true, data: { version: null, product_edition: 'lite' } }),
    JSON.stringify({ success: true, data: { version: 0, product_edition: 'lite' } }),
    JSON.stringify({ success: true, data: { version: '0.2.0', product_edition: null } }),
    JSON.stringify({ success: true, data: { version: '0.2.0', product_edition: 'lan' } }),
    JSON.stringify({ success: true, data: { version: '<private>' + privateText, product_edition: 'lite' } }),
  ]
  for (const body of cases) {
    const result = await fixture({ body }).instance.check(origin)
    assert.equal(result.code, 'invalid-status')
    assert.equal(result.status.version, null)
    assert.equal(result.status.productEdition, null)
    limitedClaims(result)
  }
  const body = JSON.stringify(statusBody)
  const exact = fixture({ body: body + ' '.repeat(65536 - Buffer.byteLength(body)) })
  assert.equal((await exact.instance.check(origin)).outcome, 'passed')
  const oversized = await fixture({ body: ' '.repeat(65537) }).instance.check(origin)
  assert.equal(oversized.code, 'response-too-large')
  const truncated = await fixture({ complete: false }).instance.check(origin)
  assert.equal(truncated.code, 'response-unavailable')
})

test('cancellation blocks late DNS, TLS completion and response completion without successful publications', async () => {
  const held = deferred(), controller = new AbortController(), f = fixture({ lookup: () => held.promise })
  const updates = [], pending = f.instance.check(origin, { signal: controller.signal, onProgress: report => { updates.push(report) } })
  await f.lookupStarted.promise
  controller.abort()
  const result = await pending, updateCount = updates.length
  held.resolve([{ address: '8.8.8.8', family: 4 }])
  await new Promise(setImmediate) // Deliver the explicitly late DNS result, after its promise jobs.
  assert.equal(result.outcome, 'cancelled')
  assert.equal(f.requests.length, 0)
  assert.equal(updates.length, updateCount)
  const beforeHTTP = fixture(), stopBeforeHTTP = new AbortController()
  assert.equal((await beforeHTTP.instance.check(origin, { signal: stopBeforeHTTP.signal, onProgress: item => { if (item.tls.state === 'checking') stopBeforeHTTP.abort() } })).outcome, 'cancelled')
  assert.equal(beforeHTTP.requests.length, 0)
  for (const stage of ['TLS', 'Response']) {
    const local = fixture({ ['hang' + stage]: true }), signal = new AbortController()
    const op = local.instance.check(origin, { signal: signal.signal })
    await (stage === 'TLS' ? local.socketStarted.promise : local.responseStarted.promise)
    signal.abort()
    assert.equal((await op).outcome, 'cancelled')
    assert.equal(local.requests[0].destroyed, true)
    assert.equal(local.waiting.size, 0)
    if (stage === 'TLS') assert.equal(local.requests[0].ends, 0)
  }
})

test('one total deadline bounds DNS and TLS/HTTP waits and clears all owned timers', async () => {
  const held = deferred(), dns = fixture({ lookup: () => held.promise })
  const dnsOp = dns.instance.check(origin)
  await dns.lookupStarted.promise; dns.deadline()
  assert.equal((await dnsOp).code, 'timeout')
  assert.equal(dns.requests.length, 0)
  assert.deepEqual(dns.delays, [10000])
  for (const stage of ['TLS', 'Response']) {
    const local = fixture({ ['hang' + stage]: true }), pending = local.instance.check(origin)
    await (stage === 'TLS' ? local.socketStarted.promise : local.responseStarted.promise)
    local.deadline()
    const result = await pending
    assert.equal(result.code, 'timeout')
    assert.equal(result.outcome, 'unknown')
    assert.equal(local.requests[0].destroyed, true)
    assert.equal(local.waiting.size, 0)
  }
  held.resolve([])
})

test('progress failures are isolated, native rejected promises are consumed and generic thenables are never executed', async () => {
  let invoked = 0
  for (const onProgress of [() => { throw new Error(privateText) }, () => Promise.reject(new Error(privateText)), () => ({ then() { invoked++ } })]) {
    const f = fixture(), result = await f.instance.check(origin, { onProgress })
    assert.equal(result.outcome, 'unknown')
    assert.equal(result.code, 'progress-unavailable')
    assert.equal(f.lookups.length, 0)
    assert.equal(f.requests.length, 0)
    assert.equal(f.waiting.size, 0)
    limitedClaims(result)
  }
  await new Promise(setImmediate)
  assert.equal(invoked, 0)
  const connected = fixture()
  const interrupted = await connected.instance.check(origin, { onProgress: item => {
    if (item.tls.state === 'passed' && item.status.state === 'not-run') throw new Error(privateText)
  } })
  assert.equal(interrupted.code, 'progress-unavailable')
  assert.equal(interrupted.outcome, 'unknown')
  assert.equal(connected.requests.length, 1)
  assert.equal(connected.requests[0].ends, 0, 'a progress failure after TLS must not send the GET')
  assert.equal(connected.requests[0].destroyed, true)
})

test('Actions owned TLS transport exercises real trust and hostname checks separately from controlled DNS decisions', { skip: process.env.GITHUB_ACTIONS !== 'true' }, async t => {
  const directory = await mkdtemp(path.join(os.tmpdir(), 'myapi-public-tls-'))
  await chmod(directory, 0o700)
  t.after(() => rm(directory, { recursive: true, force: true }))
  const config = path.join(directory, 'certificate.cnf'), keyFile = path.join(directory, 'server.key'), certFile = path.join(directory, 'server.pem')
  await writeFile(config, '[req]\ndistinguished_name=subject\nx509_extensions=extensions\nprompt=no\n[subject]\nCN=public.example.test\n[extensions]\nsubjectAltName=DNS:public.example.test\nbasicConstraints=critical,CA:TRUE\nkeyUsage=critical,keyCertSign,digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\n', { mode: 0o600 })
  execFileSync('openssl', ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-sha256', '-days', '2', '-config', config, '-keyout', keyFile, '-out', certFile], { stdio: ['ignore', 'pipe', 'pipe'], timeout: 30000 })
  await chmod(keyFile, 0o600)
  const key = await readFile(keyFile), cert = await readFile(certFile), sockets = new Set(), received = []
  const server = https.createServer({ key, cert }, (request, response) => {
    received.push({ method: request.method, url: request.url, authorization: request.headers.authorization, cookie: request.headers.cookie, proxyAuthorization: request.headers['proxy-authorization'] })
    response.setHeader('Content-Type', 'application/json'); response.end(JSON.stringify(statusBody))
  })
  server.on('tlsClientError', () => {})
  server.on('connection', socket => { sockets.add(socket); socket.once('close', () => sockets.delete(socket)) })
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve) })
  t.after(async () => { for (const socket of sockets) socket.destroy(); await new Promise(resolve => server.close(resolve)) })
  const port = server.address().port
  const transport = trusted => (options, callback) => https.request({ ...options,
    // Only this test wrapper changes trust and destination for an owned local
    // server. The production factory has neither CA nor private-target flags.
    ...(trusted ? { ca: cert } : {}), lookup: (_name, lookupOptions, done) => {
      if (lookupOptions.all) done(null, [{ address: '127.0.0.1', family: 4 }])
      else done(null, '127.0.0.1', 4)
    },
  }, callback)
  const proxySockets = new Set(), connects = []
  const proxy = http.createServer((_request, response) => { response.writeHead(405); response.end() })
  proxy.on('connection', socket => { proxySockets.add(socket); socket.once('close', () => proxySockets.delete(socket)) })
  proxy.on('connect', (request, socket) => {
    connects.push({ target: request.url, authorization: request.headers['proxy-authorization'] })
    // This owned negative control never tunnels or forwards to any destination.
    socket.end('HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\nContent-Length: 0\r\n\r\n')
  })
  await new Promise((resolve, reject) => { proxy.once('error', reject); proxy.listen(0, '127.0.0.1', resolve) })
  t.after(async () => { for (const socket of proxySockets) socket.destroy(); await new Promise(resolve => proxy.close(resolve)) })
  const originalGlobalAgent = https.globalAgent
  const proxyAgent = new https.Agent({ proxyEnv: {
    HTTPS_PROXY: `http://synthetic-user:synthetic-pass@127.0.0.1:${proxy.address().port}`,
  } })
  https.globalAgent = proxyAgent
  t.after(() => { https.globalAgent = originalGlobalAgent; proxyAgent.destroy() })
  const control = new Promise((resolve, reject) => {
    const request = transport(true)({ protocol: 'https:', hostname: 'public.example.test', port,
      servername: 'public.example.test', method: 'GET', path: '/api/status',
      agent: proxyAgent, rejectUnauthorized: true, signal: AbortSignal.timeout(10000),
    }, response => { response.destroy(); reject(new Error('Proxy control unexpectedly reached the TLS HTTP target')) })
    request.once('error', resolve)
    request.end()
  })
  await control
  assert.deepEqual(connects, [{ target: `public.example.test:${port}`, authorization: 'Basic ' + Buffer.from('synthetic-user:synthetic-pass').toString('base64') }], 'an explicitly selected proxy must receive the CONNECT control')
  assert.equal(received.length, 0, 'the rejecting proxy must not forward the control into the TLS target')
  const controlledLookup = async () => [{ address: '8.8.8.8', family: 4 }]
  const trusted = checker.createPublicAccessCheck({ lookup: controlledLookup, request: transport(true) })
  const success = await trusted.check(origin + ':' + port)
  assert.equal(success.outcome, 'passed')
  assert.deepEqual(received, [{ method: 'GET', url: '/api/status', authorization: undefined, cookie: undefined, proxyAuthorization: undefined }])
  assert.equal(connects.length, 1, 'production agent:false must bypass the configured global proxy without sending its credentials')
  limitedClaims(success)
  const mismatch = await trusted.check('https://wrong.example.test:' + port)
  assert.equal(mismatch.code, 'tls-hostname')
  const untrusted = await checker.createPublicAccessCheck({ lookup: controlledLookup, request: transport(false) }).check(origin + ':' + port)
  assert.equal(untrusted.code, 'tls-untrusted')
  assert.equal(received.length, 1, 'TLS failures must not deliver an HTTP request')
  assert.equal(connects.length, 1, 'neither successful nor failed entry checks may inherit the global proxy')
  limitedClaims(untrusted)
})
