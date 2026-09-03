#!/usr/bin/env node

// A deliberately tiny loopback-only OpenAI-shaped upstream for the isolated
// Docker smoke. It retains only booleans needed by the control endpoint.
import http from 'node:http'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const expectedPath = '/v1/chat/completions'
const expectedModel = 'smoke-model'
const expectedBearer = 'Bearer synthetic-upstream-key'
const controlPath = '/__smoke__/control'
const healthPath = '/__smoke__/health'
const maxRequestBytes = 64 * 1024

function writeJson(response, status, value, headers = {}) {
  response.writeHead(status, { 'Content-Type': 'application/json', ...headers })
  response.end(JSON.stringify(value))
}

function readJson(request) {
  return new Promise((resolve) => {
    let size = 0
    const chunks = []
    request.on('data', (chunk) => {
      size += chunk.length
      if (size <= maxRequestBytes) chunks.push(chunk)
    })
    request.on('end', () => {
      if (size > maxRequestBytes) return resolve(null)
      try { resolve(JSON.parse(Buffer.concat(chunks).toString('utf8'))) } catch { resolve(null) }
    })
    request.on('error', () => resolve(null))
  })
}

export function startFakeOpenAI({ host = '127.0.0.1', port = 19090 } = {}) {
  if (host !== '127.0.0.1' || !Number.isInteger(port) || port < 0 || port > 65535) {
    throw new Error('INVALID_FAKE_OPENAI_LISTENER')
  }
  const state = { count: 0, path_ok: false, model_ok: false, max_tokens_ok: false, stream_ok: false, bearer_ok: false, request_ok: false }
  const server = http.createServer(async (request, response) => {
    const url = new URL(request.url || '/', 'http://127.0.0.1')
    if (request.method === 'GET' && url.pathname === healthPath && !url.search) {
      writeJson(response, 200, { ok: true })
      return
    }
    if (request.method === 'GET' && url.pathname === controlPath && !url.search) {
      writeJson(response, 200, state)
      return
    }
    if (request.method !== 'POST' || url.pathname !== expectedPath || url.search) {
      writeJson(response, 404, { error: { message: 'synthetic route not found' } })
      return
    }
    state.count += 1
    const body = await readJson(request)
    state.path_ok = true
    state.model_ok = body?.model === expectedModel
    state.max_tokens_ok = body?.max_tokens === 8
    state.stream_ok = body?.stream === false
    state.bearer_ok = request.headers.authorization === expectedBearer
    state.request_ok = state.path_ok && state.model_ok && state.max_tokens_ok && state.stream_ok && state.bearer_ok
    if (!state.request_ok) {
      writeJson(response, 400, { error: { message: 'synthetic request rejected' } })
      return
    }
    writeJson(response, 200, {
      id: 'synthetic-completion',
      object: 'chat.completion',
      created: 1,
      model: expectedModel,
      choices: [{ index: 0, message: { role: 'assistant', content: 'synthetic fixed response' }, finish_reason: 'stop' }],
      usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 },
    }, { 'X-Api-Key': 'synthetic-response-header-secret' })
  })
  return new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen({ host, port }, () => {
      server.off('error', reject)
      resolve({
        host,
        port: server.address().port,
        close: () => new Promise((closeResolve, closeReject) => server.close((error) => error ? closeReject(error) : closeResolve())),
      })
    })
  })
}

async function main() {
  try {
    if (process.argv.length !== 2) throw new Error('INVALID_FAKE_OPENAI_ARGUMENTS')
    const listener = await startFakeOpenAI()
    const close = async () => {
      process.off('SIGINT', close)
      process.off('SIGTERM', close)
      await listener.close()
    }
    process.on('SIGINT', close)
    process.on('SIGTERM', close)
  } catch (error) {
    const code = error?.message === 'INVALID_FAKE_OPENAI_ARGUMENTS' || error?.message === 'INVALID_FAKE_OPENAI_LISTENER'
      ? error.message
      : 'FAKE_OPENAI_START_FAILED'
    console.error(JSON.stringify({ command: 'fake-openai', started: false, code }))
    process.exitCode = 1
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await main()
