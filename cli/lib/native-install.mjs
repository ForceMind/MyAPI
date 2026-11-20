import { execFile } from 'node:child_process'
import { randomBytes } from 'node:crypto'
import { constants, createWriteStream } from 'node:fs'
import { chmod, chown, copyFile, lstat, mkdir, mkdtemp, open, rename, rm, rmdir, stat, symlink, writeFile } from 'node:fs/promises'
import { constants as osConstants } from 'node:os'
import path from 'node:path'
import { pipeline } from 'node:stream/promises'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { promisify } from 'node:util'
import { parseNativePackageArgs, verifyReleaseNativePackage } from './native-package-verification.mjs'

const exec = promisify(execFile)
const shellEntry = fileURLToPath(new URL('../../deploy/install-native.sh', import.meta.url))
const unitName = 'my-api.service'
const commandEnv = { PATH: '/usr/sbin:/usr/bin:/sbin:/bin', LC_ALL: 'C' }
const pathKeys = ['binary', 'releases', 'config', 'data', 'state', 'backups', 'logs', 'cache', 'unit']

function refuse(code, message) {
  throw Object.assign(new Error(message), { code })
}

// Report only known categories. Child-process messages can contain command
// arguments/environment; filesystem messages can contain private paths.
function installationDiagnostic(error, stage, paths) {
  const applicationCodes = new Set(['unsafe_directory', 'account_unconfirmed', 'service_conflict',
    'service_failed', 'health_timeout', 'unexpected_service_state', 'ERR_CHILD_PROCESS_STDIO_MAXBUFFER'])
  const code = Object.hasOwn(osConstants.errno, error?.code) || applicationCodes.has(error?.code) ? error.code : 'unknown'
  const exit = Number.isInteger(error?.code) && error.code >= 0 && error.code <= 255 ? ` exit=${error.code}` : ''
  const killed = error?.killed === true ? ' killed=true' : ''
  const syscalls = new Set(['access', 'chmod', 'chown', 'copyfile', 'lstat', 'mkdir', 'open', 'read',
    'rename', 'rmdir', 'stat', 'symlink', 'unlink', 'write'])
  const actions = { account: 'useradd_or_getent', service_registration: 'systemctl_reload_or_show',
    service_start: 'systemctl_start', health: 'verify_health', failure_stop: 'systemctl_stop',
    failure_stop_check: 'systemctl_show' }
  const action = syscalls.has(error?.syscall) ? error.syscall : actions[stage] ?? 'unknown'
  let role = 'unknown'
  const managedRoles = [
    [paths.config, 'configuration'], [paths.unit, 'service_unit'],
    [path.join(paths.state, 'installation.pending.json'), 'installation_record'],
    [path.join(paths.state, 'installation.json'), 'installation_record'],
    [paths.data, 'data'], [paths.backups, 'backups'], [paths.state, 'state'],
    [paths.logs, 'logs'], [paths.cache, 'cache'], [path.dirname(paths.releases), 'program'],
    [path.dirname(paths.config), 'configuration'], [path.dirname(paths.state), 'state'],
  ]
  for (const filename of [error?.dest, error?.path]) {
    if (typeof filename !== 'string') continue
    const match = managedRoles.find(([managed]) => filename === managed || filename.startsWith(managed + '/'))
    if (match) { role = match[1]; break }
  }
  return `stage=${stage} action=${action} role=${role} cause=${code}${exit}${killed}`
}

async function inspect(filename) {
  try {
    return await lstat(filename)
  } catch (error) {
    if (error.code !== 'ENOENT') throw error
    return null
  }
}

// Check each component without following links. An untrusted writer must
// not be able to replace a parent between preflight and exclusive creation.
export async function checkNativeDirectory(directory, uid, create = false) {
  if (directory === '/') return
  await checkNativeDirectory(path.dirname(directory), uid, create)
  let entry = await inspect(directory)
  if (!entry && create) {
    try {
      await mkdir(directory, { mode: 0o700 })
    } catch (error) {
      if (error.code !== 'EEXIST') throw error
    }
    entry = await inspect(directory)
  }
  if (entry && (!entry.isDirectory() || ![0, uid].includes(entry.uid) || (entry.mode & 0o022))) {
    refuse('unsafe_directory', `Unsafe directory or symbolic link at ${directory}.`)
  }
}

