// CI-only, same-image recovery of synthetic data. Never accepts a deployment path.
import { spawnSync } from 'node:child_process'
import { createHash, randomBytes } from 'node:crypto'
import { chmodSync, cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync } from 'node:fs'
import path from 'node:path'

function docker(args, env = process.env) {
  const result = spawnSync('docker', args, { env, encoding: 'utf8', timeout: 60_000, maxBuffer: 1024 * 1024 })
  if (result.error || result.status !== 0) throw new Error('SMOKE_RESTORE_DOCKER_FAILED')
  return result.stdout.trim()
}

function databaseDigest(directory) {
  const main = path.join(directory, 'my-api.db')
  if (!existsSync(main) || readFileSync(main).subarray(0, 16).toString() !== 'SQLite format 3\u0000') throw new Error('SMOKE_RESTORE_DATABASE_MISSING')
  return ['my-api.db', 'my-api.db-wal'].map((name) => {
    const file = path.join(directory, name)
    return existsSync(file) ? createHash('sha256').update(readFileSync(file)).digest('hex') : null
  })
}

// Pause all writers while copying the whole /data directory, including WAL.
// This is a crash-consistent snapshot, not a graceful shutdown or an upgrade.
export async function withRestoredSQLite({ sha, edition, env = process.env, runDocker = docker, verify } = {}) {
  if (env.GITHUB_ACTIONS !== 'true' || env.MYAPI_ISOLATED_SMOKE !== '1' || env.MYAPI_SMOKE_RESTORE !== '1' ||
      !/^[a-f0-9]{40}$/.test(sha || '') || !['full', 'lan'].includes(edition) ||
      !path.isAbsolute(env.RUNNER_TEMP || '') || typeof verify !== 'function') throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
  const source = `myapi-smoke-${edition}-${sha}`
  const image = `myapi:smoke-${edition}-${sha}`
  const target = `myapi-smoke-restore-${edition}-${sha}`
  if (env.SMOKE_CONTAINER !== source || env.SMOKE_IMAGE !== image) throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
  const sourceData = path.join(env.RUNNER_TEMP, `myapi-smoke-data-${edition}-${sha}`)
  if (env.SMOKE_DATA_DIR !== sourceData) throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
  const owned = (name) => runDocker(['inspect', '--format', '{{index .Config.Labels "io.myapi.smoke.sha"}}', name]) === sha
  if (!owned(source) || runDocker(['inspect', '--format', '{{.State.Running}}', source]) !== 'true' ||
      runDocker(['inspect', '--format', '{{.Config.Image}}', source]) !== image) throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
  const mount = runDocker(['inspect', '--format', '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Type}}:{{.Source}}{{end}}{{end}}', source])
  if (mount !== `bind:${sourceData}`) throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
  if (runDocker(['ps', '-a', '--filter', `name=^${target}$`, '--format', '{{.Names}}']) !== '') throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
  const workspace = mkdtempSync(path.join(env.RUNNER_TEMP, 'myapi-sqlite-restore-'))
  chmodSync(workspace, 0o700)
  const backup = path.join(workspace, 'backup')
  const restored = path.join(workspace, 'restored')
  mkdirSync(backup, { mode: 0o700 })
  let paused = false
  let started = false
  try {
    // Set the flag before the call: an uncertain pause outcome still needs cleanup.
    paused = true
    runDocker(['pause', source])
    runDocker(['cp', `${source}:/data/.`, backup])
    runDocker(['unpause', source])
    paused = false
    const digest = databaseDigest(backup)
    cpSync(backup, restored, { recursive: true })
    chmodSync(restored, 0o700)
    if (JSON.stringify(databaseDigest(restored)) !== JSON.stringify(digest)) throw new Error('SMOKE_RESTORE_COPY_MISMATCH')
    const restoreEnv = { ...env, SESSION_SECRET: randomBytes(32).toString('hex') }
    started = true
    runDocker(['run', '--detach', '--name', target, '--label', `io.myapi.smoke.sha=${sha}`,
      '--user', `${process.getuid()}:${process.getgid()}`,
      '--publish', '127.0.0.1:18081:3000', '--cpus', '1', '--memory', '768m', '--pids-limit', '256',
      '--mount', `type=bind,src=${restored},dst=/data`, '--env', `MYAPI_EDITION=${edition}`,
      '--env', 'MEMORY_CACHE_ENABLED=false', '--env', 'SESSION_COOKIE_SECURE=false', '--env', 'TRUSTED_PROXIES=none',
      '--env', 'SQL_MAX_OPEN_CONNS=1', '--env', 'SQL_MAX_IDLE_CONNS=1', '--env', 'GOMEMLIMIT=512MiB',
      '--env', 'SESSION_SECRET', image], restoreEnv)
    const result = await verify('http://127.0.0.1:18081')
    if (JSON.stringify(databaseDigest(backup)) !== JSON.stringify(digest)) throw new Error('SMOKE_RESTORE_COPY_MISMATCH')
    return result
  } finally {
    // Only exact, labeled resources from this run are eligible for cleanup.
    // If removal is uncertain, retain the directory rather than unlinking a live mount.
    if (paused && owned(source)) runDocker(['unpause', source])
    if (started) {
      if (!owned(target)) throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
      runDocker(['rm', '--force', target])
    }
    rmSync(workspace, { recursive: true })
  }
}

