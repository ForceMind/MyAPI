import { setTimeout } from 'node:timers/promises'
import { check } from './safety.mjs'

async function get(url, max, fetcher) {
  const response = await fetcher(url, { method: 'GET', redirect: 'manual', signal: AbortSignal.timeout(10_000), headers: { Accept: '*/*' } })
  check(response.status === 200 && Number(response.headers.get('content-length') || 0) <= max, 'HEALTH_CHECK_FAILED', 503)
  const chunks = []; let bytes = 0
  check(response.body, 'HEALTH_CHECK_FAILED', 503)
  for await (const chunk of response.body) { bytes += chunk.length; check(bytes <= max, 'HEALTH_CHECK_FAILED', 503); chunks.push(chunk) }
  return { response, body: Buffer.concat(chunks).toString('utf8') }
}

export async function readVersion(instance, { fetcher = fetch } = {}) {
  const origin = new URL(instance.health_url).origin
  const status = await get(origin + '/api/status', 1024 * 1024, fetcher)
  let parsed
  try { parsed = JSON.parse(status.body) } catch { check(false, 'HEALTH_CHECK_FAILED', 503) }
  check(parsed.success === true && typeof parsed.data?.version === 'string', 'VERSION_UNAVAILABLE', 503)
  return parsed.data.version
}

export async function verifyHealth(instance, version, { fetcher = fetch } = {}) {
  const origin = new URL(instance.health_url).origin
  const deadline = Date.now() + 60_000
  while (true) {
    try { check(await readVersion(instance, { fetcher }) === version, 'VERSION_MISMATCH', 503); break }
    catch (error) { if (error.code === 'VERSION_MISMATCH' || Date.now() >= deadline) throw error; await setTimeout(1000) }
  }
  const html = await get(origin + '/', 4 * 1024 * 1024, fetcher)
  check(html.response.headers.get('content-type')?.includes('text/html') && /<html[\s>]/i.test(html.body), 'STATIC_CHECK_FAILED', 503)
  const assets = new Set()
  for (const match of html.body.matchAll(/(?:src|href)\s*=\s*["']([^"']+\.(?:js|css)(?:\?[^"']*)?)["']/gi)) {
    let asset
    try { asset = new URL(match[1], origin) } catch { continue }
    if (asset.origin !== origin || asset.username || asset.password || asset.hash || asset.search || !/^\/(?:(?:assets|static)\/[A-Za-z0-9_./-]+|[A-Za-z0-9_.-]+)\.(?:js|css)$/.test(asset.pathname) || asset.pathname.includes('..')) continue
    assets.add(asset.href)
    check(assets.size <= 16, 'STATIC_ASSET_LIMIT', 503)
  }
  check(assets.size > 0, 'STATIC_CHECK_FAILED', 503)
  for (const asset of assets) {
    const result = await get(asset, 16 * 1024 * 1024, fetcher)
    const type = result.response.headers.get('content-type') || ''
    check(result.body.length > 0 && (asset.endsWith('.css') ? type.includes('text/css') : /(?:javascript|ecmascript)/.test(type)), 'STATIC_CHECK_FAILED', 503)
  }
  return { health: true, version: true, static_assets: true }
}
