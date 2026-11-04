#!/usr/bin/env node
// Actions-only real Docker/SQLite qualification. This program is not an
// operations endpoint and accepts no command, container, mount or URL input.
import assert from 'node:assert/strict'
import { randomBytes } from 'node:crypto'
import { execFile, spawn } from 'node:child_process'
import { promisify } from 'node:util'
import { chmod, lstat, mkdir, readFile, realpath, rm, writeFile } from 'node:fs/promises'
import { watch } from 'node:fs'
import http from 'node:http'
import https from 'node:https'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { setTimeout } from 'node:timers/promises'
import { DockerClient, candidateConfig, configuredMounts, containerDigest, sameRuntimeConfig, samePublishedPorts } from './docker.mjs'
import { artifactInput, digest } from './safety.mjs'
import { createOperationsService } from './service.mjs'
import { claimDaemon } from './daemon.mjs'

const executeFile = promisify(execFile)
const baselineSha = '55531d1e4f21a426d209b1b2d76e451e10bcccaf'
const baselineImage = 'myapi:operations-baseline-55531d1e'
const moduleDirectory = path.dirname(fileURLToPath(import.meta.url))
const nodeExecutable = process.execPath
const docker = new DockerClient()
const labelName = 'io.myapi.operations.fixture'
const runId = process.env.GITHUB_RUN_ID
const fixtureCases = ['positive', 'identity', 'anonymous', 'publish-all', 'dynamic', 'crash', 'fault']
const children = new Set(), servers = new Set(), factoryServices = new Set(), guards = new Set()
let rootOwned = false
let root, tokenFile, certificateFile, privateKeyFile, reportFile, phase = 'gate', fixturePassword
let artifact, report

