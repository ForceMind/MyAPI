// Build and verify the offline operations kit on an Actions runner. Never
// include credentials, journals, instance registrations or test fixtures.
import { createHash } from 'node:crypto'
import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
import { chmod, lstat, mkdir, mkdtemp, readFile, readdir, realpath, rm, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { pathToFileURL } from 'node:url'

const execute = promisify(execFile)
export const kitFiles = Object.freeze([
  'VERSION', 'LICENSE', 'NOTICE',
  'tools/operations/auth.mjs', 'tools/operations/backup_data.py', 'tools/operations/capacity.mjs',
  'tools/operations/config.mjs', 'tools/operations/daemon.mjs', 'tools/operations/docker.mjs',
  'tools/operations/engine.mjs', 'tools/operations/extract_bundle.py', 'tools/operations/github.mjs',
  'tools/operations/health.mjs', 'tools/operations/lock_guard.py', 'tools/operations/python.mjs',
  'tools/operations/safety.mjs', 'tools/operations/service.mjs', 'tools/operations/sqlite_read.py', 'tools/operations/store.mjs',
  'tools/runtime/smoke-image-bundle.mjs',
  'tools/management/catalog.mjs', 'tools/management/client.mjs', 'tools/management/mcp.mjs',
  'deploy/operations/install.sh', 'deploy/operations/myapi-operations.service',
  'deploy/operations/config.example.json', 'deploy/operations/README.md', 'docs/REMOTE_MANAGEMENT.md',
])

const hash = bytes => createHash('sha256').update(bytes).digest('hex')
function check(condition) { if (!condition) throw new Error('OPERATIONS_KIT_INVALID') }

async function sourceFile(root, relative) {
  const filename = path.join(root, relative)
  const info = await lstat(filename)
  check(info.isFile() && !info.isSymbolicLink() && info.size > 0 && info.size <= 4 * 1024 * 1024 && await realpath(filename) === filename)
  return readFile(filename)
}

export async function buildOperationsKit({ root, output, sourceSha, treeSha, runId, attempt }) {
  check(path.isAbsolute(root) && path.isAbsolute(output) && await realpath(root) === root)
  check([sourceSha, treeSha].every(value => /^[a-f0-9]{40}$/.test(value)) && [runId, attempt].every(value => /^[1-9][0-9]{0,19}$/.test(value)))
  const version = (await sourceFile(root, 'VERSION')).toString('utf8').trim()
  check(/^0\.2\.0-beta\.[1-9][0-9]{0,8}$/.test(version))
  await mkdir(output, { recursive: false, mode: 0o700 })
  const temporary = await mkdtemp(path.join(output, '.kit-'))
  const directory = path.join(temporary, 'myapi-operations')
  try {
    const files = {}
    for (const relative of kitFiles) {
      const bytes = await sourceFile(root, relative)
      const target = path.join(directory, relative)
      await mkdir(path.dirname(target), { recursive: true, mode: 0o755 })
      await writeFile(target, bytes, { flag: 'wx', mode: 0o644 })
      files[relative] = { sha256: hash(bytes), bytes: bytes.length }
    }
    // Every relative runtime import must be present in this independently
    // extractable kit. No root checkout or implicit node_modules is required.
    for (const relative of kitFiles.filter(name => name.endsWith('.mjs'))) {
      const code = (await readFile(path.join(directory, relative))).toString('utf8')
      for (const match of code.matchAll(/(?:from\s*|import\s*\()\s*['"](\.[^'"]+)['"]/g)) {
        check(kitFiles.includes(path.posix.normalize(path.posix.join(path.posix.dirname(relative), match[1]))))
      }
    }
    const manifest = { schemaVersion: 1, version, sourceSha, treeSha, runId, attempt, platform: 'linux/amd64', files }
    const serialized = JSON.stringify(manifest, null, 2) + '\n'
    await writeFile(path.join(directory, 'MANIFEST.json'), serialized, { flag: 'wx', mode: 0o644 })
    const filename = `myapi-operations-${version}-${sourceSha}.tar.gz`
    const archive = path.join(output, filename)
    await execute('tar', ['--sort=name', '--mtime=@0', '--owner=0', '--group=0', '--numeric-owner', '-czf', archive, '-C', temporary, 'myapi-operations'], { timeout: 30000 })
    const bytes = await readFile(archive)
    const report = { schemaVersion: 1, version, sourceSha, treeSha, runId, attempt, file: filename, sha256: hash(bytes), manifestSha256: hash(serialized), bytes: bytes.length, fileCount: kitFiles.length }
    await writeFile(path.join(output, 'kit-identity.json'), JSON.stringify(report, null, 2) + '\n', { flag: 'wx', mode: 0o644 })
    await chmod(archive, 0o644)
    return report
  } finally { await rm(temporary, { recursive: true, force: true }) }
}

export async function verifyOperationsKit(directory, expected) {
  const bytes = await sourceFile(directory, 'MANIFEST.json')
  check(hash(bytes) === expected.manifestSha256)
  const manifest = JSON.parse(bytes.toString('utf8'))
  check(['version', 'sourceSha', 'treeSha', 'runId', 'attempt'].every(name => manifest[name] === expected[name]) && manifest.schemaVersion === 1 && manifest.platform === 'linux/amd64')
  check(JSON.stringify(Object.keys(manifest.files).sort()) === JSON.stringify([...kitFiles].sort()))
  const observed = []
  async function visit(relative) {
    for (const entry of await readdir(path.join(directory, relative), { withFileTypes: true })) {
      const name = path.posix.join(relative, entry.name)
      check(!entry.isSymbolicLink())
      if (entry.isDirectory()) await visit(name)
      else { check(entry.isFile()); observed.push(name) }
    }
  }
  await visit('')
  check(JSON.stringify(observed.sort()) === JSON.stringify([...kitFiles, 'MANIFEST.json'].sort()))
  for (const relative of kitFiles) {
    const content = await sourceFile(directory, relative)
    check(content.length === manifest.files[relative].bytes && hash(content) === manifest.files[relative].sha256)
  }
  return { passed: true, ...expected, verifiedFiles: kitFiles.length, nodeModulesRequired: false, servicesStarted: false, productionTouched: false }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    check(process.env.GITHUB_ACTIONS === 'true')
    const [action, output] = process.argv.slice(2)
    check(path.isAbsolute(output))
    if (action === 'pack') {
      const report = await buildOperationsKit({ root: await realpath(process.cwd()), output, sourceSha: process.env.GITHUB_SHA, treeSha: process.env.KIT_TREE_SHA, runId: process.env.GITHUB_RUN_ID, attempt: process.env.GITHUB_RUN_ATTEMPT })
      process.stdout.write('OPERATIONS_KIT_IDENTITY ' + JSON.stringify(report) + '\n')
    } else if (action === 'verify') {
      const identity = JSON.parse(await readFile(path.join(output, 'kit-identity.json'), 'utf8'))
      check(identity.file === `myapi-operations-${identity.version}-${identity.sourceSha}.tar.gz` && /^0\.2\.0-beta\.[1-9][0-9]{0,8}$/.test(identity.version) && /^[a-f0-9]{40}$/.test(identity.sourceSha))
      const archive = await readFile(path.join(output, identity.file))
      check(hash(archive) === identity.sha256)
      const target = await mkdtemp(path.join(output, '.verify-'))
      try {
        await execute('tar', ['-xzf', path.join(output, identity.file), '--no-same-owner', '--no-same-permissions', '-C', target], { timeout: 30000 })
        const report = await verifyOperationsKit(path.join(target, 'myapi-operations'), identity)
        // Verify syntax and all relative imports from the extracted kit only.
        for (const relative of kitFiles.filter(name => name.endsWith('.mjs'))) await execute(process.execPath, ['--check', path.join(target, 'myapi-operations', relative)], { timeout: 10000 })
        await execute(process.execPath, ['--input-type=module', '-e', 'await import(process.argv[1]); await import(process.argv[2])', pathToFileURL(path.join(target, 'myapi-operations/tools/operations/daemon.mjs')).href, pathToFileURL(path.join(target, 'myapi-operations/tools/management/mcp.mjs')).href], { timeout: 10000 })
        await execute('sh', ['-n', path.join(target, 'myapi-operations/deploy/operations/install.sh')], { timeout: 10000 })
        await writeFile(path.join(output, 'kit-verification.json'), JSON.stringify(report, null, 2) + '\n', { flag: 'wx', mode: 0o644 })
        process.stdout.write('OPERATIONS_KIT_VERIFIED ' + JSON.stringify(report) + '\n')
      } finally { await rm(target, { recursive: true, force: true }) }
    } else throw new Error('OPERATIONS_KIT_INVALID')
  } catch { process.stderr.write('OPERATIONS_KIT_FAILED\n'); process.exitCode = 1 }
}
