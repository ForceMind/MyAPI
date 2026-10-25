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
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, test } from 'node:test'

// MYAPI_EDITION accepts full, lite, and the Legacy alias lan, ignoring case
// and surrounding whitespace (middleware/edition.go). These tests pin that the
// Docker build, the CLI, and the installer treat lite exactly like lan and
// keep full unchanged.
const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const cli = path.join(repositoryRoot, 'cli/myapi.mjs')
const sessionSecret = 'a'.repeat(64)
const temporaryRoots = []

afterEach(() => {
  for (const root of temporaryRoots.splice(0)) rmSync(root, { recursive: true, force: true })
})

function temporaryRoot() {
  const root = mkdtempSync(path.join(tmpdir(), 'myapi-edition-'))
  temporaryRoots.push(root)
  return root
}

// A minimal deployment checkout: enough for the CLI and installer to read the
// Compose template and the example file, without copying the whole source tree.
function deploymentFixture(settings) {
  const root = temporaryRoot()
  const project = path.join(root, 'project')
  const deploy = path.join(project, 'deploy')
  const bin = path.join(root, 'bin')
  mkdirSync(deploy, { recursive: true })
  mkdirSync(bin)
  for (const name of ['docker-compose.yml', '.env.example', 'install.sh']) {
    copyFileSync(path.join(repositoryRoot, 'deploy', name), path.join(deploy, name))
  }
  copyFileSync(path.join(repositoryRoot, 'VERSION'), path.join(project, 'VERSION'))
  const values = {
    SESSION_SECRET: sessionSecret,
    BATCH_UPDATE_ENABLED: 'false',
    MYAPI_ACCOUNTING_CONFIG_VERSION: '1',
    MYAPI_PORT: '3000',
    ...settings,
  }
  const lines = Object.entries(values)
    .filter(([, value]) => value !== undefined)
    .map(([key, value]) => `${key}=${value}`)
  writeFileSync(path.join(deploy, '.env'), `${lines.join('\n')}\n`, { mode: 0o600 })
  const log = path.join(root, 'docker.log')
  writeFileSync(
    path.join(bin, 'docker'),
    '#!/bin/sh\n' +
      'if [ "$1" = container ] && [ "$2" = ls ]; then exit 0; fi\n' +
      'printf "%s|%s|%s|%s|%s\\n" "${MYAPI_IMAGE-unset}" "${MYAPI_EDITION-unset}" ' +
      '"${MYAPI_SESSION_COOKIE_SECURE-unset}" "${MYAPI_SESSION_COOKIE_TRUSTED_URL-unset}" ' +
      '"${MYAPI_PUBLIC_URL-unset}" >> "$MYAPI_EDITION_TEST_LOG"\n',
    { mode: 0o700 },
  )
  chmodSync(path.join(bin, 'docker'), 0o700)
  const env = {
    ...process.env,
    PATH: `${bin}:${process.env.PATH || ''}`,
    MYAPI_EDITION_TEST_LOG: log,
  }
  return { root, project, envFile: path.join(deploy, '.env'), log, env }
}

function runCli(fixture, args) {
  const result = spawnSync(process.execPath, [cli, ...args], {
    cwd: repositoryRoot,
    encoding: 'utf8',
    env: fixture.env,
  })
  return { status: result.status, output: `${result.stdout}${result.stderr}` }
}

function runInstaller(fixture) {
  const result = spawnSync('bash', [path.join(fixture.project, 'deploy/install.sh')], {
    cwd: fixture.project,
    encoding: 'utf8',
    env: fixture.env,
  })
  return { status: result.status, output: `${result.stdout}${result.stderr}` }
}

function loggedCalls(fixture) {
  try {
    return readFileSync(fixture.log, 'utf8').trim().split('\n').filter(Boolean)
  } catch {
    return []
  }
}

const restrictedSpellings = ['lan', 'lite', 'LITE', 'Lan', ' lite ']

