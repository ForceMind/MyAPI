import { createHash } from 'node:crypto'
import { constants } from 'node:fs'
import { lstat, open, readFile, readdir, readlink, realpath, stat } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { checkNativeDirectory, nativeAccountEntry, nativeServiceContext, nativeServiceState } from './native-install.mjs'
import { parseNativePackageArgs, verifyNativeTransitionPackages } from './native-package-verification.mjs'
import { compareSemver } from './release-manifest.mjs'

const readFlags = constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK
const storeKeys = new Set(['SQL_DSN', 'LOG_SQL_DSN', 'SQLITE_PATH'])
const unitName = 'my-api.service'

function refuse(code, message) {
  throw Object.assign(new Error(message), { code })
}

function unknownDatabase() {
  refuse('native_sqlite_mismatch', 'Check the SQLite path and main/log database settings in the service configuration. 请核对服务配置中的 SQLite 路径和主库、日志库设置。')
}

async function managedText(filename, uid, privateFile, errorCode) {
  await checkNativeDirectory(path.dirname(filename), uid)
  const file = await open(filename, readFlags).catch(() => refuse(errorCode, 'Check the managed file: ' + filename))
  try {
    const info = await file.stat()
    if (!info.isFile() || info.uid !== uid || info.nlink !== 1 || (info.mode & (privateFile ? 0o077 : 0o022)) ||
        info.size < 1 || info.size > 4 * 1024 * 1024) {
      refuse(errorCode, 'Check the owner, permissions and contents of ' + filename)
    }
    const bytes = Buffer.alloc(info.size + 1)
    const { bytesRead } = await file.read(bytes, 0, bytes.length, 0)
    if (bytesRead !== info.size) refuse(errorCode, 'The managed file changed while being read: ' + filename)
    return new TextDecoder('utf-8', { fatal: true }).decode(bytes.subarray(0, bytesRead))
  } finally {
    await file.close()
  }
}

// Read systemd EnvironmentFile assignments as data. Consume multiline values,
// including unrelated ones, so their contents cannot become database settings.
function databaseSettings(text) {
  const selected = new Map()
  const lines = text.split(/\r?\n/)
  for (let index = 0; index < lines.length; index++) {
    const line = lines[index].trimStart()
    if (!line || line.startsWith('#') || line.startsWith(';')) continue
    const entry = /^([A-Za-z_][A-Za-z_0-9]*)[ \t]*=[ \t]*(.*)$/.exec(line)
    if (!entry) unknownDatabase()
    const key = entry[1]
    let raw = entry[2]
    const quote = raw[0] === '"' || raw[0] === "'" ? raw[0] : null
    let value = ''
    let pos = quote ? 1 : 0
    for (;;) {
      if (pos === raw.length) {
        if (!quote) break
        if (++index === lines.length) unknownDatabase()
        raw = lines[index]
        value += '\n'
        pos = 0
        continue
      }
      const c = raw[pos++]
      if (quote && c === quote) {
        if (raw.slice(pos).trim()) unknownDatabase()
        break
      }
      if (c !== '\\' || quote === "'") {
        value += c
        continue
      }
      if (pos === raw.length) {
        if (++index === lines.length) unknownDatabase()
        raw = lines[index]
        pos = 0
      } else {
        const escaped = raw[pos++]
        value += !quote || ['"', '\\', '$', '`'].includes(escaped) ? escaped : '\\' + escaped
      }
    }
    if (storeKeys.has(key)) selected.set(key, quote ? value : value.trimEnd())
  }
  return selected
}

async function sqliteFile(dsn, data) {
  let filename = dsn?.trim()
  if (!filename) {
    // Match common.ResolveSQLitePath for an explicit empty setting.
    const canonical = path.join(data, 'my-api.db')
    const exists = await lstat(canonical).catch(error => {
      if (error.code !== 'ENOENT') throw error
      return null
    })
    filename = exists ? canonical : path.join(data, 'one-api.db')
  }
  if (filename.startsWith('file:')) {
    let uri
    try {
      uri = new URL(filename, pathToFileURL(data + '/'))
      if (uri.searchParams.get('mode') === 'memory' || uri.hash) unknownDatabase()
      filename = fileURLToPath(uri)
    } catch { unknownDatabase() }
  } else {
    const query = filename.indexOf('?')
    if (query !== -1) {
      if (new URLSearchParams(filename.slice(query + 1)).get('mode') === 'memory') unknownDatabase()
      filename = filename.slice(0, query)
    }
  }
  if (!filename || filename === ':memory:' || /[\u0000-\u001f\u007f]/.test(filename)) unknownDatabase()
  const resolved = path.resolve(data, filename)
  if (!resolved.startsWith(data + '/') || await realpath(resolved).catch(() => null) !== resolved) unknownDatabase()
  return resolved
}

