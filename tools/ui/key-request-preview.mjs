// Read-only synthetic preview for the Key -> requests -> diagnosis -> body flow.
// No production files, credentials, proxying, writes or provider traffic.
import { createReadStream, existsSync, readFileSync, statSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { createUIFixture } from './browser-fixtures.mjs'
import { createConnectionKeyFixture } from './connection-key-browser-fixtures.mjs'

const repo = resolve(fileURLToPath(new URL('../..', import.meta.url)))
const fixtureChannel = JSON.parse(readFileSync(resolve(repo, 'tools/ui/connection-editor-fixture.json')))
const failure = 'Your input exceeds the context window of this model. Please adjust your input and try again.'
const mime = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.svg': 'image/svg+xml', '.png': 'image/png', '.webp': 'image/webp', '.woff2': 'font/woff2', '.ico': 'image/x-icon' }
const ok = data => ({ status: 200, body: { success: true, data } })

export function createKeyRequestFixture() {
  const now = Math.floor(Date.now() / 1000)
  const base = createUIFixture({ role: 100, personalPolicy: true, language: 'zh' })
  const associations = createConnectionKeyFixture()
  const keys = [21, 22].map(id => ({ id, user_id: 1, name: 'Synthetic same-name Key · 合成同名 Key', key: `masked-synthetic-${id}`, status: 1, remain_quota: 100000, used_quota: 0, unlimited_quota: false, expired_time: -1, created_time: now - 3600, accessed_time: now, group: 'default', model_limits_enabled: false, model_limits: '', allow_ips: '' }))
  const channel = { ...fixtureChannel, id: 1, type: 57, name: 'Synthetic connection · 合成连接', setting: JSON.stringify({ ...JSON.parse(fixtureChannel.setting || '{}'), input_token_limits: { 'synthetic-text-model': 920000 } }) }
  const scenarios = [
    ['SYNTHETIC-CONTEXT-ERROR', 21, 5, 'failed', 'unknown', 'Synthetic name before rename'],
    ['SYNTHETIC-COMPLETED-NO-USAGE', 21, 5, 'completed', 'unknown', keys[0].name],
    ['SYNTHETIC-NOT-RECORDED', 21, 2, 'completed', 'reported', keys[0].name],
    ['SYNTHETIC-OTHER-KEY', 22, 2, 'completed', 'reported', keys[1].name],
    ['SYNTHETIC-LEGACY-NO-ID', 0, 2, 'completed', 'reported', keys[0].name],
    ['SYNTHETIC-PREFLIGHT-REJECTED', 21, 5, 'rejected', 'not_dispatched', keys[0].name],
  ]
  const rows = scenarios.map(([request_id, token_id, type, request_outcome, usage_outcome, token_name], index) => ({
    id: 100 - index, user_id: 1, username: 'Synthetic preview operator', token_id, token_name, type, request_id, upstream_request_id: `SYN-UP-${index + 1}`, created_at: now - index * 30,
    channel: 1, channel_name: 'Synthetic connection · 合成连接', model_name: 'synthetic-text-model', group: 'default', ip: '', is_stream: true, use_time: 2,
    quota: type === 2 ? 15 : 0, prompt_tokens: type === 2 ? 100 : 0, completion_tokens: type === 2 ? 20 : 0, content: type === 5 ? 'usage_pending_review' : 'Synthetic request completed',
    other: JSON.stringify({ request_outcome, usage_outcome, usage_accuracy: usage_outcome, http_status: request_outcome === 'rejected' ? 400 : 200,
      request_input_estimate: { input_tokens: request_outcome === 'rejected' ? 921000 : request_id === 'SYNTHETIC-CONTEXT-ERROR' ? 922000 : 98, method: 'o200k_base_text_estimate', complete: true, ...(request_outcome === 'rejected' ? { limit_tokens: 920000 } : {}) },
      ...(request_outcome === 'rejected' ? { preflight_rejected: true, error_code: 'request_input_limit_exceeded' } : {}),
      ...(request_id === 'SYNTHETIC-CONTEXT-ERROR' ? { upstream_error_code: 'context_length_exceeded' } : {}),
      settlement_status: request_outcome === 'rejected' ? 'not_dispatched' : type === 5 ? 'pending_review' : 'settled', reserved_quota: request_outcome === 'rejected' ? 0 : 1000, actual_quota: request_outcome === 'rejected' ? 0 : type === 5 ? null : 15, ...(type === 2 ? { token_usage_evidence: { schema: 1, input_tokens: 100, output_tokens: 20, cache_read_tokens: 0 } } : {}) }),
  }))
  const bodyFor = row => {
    if (!row || row.request_id === 'SYNTHETIC-NOT-RECORDED' || !row.token_id) return null
    const preflight = row.request_id === 'SYNTHETIC-PREFLIGHT-REJECTED'
    const response = preflight
      ? JSON.stringify({ error: { code: 'request_input_limit_exceeded', message: 'Input estimate 921000 exceeds configured limit 920000; no upstream request was sent.' } })
      : row.request_id === 'SYNTHETIC-CONTEXT-ERROR'
      ? `data: ${JSON.stringify({ error: { message: failure, type: 'invalid_request_error', code: 'context_length_exceeded' } })}\n\ndata: [DONE]\n\n`
      : `data: ${JSON.stringify({ choices: [{ delta: { content: 'Synthetic saved answer · 已保存的合成回答' } }] })}\n\ndata: [DONE]\n\n`
    const contentType = preflight ? 'application/json' : 'text/event-stream'
    return { request_id: row.request_id, timestamp: new Date(row.created_at * 1000).toISOString(), user_id: 1, token_id: row.token_id, token_name: row.token_name, model: row.model_name, status: preflight ? 400 : 200, method: 'POST', path: '/v1/chat/completions', duration_ms: preflight ? 3 : 2000, chunk_count: preflight ? 1 : 2, request_bytes: 96, response_bytes: Buffer.byteLength(response), response_body_total_bytes: Buffer.byteLength(response), response_body_truncated: false, request_body: JSON.stringify({ model: 'synthetic-text-model', stream: true, messages: [{ role: 'user', content: 'Synthetic preview request. No real provider is contacted.' }] }), response_body: response, request_content_type: 'application/json', response_content_type: contentType, request_headers: { authorization: ['[REDACTED]'] }, response_headers: { 'content-type': [contentType] }, query: {} }
  }
  const page = (items, url) => {
    const number = Math.max(1, Number(url.searchParams.get('p') || 1)), size = Math.max(1, Math.min(100, Number(url.searchParams.get('page_size') || url.searchParams.get('size') || 20)))
    return { items: items.slice((number - 1) * size, number * size), total: items.length, page: number, page_size: size }
  }
  const selectedRows = url => rows.filter(row => {
    for (const [key, field] of [['token_id', 'token_id'], ['token_name', 'token_name'], ['channel', 'channel'], ['request_id', 'request_id'], ['upstream_request_id', 'upstream_request_id'], ['model_name', 'model_name'], ['group', 'group'], ['username', 'username']]) {
      if (url.searchParams.has(key) && String(row[field]) !== url.searchParams.get(key)) return false
    }
    if (url.searchParams.get('start_timestamp') && row.created_at < Number(url.searchParams.get('start_timestamp'))) return false
    if (url.searchParams.get('end_timestamp') && row.created_at > Number(url.searchParams.get('end_timestamp'))) return false
    const kind = Number(url.searchParams.get('type') || 0)
    return !kind || row.type === kind
  })
  const requests = []
  return {
    requests,
    response(url, method = 'GET') {
      const path = url.pathname.replace(/\/$/, '')
      const ids = url.searchParams.getAll('token_id')
      if (ids.length && (ids.length !== 1 || !/^[1-9]\d*$/.test(ids[0]) || Number(ids[0]) > 2147483647)) return { status: 400, body: { success: false, message: 'Invalid synthetic Key ID' } }
      requests.push({ method, path, token_id: url.searchParams.get('token_id'), request_id: url.searchParams.get('request_id') })
      if (method !== 'GET' && !(method === 'POST' && path === '/api/user/auth/refresh')) return { status: 405, body: { success: false, message: 'Synthetic preview is read-only. No changes or generation are sent.' } }
      if (path === '/api/user/auth/refresh') {
        const result = base.response(url, method)
        result.body.data.access_expires_at = now + 86400
        result.body.data.session.expires_at = now + 86400
        return result
      }
      if (path === '/api/token' || path === '/api/token/search') return ok(page(keys, url))
      if (/^\/api\/token\/\d+$/.test(path)) {
        const key = keys.find(key => key.id === Number(path.split('/').at(-1)))
        return key ? ok(key) : { status: 404, body: { success: false, message: 'Synthetic Key not found' } }
      }
      if (path === '/api/channel' || path === '/api/channel/search') return ok(page([channel], url))
      if (path === '/api/channel/1') return ok(channel)
      if (path === '/api/channel/quota/changes') return ok({ items: [] })
      if (path === '/api/subscription-share/channels/1/pools') return ok({ channel_id: 1, has_more: false, items: [{ pool_id: 7, channel_id: 1, window_source: 'codex_wham_usage_primary', window_type: 'five_hour', enabled: true }] })
      const related = associations.response(url, method)
      if (related) return related.violation ? { status: 400, body: { success: false, message: related.violation } } : related
      if (path === '/api/log' || path === '/api/log/self') return ok(page(selectedRows(url), url))
      if (path === '/api/log/stat' || path === '/api/log/self/stat') {
        const selected = selectedRows(url), known = selected.filter(row => row.type === 2).length, unknown = selected.length - known
        return ok({ quota: known * 15, rpm: 0, tpm: 0, token_usage: { records: selected.length, known_records: known, unknown_records: unknown, input_tokens: unknown ? null : String(known * 100), output_tokens: unknown ? null : String(known * 20), total_tokens: unknown ? null : String(known * 120), known_input_tokens: String(known * 100), known_output_tokens: String(known * 20), known_total_tokens: String(known * 120), cache_known_records: known, cache_unknown_records: unknown, cache_read_tokens: '0', cache_input_tokens: String(known * 100), cache_hit_rate: known ? 0 : null } })
      }
      if (path.startsWith('/api/usage-review/')) {
        const row = rows.find(row => row.request_id === decodeURIComponent(path.split('/').at(-1)))
        return row ? ok({ request_id: row.request_id, user_id: 1, token_id: row.token_id, state: row.type === 5 ? 'usage_unknown' : 'settled', reserved_quota: 1000, actual_quota: row.type === 5 ? null : 15, can_reconcile_usage: false, can_recover_text_dispatch: false, text_dispatch_pending: false, settlement_status: 'manual', recovery_block_reason: 'automatic_settlement_manual', review_metadata: JSON.stringify({ synthetic: true, usage_outcome: row.type === 5 ? 'unknown' : 'reported' }) }) : { status: 404, body: { success: false } }
      }
      if (path === '/api/full-content-logs/files') return ok({ count: 1, total_size: 4096, files: [] })
      if (path === '/api/full-content-logs') {
        const saved = rows.map(bodyFor).filter(Boolean).filter(row => !url.searchParams.get('request_id') || row.request_id === url.searchParams.get('request_id'))
        return ok({ ...page(saved, url), enabled: true, files: { count: 1, total_size: 4096, files: [] }, facets: { models: ['synthetic-text-model'], tokens: keys.map(key => ({ id: key.id, name: key.name })) } })
      }
      if (path.startsWith('/api/full-content-logs/')) {
        const detail = bodyFor(rows.find(row => row.request_id === decodeURIComponent(path.split('/').at(-1))))
        return detail ? ok(detail) : { status: 404, body: { success: false, message: 'No saved synthetic body' } }
      }
      const result = base.response(url, method)
      if (path === '/api/user/self' && result.body?.data) result.body.data.display_name = '合成预览 · Synthetic preview'
      if (path === '/api/status' && result.body?.data) { result.body.data.version = readFileSync(resolve(repo, 'VERSION'), 'utf8').trim(); result.body.data.server_address = ''; }
      return result.violation ? { status: 503, body: { success: false, message: 'This optional module is outside the synthetic preview.' } } : result
    },
  }
}

export function createKeyRequestPreview({ dist = resolve(repo, 'web/dist') } = {}) {
  const fixture = createKeyRequestFixture()
  const root = resolve(dist)
  const server = createServer((request, response) => {
    const url = new URL(request.url, 'http://127.0.0.1')
    response.setHeader('Cache-Control', 'no-store')
    response.setHeader('X-MyAPI-Preview', 'synthetic-readonly')
    response.setHeader('Content-Security-Policy', "default-src 'self'; connect-src 'self'; img-src 'self' data: blob:; font-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; frame-src 'none'; object-src 'none'")
    if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/pg/')) {
      try {
        const result = fixture.response(url, request.method)
        response.writeHead(result.status, { 'Content-Type': 'application/json; charset=utf-8' }); response.end(JSON.stringify(result.body))
      } catch {
        response.writeHead(400, { 'Content-Type': 'application/json; charset=utf-8' }); response.end(JSON.stringify({ success: false, message: 'Invalid synthetic preview request' }))
      }
      return
    }
    if (request.method !== 'GET' && request.method !== 'HEAD') { response.writeHead(405); response.end(); return }
    if (url.pathname === '/__preview') {
      response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
      response.end('<!doctype html><html lang="zh"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>MyAPI 控制台合成预览</title><body style="font:16px system-ui;max-width:760px;margin:60px auto;padding:20px;line-height:1.8"><h1>MyAPI · 控制台预览</h1><p>合成数据、只读演示。不会访问生产数据或供应商，不会重新发送请求。</p><p><a href="/dashboard/overview">概览</a> · <a href="/upstream-accounts">上游账号</a> · <a href="/models/metadata">模型</a> · <a href="/dashboard/models">模型调用分析</a> · <a href="/profile">个人资料</a> · <a href="/system-info">系统信息</a> · <a href="/system-settings/site/system-info">站点设置</a></p><ol><li><a href="/keys">选择同名 Key 中的 #21 或 #22，查看各自请求</a></li><li><a href="/usage-logs/common?tokenId=21&pageSize=20">查看 Key #21：流内错误、成功但用量未知、未采集正文</a></li><li><a href="/connections?connection=1">连接 → 已保存分额 → Key → 请求</a></li><li><a href="/full-content-logs">查看正文采集状态与已保存 SSE</a></li></ol><p>请求列表和统计均按稳定 Key ID 筛选；未记录正文有说明，查看不会生成新请求。</p></body></html>'); return
    }
    let pathname
    try { pathname = decodeURIComponent(url.pathname) } catch { response.writeHead(400); response.end(); return }
    let file = resolve(root, '.' + pathname)
    if (file !== root && !file.startsWith(root + sep)) { response.writeHead(403); response.end(); return }
    if (!existsSync(file) || !statSync(file).isFile()) {
      if (extname(pathname)) { response.writeHead(404); response.end(); return }
      file = resolve(root, 'index.html')
    }
    if (!existsSync(file)) {
      response.writeHead(503, { 'Content-Type': 'text/plain; charset=utf-8' })
      response.end('The synthetic preview build is not ready. Build web/ first.'); return
    }
    if (file === resolve(root, 'index.html') && request.method !== 'HEAD') {
      response.writeHead(200, { 'Content-Type': mime['.html'] })
      response.end(readFileSync(file, 'utf8').replace('</head>', `<script>if(!localStorage.getItem('i18nextLng'))localStorage.setItem('i18nextLng','zh')</script></head>`))
      return
    }
    response.writeHead(200, { 'Content-Type': mime[extname(file)] || 'application/octet-stream' })
    if (request.method === 'HEAD') { response.end(); return }
    createReadStream(file).pipe(response)
  })
  return { server, fixture }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const port = Number(process.env.MYAPI_PREVIEW_PORT || 4182)
  if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error('Invalid preview port')
  const { server } = createKeyRequestPreview({ dist: process.env.MYAPI_UI_DIST })
  server.listen(port, '127.0.0.1', () => console.log(`Synthetic read-only preview: http://127.0.0.1:${port}/__preview`))
}