test('Docker frontend build enables the restricted UI for lan and lite only', () => {
  const dockerfile = readFileSync(path.join(repositoryRoot, 'Dockerfile'), 'utf8')
  const instruction = dockerfile
    .split(/\r?\n(?=[A-Z]+ )/)
    .find((candidate) => candidate.startsWith('RUN ') && candidate.includes('VITE_SELF_USE_MINIMAL'))
  assert.ok(instruction, 'frontend build RUN instruction is present')
  // Run the real shell fragment, replacing only the bun invocation and the
  // VERSION path, so the edition normalization itself is what is verified.
  const script = instruction
    .slice('RUN '.length)
    .replaceAll('\\\n', ' ')
    .replace('bun run build', `sh -c 'printf "%s" "$VITE_SELF_USE_MINIMAL"'`)
    .replace('/build/VERSION', '/dev/null')
  const minimalFor = (edition) => {
    const result = spawnSync('sh', ['-c', script], {
      encoding: 'utf8',
      env: { PATH: process.env.PATH, MYAPI_EDITION: edition },
    })
    assert.equal(result.status, 0, result.stderr)
    return result.stdout
  }
  for (const edition of restrictedSpellings) {
    assert.equal(minimalFor(edition), 'true', `edition ${JSON.stringify(edition)}`)
  }
  for (const edition of ['full', 'FULL', '']) {
    assert.equal(minimalFor(edition), 'false', `edition ${JSON.stringify(edition)}`)
  }
  assert.ok(dockerfile.includes('io.myapi.edition="${MYAPI_EDITION}"'), 'image label records the supplied value')
})

test('CLI preflight treats lite like lan for cookies, local origin, and image selection', () => {
  const observed = new Map()
  for (const edition of restrictedSpellings) {
    const fixture = deploymentFixture({ MYAPI_EDITION: edition, MYAPI_PUBLIC_URL: 'https://my-api.example.com' })
    const result = runCli(fixture, ['up', '--project-dir', fixture.project])
    assert.equal(result.status, 0, `${edition}: ${result.output}`)
    const calls = loggedCalls(fixture)
    assert.ok(calls.length > 0)
    // Insecure local cookies, no trusted HTTPS origin list, localhost origin.
    assert.ok(calls.every((line) => line.endsWith('|false||http://localhost:3000')), calls.join('\n'))
    observed.set(edition, calls.map((line) => line.split('|').slice(2).join('|')))
  }
  const reference = observed.get('lan')
  for (const [edition, calls] of observed) assert.deepEqual(calls, reference, edition)
})

test('CLI applies the LAN binding and origin rules to lite and lan identically', () => {
  for (const edition of ['lan', 'lite']) {
    const wildcard = deploymentFixture({
      MYAPI_EDITION: edition,
      MYAPI_PUBLIC_URL: 'http://localhost:3000',
      MYAPI_BIND_ADDRESS: '0.0.0.0',
      MYAPI_ALLOW_LAN: 'false',
    })
    const refusedBind = runCli(wildcard, ['up', '--project-dir', wildcard.project])
    assert.notEqual(refusedBind.status, 0)
    assert.match(refusedBind.output, /deployment preflight failed.*LAN binding is disabled by default/)
    assert.deepEqual(loggedCalls(wildcard), [])

    const publicOrigin = deploymentFixture({ MYAPI_EDITION: edition, MYAPI_PUBLIC_URL: 'http://api.example.test:3000' })
    const refusedOrigin = runCli(publicOrigin, ['up', '--project-dir', publicOrigin.project])
    assert.notEqual(refusedOrigin.status, 0)
    assert.match(refusedOrigin.output, /localhost or private-network origin in LAN edition/)
    assert.deepEqual(loggedCalls(publicOrigin), [])

    const shared = deploymentFixture({
      MYAPI_EDITION: edition,
      MYAPI_PUBLIC_URL: 'http://192.168.1.20:3000',
      MYAPI_BIND_ADDRESS: '192.168.1.20',
      MYAPI_ALLOW_LAN: 'true',
    })
    const accepted = runCli(shared, ['up', '--project-dir', shared.project])
    assert.equal(accepted.status, 0, accepted.output)
  }
})

