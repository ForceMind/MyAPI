/*
MyAPI distribution and self-hosting tooling.
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import {
  chmodSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
  statSync,
  writeFileSync,
} from 'node:fs'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, test } from 'node:test'

// First-run guidance (`init`, `configure`, `lan init`) and the read-only
// `doctor`, exercised through the real CLI entry point. Docker is replaced by
// scripts in a temporary PATH; nothing here needs a real Docker or network.
const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const cli = path.join(repositoryRoot, 'cli/myapi.mjs')
const sessionSecretPattern = /[a-f0-9]{48,}/
const temporaryRoots = []

afterEach(() => {
  for (const root of temporaryRoots.splice(0)) rmSync(root, { recursive: true, force: true })
})

function temporaryRoot() {
  const root = mkdtempSync(path.join(tmpdir(), 'myapi-bootstrap-'))
  temporaryRoots.push(root)
  return root
}

function runCli(args, env = process.env) {
  const result = spawnSync(process.execPath, [cli, ...args], { cwd: repositoryRoot, encoding: 'utf8', env })
  return { status: result.status, stdout: result.stdout, stderr: result.stderr, output: `${result.stdout}${result.stderr}` }
}

function envValue(project, key) {
  const match = new RegExp(`^${key}=(.*)$`, 'm').exec(readFileSync(path.join(project, 'deploy/.env'), 'utf8'))
  return match ? match[1] : undefined
}

async function freePort() {
  const server = createServer()
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  const { port } = server.address()
  await new Promise((resolve) => server.close(resolve))
  return port
}

function fileTree(directory, prefix = '') {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const relative = path.join(prefix, entry.name)
    return entry.isDirectory() ? [relative + '/', ...fileTree(path.join(directory, entry.name), relative)] : [relative]
  }).sort()
}

// A configured deployment without copying the whole source tree.
function doctorFixture({ settings = {}, docker = 'ok' } = {}) {
  const root = temporaryRoot()
  const project = path.join(root, 'project')
  const deploy = path.join(project, 'deploy')
  const bin = path.join(root, 'bin')
  mkdirSync(deploy, { recursive: true })
  mkdirSync(bin)
  for (const name of ['docker-compose.yml', '.env.example']) {
    copyFileSync(path.join(repositoryRoot, 'deploy', name), path.join(deploy, name))
  }
  const secret = 'b'.repeat(64)
  const values = {
    MYAPI_EDITION: 'full',
    MYAPI_PUBLIC_URL: 'https://api.example.test',
    MYAPI_BIND_ADDRESS: '127.0.0.1',
    MYAPI_ALLOW_LAN: 'false',
    MYAPI_PORT: '3000',
    SESSION_SECRET: secret,
    BATCH_UPDATE_ENABLED: 'false',
    MYAPI_ACCOUNTING_CONFIG_VERSION: '1',
    ...settings,
  }
  writeFileSync(
    path.join(deploy, '.env'),
    `${Object.entries(values).filter(([, value]) => value !== undefined).map(([key, value]) => `${key}=${value}`).join('\n')}\n`,
    { mode: 0o600 },
  )
  if (docker !== 'missing') {
    const composeVersion = docker === 'old-compose' ? 'v2.10.0' : 'v2.29.1'
    writeFileSync(
      path.join(bin, 'docker'),
      '#!/bin/sh\n' +
        'case "$*" in\n' +
        '  "version --format {{.Server.Version}}") echo 27.0.3 ;;\n' +
        `  "compose version --short") echo ${composeVersion} ;;\n` +
        '  container\\ ls*) ;;\n' +
        '  compose*config*) ;;\n' +
        '  *) echo "unexpected docker call: $*" >&2; exit 9 ;;\n' +
        'esac\n',
      { mode: 0o700 },
    )
    chmodSync(path.join(bin, 'docker'), 0o700)
  }
  // The temporary bin directory is the whole PATH, so a missing fake means
  // there is no docker executable at all.
  const env = { ...process.env, PATH: bin }
  return { root, project, deploy, secret, env }
}

test('doctor reports a healthy deployment without writing anything', async () => {
  const fixture = doctorFixture({ settings: { MYAPI_PORT: String(await freePort()) } })
  const before = fileTree(fixture.project)
  const result = runCli(['doctor', '--project-dir', fixture.project], fixture.env)
  assert.equal(result.status, 0, result.output)
  for (const name of ['MYAPI_EDITION', 'Docker daemon', 'Docker Compose', 'Listen port', 'Data directory', 'Data disk space', 'Log directory', 'Log disk space', 'Access scope', 'Legacy NEW_API_* settings']) {
    assert.match(result.stdout, new RegExp(`^(?:OK|WARN)  ${name.replace(/[*]/g, '\\*')}: `, 'm'), name)
  }
  assert.match(result.stdout, /^OK  Docker Compose: 2\.29\.1$/m)
  assert.match(result.stdout, /^OK  Docker Compose configuration: valid$/m)
  assert.deepEqual(fileTree(fixture.project), before, 'doctor created or removed files')
  assert.equal(result.output.includes(fixture.secret), false, 'doctor printed the session secret')
})

test('doctor degrades to a warning when docker is not installed', async () => {
  const fixture = doctorFixture({ docker: 'missing', settings: { MYAPI_PORT: String(await freePort()) } })
  const result = runCli(['doctor', '--project-dir', fixture.project], fixture.env)
  assert.equal(result.status, 0, result.output)
  assert.match(result.stdout, /^WARN  Docker daemon: docker was not found on PATH/m)
  assert.match(result.stdout, /fix: install Docker Desktop or Docker Engine/)
  assert.match(result.stdout, /^WARN  Docker Compose configuration: skipped/m)
  assert.doesNotMatch(result.output, /FAIL/)
  assert.doesNotMatch(result.output, /TypeError|ENOENT|at .*\.mjs/)
})

test('doctor fails an outdated Docker Compose with a fix and a non-zero exit', async () => {
  const fixture = doctorFixture({ docker: 'old-compose', settings: { MYAPI_PORT: String(await freePort()) } })
  const result = runCli(['doctor', '--project-dir', fixture.project], fixture.env)
  assert.equal(result.status, 1)
  assert.match(result.stdout, /^FAIL  Docker Compose: 2\.10\.0 is older than the required v2\.17/m)
  assert.match(result.stdout, /fix: install Docker Compose v2\.17 or newer/)
})

test('doctor warns about a port that another process already holds', async () => {
  const server = createServer()
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  try {
    const { port } = server.address()
    const fixture = doctorFixture({ settings: { MYAPI_PORT: String(port) } })
    const result = runCli(['doctor', '--project-dir', fixture.project], fixture.env)
    assert.equal(result.status, 0, result.output)
    assert.match(result.stdout, new RegExp(`^WARN  Listen port: 127\\.0\\.0\\.1:${port} is already in use by another process`, 'm'))
    assert.match(result.stdout, /fix: choose a free port by setting MYAPI_PORT/)
  } finally {
    await new Promise((resolve) => server.close(resolve))
  }
})

test('doctor rejects an unknown edition, flags legacy variables and inconsistent access settings', async () => {
  const port = String(await freePort())
  const badEdition = doctorFixture({ settings: { MYAPI_EDITION: 'pro', MYAPI_PORT: port } })
  const bad = runCli(['doctor', '--project-dir', badEdition.project], badEdition.env)
  assert.equal(bad.status, 1)
  assert.match(bad.stdout, /^FAIL  MYAPI_EDITION: "pro" is not a supported edition/m)
  assert.match(bad.stdout, /fix: set MYAPI_EDITION to full, lite, or lan/)

  const legacy = doctorFixture({ settings: { MYAPI_PORT: undefined, NEW_API_PORT: port } })
  const migrated = runCli(['doctor', '--project-dir', legacy.project], legacy.env)
  assert.equal(migrated.status, 0, migrated.output)
  assert.match(migrated.stdout, /^WARN  Legacy NEW_API_\* settings: NEW_API_PORT still in use/m)
  assert.match(migrated.stdout, /fix: run "myapi migrate --project-dir /)

  const shared = doctorFixture({ settings: { MYAPI_EDITION: 'lite', MYAPI_PUBLIC_URL: 'http://localhost:3000', MYAPI_BIND_ADDRESS: '127.0.0.1', MYAPI_ALLOW_LAN: 'true', MYAPI_PORT: port } })
  const mismatch = runCli(['doctor', '--project-dir', shared.project], shared.env)
  assert.equal(mismatch.status, 0, mismatch.output)
  assert.match(mismatch.stdout, /^WARN  Access scope: MYAPI_ALLOW_LAN=true has no effect/m)
})

test('doctor tells a fresh checkout how to create its environment file', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'project')
  mkdirSync(path.join(project, 'deploy'), { recursive: true })
  const result = runCli(['doctor', '--project-dir', project], { ...process.env, PATH: path.join(root, 'empty') })
  assert.equal(result.status, 1)
  assert.match(result.stdout, /^FAIL  Environment file: /m)
  assert.match(result.stdout, /fix: run myapi configure --project-dir .* --public-url https:\/\/<your-domain>/)
})

test('init with --public-url prepares a ready-to-start Full configuration and hides the secret', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'full')
  const result = runCli(['init', project, '--public-url', 'https://myapi.example.test'])
  assert.equal(result.status, 0, result.output)

  assert.equal(existsSync(path.join(project, 'go.mod')), true)
  assert.equal(envValue(project, 'MYAPI_PUBLIC_URL'), 'https://myapi.example.test')
  assert.match(envValue(project, 'SESSION_SECRET'), /^[a-f0-9]{64}$/)
  assert.equal(envValue(project, 'BATCH_UPDATE_ENABLED'), 'false')
  assert.equal(envValue(project, 'MYAPI_ACCOUNTING_CONFIG_VERSION'), '1')
  assert.equal(envValue(project, 'MYAPI_EDITION'), 'full')
  assert.equal(envValue(project, 'MYAPI_BIND_ADDRESS'), '127.0.0.1')
  assert.equal(statSync(path.join(project, 'deploy/.env')).mode & 0o777, 0o600)

  assert.match(result.stdout, /MyAPI source initialized at /)
  assert.match(result.stdout, new RegExp(`myapi doctor --project-dir ${project.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}`))
  assert.match(result.stdout, /myapi up --project-dir /)
  assert.equal(result.output.includes(envValue(project, 'SESSION_SECRET')), false, 'the secret was printed')
  assert.doesNotMatch(result.output, sessionSecretPattern)

  // The destination is no longer empty, so the generated configuration cannot be overwritten by a second init.
  const again = runCli(['init', project, '--public-url', 'https://other.example.test'])
  assert.notEqual(again.status, 0)
  assert.match(again.output, /refusing to overwrite non-empty directory/)
  assert.equal(envValue(project, 'MYAPI_PUBLIC_URL'), 'https://myapi.example.test')
})

test('init without --public-url asks for the origin instead of inventing one', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'full')
  const result = runCli(['init', project])
  assert.equal(result.status, 0, result.output)
  assert.equal(existsSync(path.join(project, 'deploy/.env')), false)
  assert.match(result.stdout, /myapi configure --project-dir .* --public-url https:\/\/<your-domain>/)
  assert.match(result.stdout, /public HTTPS origin is required/)
  assert.match(result.stdout, /myapi lan init/)
})

test('init rejects a public URL that Full would refuse before creating anything', () => {
  const root = temporaryRoot()
  for (const url of ['http://localhost:3000', 'https://my-api.example.com', 'https://localhost', 'ftp://host.test', 'not a url']) {
    const project = path.join(root, `rejected-${Buffer.from(url).toString('hex').slice(0, 12)}`)
    const result = runCli(['init', project, '--public-url', url])
    assert.notEqual(result.status, 0, url)
    assert.match(result.output, /--public-url must be an exact non-placeholder HTTPS origin/)
    assert.equal(existsSync(project), false, `${url} left a directory behind`)
  }
})

test('init accepts the directory after the option as well as before it', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'option-first')
  const result = runCli(['init', '--public-url', 'https://myapi.example.test', project])
  assert.equal(result.status, 0, result.output)
  assert.equal(envValue(project, 'MYAPI_PUBLIC_URL'), 'https://myapi.example.test')
})

test('configure explains the placeholder public URL and keeps the generated secret private', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  assert.equal(runCli(['init', project]).status, 0)

  const result = runCli(['configure', '--project-dir', project])
  assert.equal(result.status, 0, result.output)
  assert.equal(envValue(project, 'MYAPI_PUBLIC_URL'), 'https://my-api.example.com')
  assert.match(result.stdout, /MYAPI_PUBLIC_URL is still the placeholder https:\/\/my-api\.example\.com/)
  assert.match(result.stdout, /myapi up will refuse to start until you set your own HTTPS origin/)
  assert.match(result.stdout, /myapi doctor --project-dir /)
  assert.equal(result.output.includes(envValue(project, 'SESSION_SECRET')), false)

  const refused = runCli(['configure', '--project-dir', project, '--public-url', 'https://myapi.example.test', '--force'])
  assert.notEqual(refused.status, 0)
  assert.match(refused.output, /existing deployment configuration is preserved/)
  assert.equal(envValue(project, 'MYAPI_PUBLIC_URL'), 'https://my-api.example.com', 'an existing .env is never rewritten')
})

test('configure warns when the supplied origin is not valid for Full but still writes it', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  assert.equal(runCli(['init', project]).status, 0)
  const result = runCli(['configure', '--project-dir', project, '--public-url', 'http://localhost:3000'])
  assert.equal(result.status, 0, result.output)
  assert.match(result.stderr, /MYAPI_PUBLIC_URL .* is not an exact HTTPS origin; Full edition will refuse to start/)
  assert.equal(envValue(project, 'MYAPI_PUBLIC_URL'), 'http://localhost:3000')
})

test('lan init keeps its loopback defaults and prints the next steps without the secret', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'lite')
  const result = runCli(['lan', 'init', project])
  assert.equal(result.status, 0, result.output)
  assert.equal(envValue(project, 'MYAPI_BIND_ADDRESS'), '127.0.0.1')
  assert.equal(envValue(project, 'MYAPI_ALLOW_LAN'), 'false')
  assert.equal(envValue(project, 'MYAPI_PUBLIC_URL'), 'http://localhost:3000')
  assert.equal(envValue(project, 'MYAPI_SESSION_COOKIE_SECURE'), 'false')
  assert.equal(envValue(project, 'MYAPI_EDITION'), 'lan')
  assert.match(envValue(project, 'SESSION_SECRET'), /^[a-f0-9]{64}$/)
  assert.match(result.stdout, /myapi doctor --project-dir /)
  assert.match(result.stdout, /myapi lan start --project-dir /)
  assert.equal(result.output.includes(envValue(project, 'SESSION_SECRET')), false)
})
