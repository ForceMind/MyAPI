// CI-only recovery of synthetic data. Never accepts a deployment path.
export const handoffSHA = '71277bf6055ce68b8cd1d11f1d10f1907d7696fd'
import { spawnSync } from 'node:child_process'
import { createHash, randomBytes } from 'node:crypto'
import { chmodSync, cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync } from 'node:fs'
import path from 'node:path'

function docker(args, env = process.env) {
  const result = spawnSync('docker', args, { env, encoding: 'utf8', timeout: 60_000, maxBuffer: 1024 * 1024 })
  if (result.error || result.status !== 0) throw new Error('SMOKE_RESTORE_DOCKER_FAILED')
  return result.stdout.trim()
}

function dockerStderr(args, env = process.env) {
  const result = spawnSync('docker', args, { env, encoding: 'utf8', timeout: 60_000, maxBuffer: 1024 * 1024 })
  if (result.error || result.status !== 0) throw new Error('SMOKE_RESTORE_DOCKER_FAILED')
  return result.stderr
}

function databaseDigest(directory) {
  const main = path.join(directory, 'my-api.db')
  if (!existsSync(main) || readFileSync(main).subarray(0, 16).toString() !== 'SQLite format 3\u0000') throw new Error('SMOKE_RESTORE_DATABASE_MISSING')
  return ['my-api.db', 'my-api.db-wal'].map((name) => {
    const file = path.join(directory, name)
    return existsSync(file) ? createHash('sha256').update(readFileSync(file)).digest('hex') : null
  })
}

function settlementIdentity(value) {
  let other
  try { other = JSON.parse(value || '{}') } catch { throw new Error('SMOKE_RESTORE_SETTLEMENT_PROOF_MISMATCH') }
  const settlement = other?.admin_info?.billing_settlement
  if (settlement?.state !== 'applied' || !Number.isInteger(settlement.fact_id) || settlement.fact_id <= 0 ||
      Object.hasOwn(settlement, 'intent_id') && (!Number.isInteger(settlement.intent_id) || settlement.intent_id <= 0)) {
    throw new Error('SMOKE_RESTORE_SETTLEMENT_PROOF_MISMATCH')
  }
  return { factId: settlement.fact_id, intentId: Object.hasOwn(settlement, 'intent_id') ? settlement.intent_id : null }
}

