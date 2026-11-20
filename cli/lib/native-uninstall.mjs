import { createHash } from 'node:crypto'
import { lstat, mkdir, readdir, readFile, readlink, realpath, rmdir, unlink } from 'node:fs/promises'
import path from 'node:path'
import { pathToFileURL } from 'node:url'
import { checkNativeDirectory, nativeServiceContext, nativeServiceState } from './native-install.mjs'
import { prepareNativeServiceStop } from './native-service-stop.mjs'
import { assertStoppedInstallation, liveInstallation, nativeSQLite, requireSamePlan, verifyNativeRetainedRelease } from './native-transition.mjs'
import { nativeEntry, readNativeJournal, refuseNative as refuse, requireNoNativeUninstall, requireSettledTransition, storeNativeRecord, syncNativePath } from './native-transition-record.mjs'

const serviceMismatch = () => refuse('native_service_mismatch',
  '无法确认 systemd 服务归属，请核对单元文件及开机启动设置。 / The systemd unit or its enablement could not be verified.')
const releaseConflict = () => refuse('native_release_conflict',
  '程序目录或保留路径与安装记录不符，请先核对现场。 / Verify the managed program and preservation paths.')

function fileStamp(info) {
  return [info.dev, info.ino, info.mode, info.uid, info.gid, info.nlink, info.ctimeNs].map(String)
}

async function assertNoRemovalMounts(paths, targets) {
  const content = await readFile('/proc/self/mountinfo', 'utf8').catch(releaseConflict)
  const lines = content.trim().split('\n')
  for (const line of lines) {
    const mount = line.split(' ')[4]
    if (!mount?.startsWith('/') || !line.includes(' - ')) releaseConflict()
    const location = mount.replace(/\\([0-7]{3})/g, (_, octal) => String.fromCharCode(Number.parseInt(octal, 8)))
    if (targets.includes(location) || location.startsWith(paths.releases + '/')) releaseConflict()
  }
}

async function inspectRemoval(context, before) {
  const { paths, scope, uid, manager } = context
  const current = Object.fromEntries(['version', 'artifact', 'sha256', 'sourceSha'].map(key => [key, before.installed[key]]))
  const releases = [...(before.record.ownedReleases ?? [current])].sort((a, b) => a.version.localeCompare(b.version))
  const names = releases.map(item => item.version).sort()
  if (new Set(names).size !== names.length || JSON.stringify((await readdir(paths.releases).catch(releaseConflict)).sort()) !== JSON.stringify(names)) {
    releaseConflict()
  }
  const programs = []
  for (const release of releases) {
    const owned = await verifyNativeRetainedRelease(context, before, release)
    programs.push([owned.binary, false], [owned.release, true], [path.dirname(owned.release), true])
  }

  const service = await nativeServiceState(manager)
  const target = scope === 'system' ? 'multi-user.target.wants' : 'default.target.wants'
  const enableLink = path.join(path.dirname(paths.unit), target, path.basename(paths.unit))
  await checkNativeDirectory(path.dirname(enableLink), uid).catch(serviceMismatch)
  const enabled = await nativeEntry(enableLink)
  if (enabled) {
    if (service.UnitFileState !== 'enabled' || !enabled.isSymbolicLink() || enabled.uid !== uid ||
        path.resolve(path.dirname(enableLink), await readlink(enableLink)) !== paths.unit ||
        await realpath(enableLink).catch(serviceMismatch) !== paths.unit) serviceMismatch()
  } else if (service.UnitFileState !== 'disabled') {
    serviceMismatch()
  }
  if (service.LoadState !== 'loaded' || service.FragmentPath !== paths.unit || service.DropInPaths !== '' ||
      service.NeedDaemonReload !== 'no' || await nativeEntry(paths.unit + '.d')) serviceMismatch()

  const pointer = path.dirname(paths.binary)
  const systemdFiles = [...(enabled ? [[enableLink, false]] : []), [paths.unit, false]]
  await assertNoRemovalMounts(paths, [paths.releases, pointer, paths.unit, enableLink])
  const removal = await Promise.all([
    ...systemdFiles, [pointer, false],
    ...programs, [paths.releases, true],
  ].map(async ([filename, directory]) => {
    const info = await lstat(filename, { bigint: true }).catch(releaseConflict)
    if (info.uid !== BigInt(uid) || (directory ? !info.isDirectory() : info.isDirectory())) releaseConflict()
    return { filename, directory, stamp: fileStamp(info) }
  }))
  const preserve = [paths.config, paths.data, paths.logs, paths.backups, paths.state, paths.cache]
  const preservation = await Promise.all(preserve.map(async filename => {
    const info = await lstat(filename, { bigint: true }).catch(releaseConflict)
    if (filename === paths.config ? !info.isFile() : !info.isDirectory()) releaseConflict()
    return [filename, String(info.dev), String(info.ino)]
  }))
  // Independently bind the release tree, the optional boot link, and the
  // root identities of the paths the administrator expects to keep.
  return { current, removal, preserve, preservation, systemdFiles: systemdFiles.length }
}

async function inspectUninstall(context) {
  const before = await liveInstallation({}, context)
  const effects = await inspectRemoval(context, before)
  requireSamePlan((await liveInstallation({}, context)).identity, before.identity)
  const confirmation = createHash('sha256').update(JSON.stringify(['native-uninstall-v1', before.identity, effects])).digest('hex')
  return { before, effects, confirmation }
}

function describeUninstall(inspection) {
  const { before, effects, confirmation } = inspection
  return {
    eligible: true, operation: 'uninstall', confirmation, service: before.service, database: before.database,
    remove: effects.removal.map(entry => entry.filename),
    preserve: effects.preserve,
  }
}

