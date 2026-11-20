import { execFile } from 'node:child_process'
import { createHash, randomUUID } from 'node:crypto'
import { chmod, lstat, mkdir, mkdtemp, open, readFile, readdir, readlink, realpath, rename, rmdir, stat, statfs, symlink, unlink } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { checkNativeDirectory, nativeAccountEntry, nativeServiceContext, nativeServiceState, stageBinary, waitForService } from './native-install.mjs'
import { binaryDigest, parseNativePackageArgs, readDocument, verifyNativeTransitionPackages } from './native-package-verification.mjs'
import { prepareNativeServiceStop } from './native-service-stop.mjs'
import { isNativeReleaseIdentity, managedFile, nativeEntry, nativeReadFlags as readFlags, readNativeJournal, readNativeTransitionStatus, refuseNative as refuse, requireSettledTransition, requireNoNativeUninstall, storeNativeRecord, syncNativePath } from './native-transition-record.mjs'
import { compareSemver } from './release-manifest.mjs'

export { readNativeTransitionStatus }

const storeKeys = new Set(['SQL_DSN', 'LOG_SQL_DSN', 'SQLITE_PATH', 'BATCH_UPDATE_ENABLED', 'PORT', 'MYAPI_BIND_ADDRESS', 'MYAPI_EDITION'])
const unitName = 'my-api.service'
const sqliteHelper = fileURLToPath(new URL('../../tools/operations/native_sqlite_transition.py', import.meta.url))

