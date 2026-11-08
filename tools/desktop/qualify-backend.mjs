import { createHash, randomBytes } from 'node:crypto'
import { execFileSync, fork } from 'node:child_process'
import { once } from 'node:events'
import { constants } from 'node:fs'
import { chmod, copyFile, lstat, mkdir, mkdtemp, open, readFile, realpath, rm, writeFile } from 'node:fs/promises'
import http from 'node:http'
import https from 'node:https'
import publicAccessCheck from '../../electron/public-access-check.js'
import net from 'node:net'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { createNativeHost } from '../../electron/test/fixtures/native-host.mjs'

const repository = fileURLToPath(new URL('../..', import.meta.url))
const sha256 = value => createHash('sha256').update(value).digest('hex')
const demand = (ok, code) => { if (!ok) throw new Error(code) }
const safeError = error => /^[A-Z][A-Z0-9_]{0,79}$/.test(error?.message) ? error.message : 'DESKTOP_QUALIFICATION_FAILED'
const bounded = (promise, timeout, code) => new Promise((resolve, reject) => {
  const timer = setTimeout(() => reject(new Error(code)), timeout)
  Promise.resolve(promise).then(value => { clearTimeout(timer); resolve(value) }, error => { clearTimeout(timer); reject(error) })
})

export function qualificationScope(env, args, platform = process.platform, arch = process.arch) {
  demand(args.length === 0 && env.MYAPI_DESKTOP_BACKEND_QUALIFICATION === '1' && env.GITHUB_ACTIONS === 'true' &&
    env.GITHUB_EVENT_NAME === 'workflow_dispatch' && env.GITHUB_REF === 'refs/heads/dev' &&
    env.GITHUB_REPOSITORY === 'ForceMind/MyAPI', 'DESKTOP_SCOPE_REJECTED')
  demand(platform === 'darwin' && arch === 'arm64' || platform === 'win32' && arch === 'x64', 'DESKTOP_PLATFORM_REJECTED')
  demand(/^[a-f0-9]{40}$/.test(env.GITHUB_SHA) && /^[1-9][0-9]*$/.test(env.GITHUB_RUN_ID) &&
    /^[1-9][0-9]*$/.test(env.GITHUB_RUN_ATTEMPT) && /^[a-f0-9]{64}$/.test(env.MYAPI_DESKTOP_BINARY_SHA256) &&
    typeof env.RUNNER_TEMP === 'string' && env.RUNNER_TEMP.length <= 4096 && path.isAbsolute(env.RUNNER_TEMP) &&
    !/[\u0000-\u001f\u007f]/.test(env.RUNNER_TEMP), 'DESKTOP_IDENTITY_REJECTED')
  return { source_sha: env.GITHUB_SHA, run_id: env.GITHUB_RUN_ID, run_attempt: env.GITHUB_RUN_ATTEMPT,
    platform, architecture: arch, binary_sha256: env.MYAPI_DESKTOP_BINARY_SHA256 }
}

export async function binaryDigest(filename, expected) {
  demand(/^[a-f0-9]{64}$/.test(expected), 'DESKTOP_BINARY_HASH_INVALID')
  const stat = await lstat(filename)
  demand(stat.isFile() && !stat.isSymbolicLink(), 'DESKTOP_BINARY_TYPE_REJECTED')
  const descriptor = await open(filename, constants.O_RDONLY | (constants.O_NOFOLLOW || 0))
  try {
    const before = await descriptor.stat()
    demand(before.dev === stat.dev && before.ino === stat.ino, 'DESKTOP_BINARY_CHANGED')
    const digest = sha256(await descriptor.readFile())
    const after = await descriptor.stat()
    demand(before.size === after.size && before.mtimeMs === after.mtimeMs && before.ctimeMs === after.ctimeMs &&
      digest === expected, 'DESKTOP_BINARY_CHANGED')
    return digest
  } finally { await descriptor.close() }
}

export async function createOwnedFixture(parent, scope) {
  const actual = await realpath(parent)
  const parentStat = await lstat(actual)
  demand(parentStat.isDirectory() && actual !== path.parse(actual).root &&
    (process.platform === 'win32' || parentStat.uid === process.getuid()), 'DESKTOP_TEMP_REJECTED')
  const root = await mkdtemp(path.join(actual, `myapi-desktop-${scope.source_sha}-${scope.run_id}-${scope.run_attempt}-`))
  await chmod(root, 0o700)
  const ownership = { schema_version: 1, ...scope, nonce: randomBytes(16).toString('hex') }
  await writeFile(path.join(root, 'ownership.json'), JSON.stringify(ownership), { flag: 'wx', mode: 0o600 })
  return { root, ownership }
}

export async function removeOwnedFixture(fixture) {
  const stat = await lstat(fixture.root)
  const marker = path.join(fixture.root, 'ownership.json')
  const markerStat = await lstat(marker)
  demand(stat.isDirectory() && !stat.isSymbolicLink() && await realpath(fixture.root) === fixture.root &&
    markerStat.isFile() && !markerStat.isSymbolicLink() &&
    sha256(await readFile(marker)) === sha256(JSON.stringify(fixture.ownership)), 'DESKTOP_CLEANUP_OWNERSHIP_UNKNOWN')
  await rm(fixture.root, { recursive: true })
}

export function childEnvironment(env) {
  const result = {}
  for (const name of ['PATH', 'SystemRoot', 'WINDIR', 'COMSPEC', 'PATHEXT', 'LANG', 'LC_ALL', 'TZ']) {
    if (typeof env[name] === 'string') result[name] = env[name]
  }
  return result
}

async function availablePort() {
  const server = net.createServer()
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve) })
  const port = server.address().port
  await new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve()))
  return port
}