class QualificationError extends Error { constructor(code) { super(code); this.code = code } }
function demand(condition, code) { if (!condition) throw new QualificationError(code) }
function safeFailure(error) { return typeof error?.code === 'string' && /^[A-Z][A-Z0-9_]{0,79}$/.test(error.code) ? error.code : 'QUALIFICATION_ASSERTION_FAILED' }
function mark(name) { report.checks[name] = true }
function progress(value) { phase = value; process.stdout.write('OPERATIONS_QUALIFICATION_' + value.toUpperCase().replaceAll('-', '_') + '\n') }
async function command(executable, arguments_, { input, timeout = 60_000 } = {}) {
  try {
    const promise = executeFile(executable, arguments_, { encoding: 'utf8', maxBuffer: 8 * 1024 * 1024, timeout, killSignal: 'SIGKILL', env: { PATH: '/usr/local/bin:/usr/bin:/bin', LANG: 'C.UTF-8', GITHUB_ACTIONS: 'true', GITHUB_RUN_ID: runId } })
    if (input !== undefined) { promise.child.stdin.on('error', () => {}); promise.child.stdin.end(input) }
    return (await promise).stdout
  } catch { throw new QualificationError('FIXED_COMMAND_FAILED') }
}
async function dockerCommand(arguments_, options) { return command('/usr/bin/docker', arguments_, options) }
async function until(check, code, timeout = 120_000, interval = 150) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    try { const result = await check(); if (result) return result } catch {}
    await setTimeout(Math.min(interval, Math.max(1, deadline - Date.now())))
  }
  throw new QualificationError(code)
}
async function protectedWrite(filename, value) { await writeFile(filename, value, { flag: 'wx', mode: 0o600 }); await chmod(filename, 0o600) }
async function fixtureDatabase(fixture, action) {
  const input = { root, case: fixture.name, action }
  if (action === 'seed') { input.management_hash = digest(fixture.managementKey); input.token_secret = randomBytes(32).toString('hex') }
  let output
  try { output = await command('/usr/bin/python3', ['-I', path.join(moduleDirectory, 'qualification-fixture.py')], { input: JSON.stringify(input), timeout: 30_000 }); return JSON.parse(output) }
  catch { throw new QualificationError('OWNED_SQLITE_FIXTURE_FAILED') }
}
async function jsonHTTP(url, { method = 'GET', body, headers = {}, timeout = 10_000 } = {}) {
  const response = await fetch(url, { method, redirect: 'manual', signal: AbortSignal.timeout(timeout), headers: { ...(body === undefined ? {} : { 'Content-Type': 'application/json' }), ...headers }, body: body === undefined ? undefined : JSON.stringify(body) })
  const text = await response.text(); demand(Buffer.byteLength(text) <= 2 * 1024 * 1024, 'HTTP_RESPONSE_BOUND')
  let parsed
  try { parsed = JSON.parse(text) } catch { throw new QualificationError('HTTP_JSON_FAILED') }
  return { status: response.status, body: parsed }
}
async function transport(fixture, route, { bridge = false, method = 'GET', body, timeout = 12_000 } = {}) {
  const serialized = body === undefined ? undefined : Buffer.from(JSON.stringify(body))
  const options = { method, path: route, headers: { Authorization: 'Bearer ' + (bridge ? fixture.bridgeToken : fixture.managementKey), ...(serialized ? { 'Content-Type': 'application/json', 'Content-Length': serialized.length } : {}), ...(bridge ? { 'X-MyAPI-Actor-ID': '1', 'X-MyAPI-Instance-ID': 'primary' } : {}) } }
  const implementation = bridge ? http : https
  if (bridge) options.socketPath = fixture.config.unix_socket
  else Object.assign(options, { hostname: '127.0.0.1', port: fixture.config.listen.port, ca: await readFile(certificateFile), servername: 'localhost', rejectUnauthorized: true })
  return new Promise((resolve, reject) => {
    const request = implementation.request(options, response => {
      const chunks = []; let bytes = 0
      response.on('data', chunk => { bytes += chunk.length; if (bytes > 2 * 1024 * 1024) response.destroy(); else chunks.push(chunk) })
      response.on('error', () => reject(new QualificationError('TLS_RESPONSE_FAILED')))
      response.on('end', () => { try { resolve({ status: response.statusCode, body: JSON.parse(Buffer.concat(chunks)) }) } catch { reject(new QualificationError('TLS_JSON_FAILED')) } })
    })
    request.setTimeout(timeout, () => request.destroy())
    request.on('error', () => reject(new QualificationError('TLS_REQUEST_FAILED')))
    request.end(serialized)
  })
}
function success(result, code) {
  if (!(result.status === 200 && result.body?.success === true)) throw new QualificationError(/^OPERATIONS_[A-Z0-9_]{1,80}$/.test(result.body?.code || '') ? result.body.code : code)
  return result.body.data
}
async function ownedContainer(id) {
  const info = await docker.inspectContainer(id)
  demand(info.Config?.Labels?.[labelName] === runId, 'CONTAINER_OWNERSHIP_MISMATCH')
  return info
}
async function ownedIds() {
  const output = await dockerCommand(['container', 'ls', '--all', '--quiet', '--no-trunc', '--filter', `label=${labelName}=${runId}`])
  const values = output.trim() ? output.trim().split(/\s+/) : []
  demand(values.every(value => /^[a-f0-9]{64}$/.test(value)), 'CONTAINER_INVENTORY_INVALID')
  for (const id of values) await ownedContainer(id)
  return values
}
async function createFixture(name, index, mode = 'fixed') {
  demand(fixtureCases.includes(name), 'CASE_INVALID')
  progress(name + '-create-source')
  const directory = path.join(root, name)
  const dataDir = path.join(directory, 'data'), socketDirectory = path.join(directory, 'socket'), credentials = path.join(directory, 'bridge-credentials')
  for (const item of [directory, dataDir, socketDirectory, credentials, path.join(directory, 'state'), path.join(directory, 'backups')]) await mkdir(item, { mode: 0o700 })
  const fixture = { name, directory, dataDir, managementKey: 'myapi_mgmt_' + randomBytes(32).toString('hex'), bridgeToken: randomBytes(32).toString('base64url') }
  const bridgeTokenFile = path.join(credentials, 'bridge.token')
  await protectedWrite(bridgeTokenFile, fixture.bridgeToken)
  const env = [
    'MYAPI_EDITION=full', 'BATCH_UPDATE_ENABLED=false', 'MYAPI_ACCOUNTING_CONFIG_VERSION=1', 'CHANNEL_QUOTA_SYNC_ENABLED=false',
    'MYAPI_CODEX_CHAT_DEFAULT_CACHE_IDENTITY_ENABLED=true', 'FULL_CONTENT_LOG_ENABLED=true', 'FULL_CONTENT_LOG_DIR=/data/full-content',
    'MYAPI_UPSTREAM_DIAGNOSTIC_ENABLED=false', 'SQLITE_PATH=/data/my-api.db?_busy_timeout=30000', 'SESSION_SECRET=' + randomBytes(32).toString('hex'),
    'MYAPI_OPERATIONS_SOCKET=/operations/bridge.sock', 'MYAPI_OPERATIONS_BRIDGE_TOKEN_FILE=/operations-credentials/bridge.token', 'MYAPI_OPERATIONS_INSTANCE_ID=primary',
  ]
  fixture.envDigest = digest(env)
  const envFile = path.join(directory, 'container.env'); await protectedWrite(envFile, env.join('\n') + '\n')
  const name_ = `myapi-operations-fixture-${runId}-${name}`
  const args = ['create', '--pull=never', '--platform=linux/amd64', '--name', name_, '--label', `${labelName}=${runId}`, '--restart=always', '--memory=768m', '--cpus=1', '--env-file', envFile,
    '--mount', `type=bind,src=${dataDir},dst=/data`, '--mount', `type=bind,src=${socketDirectory},dst=/operations,readonly`, '--mount', `type=bind,src=${credentials},dst=/operations-credentials,readonly`]
  if (mode === 'publish-all') args.push('--publish-all')
  else args.push('--publish', `127.0.0.1:${mode === 'dynamic' ? '0' : String(18880 + index)}:3000`)
  if (mode === 'anonymous') args.push('--mount', 'type=volume,dst=/anonymous-fixture')
  args.push(baselineImage)
  const id = (await dockerCommand(args)).trim(); demand(/^[a-f0-9]{64}$/.test(id), 'SOURCE_CREATE_FAILED'); fixture.sourceId = id
  await docker.start(id)
  const source = await ownedContainer(id)
  const published = source.NetworkSettings?.Ports?.['3000/tcp']?.find(item => ['127.0.0.1', '0.0.0.0', '::'].includes(item.HostIp))
  demand(published && /^[1-9][0-9]{0,4}$/.test(published.HostPort), 'SOURCE_PORT_UNAVAILABLE')
  fixture.applicationURL = `http://127.0.0.1:${published.HostPort}`
  await until(async () => { const response = await jsonHTTP(fixture.applicationURL + '/api/status'); return response.status === 200 && response.body.success === true && response.body.data?.version === '0.2.0-beta.32' && response.body.data.enable_batch_update === false }, 'BASELINE_STATUS_FAILED')
  const setup = await jsonHTTP(fixture.applicationURL + '/api/setup', { method: 'POST', body: { username: 'opsroot', password: fixturePassword, confirmPassword: fixturePassword, SelfUseModeEnabled: false, DemoSiteEnabled: false } })
  success(setup, 'BASELINE_SETUP_FAILED')
  progress(name + '-seed-sqlite')
  fixture.baselineAccounting = await fixtureDatabase(fixture, 'seed')
  fixture.source = await ownedContainer(id)
  demand(env.every(value => fixture.source.Config.Env.includes(value)), 'SOURCE_ENVIRONMENT_MISMATCH')
  fixture.sourceDigest = containerDigest(fixture.source)
  fixture.config = {
    listen: { host: '127.0.0.1', port: 19443 + index, tls_cert_file: certificateFile, tls_key_file: privateKeyFile }, unix_socket: path.join(socketDirectory, 'bridge.sock'),
    bridge_token_file: bridgeTokenFile, github_token_file: tokenFile, docker_socket: '/var/run/docker.sock', state_dir: path.join(directory, 'state'), backups_dir: path.join(directory, 'backups'),
    instances: [{ id: 'primary', container: id, data_dir: dataDir, sqlite_file: path.join(dataDir, 'my-api.db'), health_url: fixture.applicationURL + '/' }],
  }
  fixture.configFile = path.join(directory, 'config.json'); await protectedWrite(fixture.configFile, JSON.stringify(fixture.config))
  return fixture
}
function spawnDaemon(fixture) {
  const child = spawn(nodeExecutable, [path.join(moduleDirectory, 'daemon.mjs'), fixture.configFile], { stdio: ['ignore', 'ignore', 'ignore'], env: { PATH: '/usr/local/bin:/usr/bin:/bin', LANG: 'C.UTF-8' } })
  children.add(child); child.once('close', () => children.delete(child)); child.on('error', () => {})
  return child
}
async function startDaemon(fixture) {
  progress(fixture.name + '-start-tls-daemon')
  const child = spawnDaemon(fixture)
  await until(async () => {
    if (child.exitCode !== null || child.signalCode !== null) throw new QualificationError('DAEMON_EXITED')
    const response = await transport(fixture, '/v1/capabilities'); return response.status === 200 && response.body?.success === true
  }, 'DAEMON_TLS_START_FAILED', 30_000)
  fixture.daemon = child
  return child
}
async function childExit(child, timeout = 15_000) {
  if (child.exitCode !== null || child.signalCode !== null) return
  await Promise.race([new Promise(resolve => child.once('close', resolve)), new Promise((_, reject) => { const timer = globalThis.setTimeout(() => reject(new QualificationError('CHILD_EXIT_TIMEOUT')), timeout); timer.unref() })])
}
async function stopDaemon(fixture, signal = 'SIGTERM') { const child = fixture.daemon; if (!child) return; child.kill(signal); await childExit(child, 90_000); fixture.daemon = undefined }
async function prepare(fixture, target = artifact) { return success(await transport(fixture, '/v1/plans', { method: 'POST', body: { instance_id: 'primary', artifact: target } }), 'PREPARE_FAILED') }
async function approve(fixture, plan) {
  const result = success(await transport(fixture, `/v1/plans/${plan.id}/approve`, { bridge: true, method: 'POST', body: { digest: plan.digest } }), 'APPROVAL_FAILED')
  assert.equal(result.digest, plan.digest); assert.equal(result.approved_by, 1)
  return result
}
async function execute(fixture, plan) { return transport(fixture, `/v1/plans/${plan.id}/execute`, { method: 'POST', body: { digest: plan.digest } }) }
async function readPlan(fixture, plan) { return success(await transport(fixture, `/v1/plans/${plan.id}`), 'PLAN_READ_FAILED') }
async function candidateDifference(fixture, result) {
  // Names come only from these fixed Engine API field allowlists, never from
  // Env, labels, user-defined keys, paths, headers or raw inspect output.
  const configFields = ['Hostname', 'Domainname', 'User', 'AttachStdin', 'AttachStdout', 'AttachStderr', 'ExposedPorts', 'Tty', 'OpenStdin', 'StdinOnce', 'Env', 'Cmd', 'Healthcheck', 'ArgsEscaped', 'Image', 'Volumes', 'WorkingDir', 'Entrypoint', 'NetworkDisabled', 'MacAddress', 'OnBuild', 'Labels', 'StopSignal', 'StopTimeout', 'Shell']
  const hostFields = ['Binds', 'ContainerIDFile', 'LogConfig', 'NetworkMode', 'PortBindings', 'RestartPolicy', 'AutoRemove', 'VolumeDriver', 'VolumesFrom', 'ConsoleSize', 'Annotations', 'CapAdd', 'CapDrop', 'CgroupnsMode', 'Dns', 'DnsOptions', 'DnsSearch', 'ExtraHosts', 'GroupAdd', 'IpcMode', 'Cgroup', 'Links', 'OomScoreAdj', 'PidMode', 'Privileged', 'PublishAllPorts', 'ReadonlyRootfs', 'SecurityOpt', 'StorageOpt', 'Tmpfs', 'UTSMode', 'UsernsMode', 'ShmSize', 'Sysctls', 'Runtime', 'Isolation', 'CpuShares', 'Memory', 'NanoCpus', 'CgroupParent', 'BlkioWeight', 'BlkioWeightDevice', 'BlkioDeviceReadBps', 'BlkioDeviceWriteBps', 'BlkioDeviceReadIOps', 'BlkioDeviceWriteIOps', 'CpuPeriod', 'CpuQuota', 'CpuRealtimePeriod', 'CpuRealtimeRuntime', 'CpusetCpus', 'CpusetMems', 'Devices', 'DeviceCgroupRules', 'DeviceRequests', 'MemoryReservation', 'MemorySwap', 'MemorySwappiness', 'OomKillDisable', 'PidsLimit', 'Ulimits', 'CpuCount', 'CpuPercent', 'IOMaximumIOps', 'IOMaximumBandwidth', 'MaskedPaths', 'ReadonlyPaths', 'Init', 'Mounts']
  let differences = { available: false }
  try {
    const journal = JSON.parse(await readFile(path.join(fixture.config.state_dir, result.id + '.json')))
    demand(journal.id === result.id && journal.instance_id === 'primary', 'DIAGNOSTIC_JOURNAL_MISMATCH')
    const actual = await ownedContainer(result.target_container_id)
    const targetImage = await docker.inspectImage(result.target_image_id)
    const desired = candidateConfig(journal.source_snapshot, result.target_image_id, targetImage)
    const expectedConfig = { ...journal.source_snapshot.Config, Image: result.target_image_id, Labels: desired.Labels }
    const different = (expected, value, field) => digest({ present: Object.hasOwn(expected || {}, field), value: expected?.[field] }) !== digest({ present: Object.hasOwn(value || {}, field), value: value?.[field] })
    const unknown = (expected, value, fields) => [...new Set([...Object.keys(expected || {}), ...Object.keys(value || {})])].some(field => !fields.includes(field) && different(expected, value, field))
    differences = { available: true, phase: result.target_start_attempted ? 'after_start' : 'before_start',
      config_fields: configFields.filter(field => different(expectedConfig, actual.Config, field)),
      host_config_fields: hostFields.filter(field => different(journal.source_snapshot.HostConfig, actual.HostConfig, field)),
      mounts_changed: digest(configuredMounts(journal.source_snapshot)) !== digest(configuredMounts(actual)),
      published_ports_changed: result.target_start_attempted ? !samePublishedPorts(journal.source_snapshot, actual) : null,
      unknown_config_fields_changed: unknown(expectedConfig, actual.Config, configFields), unknown_host_config_fields_changed: unknown(journal.source_snapshot.HostConfig, actual.HostConfig, hostFields) }
  } catch {}
  report.candidate_difference = differences
  process.stdout.write('OPERATIONS_CANDIDATE_DIFFERENCE ' + JSON.stringify(differences) + '\n')
}
async function terminal(fixture, plan, timeout = 300_000) {
  const result = await until(async () => { const value = await readPlan(fixture, plan); return ['succeeded', 'failed_before_start', 'recovery_required', 'interrupted'].includes(value.state) ? value : null }, 'TERMINAL_STATE_TIMEOUT', timeout, 250)
  if (['CANDIDATE_CONFIG_MISMATCH', 'TARGET_CONFIG_MISMATCH'].includes(result.code)) await candidateDifference(fixture, result)
  return result
}