async function layout(scope) {
  const env = { ...commandEnv }
  for (const key of ['HOME', 'XDG_DATA_HOME', 'XDG_CONFIG_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME']) {
    if (process.env[key] !== undefined) env[key] = process.env[key]
  }
  const { stdout } = await exec('/bin/bash', [shellEntry, 'plan', '--scope', scope], { env })
  const paths = {}
  for (const key of pathKeys) {
    const values = stdout.split('\n').filter(line => line.startsWith(`${key}: /`))
    if (values.length !== 1) refuse('invalid_layout', 'Could not read the native installation paths.')
    paths[key] = path.normalize(values[0].slice(key.length + 2))
  }
  const roots = scope === 'system'
    ? [path.dirname(paths.releases), path.dirname(paths.config), path.dirname(paths.data), paths.logs, paths.cache]
    : [path.dirname(paths.releases), path.dirname(paths.config), path.dirname(paths.state), paths.cache]
  if (paths.data.includes('?')) refuse('invalid_layout', 'The SQLite data path cannot contain a question mark.')
  for (const [index, root] of roots.entries()) {
    if (roots.some((other, position) => position !== index && (root === other || root.startsWith(other + '/'))) ||
      paths.unit.startsWith(root + '/')) {
      refuse('invalid_layout', 'Installation roots and the service file must have distinct locations.')
    }
  }
  const { stdout: unit } = await exec('/bin/bash', [shellEntry, 'render-unit', '--scope', scope], { env })
  return { paths, roots, unit }
}

export async function stageBinary(source, target) {
  const file = await open(source, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK)
  try {
    const info = await file.stat()
    if (!info.isFile() || info.size < 64 || !Number.isSafeInteger(info.size)) {
      refuse('invalid_binary', 'Select a regular Linux binary.')
    }
    await pipeline(file.createReadStream({ autoClose: false, start: 0, end: info.size - 1 }),
      createWriteStream(target, { flags: 'wx', mode: 0o600 }))
  } finally {
    await file.close()
  }
}

export async function nativeAccountEntry(database) {
  try {
    const { stdout } = await exec('getent', [database, 'my-api'], { env: commandEnv, timeout: 5_000 })
    return stdout.trim()
  } catch (error) {
    if (error.code === 2) return null
    throw error
  }
}

export async function nativeServiceState(manager) {
  const args = ['show', unitName, '--property=LoadState,ActiveState,SubState,MainPID,ControlPID,FragmentPath,DropInPaths,NeedDaemonReload,InvocationID,Result,ExecMainPID,ExecMainCode,ExecMainStatus,ExecMainStartTimestampMonotonic']
  let stdout
  try {
    ({ stdout } = await manager(args))
  } catch (error) {
    if (error.code !== 1 || !/^LoadState=not-found$/m.test(error.stdout || '')) throw error
    stdout = error.stdout
  }
  return Object.fromEntries(stdout.trimEnd().split('\n').map(line => {
    const delimiter = line.indexOf('=')
    return [line.slice(0, delimiter), line.slice(delimiter + 1)]
  }))
}

async function assertFresh({ roots, paths, manager, scope, uid, port }) {
  for (const target of [...roots, paths.unit, paths.unit + '.d']) {
    await checkNativeDirectory(path.dirname(target), uid)
    if (await inspect(target)) refuse('installation_conflict', `Path already exists at ${target}.`)
  }
  const state = await nativeServiceState(manager)
  if (state.LoadState !== 'not-found' || state.DropInPaths !== '' || state.FragmentPath !== '' ||
    state.ActiveState !== 'inactive' || state.MainPID !== '0') {
    refuse('service_conflict', 'systemd already knows a MyAPI service or override.')
  }
  if (scope === 'system' && (await nativeAccountEntry('passwd') || await nativeAccountEntry('group'))) {
    refuse('account_conflict', 'The dedicated my-api user or group already exists.')
  }
  const { stdout } = await exec('ss', ['-H', '-ltn', 'sport', '=', `:${port}`], { env: commandEnv, timeout: 5_000 })
  if (stdout.trim()) refuse('port_conflict', `TCP port ${port} is already in use.`)
}

