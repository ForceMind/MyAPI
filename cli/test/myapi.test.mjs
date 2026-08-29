/*
MyAPI distribution and self-hosting tooling.
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
  writeFileSync,
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

test('version exposes the MyAPI distribution identity', () => {
  const version = JSON.parse(runCli('version', '--json'))

  assert.equal(version.package, '@forcemind/myapi')
  assert.equal(version.version, '0.1.1')
  assert.equal(version.distribution, 'MyAPI')
  assert.equal(version.machineSlug, 'my-api')
  assert.doesNotMatch(JSON.stringify(version), /New API|QuantumNous/)
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
  assert.match(env, /^MYAPI_PUBLIC_URL=https:\/\/myapi\.example\.test$/m)
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
  assert.match(env, new RegExp(`^MYAPI_DATA_DIR=${data}$`, 'm'))
  assert.match(env, new RegExp(`^MYAPI_LOGS_DIR=${logs}$`, 'm'))
  assert.equal(existsSync(data), true)
  assert.equal(existsSync(logs), true)
})

test('legacy deployment variables are accepted and can be migrated explicitly', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  runCli('init', project)
  runCli('configure', '--project-dir', project, '--public-url', 'https://myapi.example.test')

  const envPath = path.join(project, 'deploy/.env')
  let env = readFileSync(envPath, 'utf8')
  env = env
    .replace(/^MYAPI_IMAGE=.*$/m, 'NEW_API_IMAGE=local/legacy-api:test')
    .replace(/^MYAPI_PORT=.*$/m, 'NEW_API_PORT=3456')
    .replace(/^MYAPI_PUBLIC_URL=.*$/m, 'NEW_API_PUBLIC_URL=https://legacy.example.test')
    .replace(/^MYAPI_DATA_DIR=.*$/m, 'NEW_API_DATA_DIR=./legacy-data')
    .replace(/^MYAPI_LOGS_DIR=.*$/m, 'NEW_API_LOGS_DIR=./legacy-logs')
  writeFileSync(envPath, env, { mode: 0o600 })

  runCli('migrate', '--project-dir', project)

  const migrated = readFileSync(envPath, 'utf8')
  assert.match(migrated, /^MYAPI_IMAGE=local\/legacy-api:test$/m)
  assert.match(migrated, /^MYAPI_PORT=3456$/m)
  assert.match(migrated, /^MYAPI_PUBLIC_URL=https:\/\/legacy\.example\.test$/m)
  assert.match(migrated, /^MYAPI_DATA_DIR=\.\/legacy-data$/m)
  assert.match(migrated, /^MYAPI_LOGS_DIR=\.\/legacy-logs$/m)
  assert.match(migrated, /^# Legacy NEW_API_IMAGE migrated to MYAPI_IMAGE:/m)
  assert.match(migrated, /^# Legacy NEW_API_PUBLIC_URL migrated to MYAPI_PUBLIC_URL:/m)
})
