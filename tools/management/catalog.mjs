// This is a fixed API catalogue, never a caller-supplied URL or HTTP method.
const id = { type: 'integer', minimum: 1, maximum: 2147483647 }
const page = { type: 'integer', minimum: 1, maximum: 1000000 }
const pageSize = { type: 'integer', minimum: 1, maximum: 100 }
const list = { p: page, page_size: pageSize }
const timestamp = { type: 'integer', minimum: 0, maximum: 9007199254740991 }
const text = { type: 'string', maxLength: 256 }
const logFilters = {
  start_timestamp: timestamp, end_timestamp: timestamp,
  request_id: text, upstream_request_id: text, model_name: text,
  username: text, token_name: text, group: text, channel: id,
  type: { type: 'integer', minimum: 0, maximum: 20 },
  result: { type: 'string', enum: ['failed'], maxLength: 16 },
}
// The legacy quota totals endpoint does not apply request/result filters to
// every returned total. Do not advertise those filters as if it did.
const statisticFilters = Object.fromEntries(Object.entries(logFilters).filter(([name]) => !['request_id', 'upstream_request_id', 'result'].includes(name)))
const planID = { type: 'string', pattern: '^[a-f0-9]{32,64}$', maxLength: 64 }
const digest = { type: 'string', pattern: '^[a-f0-9]{64}$', maxLength: 64 }
const githubID = { type: 'string', pattern: '^[1-9][0-9]{0,19}$', maxLength: 20 }

function operation(name, scope, description, path, properties = {}, required = [], write = false) {
  return {
    name: `myapi_${name.replaceAll('.', '_')}`,
    operation: name, scope, path, write,
    description,
    inputSchema: { type: 'object', properties, required, additionalProperties: false },
    annotations: { readOnlyHint: !write, destructiveHint: write, idempotentHint: !write, openWorldHint: false },
  }
}