test('CLI keeps full on HTTPS-only rules and rejects unknown edition values', () => {
  const full = deploymentFixture({ MYAPI_EDITION: 'full', MYAPI_PUBLIC_URL: 'http://localhost:3000' })
  const refusedFull = runCli(full, ['up', '--project-dir', full.project])
  assert.notEqual(refusedFull.status, 0)
  assert.match(refusedFull.output, /exact non-placeholder HTTPS origin/)

  const fullOk = deploymentFixture({ MYAPI_EDITION: 'full', MYAPI_PUBLIC_URL: 'https://api.example.test' })
  assert.equal(runCli(fullOk, ['up', '--project-dir', fullOk.project]).status, 0)
  assert.ok(loggedCalls(fullOk).every((line) => line.includes('|true|https://api.example.test|')))

  for (const edition of ['pro', 'lit', 'lan-lite', 'fulll']) {
    const fixture = deploymentFixture({ MYAPI_EDITION: edition, MYAPI_PUBLIC_URL: 'https://api.example.test' })
    const up = runCli(fixture, ['up', '--project-dir', fixture.project])
    assert.notEqual(up.status, 0, edition)
    assert.match(up.output, /MYAPI_EDITION must be full, lite, or lan/)
    const dryRun = runCli(fixture, ['upgrade', '--project-dir', fixture.project, '--version', 'v0.2.0', '--dry-run'])
    assert.notEqual(dryRun.status, 0, edition)
    assert.match(dryRun.output, /MYAPI_EDITION must be full, lite, or lan/)
    assert.deepEqual(loggedCalls(fixture), [])
  }
})

test('upgrade selects the myapi-lan repository for lite and lan and reports the normalized edition', () => {
  const expected = [
    ['full', 'full', 'https://api.example.test', 'ghcr.io/forcemind/myapi:v0.2.0'],
    ['FULL', 'full', 'https://api.example.test', 'ghcr.io/forcemind/myapi:v0.2.0'],
    ['lan', 'lan', 'http://localhost:3000', 'ghcr.io/forcemind/myapi-lan:v0.2.0'],
    ['lite', 'lite', 'http://localhost:3000', 'ghcr.io/forcemind/myapi-lan:v0.2.0'],
    ['LITE', 'lite', 'http://localhost:3000', 'ghcr.io/forcemind/myapi-lan:v0.2.0'],
    [' Lan ', 'lan', 'http://localhost:3000', 'ghcr.io/forcemind/myapi-lan:v0.2.0'],
  ]
  for (const [spelling, edition, publicUrl, targetImage] of expected) {
    const fixture = deploymentFixture({ MYAPI_EDITION: spelling, MYAPI_PUBLIC_URL: publicUrl })
    const before = readFileSync(fixture.envFile, 'utf8')
    const result = runCli(fixture, ['upgrade', '--project-dir', fixture.project, '--version', 'v0.2.0', '--dry-run', '--json'])
    assert.equal(result.status, 0, `${spelling}: ${result.output}`)
    const plan = JSON.parse(result.output)
    assert.equal(plan.edition, edition, spelling)
    assert.equal(plan.targetImage, targetImage, spelling)
    assert.equal(readFileSync(fixture.envFile, 'utf8'), before, 'dry-run does not rewrite the edition value')
  }
})

