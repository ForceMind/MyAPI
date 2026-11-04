import { readFileSync, statSync } from 'node:fs'
import { operations, validateArguments } from './catalog.mjs'

const maxResponseBytes = 1024 * 1024

export function loadConfiguration(env = process.env) {
  const origin = new URL(env.MYAPI_MANAGEMENT_URL || '')
  const loopback = ['localhost', '127.0.0.1', '[::1]'].includes(origin.hostname)
  if (origin.username || origin.password || origin.search || origin.hash || origin.pathname !== '/' ||
      (origin.protocol !== 'https:' && !(origin.protocol === 'http:' && loopback))) throw new Error('INVALID_ORIGIN')
  if (env.MYAPI_MANAGEMENT_KEY && env.MYAPI_MANAGEMENT_KEY_FILE) throw new Error('AMBIGUOUS_CREDENTIAL')
  let key = env.MYAPI_MANAGEMENT_KEY
  if (env.MYAPI_MANAGEMENT_KEY_FILE) {
    const stat = statSync(env.MYAPI_MANAGEMENT_KEY_FILE)
    if (!stat.isFile() || stat.size > 256 || (process.platform !== 'win32' && (stat.mode & 0o077) !== 0)) throw new Error('UNSAFE_CREDENTIAL_FILE')
    key = readFileSync(env.MYAPI_MANAGEMENT_KEY_FILE, 'utf8').trim()
  }
  if (typeof key !== 'string' || !/^myapi_mgmt_[A-Za-z0-9_-]{20,200}$/.test(key)) throw new Error('INVALID_CREDENTIAL')
  return { origin: origin.origin, key }
}

// Defence in depth: the server supplies endpoint-specific safe projections.
// Do not expose transport errors, raw error messages, headers or credentials.
export function redact(value, secret) {
  if (JSON.isRawJSON?.(value)) return value
  if (typeof value === 'string') return value.replaceAll(secret, '[REDACTED]')
  if (Array.isArray(value)) return value.map(item => redact(item, secret))
  if (value && typeof value === 'object') {
    const output = {}
    for (const [name, item] of Object.entries(value)) {
      if (/^(key|secret|key_hash|token_hash|access_token|refresh_token|id_token|password|authorization|cookie|set-cookie|headers|request_body|response_body|content|other)$/i.test(name)) continue
      Object.defineProperty(output, name, { value: redact(item, secret), enumerable: true })
    }
    return output
  }
  return value
}

export function createClient(config, fetcher = fetch) {
  async function request(path, options = {}, write = false) {
    let response
    try {
      response = await fetcher(`${config.origin}${path}`, {
        ...options,
        headers: { Authorization: `Bearer ${config.key}`, Accept: 'application/json', ...options.headers },
        redirect: 'manual', signal: AbortSignal.timeout(30000),
      })
      if (response.status >= 300 && response.status < 400) throw new Error('REDIRECT_REJECTED')
      if (!response.ok) {
        await response.body?.cancel()
        return { success: false, code: `HTTP_${response.status}`, ...(write && response.status >= 500 ? { outcome: 'unknown', replay: false } : {}) }
      }
      const chunks = []
      let size = 0
      if (!response.body) throw new Error('EMPTY_RESPONSE')
      for await (const chunk of response.body) {
        size += chunk.length
        if (size > maxResponseBytes) throw new Error('RESPONSE_TOO_LARGE')
        chunks.push(chunk)
      }
      const data = JSON.parse(Buffer.concat(chunks).toString('utf8'), (name, value, context) => {
        if (typeof value === 'number' && (!Number.isFinite(value) || (Number.isInteger(value) && !Number.isSafeInteger(value)))) {
          if (!context?.source || typeof JSON.rawJSON !== 'function') throw new Error('NUMBER_PRECISION_UNAVAILABLE')
          return JSON.rawJSON(context.source)
        }
        return value
      })
      if (!data || typeof data !== 'object' || Array.isArray(data) || typeof data.success !== 'boolean') throw new Error('INVALID_RESPONSE')
      if (data.success !== true) return { success: false, code: 'API_REJECTED' }
      return redact(data, config.key)
    } catch {
      // No retry, refresh, alternate URL or fallback transport, especially after POST.
      return { success: false, code: 'REQUEST_UNCONFIRMED', ...(write ? { outcome: 'unknown', replay: false } : {}) }
    }
  }
  return {
    capabilities: () => request('/api/management/capabilities'),
    async call(name, args = {}) {
      const operation = operations.find(item => item.name === name)
      if (!operation) throw new Error('UNKNOWN_TOOL')
      validateArguments(operation, args)
      let path = operation.path.replace(':id', String(args.id))
      const query = new URLSearchParams()
      for (const [field, value] of Object.entries(args)) if (!['id', 'status', 'confirmation'].includes(field)) query.set(field, String(value))
      if (query.size) path += `?${query}`
      if (!operation.write) return request(path)
      return request(path, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-MyAPI-Confirm': args.confirmation },
        body: JSON.stringify({ status: args.status }),
      }, true)
    },
  }
}