function localAPI(port) {
  let accessToken = null
  return {
    setToken(value) { demand(typeof value === 'string' && value.length > 0, 'DESKTOP_AUTH_TOKEN_MISSING'); accessToken = value },
    request(method, pathname, body) {
      demand((method === 'GET' && ['/api/status', '/api/user/self'].includes(pathname)) ||
        (method === 'POST' && ['/api/setup', '/api/user/login'].includes(pathname)) ||
        (method === 'PUT' && pathname === '/api/user/self'), 'DESKTOP_API_SCOPE_REJECTED')
      const payload = body === undefined ? null : Buffer.from(JSON.stringify(body))
      return new Promise((resolve, reject) => {
        const request = http.request({ hostname: '127.0.0.1', port, method, path: pathname, timeout: 10000,
          headers: { ...(payload ? { 'Content-Type': 'application/json', 'Content-Length': payload.length } : {}),
            ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}) } }, response => {
          const chunks = []; let size = 0
          response.on('data', chunk => { size += chunk.length; if (size > 1024 * 1024) response.destroy(new Error('DESKTOP_API_RESPONSE_TOO_LARGE')); else chunks.push(chunk) })
          response.on('error', () => reject(new Error('DESKTOP_API_READ_FAILED')))
          response.on('end', () => {
            try {
              const value = JSON.parse(Buffer.concat(chunks))
              demand(response.statusCode === 200 && value.success === true, 'DESKTOP_API_REJECTED')
              resolve(value.data ?? null)
            } catch { reject(new Error('DESKTOP_API_REJECTED')) }
          })
        })
        request.on('timeout', () => request.destroy(new Error('DESKTOP_API_TIMEOUT')))
        request.on('error', () => reject(new Error('DESKTOP_API_CONNECTION_FAILED')))
        request.end(payload)
      })
    },
  }
}

function businessProjection(value) {
  const keys = ['id', 'username', 'display_name', 'role', 'status', 'group', 'quota', 'used_quota', 'request_count', 'setting']
  demand(value && keys.every(key => Object.hasOwn(value, key)), 'DESKTOP_BUSINESS_FIELDS_MISSING')
  demand(Number.isSafeInteger(value.id) && value.id > 0 && value.username === 'desktopqa' && value.role === 100 &&
    value.status === 1 && value.quota === 0 && value.used_quota === 0 && value.request_count === 0, 'DESKTOP_BUSINESS_MISMATCH')
  demand(value.display_name === 'Desktop QA marker' && typeof value.setting === 'string', 'DESKTOP_MARKER_MISSING')
  return Object.fromEntries(keys.map(key => [key, value[key]]))
}

