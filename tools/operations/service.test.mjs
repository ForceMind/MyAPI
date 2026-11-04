import test from 'node:test'
import assert from 'node:assert/strict'
import http from 'node:http'
import { mkdtemp, mkdir, writeFile, rm, readFile } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { createOperationsService } from './service.mjs'
import { publicPlan, JournalStore } from './store.mjs'
import { OperationsError } from './safety.mjs'

const oldId = 'a'.repeat(64), newId = 'b'.repeat(64)
const oldImage = 'sha256:' + '1'.repeat(64), newImage = 'sha256:' + '2'.repeat(64)
const secret = 'myapi_mgmt_' + 'a'.repeat(64), bridgeToken = 'z'.repeat(48)
const artifact = { run_id: '123', producer_attempt: '1', artifact_id: '456', source_sha: 'c'.repeat(40), tree_sha: 'd'.repeat(40), manifest_sha256: 'e'.repeat(64), image_id: newImage, version: '0.2.0-beta.35' }

async function fixture(t, options = {}) {
  const root = await mkdtemp(path.join(os.tmpdir(), 'myapi-operations-'))
  const state = path.join(root, 'state'), data = path.join(root, 'data'), backups = path.join(root, 'backups')
  for (const directory of [state, data, backups]) await mkdir(directory, { mode: 0o700 })
  await writeFile(path.join(data, 'my-api.db'), 'synthetic database', { mode: 0o600 })
  const instance = { id: 'primary', container: 'my-api', data_dir: data, sqlite_file: path.join(data, 'my-api.db'), health_url: 'http://127.0.0.1:3000/' }
  const config = { state_dir: state, backups_dir: backups, instances: [instance], docker_socket: '/var/run/docker.sock' }
  let revoked = false, owner = 1, keyId = 7, now = 1000, changed = false, sourceVersion = '0.2.0-beta.34'
  const calls = [], validations = []
  const source = { Id: oldId, Image: oldImage, State: { Running: true }, Config: { Env: ['MYAPI_EDITION=full', 'FEATURE_FLAG=preserve', 'SQLITE_PATH=/data/my-api.db'], WorkingDir: '/data', Image: oldImage, Labels: { 'io.myapi.edition': 'full' } }, HostConfig: { Binds: [data + ':/data'], PortBindings: { '3000/tcp': [{ HostIp: '127.0.0.1', HostPort: '3000' }] }, AutoRemove: false, Memory: 67108864, RestartPolicy: { Name: 'always', MaximumRetryCount: 0 } }, Mounts: [{ Source: data, Destination: '/data', Type: 'bind', RW: true }], NetworkSettings: { Networks: { bridge: { IPAMConfig: null, Aliases: null } } } }
  const publishedPorts = { '3000/tcp': [{ HostIp: '127.0.0.1', HostPort: '3000' }] }
  source.NetworkSettings.Ports = structuredClone(publishedPorts)
  let candidate
  const docker = {
    async inspectContainer(id) { calls.push(['inspect', id]); if (id === newId) return structuredClone(candidate); const result = structuredClone(source); if (changed) result.Config.Env.push('CHANGED=1'); return result },
    async inspectImage(id) { return { Id: id, Os: 'linux', Architecture: 'amd64', Config: { Labels: { 'io.myapi.edition': 'full' } } } },
    async disableRestart(id) { calls.push(['disableRestart', id]); source.HostConfig.RestartPolicy = { Name: 'no', MaximumRetryCount: 0 }; if (options.disableUnknown) throw new Error('private update response lost') },
    async stop(id) { calls.push(['stop', id]); source.State.Running = false; source.NetworkSettings.Ports = {}; source.NetworkSettings.Networks.bridge.IPAddress = ''; source.NetworkSettings.Networks.bridge.EndpointID = '' },
    async create(name, value) { calls.push(['create', name, structuredClone(value)]); candidate = { ...structuredClone(source), Id: newId, Image: value.Image, Config: Object.fromEntries(Object.entries(value).filter(([k]) => !['HostConfig', 'NetworkingConfig'].includes(k))), HostConfig: value.HostConfig }; return { Id: newId } },
    async start(id) { calls.push(['start', id]); candidate.State.Running = true; candidate.NetworkSettings.Ports = structuredClone(publishedPorts); if (options.startFailure) throw new Error('private Docker error /secret/path') },
  }
  const summary = { db: { bytes: 18, sha256: 'f'.repeat(64) }, '-wal': null, '-shm': null }
  const service = await createOperationsService(config, {
    bridgeToken, docker,
    github: { async verify() { calls.push(['github.verify']); return { zip_sha256: 'f'.repeat(64), zip_bytes: 42 } }, async download() { calls.push(['github.download']); if (options.downloadGate) await options.downloadGate } },
    async validateManagementKey(instance, request) {
      validations.push(structuredClone(request))
      if (revoked || request.secret !== undefined && request.secret !== secret || request.key_id !== undefined && request.key_id !== keyId || request.owner_id !== undefined && request.owner_id !== owner || options.prepareOnly && request.scope === 'operations.execute') throw new OperationsError('UNAUTHORIZED', 401)
      return { owner_id: owner, key_id: keyId }
    },
    async validateActor(instance, value) { if (value !== owner) throw new OperationsError('UNAUTHORIZED', 401); return { owner_id: value } },
    async verifyBundle() { calls.push(['verifyBundle']); if (options.imageMismatch) throw new OperationsError('IMAGE_ID_MISMATCH'); return { exact_image: true } },
    async readVersion() { return sourceVersion },
    async verifyHealth() { calls.push(['health']); if (options.healthFailure) throw new Error('secret upstream URL'); return { health: true, version: true, static_assets: true } },
    async backup(request) { calls.push(['backup', structuredClone(request)]); return request.action === 'summary' ? structuredClone(summary) : { before: summary, after: summary, copied: summary } },
    async capacity() {}, async dataBytes() { return 18 },
    clock: () => now,
  })
  const servers = [http.createServer(service.publicHandler), http.createServer(service.bridgeHandler)]
  for (const server of servers) await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  async function request(route, { bridge = false, method = 'GET', body, token = bridge ? bridgeToken : secret, headers = {} } = {}) {
    const address = servers[bridge ? 1 : 0].address()
    const response = await fetch(`http://127.0.0.1:${address.port}${route}`, { method, headers: { Authorization: 'Bearer ' + token, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }), ...(bridge ? { 'X-MyAPI-Actor-ID': String(owner), 'X-MyAPI-Instance-ID': 'primary' } : {}), ...headers }, body: body === undefined ? undefined : JSON.stringify(body) })
    return { status: response.status, body: await response.json() }
  }
  async function prepare() { const result = await request('/v1/plans', { method: 'POST', body: { instance_id: 'primary', artifact } }); assert.equal(result.status, 200); return result.body.data }
  async function approve(plan) { return request(`/v1/plans/${plan.id}/approve`, { bridge: true, method: 'POST', body: { digest: plan.digest } }) }
  async function execute(plan) { return request(`/v1/plans/${plan.id}/execute`, { method: 'POST', body: { digest: plan.digest } }) }
  t.after(async () => { await service.close(); await Promise.all(servers.map(server => new Promise(resolve => { server.closeAllConnections(); server.close(resolve) }))); await rm(root, { recursive: true, force: true }) })
  return { root, config, service, source, calls, validations, request, prepare, approve, execute, revoke: () => { revoked = true }, change: () => { changed = true }, setOwner: value => { owner = value }, setKey: value => { keyId = value }, setTime: value => { now = value }, setVersion: value => { sourceVersion = value } }
}

