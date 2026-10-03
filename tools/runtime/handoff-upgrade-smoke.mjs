#!/usr/bin/env node

// Fixed, synthetic R1 source-snapshot rehearsal. Not a production upgrade tool.
import { createHash, randomBytes } from 'node:crypto'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { probeRelayFixture } from './docker-smoke.mjs'
import { handoffSHA, withRestoredSQLite } from './sqlite-restore-smoke.mjs'

const sourceURL = 'http://127.0.0.1:18080'
const cloneURL = 'http://127.0.0.1:18081'

async function api(fetchImpl, origin, pathname, options = {}) {
  if (![sourceURL, cloneURL].includes(origin)) throw new Error('HANDOFF_SCOPE_REJECTED')
  const response = await fetchImpl(origin + pathname, { ...options, redirect: 'error', signal: AbortSignal.timeout(5000) })
  const body = await response.json().catch(() => null)
  if (response.status !== 200 || body?.success !== true) throw new Error('HANDOFF_API_FAILED')
  return body.data
}

export async function verifyHandoffSnapshot({ username, password, userName, userPassword, restore, fetchImpl = globalThis.fetch } = {}) {
  if (!username || !password || !userName || !userPassword || typeof restore !== 'function') throw new Error('HANDOFF_SCOPE_REJECTED')
  const read = async (origin, current) => {
    const setup = await api(fetchImpl, origin, '/api/setup')
    const status = await api(fetchImpl, origin, '/api/status')
    if (setup?.status !== true || status?.user_funding_mode !== 'disabled') throw new Error('HANDOFF_STATE_MISMATCH')
    const login = async (name, secret) => {
      const data = await api(fetchImpl, origin, '/api/user/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username: name, password: secret }) })
      if (typeof data?.access_token !== 'string' || !data.access_token) throw new Error('HANDOFF_API_FAILED')
      return { Authorization: `Bearer ${data.access_token}` }
    }
    const rootHeaders = await login(username, password)
    const userHeaders = await login(userName, userPassword)
    const root = await api(fetchImpl, origin, '/api/user/self', { headers: rootHeaders })
    const user = await api(fetchImpl, origin, '/api/user/self', { headers: userHeaders })
    // Migration must not silently grant the new zero-wallet policy to old users.
    if (current && [root, user].some((item) => item.self_use_no_balance !== false || item.usage_policy_revision !== 0)) throw new Error('HANDOFF_POLICY_MISMATCH')
    if (root?.role !== 100 || root?.quota !== 100000000 || root?.used_quota !== 0 || root?.request_count !== 0 ||
        user?.role !== 1 || user?.quota !== 999985 || user?.used_quota !== 15 || user?.request_count !== 1) throw new Error('HANDOFF_STATE_MISMATCH')
    const listed = await api(fetchImpl, origin, '/api/token/search?keyword=smoke-restricted-key&p=1&page_size=10', { headers: userHeaders })
    const keys = listed?.items?.filter((item) => item.name === 'smoke-restricted-key')
    if (keys?.length !== 1 || !Number.isInteger(keys[0].id)) throw new Error('HANDOFF_STATE_MISMATCH')
    const key = await api(fetchImpl, origin, `/api/token/${keys[0].id}`, { headers: userHeaders })
    const secret = await api(fetchImpl, origin, `/api/token/${keys[0].id}/key`, { method: 'POST', headers: userHeaders })
    if (key?.remain_quota !== 999985 || key?.used_quota !== 15 || key?.unlimited_quota !== false ||
        key?.model_limits_enabled !== true || key?.model_limits !== 'smoke-model' || typeof secret?.key !== 'string' || !secret.key) throw new Error('HANDOFF_STATE_MISMATCH')
    const channels = await api(fetchImpl, origin, '/api/channel/search?keyword=smoke-openai-channel&p=1&page_size=10', { headers: rootHeaders })
    const matches = channels?.items?.filter((item) => item.name === 'smoke-openai-channel')
    if (matches?.length !== 1) throw new Error('HANDOFF_STATE_MISMATCH')
    const channel = await api(fetchImpl, origin, `/api/channel/${matches[0].id}`, { headers: rootHeaders })
    const logs = await api(fetchImpl, origin, '/api/log/?type=2&p=1&page_size=100', { headers: rootHeaders })
    const row = logs?.items?.[0]
    if (channel?.used_quota !== 15 || channel?.base_url !== 'http://127.0.0.1:19090' || channel?.models !== 'smoke-model' ||
        logs?.total !== 1 || logs?.items?.length !== 1 || row?.quota !== 15 || row?.user_id !== user.id ||
        row?.token_id !== key.id || row?.channel !== channel.id || !row?.request_id) throw new Error('HANDOFF_STATE_MISMATCH')
    return { rootId: root.id, userId: user.id, keyId: key.id, keyHash: createHash('sha256').update(secret.key).digest('hex'),
      keyStatus: key.status, keyGroup: key.group, keyExpiry: key.expired_time,
      channelId: channel.id, channelStatus: channel.status, logId: row.id, requestId: row.request_id }
  }
  const before = await read(sourceURL, false)
  const phases = []
  await restore(async (origin, phase) => {
    if (origin !== cloneURL || phase !== ['upgraded', 'restored'][phases.length]) throw new Error('HANDOFF_SCOPE_REJECTED')
    let ready = false
    for (let attempt = 0; attempt < 60; attempt += 1) {
      try {
        const status = await api(fetchImpl, origin, '/api/status')
        if (status?.setup === true) { ready = true; break }
      } catch { /* Read-only readiness check for the disposable clone. */ }
      await new Promise((resolve) => setTimeout(resolve, 1000))
    }
    if (!ready) throw new Error('HANDOFF_READINESS_FAILED')
    if (JSON.stringify(await read(origin, phase === 'upgraded')) !== JSON.stringify(before)) throw new Error('HANDOFF_STATE_MISMATCH')
    phases.push(phase)
  })
  if (phases.length !== 2) throw new Error('HANDOFF_STATE_MISMATCH')
  const response = await fetchImpl('http://127.0.0.1:19090/__smoke__/control', { redirect: 'error', signal: AbortSignal.timeout(5000) })
  const control = await response.json().catch(() => null)
  if (response.status !== 200 || control?.count !== 1 || control?.request_ok !== true) throw new Error('HANDOFF_UPSTREAM_MISMATCH')
  return { name: 'handoff to current migration and original-backup old-image recovery preserve users, finite Key identity, exact usage and log identity without enabling old-user zero-wallet policy', ok: true }
}

async function main() {
  try {
    const env = process.env
    if (env.GITHUB_ACTIONS !== 'true' || env.MYAPI_ISOLATED_SMOKE !== '1' || env.MYAPI_SMOKE_HANDOFF_UPGRADE !== '1' ||
        env.MYAPI_SMOKE_EDITION !== 'full' || env.MYAPI_SMOKE_BASE_URL !== sourceURL || !/^[a-f0-9]{40}$/.test(env.GITHUB_SHA || '')) throw new Error('HANDOFF_SCOPE_REJECTED')
    const setup = await api(globalThis.fetch, sourceURL, '/api/setup')
    if (setup?.status !== false || setup?.root_init !== false || setup?.database_type !== 'sqlite') throw new Error('HANDOFF_REQUIRES_FRESH_SQLITE')
    const username = 'smokeadmin'
    const password = randomBytes(24).toString('hex')
    await api(globalThis.fetch, sourceURL, '/api/setup', { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password, confirmPassword: password, SelfUseModeEnabled: false, DemoSiteEnabled: false }) })
    const report = await probeRelayFixture({ baseUrl: sourceURL, edition: 'full', sha: env.GITHUB_SHA, username, password, isolated: true,
      upstreamBaseUrl: 'http://127.0.0.1:19090', fullContentExpected: true,
      verifyPersistence: (credentials) => verifyHandoffSnapshot({ ...credentials,
        restore: (verify) => withRestoredSQLite({ sha: env.GITHUB_SHA, edition: 'full', handoffUpgrade: true, verify }) }) })
    console.log(JSON.stringify({ command: 'handoff:smoke', source_sha: handoffSHA, target_sha: env.GITHUB_SHA, ...report }, null, 2))
    if (!report.passed) process.exitCode = 1
  } catch (error) {
    // Only fixed local codes, never raw transport or API errors/credentials.
    const code = /^(HANDOFF_|SMOKE_)[A-Z_]+$/.test(error?.message || '') ? error.message : 'HANDOFF_UNEXPECTED_FAILURE'
    console.error(JSON.stringify({ command: 'handoff:smoke', passed: false, code }))
    process.exitCode = 1
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await main()