function assertCharged(before, after) {
  assert.deepEqual(after.user, { quota: before.user.quota - 17, used_quota: before.user.used_quota + 17, request_count: before.user.request_count + 1 })
  assert.deepEqual(after.token, { remain_quota: before.token.remain_quota - 17, used_quota: before.token.used_quota + 17, quota_version: before.token.quota_version + 1 })
  assert.equal(after.logs.length, before.logs.length + 1)
  assert.equal(after.logs.find(log => log.request_id === 'operations-synthetic-after-start-17')?.quota, 17)
  assert.deepEqual(after.logs.find(log => log.request_id === 'operations-synthetic-baseline'), before.logs.find(log => log.request_id === 'operations-synthetic-baseline'))
  assert.equal(after.options_sha256, before.options_sha256)
}
async function assertReplay(fixture, plan) {
  const before = (await ownedIds()).sort()
  assert.equal((await execute(fixture, plan)).status, 409)
  assert.deepEqual((await ownedIds()).sort(), before)
}
async function assertRetainedOld(fixture, running, restart) { const old = await ownedContainer(fixture.sourceId); assert.equal(old.State.Running, running); assert.equal(old.HostConfig.RestartPolicy.Name, restart); return old }

async function positiveCase() {
  progress('positive')
  const fixture = await createFixture('positive', 0); await startDaemon(fixture)
  progress('positive-prepare-execute')
  const plan = await prepare(fixture); assert.equal(plan.source_version, '0.2.0-beta.32'); assert.equal(plan.version, artifact.version)
  const digestBefore = plan.digest; await approve(fixture, plan); success(await execute(fixture, plan), 'EXECUTE_FAILED')
  const result = await terminal(fixture, plan); demand(result.state === 'succeeded', /^[A-Z0-9_]+$/.test(result.code || '') ? 'TARGET_' + result.code : 'TARGET_NOT_SUCCEEDED'); assert.equal(result.digest, digestBefore)
  const current = await ownedContainer(result.target_container_id)
  assert.equal(current.Image, artifact.image_id); assert.equal(current.State.Running, true)
  const desired = candidateConfig(fixture.source, artifact.image_id, await docker.inspectImage(artifact.image_id))
  assert.equal(sameRuntimeConfig(fixture.source, current, desired.Labels), true); assert.equal(samePublishedPorts(fixture.source, current), true)
  assert.equal(current.HostConfig.RestartPolicy.Name, 'always')
  assert.equal(current.Mounts.find(mount => mount.Destination === '/data')?.Source, fixture.dataDir)
  await assertRetainedOld(fixture, false, 'no')
  const persisted = JSON.parse(await readFile(path.join(fixture.config.state_dir, plan.id + '.json')))
  assert.deepEqual(persisted.database_before, persisted.database_after_backup)
  for (const suffix of ['db', '-wal', '-shm']) {
    assert.equal(Object.hasOwn(persisted.database_before, suffix), true)
    const backupFile = path.join(fixture.config.backups_dir, plan.id, 'my-api.db' + (suffix === 'db' ? '' : suffix))
    const expected = persisted.database_before[suffix]
    if (expected === null) await assert.rejects(lstat(backupFile), { code: 'ENOENT' })
    else { const bytes = await readFile(backupFile); assert.equal(bytes.length, expected.bytes); assert.equal(digest(bytes), expected.sha256) }
  }
  demand(persisted.database_before.db?.sha256 && result.backup_created && result.checks?.consistent_backup && result.checks?.static_assets, 'BACKUP_OR_STATIC_PROOF_MISSING')
  const backupNames = await import('node:fs/promises').then(fs => fs.readdir(path.join(fixture.config.backups_dir, plan.id)))
  assert.equal(backupNames.includes('my-api.db'), true)
  assert.deepEqual(await fixtureDatabase(fixture, 'snapshot'), fixture.baselineAccounting)
  const statusAfter = success(await jsonHTTP(fixture.applicationURL + '/api/status'), 'TARGET_STATUS_FAILED'); assert.equal(statusAfter.enable_batch_update, false)
  mark('positive_exact_image_and_configuration'); mark('old_retained_with_restart_disabled'); mark('backup_db_wal_shm_summaries'); mark('real_status_version_and_static_assets'); mark('seeded_accounting_preserved')
  const charged = await fixtureDatabase(fixture, 'charge'); assertCharged(fixture.baselineAccounting, charged)
  await stopDaemon(fixture)
  await startDaemon(fixture)
  assert.deepEqual(await fixtureDatabase(fixture, 'snapshot'), charged)
  const instances = success(await transport(fixture, '/v1/instances'), 'INSTANCE_READ_FAILED')
  assert.equal(instances[0].current_container_id, result.target_container_id); assert.equal(instances[0].current_image_id, artifact.image_id)
  assert.equal((await readPlan(fixture, plan)).state, 'succeeded')
  await assertRetainedOld(fixture, false, 'no'); await assertReplay(fixture, plan)
  // This is an explicit synthetic HTTP client, not an executor login or model
  // request. It verifies the actual new Go application's directory-mounted bridge.
  const login = success(await jsonHTTP(fixture.applicationURL + '/api/user/login', { method: 'POST', body: { username: 'opsroot', password: fixturePassword } }), 'TARGET_SESSION_LOGIN_FAILED')
  demand(typeof login.access_token === 'string' && login.access_token.length > 20, 'TARGET_SESSION_MISSING')
  const plans = success(await jsonHTTP(fixture.applicationURL + '/api/operations/plans', { headers: { Authorization: 'Bearer ' + login.access_token, Origin: fixture.applicationURL } }), 'TARGET_DIRECTORY_BRIDGE_FAILED')
  assert.equal(plans.some(item => item.id === plan.id && item.digest === digestBefore && item.state === 'succeeded'), true)
  assert.deepEqual(await fixtureDatabase(fixture, 'snapshot'), charged)
  mark('committed_17_units_survive_daemon_restart'); mark('current_chain_pointer_after_restart'); mark('directory_socket_bridge_survives_restart'); mark('success_replay_rejected_without_second_candidate')
  await stopDaemon(fixture)
}

