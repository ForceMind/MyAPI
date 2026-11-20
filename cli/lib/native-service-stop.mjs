import { spawn } from 'node:child_process'
import { constants } from 'node:fs'
import { lstat, open, readFile, readdir, readlink, stat } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { nativeServiceState } from './native-install.mjs'

const readFlags = constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK
const maxEvidenceBytes = 4 * 1024 * 1024
const message = 'Service drain could not be confirmed. Preserve the installation and inspect the service locally; do not retry, replace or restart it. 无法确认服务正常排空；请保留现场并在本机检查，勿重试、替换或重启。'

function refuse() {
  throw Object.assign(new Error(message), { code: 'native_stop_unconfirmed' })
}

/** Keep a sender-bound RefUnit alive; the injectable process boundary is used by protocol tests. */
export async function openNativeServiceReference(context, spawnReference = spawn) {
  if (!['system', 'user'].includes(context.scope) || !Number.isSafeInteger(context.uid) ||
      (context.scope === 'system' ? context.uid !== 0 : context.uid <= 0)) refuse()
  const helper = fileURLToPath(new URL('./native-service-ref.py', import.meta.url))
  const env = { PATH: '/usr/sbin:/usr/bin:/sbin:/bin', LC_ALL: 'C' }
  if (context.scope === 'user') {
    if (!Number.isSafeInteger(context.uid) || context.uid <= 0 || context.runtime !== '/run/user/' + context.uid) refuse()
    env.XDG_RUNTIME_DIR = context.runtime
    env.DBUS_SESSION_BUS_ADDRESS = 'unix:path=' + context.runtime + '/bus'
  }
  let child
  try {
    child = spawnReference('/usr/bin/python3', ['-I', '-B', helper, context.scope], {
      env, cwd: path.dirname(helper), stdio: ['pipe', 'pipe', 'ignore'], windowsHide: true,
    })
  } catch { refuse() }
  let broken = false
  let ended = false
  let pending = null
  let output = ''
  let outputBytes = 0
  let closePromise
  const fail = () => {
    broken = true
    child.stdin.end()
    if (pending) {
      pending.reject(Object.assign(new Error(message), { code: 'native_stop_unconfirmed' }))
      pending = null
    }
  }
  const completion = new Promise(resolve => {
    child.once('error', fail)
    child.stdin.on('error', fail)
    child.stdout.on('error', fail)
    child.once('close', (code, signal) => {
      ended = true
      if (code !== 0 || signal || pending) fail()
      resolve({ code, signal })
    })
  })
  child.stdout.setEncoding('utf8')
  child.stdout.on('data', chunk => {
    outputBytes += Buffer.byteLength(chunk)
    output += chunk
    if (outputBytes > 1024) { fail(); return }
    while (output.includes('\n')) {
      const index = output.indexOf('\n')
      const line = output.slice(0, index)
      output = output.slice(index + 1)
      if (!pending || pending.expected !== line || broken) { fail(); return }
      pending.resolve()
      pending = null
    }
  })
  const exchange = async (expected, command) => {
    if (broken || ended || pending) refuse()
    let timer
    try {
      await new Promise((resolve, reject) => {
        pending = { expected, resolve, reject }
        timer = setTimeout(fail, 7000)
        if (command) child.stdin.write(command + '\n')
      })
      if (broken || (ended && expected !== 'released')) refuse()
    } finally { clearTimeout(timer) }
  }
  const close = () => {
    if (!closePromise) closePromise = (async () => {
      try {
        await exchange('released', 'release')
        child.stdin.end()
        let timer
        const result = await Promise.race([
          completion,
          new Promise((_, reject) => { timer = setTimeout(() => {
            fail()
            reject(Object.assign(new Error(message), { code: 'native_stop_unconfirmed' }))
          }, 7000) }),
        ]).finally(() => clearTimeout(timer))
        if (broken || output || result.code !== 0 || result.signal) refuse()
      } catch {
        child.stdin.end()
        refuse()
      }
    })()
    return closePromise
  }
  try {
    await exchange('held')
    return { check: () => exchange('held', 'check'), close }
  } catch {
    child.stdin.end()
    refuse()
  }
}

function sameFile(left, right) {
  return left.dev === right.dev && left.ino === right.ino
}