// The only transport substitution is owned by this qualification fixture. The
// real checker retains its validation, pinned lookup, TLS checks and fixed GET.
async function createOwnedPublicProxy(root, backendPort, version, env) {
  const directory = path.join(root, 'owned-public-entry-tls')
  await mkdir(directory, { mode: 0o700 })
  const config = path.join(directory, 'certificate.cnf')
  const keyFile = path.join(directory, 'server.key'), certFile = path.join(directory, 'server.pem')
  const hostnames = ['public.example.test', 'alternate.example.test']
  await writeFile(config, '[req]\ndistinguished_name=subject\nx509_extensions=extensions\nprompt=no\n[subject]\nCN=public.example.test\n[extensions]\nsubjectAltName=DNS:public.example.test,DNS:alternate.example.test\nbasicConstraints=critical,CA:TRUE\nkeyUsage=critical,keyCertSign,digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\n', { flag: 'wx', mode: 0o600 })
  try {
    execFileSync('openssl', ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-sha256', '-days', '2',
      '-config', config, '-keyout', keyFile, '-out', certFile], {
      env: childEnvironment(env), stdio: ['ignore', 'pipe', 'pipe'], timeout: 30000, maxBuffer: 65536,
    })
  } catch { throw new Error('DESKTOP_PUBLIC_CERTIFICATE_UNAVAILABLE') }
  await chmod(keyFile, 0o600)
  await chmod(certFile, 0o600)
  const key = await readFile(keyFile), cert = await readFile(certFile)
  const sockets = new Set(), clientRequests = new Set(), backendRequests = new Set()
  let proxyCount = 0, backendCount = 0, dnsCount = 0, proxyPort = null, nextHold = null, violation = null, closing = null
  const invalid = code => { violation ??= code }
  const server = https.createServer({ key, cert }, (request, response) => {
    if (closing) { request.resume(); response.destroy(); return }
    const socketClosed = Promise.withResolvers()
    request.socket.once('close', socketClosed.resolve)
    const held = nextHold
    nextHold = null
    const record = { socketClosed: socketClosed.promise, released: false, body: null, release: null }
    response.on('error', () => {}) // Cancelled/stale requests may close before their held response.
    const expectedHost = hostnames.map(host => `${host}:${proxyPort}`)
    if (request.method !== 'GET' || request.url !== '/api/status' || !expectedHost.includes(request.headers.host) ||
        !hostnames.includes(request.socket.servername) ||
        ['authorization', 'cookie', 'proxy-authorization', 'transfer-encoding'].some(name => request.headers[name] !== undefined) ||
        request.headers['content-length'] !== undefined && request.headers['content-length'] !== '0') {
      invalid('DESKTOP_PUBLIC_REQUEST_REJECTED')
      request.resume(); response.destroy(); return
    }
    proxyCount++
    let bodyBytes = 0
    request.on('data', chunk => { bodyBytes += chunk.length; invalid('DESKTOP_PUBLIC_REQUEST_BODY_REJECTED'); response.destroy() })
    request.on('error', () => response.destroy())
    request.on('end', () => {
      if (closing || bodyBytes !== 0 || response.destroyed) return
      const outgoing = http.request({ hostname: '127.0.0.1', port: backendPort, method: 'GET', path: '/api/status',
        headers: { Accept: 'application/json' }, agent: false, timeout: 10000 }, incoming => {
        const chunks = []; let size = 0
        incoming.on('error', () => { invalid('DESKTOP_PUBLIC_BACKEND_READ_FAILED'); response.destroy() })
        incoming.on('data', chunk => {
          size += chunk.length
          if (size > 65536) { invalid('DESKTOP_PUBLIC_BACKEND_RESPONSE_TOO_LARGE'); incoming.destroy(); response.destroy() }
          else chunks.push(chunk)
        })
        incoming.on('end', () => {
          let value
          const body = Buffer.concat(chunks)
          try { value = JSON.parse(body.toString('utf8')) } catch { value = null }
          if (!incoming.complete || incoming.statusCode !== 200 || value?.success !== true ||
              value.data?.version !== version || value.data?.product_edition !== 'lite' || value.data?.setup !== true) {
            invalid('DESKTOP_PUBLIC_BACKEND_IDENTITY_MISMATCH'); response.destroy(); return
          }
          if (closing) { response.destroy(); return }
          record.body = body
          record.release = () => {
            demand(!record.released, 'DESKTOP_PUBLIC_RESPONSE_ALREADY_RELEASED')
            record.released = true
            const bytes = record.body
            record.body = null
            // Releasing an already closed response drops only these owned
            // bytes. It cannot create another request or synthetic result.
            if (!response.destroyed) { response.writeHead(incoming.statusCode, { 'Content-Type': 'application/json' }); response.end(bytes) }
          }
          if (held) held.resolve(record)
          else record.release()
        })
      })
      const closed = Promise.withResolvers()
      const tracked = { request: outgoing, closed: closed.promise }
      backendRequests.add(tracked)
      outgoing.once('close', () => { backendRequests.delete(tracked); closed.resolve() })
      outgoing.on('error', () => { invalid('DESKTOP_PUBLIC_BACKEND_CONNECTION_FAILED'); response.destroy() })
      outgoing.on('timeout', () => outgoing.destroy(new Error('DESKTOP_PUBLIC_BACKEND_TIMEOUT')))
      backendCount++
      outgoing.end()
    })
    request.resume()
  })
  server.on('tlsClientError', () => { invalid('DESKTOP_PUBLIC_TLS_CLIENT_FAILED') })
  server.on('error', () => { invalid('DESKTOP_PUBLIC_PROXY_FAILED') })
  server.on('connection', socket => { sockets.add(socket); socket.once('close', () => sockets.delete(socket)) })
  const close = () => {
    if (closing) return closing
    closing = (async () => {
      const waits = []
      for (const tracked of [...clientRequests, ...backendRequests]) { waits.push(tracked.closed); tracked.request.destroy() }
      for (const socket of sockets) { waits.push(new Promise(resolve => socket.once('close', resolve))); socket.destroy() }
      const stopped = new Promise((resolve, reject) => server.close(error => error ? reject(new Error('DESKTOP_PUBLIC_PROXY_CLOSE_UNKNOWN')) : resolve()))
      await bounded(Promise.all([...waits, stopped]), 10000, 'DESKTOP_PUBLIC_PROXY_CLOSE_TIMEOUT')
      demand(!server.listening && sockets.size === 0 && clientRequests.size === 0 && backendRequests.size === 0,
        'DESKTOP_PUBLIC_PROXY_CLOSE_UNKNOWN')
    })()
    return closing
  }
  try {
    await bounded(new Promise((resolve, reject) => {
      server.once('error', () => reject(new Error('DESKTOP_PUBLIC_PROXY_UNAVAILABLE')))
      server.listen(0, '127.0.0.1', resolve)
    }), 10000, 'DESKTOP_PUBLIC_PROXY_LISTEN_TIMEOUT')
    demand(server.address().port !== backendPort, 'DESKTOP_PUBLIC_PORT_COLLISION')
  } catch (error) {
    await close()
    throw error
  }
  const port = server.address().port
  proxyPort = port
  return {
    origins: hostnames.map(host => `https://${host}:${port}`),
    stats: () => ({ proxyCount, backendCount, dnsCount, violation }),
    holdNextResponse() {
      demand(nextHold === null, 'DESKTOP_PUBLIC_HOLD_ALREADY_PENDING')
      nextHold = Promise.withResolvers()
      return nextHold.promise
    },
    checkFactory: () => publicAccessCheck.createPublicAccessCheck({
      lookup: async (hostname, options) => {
        demand(!closing && hostnames.includes(hostname) && options.all === true && options.verbatim === true, 'DESKTOP_PUBLIC_DNS_SCOPE_REJECTED')
        dnsCount++
        // This public address is a logical validation input, never a network destination.
        return [{ address: '8.8.8.8', family: 4 }]
      },
      request: (options, callback) => {
        demand(!closing && options.protocol === 'https:' && hostnames.includes(options.hostname) && Number(options.port) === port &&
          options.servername === options.hostname && options.method === 'GET' && options.path === '/api/status' &&
          options.agent === false && options.rejectUnauthorized === true && typeof options.checkServerIdentity === 'function' &&
          !Object.hasOwn(options, 'ca') && Object.keys(options.headers).length === 1 && options.headers.Accept === 'application/json',
        'DESKTOP_PUBLIC_TRANSPORT_SCOPE_REJECTED')
        const outgoing = https.request({ ...options, ca: cert,
          lookup: (hostname, lookupOptions, done) => options.lookup(hostname, lookupOptions, (error, address, family) => {
            if (error) { done(error); return }
            const valid = lookupOptions.all
              ? Array.isArray(address) && address.length === 1 && address[0].address === '8.8.8.8' && address[0].family === 4
              : address === '8.8.8.8' && family === 4
            if (!valid) { done(new Error('DESKTOP_PUBLIC_PINNED_ADDRESS_REJECTED')); return }
            if (lookupOptions.all) done(null, [{ address: '127.0.0.1', family: 4 }])
            else done(null, '127.0.0.1', 4)
          }),
        }, callback)
        const closed = Promise.withResolvers()
        const tracked = { request: outgoing, closed: closed.promise }
        clientRequests.add(tracked)
        outgoing.once('close', () => { clientRequests.delete(tracked); closed.resolve() })
        return outgoing
      },
    }),
    close,
  }
}