test('public channel cannot approve; bridge cannot prepare or execute, and requires a distinct token', async t => {
  const f = await fixture(t); const plan = await f.prepare()
  assert.equal((await f.request(`/v1/plans/${plan.id}/approve`, { method: 'POST', body: { digest: plan.digest } })).status, 404)
  assert.equal((await f.request('/v1/plans', { bridge: true, method: 'POST', body: { instance_id: 'primary', artifact } })).status, 404)
  assert.equal((await f.request(`/v1/plans/${plan.id}/execute`, { bridge: true, method: 'POST', body: { digest: plan.digest } })).status, 404)
  assert.equal((await f.request('/v1/plans', { bridge: true, token: secret })).status, 401)
  assert.equal((await f.request('/v1/instances', { headers: { Origin: 'https://untrusted.example' } })).status, 403)
})

test('exact image mismatch is durable and leaves old running without stop or backup', async t => {
  const f = await fixture(t, { imageMismatch: true }); const plan = await f.prepare()
  assert.equal((await f.approve(plan)).status, 200)
  assert.equal((await f.execute(plan)).status, 200)
  await f.service.engine.drain()
  const result = (await f.request(`/v1/plans/${plan.id}`)).body.data
  assert.equal(result.state, 'failed_before_start'); assert.equal(result.code, 'IMAGE_ID_MISMATCH')
  assert.equal(result.target_start_attempted, false); assert.equal(result.backup_created, false)
  assert.equal(f.source.State.Running, true)
  assert.equal(f.calls.some(([action]) => ['disableRestart', 'stop', 'create', 'start', 'backup'].includes(action)), false)
  assert.equal((await f.execute(plan)).status, 409)
})