/** Unknown/dotenv-supplied durations wait for systemd, never an assumed short deadline. */
export function nativeServiceStopTimeout(raw) {
  if (raw === undefined) return 0
  if (raw === '') return 240_000 // Go's explicit-empty default is 120s, plus cleanup margin.
  if (!/^\+?\d+$/.test(raw) || raw.length > 20) return 0
  const seconds = BigInt(raw)
  const timeout = seconds * 1000n + 120_000n
  // Node's finite timer cannot represent larger values. Zero means no command timeout.
  if (seconds <= 0n || timeout > 2_147_483_647n) return 0
  return Number(timeout)
}

function originalUnit(state, context) {
  if (state.LoadState !== 'loaded' || state.FragmentPath !== context.paths.unit ||
      state.DropInPaths !== '' || state.NeedDaemonReload !== 'no') refuse()
}

async function unitText(context) {
  const file = await open(context.paths.unit, readFlags)
  try {
    const info = await file.stat()
    if (!info.isFile() || info.uid !== context.uid || info.nlink !== 1 ||
        (info.mode & 0o022) || info.size < 1 || info.size > maxEvidenceBytes) refuse()
    const bytes = Buffer.alloc(info.size + 1)
    const { bytesRead } = await file.read(bytes, 0, bytes.length, 0)
    if (bytesRead !== info.size) refuse()
    return new TextDecoder('utf-8', { fatal: true }).decode(bytes.subarray(0, bytesRead))
  } finally {
    await file.close()
  }
}

async function processIdentity(context, pid, invocation) {
  const proc = '/proc/' + pid
  const [procStat, status, environment, executable, binary, cwd, data] = await Promise.all([
    readFile(proc + '/stat', 'utf8'), readFile(proc + '/status', 'utf8'),
    readFile(proc + '/environ', 'utf8'), stat(proc + '/exe', { bigint: true }),
    stat(context.paths.binary, { bigint: true }), readlink(proc + '/cwd'), stat(context.paths.data),
  ])
  // comm can contain spaces and parentheses. Fields after its final ')' start at field 3.
  const startTicks = procStat.slice(procStat.lastIndexOf(')') + 2).split(' ')[19]
  const uid = Number(/^Uid:\s+\d+\s+(\d+)\s/m.exec(status)?.[1])
  if (!procStat.startsWith(pid + ' (') || !/^[1-9]\d*$/.test(startTicks ?? '') ||
      !sameFile(executable, binary) || !binary.isFile() || cwd !== context.paths.data ||
      !data.isDirectory() || !Number.isSafeInteger(uid) || uid <= 0 || data.uid !== uid ||
      (context.scope === 'user' && uid !== context.uid) ||
      environment.split('\0').filter(entry => entry === 'INVOCATION_ID=' + invocation).length !== 1) refuse()
  const shutdown = environment.split('\0').filter(entry => entry.startsWith('SHUTDOWN_TIMEOUT_SECONDS='))
  const shutdownWaitMs = shutdown.length === 1
    ? nativeServiceStopTimeout(shutdown[0].slice('SHUTDOWN_TIMEOUT_SECONDS='.length))
    : 0
  return { startTicks, uid, executable, shutdownWaitMs }
}

async function processLog(context, pid, uid) {
  const found = new Map()
  for (const fd of await readdir('/proc/' + pid + '/fd')) {
    const target = '/proc/' + pid + '/fd/' + fd
    const filename = await readlink(target).catch(() => null)
    if (!filename || path.dirname(filename) !== context.paths.logs ||
        !/^myapi-\d{14}\.log$/.test(path.basename(filename))) continue
    const held = await stat(target, { bigint: true }).catch(() => null)
    if (!held?.isFile()) refuse()
    found.set(held.dev + ':' + held.ino, { filename, held })
  }
  // Rotation or multiple candidate files cannot be silently combined into one drain proof.
  if (found.size !== 1) refuse()
  const [{ filename, held }] = found.values()
  const file = await open(filename, readFlags)
  try {
    const info = await file.stat({ bigint: true })
    if (!sameFile(info, held) || !info.isFile() || info.nlink !== 1n || info.uid !== BigInt(uid) ||
        (info.mode & 0o022n) || info.size > BigInt(Number.MAX_SAFE_INTEGER)) refuse()
    const offset = Number(info.size)
    const anchor = Buffer.alloc(Math.min(offset, 4096))
    if (anchor.length) {
      const { bytesRead } = await file.read(anchor, 0, anchor.length, offset - anchor.length)
      if (bytesRead !== anchor.length || anchor.at(-1) !== 10) refuse()
    }
    return { file, filename, info, offset, anchor }
  } catch (error) {
    await file.close()
    throw error
  }
}