// Pause all writers while copying the whole /data directory, including WAL.
// This is a crash-consistent snapshot, not a graceful shutdown or an upgrade.
export async function withRestoredSQLite({
  sha, edition, env = process.env, runDocker = docker, readDockerStderr = dockerStderr, verify,
  handoffUpgrade = false, expectUpgradeRejection = false, wait = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds)),
} = {}) {
  if (env.GITHUB_ACTIONS !== 'true' || env.MYAPI_ISOLATED_SMOKE !== '1' || env.MYAPI_SMOKE_RESTORE !== '1' ||
      !/^[a-f0-9]{40}$/.test(sha || '') || !['full', 'lan'].includes(edition) ||
      !path.isAbsolute(env.RUNNER_TEMP || '') || typeof verify !== 'function' || typeof wait !== 'function' ||
      typeof readDockerStderr !== 'function') throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
  if (handoffUpgrade && (env.MYAPI_SMOKE_HANDOFF_UPGRADE !== '1' || edition !== 'full')) throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
  if (expectUpgradeRejection && !handoffUpgrade) throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
  const source = `myapi-smoke-${edition}-${sha}`
  const image = `myapi:smoke-${edition}-${sha}`
  const sourceImage = handoffUpgrade ? `myapi:handoff-${handoffSHA}` : image
  const target = `myapi-smoke-restore-${edition}-${sha}`
  if (env.SMOKE_CONTAINER !== source || env.SMOKE_IMAGE !== image) throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
  const sourceData = path.join(env.RUNNER_TEMP, `myapi-smoke-data-${edition}-${sha}`)
  if (env.SMOKE_DATA_DIR !== sourceData) throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
  const owned = (name) => runDocker(['inspect', '--format', '{{index .Config.Labels "io.myapi.smoke.sha"}}', name]) === sha
  if (!owned(source) || runDocker(['inspect', '--format', '{{.State.Running}}', source]) !== 'true' ||
      runDocker(['inspect', '--format', '{{.Config.Image}}', source]) !== sourceImage) throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
  const mount = runDocker(['inspect', '--format', '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Type}}:{{.Source}}{{end}}{{end}}', source])
  if (mount !== `bind:${sourceData}`) throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
  if (runDocker(['ps', '-a', '--filter', `name=^${target}$`, '--format', '{{.Names}}']) !== '') throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
  const workspace = mkdtempSync(path.join(env.RUNNER_TEMP, 'myapi-sqlite-restore-'))
  chmodSync(workspace, 0o700)
  const backup = path.join(workspace, 'backup')
  mkdirSync(backup, { mode: 0o700 })
  let paused = false
  let started = false
  let removalUncertain = false
  try {
    // Set the flag before the call: an uncertain pause outcome still needs cleanup.
    paused = true
    runDocker(['pause', source])
    runDocker(['cp', `${source}:/data/.`, backup])
    runDocker(['unpause', source])
    paused = false
    const digest = databaseDigest(backup)
    const phases = handoffUpgrade ? [expectUpgradeRejection ? 'rejected' : 'upgraded', 'restored'] : ['restored']
    let result
    for (const phase of phases) {
      const restored = path.join(workspace, phase)
      const restoreImage = handoffUpgrade && phase === 'restored' ? sourceImage : image
      cpSync(backup, restored, { recursive: true })
      chmodSync(restored, 0o700)
      if (JSON.stringify(databaseDigest(restored)) !== JSON.stringify(digest)) throw new Error('SMOKE_RESTORE_COPY_MISMATCH')
      const restoreEnv = { ...env, SESSION_SECRET: randomBytes(32).toString('hex') }
      started = true
      runDocker(['run', '--pull=never', '--detach', '--name', target, '--label', `io.myapi.smoke.sha=${sha}`,
        '--user', `${process.getuid()}:${process.getgid()}`,
        '--publish', '127.0.0.1:18081:3000', '--cpus', '1', '--memory', '768m', '--pids-limit', '256',
        '--mount', `type=bind,src=${restored},dst=/data`, '--env', `MYAPI_EDITION=${edition}`,
        '--env', 'MEMORY_CACHE_ENABLED=false', '--env', 'SESSION_COOKIE_SECURE=false', '--env', 'TRUSTED_PROXIES=none',
        '--env', 'SQL_MAX_OPEN_CONNS=1', '--env', 'SQL_MAX_IDLE_CONNS=1', '--env', 'GOMEMLIMIT=512MiB',
        '--env', 'SESSION_SECRET', restoreImage], restoreEnv)
      if (phase === 'rejected') {
        let state = ''
        for (let attempt = 0; attempt < 60; attempt += 1) {
          state = runDocker(['inspect', '--format', '{{.State.Status}} {{.State.ExitCode}} {{.State.OOMKilled}}', target])
          if (state.startsWith('exited ')) break
          if (!/^(created|running|restarting) /.test(state)) break
          if (attempt < 59) await wait(1000)
        }
        if (/^(created|running|restarting) /.test(state)) throw new Error('SMOKE_RESTORE_UPGRADE_REJECTION_MISSING')
        const [status, exitCode, oomKilled] = state.split(' ')
        if (oomKilled === 'true') throw new Error('SMOKE_RESTORE_UPGRADE_OOM')
        if (status !== 'exited' || exitCode !== '1' || oomKilled !== 'false') throw new Error('SMOKE_RESTORE_UPGRADE_EXIT_MISMATCH')
        // FatalLog writes to the container's stderr. Keep it in memory and
        // compare only the fixed, already-sanitized startup error.
        const stderr = readDockerStderr(['logs', '--tail', '100', target], restoreEnv)
        if (!stderr.includes('failed to initialize database: cannot verify account quota execution gate')) {
          throw new Error('SMOKE_RESTORE_UPGRADE_REASON_MISMATCH')
        }
        if (JSON.stringify(databaseDigest(restored)) !== JSON.stringify(digest)) throw new Error('SMOKE_RESTORE_COPY_MISMATCH')
        await verify('http://127.0.0.1:18081', phase)
      } else {
        result = await verify('http://127.0.0.1:18081', phase)
      }
      if (JSON.stringify(databaseDigest(backup)) !== JSON.stringify(digest)) throw new Error('SMOKE_RESTORE_COPY_MISMATCH')
      if (!owned(target)) throw new Error('SMOKE_RESTORE_SCOPE_REJECTED')
      removalUncertain = true
      runDocker(['rm', '--force', target])
      removalUncertain = false
      started = false
    }
    return result
  } finally {
    // Only exact, labeled resources from this run are eligible for cleanup.
    // If removal is uncertain, retain the directory rather than unlinking a live mount.
    if (paused && owned(source)) runDocker(['unpause', source])
    if (removalUncertain) throw new Error('SMOKE_RESTORE_DOCKER_FAILED')
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
    if (self?.role !== 100 || self?.status !== 1 || self?.quota !== 0 || self?.self_use_no_balance !== true ||
        self?.usage_policy_revision !== 1 || self?.used_quota !== 15 || self?.request_count !== 1 ||
        key?.status !== 1 || key?.remain_quota !== 985 || key?.used_quota !== 15 || key?.unlimited_quota !== false ||
        key?.model_limits_enabled !== true || key?.model_limits !== 'smoke-model' || key?.group !== 'default' || key?.expired_time !== -1 ||
        ordinary[0].role !== 1 || ordinary[0].status !== 1 || ordinary[0].quota !== 999985 ||
        ordinary[0].used_quota !== 15 || ordinary[0].request_count !== 1 ||
        channel?.status !== 1 || channel?.used_quota !== 30 || channel?.base_url !== 'http://127.0.0.1:19090' ||
        channel?.models !== 'smoke-model' || channel?.group !== 'default' ||
        logs?.total !== 2 || !Array.isArray(logs.items) || logs.items.length !== 2) throw new Error('SMOKE_RESTORE_STATE_MISMATCH')
    const ordinaryLog = logs.items.find((row) => row?.user_id === ordinary[0].id)
    const selfUseLog = logs.items.find((row) => row?.user_id === self.id)
    if (!ordinaryLog || !selfUseLog || ordinaryLog === selfUseLog || !Number.isInteger(ordinaryLog.token_id) || ordinaryLog.token_id <= 0 ||
        ordinaryLog.channel !== channel.id || ordinaryLog.quota !== 15 || ordinaryLog.prompt_tokens !== 10 || ordinaryLog.completion_tokens !== 5 ||
        typeof ordinaryLog.request_id !== 'string' || !ordinaryLog.request_id || selfUseLog.token_id !== key.id ||
        selfUseLog.channel !== channel.id || selfUseLog.quota !== 15 || selfUseLog.prompt_tokens !== 10 || selfUseLog.completion_tokens !== 5 ||
        typeof selfUseLog.request_id !== 'string' || !selfUseLog.request_id) throw new Error('SMOKE_RESTORE_STATE_MISMATCH')
    return {
      root: { id: self.id, role: self.role, status: self.status, quota: self.quota, policy: self.self_use_no_balance,
        policyRevision: self.usage_policy_revision, usedQuota: self.used_quota, requestCount: self.request_count },
      key: { id: key.id, hash: createHash('sha256').update(secret.key).digest('hex'), status: key.status,
        remainQuota: key.remain_quota, usedQuota: key.used_quota, unlimited: key.unlimited_quota,
        modelLimitsEnabled: key.model_limits_enabled, modelLimits: key.model_limits, group: key.group, expiry: key.expired_time },
      ordinaryUser: { id: ordinary[0].id, role: ordinary[0].role, status: ordinary[0].status,
        quota: ordinary[0].quota, usedQuota: ordinary[0].used_quota, requestCount: ordinary[0].request_count },
      channel: { id: channel.id, status: channel.status, usedQuota: channel.used_quota,
        baseUrl: channel.base_url, models: channel.models, group: channel.group },
      logs: logs.items.map((row) => ({ id: row.id, requestId: row.request_id, userId: row.user_id,
        tokenId: row.token_id, channelId: row.channel, quota: row.quota, promptTokens: row.prompt_tokens,
        completionTokens: row.completion_tokens, settlement: settlementIdentity(row.other) })).sort((a, b) => a.id - b.id),
    }
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
  return { name: 'paused SQLite and WAL snapshot restores identities, finite limits, status, settlement proofs and exact usage in a separate same-image container', ok: true }
}
