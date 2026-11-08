import assert from 'node:assert/strict'
import { spawn, spawnSync } from 'node:child_process'
import { chmod, chown, link, mkdir, mkdtemp, readFile, rm, symlink, utimes, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

assert.equal(process.platform, 'linux')
assert.equal(process.arch, 'x64')
assert.equal(process.getuid(), 0, 'archive usage evidence requires explicit sudo; no skip fallback')

const helper = fileURLToPath(new URL('./writer_guard.py', import.meta.url))
const environment = { PATH: '/usr/bin:/bin', LANG: 'C.UTF-8' }
const payload = 'SYNTHETIC_ARCHIVE_CONTENT_NOT_FOR_STDOUT'

function guard(input) {
  const result = spawnSync('/usr/bin/python3', ['-I', helper], {
    input: JSON.stringify(input), encoding: 'utf8', env: environment, timeout: 120000, maxBuffer: 65536,
  })
  assert.equal(result.error, undefined)
  assert.equal(result.signal, null)
  assert.equal(result.stderr, '')
  assert.equal(result.stdout.includes(payload), false)
  assert.equal(result.stdout.includes('payload.bin'), false)
  const value = JSON.parse(result.stdout)
  if (value.status === 'observed') assert.equal(result.status, 0)
  else {
    assert.notEqual(result.status, 0)
    assert.ok(['blocked', 'unknown'].includes(value.status))
    assert.equal(Object.hasOwn(value, 'binding'), false)
    assert.equal(Object.hasOwn(value, 'evidence'), false)
  }
  if (input.action === 'archive_scan') assert.equal(value.deletion_authorized, false)
  return value
}

async function fixture(t) {
  const root = await mkdtemp('/tmp/myapi-archive-usage-')
  await chmod(root, 0o700)
  const rootfs = path.join(root, 'rootfs')
  const archive = path.join(rootfs, 'data')
  const childDirectory = path.join(archive, 'child')
  await mkdir(childDirectory, { recursive: true, mode: 0o700 })
  await chmod(archive, 0o700)
  await writeFile(path.join(archive, 'payload.bin'), payload, { mode: 0o600 })
  const children = new Set()
  t.after(async () => {
    for (const child of children) await child.cleanup()
    await rm(root, { recursive: true, force: true })
  })
  return { root, rootfs, archive, childDirectory, children }
}

const holderProgram = String.raw`
import ctypes, mmap, os, sys
base, mode = sys.argv[1:3]
fd = None
mapping = None
if mode == "root":
    os.chroot(base)
    os.chdir("/")
elif mode == "cwd":
    os.chdir(base)
elif mode == "dirfd":
    fd = os.open(base, os.O_RDONLY | os.O_DIRECTORY)
else:
    filename = os.path.join(base, "payload.bin")
    fd = os.open(filename, os.O_RDWR if mode in ("rw", "shared-map") else os.O_RDONLY)
    if mode in ("private-map", "shared-map"):
        size = os.fstat(fd).st_size
        identity = (os.fstat(fd).st_dev, os.fstat(fd).st_ino)
        libc = ctypes.CDLL(None, use_errno=True)
        libc.mmap.argtypes = [ctypes.c_void_p, ctypes.c_size_t, ctypes.c_int, ctypes.c_int, ctypes.c_int, ctypes.c_long]
        libc.mmap.restype = ctypes.c_void_p
        libc.munmap.argtypes = [ctypes.c_void_p, ctypes.c_size_t]
        libc.munmap.restype = ctypes.c_int
        protection = mmap.PROT_READ | (mmap.PROT_WRITE if mode == "shared-map" else 0)
        sharing = mmap.MAP_SHARED if mode == "shared-map" else mmap.MAP_PRIVATE
        mapping = libc.mmap(None, size, protection, sharing, fd, 0)
        assert mapping != ctypes.c_void_p(-1).value
        os.close(fd)
        fd = None
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
if fd is not None:
    os.close(fd)
`

const sourceProgram = String.raw`
import os, sys
os.chroot(sys.argv[1])
os.chdir("/")
print("ready", flush=True)
sys.stdin.buffer.read(1)
`

async function hold(t, value, program, arguments_) {
  const child = spawn('/usr/bin/python3', ['-I', '-u', '-c', program, ...arguments_], {
    env: environment, stdio: ['pipe', 'pipe', 'pipe'],
  })
  let closed = false
  const completion = new Promise((resolve, reject) => {
    child.once('error', reject)
    child.once('close', (code, signal) => { closed = true; resolve({ code, signal }) })
  })
  const resource = {
    pid: child.pid,
    cleanup: async () => {
      if (!closed) child.stdin.end()
      const timer = setTimeout(() => { if (!closed) child.kill('SIGKILL') }, 5000)
      try { await completion } finally { clearTimeout(timer) }
    },
    close: async () => { child.stdin.end(); assert.deepEqual(await completion, { code: 0, signal: null }) },
  }
  value.children.add(resource)
  t.after(resource.cleanup)
  await new Promise((resolve, reject) => {
    let output = '', errors = ''
    const timer = setTimeout(() => reject(new Error('synthetic holder readiness deadline')), 5000)
    child.stdout.on('data', chunk => {
      output += chunk
      if (output.includes('\n')) {
        clearTimeout(timer)
        if (output.trim() === 'ready') resolve()
        else reject(new Error('invalid synthetic readiness'))
      }
    })
    child.stderr.on('data', chunk => { errors += chunk })
    child.once('error', error => { clearTimeout(timer); reject(error) })
    child.once('close', () => {
      clearTimeout(timer)
      if (!output.includes('\n')) reject(new Error('synthetic holder failed: ' + errors))
    })
  })
  return resource
}

function pure(body, input = {}) {
  const setup = String.raw`
import contextlib, importlib.util, io, json, os, stat, sys
from types import SimpleNamespace
from unittest.mock import patch
sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("guard", sys.argv[1])
guard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(guard)
input = json.load(sys.stdin)
def capture(fn):
    try:
        return {"value": fn()}
    except guard.GuardError as error:
        return {"status": error.status, "code": error.code}
`
  const result = spawnSync('/usr/bin/python3', ['-I', '-B', '-c', setup + '\n' + body, helper], {
    input: JSON.stringify(input), env: environment, encoding: 'utf8', timeout: 120000, maxBuffer: 65536,
  })
  assert.equal(result.error, undefined)
  assert.equal(result.status, 0, result.stderr)
  assert.equal(result.stderr, '')
  return JSON.parse(result.stdout)
}

test('archive census does not hold its own tree descriptors or claim deletion/manifest authority', async t => {
  const value = await fixture(t)
  const first = guard({ action: 'archive_scan', archive_dir: value.archive })
  assert.equal(first.status, 'observed')
  assert.equal(first.action, 'archive_scan')
  assert.equal(first.binding.kind, 'archive')
  assert.equal(first.binding.archive_root.mode, '0700')
  assert.equal(Object.hasOwn(first.binding, 'source_process'), false)
  assert.equal(Object.hasOwn(first.binding.archive_root, 'mtime_ns'), false)
  assert.equal(Object.hasOwn(first.binding.archive_root, 'ctime_ns'), false)
  assert.equal(first.evidence.manifest_verified, false)
  for (const key of ['archive_file_fds', 'archive_directory_fds', 'archive_cwd_refs', 'archive_root_refs', 'archive_mappings']) {
    assert.equal(first.evidence[key], 0)
  }
  assert.ok(first.evidence.processes > 0)
  assert.ok(first.evidence.cwd_checks > 0 && first.evidence.root_checks > 0)
  const before = await readFile(path.join(value.archive, 'payload.bin'))
  await utimes(value.archive, new Date(1000), new Date(2000))
  const second = guard({ action: 'archive_scan', archive_dir: value.archive })
  assert.equal(second.status, 'observed')
  assert.deepEqual(second.binding, first.binding)
  assert.notEqual(second.evidence.tree_metadata_sha256, first.evidence.tree_metadata_sha256)
  assert.deepEqual(await readFile(path.join(value.archive, 'payload.bin')), before)
})

test('real readonly/writable file, directory, cwd and root inode references block until closed', async t => {
  for (const [mode, subdir, code] of [
    ['ro', false, 'ARCHIVE_FILE_FD_PRESENT'], ['rw', false, 'ARCHIVE_FILE_FD_PRESENT'],
    ['dirfd', false, 'ARCHIVE_DIRECTORY_FD_PRESENT'], ['dirfd', true, 'ARCHIVE_DIRECTORY_FD_PRESENT'],
    ['cwd', true, 'ARCHIVE_CWD_PRESENT'], ['root', false, 'ARCHIVE_ROOT_PRESENT'],
  ]) {
    await t.test(mode + (subdir ? '-child' : '-root'), async t => {
      const value = await fixture(t)
      const child = await hold(t, value, holderProgram, [subdir ? value.childDirectory : value.archive, mode])
      const response = guard({ action: 'archive_scan', archive_dir: value.archive })
      assert.equal(response.status, 'blocked')
      assert.equal(response.code, code)
      await child.close()
      assert.equal(guard({ action: 'archive_scan', archive_dir: value.archive }).status, 'observed')
    })
  }
})

test('closed-FD private readonly and shared writable libc mappings remain archive uses', async t => {
  for (const mode of ['private-map', 'shared-map']) {
    await t.test(mode, async t => {
      const value = await fixture(t)
      const child = await hold(t, value, holderProgram, [value.archive, mode])
      const response = guard({ action: 'archive_scan', archive_dir: value.archive })
      assert.equal(response.status, 'blocked')
      assert.equal(response.code, 'ARCHIVE_MMAP_PRESENT')
      await child.close()
      const afterClose = guard({ action: 'archive_scan', archive_dir: value.archive })
      assert.equal(afterClose.status, 'observed', afterClose.code)
    })
  }
})

test('own PID is not exempt and uint64 archive identity remains decimal text', async t => {
  const value = await fixture(t)
  const result = pure(String.raw`
with open(os.path.join(input["archive"], "payload.bin"), "rb") as held:
    own = capture(lambda: guard.observe({"action": "archive_scan", "archive_dir": input["archive"]}))
big = guard.root_identity(SimpleNamespace(st_dev=2**60+17, st_ino=2**61+3, st_uid=0, st_gid=0, st_mode=stat.S_IFDIR | 0o700))
print(json.dumps({"own": own, "big": big}))
`, { archive: value.archive })
  assert.equal(result.own.status, 'blocked')
  assert.equal(result.own.code, 'ARCHIVE_FILE_FD_PRESENT')
  assert.equal(result.big.dev, ((1n << 60n) + 17n).toString())
  assert.equal(result.big.ino, ((1n << 61n) + 3n).toString())
})

test('strict schema, non-private archives and link targets are refused without repair', async t => {
  const value = await fixture(t)
  for (const extra of [{ verified: true }, { proc_root: value.root }, { ignore_use: true }, { binding: {} }]) {
    assert.equal(guard({ action: 'archive_scan', archive_dir: value.archive, ...extra }).code, 'INVALID_REQUEST')
  }
  await chmod(value.archive, 0o755)
  assert.equal(guard({ action: 'archive_scan', archive_dir: value.archive }).code, 'ARCHIVE_ROOT_NOT_PRIVATE')
  await chmod(value.archive, 0o700)
  await symlink(path.join(value.archive, 'payload.bin'), path.join(value.archive, 'linked'))
  assert.equal(guard({ action: 'archive_scan', archive_dir: value.archive }).code, 'UNSAFE_SOURCE_ENTRY')
  await rm(path.join(value.archive, 'linked'))
  await link(path.join(value.archive, 'payload.bin'), path.join(value.archive, 'hard'))
  assert.equal(guard({ action: 'archive_scan', archive_dir: value.archive }).code, 'UNSAFE_SOURCE_ENTRY')
  await rm(path.join(value.archive, 'hard'))
  await chown(value.archive, 12345, 0)
  assert.equal(guard({ action: 'archive_scan', archive_dir: value.archive }).code, 'UNSAFE_SOURCE_ANCESTOR')
})

test('hidden proc, permission and unexplained alive-task absence remain unknown', async t => {
  const value = await fixture(t)
  const result = pure(String.raw`
from collections import defaultdict
hidden = guard.parse_mountinfo("1 0 0:1 / /proc rw - proc proc rw,hidepid=2\n")
output = io.StringIO()
stdin = SimpleNamespace(buffer=io.BytesIO(json.dumps({"action": "archive_scan", "archive_dir": input["archive"]}).encode()))
with patch.object(sys, "stdin", stdin), patch.object(guard, "read_proc", side_effect=PermissionError), contextlib.redirect_stdout(output):
    permission_exit = guard.main()
permission = {"exit": permission_exit, "reply": json.loads(output.getvalue())}
with patch.object(guard.os, "stat", side_effect=FileNotFoundError), patch.object(guard, "confirm_process_gone", return_value=False), patch.object(guard, "task_without_fs_reference", return_value=False):
    race = capture(lambda: guard.archive_task_references(123, 123, 456, set(), defaultdict(int)))
def record(state, flags, start):
    return "123 (synthetic) " + " ".join([state] + ["0"]*5 + [str(flags)] + ["0"]*12 + [str(start)])
with patch.object(guard, "read_proc", return_value=record("I", 0x00200000, 456)):
    kernel = guard.task_without_fs_reference(123, 123, 456)
with patch.object(guard, "read_proc", return_value=record("Z", 0, 456)):
    zombie = guard.task_without_fs_reference(123, 123, 456)
with patch.object(guard, "read_proc", side_effect=[record("Z", 0, 456), record("Z", 0, 789)]):
    reuse = capture(lambda: guard.task_without_fs_reference(123, 123, 456))
print(json.dumps({"hidden": capture(lambda: guard.proc_view(hidden)), "permission": permission, "race": race, "kernel": kernel, "zombie": zombie, "reuse": reuse}))
`, { archive: value.archive })
  assert.deepEqual(result.hidden, { status: 'unknown', code: 'PROC_VISIBILITY_UNKNOWN' })
  assert.equal(result.permission.exit, 2)
  assert.equal(result.permission.reply.status, 'unknown')
  assert.equal(result.permission.reply.code, 'KERNEL_OBSERVATION_UNAVAILABLE')
  assert.equal(result.permission.reply.deletion_authorized, false)
  assert.equal(Object.hasOwn(result.permission.reply, 'evidence'), false)
  assert.deepEqual(result.race, { status: 'unknown', code: 'PROC_SCAN_RACE' })
  assert.equal(result.kernel, true)
  assert.equal(result.zombie, true)
  assert.deepEqual(result.reuse, { status: 'unknown', code: 'PROC_SCAN_RACE' })
})

test('unknown filesystem, nested mounts and cross-device entries cannot become observations', async t => {
  const value = await fixture(t)
  const result = pure(String.raw`
archive = input["archive"]
info = os.stat(archive)
device = str(os.major(info.st_dev)) + ":" + str(os.minor(info.st_dev))
mount = {"point": archive, "type": "ext4", "device": device, "id": 123, "options": set()}
unsupported = capture(lambda: guard.source_root(archive, [{**mount, "type": "nfs"}]))
nested = capture(lambda: guard.source_root(archive, [mount, {**mount, "point": archive + "/child"}]))
expected = guard.root_identity(info)
real_stat = os.stat
def cross_stat(filename, *args, **kwargs):
    if filename == "payload.bin" and kwargs.get("dir_fd") is not None:
        return SimpleNamespace(st_dev=info.st_dev+1)
    return real_stat(filename, *args, **kwargs)
with patch.object(guard.os, "stat", side_effect=cross_stat):
    cross = capture(lambda: guard.snapshot_tree(archive, expected))
print(json.dumps({"unsupported": unsupported, "nested": nested, "cross": cross}))
`, { archive: value.archive })
  assert.equal(result.unsupported.code, 'UNSUPPORTED_FILESYSTEM')
  assert.equal(result.nested.code, 'NESTED_SOURCE_MOUNT')
  assert.equal(result.cross.code, 'NESTED_SOURCE_FILESYSTEM')
})

test('source stopped_scan still allows readonly FD and private readonly mappings', async t => {
  const value = await fixture(t)
  const source = await hold(t, value, sourceProgram, [value.rootfs])
  const running = guard({ action: 'bind_running', data_dir: value.archive, host_pid: source.pid })
  assert.equal(running.status, 'observed')
  await source.close()
  for (const mode of ['ro', 'private-map']) {
    const child = await hold(t, value, holderProgram, [value.archive, mode])
    const stopped = guard({ action: 'stopped_scan', data_dir: value.archive, binding: running.binding })
    assert.equal(stopped.status, 'observed')
    assert.deepEqual(stopped.binding, running.binding)
    assert.equal(stopped.evidence.old_generation_gone, true)
    assert.equal(Object.hasOwn(stopped.evidence, 'archive_mappings'), false)
    assert.equal(guard({ action: 'archive_scan', archive_dir: value.archive }).status, 'blocked')
    await child.close()
  }
})