test('one approval starts only exact candidate with original environment, limits, ports and registered data; old stays retained', async t => {
  const f = await fixture(t); const plan = await f.prepare(); assert.equal((await f.approve(plan)).status, 200)
  assert.equal((await f.execute(plan)).status, 200); await f.service.engine.drain()
  assert.equal((await f.request(`/v1/plans/${plan.id}`)).body.data.state, 'succeeded')
  const create = f.calls.find(([action]) => action === 'create')[2]
  assert.deepEqual(create.Env, f.source.Config.Env); assert.deepEqual(create.HostConfig, f.service.engine.get(plan.id).source_snapshot.HostConfig); assert.equal(create.Image, newImage)
  assert.deepEqual(f.calls.filter(([action]) => action === 'stop'), [['stop', oldId]])
  assert.deepEqual(f.calls.filter(([action]) => action === 'start'), [['start', newId]])
  assert.equal(f.calls.find(([action, request]) => action === 'backup' && request.action === 'backup')[1].data_dir, f.config.instances[0].data_dir)
  assert.equal((await f.execute(plan)).status, 409)
  const serialized = JSON.stringify((await f.request(`/v1/plans/${plan.id}`)).body)
  for (const hidden of ['FEATURE_FLAG', f.root, 'source_snapshot', 'database_before', secret]) assert.equal(serialized.includes(hidden), false)
})

test('changed source config rejects before stop and stale approval cannot execute', async t => {
  const f = await fixture(t); const plan = await f.prepare(); await f.approve(plan); f.change(); await f.execute(plan); await f.service.engine.drain()
  assert.equal(f.service.engine.get(plan.id).code, 'SOURCE_CONFIG_CHANGED'); assert.equal(f.source.State.Running, true)
  const g = await fixture(t); const expired = await g.prepare(); await g.approve(expired); g.setTime(1301)
  assert.equal((await g.execute(expired)).status, 409); assert.equal(g.calls.some(([action]) => action === 'stop'), false)
})

test('revocation, original key identity and Root actor are revalidated; prepare-only keys never approve', async t => {
  const f = await fixture(t); const plan = await f.prepare(); await f.approve(plan); f.revoke()
  assert.equal((await f.execute(plan)).status, 401)
  const g = await fixture(t, { prepareOnly: true }); const restricted = await g.prepare()
  assert.equal((await g.approve(restricted)).status, 401)
  assert.equal(g.source.State.Running, true)
  const h = await fixture(t); const owned = await h.prepare(); h.setKey(8)
  assert.equal((await h.request(`/v1/plans/${owned.id}`)).status, 404)
  h.setKey(7); h.setOwner(2)
  assert.equal((await h.request(`/v1/plans/${owned.id}`, { bridge: true })).status, 404)
})

