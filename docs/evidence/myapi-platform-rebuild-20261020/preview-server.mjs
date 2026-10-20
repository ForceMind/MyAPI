import { createServer } from 'node:http'
import { readFileSync, existsSync, statSync, createReadStream } from 'node:fs'
import { resolve, extname, sep } from 'node:path'
import { createUIFixture, FIXTURE_TIME } from '/root/myapi/tools/ui/browser-fixtures.mjs'

const root = resolve(process.argv[2])
if (!existsSync(resolve(root, 'index.html'))) throw new Error('Missing verified preview build')
const fixtures = new Map([0, 100].map(role => [role, createUIFixture({ role, language: 'zhCN' })]))
const banner = `<aside style="height:28px;padding:4px 12px;background:#e9edf8;color:#304799;font:12px/20px sans-serif;text-align:center">隔离预览 · 合成数据 · 写操作关闭 <a href="/__preview/guest">首页</a> · <a href="/__preview/operator">控制台</a></aside><style>.personal-console{height:calc(100svh - 28px)!important}</style><script>if(!localStorage.getItem('i18nextLng'))localStorage.setItem('i18nextLng','zhCN')</script>`
const mime = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript', '.css': 'text/css', '.png': 'image/png', '.svg': 'image/svg+xml', '.woff2': 'font/woff2', '.ico': 'image/x-icon', '.json': 'application/json' }
const syntheticLog = id => ({ id, user_id: 1, token_id: 21, channel: 1, channel_name: '合成上游连接', created_at: FIXTURE_TIME / 1000 - id, type: 5, content: '合成响应已完成，供应商未返回 usage', username: 'synthetic-operator', token_name: 'synthetic-key', model_name: 'synthetic-text-model', quota: 0, prompt_tokens: 0, completion_tokens: 0, use_time: 2, is_stream: true, request_id: `REQ-SYNTHETIC-${id}`, group: 'default', ip: '', other: JSON.stringify({ request_outcome: 'completed', usage_outcome: 'missing' }) })
function syntheticWindows() {
  const observed = Math.floor(Date.now() / 1000) - 60
  const primary = { metric_type: 'codex_rate_limit', unit: 'percent', window_type: 'five_hour', window_seconds: 18000, source: 'codex_wham_usage_primary', status: 'success', observed_at: observed }
  const secondary = { ...primary, window_type: 'weekly', window_seconds: 604800, source: 'codex_wham_usage_secondary' }
  return [
    { ...primary, channel_id: 1, series_id: '1a'.padStart(64, '0'), name: '合成订阅账号 · 采样异常', status: 'error', current_available: null },
    { ...secondary, channel_id: 1, series_id: '1b'.padStart(64, '0'), name: '合成订阅账号 · 采样异常', status: 'unsupported', current_available: null },
    { ...primary, channel_id: 2, series_id: '2a'.padStart(64, '0'), name: '合成订阅账号 · 正常与过期样本', current_available: 100 },
    { ...secondary, channel_id: 2, series_id: '2b'.padStart(64, '0'), name: '合成订阅账号 · 正常与过期样本', current_available: 32, observed_at: observed - 600 },
    { ...primary, channel_id: 3, series_id: '3a'.padStart(64, '0'), name: '合成订阅账号 · 证据不完整', status: 'unavailable', current_available: null },
    { ...secondary, channel_id: 3, series_id: '3b'.padStart(64, '0'), name: '合成订阅账号 · 证据不完整', current_available: 45, observed_at: undefined },
  ]
}
const server = createServer((request, response) => {
  const url = new URL(request.url, 'http://127.0.0.1:4181')
  response.setHeader('cache-control', 'no-store')
  response.setHeader('x-myapi-preview', 'synthetic; no-provider; no-business-writes')
  response.setHeader('content-security-policy', "default-src 'self' data: blob:; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-src 'none'")
  if (url.pathname.startsWith('/__preview/')) {
    const role = url.pathname === '/__preview/guest' ? 0 : 100
    response.writeHead(303, { location: role ? '/dashboard/overview' : '/', 'set-cookie': `myapi_preview_role=${role}; Path=/; SameSite=Strict` }).end(); return
  }
  if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/pg/')) {
    const role = /myapi_preview_role=0(?:;|$)/.test(request.headers.cookie || '') ? 0 : 100
    const fixture = fixtures.get(role)
    let result
    if (role === 100 && request.method === 'GET' && url.pathname === '/api/channel/quota/changes') {
      result = { status: 200, body: { success: true, data: { items: syntheticWindows(), source_complete: true, items_complete: true, generated_at: Math.floor(Date.now() / 1000) } } }
    } else if (role === 100 && request.method === 'GET' && ['/api/channel', '/api/channel/search'].includes(url.pathname)) {
      const template = fixture.response(new URL('/api/channel', url), 'GET').body.data.items[0]
      let items = [...new Map(syntheticWindows().map(item => [item.channel_id, item])).values()].map(item => ({ ...template, id: item.channel_id, name: item.name, type: 57, key: '', status: 1 }))
      items.push({ ...template, id: 4, name: '合成普通 API 渠道', type: 1, key: '', status: 1 })
      const keyword = url.searchParams.get('keyword')
      if (keyword) items = items.filter(item => item.name.includes(keyword) || String(item.id) === keyword)
      result = { status: 200, body: { success: true, data: { items, total: items.length, page: 1, page_size: Number(url.searchParams.get('page_size') || 10) } } }
    } else if (role === 100 && request.method === 'GET' && /^\/api\/log(\/self)?$/.test(url.pathname)) {
      const page = Number(url.searchParams.get('p') || 1), size = Math.min(100, Math.max(1, Number(url.searchParams.get('page_size') || 20)))
      let items = Array.from({ length: 42 }, (_, index) => syntheticLog(501 + index))
      for (const [param, field] of [['request_id', 'request_id'], ['model_name', 'model_name'], ['token_name', 'token_name']]) if (url.searchParams.get(param)) items = items.filter(item => item[field].includes(url.searchParams.get(param)))
      result = { status: 200, body: { success: true, data: { total: items.length, page, page_size: size, items: items.slice((page - 1) * size, page * size) } } }
    } else if (role === 100 && request.method === 'GET' && /^\/api\/full-content-logs\/REQ-SYNTHETIC-\d+$/.test(url.pathname)) {
      result = { status: 200, body: { success: true, data: { request_id: url.pathname.split('/').pop(), timestamp: new Date(FIXTURE_TIME).toISOString(), method: 'POST', path: '/v1/responses', status: 200, duration_ms: 2000, chunk_count: 1, request_bytes: 10, response_bytes: 100, request_body: '{"input":"SYNTHETIC ONLY"}', response_body: 'data: {"choices":[{"delta":{"content":"合成响应，仅供页面验收"}}]}\n\ndata: [DONE]\n\n', response_content_type: 'text/event-stream', request_headers: {}, response_headers: {}, query: {} } } }
    } else result = fixture.response(url, request.method)
    if (result.violation) result = { status: 409, body: { success: false, message: '隔离预览未启用此操作。只使用合成数据，不连接真实服务。' } }
    if (url.pathname === '/api/user/auth/refresh' && role === 100) result.body.data.access_expires_at = Date.now() / 1000 + 3600
    response.writeHead(result.status || 200, { 'content-type': 'application/json' }).end(JSON.stringify(result.body)); return
  }
  let file
  try { file = resolve(root, `.${decodeURIComponent(url.pathname)}`) } catch { response.writeHead(400).end(); return }
  if (file !== root && !file.startsWith(root + sep)) { response.writeHead(403).end(); return }
  if (!existsSync(file) || statSync(file).isDirectory()) file = resolve(root, 'index.html')
  if (extname(file) === '.html') { response.writeHead(200, { 'content-type': mime['.html'] }).end(readFileSync(file, 'utf8').replace('<body>', `<body>${banner}`)); return }
  response.writeHead(200, { 'content-type': mime[extname(file)] || 'application/octet-stream' }); createReadStream(file).pipe(response)
})
server.listen(4181, '127.0.0.1', () => console.log('Synthetic platform preview: http://127.0.0.1:4181'))