export async function planNativeUninstall({ scope }) {
  const context = await nativeServiceContext(scope)
  await requireNoNativeUninstall(context)
  requireSettledTransition(await readNativeJournal(context))
  return describeUninstall(await inspectUninstall(context))
}

export async function executeNativeUninstall({ scope, confirm }, { onPhase, prepareDrain = prepareNativeServiceStop } = {}) {
  if (!/^[a-f0-9]{64}$/.test(confirm ?? '')) {
    refuse('native_confirmation_required', '请提供卸载预检的 confirmation。 / Supply the confirmation from uninstall-plan.')
  }
  const context = await nativeServiceContext(scope)
  const { paths, uid, manager, runtime } = context
  await requireNoNativeUninstall(context, confirm)
  requireSettledTransition(await readNativeJournal(context), confirm)
  await checkNativeDirectory(runtime, uid)
  const lock = path.join(runtime, 'myapi-native-install')
  try { await mkdir(lock, { mode: 0o700 }) } catch (error) {
    if (error.code === 'EEXIST') refuse('installation_busy', '已有原生服务操作持锁。 / Another native operation holds the installation lock.')
    throw error
  }
  let drain
  try {
    await requireNoNativeUninstall(context, confirm)
    requireSettledTransition(await readNativeJournal(context), confirm)
    const inspection = await inspectUninstall(context)
    const { before, effects, confirmation } = inspection
    const plan = describeUninstall(inspection)
    requireSamePlan(confirmation, confirm)
    await nativeSQLite('check-work', context, inspection)
    drain = await prepareDrain(context, before.service)
    const filename = path.join(paths.state, 'native-uninstall.json')
    const checkpoint = async phase => {
      await storeNativeRecord(filename, { schema: 1, scope, operation: 'uninstall', phase, confirmation,
        current: effects.current, remove: plan.remove, preserve: plan.preserve }, phase === 'prepared')
      if (onPhase) await onPhase(phase)
    }
    await checkpoint('prepared')
    await checkpoint('stop_requested')
    requireSamePlan((await inspectUninstall(context)).confirmation, confirmation)
    if ((await drain.stop())?.drained !== true) {
      refuse('native_stop_unconfirmed', '无法确认服务已排空，请查看 transition-status。 / Service drain could not be confirmed.')
    }
    await checkpoint('stopped')
    await assertStoppedInstallation(context, before, before.binary)
    await checkpoint('remove_requested')
    requireSamePlan(JSON.stringify(await inspectRemoval(context, before)), JSON.stringify(effects))
    await assertStoppedInstallation(context, before, before.binary)
    for (const [index, entry] of effects.removal.entries()) {
      if (index === effects.systemdFiles) {
        await drain.close()
        drain = null
        // The manager can stop using the old unit before its executable goes away.
        await manager(['daemon-reload'])
        const service = await nativeServiceState(manager)
        if (service.LoadState !== 'not-found' || service.MainPID !== '0' || service.ActiveState !== 'inactive') serviceMismatch()
      }
      const actual = fileStamp(await lstat(entry.filename, { bigint: true }).catch(releaseConflict))
      // Removing our own children changes a directory's ctime and link count.
      const count = entry.directory ? 5 : actual.length
      if (actual.slice(0, count).some((value, index) => value !== entry.stamp[index])) releaseConflict()
      if (entry.directory) await rmdir(entry.filename)
      else await unlink(entry.filename)
      await syncNativePath(path.dirname(entry.filename))
    }
    const service = await nativeServiceState(manager)
    if (service.LoadState !== 'not-found' || service.MainPID !== '0' || service.ActiveState !== 'inactive') serviceMismatch()
    for (const [preserved, dev, ino] of effects.preservation) {
      const info = await lstat(preserved, { bigint: true }).catch(releaseConflict)
      if (String(info.dev) !== dev || String(info.ino) !== ino) releaseConflict()
    }
    const database = await lstat(before.database, { bigint: true }).catch(releaseConflict)
    if (!database.isFile() || database.dev + ':' + database.ino !== before.databaseId) releaseConflict()
    await checkpoint('uninstalled')
    return { status: 'uninstalled', operation: 'uninstall', database: before.database, remove: plan.remove, preserve: plan.preserve }
  } finally {
    try { if (drain) await drain.close() } finally { await rmdir(lock) }
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  Promise.resolve().then(() => {
    const [action, scopeFlag, scope, confirmFlag, confirm, ...rest] = process.argv.slice(2)
    if (scopeFlag !== '--scope' || !['system', 'user'].includes(scope) || rest.length ||
        (action === 'uninstall-plan' ? confirmFlag !== undefined :
          action !== 'uninstall' || (confirmFlag !== undefined && (confirmFlag !== '--confirm' || confirm === undefined)))) {
      refuse('invalid_arguments', 'Use uninstall-plan --scope system|user or uninstall --scope system|user --confirm CONFIRMATION.')
    }
    return action === 'uninstall-plan' ? planNativeUninstall({ scope }) : executeNativeUninstall({ scope, confirm })
  }).then(result => console.log(JSON.stringify(result))).catch(error => {
    const code = /^[a-z_]+$/.test(error.code ?? '') ? error.code : 'uninstall_inspection_failed'
    const reason = code === 'uninstall_inspection_failed'
      ? '请先查看 transition-status，然后核对实际服务、程序和保留路径。 / Inspect transition-status, systemd and the managed paths.'
      : error.message
    console.error(JSON.stringify({ eligible: false, code, reason }))
    process.exitCode = 2
  })
}