async function qualifyPublicEntry(host, proxy, userData, version, report, checkIds) {
  const [originA, originB] = proxy.origins
  const initial = await host.invoke('snapshot')
  demand(initial.publicAccess?.status === 'not-configured' && initial.publicAccess.recordRevision === 0 &&
    initial.publicAccess.localCheck === null && proxy.stats().dnsCount === 0 && proxy.stats().proxyCount === 0 &&
    proxy.stats().backendCount === 0, 'DESKTOP_PUBLIC_AUTOMATIC_CHECK_DETECTED')
  const entryFile = path.join(userData, 'myapi-desktop-public-access.json')
  const cancelledSave = await host.invoke('savePublicEntry', originA, 0)
  demand(cancelledSave.publicSaveResult?.outcome === 'cancelled' && cancelledSave.publicSaveResult.unchanged === true &&
    cancelledSave.publicAccess.recordRevision === 0 && cancelledSave.publicAccess.savedOrigin === null,
    'DESKTOP_PUBLIC_DEFAULT_CANCEL_CHANGED_ENTRY')
  try { await lstat(entryFile); throw new Error('DESKTOP_PUBLIC_DEFAULT_CANCEL_CHANGED_ENTRY') }
  catch (error) { demand(error.code === 'ENOENT', 'DESKTOP_PUBLIC_DEFAULT_CANCEL_CHANGED_ENTRY') }
  host.answers.push(1)
  const saved = await host.invoke('savePublicEntry', originA, 0)
  demand(saved.publicSaveResult?.outcome === 'saved' && saved.publicSaveResult.expectedRevision === 0 &&
    saved.publicSaveResult.origin === originA && saved.publicSaveResult.recordRevision === 1 && saved.publicSaveResult.unchanged === false &&
    saved.publicAccess.savedOrigin === originA && saved.publicAccess.recordRevision === 1 && saved.publicAccess.status === 'unverified',
    'DESKTOP_PUBLIC_SAVE_UNCONFIRMED')
  const entry = await lstat(entryFile)
  demand(entry.isFile() && !entry.isSymbolicLink() && await realpath(entryFile) === entryFile &&
    (process.platform === 'win32' || entry.uid === process.getuid() && (entry.mode & 0o777) === 0o600) &&
    await readFile(entryFile, 'utf8') === JSON.stringify({ schemaVersion: 1, origin: originA }) + '\n',
    'DESKTOP_PUBLIC_SAVED_RECORD_MISMATCH')
  report.checks.saved_record_bound = true
  const progress = []
  const unsubscribe = host.onSnapshot(value => {
    const item = value.publicAccess?.localCheck
    if (item?.state === 'checking' && item.result) progress.push({ id: item.id, value: item.result })
  })
  try {
    const started = await host.invoke('checkPublicEntry', originA, 1)
    const first = started.publicAccess?.localCheck
    demand(started.publicCheckControlResult?.action === 'start' && started.publicCheckControlResult.outcome === 'started' &&
      started.publicCheckControlResult.expectedRevision === 1 && started.publicCheckControlResult.origin === originA &&
      typeof first?.id === 'string' && /^[a-f0-9]{32}$/.test(first.id) && started.publicCheckControlResult.checkId === first.id && first.origin === originA &&
      first.recordRevision === 1 && first.generation === started.generation && first.state === 'checking', 'DESKTOP_PUBLIC_START_UNCONFIRMED')
    checkIds.add(first.id)
    const busy = await host.invoke('checkPublicEntry', originA, 1)
    demand(busy.publicCheckControlResult?.outcome === 'busy' && busy.publicCheckControlResult.checkId === first.id,
      'DESKTOP_PUBLIC_DUPLICATE_CHECK_STARTED')
    const complete = await host.waitForSnapshot(value => value.publicAccess?.localCheck?.id === first.id && value.publicAccess.localCheck.state === 'completed')
    const result = complete.publicAccess.localCheck.result
    demand(result?.outcome === 'passed' && result.code === 'ok' && result.status.version === version &&
      result.status.productEdition === 'lite' && Number.isSafeInteger(result.checkedAt) && result.checkedAt >= 0 &&
      [result.dns, result.tls, result.status].every(stage => stage.state === 'passed'), 'DESKTOP_PUBLIC_GO_STATUS_UNVERIFIED')
    report.checks.real_go_status_forwarded = true
    const firstProgress = progress.filter(item => item.id === first.id).map(item => item.value)
    const dns = firstProgress.findIndex(item => item.dns.state === 'checking')
    const tls = firstProgress.findIndex(item => item.tls.state === 'checking')
    const status = firstProgress.findIndex(item => item.status.state === 'checking')
    demand(dns >= 0 && tls > dns && status > tls && firstProgress.every(item => item.checkedAt === null && item.outcome === 'unknown'),
      'DESKTOP_PUBLIC_PROGRESS_UNVERIFIED')
    report.checks.progress_observed = true

    const cancelResponse = proxy.holdNextResponse()
    const second = await host.invoke('checkPublicEntry', originA, 1)
    const secondId = second.publicAccess?.localCheck?.id
    demand(second.publicCheckControlResult?.action === 'start' && second.publicCheckControlResult.outcome === 'started' &&
      second.publicCheckControlResult.expectedRevision === 1 && second.publicCheckControlResult.origin === originA &&
      typeof secondId === 'string' && /^[a-f0-9]{32}$/.test(secondId) && !checkIds.has(secondId) &&
      second.publicCheckControlResult.checkId === secondId && second.publicAccess.localCheck.origin === originA &&
      second.publicAccess.localCheck.recordRevision === 1 && second.publicAccess.localCheck.generation === second.generation &&
      second.publicAccess.localCheck.state === 'checking',
      'DESKTOP_PUBLIC_CANCEL_START_UNCONFIRMED')
    checkIds.add(secondId)
    const heldCancel = await bounded(cancelResponse, 15000, 'DESKTOP_PUBLIC_CANCEL_RESPONSE_UNOBSERVED')
    const cancelled = await host.invoke('cancelPublicEntryCheck', secondId)
    demand(cancelled.publicCheckControlResult?.action === 'cancel' && cancelled.publicCheckControlResult.outcome === 'cancelled' &&
      cancelled.publicCheckControlResult.checkId === secondId && cancelled.publicCheckControlResult.expectedRevision === null &&
      cancelled.publicCheckControlResult.origin === originA && cancelled.publicAccess.localCheck.id === secondId &&
      cancelled.publicAccess.localCheck.state === 'cancelled', 'DESKTOP_PUBLIC_CANCEL_UNCONFIRMED')
    await bounded(heldCancel.socketClosed, 10000, 'DESKTOP_PUBLIC_CANCEL_SOCKET_OPEN')
    heldCancel.release()
    const afterCancel = await host.invoke('snapshot')
    demand(afterCancel.publicAccess.localCheck.id === secondId && afterCancel.publicAccess.localCheck.state === 'cancelled',
      'DESKTOP_PUBLIC_CANCEL_OVERWRITTEN')
    report.checks.explicit_cancel_confirmed = true

    const staleResponse = proxy.holdNextResponse()
    const third = await host.invoke('checkPublicEntry', originA, 1)
    const thirdId = third.publicAccess?.localCheck?.id
    demand(third.publicCheckControlResult?.action === 'start' && third.publicCheckControlResult.outcome === 'started' &&
      third.publicCheckControlResult.expectedRevision === 1 && third.publicCheckControlResult.origin === originA &&
      typeof thirdId === 'string' && /^[a-f0-9]{32}$/.test(thirdId) && !checkIds.has(thirdId) &&
      third.publicCheckControlResult.checkId === thirdId && third.publicAccess.localCheck.origin === originA &&
      third.publicAccess.localCheck.recordRevision === 1 && third.publicAccess.localCheck.generation === third.generation &&
      third.publicAccess.localCheck.state === 'checking',
      'DESKTOP_PUBLIC_STALE_START_UNCONFIRMED')
    checkIds.add(thirdId)
    const heldStale = await bounded(staleResponse, 15000, 'DESKTOP_PUBLIC_STALE_RESPONSE_UNOBSERVED')
    host.answers.push(1)
    const replaced = await host.invoke('savePublicEntry', originB, 1)
    demand(replaced.publicSaveResult?.outcome === 'saved' && replaced.publicSaveResult.expectedRevision === 1 &&
      replaced.publicSaveResult.origin === originB && replaced.publicSaveResult.recordRevision === 2 && replaced.publicSaveResult.unchanged === false &&
      replaced.publicAccess.savedOrigin === originB && replaced.publicAccess.recordRevision === 2 &&
      replaced.publicAccess.localCheck.id === thirdId && replaced.publicAccess.localCheck.state === 'stale' &&
      await readFile(entryFile, 'utf8') === JSON.stringify({ schemaVersion: 1, origin: originB }) + '\n', 'DESKTOP_PUBLIC_STALE_ENTRY_UNCONFIRMED')
    await bounded(heldStale.socketClosed, 10000, 'DESKTOP_PUBLIC_STALE_SOCKET_OPEN')
    const beforeOld = proxy.stats()
    const old = await host.invoke('checkPublicEntry', originA, 1)
    demand(old.publicCheckControlResult?.outcome === 'stale' && proxy.stats().dnsCount === beforeOld.dnsCount &&
      proxy.stats().proxyCount === beforeOld.proxyCount && proxy.stats().backendCount === beforeOld.backendCount,
      'DESKTOP_PUBLIC_OLD_ENTRY_REUSED')
    const fourth = await host.invoke('checkPublicEntry', originB, 2)
    const fourthId = fourth.publicAccess?.localCheck?.id
    demand(fourth.publicCheckControlResult?.action === 'start' && fourth.publicCheckControlResult.outcome === 'started' &&
      fourth.publicCheckControlResult.expectedRevision === 2 && fourth.publicCheckControlResult.origin === originB &&
      typeof fourthId === 'string' && /^[a-f0-9]{32}$/.test(fourthId) && !checkIds.has(fourthId) &&
      fourth.publicCheckControlResult.checkId === fourthId && fourth.publicAccess.localCheck.origin === originB &&
      fourth.publicAccess.localCheck.recordRevision === 2 && fourth.publicAccess.localCheck.generation === fourth.generation &&
      fourth.publicAccess.localCheck.state === 'checking',
      'DESKTOP_PUBLIC_NEW_ENTRY_START_UNCONFIRMED')
    checkIds.add(fourthId)
    const newComplete = await host.waitForSnapshot(value => value.publicAccess?.localCheck?.id === fourthId && value.publicAccess.localCheck.state === 'completed')
    demand(newComplete.publicAccess.localCheck.result?.outcome === 'passed' && newComplete.publicAccess.localCheck.result.status.version === version &&
      newComplete.publicAccess.localCheck.result.status.productEdition === 'lite', 'DESKTOP_PUBLIC_NEW_ENTRY_UNVERIFIED')
    heldStale.release()
    const oldCancel = await host.invoke('cancelPublicEntryCheck', thirdId)
    const current = oldCancel.publicAccess.localCheck
    demand(oldCancel.publicCheckControlResult?.outcome === 'stale' && current.id === fourthId && current.origin === originB &&
      current.recordRevision === 2 && current.state === 'completed' && current.result.outcome === 'passed', 'DESKTOP_PUBLIC_LATE_RESPONSE_OVERWROTE_ENTRY')
    report.checks.stale_entry_ignored = true
    demand(current.result.source === 'this-computer' && current.result.externalVerification === 'unknown' &&
      current.result.sameInstanceVerified === false && current.result.modelVerified === false && current.result.quotaVerified === false &&
      oldCancel.publicAccess.verification === 'unknown' && oldCancel.publicAccess.status === 'unverified', 'DESKTOP_PUBLIC_VERIFICATION_OVERCLAIMED')
    report.checks.declaration_unverified = true
    const counts = proxy.stats()
    demand(counts.violation === null && counts.proxyCount === 4 && counts.backendCount === 4 && counts.dnsCount === 4,
      'DESKTOP_PUBLIC_REQUEST_COUNTS_MISMATCH')
    report.checks.credential_free_get_only = true
    return { id: fourthId, origin: originB, revision: 2 }
  } finally { unsubscribe() }
}

