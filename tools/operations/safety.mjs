import { createHash, timingSafeEqual } from 'node:crypto'
import { constants, lstat, open, realpath } from 'node:fs/promises'
import path from 'node:path'

export class OperationsError extends Error {
  constructor(code, status = 400) { super(code); this.code = code; this.status = status }
}
export function check(ok, code, status = 400) { if (!ok) throw new OperationsError(code, status) }
export function canonical(value) {
  if (Array.isArray(value)) return '[' + value.map(canonical).join(',') + ']'
  if (value && typeof value === 'object') return '{' + Object.keys(value).sort().map(k => JSON.stringify(k) + ':' + canonical(value[k])).join(',') + '}'
  return JSON.stringify(value)
}
export const digest = value => createHash('sha256').update(typeof value === 'string' || Buffer.isBuffer(value) ? value : canonical(value)).digest('hex')
export function secretEqual(a, b) {
  if (typeof a !== 'string' || typeof b !== 'string') return false
  const x = Buffer.from(a), y = Buffer.from(b)
  return x.length === y.length && timingSafeEqual(x, y)
}
export function exactKeys(value, keys, code = 'INVALID_REQUEST') {
  check(value && typeof value === 'object' && !Array.isArray(value) && Object.keys(value).sort().join() === [...keys].sort().join(), code)
}
export function absolute(value) { return typeof value === 'string' && path.isAbsolute(value) && path.resolve(value) === value && !value.includes('\0') }
export async function noSymlinks(filename) {
  check(absolute(filename), 'UNSAFE_PATH')
  check(await realpath(filename) === filename, 'UNSAFE_PATH')
}
export async function protectedRead(filename, { uid = 0, max = 1024 * 1024 } = {}) {
  await noSymlinks(filename)
  const before = await lstat(filename)
  check(before.isFile() && before.nlink === 1 && before.uid === uid && (before.mode & 0o777) === 0o600 && before.size > 0 && before.size <= max, 'UNSAFE_FILE')
  const file = await open(filename, constants.O_RDONLY | constants.O_NOFOLLOW)
  try {
    const after = await file.stat()
    check(after.dev === before.dev && after.ino === before.ino, 'UNSAFE_FILE')
    return await file.readFile()
  } finally { await file.close() }
}
export async function privateDirectory(directory, uid = process.getuid()) {
  await noSymlinks(directory)
  const stat = await lstat(directory)
  check(stat.isDirectory() && stat.uid === uid && (stat.mode & 0o077) === 0, 'UNSAFE_DIRECTORY')
}
export function safeCode(error) { return error instanceof OperationsError ? error.code : 'OPERATIONS_FAILED' }
export function artifactInput(value) {
  exactKeys(value, ['run_id', 'producer_attempt', 'artifact_id', 'source_sha', 'tree_sha', 'manifest_sha256', 'image_id', 'version'])
  check(['run_id', 'artifact_id'].every(k => typeof value[k] === 'string' && /^[1-9][0-9]{0,19}$/.test(value[k])) &&
    typeof value.producer_attempt === 'string' && /^[1-9][0-9]{0,5}$/.test(value.producer_attempt) &&
    ['source_sha', 'tree_sha'].every(k => typeof value[k] === 'string' && /^[a-f0-9]{40}$/.test(value[k])) &&
    /^[a-f0-9]{64}$/.test(value.manifest_sha256) && /^sha256:[a-f0-9]{64}$/.test(value.image_id) &&
    typeof value.version === 'string' && /^[A-Za-z0-9][A-Za-z0-9.+_-]{0,95}$/.test(value.version), 'INVALID_ARTIFACT')
  return structuredClone(value)
}