async function identityCase() {
  progress('identity')
  const fixture = await createFixture('identity', 1); await startDaemon(fixture)
  const wrong = { ...artifact, image_id: 'sha256:' + (artifact.image_id.endsWith('0') ? '1' : '0').repeat(64) }
  progress('identity-prepare-execute')
  const plan = await prepare(fixture, wrong); await approve(fixture, plan)
  const before = (await ownedIds()).sort(); success(await execute(fixture, plan), 'IDENTITY_EXECUTE_FAILED')
  const result = await terminal(fixture, plan)
  assert.equal(result.state, 'failed_before_start'); assert.equal(result.source_mutation_attempted, false); assert.equal(result.backup_created, false); assert.equal(result.target_start_attempted, false)
  demand(['IMAGE_ID_MISMATCH', 'MANIFEST_MISMATCH', 'MANIFEST_DIGEST_MISMATCH'].includes(result.code), 'IDENTITY_FAILURE_CODE_UNEXPECTED')
  assert.equal(containerDigest(await assertRetainedOld(fixture, true, 'always')), fixture.sourceDigest)
  assert.deepEqual((await ownedIds()).sort(), before); assert.deepEqual(await fixtureDatabase(fixture, 'snapshot'), fixture.baselineAccounting)
  await assertReplay(fixture, plan); mark('wrong_exact_identity_fails_before_source_mutation')
  await stopDaemon(fixture)
}