async function logIdentity(log) {
  const [held, named] = await Promise.all([log.file.stat({ bigint: true }), lstat(log.filename, { bigint: true })])
  if (!sameFile(held, log.info) || !sameFile(named, held) || !named.isFile() ||
      named.nlink !== 1n || named.uid !== log.info.uid || (named.mode & 0o022n) ||
      held.size < BigInt(log.offset) || held.size > BigInt(Number.MAX_SAFE_INTEGER)) refuse()
  if (log.anchor.length) {
    const anchor = Buffer.alloc(log.anchor.length)
    const { bytesRead } = await log.file.read(anchor, 0, anchor.length, log.offset - anchor.length)
    if (bytesRead !== anchor.length || !anchor.equals(log.anchor)) refuse()
  }
  return Number(held.size)
}

/** Pure proof check shared by the held-FD reader and deterministic regression fixtures. */
export function verifyNativeServiceDrain({ before, after, appendedLog }) {
  if (!before || !after || !/^[1-9]\d*$/.test(before.MainPID ?? '') ||
      after.ActiveState !== 'inactive' || after.SubState !== 'dead' || after.MainPID !== '0' ||
      after.ControlPID !== '0' || after.Result !== 'success' || after.ExecMainCode !== '1' ||
      after.ExecMainStatus !== '0' || after.ExecMainPID !== before.MainPID ||
      after.ExecMainStartTimestampMonotonic !== before.ExecMainStartTimestampMonotonic ||
      !/^[1-9]\d*$/.test(before.ExecMainStartTimestampMonotonic ?? '') ||
      !/^[0-9a-f]{32}$/i.test(before.InvocationID ?? '') ||
      (after.InvocationID !== '' && after.InvocationID !== before.InvocationID) ||
      typeof appendedLog !== 'string' || Buffer.byteLength(appendedLog) > maxEvidenceBytes ||
      !appendedLog.endsWith('\n')) refuse()
  let received = false
  let exited = false
  for (const line of appendedLog.split('\n').slice(0, -1)) {
    if (!line.trim()) continue
    const entry = /^\[(SYS|INFO|WARN|ERR|DEBUG|FATAL|GIN)\] (\d{4}\/\d{2}\/\d{2} - \d{2}:\d{2}:\d{2}) \| (.*)$/.exec(line)
    // Unknown multiline output cannot establish that a marker belongs to a real log entry.
    if (!entry) refuse()
    let body = entry[3]
    // SysLog/logHelper terminate with a literal space. The access logger does not.
    if (entry[1] !== 'GIN') {
      if (!body.endsWith(' ')) refuse()
      body = body.slice(0, -1)
    }
    // Request errors are not drain failures. Match the actual system/flush diagnostics,
    // without searching arbitrary provider text or access-log paths for "error".
    if (entry[1] === 'FATAL' || (entry[1] === 'SYS' && (
      /^(?:server forced to shutdown:|panic detected:|InitChannelCache panic:|failed to close database:|increaseQuotaData error:)/.test(body) ||
      /^failed to (?:persist quota balance (?:batch drain|generation error)|recover quota balance (?:compensation|batch preparation)|list quota balance batch drains|apply quota balance batch drain|cleanup quota balance batch drains|batch update token quota):/.test(body)
    ))) refuse()
    const signal = body === 'received signal: terminated, shutting down...'
    const exit = body === 'server exited'
    if (/received signal|server exited/.test(body) && (!signal && !exit || entry[1] !== 'SYS')) refuse()
    if (!signal && !exit) continue
    const parts = entry[2].match(/\d+/g).map(Number)
    // The application and CLI may use different TZ values. Validate calendar fields
    // in UTC only; freshness is established by the held descriptor and stop offset.
    const date = new Date(Date.UTC(parts[0], parts[1] - 1, parts[2], parts[3], parts[4], parts[5]))
    const actual = [date.getUTCFullYear(), date.getUTCMonth() + 1, date.getUTCDate(), date.getUTCHours(), date.getUTCMinutes(), date.getUTCSeconds()]
    if (parts.some((value, index) => value !== actual[index])) refuse()
    if ((signal && (received || exited)) || (exit && (!received || exited))) refuse()
    if (signal) received = true
    if (exit) exited = true
  }
  if (!received || !exited) refuse()
  return { drained: true }
}