async function closeOwned(child, closedPromise) {
  if (child.exitCode !== null || child.signalCode !== null) { await bounded(closedPromise, 5000, 'DESKTOP_CHILD_CLOSE_UNKNOWN'); return }
  child.kill('SIGTERM')
  try { await bounded(closedPromise, 10000, 'DESKTOP_CLEANUP_TERM_TIMEOUT') }
  catch { child.kill('SIGKILL'); await bounded(closedPromise, 5000, 'DESKTOP_CLEANUP_CLOSE_UNKNOWN') }
}

export async function runQualification(env = process.env, args = process.argv.slice(2)) {
  const report = { schema_version: 1, source_sha: null, run_id: null, run_attempt: null, platform: process.platform,
    architecture: process.arch, binary_sha256: null, passed: false, phase: 'scope', error_code: null, checks: {}, lifecycle: [],
    qualified_scope: 'owned_backend_lifecycle_and_persistence', graceful_stop_verified: false,
    termination_semantics: 'not_verified', accounting_verified: false,
    cleanup: { confirmed: false, owned_children_closed: false, fixture_removed: false },
    electron_started: false, gui_verified: false, model_requests: 0, production_touched: false,
    public_entry_check: {
      scope: 'owned_go_https_reverse_proxy_main_controls', transport: 'controlled-dns-ca-loopback',
      external_verification: 'unknown', same_instance_verified: false, model_requests: 0,
      proxy_get_count: null, backend_status_get_count: null,
      checks: { saved_record_bound: false, real_go_status_forwarded: false, progress_observed: false,
        explicit_cancel_confirmed: false, stale_entry_ignored: false, credential_free_get_only: false,
        declaration_unverified: false, owned_proxy_closed: false },
    } }
  let publicProxy = null, publicEntryBinding = null, publicProxyCreationStarted = false
  const publicCheckIds = new Set()
  let fixture = null, host = null, canary = null, canaryClosed = null
  try {
    const scope = qualificationScope(env, args)
    Object.assign(report, scope)
    report.phase = 'source'
    const git = (...parts) => execFileSync('git', ['-c', 'core.fileMode=false', ...parts], { cwd: repository, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim()
    demand(git('rev-parse', 'HEAD') === scope.source_sha && git('status', '--porcelain', '--untracked-files=no') === '', 'DESKTOP_SOURCE_IDENTITY_MISMATCH')
    const version = (await readFile(path.join(repository, 'VERSION'), 'utf8')).trim()
    demand(/^\d+\.\d+\.\d+(?:[.-][0-9A-Za-z.-]+)?$/.test(version), 'DESKTOP_VERSION_INVALID')
    const binaryName = process.platform === 'win32' ? 'my-api.exe' : 'my-api'
    const sourceBinary = path.join(repository, binaryName)
    await binaryDigest(sourceBinary, scope.binary_sha256)
    report.checks.source_and_binary_bound = true
    fixture = await createOwnedFixture(env.RUNNER_TEMP, scope)
    const resources = path.join(fixture.root, 'resources space 资源')
    const userData = path.join(fixture.root, 'profile space 用户')
    await mkdir(path.join(resources, 'bin'), { recursive: true, mode: 0o700 })
    await mkdir(userData, { mode: 0o700 })
    const binary = path.join(resources, 'bin', binaryName)
    await copyFile(sourceBinary, binary, constants.COPYFILE_EXCL)
    if (process.platform !== 'win32') await chmod(binary, 0o700)
    await binaryDigest(binary, scope.binary_sha256)
    const safeEnv = childEnvironment(env)
    Object.assign(safeEnv, { TMPDIR: fixture.root, TMP: fixture.root, TEMP: fixture.root })
    canary = fork(path.join(repository, 'electron/test/fixtures/service-child.mjs'), [], { cwd: fixture.root,
      env: { ...safeEnv, MYAPI_DESKTOP_CHILD_FIXTURE: '1' }, execArgv: [], stdio: ['ignore', 'ignore', 'ignore', 'ipc'] })
    canaryClosed = once(canary, 'close')
    const canaryMessage = await bounded(once(canary, 'message'), 10000, 'DESKTOP_CANARY_UNREADY')
    demand(canaryMessage[0]?.ready === true, 'DESKTOP_CANARY_UNREADY')
    const port = await availablePort()
    publicProxyCreationStarted = true
    publicProxy = await createOwnedPublicProxy(fixture.root, port, version, env)
    report.public_entry_check.proxy_get_count = publicProxy.stats().proxyCount
    report.public_entry_check.backend_status_get_count = publicProxy.stats().backendCount
    host = await createNativeHost({ electronDirectory: path.join(repository, 'electron'), resources, userData,
      env: safeEnv, version, port, publicAccessCheckFactory: publicProxy.checkFactory })
    report.phase = 'start'
    const ready = await host.waitForState('running')
    demand(host.children.length === 1 && ready.owned === true && ready.generation === 1 &&
      Number.isSafeInteger(host.children[0].pid) && host.children[0].pid > 0 &&
      ready.addresses.length === 1 && ready.addresses[0] === `http://127.0.0.1:${port}/v1`, 'DESKTOP_START_IDENTITY_MISMATCH')
    const api = localAPI(port)
    const status = await api.request('GET', '/api/status')
    demand(status.product_edition === 'lite' && status.version === version && status.setup === false, 'DESKTOP_RUNTIME_IDENTITY_MISMATCH')
    report.checks.real_lite_status = true
    report.phase = 'configure'
    const password = 'Qa-' + randomBytes(24).toString('hex')
    const login = async () => {
      const value = await api.request('POST', '/api/user/login', { username: 'desktopqa', password })
      api.setToken(value?.access_token)
    }
    await api.request('POST', '/api/setup', { username: 'desktopqa', password, confirmPassword: password, SelfUseModeEnabled: true, DemoSiteEnabled: false })
    await login()
    await api.request('PUT', '/api/user/self', { username: 'desktopqa', display_name: 'Desktop QA marker' })
    const before = businessProjection(await api.request('GET', '/api/user/self'))
    const secretPath = path.join(userData, 'myapi-session-secret')
    const secret = await readFile(secretPath)
    demand(/^[a-f0-9]{64}\n$/.test(secret.toString()), 'DESKTOP_SECRET_INVALID')
    const database = await lstat(path.join(userData, 'data', 'my-api.db'))
    demand(database.isFile() && !database.isSymbolicLink() && database.size > 0, 'DESKTOP_DATABASE_MISSING')
    const secretStat = await lstat(secretPath)
    const profileStat = await lstat(userData)
    demand(secretStat.isFile() && !secretStat.isSymbolicLink() && profileStat.isDirectory() &&
      !profileStat.isSymbolicLink() && await realpath(userData) === userData &&
      (process.platform === 'win32' || profileStat.uid === process.getuid() && (profileStat.mode & 0o777) === 0o700 &&
        secretStat.uid === process.getuid() && (secretStat.mode & 0o777) === 0o600), 'DESKTOP_FILESYSTEM_IDENTITY_MISMATCH')
    report.checks.owned_filesystem_bound = true
    report.checks.local_configuration_saved = true
    report.phase = 'public-entry'
    publicEntryBinding = await qualifyPublicEntry(host, publicProxy, userData, version, report.public_entry_check, publicCheckIds)
    demand(JSON.stringify(businessProjection(await api.request('GET', '/api/user/self'))) === JSON.stringify(before) &&
      Buffer.compare(secret, await readFile(secretPath)) === 0, 'DESKTOP_PUBLIC_BUSINESS_OR_SECRET_CHANGED')
    const first = host.children[0]
    host.panel().close()
    const hidden = await host.invoke('snapshot')
    demand(!host.panel().isVisible() && hidden.state === 'running' && !first.closed && first.child.pid === first.pid, 'DESKTOP_WINDOW_CLOSE_CHANGED_SERVICE')
    const cancelled = await host.invoke('stop') // The facade defaults to Cancel.
    demand(cancelled.state === 'running' && first.requested_signals.length === 0 && !first.closed && host.exits === 0, 'DESKTOP_CANCEL_CHANGED_SERVICE')
    report.checks.close_and_cancel_preserve_process = true
    report.phase = 'stop'
    host.answers.push(1)
    const stopped = await bounded(host.invoke('stop'), 15000, 'DESKTOP_STOP_TIMEOUT')
    await bounded(first.closedPromise, 5000, 'DESKTOP_STOP_CLOSE_UNKNOWN')
    demand(stopped.state === 'stopped' && stopped.owned === false && first.closed && host.exits === 0 &&
      (Number.isInteger(first.exit_code) || typeof first.signal_code === 'string') &&
      !first.requested_signals.includes('SIGKILL'), 'DESKTOP_STOP_UNCONFIRMED')
    demand(stopped.publicAccess.localCheck?.id === publicEntryBinding.id && stopped.publicAccess.localCheck.state === 'stale' &&
      stopped.publicAccess.savedOrigin === publicEntryBinding.origin && stopped.publicAccess.recordRevision === publicEntryBinding.revision,
      'DESKTOP_PUBLIC_STOP_INVALIDATION_UNVERIFIED')
    demand(canary.exitCode === null && canary.signalCode === null && canary.connected, 'DESKTOP_UNRELATED_CHILD_STOPPED')
    if (process.platform === 'darwin') demand(first.exit_code === 0 && first.signal_code === null, 'DESKTOP_STOP_NOT_GRACEFUL')
    report.checks.owned_stop_confirmed = true
    report.checks.unrelated_child_preserved = true
    report.phase = 'restart'
    host.answers.push(1)
    await host.invoke('start')
    const restarted = await host.waitForState('running')
    demand(host.children.length === 2 && restarted.generation === 2 &&
      Number.isSafeInteger(host.children[1].pid) && host.children[1].pid > 0, 'DESKTOP_RESTART_IDENTITY_MISMATCH')
    demand(restarted.publicAccess.localCheck?.id === publicEntryBinding.id && restarted.publicAccess.localCheck.state === 'stale' &&
      restarted.publicAccess.savedOrigin === publicEntryBinding.origin && restarted.publicAccess.recordRevision === publicEntryBinding.revision &&
      restarted.publicAccess.verification === 'unknown', 'DESKTOP_PUBLIC_RESTART_BINDING_CHANGED')
    const afterStatus = await api.request('GET', '/api/status')
    demand(afterStatus.product_edition === 'lite' && afterStatus.version === version && afterStatus.setup === true, 'DESKTOP_RESTART_SETUP_CHANGED')
    // The pre-stop authenticated session must still work with the same secret.
    demand(JSON.stringify(businessProjection(await api.request('GET', '/api/user/self'))) === JSON.stringify(before) &&
      Buffer.compare(secret, await readFile(secretPath)) === 0, 'DESKTOP_PERSISTENCE_CHANGED')
    await login()
    demand(JSON.stringify(businessProjection(await api.request('GET', '/api/user/self'))) === JSON.stringify(before), 'DESKTOP_RELOGIN_CHANGED')
    await binaryDigest(binary, scope.binary_sha256)
    report.checks.restart_business_and_secret_preserved = true
    report.phase = 'quit'
    host.answers.push(1)
    await bounded(host.invoke('quit'), 15000, 'DESKTOP_QUIT_TIMEOUT')
    const second = host.children[1]
    await bounded(second.closedPromise, 5000, 'DESKTOP_QUIT_CLOSE_UNKNOWN')
    demand(host.exits === 1 && second.closed && !second.requested_signals.includes('SIGKILL') &&
      (Number.isInteger(second.exit_code) || typeof second.signal_code === 'string') &&
      canary.exitCode === null && canary.signalCode === null, 'DESKTOP_QUIT_UNCONFIRMED')
    if (process.platform === 'darwin') demand(second.exit_code === 0 && second.signal_code === null, 'DESKTOP_QUIT_NOT_GRACEFUL')
    report.checks.quit_waited_for_close = true
    report.graceful_stop_verified = process.platform === 'darwin'
    report.termination_semantics = process.platform === 'darwin' ? 'normal_exit_zero_without_sigkill' : 'windows_graceful_flush_not_verified'
    report.phase = 'completed'
    report.passed = true
  } catch (error) { report.error_code = safeError(error) }
  finally {
    // A failed partial proxy construction is not proof that no handles exist.
    let networkClosed = !publicProxyCreationStarted
    if (publicProxy) {
      try {
        const current = host ? await bounded(host.invoke('snapshot'), 10000, 'DESKTOP_PUBLIC_CLEANUP_SNAPSHOT_UNKNOWN') : null
        const check = current?.publicAccess?.localCheck
        if (check?.state === 'checking') {
          demand(publicCheckIds.has(check.id), 'DESKTOP_PUBLIC_CLEANUP_CHECK_NOT_OWNED')
          const cancelled = await bounded(host.invoke('cancelPublicEntryCheck', check.id), 10000, 'DESKTOP_PUBLIC_CLEANUP_CANCEL_TIMEOUT')
          demand(cancelled.publicCheckControlResult?.checkId === check.id &&
            ['cancelled', 'unchanged', 'stale'].includes(cancelled.publicCheckControlResult.outcome) &&
            cancelled.publicAccess.localCheck?.state !== 'checking', 'DESKTOP_PUBLIC_CLEANUP_CANCEL_UNCONFIRMED')
        }
      } catch (error) {
        report.passed = false
        report.error_code ??= safeError(error)
      }
      try {
        await publicProxy.close()
        networkClosed = true
        report.public_entry_check.checks.owned_proxy_closed = true
      } catch (error) {
        report.passed = false
        report.error_code ??= safeError(error)
      }
      const counts = publicProxy.stats()
      report.public_entry_check.proxy_get_count = counts.proxyCount
      report.public_entry_check.backend_status_get_count = counts.backendCount
      if (counts.violation !== null) {
        report.public_entry_check.checks.credential_free_get_only = false
        report.passed = false
        report.error_code ??= counts.violation
      }
    }
    // A network cleanup failure cannot prevent independent close attempts on
    // every held child. Unknown closure retains the owned fixture for evidence.
    let childrenClosed = true
    for (const record of host?.children ?? []) {
      try { await closeOwned(record.child, record.closedPromise) }
      catch (error) {
        childrenClosed = false
        report.passed = false
        report.error_code ??= safeError(error)
      }
    }
    if (canary) {
      try {
        if (canary.exitCode === null && canary.signalCode === null && canary.connected) canary.send('exit')
      } catch (error) {
        report.passed = false
        report.error_code ??= safeError(error)
      }
      try { await bounded(canaryClosed, 5000, 'DESKTOP_CANARY_CLOSE_UNKNOWN') }
      catch (error) {
        childrenClosed = false
        report.passed = false
        report.error_code ??= safeError(error)
      }
    }
    report.cleanup.owned_children_closed = childrenClosed
    if (networkClosed && childrenClosed) {
      try {
        if (fixture) await removeOwnedFixture(fixture)
        report.cleanup.fixture_removed = true
        report.cleanup.confirmed = true
      } catch (error) {
        report.passed = false
        report.error_code ??= safeError(error)
      }
    }
    report.lifecycle = (host?.children ?? []).map(({ generation, pid, requested_signals, exit_code, signal_code, closed }) =>
      ({ generation, pid, requested_signals, exit_code, signal_code, closed }))
  }
  if (publicProxy) {
    const counts = publicProxy.stats()
    report.public_entry_check.proxy_get_count = counts.proxyCount
    report.public_entry_check.backend_status_get_count = counts.backendCount
  }
  if (report.passed && (Object.values(report.public_entry_check.checks).some(value => value !== true) ||
      report.public_entry_check.proxy_get_count !== 4 || report.public_entry_check.backend_status_get_count !== 4 ||
      !Object.values(report.cleanup).every(value => value === true))) {
    report.passed = false
    report.error_code ??= 'DESKTOP_PUBLIC_QUALIFICATION_INCOMPLETE'
  }
  return report
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const report = await runQualification()
  process.stdout.write(JSON.stringify(report) + '\n')
  process.exitCode = report.passed ? 0 : 1
}