export const operations = [
  operation('channels.list', 'channels.read', 'List saved channel metadata. Does not contact upstreams. Scoped keys may require channels.get instead.', '/api/channel/', list),
  operation('channels.get', 'channels.read', 'Read saved metadata for one allowed channel; never return its credential.', '/api/channel/:id', { id }, ['id']),
  operation('channels.ops', 'channels.read', 'Read saved channel operations state without testing channels.', '/api/channel/ops'),
  operation('channels.quota_history', 'channels.read', 'Read saved quota observations; absent, null and zero remain distinct.', '/api/channel/:id/quota/history', {
    id, start: timestamp, end: timestamp,
    range: { type: 'string', enum: ['1h', '6h', '24h', '7d', '30d', '90d', 'custom'], maxLength: 16 },
    limit: { type: 'integer', minimum: 1, maximum: 500 },
  }, ['id']),
  operation('channels.quota_status', 'channels.read', 'Read the saved quota sampling status, without starting a sample.', '/api/channel/quota/status'),
  operation('channels.set_status', 'channels.operate', 'Change one channel status after explicit user approval. confirmation must be channel.status:<id>:<status>. A network failure means outcome_unknown; do not automatically replay.', '/api/channel/:id/status', {
    id, status: { type: 'integer', enum: [1, 2] },
    confirmation: { type: 'string', maxLength: 100 },
  }, ['id', 'status', 'confirmation'], true),
  operation('tokens.list', 'tokens.read', 'List this owner\'s API Key metadata, without key values. Scoped keys may require tokens.get instead.', '/api/token/', list),
  operation('tokens.get', 'tokens.read', 'Read one allowed API Key owned by the management Key creator, without its secret.', '/api/token/:id', { id }, ['id']),
  operation('logs.list', 'logs.read', 'Read paginated saved usage records without request/response bodies or credentials.', '/api/log/', { ...list, ...logFilters }),
  operation('logs.stats', 'logs.read', 'Read saved usage totals using the shared supported filters. For one request use logs.list. This does not send model requests.', '/api/log/stat', statisticFilters),
  operation('logs.overview', 'logs.read', 'Read the recent saved request overview, without full-content logs.', '/api/log/overview'),
  operation('system.instances', 'system.read', 'Read registered application instances. Requires the creator\'s current Root permission.', '/api/system-info/instances'),
  operation('system.self', 'system.read', 'Read the current management Key owner\'s public account state.', '/api/user/self'),
  { ...operation('operations.instances', 'operations.read', 'Read registered Docker Full SQLite instances through the independent operations service, including while MyAPI is stopped.', '/v1/instances'), operationsService: true },
  { ...operation('operations.plans', 'operations.read', 'Read your persisted upgrade plans. Interrupted or recovery_required plans must not be replayed.', '/v1/plans'), operationsService: true },
  { ...operation('operations.plan', 'operations.read', 'Read one immutable upgrade plan and its recorded outcome. No upgrade is started.', '/v1/plans/:id', { id: planID }, ['id']), operationsService: true },
  { ...operation('operations.prepare', 'operations.prepare', 'Prepare a bounded upgrade plan for a registered instance and a successful ForceMind/MyAPI GitHub Actions artifact. Does not stop a container. A Root user must approve the exact plan in the dashboard using 2FA or Passkey before execution.', '/v1/plans', {
    instance_id: { type: 'string', pattern: '^[a-z0-9][a-z0-9-]{0,47}$', maxLength: 48 },
    artifact: { type: 'object', additionalProperties: false, properties: {
      run_id: githubID, producer_attempt: githubID, artifact_id: githubID,
      source_sha: { type: 'string', pattern: '^[a-f0-9]{40}$', maxLength: 40 },
      tree_sha: { type: 'string', pattern: '^[a-f0-9]{40}$', maxLength: 40 },
      manifest_sha256: digest,
      image_id: { type: 'string', pattern: '^sha256:[a-f0-9]{64}$', maxLength: 71 },
      version: { type: 'string', pattern: '^0\\.2\\.0-beta\\.[1-9][0-9]{0,6}$', maxLength: 32 },
    }, required: ['run_id', 'producer_attempt', 'artifact_id', 'source_sha', 'tree_sha', 'manifest_sha256', 'image_id', 'version'] },
  }, ['instance_id', 'artifact'], true), operationsService: true, annotations: { readOnlyHint: false, destructiveHint: false, idempotentHint: false, openWorldHint: false } },
  { ...operation('operations.execute', 'operations.execute', 'Execute a previously approved, exact upgrade plan once. Requires explicit user authorization and confirmation operations.execute:<plan-id>:<digest>. Unknown outcomes must be queried by plan ID; never retry execution. Does not approve plans or bypass the dashboard proof.', '/v1/plans/:id/execute', {
    id: planID, digest, confirmation: { type: 'string', maxLength: 160 },
  }, ['id', 'digest', 'confirmation'], true), operationsService: true },
]

function validateObject(schema, args) {
  if (!args || typeof args !== 'object' || Array.isArray(args)) throw new Error('INVALID_ARGUMENTS')
  for (const name of schema.required) if (!Object.hasOwn(args, name)) throw new Error('INVALID_ARGUMENTS')
  for (const [name, value] of Object.entries(args)) {
    if (!Object.hasOwn(schema.properties, name)) throw new Error('INVALID_ARGUMENTS')
    const rule = schema.properties[name]
    if (!rule) throw new Error('INVALID_ARGUMENTS')
    if (rule.type === 'integer' && (!Number.isSafeInteger(value) || value < (rule.minimum ?? 0) || value > (rule.maximum ?? 2147483647))) throw new Error('INVALID_ARGUMENTS')
    if (rule.type === 'string' && (typeof value !== 'string' || value.length > rule.maxLength)) throw new Error('INVALID_ARGUMENTS')
    if (rule.pattern && !new RegExp(rule.pattern).test(value)) throw new Error('INVALID_ARGUMENTS')
    if (rule.type === 'object') validateObject(rule, value)
    if (rule.enum && !rule.enum.includes(value)) throw new Error('INVALID_ARGUMENTS')
  }
}

export function validateArguments(operation, args) {
  validateObject(operation.inputSchema, args)
  if (operation.operation === 'channels.set_status' && args.confirmation !== `channel.status:${args.id}:${args.status}`) throw new Error('CONFIRMATION_REQUIRED')
  if (operation.operation === 'operations.execute' && args.confirmation !== `operations.execute:${args.id}:${args.digest}`) throw new Error('CONFIRMATION_REQUIRED')
}
