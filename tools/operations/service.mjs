import { check, exactKeys, OperationsError, safeCode, secretEqual } from './safety.mjs'
import { OperationsEngine } from './engine.mjs'
import { publicPlan } from './store.mjs'

async function body(request) {
  check(request.headers['content-type']?.split(';')[0] === 'application/json', 'INVALID_CONTENT_TYPE', 415)
  let bytes = 0; const chunks = []
  for await (const chunk of request) { bytes += chunk.length; check(bytes <= 8192, 'REQUEST_TOO_LARGE', 413); chunks.push(chunk) }
  try { return JSON.parse(Buffer.concat(chunks)) } catch { throw new OperationsError('INVALID_JSON') }
}
function bearer(request) {
  const value = request.headers.authorization
  check(typeof value === 'string' && value.startsWith('Bearer ') && value.length <= 512, 'UNAUTHORIZED', 401)
  return value.slice(7)
}
function send(response, status, payload) {
  response.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8', 'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff', 'Content-Security-Policy': "default-src 'none'" })
  response.end(JSON.stringify(payload))
}

export async function createOperationsService(config, dependencies = {}) {
  const engine = await new OperationsEngine(config, dependencies).init()
  const bridgeToken = dependencies.bridgeToken
  check(typeof bridgeToken === 'string' && /^[A-Za-z0-9_-]{32,256}$/.test(bridgeToken), 'INVALID_BRIDGE_TOKEN')
  async function publicIdentities(secret, scope) {
    const result = []
    for (const instance of config.instances) {
      try { result.push({ instance, identity: await engine.validateManagementKey(instance, { secret, scope }) }) } catch {}
    }
    check(result.length > 0, 'UNAUTHORIZED', 401)
    return result
  }
  let activeRequests = 0
  const handle = bridge => async (request, response) => {
    let admitted = false
    try {
      check(activeRequests < 16, 'REQUEST_BUSY', 429); activeRequests++; admitted = true
      // No CORS, cookies, browser-origin requests or redirect-based routing.
      check(!request.headers.origin && !request.headers.cookie, 'BROWSER_CHANNEL_FORBIDDEN', 403)
      const url = new URL(request.url, 'https://operations.invalid')
      check(url.origin === 'https://operations.invalid' && !url.hash && !url.search && url.pathname === request.url, 'INVALID_ROUTE', 404)
      const route = url.pathname
      const match = /^\/v1\/plans\/([a-f0-9]{64})(?:\/(approve|execute))?$/.exec(route)
      const publicAllowed = request.method === 'GET' && ['/v1/capabilities', '/v1/instances', '/v1/plans'].includes(route) || request.method === 'POST' && route === '/v1/plans' ||
        match && (request.method === 'GET' && !match[2] || request.method === 'POST' && match[2] === 'execute')
      const bridgeAllowed = request.method === 'GET' && ['/v1/instances', '/v1/plans'].includes(route) || match &&
        (request.method === 'GET' && !match[2] || request.method === 'POST' && match[2] === 'approve')
      check(bridge ? bridgeAllowed : publicAllowed, 'ROUTE_NOT_FOUND', 404)
      const secret = bearer(request)
      let actor, bridgeInstance
      if (bridge) {
        check(secretEqual(secret, bridgeToken), 'UNAUTHORIZED', 401)
        const actorHeader = request.headers['x-myapi-actor-id']
        check(typeof actorHeader === 'string' && /^[1-9][0-9]{0,9}$/.test(actorHeader), 'UNAUTHORIZED', 401)
        bridgeInstance = engine.instance(request.headers['x-myapi-instance-id'])
        actor = await engine.validateActor(bridgeInstance, Number(actorHeader))
      }
      let data
      if (route === '/v1/capabilities') {
        const operations = new Set()
        for (const [scope, names] of [['operations.read', ['operations.instances', 'operations.plans', 'operations.plan']], ['operations.prepare', ['operations.prepare']], ['operations.execute', ['operations.execute']]]) {
          try { await publicIdentities(secret, scope); for (const name of names) operations.add(name) } catch {}
        }
        check(operations.size > 0, 'UNAUTHORIZED', 401)
        data = { operations: [...operations] }
      } else if (route === '/v1/instances') {
        const allowed = bridge ? [{ instance: bridgeInstance, identity: actor }] : await publicIdentities(secret, 'operations.read')
        data = await Promise.all(allowed.map(({ instance }) => engine.instanceStatus(instance)))
      } else if (route === '/v1/plans' && request.method === 'GET') {
        const allowed = bridge ? [{ instance: bridgeInstance, identity: actor }] : await publicIdentities(secret, 'operations.read')
        const plans = allowed.flatMap(({ instance, identity }) => engine.list(identity, instance.id)).sort((a, b) => b.created_at - a.created_at)
        const terminal = new Set(['succeeded', 'failed_before_start'])
        data = [...plans.filter(plan => !terminal.has(plan.state)), ...plans.filter(plan => terminal.has(plan.state)).slice(0, 100)]
      } else if (route === '/v1/plans') {
        const input = await body(request); exactKeys(input, ['instance_id', 'artifact'])
        const instance = engine.instance(input.instance_id)
        const identity = await engine.validateManagementKey(instance, { secret, scope: 'operations.prepare' })
        data = await engine.prepare(instance.id, input.artifact, identity)
      } else {
        const plan = engine.get(match[1]); const instance = engine.instance(plan.instance_id)
        const identity = bridge ? actor : await engine.validateManagementKey(instance, { secret, scope: match[2] === 'execute' ? 'operations.execute' : 'operations.read' })
        engine.authorizePlan(plan, identity, bridge ? bridgeInstance.id : instance.id)
        if (!match[2]) data = publicPlan(plan)
        else {
          const input = await body(request); exactKeys(input, ['digest']); check(typeof input.digest === 'string' && /^[a-f0-9]{64}$/.test(input.digest), 'INVALID_DIGEST')
          data = match[2] === 'approve' ? await engine.approve(plan.id, input.digest, actor, bridgeInstance.id) : await engine.execute(plan.id, input.digest, identity)
        }
      }
      send(response, 200, { success: true, data })
    } catch (error) { if (!response.headersSent) send(response, error instanceof OperationsError ? error.status : 503, { success: false, code: `OPERATIONS_${safeCode(error)}` }); else response.destroy() }
    finally { if (admitted) activeRequests-- }
  }
  return { publicHandler: handle(false), bridgeHandler: handle(true), engine, close: () => engine.drain() }
}