/** Capture only; the caller owns the durable journal and the final package/configuration checks. */
export async function prepareNativeServiceStop(context, service) {
  let log
  let reference
  try {
    if (!context || !service || service.name !== 'my-api.service' || service.scope !== context.scope ||
        !['system', 'user'].includes(context.scope) || !Number.isSafeInteger(context.uid) ||
        (context.scope === 'system' ? context.uid !== 0 : context.uid <= 0) ||
        !Number.isSafeInteger(service.pid) || service.pid <= 0 || typeof context.manager !== 'function') refuse()
    const pid = String(service.pid)
    const before = await nativeServiceState(context.manager)
    originalUnit(before, context)
    if (before.ActiveState !== 'active' || before.SubState !== 'running' || before.MainPID !== pid ||
        before.ExecMainPID !== pid || before.ControlPID !== '0' ||
        !/^[0-9a-f]{32}$/i.test(before.InvocationID ?? '') ||
        !/^[1-9]\d*$/.test(before.ExecMainStartTimestampMonotonic ?? '') || await unitText(context) !== context.unit) refuse()
    reference = await openNativeServiceReference(context)
    const identity = await processIdentity(context, pid, before.InvocationID)
    log = await processLog(context, pid, identity.uid)
    const captured = await nativeServiceState(context.manager)
    originalUnit(captured, context)
    const capturedProcess = await processIdentity(context, pid, before.InvocationID)
    if (['MainPID', 'ExecMainPID', 'InvocationID', 'ExecMainStartTimestampMonotonic', 'ActiveState', 'SubState', 'ControlPID']
      .some(key => captured[key] !== before[key]) || capturedProcess.startTicks !== identity.startTicks ||
        !sameFile(capturedProcess.executable, identity.executable) || await unitText(context) !== context.unit) refuse()
    let closed = false
    let attempted = false
    return {
      async close() {
        if (closed) return
        closed = true
        try { await reference.close() } finally { await log.file.close().catch(refuse) }
      },
      async stop() {
        if (closed || attempted) refuse()
        attempted = true
        try {
          await reference.check()
          const current = await nativeServiceState(context.manager)
          originalUnit(current, context)
          if (['MainPID', 'ExecMainPID', 'InvocationID', 'ExecMainStartTimestampMonotonic', 'ActiveState', 'SubState', 'ControlPID']
            .some(key => current[key] !== before[key]) || await unitText(context) !== context.unit) refuse()
          const actual = await processIdentity(context, pid, before.InvocationID)
          if (actual.startTicks !== identity.startTicks || actual.uid !== identity.uid ||
              !sameFile(actual.executable, identity.executable)) refuse()
          const actualLog = await processLog(context, pid, identity.uid)
          try {
            if (!sameFile(actualLog.info, log.info) || actualLog.filename !== log.filename) refuse()
          } finally {
            await actualLog.file.close()
          }
          // No shutdown marker may predate our stop command, even if it arrived after prepare().
          const precedingSize = await logIdentity(log)
          if (precedingSize !== log.offset) {
            const length = precedingSize - log.offset
            if (length > maxEvidenceBytes) refuse()
            const pending = Buffer.alloc(length)
            const { bytesRead } = await log.file.read(pending, 0, length, log.offset)
            if (bytesRead !== length || await logIdentity(log) !== precedingSize ||
                /received signal|server exited/.test(pending.toString('utf8'))) refuse()
          }
          // Only bytes appended after the final pre-stop read may prove this stop.
          const stopOffset = precedingSize
          await context.manager(['stop', service.name], actual.shutdownWaitMs)
          await reference.check()
          const after = await nativeServiceState(context.manager)
          originalUnit(after, context)
          if (await unitText(context) !== context.unit) refuse()
          try {
            await lstat('/proc/' + pid)
            refuse()
          } catch (error) {
            if (error.code !== 'ENOENT' && error.code !== 'ESRCH') throw error
          }
          const size = await logIdentity(log)
          const length = size - stopOffset
          if (length <= 0 || length > maxEvidenceBytes) refuse()
          const bytes = Buffer.alloc(length)
          const { bytesRead } = await log.file.read(bytes, 0, length, stopOffset)
          if (bytesRead !== length || await logIdentity(log) !== size) refuse()
          const result = verifyNativeServiceDrain({ before, after,
            appendedLog: new TextDecoder('utf-8', { fatal: true }).decode(bytes) })
          await reference.check()
          await reference.close()
          return result
        } catch {
          refuse()
        }
      },
    }
  } catch {
    if (log) await log.file.close().catch(() => {})
    if (reference) await reference.close().catch(() => {})
    refuse()
  }
}
