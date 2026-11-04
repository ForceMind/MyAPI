import { check, digest } from './safety.mjs'
import { python } from './python.mjs'

export async function validateManagementKey(instance, { secret, scope, key_id, owner_id }) {
  check(typeof scope === 'string' && ['operations.read', 'operations.prepare', 'operations.execute'].includes(scope), 'INVALID_SCOPE')
  let request
  if (secret !== undefined) {
    check(typeof secret === 'string' && /^myapi_mgmt_[a-f0-9]{64}$/.test(secret), 'UNAUTHORIZED', 401)
    request = { action: 'key', secret_hash: digest(secret) }
  } else {
    check(Number.isSafeInteger(key_id) && key_id > 0 && Number.isSafeInteger(owner_id) && owner_id > 0, 'UNAUTHORIZED', 401)
    request = { action: 'key_id', key_id, owner_id }
  }
  const result = await python('sqlite_read.py', { ...request, sqlite_file: instance.sqlite_file, scope })
  check(result.valid === true, 'UNAUTHORIZED', 401)
  return { key_id: result.key_id, owner_id: result.owner_id }
}
export async function validateActor(instance, owner_id) {
  check(Number.isSafeInteger(owner_id) && owner_id > 0, 'UNAUTHORIZED', 401)
  const result = await python('sqlite_read.py', { action: 'actor', sqlite_file: instance.sqlite_file, owner_id })
  check(result.valid === true, 'UNAUTHORIZED', 401)
  return { owner_id }
}
