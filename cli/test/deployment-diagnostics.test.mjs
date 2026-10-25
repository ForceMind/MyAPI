/*
MyAPI distribution and self-hosting tooling.
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import assert from 'node:assert/strict'
import { test } from 'node:test'

import {
  MIN_FREE_BYTES,
  createDiagnosticsEnvironment,
  parseComposeVersion,
  runDeploymentDiagnostics,
} from '../lib/deployment-diagnostics.mjs'

const GIB = 1024 ** 3
const dockerVersionArguments = 'version --format {{.Server.Version}}'
const composeVersionArguments = 'compose version --short'
const runningContainerArguments = 'container ls --filter name=^/my-api$ --filter status=running --format {{.ID}}'

// Every probe is injected: the fake records external commands and reports
// fixed answers, so no test depends on the host's Docker, ports, or disks.
function fakeEnvironment(overrides = {}) {
  const calls = []
  const answers = {
    [dockerVersionArguments]: { status: 0, stdout: '27.0.3\n', stderr: '' },
    [composeVersionArguments]: { status: 0, stdout: 'v2.29.1\n', stderr: '' },
    [runningContainerArguments]: { status: 0, stdout: '', stderr: '' },
    ...overrides.answers,
  }
  const environment = {
    calls,
    exec(command, args) {
      calls.push([command, ...args].join(' '))
      assert.equal(command, 'docker', 'only docker is ever executed')
      const answer = answers[args.join(' ')]
      assert.ok(answer, `unexpected command: docker ${args.join(' ')}`)
      return answer
    },
    probePort: overrides.probePort ?? (async () => ({ state: 'free' })),
    inspectDirectory: overrides.inspectDirectory ?? ((directory) => ({ kind: 'directory', nearest: directory, writable: true })),
    freeBytes: overrides.freeBytes ?? (() => 5 * GIB),
  }
  return environment
}

function baseInput(overrides = {}) {
  return {
    projectDir: '/srv/myapi',
    rawEdition: 'full',
    bindAddress: '127.0.0.1',
    port: 3000,
    allowLan: 'false',
    publicUrl: 'https://api.example.test',
    publicUrlIsLocal: false,
    dataDir: '/srv/myapi/deploy/data',
    logsDir: '/srv/myapi/deploy/logs',
    legacyKeys: [],
    ...overrides,
  }
}

async function diagnose(inputOverrides = {}, environmentOverrides = {}) {
  const environment = fakeEnvironment(environmentOverrides)
  const result = await runDeploymentDiagnostics(baseInput(inputOverrides), environment)
  const byName = Object.fromEntries(result.checks.map((check) => [check.name, check]))
  return { ...result, byName, environment }
}

test('compose versions are parsed from plain, v-prefixed and desktop-suffixed output', () => {
  assert.deepEqual(parseComposeVersion('v2.24.1\n'), [2, 24, 1])
  assert.deepEqual(parseComposeVersion('2.17.0-desktop.1'), [2, 17, 0])
  assert.deepEqual(parseComposeVersion('v2.20'), [2, 20, 0])
  assert.equal(parseComposeVersion('docker-compose version unknown'), null)
  assert.equal(parseComposeVersion(''), null)
})

test('a healthy host reports every check as ok', async () => {
  const { checks, dockerFound, byName } = await diagnose()
  assert.equal(dockerFound, true)
  assert.deepEqual(checks.map((check) => check.status), checks.map(() => 'ok'))
  assert.deepEqual(Object.keys(byName), [
    'MYAPI_EDITION',
    'Docker daemon',
    'Docker Compose',
    'Listen port',
    'Data directory',
    'Data disk space',
    'Log directory',
    'Log disk space',
    'Access scope',
    'Legacy NEW_API_* settings',
  ])
  assert.equal(byName['Docker daemon'].detail, '27.0.3')
  assert.equal(byName['Docker Compose'].detail, '2.29.1')
})

test('MYAPI_EDITION accepts full, lite and the Legacy lan alias in any case and rejects the rest', async () => {
  for (const [raw, detail] of [['full', 'full'], ['', 'full'], [undefined, 'full'], [' LITE ', 'lite'], ['Lan', 'lan (Legacy alias of lite)']]) {
    const { byName } = await diagnose({ rawEdition: raw })
    assert.equal(byName.MYAPI_EDITION.status, 'ok', String(raw))
    assert.equal(byName.MYAPI_EDITION.detail, detail)
  }
  for (const raw of ['pro', 'lit', 'lan-lite']) {
    const { byName } = await diagnose({ rawEdition: raw })
    assert.equal(byName.MYAPI_EDITION.status, 'fail', raw)
    assert.match(byName.MYAPI_EDITION.detail, new RegExp(raw))
    assert.match(byName.MYAPI_EDITION.fix, /full, lite, or lan/)
  }
})

test('a missing docker executable degrades to one warning and skips the Docker follow-ups', async () => {
  const environment = fakeEnvironment({
    answers: { [dockerVersionArguments]: { status: null, stdout: '', stderr: '', errorCode: 'ENOENT' } },
  })
  const { checks, dockerFound } = await runDeploymentDiagnostics(baseInput(), environment)
  const byName = Object.fromEntries(checks.map((check) => [check.name, check]))
  assert.equal(dockerFound, false)
  assert.equal(byName['Docker daemon'].status, 'warn')
  assert.match(byName['Docker daemon'].detail, /docker was not found on PATH/)
  assert.match(byName['Docker daemon'].fix, /install Docker/i)
  assert.equal('Docker Compose' in byName, false)
  assert.deepEqual(environment.calls, [`docker ${dockerVersionArguments}`])
  assert.ok(checks.every((check) => check.status !== 'fail'), 'no docker is not a failure of the other checks')
})

test('a hung docker command fails with a timeout hint instead of waiting forever', async () => {
  const { byName } = await diagnose({}, {
    answers: {
      [dockerVersionArguments]: { status: null, stdout: '', stderr: '', errorCode: 'ETIMEDOUT' },
      [composeVersionArguments]: { status: null, stdout: '', stderr: '', errorCode: 'ETIMEDOUT' },
    },
  })
  assert.equal(byName['Docker daemon'].status, 'fail')
  assert.match(byName['Docker daemon'].detail, /did not answer within 10 seconds/)
  assert.equal(byName['Docker Compose'].status, 'fail')
  assert.match(byName['Docker Compose'].detail, /did not answer within 10 seconds/)
})

test('a stopped Docker daemon fails with a start hint while Compose is still checked', async () => {
  const { byName } = await diagnose({}, {
    answers: { [dockerVersionArguments]: { status: 1, stdout: '', stderr: 'Cannot connect to the Docker daemon' } },
  })
  assert.equal(byName['Docker daemon'].status, 'fail')
  assert.match(byName['Docker daemon'].fix, /start Docker/i)
  assert.equal(byName['Docker Compose'].status, 'ok')
})

test('Docker Compose must be v2.17 or newer', async () => {
  for (const [output, status] of [
    ['v2.16.9\n', 'fail'],
    ['1.29.2\n', 'fail'],
    ['v2.17.0\n', 'ok'],
    ['2.20.3-desktop.1\n', 'ok'],
    ['v3.0.0\n', 'ok'],
    ['unknown\n', 'warn'],
  ]) {
    const { byName } = await diagnose({}, { answers: { [composeVersionArguments]: { status: 0, stdout: output, stderr: '' } } })
    assert.equal(byName['Docker Compose'].status, status, output)
    if (status === 'fail') {
      assert.match(byName['Docker Compose'].detail, /older than the required v2\.17/)
      assert.match(byName['Docker Compose'].fix, /v2\.17 or newer/)
    }
  }
  const { byName } = await diagnose({}, { answers: { [composeVersionArguments]: { status: 1, stdout: '', stderr: "'compose' is not a docker command" } } })
  assert.equal(byName['Docker Compose'].status, 'fail')
  assert.match(byName['Docker Compose'].fix, /v2\.17 or newer/)
})

test('a busy port is a warning unless the running my-api container owns it', async () => {
  const busy = async () => ({ state: 'in-use', code: 'EADDRINUSE' })
  const foreign = await diagnose({}, { probePort: busy })
  assert.equal(foreign.byName['Listen port'].status, 'warn')
  assert.match(foreign.byName['Listen port'].detail, /127\.0\.0\.1:3000 is already in use/)
  assert.match(foreign.byName['Listen port'].fix, /MYAPI_PORT/)

  const own = await diagnose({}, {
    probePort: busy,
    answers: { [runningContainerArguments]: { status: 0, stdout: '0123456789ab\n', stderr: '' } },
  })
  assert.equal(own.byName['Listen port'].status, 'ok')
  assert.match(own.byName['Listen port'].detail, /running my-api container/)

  const probed = []
  await diagnose({ bindAddress: 'localhost', port: 4242 }, { probePort: async (host, port) => { probed.push([host, port]); return { state: 'free' } } })
  assert.deepEqual(probed, [['127.0.0.1', 4242]], 'localhost is probed through its loopback address')

  const unassigned = await diagnose({ bindAddress: '192.168.1.20', allowLan: 'true' }, { probePort: async () => ({ state: 'unavailable', code: 'EADDRNOTAVAIL' }) })
  assert.equal(unassigned.byName['Listen port'].status, 'warn')
  assert.match(unassigned.byName['Listen port'].detail, /not an address of this machine/)
  const privileged = await diagnose({ port: 80 }, { probePort: async () => ({ state: 'unavailable', code: 'EACCES' }) })
  assert.match(privileged.byName['Listen port'].fix, /above 1023/)

  const invalid = await diagnose({ port: 70000 })
  assert.equal('Listen port' in invalid.byName, false, 'an invalid port is reported by the configuration check, not probed')
})

test('data and log directories report type, writability and free space', async () => {
  const states = {
    '/srv/myapi/deploy/data': { kind: 'file', nearest: '/srv/myapi/deploy/data', writable: true },
    '/srv/myapi/deploy/logs': { kind: 'directory', nearest: '/srv/myapi/deploy/logs', writable: false },
  }
  const { byName } = await diagnose({}, { inspectDirectory: (directory) => states[directory] })
  assert.equal(byName['Data directory'].status, 'fail')
  assert.match(byName['Data directory'].fix, /MYAPI_DATA_DIR/)
  assert.equal('Data disk space' in byName, false, 'a regular file has no meaningful volume check')
  assert.equal(byName['Log directory'].status, 'warn')
  assert.match(byName['Log directory'].detail, /not writable by the current user/)

  const missing = await diagnose({}, { inspectDirectory: (directory) => ({ kind: 'missing', nearest: '/srv/myapi/deploy', writable: true }) })
  assert.equal(missing.byName['Data directory'].status, 'ok')
  assert.match(missing.byName['Data directory'].detail, /will be created on first start/)

  const blocked = await diagnose({}, { inspectDirectory: () => ({ kind: 'missing', nearest: '/srv', writable: false }) })
  assert.equal(blocked.byName['Data directory'].status, 'warn')
  assert.match(blocked.byName['Data directory'].detail, /\/srv is not writable/)
})

test('free space below 1 GiB warns at the exact boundary and unreadable volumes degrade to a warning', async () => {
  const below = await diagnose({}, { freeBytes: () => MIN_FREE_BYTES - 1 })
  assert.equal(below.byName['Data disk space'].status, 'warn')
  assert.match(below.byName['Data disk space'].detail, /1\.0 GiB\)/)
  assert.match(below.byName['Data disk space'].fix, /larger volume/)
  const exact = await diagnose({}, { freeBytes: () => MIN_FREE_BYTES })
  assert.equal(exact.byName['Data disk space'].status, 'ok')
  const unreadable = await diagnose({}, { freeBytes: () => { throw new Error('statfs failed') } })
  assert.equal(unreadable.byName['Data disk space'].status, 'warn')
  assert.match(unreadable.byName['Data disk space'].detail, /could not read free space/)
})

test('access scope warns when binding, LAN opt-in and public URL disagree', async () => {
  const local = await diagnose()
  assert.equal(local.byName['Access scope'].status, 'ok')
  assert.match(local.byName['Access scope'].detail, /this machine only/)

  const optInWithoutShare = await diagnose({ allowLan: 'true' })
  assert.equal(optInWithoutShare.byName['Access scope'].status, 'warn')
  assert.match(optInWithoutShare.byName['Access scope'].detail, /no effect while MYAPI_BIND_ADDRESS is a loopback address/)

  const wildcard = await diagnose({ bindAddress: '0.0.0.0', allowLan: 'true' })
  assert.equal(wildcard.byName['Access scope'].status, 'warn')
  assert.match(wildcard.byName['Access scope'].detail, /all interfaces/)
  assert.match(wildcard.byName['Access scope'].fix, /reverse proxy/)

  const privateBind = await diagnose({ bindAddress: '192.168.1.20', allowLan: 'true' })
  assert.match(privateBind.byName['Access scope'].detail, /reachable beyond this machine/)

  const liteWithPublicUrl = await diagnose({ rawEdition: 'lite', publicUrl: 'https://api.example.test', publicUrlIsLocal: false })
  assert.equal(liteWithPublicUrl.byName['Access scope'].status, 'warn')
  assert.match(liteWithPublicUrl.byName['Access scope'].detail, /public origin/)
  const liteLocal = await diagnose({ rawEdition: 'lite', publicUrl: 'http://localhost:3000', publicUrlIsLocal: true })
  assert.equal(liteLocal.byName['Access scope'].status, 'ok')
  const fullBehindProxy = await diagnose({ rawEdition: 'full', publicUrl: 'https://api.example.test', publicUrlIsLocal: false })
  assert.equal(fullBehindProxy.byName['Access scope'].status, 'ok', 'Full normally sits behind a reverse proxy on loopback')
})

test('legacy NEW_API_* variables get a migration hint', async () => {
  const none = await diagnose()
  assert.equal(none.byName['Legacy NEW_API_* settings'].status, 'ok')
  const legacy = await diagnose({ legacyKeys: ['NEW_API_PORT', 'NEW_API_IMAGE'] })
  assert.equal(legacy.byName['Legacy NEW_API_* settings'].status, 'warn')
  assert.match(legacy.byName['Legacy NEW_API_* settings'].detail, /NEW_API_PORT, NEW_API_IMAGE/)
  assert.equal(legacy.byName['Legacy NEW_API_* settings'].fix, 'run "myapi migrate --project-dir /srv/myapi" to write the MYAPI_* names')
})

test('the production command runner enforces its timeout and reports a missing executable', () => {
  const environment = createDiagnosticsEnvironment({ timeoutMs: 200 })
  const hung = environment.exec(process.execPath, ['-e', 'setTimeout(() => {}, 30000)'])
  assert.equal(hung.errorCode, 'ETIMEDOUT')
  const missing = environment.exec('myapi-definitely-not-installed', [])
  assert.equal(missing.errorCode, 'ENOENT')
  const ok = environment.exec(process.execPath, ['-e', 'process.stdout.write("hello")'])
  assert.equal(ok.status, 0)
  assert.equal(ok.stdout, 'hello')
})
