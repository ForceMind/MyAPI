// Synthetic page-layout data. No supplier calls, credentials or saved writes.
import { pathToFileURL } from 'node:url'
import { resolve } from 'node:path'
import { createKeyRequestPreview } from './key-request-preview.mjs'

export function createPageLayoutPreview(options) {
  const preview = createKeyRequestPreview(options)
  const response = preview.fixture.response.bind(preview.fixture)
  const state = { accounts: 'populated' }
  preview.fixture.response = (url, method = 'GET') => {
    if (method === 'GET' && url.pathname === '/api/channel/quota/changes') {
      if (state.accounts === 'error') return { status: 503, body: { success: false, message: 'Synthetic account observations unavailable' } }
      const now = Math.floor(Date.now() / 1000)
      const items = [1, 2].flatMap(channel_id => ['five_hour', 'weekly'].map((window_type, index) => ({
        channel_id, series_id: `${channel_id}${index}`.repeat(32),
        name: `Synthetic subscription account ${channel_id}`,
        account_label: channel_id === 1 ? 'Synthetic subscription · 合成订阅账号' : 'Synthetic long account name · 测试长名称的订阅账号与窗口切换',
        metric_type: 'codex_rate_limit', window_type,
        source: index === 0 ? 'codex_wham_usage_primary' : 'codex_wham_usage_secondary',
        window_seconds: index === 0 ? 18000 : 604800,
        unit: 'percent', status: 'success', current_available: index === 0 ? 45 : 32,
        observed_at: now - 60,
      })))
      return { status: 200, body: { success: true, data: { items: state.accounts === 'empty' ? [] : items, generated_at: now, items_complete: true, source_complete: true } } }
    }
    return response(url, method)
  }
  return { ...preview, state }
}

if (process.argv[1] && pathToFileURL(resolve(process.argv[1])).href === import.meta.url) {
  const port = Number(process.env.MYAPI_PREVIEW_PORT || 4184)
  const { server } = createPageLayoutPreview({ dist: process.env.MYAPI_UI_DIST })
  server.listen(port, '127.0.0.1', () => console.log(`Synthetic page preview: http://127.0.0.1:${port}/__preview`))
}