async function runningDatabase(context, pid, config, serviceUid) {
  const { paths, manager } = context
  const saved = databaseSettings(config)
  const live = new Map()
  const env = await readFile('/proc/' + pid + '/environ', 'utf8').catch(unknownDatabase)
  for (const assignment of env.split('\0')) {
    const split = assignment.indexOf('=')
    const key = assignment.slice(0, split)
    if (split > 0 && storeKeys.has(key)) live.set(key, assignment.slice(split + 1))
  }
  // A user manager can inject variables omitted by an edited EnvironmentFile.
  // B3c configurations explicitly clear both DSNs and need no manager lookup.
  const inherited = new Map()
  if (context.scope === 'user' && (!saved.has('SQL_DSN') || !saved.has('LOG_SQL_DSN'))) {
    const { stdout } = await manager(['show-environment'])
    for (const line of stdout.split('\n')) {
      const [key] = line.split('=', 1)
      if (storeKeys.has(key)) inherited.set(key, line.slice(key.length + 1))
    }
  }
  // main.go loads .env from the working directory for missing variables.
  // /proc only exposes the original environment, before godotenv runs.
  const dotenv = await lstat(path.join(paths.data, '.env')).catch(error => {
    if (error.code !== 'ENOENT') unknownDatabase()
    return null
  })
  if (dotenv && [...storeKeys].some(key => !live.has(key) ||
      (key !== 'SQLITE_PATH' && !saved.has(key) && !inherited.has(key)))) unknownDatabase()
  for (const key of ['SQL_DSN', 'LOG_SQL_DSN']) {
    for (const dsn of [live.get(key), saved.get(key) ?? inherited.get(key)]) {
      if (dsn && !dsn.startsWith('local')) unknownDatabase()
    }
  }
  const database = await sqliteFile(live.get('SQLITE_PATH'), paths.data)
  const next = await sqliteFile(saved.get('SQLITE_PATH') ?? path.join(paths.data, 'my-api.db'), paths.data)
  if (database !== next) unknownDatabase()
  const db = await open(database, readFlags).catch(unknownDatabase)
  try {
    const info = await db.stat({ bigint: true })
    if (!info.isFile() || info.nlink !== 1n || info.uid !== BigInt(serviceUid)) unknownDatabase()
    const header = Buffer.alloc(16)
    const { bytesRead } = await db.read(header, 0, header.length, 0)
    if (bytesRead !== header.length || !header.equals(Buffer.from('SQLite format 3\0'))) unknownDatabase()
    const fdDir = '/proc/' + pid + '/fd'
    for (const fd of await readdir(fdDir).catch(unknownDatabase)) {
      const entry = path.join(fdDir, fd)
      if (await readlink(entry).catch(() => null) !== database) continue
      const held = await stat(entry, { bigint: true }).catch(() => null)
      if (held?.dev === info.dev && held.ino === info.ino) {
        return { database, id: info.dev + ':' + info.ino }
      }
    }
    unknownDatabase()
  } finally {
    await db.close()
  }
}

