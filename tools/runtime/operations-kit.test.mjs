import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
import { mkdir, mkdtemp, readFile, rm, symlink, writeFile } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'
import { buildOperationsKit, kitFiles, verifyOperationsKit } from './operations-kit.mjs'

const execute = promisify(execFile)
const repository = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const identity = { sourceSha: 'a'.repeat(40), treeSha: 'b'.repeat(40), runId: '123', attempt: '1' }

test('offline kit preserves every intended file and rejects tampering and unlisted credentials', async t => {
  const directory = await mkdtemp(path.join(os.tmpdir(), 'myapi-kit-'))
  t.after(() => rm(directory, { recursive: true, force: true }))
  const output = path.join(directory, 'package')
  const report = await buildOperationsKit({ root: await import('node:fs/promises').then(fs => fs.realpath(repository)), output, ...identity })
  assert.equal(report.fileCount, kitFiles.length)
  assert.ok(report.bytes > 0)
  const extracted = path.join(directory, 'extracted')
  await mkdir(extracted)
  await execute('tar', ['-xzf', path.join(output, report.file), '-C', extracted])
  const root = path.join(extracted, 'myapi-operations')
  const verified = await verifyOperationsKit(root, report)
  assert.equal(verified.passed, true)
  assert.equal(verified.servicesStarted, false)
  assert.equal(verified.productionTouched, false)
  assert.ok(kitFiles.includes('tools/runtime/smoke-image-bundle.mjs'))
  assert.ok(kitFiles.includes('tools/management/mcp.mjs'))
  assert.ok(kitFiles.includes('deploy/operations/install.sh'))
  assert.ok(kitFiles.every(name => !/\.test\.|qualification|journal|\.env|github\.token|bridge\.token|tls\.key/.test(name)))
  const daemon = path.join(root, 'tools/operations/daemon.mjs')
  const original = await readFile(daemon)
  await writeFile(daemon, original.toString('utf8') + '\n// tampered\n')
  await assert.rejects(verifyOperationsKit(root, report))
  await writeFile(daemon, original)
  await writeFile(path.join(root, 'unexpected-secret'), 'must-not-be-in-delivery')
  await assert.rejects(verifyOperationsKit(root, report))
  await rm(path.join(root, 'unexpected-secret'))
  await rm(daemon)
  await symlink(path.join(repository, 'tools/operations/daemon.mjs'), daemon)
  await assert.rejects(verifyOperationsKit(root, report))
})