export async function waitForService(manager, paths, port, receipt) {
  const installed = await stat(paths.binary)
  const deadline = Date.now() + 60_000
  while (Date.now() < deadline) {
    const state = await nativeServiceState(manager)
    if (state.ActiveState === 'failed') refuse('service_failed', 'The new MyAPI service failed to start.')
    const pid = state.MainPID
    if (state.ActiveState === 'active' && /^[1-9][0-9]*$/.test(pid)) {
      const executable = await stat(`/proc/${pid}/exe`).catch(error => {
        if (error.code !== 'ENOENT' && error.code !== 'ESRCH') throw error
      })
      const { stdout } = await exec('ss', ['-H', '-ltnp', 'sport', '=', `:${port}`], {
        env: commandEnv, timeout: 5_000,
      })
      if (executable?.ino === installed.ino && executable.dev === installed.dev &&
        stdout.split('\n').some(line => line.includes(`pid=${pid},`) && line.split(/\s+/)[3] === `127.0.0.1:${port}`)) {
        const status = await fetch(`http://127.0.0.1:${port}/api/status`, {
          redirect: 'error', signal: AbortSignal.timeout(2_000),
        }).then(response => response.ok ? response.json() : null).catch(() => null)
        const current = await nativeServiceState(manager)
        if (current.ActiveState === 'active' && current.MainPID === pid && status?.success === true &&
          status.data?.product_edition === receipt.edition && status.data?.version === receipt.version) return
      }
    }
    await delay(500)
  }
  refuse('health_timeout', 'The new process did not report the expected MyAPI version and edition.')
}

export async function nativeServiceContext(scope) {
  const uid = process.getuid?.()
  if (process.platform !== 'linux' || uid === undefined || uid !== process.geteuid() ||
    (scope !== 'system' && scope !== 'user') || (scope === 'system' ? uid !== 0 : uid === 0)) {
    refuse('invalid_scope', 'Run a system service operation as root or a user service operation as the intended user.')
  }
  const { paths, roots, unit } = await layout(scope)
  const runtime = scope === 'system' ? '/run' : `/run/user/${uid}`
  const env = scope === 'system' ? commandEnv : { ...commandEnv, XDG_RUNTIME_DIR: runtime, DBUS_SESSION_BUS_ADDRESS: `unix:path=${runtime}/bus` }
  const manager = (args, timeout = 20_000) => exec('systemctl', [...(scope === 'user' ? ['--user'] : []), ...args], { env, timeout })
  return { paths, roots, unit, manager, scope, uid, runtime }
}