function unknownDatabase() {
  refuse('native_sqlite_mismatch', 'Check the SQLite path and main/log database settings in the service configuration. 请核对服务配置中的 SQLite 路径和主库、日志库设置。')
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

async function runningDatabase(context, pid, config, serviceUid, record) {
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
  if (context.scope === 'user' && [...storeKeys].some(key => key !== 'SQLITE_PATH' && !saved.has(key))) {
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
  // SQLite alone cannot prove pending balances of the optional Redis batch
  // writer. Never stop that writer or change its flag on the user's behalf.
  if ([live.get('BATCH_UPDATE_ENABLED'), saved.get('BATCH_UPDATE_ENABLED') ?? inherited.get('BATCH_UPDATE_ENABLED')]
    .some(setting => setting && setting !== 'false')) {
    refuse('native_batch_writer_unknown', '无法从 SQLite 核对批量记账的待写金额。请检查当前记账方式。 / Pending batch balances cannot be verified from SQLite.')
  }
  const defaults = { PORT: '3000', MYAPI_EDITION: 'full', MYAPI_BIND_ADDRESS: '' }
  const expected = { PORT: String(record.port), MYAPI_EDITION: record.edition, MYAPI_BIND_ADDRESS: '127.0.0.1' }
  for (const [key, value] of Object.entries(expected)) {
    if ((live.get(key) || defaults[key]) !== value || ((saved.get(key) ?? inherited.get(key)) || defaults[key]) !== value) {
      refuse('native_listener_mismatch', '请核对已安装服务的监听地址、端口和版本形态。 / Check the saved listener, port and edition before stopping.')
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

export async function liveInstallation(inputs, context, expectedRelease) {
  const { scope, uid, paths, manager, unit } = context
  const receiptPath = path.join(paths.state, 'installation.json')
  const receiptFile = await managedFile(receiptPath, uid, true, 'invalid_installation_record')
  const raw = receiptFile.text
  let record
  try {
    record = JSON.parse(raw)
    if (typeof record?.version !== 'string' || record.version.length > 256) throw new Error()
    compareSemver(record.version, record.version)
  } catch { refuse('invalid_installation_record', 'Check the completed installation record: ' + receiptPath) }
  if (record.scope !== scope || !['full', 'lite'].includes(record.edition) ||
      (scope === 'user' && record.edition !== 'lite') ||
      (inputs.edition !== undefined && record.edition !== inputs.edition) || !['amd64', 'arm64'].includes(record.arch) ||
      ['sourceSha', 'sha256', 'artifact'].some(k => typeof record[k] !== 'string') ||
      !/^[a-z0-9][a-z0-9._-]{0,127}$/.test(record.artifact) ||
      !/^[0-9a-f]{40}$/i.test(record.sourceSha ?? '') || !/^[0-9a-f]{64}$/.test(record.sha256 ?? '') ||
      !Number.isInteger(record.port) || record.port < 1024 || record.port > 65535 ||
      record.binary !== paths.binary || record.config !== paths.config || record.unit !== paths.unit) {
    refuse('invalid_installation_record', 'Check the edition, installed artifact and paths in ' + receiptPath)
  }
  if (record.ownedReleases !== undefined && (!Array.isArray(record.ownedReleases) ||
      !record.ownedReleases.every(isNativeReleaseIdentity) || !record.ownedReleases.some(item =>
        item.version === record.version && item.artifact === record.artifact && item.sha256 === record.sha256 && item.sourceSha === record.sourceSha))) {
    refuse('invalid_installation_record', '请核对安装记录中的版本归属。 / Check the release inventory in ' + receiptPath)
  }
  const installed = Object.fromEntries(['version', 'sourceSha', 'artifact', 'sha256', 'edition', 'scope', 'arch'].map(k => [k, record[k]]))
  installed.sourceSha = installed.sourceSha.toLowerCase()
  installed.sha256 = installed.sha256.toLowerCase()
  if (expectedRelease) Object.assign(installed, expectedRelease)
  const binary = path.join(paths.releases, installed.version, installed.artifact, 'my-api')
  const unexpected = () => refuse('native_service_mismatch', 'Check the running service, unit and current release. 请核对运行中的服务、单元文件和当前程序目录。')
  await checkNativeDirectory(path.dirname(binary), uid)
  const [state, unitFile, configFile] = await Promise.all([
    nativeServiceState(manager),
    managedFile(paths.unit, uid, false, 'native_service_mismatch'),
    managedFile(paths.config, uid, true, 'native_sqlite_mismatch'),
  ])
  const unitText = unitFile.text
  const config = configFile.text
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
  const pointer = path.dirname(paths.binary)
  const selectedLink = await lstat(pointer, { bigint: true }).catch(unexpected)
  if (!selectedLink.isSymbolicLink() || selectedLink.uid !== BigInt(uid) ||
      await readlink(pointer) !== path.relative(path.dirname(pointer), path.dirname(binary))) unexpected()
  const { database, id } = await runningDatabase(context, pid, config, serviceUid, record)
  const filesIdentity = createHash('sha256').update(JSON.stringify([receiptFile, configFile, unitFile])).digest('hex')
  const settingsIdentity = createHash('sha256').update(JSON.stringify([configFile, unitFile])).digest('hex')
  const identity = createHash('sha256').update(JSON.stringify([
    filesIdentity, state.InvocationID, pid, binary, String(selectedLink.ino), String(selectedLink.ctimeNs), String(metadata.dev), String(metadata.ino),
    String(metadata.ctimeNs), database, id,
  ])).digest('hex')
  return { installed, record, binary, database, databaseId: id, filesIdentity, settingsIdentity, invocationId: state.InvocationID,
    service: { name: unitName, scope, pid: Number(pid) }, identity }
}

async function signedDocuments(inputs) {
  const keys = ['current-manifest', 'current-bundle', 'manifest', 'bundle']
  const digests = await Promise.all(keys.map(async key =>
    createHash('sha256').update(await readDocument(inputs[key])).digest('hex')))
  return digests.join(':')
}

async function inspectTransition(inputs, context, verifyPackages, operation) {
  const before = await liveInstallation(inputs, context)
  const documents = await signedDocuments(inputs)
  const transition = await verifyPackages(inputs, before.installed, before.binary, operation)
  const placement = await inspectTargetRelease(context, before, transition.target, inputs.package)
  const after = await liveInstallation(inputs, context)
  if (after.identity !== before.identity || await signedDocuments(inputs) !== documents) {
    refuse('native_state_changed', 'The installation or signed inputs changed during verification. 请重新检查安装和签名文件。')
  }
  // SQLite contents can legitimately grow between plan and confirm. Its
  // path/inode are bound in identity; its latest contents are captured after stop.
  const bindings = ['native-confirm-v1', before.identity, transition, documents]
  if (placement.retained) bindings.push(placement.retained)
  const confirmation = createHash('sha256').update(JSON.stringify(bindings)).digest('hex')
  return { before, transition, confirmation, documents, placement }
}

export function nativeSQLite(action, context, inspection) {
  const { paths } = context
  const { before, confirmation } = inspection
  const [device, inode] = before.databaseId.split(':')
  const backup = path.join(paths.backups, confirmation)
  const request = { action, data: paths.data, database: before.database, device, inode, backup,
    config: paths.config, receipt: path.join(paths.state, 'installation.json') }
  return new Promise((resolve, reject) => {
    const child = execFile('/usr/bin/python3', ['-I', '-B', sqliteHelper], {
      env: { PATH: '/usr/bin:/bin', LC_ALL: 'C' }, timeout: 15 * 60_000, maxBuffer: 64 * 1024,
    }, (error, stdout) => {
      let reply
      try { reply = JSON.parse(stdout) } catch { /* No confirmed result. */ }
      if (!error && reply?.ok === true &&
          (action === 'preflight' ? reply.preflight === true : action === 'check-work' ? reply.workSettled === true : reply.backup === backup &&
            reply.backupVerified === true && reply.accountingVerified === true &&
            (action !== 'check-stopped' || reply.sourceUnchanged === true) &&
            (action !== 'check-live' || reply.historicalRowsVerified === true))) {
        resolve(reply)
        return
      }
      const code = /^[a-z_]{1,96}$/.test(reply?.code ?? '') ? reply.code : 'native_sqlite_check_unavailable'
      const reasons = {
        work_pending: '仍有运行中或结果待核对的任务。请先查看任务和账务记录。 / Review pending tasks and accounting before switching.',
        no_space: '备份与核验需要更多可用磁盘空间。 / More backup space is required.',
        backup_parent_not_private: '请核对备份目录的所有者和权限。目录应归安装管理员所有，权限为 0700。 / Check the backup directory owner and mode 0700.',
        extended_metadata_unsupported: '数据、配置或安装记录含当前备份格式无法保留的扩展属性。请保留现场并核对备份方式。 / The backup cannot preserve an extended attribute on the data, configuration or installation record.',
        acl_unavailable: '无法读全文件的访问控制列表。请核对数据目录和备份目录支持的权限类型。 / The file access controls could not be read in full.',
        acl_copy_unverified: '备份副本的权限与原文件不一致。请保留原数据并检查备份文件系统。 / Check whether the backup filesystem can preserve the original permissions.',
        submount_rejected: '数据目录内有另一个挂载点。请核对该挂载点的数据归属。 / Check the nested mount in the data directory.',
        metadata_invalid: '请核对配置、安装记录或备份清单的所有者、权限和文件类型。 / Check ownership, permissions and file type for configuration and backup records.',
        source_changed: '原始文件或数据库身份发生变化。请查看 transition-status。 / Inspect the current files and service.',
        accounting_changed: '历史账务与备份不一致，请保留最新数据库并查看 transition-status。 / Reconcile the historical records against the backup.',
        accounting_unknown: '有账务字段无法核对，请保留最新数据库并查看 transition-status。 / Historical accounting could not be verified.',
        backup_unverified: '备份缺少可靠校验结果，请查看 transition-status。 / Inspect the unverified backup.',
      }
      const reason = reasons[code] ?? 'SQLite 核验未通过。请检查数据权限和升级状态。 / Review SQLite permissions and transition-status.'
      reject(Object.assign(new Error(reason), { code }))
    })
    child.stdin.on('error', () => {}) // execFile reports early exit/spawn failure.
    child.stdin.end(JSON.stringify(request))
  })
}

export async function verifyNativeRetainedRelease(context, before, target) {
  const { paths, uid } = context
  await checkNativeDirectory(paths.releases, uid)
  const directory = path.join(paths.releases, target.version)
  const release = path.join(directory, target.artifact)
  const binary = path.join(release, 'my-api')
  const known = (before.record.ownedReleases ?? [before.installed]).filter(item => item.version === target.version)
  const conflict = () => refuse('native_release_conflict', '版本目录与受管记录不符，请核对其归属和内容。 / Check the program files and their installation record.')
  if (known.length !== 1 || known[0].artifact !== target.artifact ||
      known[0].sha256 !== target.sha256 || known[0].sourceSha !== target.sourceSha) conflict()
  await checkNativeDirectory(release, uid).catch(conflict)
  const identity = async () => {
    const files = await Promise.all([directory, release, binary].map(filename => lstat(filename, { bigint: true }))).catch(conflict)
    if (files.some(info => info.uid !== BigInt(uid) || (info.mode & 0o7022n)) ||
        !files[0].isDirectory() || !files[1].isDirectory() ||
        !files[2].isFile() || files[2].nlink !== 1n || !(files[2].mode & 0o100n)) conflict()
    return files.map(info => [info.dev, info.ino, info.ctimeNs].join(':')).join('/')
  }
  const retained = await identity()
  const [versions, entries] = await Promise.all([readdir(directory), readdir(release)]).catch(conflict)
  if (versions.length !== 1 || versions[0] !== target.artifact || entries.length !== 1 || entries[0] !== 'my-api') conflict()
  if (await binaryDigest(binary, before.record.arch) !== target.sha256) {
    refuse('digest_mismatch', '受管程序与已记录的摘要不符。 / The program does not match its recorded digest.')
  }
  if (await identity() !== retained) conflict()
  return { release, binary, retained }
}

async function inspectTargetRelease(context, before, target, sourcePackage) {
  const { paths, uid } = context
  await checkNativeDirectory(paths.releases, uid)
  const directory = path.join(paths.releases, target.version)
  if (await nativeEntry(directory)) return verifyNativeRetainedRelease(context, before, target)
  const [disk, source] = await Promise.all([statfs(paths.releases, { bigint: true }), lstat(sourcePackage, { bigint: true })])
  if (!source.isFile() || disk.bavail * disk.bsize < source.size) {
    refuse('native_stage_unavailable', '请检查程序文件和版本目录的剩余空间。 / Check the package and available staging space.')
  }
  const release = path.join(directory, target.artifact)
  return { release, binary: path.join(release, 'my-api'), retained: null }
}

export function requireSamePlan(actual, expected) {
  if (actual !== expected) refuse('native_confirmation_stale', '预检后现场或签名材料已变化，请重新生成本次操作的预检结果。 / Run the plan again for this installation.')
}

export async function planNativeTransition(inputs, verifyPackages = verifyNativeTransitionPackages, { operation = 'upgrade' } = {}) {
  const context = await nativeServiceContext(inputs.scope)
  await requireNoNativeUninstall(context)
  requireSettledTransition(await readNativeJournal(context))
  const inspection = await inspectTransition(inputs, context, verifyPackages, operation)
  const { before, transition, confirmation, placement } = inspection
  await nativeSQLite('preflight', context, inspection)
  requireSamePlan((await liveInstallation(inputs, context)).identity, before.identity)
  return {
    eligible: true, ...transition, confirmation, database: before.database, service: before.service,
    dataPolicy: 'keep_current_sqlite', reusesInstalledBinary: Boolean(placement.retained),
    steps: ['确认后排空并停止服务 / Drain and stop after confirmation',
      '备份并核验最新 SQLite / Back up and verify the latest SQLite data',
      '切换程序后核对服务和账务 / Switch the program and check service and accounting'],
  }
}

export async function assertStoppedInstallation(context, before, expectedBinary) {
  const { paths, uid, manager, unit } = context
  const [receipt, config, unitFile, state, database, pointer] = await Promise.all([
    managedFile(path.join(paths.state, 'installation.json'), uid, true, 'invalid_installation_record'),
    managedFile(paths.config, uid, true, 'native_configuration_changed'),
    managedFile(paths.unit, uid, false, 'native_service_mismatch'), nativeServiceState(manager),
    lstat(before.database, { bigint: true }), lstat(path.dirname(paths.binary)),
  ])
  if (unitFile.text !== unit || createHash('sha256').update(JSON.stringify([receipt, config, unitFile])).digest('hex') !== before.filesIdentity ||
      state.LoadState !== 'loaded' || state.FragmentPath !== paths.unit || state.DropInPaths !== '' || state.NeedDaemonReload !== 'no' ||
      state.ActiveState !== 'inactive' || state.MainPID !== '0' || state.ControlPID !== '0' ||
      (state.ExecMainPID !== '0' && state.ExecMainPID !== String(before.service.pid)) ||
      (state.InvocationID && !/^0{32}$/.test(state.InvocationID) && state.InvocationID !== before.invocationId) ||
      !database.isFile() || database.nlink !== 1n || database.dev + ':' + database.ino !== before.databaseId ||
      !pointer.isSymbolicLink() || pointer.uid !== uid || await realpath(paths.binary) !== expectedBinary ||
      await readlink(path.dirname(paths.binary)) !== path.relative(path.dirname(path.dirname(paths.binary)), path.dirname(expectedBinary))) {
    refuse('native_state_changed', '服务、指针、数据或配置已变化，请查看 transition-status。 / Inspect the current stopped installation.')
  }
}

export async function executeNativeTransition(inputs, verifyPackages = verifyNativeTransitionPackages, { operation = 'upgrade', onPhase } = {}) {
  if (!/^[0-9a-f]{64}$/.test(inputs.confirm ?? '')) {
    refuse('native_confirmation_required', '请将刚才预检的 confirmation 传给 --confirm。 / Supply the confirmation from the plan.')
  }
  const context = await nativeServiceContext(inputs.scope)
  const { paths, runtime, uid, manager } = context
  await requireNoNativeUninstall(context)
  requireSettledTransition(await readNativeJournal(context), inputs.confirm)
  const lock = path.join(runtime, 'myapi-native-install')
  await checkNativeDirectory(runtime, uid)
  try { await mkdir(lock, { mode: 0o700 }) } catch (error) {
    if (error.code === 'EEXIST') refuse('installation_busy', '已有安装操作持锁。请查看 transition-status。 / Another operation holds the installation lock.')
    throw error
  }
  let stagedDirectory
  let drain
  let recorded = false
  try {
    await requireNoNativeUninstall(context)
    const previous = await readNativeJournal(context)
    requireSettledTransition(previous, inputs.confirm)
    const inspection = await inspectTransition(inputs, context, verifyPackages, operation)
    const { before, transition, confirmation, placement } = inspection
    requireSamePlan(confirmation, inputs.confirm)
    const { release, binary: nextBinary, retained } = placement
    await waitForService(manager, paths, before.record.port, before.installed)
    let stagedInputs = { ...inputs, package: nextBinary }
    if (!retained) {
      stagedDirectory = await mkdtemp(path.join(paths.releases, '.native-stage-'))
      const staged = path.join(stagedDirectory, 'my-api')
      await stageBinary(inputs.package, staged)
      stagedInputs = { ...inputs, package: staged }
      const verified = await inspectTransition(stagedInputs, context, verifyPackages, operation)
      requireSamePlan(verified.confirmation, confirmation)
      await chmod(staged, 0o755)
      await syncNativePath(staged)
      await syncNativePath(stagedDirectory)
    }
    // Staging and backups can share a filesystem. Check remaining capacity
    // with the verified binary already on disk, while the old service is live.
    await nativeSQLite('preflight', context, inspection)
    await nativeSQLite('check-work', context, inspection)
    drain = await prepareNativeServiceStop(context, before.service)

    const backup = path.join(paths.backups, confirmation)
    const journal = { schema: 1, scope: context.scope, confirmation, operation: transition.operation,
      current: transition.current, target: transition.target, backup }
    const journalPath = path.join(paths.state, 'native-transition.json')
    const checkpoint = async phase => {
      await storeNativeRecord(journalPath, { ...journal, phase }, phase === 'prepared' && !previous)
      recorded = true
      if (onPhase) await onPhase(phase)
    }
    await checkpoint('prepared')
    await checkpoint('stop_requested')
    requireSamePlan((await liveInstallation(inputs, context)).identity, before.identity)
    if (retained) {
      requireSamePlan((await inspectTargetRelease(context, before, transition.target, inputs.package)).retained, retained)
    }
    if ((await drain.stop())?.drained !== true) refuse('native_stop_unconfirmed', '无法确认服务已排空。 / Service drain is unconfirmed.')
    await checkpoint('stopped')
    await assertStoppedInstallation(context, before, before.binary)

    await checkpoint('backup_requested')
    await nativeSQLite('backup', context, inspection)
    await checkpoint('backed_up')
    // Verify the bytes which will be started and both original signed
    // declarations once more before changing any live program pointer.
    const checked = await verifyPackages(stagedInputs, before.installed, before.binary, transition.operation)
    requireSamePlan(JSON.stringify(checked), JSON.stringify(transition))
    requireSamePlan(await signedDocuments(inputs), inspection.documents)
    await nativeSQLite('check-stopped', context, inspection)
    await assertStoppedInstallation(context, before, before.binary)
    await checkpoint('switch_requested')
    if (stagedDirectory) {
      const versionDirectory = path.dirname(release)
      await mkdir(versionDirectory, { mode: 0o755 })
      await chmod(versionDirectory, 0o755)
      await chmod(stagedDirectory, 0o755)
      await rename(stagedDirectory, release)
      stagedDirectory = null
      await syncNativePath(versionDirectory)
      await syncNativePath(paths.releases)
    }
    const pointer = path.dirname(paths.binary)
    const base = path.dirname(pointer)
    const next = path.join(base, '.native-current-' + randomUUID())
    await symlink(path.relative(base, release), next)
    try {
      if (retained) {
        requireSamePlan((await inspectTargetRelease(context, before, transition.target, inputs.package)).retained, retained)
      }
      await assertStoppedInstallation(context, before, before.binary)
      await rename(next, pointer)
    } finally {
      await unlink(next).catch(error => { if (error.code !== 'ENOENT') throw error })
    }
    await syncNativePath(base)
    await checkpoint('switched')
    await assertStoppedInstallation(context, before, nextBinary)
    if (await binaryDigest(nextBinary, before.record.arch) !== transition.target.sha256) {
      refuse('digest_mismatch', '启动前程序摘要不符。 / Installed binary digest changed before start.')
    }
    await checkpoint('start_requested')
    await manager(['start', unitName])
    await waitForService(manager, paths, before.record.port, { ...before.installed, ...transition.target })
    await checkpoint('started')
    const current = await liveInstallation(inputs, context, transition.target)
    if (current.databaseId !== before.databaseId || current.filesIdentity !== before.filesIdentity) {
      refuse('native_state_changed', '启动后数据或配置身份不符。 / Check the running database and configuration.')
    }
    await nativeSQLite('check-live', context, inspection)
    requireSamePlan((await liveInstallation(inputs, context, transition.target)).identity, current.identity)
    // Keep signed release identities for future provenance checks. This
    // inventory is not permission to remove any data or unrecognized file.
    const known = Array.isArray(before.record.ownedReleases) ? before.record.ownedReleases : []
    const ownedReleases = [...known.filter(item => ![transition.current, transition.target].some(release =>
      item.version === release.version && item.artifact === release.artifact)),
      transition.current, transition.target]
    await checkpoint('receipt_requested')
    await storeNativeRecord(path.join(paths.state, 'installation.json'), { ...before.record, ...transition.target, ownedReleases })
    const committed = await liveInstallation(inputs, context)
    if (committed.databaseId !== before.databaseId || committed.settingsIdentity !== before.settingsIdentity || committed.service.pid !== current.service.pid ||
        committed.installed.sha256 !== transition.target.sha256) {
      refuse('native_state_changed', '核对新安装记录时现场变化。 / Inspect the installation record.')
    }
    await checkpoint('committed')
    return { status: 'healthy', ...transition, backup, database: before.database, accountingVerified: true }
  } finally {
    try {
      if (drain) await drain.close()
    } finally {
      try {
        if (stagedDirectory && !recorded) {
          await unlink(path.join(stagedDirectory, 'my-api')).catch(error => { if (error.code !== 'ENOENT') throw error })
          await rmdir(stagedDirectory)
        }
      } finally { await rmdir(lock) }
    }
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  Promise.resolve().then(async () => {
    const [action, ...args] = process.argv.slice(2)
    if (action === 'transition-status') {
      if (args.length !== 2 || args[0] !== '--scope') refuse('invalid_arguments', 'Supply --scope system or --scope user.')
      return readNativeTransitionStatus(args[1])
    }
    if (!['upgrade-plan', 'upgrade', 'rollback-plan', 'rollback'].includes(action)) {
      refuse('invalid_arguments', 'Select upgrade-plan, upgrade, rollback-plan, rollback or transition-status.')
    }
    const operation = action.startsWith('rollback') ? 'rollback' : 'upgrade'
    const planning = action.endsWith('-plan')
    const extra = ['current-manifest', 'current-bundle']
    const inputs = parseNativePackageArgs(args, planning ? extra : [...extra, 'confirm'])
    if (extra.some(key => !inputs[key])) refuse('invalid_arguments', 'Supply both --current-manifest and --current-bundle.')
    return planning ? planNativeTransition(inputs, undefined, { operation }) : executeNativeTransition(inputs, undefined, { operation })
  }).then(result => console.log(JSON.stringify(result))).catch(error => {
    const code = /^[a-z_]+$/.test(error.code ?? '') ? error.code : 'inspection_failed'
    const reason = code === 'inspection_failed' ? '请核对文件和 systemd，再用 transition-status 查看实际进度。 / Check files, systemd and transition-status.' : error.message
    console.error(JSON.stringify({ eligible: false, code, reason }))
    process.exitCode = 2
  })
}