async function unsupportedCases() {
  for (const [name, index] of [['anonymous', 2], ['publish-all', 3], ['dynamic', 4]]) {
    progress(name)
    const fixture = await createFixture(name, index, name); await startDaemon(fixture)
    const before = (await ownedIds()).sort()
    const response = await transport(fixture, '/v1/plans', { method: 'POST', body: { instance_id: 'primary', artifact } })
    demand(response.status >= 400 && response.status < 500 && response.body?.success === false, 'UNSUPPORTED_INSTALLATION_ACCEPTED')
    assert.equal(containerDigest(await assertRetainedOld(fixture, true, 'always')), fixture.sourceDigest)
    assert.deepEqual((await ownedIds()).sort(), before); assert.deepEqual(await fixtureDatabase(fixture, 'snapshot'), fixture.baselineAccounting)
    mark(name.replaceAll('-', '_') + '_rejected_before_source_mutation')
    await stopDaemon(fixture)
  }
}

async function crashCase() {
  progress('crash')
  const fixture = await createFixture('crash', 5); const child = await startDaemon(fixture)
  progress('crash-prepare-approve')
  const plan = await prepare(fixture); await approve(fixture, plan)
  const filename = path.join(fixture.config.state_dir, plan.id + '.json')
  const approvedDigest = digest(await readFile(filename))
  const competing = spawnDaemon(fixture); await childExit(competing)
  demand(competing.exitCode !== 0, 'SECOND_DAEMON_ACQUIRED_LOCK')
  assert.equal(digest(await readFile(filename)), approvedDigest); assert.equal(child.exitCode, null)
  mark('second_daemon_cannot_mutate_live_journal')
  let observed = false, watcher
  const observe = async () => {
    if (observed) return true
    let journal
    try { journal = JSON.parse(await readFile(filename)) } catch { return false }
    if (['queued', 'verifying_artifact'].includes(journal.state) && !journal.source_mutation_attempted) { observed = true; child.kill('SIGKILL'); return true }
    return false
  }
  watcher = watch(fixture.config.state_dir, () => { observe().catch(() => {}) })
  try {
    const request = execute(fixture, plan).catch(() => null)
    await until(observe, 'DURABLE_QUEUED_NOT_OBSERVED', 30_000, 25)
    await childExit(child); fixture.daemon = undefined; await request
  } finally { watcher.close() }
  await startDaemon(fixture)
  const result = await readPlan(fixture, plan); assert.equal(result.state, 'interrupted')
  const instances = success(await transport(fixture, '/v1/instances'), 'CRASH_INSTANCE_READ_FAILED')
  assert.equal(instances[0].frozen, true); assert.equal(instances[0].state, 'recovery_required')
  assert.equal(containerDigest(await assertRetainedOld(fixture, true, 'always')), fixture.sourceDigest)
  assert.deepEqual(await fixtureDatabase(fixture, 'snapshot'), fixture.baselineAccounting)
  await assertReplay(fixture, plan)
  mark('sigkill_releases_os_lock_and_marks_interrupted'); mark('interrupted_instance_frozen_and_queryable'); mark('crash_preserves_original_data_and_configuration')
  await stopDaemon(fixture)
}