export async function verifySQLiteRestore({ baseUrl, username, password, fetchImpl = globalThis.fetch, restore } = {}) {
  if (baseUrl !== 'http://127.0.0.1:18080' || !username || !password || typeof restore !== 'function') throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
  const getState = async (origin) => {
    if (!['http://127.0.0.1:18080', 'http://127.0.0.1:18081'].includes(origin)) throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
    const json = async (pathname, options = {}) => {
      const response = await fetchImpl(origin + pathname, { ...options, redirect: 'error', signal: AbortSignal.timeout(5000) })
      const body = await response.json().catch(() => null)
      if (response.status !== 200 || body?.success !== true) throw new Error('SMOKE_RESTORE_API_FAILED')
      return body.data
    }
    const setup = await json('/api/setup')
    // Initialized setup intentionally omits database_type; the source fixture
    // and copied SQLite file header establish the database type independently.
    if (setup?.status !== true) throw new Error('SMOKE_RESTORE_DATABASE_MISSING')
    const login = await json('/api/user/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password }) })
    if (typeof login?.access_token !== 'string' || !login.access_token) throw new Error('SMOKE_RESTORE_API_FAILED')
    const headers = { Authorization: `Bearer ${login.access_token}` }
    const self = await json('/api/user/self', { headers })
    const keyPage = await json('/api/token/search?keyword=smoke-self-use-key&p=1&page_size=10', { headers })
    const keys = keyPage?.items?.filter((item) => item.name === 'smoke-self-use-key')
    if (keys?.length !== 1 || !Number.isInteger(keys[0].id)) throw new Error('SMOKE_RESTORE_STATE_MISMATCH')
    const key = await json(`/api/token/${keys[0].id}`, { headers })
    const secret = await json(`/api/token/${keys[0].id}/key`, { method: 'POST', headers })
    if (typeof secret?.key !== 'string' || !secret.key) throw new Error('SMOKE_RESTORE_STATE_MISMATCH')
    const users = await json('/api/user/search?keyword=smokeuser&p=1&page_size=10', { headers })
    const ordinary = users?.items?.filter((item) => item.username === 'smokeuser')
    const channels = await json('/api/channel/search?keyword=smoke-openai-channel&p=1&page_size=10', { headers })
    const selectedChannels = channels?.items?.filter((item) => item.name === 'smoke-openai-channel')
    if (ordinary?.length !== 1 || selectedChannels?.length !== 1) throw new Error('SMOKE_RESTORE_STATE_MISMATCH')
    const channel = await json(`/api/channel/${selectedChannels[0].id}`, { headers })
    const logs = await json('/api/log/?type=2&p=1&page_size=100', { headers })
    if (self?.quota !== 0 || self?.self_use_no_balance !== true || self?.used_quota !== 15 || self?.request_count !== 1 ||
        key?.remain_quota !== 985 || key?.used_quota !== 15 || ordinary[0].quota !== 999985 ||
        ordinary[0].used_quota !== 15 || ordinary[0].request_count !== 1 || channel?.used_quota !== 30 ||
        logs?.total !== 2 || !Array.isArray(logs.items) || logs.items.length !== 2) throw new Error('SMOKE_RESTORE_STATE_MISMATCH')
    return { userId: self.id, policy: self.self_use_no_balance, policyRevision: self.usage_policy_revision,
      keyId: key.id, keyHash: createHash('sha256').update(secret.key).digest('hex'),
      ordinaryUserId: ordinary[0].id, channelId: channel.id,
      logs: logs.items.map((row) => ({ id: row.id, request_id: row.request_id, user_id: row.user_id, token_id: row.token_id, quota: row.quota })).sort((a, b) => a.id - b.id) }
  }
  const before = await getState(baseUrl)
  await restore(async (origin) => {
    if (origin !== 'http://127.0.0.1:18081') throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
    // Readiness retries cannot create accounts or replay a billable request.
    let ready = false
    for (let attempt = 0; attempt < 60; attempt += 1) {
      try {
        const response = await fetchImpl(origin + '/api/status', { redirect: 'error', signal: AbortSignal.timeout(2000) })
        const body = await response.json()
        if (response.status === 200 && body?.success === true && body.data?.setup === true) { ready = true; break }
      } catch { /* The new disposable container may still be starting. */ }
      await new Promise((resolve) => setTimeout(resolve, 1000))
    }
    if (!ready) throw new Error('SMOKE_RESTORE_READINESS_FAILED')
    const after = await getState(origin)
    if (JSON.stringify(before) !== JSON.stringify(after)) throw new Error('SMOKE_RESTORE_STATE_MISMATCH')
  })
  return { name: 'paused SQLite and WAL snapshot restores login, Key identity, exact usage and logs in a separate same-image container', ok: true }
}
