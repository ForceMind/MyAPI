import { randomUUID } from 'node:crypto'
import { constants } from 'node:fs'
import { link, lstat, open, readlink, realpath, rename, unlink } from 'node:fs/promises'
import path from 'node:path'
import { checkNativeDirectory, nativeServiceContext, nativeServiceState } from './native-install.mjs'

export const nativeReadFlags = constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK
const phases = new Set(['prepared', 'stop_requested', 'stopped', 'backup_requested', 'backed_up',
  'switch_requested', 'switched', 'start_requested', 'started', 'receipt_requested', 'committed'])

export function refuseNative(code, message) {
  throw Object.assign(new Error(message), { code })
}

export function isNativeReleaseIdentity(release) {
  return release && Object.keys(release).length === 4 &&
    /^[0-9][0-9A-Za-z.+-]{0,255}$/.test(release.version ?? '') &&
    /^[a-z0-9][a-z0-9._-]{0,127}$/.test(release.artifact ?? '') &&
    /^[a-f0-9]{40}$/.test(release.sourceSha ?? '') && /^[a-f0-9]{64}$/.test(release.sha256 ?? '')
}

export async function nativeEntry(filename) {
  try {
    return await lstat(filename)
  } catch (error) {
    if (error.code === 'ENOENT') return null
    throw error
  }
}

export async function managedFile(filename, uid, privateFile, errorCode) {
  await checkNativeDirectory(path.dirname(filename), uid)
  const file = await open(filename, nativeReadFlags).catch(() => refuseNative(errorCode, 'Check the managed file: ' + filename))
  try {
    const info = await file.stat({ bigint: true })
    if (!info.isFile() || info.uid !== BigInt(uid) || info.nlink !== 1n || (info.mode & (privateFile ? 0o077n : 0o022n)) ||
        info.size < 1n || info.size > 4n * 1024n * 1024n) {
      refuseNative(errorCode, 'Check the owner, permissions and contents of ' + filename)
    }
    const bytes = Buffer.alloc(Number(info.size) + 1)
    const { bytesRead } = await file.read(bytes, 0, bytes.length, 0)
    const named = await lstat(filename, { bigint: true })
    if (BigInt(bytesRead) !== info.size || named.ino !== info.ino || named.dev !== info.dev || named.ctimeNs !== info.ctimeNs) {
      refuseNative(errorCode, 'The managed file changed while being read: ' + filename)
    }
    return { text: new TextDecoder('utf-8', { fatal: true }).decode(bytes.subarray(0, bytesRead)),
      stamp: [info.dev, info.ino, info.ctimeNs].join(':') }
  } finally {
    await file.close()
  }
}

export async function syncNativePath(filename) {
  const file = await open(filename, nativeReadFlags)
  try { await file.sync() } finally { await file.close() }
}

// A phase is durable before its effect. Exclusive first publication also
// protects an unmanaged file that appeared after the initial inspection.
export async function storeNativeRecord(filename, record, exclusive = false) {
  const directory = path.dirname(filename)
  const temporary = path.join(directory, '.native-record-' + randomUUID())
  const file = await open(temporary, 'wx', 0o600)
  try {
    try {
      await file.writeFile(JSON.stringify(record) + '\n')
      await file.sync()
    } finally { await file.close() }
    if (exclusive) await link(temporary, filename)
    else await rename(temporary, filename)
  } finally {
    await unlink(temporary).catch(error => { if (error.code !== 'ENOENT') throw error })
  }
  await syncNativePath(directory)
}

export async function readNativeJournal(context) {
  const filename = path.join(context.paths.state, 'native-transition.json')
  if (!await nativeEntry(filename)) return null
  const { text: raw } = await managedFile(filename, context.uid, true, 'native_journal_unknown')
  let record
  try { record = JSON.parse(raw) } catch { /* Invalid state requires inspection. */ }
  if (record?.schema !== 1 || !phases.has(record.phase) || record.scope !== context.scope ||
      record.operation !== 'upgrade' || !/^[a-f0-9]{64}$/.test(record.confirmation ?? '') ||
      record.backup !== path.join(context.paths.backups, record.confirmation) ||
      ![record.current, record.target].every(isNativeReleaseIdentity)) {
    refuseNative('native_journal_unknown', '无法确认升级记录，请核对 ' + filename + ' / Review the protected transition record.')
  }
  return record
}

export function requireSettledTransition(record, confirmation) {
  if (record && record.phase !== 'committed') {
    refuseNative('native_transition_pending', '上次升级停在 ' + record.phase + '。请先查看 transition-status。 / Inspect the unfinished transition before taking action.')
  }
  if (record && record.confirmation === confirmation) {
    refuseNative('native_confirmation_consumed', '这次确认已执行，请查看 transition-status。 / This confirmation has already been used.')
  }
}

export async function readNativeTransitionStatus(scope) {
  const context = await nativeServiceContext(scope)
  const { paths, manager, uid } = context
  let journal
  let phase = 'unknown'
  try {
    journal = await readNativeJournal(context)
    phase = journal?.phase ?? 'none'
  } catch (error) {
    if (error.code !== 'native_journal_unknown' && error.code !== 'unsafe_directory') throw error
  }
  const service = await nativeServiceState(manager)
  const pointer = path.dirname(paths.binary)
  await checkNativeDirectory(path.dirname(pointer), uid)
  const entry = await nativeEntry(pointer)
  let activeBinary = null
  if (entry?.isSymbolicLink() && entry.uid === uid) {
    const selected = path.resolve(path.dirname(pointer), await readlink(pointer), 'my-api')
    if (selected.startsWith(paths.releases + '/') && await realpath(paths.binary).catch(() => null) === selected) {
      activeBinary = selected
    }
  }
  return { phase, operation: journal?.operation ?? null,
    current: journal?.current ?? null, target: journal?.target ?? null, backup: journal?.backup ?? null,
    locked: Boolean(await nativeEntry(path.join(context.runtime, 'myapi-native-install'))),
    activeBinary, service: { state: service.ActiveState, pid: /^\d+$/.test(service.MainPID) ? Number(service.MainPID) : null } }
}