async function startFaultFactory(fixture) {
  const guard = await claimDaemon(fixture.config); guards.add(guard)
  guard.onLost(() => { process.stderr.write('OPERATIONS_QUALIFICATION_GUARD_LOST\n'); process.exit(1) })
  const actualDocker = new DockerClient()
  const faultDocker = Object.create(actualDocker)
  let injected = false
  faultDocker.start = async id => {
    demand(!injected, 'FAULT_START_REPLAYED'); injected = true
    await actualDocker.start(id)
    await ownedContainer(id)
    await until(async () => { const response = await jsonHTTP(fixture.applicationURL + '/api/status'); return response.body?.success === true && response.body.data?.version === artifact.version }, 'FAULT_TARGET_NOT_STARTED', 120_000)
    fixture.committedAfterStart = await fixtureDatabase(fixture, 'charge')
    assertCharged(fixture.baselineAccounting, fixture.committedAfterStart)
    await dockerCommand(['kill', '--signal', 'STOP', id]); fixture.stoppedTargetId = id
  }
  const service = await createOperationsService(fixture.config, { bridgeToken: fixture.bridgeToken, docker: faultDocker })
  factoryServices.add(service)
  const publicServer = https.createServer({ cert: await readFile(certificateFile), key: await readFile(privateKeyFile), minVersion: 'TLSv1.2' }, service.publicHandler)
  const bridgeServer = http.createServer(service.bridgeHandler)
  for (const server of [publicServer, bridgeServer]) { servers.add(server); server.requestTimeout = 30_000 }
  const listen = (server, options) => new Promise((resolve, reject) => { server.once('error', reject); server.listen(options, resolve) })
  await listen(bridgeServer, fixture.config.unix_socket); await chmod(fixture.config.unix_socket, 0o600)
  await listen(publicServer, { host: fixture.config.listen.host, port: fixture.config.listen.port })
  fixture.factory = { service, publicServer, bridgeServer, guard }
  return service
}
async function closeFactory(fixture) {
  const factory = fixture.factory
  if (!factory) return
  await factory.service.close(); factoryServices.delete(factory.service)
  await Promise.all([factory.publicServer, factory.bridgeServer].map(server => new Promise(resolve => { server.closeAllConnections(); server.close(() => { servers.delete(server); resolve() }) })))
  await factory.guard.release(); guards.delete(factory.guard); fixture.factory = undefined
}
async function faultCase() {
  progress('fault')
  const fixture = await createFixture('fault', 6); await startFaultFactory(fixture)
  progress('fault-prepare-execute')
  const plan = await prepare(fixture); await approve(fixture, plan); success(await execute(fixture, plan), 'FAULT_EXECUTE_FAILED')
  const result = await terminal(fixture, plan, 240_000)
  assert.equal(result.state, 'recovery_required'); assert.equal(result.target_start_attempted, true); assert.equal(result.backup_created, true)
  demand(fixture.committedAfterStart && fixture.stoppedTargetId === result.target_container_id, 'FAULT_BOUNDARY_NOT_REACHED')
  assert.deepEqual(await fixtureDatabase(fixture, 'snapshot'), fixture.committedAfterStart)
  await assertRetainedOld(fixture, false, 'no')
  assert.equal((await ownedContainer(result.target_container_id)).Image, artifact.image_id)
  const countBefore = fixture.committedAfterStart.user.request_count; await assertReplay(fixture, plan)
  assert.equal((await fixtureDatabase(fixture, 'snapshot')).user.request_count, countBefore)
  const instances = success(await transport(fixture, '/v1/instances'), 'FAULT_INSTANCE_QUERY_FAILED'); assert.equal(instances[0].frozen, true)
  mark('real_start_sqlite_commit_sigstop_fault_injected'); mark('health_failure_preserves_committed_17_units'); mark('recovery_required_never_restarts_old_or_restores_database'); mark('failure_replay_has_no_second_candidate_or_charge')
  await closeFactory(fixture)
}