test('lan commands accept a lite project and never rewrite its edition value', () => {
  const fixture = deploymentFixture({
    MYAPI_EDITION: 'lite',
    MYAPI_PUBLIC_URL: 'http://localhost:3000',
    MYAPI_BIND_ADDRESS: '127.0.0.1',
    MYAPI_ALLOW_LAN: 'false',
  })
  const status = runCli(fixture, ['lan', 'status', '--project-dir', fixture.project])
  assert.equal(status.status, 0, status.output)

  const shared = runCli(fixture, [
    'lan', 'start', '--project-dir', fixture.project,
    '--bind-address', '192.168.1.20', '--allow-lan',
  ])
  assert.equal(shared.status, 0, shared.output)
  const env = readFileSync(fixture.envFile, 'utf8')
  assert.match(env, /^MYAPI_EDITION=lite$/m)
  assert.match(env, /^MYAPI_BIND_ADDRESS=192\.168\.1\.20$/m)
  assert.match(env, /^MYAPI_ALLOW_LAN=true$/m)

  const full = deploymentFixture({ MYAPI_EDITION: 'full', MYAPI_PUBLIC_URL: 'https://api.example.test' })
  const refused = runCli(full, ['lan', 'status', '--project-dir', full.project])
  assert.notEqual(refused.status, 0)
  assert.match(refused.output, /is not configured for MYAPI_EDITION=lan or lite/)
})

test('lan init keeps writing the Legacy lan value', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'lan')
  const result = spawnSync(process.execPath, [cli, 'lan', 'init', project], { cwd: repositoryRoot, encoding: 'utf8' })
  assert.equal(result.status, 0, result.stderr)
  assert.match(readFileSync(path.join(project, 'deploy/.env'), 'utf8'), /^MYAPI_EDITION=lan$/m)
})

test('help documents lite as an accepted edition value', () => {
  const result = spawnSync(process.execPath, [cli, 'help'], { cwd: repositoryRoot, encoding: 'utf8' })
  assert.equal(result.status, 0)
  assert.match(result.stdout, /MYAPI_EDITION=lite/)
  assert.match(result.stdout, /lan.*(?:Legacy|alias)/is)
})

test('installer treats lite like lan and keeps full unchanged', () => {
  for (const edition of restrictedSpellings) {
    const fixture = deploymentFixture({ MYAPI_EDITION: edition, MYAPI_PUBLIC_URL: 'https://my-api.example.com' })
    const result = runInstaller(fixture)
    assert.equal(result.status, 0, `${edition}: ${result.output}`)
    const calls = loggedCalls(fixture)
    assert.ok(calls.length > 0)
    const version = readFileSync(path.join(repositoryRoot, 'VERSION'), 'utf8').trim()
    assert.ok(
      calls.every((line) => line.startsWith(`ghcr.io/forcemind/myapi-lan:v${version}|`) && line.endsWith('|false||http://localhost:3000')),
      calls.join('\n'),
    )
  }

  const full = deploymentFixture({ MYAPI_EDITION: 'full', MYAPI_PUBLIC_URL: 'https://api.example.test' })
  assert.equal(runInstaller(full).status, 0)
  const version = readFileSync(path.join(repositoryRoot, 'VERSION'), 'utf8').trim()
  assert.ok(loggedCalls(full).every((line) => line.startsWith(`ghcr.io/forcemind/myapi:v${version}|`)))

  for (const edition of ['lan', 'lite']) {
    const publicOrigin = deploymentFixture({ MYAPI_EDITION: edition, MYAPI_PUBLIC_URL: 'http://api.example.test:3000' })
    const refused = runInstaller(publicOrigin)
    assert.notEqual(refused.status, 0)
    assert.match(refused.output, /localhost, loopback, or private-network origin in LAN edition/)
    assert.deepEqual(loggedCalls(publicOrigin), [])
  }
})

test('installer rejects unknown edition values before any Docker call', () => {
  for (const edition of ['pro', 'lit', 'lan-lite']) {
    const fixture = deploymentFixture({ MYAPI_EDITION: edition, MYAPI_PUBLIC_URL: 'https://api.example.test' })
    const result = runInstaller(fixture)
    assert.notEqual(result.status, 0, edition)
    assert.match(result.output, /MYAPI_EDITION must be full, lite, or lan/)
    assert.deepEqual(loggedCalls(fixture), [])
  }
})