// The command uses the fixed verifier. Integration tests supply a verifier
// for their signed fixture while exercising the real filesystem and manager.
export async function installNative(inputs, verify = verifyReleaseNativePackage) {
  const context = await nativeServiceContext(inputs.scope)
  const { paths, roots, unit, manager, scope, uid, runtime } = context
  const port = inputs.port ?? '3000'
  if (!/^[1-9][0-9]{3,4}$/.test(port) || Number(port) < 1024 || Number(port) > 65535) {
    refuse('invalid_port', 'Choose a TCP port from 1024 to 65535.')
  }
  const lock = path.join(runtime, 'myapi-native-install')
  const installContext = { ...context, port }
  await assertFresh(installContext)

  const scratch = await mkdtemp('/tmp/myapi-native-install-')
  try {
    const staged = path.join(scratch, 'my-api')
    await stageBinary(inputs.package, staged)
    const receipt = await verify({ ...inputs, package: staged })
    await checkNativeDirectory(runtime, uid)
    try {
      await mkdir(lock, { mode: 0o700 })
    } catch (error) {
      if (error.code === 'EEXIST') refuse('installation_busy', 'Another install holds the lock. Check it before retrying.')
      throw error
    }
    try {
      await assertFresh(installContext)
      let startIssued = false
      let writesStarted = false
      let stage = 'directories'
      try {
        for (const root of roots) {
          await checkNativeDirectory(path.dirname(root), uid, true)
          await mkdir(root, { mode: 0o700 })
          writesStarted = true
        }
        if (scope === 'system') {
          await chmod(path.dirname(paths.releases), 0o755)
          await chmod(path.dirname(paths.data), 0o755)
        }
        for (const directory of [paths.releases, paths.data, paths.state, paths.backups, paths.logs, paths.cache]) {
          if (!roots.includes(directory)) await mkdir(directory, { mode: 0o700 })
        }
        if (scope === 'system') {
          stage = 'account'
          await exec('useradd', ['--system', '--user-group', '--no-create-home', '--home-dir', path.dirname(paths.data),
            '--shell', '/usr/sbin/nologin', 'my-api'], { env: commandEnv, timeout: 10_000 })
          const account = (await nativeAccountEntry('passwd'))?.split(':')
          const user = Number(account?.[2])
          const group = Number(account?.[3])
          if (!Number.isSafeInteger(user) || user <= 0 || !Number.isSafeInteger(group) || group <= 0) {
            refuse('account_unconfirmed', 'Could not confirm the dedicated service user.')
          }
          for (const directory of [paths.data, paths.logs, paths.cache]) await chown(directory, user, group)
        }

        stage = 'program'
        const versionDirectory = path.join(paths.releases, receipt.version)
        const release = path.join(versionDirectory, receipt.artifact)
        for (const directory of [versionDirectory, release]) await mkdir(directory, { mode: 0o700 })
        for (const directory of [paths.releases, versionDirectory, release]) await chmod(directory, 0o755)
        const binary = path.join(release, 'my-api')
        await copyFile(staged, binary, constants.COPYFILE_EXCL)
        await chmod(binary, 0o755)
        await symlink(path.relative(path.dirname(paths.releases), release), path.dirname(paths.binary), 'dir')
        // A new SQLite service must not inherit a manager's database or Redis
        // connection. In particular, systemd user services inherit its env.
        const config = [
          `MYAPI_EDITION=${receipt.edition}`,
          'MYAPI_BIND_ADDRESS=127.0.0.1',
          `PORT=${port}`,
          `SESSION_SECRET=${randomBytes(48).toString('hex')}`,
          'CRYPTO_SECRET=',
          'SQL_DSN=',
          'LOG_SQL_DSN=',
          'REDIS_CONN_STRING=',
          'NODE_TYPE=master',
          'BATCH_UPDATE_ENABLED=false',
          '',
        ]
        stage = 'configuration_record'
        await writeFile(paths.config, config.join('\n'), { flag: 'wx', mode: 0o600 })
        const pending = path.join(paths.state, 'installation.pending.json')
        const installed = { ...receipt, port: Number(port), binary: paths.binary, config: paths.config, unit: paths.unit }
        await writeFile(pending, JSON.stringify(installed) + '\n', { flag: 'wx', mode: 0o600 })
        stage = 'service_registration'
        await checkNativeDirectory(path.dirname(paths.unit), uid, true)
        await writeFile(paths.unit, unit, { flag: 'wx', mode: 0o644 })
        await manager(['daemon-reload'])
        const state = await nativeServiceState(manager)
        if (state.LoadState !== 'loaded' || state.FragmentPath !== paths.unit || state.DropInPaths !== '' ||
          state.ActiveState !== 'inactive' || state.MainPID !== '0') {
          refuse('service_conflict', 'systemd did not load the newly written unit on its own.')
        }
        startIssued = true
        stage = 'service_start'
        await manager(['start', unitName])
        stage = 'health'
        await waitForService(manager, paths, port, receipt)
        stage = 'confirmation_record'
        await rename(pending, path.join(paths.state, 'installation.json'))
        return { ...installed, status: 'healthy', url: `http://127.0.0.1:${port}` }
      } catch (error) {
        const diagnostic = installationDiagnostic(error, stage, paths)
        if (startIssued) {
          let stopStage = 'failure_stop'
          let stopError
          const stopped = await manager(['stop', unitName]).then(() => {
            stopStage = 'failure_stop_check'
            return nativeServiceState(manager)
          }).catch(failure => {
            stopError = failure
            return null
          })
          if (!stopped || !['inactive', 'failed'].includes(stopped.ActiveState) || stopped.MainPID !== '0') {
            const stopDiagnostic = installationDiagnostic(stopError ?? { code: 'unexpected_service_state' }, stopStage, paths)
            refuse('stop_unconfirmed', `请保留安装现场，先确认服务是否已停。 / Service stop is unconfirmed. Installation (${diagnostic}). Stop check (${stopDiagnostic}).`)
          }
        }
        if (writesStarted) {
          refuse('installation_incomplete', `安装中断，已保留新文件。请按失败阶段检查文件或服务。 / Installation stopped with new files preserved. ${diagnostic}`)
        }
        throw error
      }
    } finally {
      await rmdir(lock)
    }
  } finally {
    await rm(scratch, { recursive: true })
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  Promise.resolve().then(async () => {
    const result = await installNative(parseNativePackageArgs(process.argv.slice(2), ['port']))
    console.log(JSON.stringify(result))
  }).catch(error => {
    const code = typeof error.code === 'string' ? error.code : 'installation_failed'
    const reason = /^[a-z_]+$/.test(code) ? error.message : 'Check file permissions and systemd availability.'
    console.error(`MyAPI installation failed [${code}]. ${reason}`)
    process.exitCode = 2
  })
}