async function cleanup() {
  // Every mutation below checks the run-specific ownership label. This never
  // scans or changes a production process, image, unlabelled container or data.
  for (const child of [...children]) { child.kill('SIGKILL'); await childExit(child, 5000).catch(() => {}) }
  for (const service of [...factoryServices]) await service.close().catch(() => {})
  for (const server of [...servers]) { server.closeAllConnections(); await new Promise(resolve => server.close(() => resolve())) }
  for (const guard of [...guards]) await guard.release()
  if (rootOwned) {
    for (const id of await ownedIds()) {
      await ownedContainer(id)
      await dockerCommand(['kill', '--signal', 'CONT', id]).catch(() => {})
      await dockerCommand(['rm', '--force', '--volumes', id])
    }
    const owned = JSON.parse(await readFile(path.join(root, '.operations-owned.json')))
    demand(owned.run_id === runId && (await lstat(root)).uid === 0, 'CLEANUP_DIRECTORY_OWNERSHIP_MISMATCH')
    await rm(root, { recursive: true })
  }
}
async function main() {
  demand(process.env.GITHUB_ACTIONS === 'true' && process.env.GITHUB_REPOSITORY === 'ForceMind/MyAPI' && process.env.GITHUB_REF === 'refs/heads/dev' &&
    /^[1-9][0-9]{0,19}$/.test(runId || '') && /^[a-f0-9]{40}$/.test(process.env.GITHUB_SHA || '') && process.platform === 'linux' && process.arch === 'x64' && process.getuid() === 0 && Number(process.versions.node.split('.')[0]) >= 22, 'ACTIONS_ROOT_LINUX_GATE')
  demand(process.argv.length === 4 && process.argv[2] === '--artifact', 'ARGUMENTS_INVALID')
  const runnerTemp = await realpath(process.env.RUNNER_TEMP)
  demand(path.isAbsolute(runnerTemp), 'RUNNER_TEMP_INVALID')
  const artifactFile = await realpath(process.argv[3]); demand(artifactFile.startsWith(runnerTemp + path.sep), 'ARTIFACT_INPUT_LOCATION_INVALID')
  const version = (await readFile(path.resolve(moduleDirectory, '../../VERSION'), 'utf8')).trim()
  demand(/^0\.2\.0-beta\.[1-9][0-9]{0,8}$/.test(version), 'VERSION_INPUT_INVALID')
  artifact = artifactInput({ ...JSON.parse(await readFile(artifactFile, 'utf8')), version })
  reportFile = path.join(runnerTemp, 'myapi-operations-report.json')
  report = { source_sha: artifact.source_sha, test_sha: process.env.GITHUB_SHA, target_image_id: artifact.image_id, baseline_sha: baselineSha, checks: {}, browser: 'not_run', production_touched: false, model_requests: 0, passed: false }
  root = path.join(runnerTemp, `myapi-operations-${runId}`)
  await mkdir(root, { mode: 0o700 }); rootOwned = true; await protectedWrite(path.join(root, '.operations-owned.json'), JSON.stringify({ run_id: runId }))
  const token = process.env.GITHUB_TOKEN
  demand(typeof token === 'string' && token.length >= 16 && !/[\s\0]/.test(token), 'GITHUB_TOKEN_UNAVAILABLE')
  tokenFile = path.join(root, 'github.token'); await protectedWrite(tokenFile, token); delete process.env.GITHUB_TOKEN
  fixturePassword = 'Ops!' + randomBytes(24).toString('hex')
  certificateFile = path.join(root, 'tls.crt'); privateKeyFile = path.join(root, 'tls.key')
  await command('/usr/bin/openssl', ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-keyout', privateKeyFile, '-out', certificateFile, '-days', '1', '-subj', '/CN=localhost', '-addext', 'subjectAltName=DNS:localhost,IP:127.0.0.1'])
  await chmod(certificateFile, 0o600); await chmod(privateKeyFile, 0o600)
  const baseline = JSON.parse(await dockerCommand(['image', 'inspect', baselineImage]))[0]
  demand(baseline.Os === 'linux' && baseline.Architecture === 'amd64' && baseline.Config?.Labels?.['io.myapi.edition'] === 'full', 'BASELINE_IMAGE_INVALID')
  await positiveCase(); await identityCase(); await unsupportedCases(); await crashCase(); await faultCase()
  demand(Object.values(report.checks).every(value => value === true), 'QUALIFICATION_CHECK_FAILED')
  report.passed = true
}

try { await main() }
catch (error) {
  if (report) { report.passed = false; report.failure = { phase, code: safeFailure(error) } }
  process.stderr.write('OPERATIONS_QUALIFICATION_FAILED_' + safeFailure(error) + '\n'); process.exitCode = 1
} finally {
  try { await cleanup() } catch { if (report) { report.passed = false; report.failure = { phase: 'cleanup', code: 'OWNED_CLEANUP_FAILED' } } process.exitCode = 1 }
  if (report && reportFile) { await writeFile(reportFile, JSON.stringify(report, null, 2) + '\n', { mode: 0o644 }); await chmod(reportFile, 0o644) }
}
