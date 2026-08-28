/*
MyAPI distribution tooling for the New API based custom source release.
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  statSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, test } from 'node:test'

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '../..'
)
const cli = path.join(repositoryRoot, 'cli/myapi.mjs')
const temporaryRoots = []

function runCli(...args) {
  return execFileSync(process.execPath, [cli, ...args], {
    cwd: repositoryRoot,
    encoding: 'utf8',
  })
}

function temporaryRoot() {
  const root = mkdtempSync(path.join(tmpdir(), 'myapi-cli-'))
  temporaryRoots.push(root)
  return root
}

afterEach(() => {
  for (const root of temporaryRoots.splice(0)) {
    rmSync(root, { recursive: true, force: true })
  }
})

test('version exposes the distribution and upstream identity', () => {
  const version = JSON.parse(runCli('version', '--json'))

  assert.equal(version.package, '@forcemind/myapi')
  assert.equal(version.version, '0.1.0')
  assert.match(version.upstream, /New API/)
})

test('init copies source without runtime data and configure protects secrets', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')

  runCli('init', project)

  assert.equal(existsSync(path.join(project, 'go.mod')), true)
  assert.equal(existsSync(path.join(project, 'web/public/myapi-logo-v1.png')), true)
  assert.equal(
    existsSync(path.join(project, 'web/src/features/usage-logs/data/schema.ts')),
    true
  )
  assert.equal(existsSync(path.join(project, '.git')), false)
  assert.equal(existsSync(path.join(project, 'deploy/.env')), false)
  const initializedPackage = JSON.parse(
    readFileSync(path.join(project, 'package.json'), 'utf8')
  )
  assert.equal(initializedPackage.private, true)
  assert.equal('publishConfig' in initializedPackage, false)
  assert.match(
    readFileSync(path.join(project, '.gitignore'), 'utf8'),
    /\*\*\/\.env/
  )
  assert.match(
    readFileSync(path.join(project, '.npmignore'), 'utf8'),
    /\*\*\/\.env/
  )
  assert.match(
    readFileSync(path.join(project, '.dockerignore'), 'utf8'),
    /\*\*\/\.env/
  )

  execFileSync('git', ['init', '--quiet'], { cwd: project })
  assert.equal(
    execFileSync('git', ['check-ignore', 'deploy/.env'], {
      cwd: project,
      encoding: 'utf8',
    }).trim(),
    'deploy/.env'
  )
  assert.throws(
    () =>
      execFileSync(
        'git',
        ['check-ignore', 'web/src/features/usage-logs/data/schema.ts'],
        { cwd: project, stdio: 'pipe' }
      ),
    (error) =>
      Boolean(
        error &&
          typeof error === 'object' &&
          'status' in error &&
          error.status === 1
      )
  )

  runCli(
    'configure',
    '--project-dir',
    project,
    '--public-url',
    'https://myapi.example.test'
  )

  const envPath = path.join(project, 'deploy/.env')
  const env = readFileSync(envPath, 'utf8')
  assert.match(env, /^NEW_API_PUBLIC_URL=https:\/\/myapi\.example\.test$/m)
  assert.match(env, /^MYAPI_BRAND_NAME=MyAPI$/m)
  assert.match(env, /^MYAPI_BRAND_LOGO=\/myapi-logo-v1\.png$/m)
  assert.match(env, /^SESSION_SECRET=[a-f0-9]{64}$/m)
  assert.equal(statSync(envPath).mode & 0o777, 0o600)
})

test('argument validation and deployment preflight reject unsafe input', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  runCli('init', project)
  runCli('configure', '--project-dir', project)

  assert.throws(
    () => runCli('configure', '--project-dir'),
    /requires a value/
  )
  assert.throws(() => runCli('doctor', '--unknown'), /unknown option/)
  assert.throws(
    () => runCli('up', '--project-dir', project),
    /deployment preflight failed/
  )
})

test('adopt records existing absolute data paths without moving them', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  const data = path.join(root, 'existing-data')
  const logs = path.join(root, 'existing-logs')
  mkdirSync(data)
  mkdirSync(logs)
  runCli('init', project)
  runCli(
    'configure',
    '--project-dir',
    project,
    '--public-url',
    'https://myapi.example.test'
  )

  runCli(
    'adopt',
    '--project-dir',
    project,
    '--data-dir',
    data,
    '--logs-dir',
    logs
  )

  const env = readFileSync(path.join(project, 'deploy/.env'), 'utf8')
  assert.match(env, new RegExp(`^NEW_API_DATA_DIR=${data}$`, 'm'))
  assert.match(env, new RegExp(`^NEW_API_LOGS_DIR=${logs}$`, 'm'))
  assert.equal(existsSync(data), true)
  assert.equal(existsSync(logs), true)
})
