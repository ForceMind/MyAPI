import assert from 'node:assert/strict'
import { spawn, spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { chmod, chown, link, mkdir, mkdtemp, readFile, rename, rm, stat, symlink, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

assert.equal(process.platform, 'linux', 'root writer evidence requires Linux')
assert.equal(process.arch, 'x64', 'root writer evidence requires amd64')
assert.equal(process.getuid(), 0, 'run this file explicitly with sudo; absence of privilege is not a skip')

const helper = fileURLToPath(new URL('./writer_guard.py', import.meta.url))
const python = '/usr/bin/python3'
const environment = { PATH: '/usr/bin:/bin', LANG: 'C.UTF-8' }
const secretFixture = 'SYNTHETIC_PAYLOAD_MUST_NOT_APPEAR_IN_EVIDENCE'

function observe(request) {
  const result = spawnSync(python, ['-I', helper], {
    input: JSON.stringify(request), encoding: 'utf8', env: environment, timeout: 120_000, maxBuffer: 65536,
  })
  assert.equal(result.error, undefined)
  assert.equal(result.signal, null)
  assert.equal(result.stderr, '')
  const value = JSON.parse(result.stdout)
  assert.equal(value.schema_version, 1)
  assert.equal(result.stdout.includes(secretFixture), false)
  assert.equal(result.stdout.includes('cmdline'), false)
  assert.equal(result.stdout.includes('ledger.bin'), false)
  if (value.status === 'observed') assert.equal(result.status, 0)
  else {
    assert.ok(['blocked', 'unknown'].includes(value.status))
    assert.notEqual(result.status, 0)
    assert.equal(Object.hasOwn(value, 'binding'), false)
    assert.equal(Object.hasOwn(value, 'evidence'), false)
  }
  return value
}

async function fixture(t, mode = 0o755) {
  // The root-owned sticky /tmp boundary protects the root-owned top directory.
  // A runner-owned RUNNER_TEMP ancestor is deliberately not used.
  const root = await mkdtemp('/tmp/myapi-writer-guard-')
  await chmod(root, 0o700)
  const children = new Set()
  t.after(async () => {
    for (const child of children) await child.cleanup()
    await rm(root, { recursive: true, force: true })
  })
  const rootfs = path.join(root, 'rootfs')
  const data = path.join(rootfs, 'data')
  await mkdir(data, { recursive: true, mode: 0o755 })
  await chmod(data, mode)
  const payload = path.join(data, 'ledger.bin')
  await writeFile(payload, secretFixture, { mode: 0o600 })
  return { root, rootfs, data, payload, children }
}

async function controlledChild(t, source, arguments_, children) {
  const child = spawn(python, ['-I', '-u', '-c', source, ...arguments_], {
    stdio: ['pipe', 'pipe', 'pipe'], env: environment,
  })
  let closed = false
  const completed = new Promise((resolve, reject) => {
    child.once('error', reject)
    child.once('close', (code, signal) => { closed = true; resolve({ code, signal }) })
  })
  const controller = {
    pid: child.pid,
    cleanup: async () => {
      if (!closed) child.stdin.end()
      const timer = setTimeout(() => { if (!closed) child.kill('SIGKILL') }, 5000)
      try { await completed } finally { clearTimeout(timer) }
    },
    stop: async () => {
      child.stdin.end()
      assert.deepEqual(await completed, { code: 0, signal: null })
    },
  }
  children.add(controller)
  t.after(controller.cleanup)
  await new Promise((resolve, reject) => {
    let output = ''
    let errors = ''
    const timer = setTimeout(() => reject(new Error('synthetic child did not become ready')), 5000)
    child.stdout.on('data', chunk => {
      output += chunk
      if (output.includes('\n')) {
        clearTimeout(timer)
        if (output.trim() === 'ready') resolve()
        else reject(new Error('unexpected synthetic readiness'))
      }
    })
    child.stderr.on('data', chunk => { errors += chunk })
    child.once('error', error => { clearTimeout(timer); reject(error) })
    child.once('close', () => {
      clearTimeout(timer)
      if (!output.includes('\n')) reject(new Error('synthetic child setup failed: ' + errors))
    })
  })
  return controller
}

const sourceProgram = `
import os, sys
os.chroot(sys.argv[1])
os.chdir("/")
print("ready", flush=True)
sys.stdin.buffer.read(1)
`

const writerProgram = `
import ctypes, mmap, os, sys
fd = os.open(sys.argv[1], os.O_RDWR)
mapping = None
if sys.argv[2] == "mmap":
    # Python mmap normally duplicates the FD internally. Use libc directly
    # so this fixture genuinely retains only the shared writable mapping.
    size = os.fstat(fd).st_size
    identity = (os.fstat(fd).st_dev, os.fstat(fd).st_ino)
    libc = ctypes.CDLL(None, use_errno=True)
    libc.mmap.argtypes = [ctypes.c_void_p, ctypes.c_size_t, ctypes.c_int, ctypes.c_int, ctypes.c_int, ctypes.c_long]
    libc.mmap.restype = ctypes.c_void_p
    libc.munmap.argtypes = [ctypes.c_void_p, ctypes.c_size_t]
    libc.munmap.restype = ctypes.c_int
    mapping = libc.mmap(None, size, mmap.PROT_READ | mmap.PROT_WRITE, mmap.MAP_SHARED, fd, 0)
    if mapping == ctypes.c_void_p(-1).value:
        raise OSError(ctypes.get_errno(), "synthetic mmap failed")
    os.close(fd)
    for entry in os.scandir("/proc/self/fd"):
        try:
            info = os.stat(entry.path)
        except FileNotFoundError:
            continue
        assert (info.st_dev, info.st_ino) != identity
print("ready", flush=True)
sys.stdin.buffer.read(1)
if mapping is not None:
    assert libc.munmap(mapping, size) == 0
else:
    os.close(fd)
`

async function boundSource(t, mode) {
  const value = await fixture(t, mode)
  const source = await controlledChild(t, sourceProgram, [value.rootfs], value.children)
  const response = observe({ action: 'bind_running', data_dir: value.data, host_pid: source.pid })
  assert.equal(response.status, 'observed')
  assert.equal(response.binding.data_root.uid, 0)
  assert.equal(response.binding.data_root.mode, mode.toString(8).padStart(4, '0'))
  assert.equal(response.binding.source_process.pid, source.pid)
  assert.match(response.binding.source_process.starttime, /^[0-9]+$/)
  assert.match(response.binding.fingerprint, /^[a-f0-9]{64}$/)
  return { ...value, source, binding: response.binding }
}

function pure(body) {
  const setup = `
import importlib.util, json, sys
from unittest.mock import patch
sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("guard", sys.argv[1])
guard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(guard)
def capture(fn):
    try:
        return {"value": fn()}
    except guard.GuardError as error:
        return {"status": error.status, "code": error.code}
`
  const result = spawnSync(python, ['-I', '-B', '-c', setup + '\n' + body, helper], {
    encoding: 'utf8', env: environment, timeout: 30_000, maxBuffer: 65536,
  })
  assert.equal(result.error, undefined)
  assert.equal(result.status, 0, result.stderr)
  assert.equal(result.stderr, '')
  return JSON.parse(result.stdout)
}

for (const mode of [0o755, 0o700]) {
  test('real chroot binding and repeatable stopped scan preserve root-owned ' + mode.toString(8) + ' source', async t => {
    const value = await boundSource(t, mode)
    await value.source.stop()
    const expected = await readFile(value.payload)
    const first = observe({ action: 'stopped_scan', data_dir: value.data, binding: value.binding })
    assert.equal(first.status, 'observed')
    assert.deepEqual(first.binding, value.binding)
    assert.equal(first.evidence.old_generation_gone, true)
    assert.equal(first.evidence.fd_writers, 0)
    assert.equal(first.evidence.shared_writable_mappings, 0)
    assert.ok(first.evidence.processes > 0)
    assert.ok(first.evidence.tasks >= first.evidence.processes)
    const second = observe({ action: 'stopped_scan', data_dir: value.data, binding: value.binding })
    assert.equal(second.status, 'observed')
    assert.deepEqual(second.binding, value.binding)
    assert.equal(second.evidence.tree_metadata_sha256, first.evidence.tree_metadata_sha256)
    assert.deepEqual(await readFile(value.payload), expected)
    assert.equal((await stat(value.data)).mode & 0o777, mode)
    assert.equal((await stat(value.payload)).mode & 0o777, 0o600)
  })
}

test('visible descriptor and closed-descriptor shared writable mapping are independently blocked', async t => {
  for (const [kind, code] of [['fd', 'WRITABLE_FD_PRESENT'], ['mmap', 'WRITABLE_MMAP_PRESENT']]) {
    await t.test(kind, async t => {
      const value = await boundSource(t, 0o755)
      await value.source.stop()
      const before = await readFile(value.payload)
      const writer = await controlledChild(t, writerProgram, [value.payload, kind], value.children)
      const blocked = observe({ action: 'stopped_scan', data_dir: value.data, binding: value.binding })
      assert.equal(blocked.status, 'blocked')
      assert.equal(blocked.code, code)
      assert.deepEqual(await readFile(value.payload), before)
      await writer.stop()
      const clear = observe({ action: 'stopped_scan', data_dir: value.data, binding: value.binding })
      assert.equal(clear.status, 'observed')
    })
  }
})

test('live source generation blocks scan while a post-stop replacement inode remains allowed', async t => {
  const value = await boundSource(t, 0o700)
  const alive = observe({ action: 'stopped_scan', data_dir: value.data, binding: value.binding })
  assert.equal(alive.status, 'blocked')
  assert.equal(alive.code, 'SOURCE_GENERATION_ALIVE')
  const replacement = path.join(value.data, 'replacement.bin')
  await writeFile(replacement, 'synthetic checkpointed data', { mode: 0o600 })
  const prior = await stat(value.payload, { bigint: true })
  const next = await stat(replacement, { bigint: true })
  assert.notEqual(prior.ino, next.ino)
  await value.source.stop()
  await rename(replacement, value.payload)
  const expected = await readFile(value.payload)
  const clear = observe({ action: 'stopped_scan', data_dir: value.data, binding: value.binding })
  assert.equal(clear.status, 'observed')
  assert.deepEqual(clear.binding, value.binding)
  assert.deepEqual(await readFile(value.payload), expected)
})

test('unsafe owners, writable roots, links and caller bypass fields never produce observations', async t => {
  const value = await fixture(t)
  await chmod(value.data, 0o777)
  assert.equal(observe({ action: 'bind_running', data_dir: value.data, host_pid: process.pid }).code, 'SOURCE_ROOT_WRITABLE')
  await chmod(value.data, 0o755)
  await chown(value.data, 12345, 0)
  assert.equal(observe({ action: 'bind_running', data_dir: value.data, host_pid: process.pid }).code, 'UNSAFE_SOURCE_ANCESTOR')
  await chown(value.data, 0, 0)
  await chown(value.rootfs, 12345, 0)
  assert.equal(observe({ action: 'bind_running', data_dir: value.data, host_pid: process.pid }).code, 'UNSAFE_SOURCE_ANCESTOR')
  await chown(value.rootfs, 0, 0)
  await chmod(value.rootfs, 0o777)
  assert.equal(observe({ action: 'bind_running', data_dir: value.data, host_pid: process.pid }).code, 'UNSAFE_SOURCE_ANCESTOR')
  await chmod(value.rootfs, 0o755)
  await link(value.payload, path.join(value.data, 'alias.bin'))
  assert.equal(observe({ action: 'bind_running', data_dir: value.data, host_pid: process.pid }).code, 'UNSAFE_SOURCE_ENTRY')
  await rm(path.join(value.data, 'alias.bin'))
  await symlink(value.payload, path.join(value.data, 'linked.bin'))
  assert.equal(observe({ action: 'bind_running', data_dir: value.data, host_pid: process.pid }).code, 'UNSAFE_SOURCE_ENTRY')
  await rm(path.join(value.data, 'linked.bin'))
  const alias = path.join(value.root, 'data-link')
  await symlink(value.data, alias)
  assert.equal(observe({ action: 'bind_running', data_dir: alias, host_pid: process.pid }).code, 'UNSAFE_SOURCE_LINK')
  assert.equal(observe({ action: 'bind_running', data_dir: value.data, host_pid: process.pid, verified: true }).code, 'INVALID_REQUEST')
  assert.equal(observe({ action: 'bind_running', data_dir: value.data, host_pid: process.pid, proc_root: value.root }).code, 'INVALID_REQUEST')
})

test('kernel parsers preserve octal access, shared mapping and PID-generation evidence', () => {
  const result = pure(`
tail = ["S"] + ["0"] * 18 + ["456"]
record = "123 (synthetic ) worker) " + " ".join(tail)
print(json.dumps({
    "start": guard.parse_process_stat(record, 123),
    "reuse": guard.generation_gone(456, 789),
    "same": guard.generation_gone(456, 456),
    "gone": guard.generation_gone(456, None),
    "fd": guard.parse_fdinfo("pos:\\t0\\nflags:\\t0100002\\nmnt_id:\\t19\\nino:\\t63107\\n"),
    "map": guard.parse_maps("1000-2000 rw-s 00000000 08:01 63107 /synthetic\\n"),
    "bad_fd": capture(lambda: guard.parse_fdinfo("flags: 09\\n")),
    "bad_maps": capture(lambda: guard.parse_maps("not kernel maps")),
}))
`)
  assert.equal(result.start, 456)
  assert.equal(result.reuse, true)
  assert.equal(result.same, false)
  assert.equal(result.gone, true)
  assert.deepEqual(result.fd, [0o100002, 19, 63107])
  assert.deepEqual(result.map, [[8, 1, 63107, 'rw-s']])
  assert.deepEqual(result.bad_fd, { status: 'unknown', code: 'FDINFO_PARSE_UNKNOWN' })
  assert.deepEqual(result.bad_maps, { status: 'unknown', code: 'MAPS_PARSE_UNKNOWN' })
})

test('hidden proc, permission denial and unexplained disappearance remain unknown', () => {
  const result = pure(`
hidden = guard.parse_mountinfo("1 0 0:1 / /proc rw - proc proc rw,hidepid=2\\n")
with patch.object(guard, "read_proc", side_effect=PermissionError):
    permission = capture(lambda: guard.confirm_process_gone(123, 456))
with patch.object(guard, "read_proc", side_effect=FileNotFoundError), patch.object(guard, "path_absent", return_value=False):
    race = capture(lambda: guard.confirm_process_gone(123, 456))
with patch.object(guard, "read_proc", side_effect=FileNotFoundError), patch.object(guard, "path_absent", return_value=True):
    vanished = capture(lambda: guard.confirm_process_gone(123, 456))
print(json.dumps({"hidden": capture(lambda: guard.proc_view(hidden)), "permission": permission, "race": race, "vanished": vanished}))
`)
  assert.deepEqual(result.hidden, { status: 'unknown', code: 'PROC_VISIBILITY_UNKNOWN' })
  assert.deepEqual(result.permission, { status: 'unknown', code: 'PROC_ACCESS_UNKNOWN' })
  assert.deepEqual(result.race, { status: 'unknown', code: 'PROC_SCAN_RACE' })
  assert.deepEqual(result.vanished, { value: true })
})

test('binding tampering and mismatched live inode cannot masquerade as a stopped proof', async t => {
  const value = await boundSource(t, 0o755)
  const tampered = structuredClone(value.binding)
  tampered.source_process.starttime = '0'
  const result = observe({ action: 'stopped_scan', data_dir: value.data, binding: tampered })
  assert.equal(result.status, 'blocked')
  assert.equal(result.code, 'INVALID_BINDING')
  const foreign = await fixture(t)
  const mismatch = observe({ action: 'bind_running', data_dir: foreign.data, host_pid: value.source.pid })
  assert.equal(mismatch.status, 'blocked')
  assert.equal(mismatch.code, 'HOST_DATA_BINDING_MISMATCH')
  const bytes = await readFile(value.payload)
  assert.equal(createHash('sha256').update(bytes).digest('hex'), createHash('sha256').update(secretFixture).digest('hex'))
})