test('startup or verification failure freezes instance and never restarts old or restores database', async t => {
  for (const failure of ['startFailure', 'healthFailure']) {
    const f = await fixture(t, { [failure]: true }); const plan = await f.prepare(); await f.approve(plan); await f.execute(plan); await f.service.engine.drain()
    const result = (await f.request(`/v1/plans/${plan.id}`)).body.data
    assert.equal(result.state, 'recovery_required'); assert.equal(result.target_start_attempted, true); assert.equal(result.backup_created, true)
    assert.deepEqual(f.calls.filter(([action]) => action === 'start'), [['start', newId]])
    assert.equal(f.service.engine.store.frozen.has('primary'), true)
    assert.equal((await f.request('/v1/plans', { method: 'POST', body: { instance_id: 'primary', artifact } })).status, 409)
    assert.equal(JSON.stringify(result).includes('secret'), false)
  }
})

test('journal restart marks in-flight plans interrupted, freezes instance and preserves queryable result', async t => {
  const f = await fixture(t); const plan = await f.prepare()
  const internal = f.service.engine.get(plan.id); internal.state = 'starting'; internal.target_start_attempted = true
  await f.service.engine.store.lock('primary'); await f.service.engine.store.save(internal)
  const restarted = new JournalStore(f.config.state_dir); await restarted.init()
  assert.equal(restarted.plans.get(plan.id).state, 'interrupted'); assert.equal(restarted.frozen.has('primary'), true)
  assert.equal(JSON.parse(await readFile(path.join(f.config.state_dir, plan.id + '.json'))).state, 'interrupted')
  assert.equal(f.calls.some(([action]) => action === 'start'), false)
})

test('capabilities are served independently and credentials are validated on every request', async t => {
  const f = await fixture(t)
  assert.deepEqual((await f.request('/v1/capabilities')).body.data.operations, ['operations.instances', 'operations.plans', 'operations.plan', 'operations.prepare', 'operations.execute'])
  const before = f.validations.length
  assert.equal((await f.request('/v1/instances')).status, 200); assert.ok(f.validations.length > before)
  f.revoke(); assert.equal((await f.request('/v1/capabilities')).status, 401)
})

test('version downgrade and changed artifact after approval are rejected before old container stop', async t => {
  const f = await fixture(t)
  const downgrade = { ...artifact, version: '0.2.0-beta.33' }
  const result = await f.request('/v1/plans', { method: 'POST', body: { instance_id: 'primary', artifact: downgrade } })
  assert.equal(result.status, 400); assert.equal(result.body.code, 'OPERATIONS_VERSION_NOT_FORWARD')
  const plan = await f.prepare(); await f.approve(plan)
  const internal = f.service.engine.store.plans.get(plan.id)
  internal.artifact.manifest_sha256 = '8'.repeat(64)
  assert.equal((await f.execute(plan)).status, 409)
  assert.equal(f.calls.some(([action]) => action === 'stop'), false)
})

test('risk plans remain listed beyond latest one hundred terminal records and failed inspect stays unknown', async t => {
  const f = await fixture(t); const plan = await f.prepare()
  const internal = f.service.engine.get(plan.id); internal.state = 'interrupted'; internal.created_at = 1
  f.service.engine.store.plans.set(plan.id, internal)
  for (let index = 0; index < 105; index++) f.service.engine.store.plans.set(String(index), { ...internal, id: index.toString(16).padStart(64, '0'), state: 'succeeded', created_at: 2000 + index })
  const result = (await f.request('/v1/plans')).body.data
  assert.equal(result.length, 101); assert.equal(result.some(item => item.id === plan.id && item.state === 'interrupted'), true)
  f.service.engine.docker.inspectContainer = async () => { throw new Error('secret host failure') }
  const instances = (await f.request('/v1/instances')).body.data
  assert.equal(instances[0].state, 'unknown'); assert.equal(instances[0].code, 'INSTANCE_UNAVAILABLE')
})