async function liveInstallation(inputs, context) {
  const { scope, uid, paths, manager, unit } = context
  const receiptPath = path.join(paths.state, 'installation.json')
  const raw = await managedText(receiptPath, uid, true, 'invalid_installation_record')
  let record
  try {
    record = JSON.parse(raw)
    if (typeof record?.version !== 'string' || record.version.length > 256) throw new Error()
    compareSemver(record.version, record.version)
  } catch { refuse('invalid_installation_record', 'Check the completed installation record: ' + receiptPath) }
  if (record.scope !== scope || record.edition !== inputs.edition || !['amd64', 'arm64'].includes(record.arch) ||
      ['sourceSha', 'sha256', 'artifact'].some(k => typeof record[k] !== 'string') ||
      !/^[a-z0-9][a-z0-9._-]{0,127}$/.test(record.artifact) ||
      !/^[0-9a-f]{40}$/i.test(record.sourceSha ?? '') || !/^[0-9a-f]{64}$/.test(record.sha256 ?? '') ||
      !Number.isInteger(record.port) || record.port < 1024 || record.port > 65535 ||
      record.binary !== paths.binary || record.config !== paths.config || record.unit !== paths.unit) {
    refuse('invalid_installation_record', 'Check the edition, installed artifact and paths in ' + receiptPath)
  }
  const installed = Object.fromEntries(['version', 'sourceSha', 'artifact', 'sha256', 'edition', 'scope', 'arch'].map(k => [k, record[k]]))
  installed.sourceSha = installed.sourceSha.toLowerCase()
  installed.sha256 = installed.sha256.toLowerCase()
  const binary = path.join(paths.releases, record.version, record.artifact, 'my-api')
  const unexpected = () => refuse('native_service_mismatch', 'Check the running service, unit and current release. 请核对运行中的服务、单元文件和当前程序目录。')
  await checkNativeDirectory(path.dirname(binary), uid)
  const [state, unitText, config] = await Promise.all([
    nativeServiceState(manager),
    managedText(paths.unit, uid, false, 'native_service_mismatch'),
    managedText(paths.config, uid, true, 'native_sqlite_mismatch'),
  ])
  const pid = state.MainPID
  if (state.ActiveState !== 'active' || state.LoadState !== 'loaded' || state.FragmentPath !== paths.unit ||
      state.DropInPaths !== '' || state.NeedDaemonReload !== 'no' || !/^[1-9]\d*$/.test(pid) ||
      !/^[0-9a-f]{32}$/i.test(state.InvocationID ?? '') || unitText !== unit) unexpected()
  const proc = '/proc/' + pid
  const [selected, metadata, exe, cwd, procStatus, dataRoot] = await Promise.all([
    realpath(paths.binary), lstat(binary, { bigint: true }), stat(proc + '/exe', { bigint: true }),
    readlink(proc + '/cwd'), readFile(proc + '/status', 'utf8'), lstat(paths.data),
  ]).catch(unexpected)
  const serviceUid = Number(/^Uid:\s+\d+\s+(\d+)\s/m.exec(procStatus)?.[1])
  const expectedUid = scope === 'system' ? Number((await nativeAccountEntry('passwd'))?.split(':')[2]) : uid
  if (selected !== binary || !metadata.isFile() || metadata.uid !== BigInt(uid) || (metadata.mode & 0o022n) ||
      metadata.dev !== exe.dev || metadata.ino !== exe.ino || cwd !== paths.data ||
      !dataRoot.isDirectory() || dataRoot.uid !== serviceUid || !serviceUid || serviceUid !== expectedUid) unexpected()
  const { database, id } = await runningDatabase(context, pid, config, serviceUid)
  const identity = createHash('sha256').update(JSON.stringify([
    raw, config, unitText, state.InvocationID, pid, binary, String(metadata.dev), String(metadata.ino),
    String(metadata.ctimeNs), database, id,
  ])).digest('hex')
  return { installed, binary, database, service: { name: unitName, scope, pid: Number(pid) }, identity }
}

export async function planNativeTransition(inputs, verifyPackages = verifyNativeTransitionPackages) {
  const context = await nativeServiceContext(inputs.scope)
  const before = await liveInstallation(inputs, context)
  const transition = await verifyPackages(inputs, before.installed, before.binary, 'upgrade')
  const after = await liveInstallation(inputs, context)
  if (after.identity !== before.identity) {
    refuse('native_state_changed', 'The running installation changed during verification. 请重新检查运行中的安装。')
  }
  return {
    eligible: true, ...transition, database: before.database, service: before.service,
    steps: ['确认后排空并停止服务 / Drain and stop after confirmation',
      '备份并核验最新 SQLite / Back up and verify the latest SQLite data',
      '切换程序后核对服务和账务 / Switch the program and check service and accounting'],
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  Promise.resolve().then(async () => {
    const extra = ['current-manifest', 'current-bundle']
    const inputs = parseNativePackageArgs(process.argv.slice(2), extra)
    if (extra.some(key => !inputs[key])) refuse('invalid_arguments', 'Supply both --current-manifest and --current-bundle.')
    console.log(JSON.stringify(await planNativeTransition(inputs)))
  }).catch(error => {
    const code = /^[a-z_]+$/.test(error.code ?? '') ? error.code : 'inspection_failed'
    const reason = code === 'inspection_failed' ? 'Check the installation record, file permissions and systemd. 请核对安装记录、文件权限和 systemd。' : error.message
    console.error(JSON.stringify({ eligible: false, code, reason }))
    process.exitCode = 2
  })
}