test('concurrent execute consumes one approval; replay cannot clear the live instance lock', async t => {
  let release
  const downloadGate = new Promise(resolve => { release = resolve })
  t.after(() => release())
  const f = await fixture(t, { downloadGate }); const plan = await f.prepare(); await f.approve(plan)
  const outcomes = await Promise.all([f.execute(plan), f.execute(plan)])
  assert.deepEqual(outcomes.map(result => result.status).sort(), [200, 409])
  assert.equal(f.service.engine.busy.has('primary'), true)
  release(); await f.service.engine.drain()
  assert.deepEqual(f.calls.filter(([action]) => action === 'stop'), [['stop', oldId]])
  assert.deepEqual(f.calls.filter(([action]) => action === 'start'), [['start', newId]])
})

test('revocation, approval expiry or source version change during download aborts before stopping', async t => {
  for (const change of ['revoke', 'expire', 'version']) {
    let release
    const downloadGate = new Promise(resolve => { release = resolve })
    t.after(() => release())
    const f = await fixture(t, { downloadGate }); const plan = await f.prepare(); await f.approve(plan); await f.execute(plan)
    if (change === 'revoke') f.revoke()
    if (change === 'expire') f.setTime(1301)
    if (change === 'version') f.setVersion('0.2.0-beta.33')
    release(); await f.service.engine.drain()
    assert.equal(f.service.engine.get(plan.id).state, 'failed_before_start')
    assert.equal(f.calls.some(([action]) => action === 'stop'), false)
    assert.equal(f.source.State.Running, true)
  }
})

test('old always policy is disabled durably before stop; candidate keeps original policy, runtime network reset is tolerated', async t => {
  const f = await fixture(t)
  f.source.NetworkSettings.Networks.bridge.IPAddress = '172.17.0.9'
  f.source.NetworkSettings.Networks.bridge.EndpointID = '9'.repeat(64)
  const plan = await f.prepare(); await f.approve(plan); await f.execute(plan); await f.service.engine.drain()
  const result = f.service.engine.get(plan.id)
  assert.equal(result.state, 'succeeded'); assert.equal(result.source_mutation_attempted, true); assert.equal(result.source_restart_disabled, true)
  assert.deepEqual(f.source.HostConfig.RestartPolicy, { Name: 'no', MaximumRetryCount: 0 })
  assert.deepEqual(f.calls.find(([action]) => action === 'create')[2].HostConfig.RestartPolicy, { Name: 'always', MaximumRetryCount: 0 })
  assert.ok(f.calls.findIndex(([action]) => action === 'disableRestart') < f.calls.findIndex(([action]) => action === 'stop'))
})

test('unknown restart-policy update outcome freezes instance without stop, retry or old-policy restoration', async t => {
  const f = await fixture(t, { disableUnknown: true }); const plan = await f.prepare(); await f.approve(plan); await f.execute(plan); await f.service.engine.drain()
  const result = f.service.engine.get(plan.id)
  assert.equal(result.state, 'recovery_required'); assert.equal(result.source_mutation_attempted, true); assert.equal(result.source_restart_disabled, false)
  assert.equal(f.calls.filter(([action]) => action === 'disableRestart').length, 1)
  assert.equal(f.calls.some(([action]) => ['stop', 'start', 'create'].includes(action)), false)
  assert.deepEqual(f.source.HostConfig.RestartPolicy, { Name: 'no', MaximumRetryCount: 0 })
  assert.equal(f.service.engine.store.frozen.has('primary'), true)
})

test('anonymous volumes and dynamic published ports fail prepare before any source mutation', async t => {
  for (const mode of ['volume', 'publishAll', 'portZero', 'portEmpty']) {
    const f = await fixture(t)
    if (mode === 'volume') f.source.Mounts.push({ Type: 'volume', Name: 'anonymous-generated-id', Source: '/var/lib/docker/volumes/anonymous-generated-id/_data', Destination: '/extra', RW: true })
    if (mode === 'publishAll') f.source.HostConfig.PublishAllPorts = true
    if (mode === 'portZero') f.source.HostConfig.PortBindings['3000/tcp'][0].HostPort = '0'
    if (mode === 'portEmpty') f.source.HostConfig.PortBindings['3000/tcp'][0].HostPort = ''
    const result = await f.request('/v1/plans', { method: 'POST', body: { instance_id: 'primary', artifact } })
    assert.equal(result.status, 400)
    assert.equal(f.calls.some(([action]) => ['disableRestart', 'stop', 'create', 'start'].includes(action)), false)
    assert.equal(f.source.State.Running, true)
  }
})

test('health version source must be the registered container fixed loopback publication before mutation', async t => {
  for (const mode of ['https', 'remote', 'otherPort', 'unpublished', 'host']) {
    const f = await fixture(t)
    if (mode === 'https') f.config.instances[0].health_url = 'https://127.0.0.1:3000/'
    if (mode === 'remote') f.config.instances[0].health_url = 'http://other-instance.example:3000/'
    if (mode === 'otherPort') f.config.instances[0].health_url = 'http://127.0.0.1:3001/'
    if (mode === 'unpublished') f.source.NetworkSettings.Ports = {}
    if (mode === 'host') f.source.HostConfig.NetworkMode = 'host'
    const result = await f.request('/v1/plans', { method: 'POST', body: { instance_id: 'primary', artifact } })
    assert.equal(result.status, 400); assert.equal(result.body.code, 'OPERATIONS_HEALTH_BINDING_MISMATCH')
    assert.equal(f.calls.some(([action]) => ['disableRestart', 'stop', 'start', 'create'].includes(action)), false)
  }
})

test('mount order does not change approval digest or configuration equality; every mount field still matters', async t => {
  const { containerDigest, sameRuntimeConfig } = await import('./docker.mjs')
  const f = await fixture(t)
  f.source.Mounts.push({ Type: 'bind', Source: '/fixed/logs', Destination: '/logs', RW: false }, { Type: 'bind', Source: '/fixed/bridge', Destination: '/operations', RW: false })
  const reordered = structuredClone(f.source); reordered.Mounts.reverse()
  assert.equal(containerDigest(reordered), containerDigest(f.source)); assert.equal(sameRuntimeConfig(f.source, reordered), true)
  reordered.Mounts[0].RW = true
  assert.notEqual(containerDigest(reordered), containerDigest(f.source)); assert.equal(sameRuntimeConfig(f.source, reordered), false)
  const envChanged = structuredClone(f.source); envChanged.Config.Env.reverse()
  assert.notEqual(containerDigest(envChanged), containerDigest(f.source)); assert.equal(sameRuntimeConfig(f.source, envChanged), false)
})

test('journal same-second successful A to B to C chain recovers C; forks and cycles freeze without guessing', async t => {
  for (const mode of ['chain', 'fork', 'cycle']) {
    const f = await fixture(t); const plan = await f.prepare()
    const original = f.service.engine.get(plan.id)
    const first = { ...original, id: '1'.repeat(64), state: 'succeeded', source_container_id: 'a'.repeat(64), target_container_id: 'b'.repeat(64), created_at: 1000 }
    const second = { ...original, id: '0'.repeat(64), state: 'succeeded', source_container_id: mode === 'fork' ? 'a'.repeat(64) : 'b'.repeat(64), target_container_id: mode === 'cycle' ? 'a'.repeat(64) : 'c'.repeat(64), created_at: 1000 }
    await f.service.engine.store.save(first); await f.service.engine.store.save(second)
    const restarted = new JournalStore(f.config.state_dir); await restarted.init()
    if (mode === 'chain') { assert.equal(restarted.current.get('primary').target_container_id, 'c'.repeat(64)); assert.equal(restarted.frozen.has('primary'), false) }
    else { assert.equal(restarted.current.has('primary'), false); assert.equal(restarted.frozen.has('primary'), true) }
  }
})
