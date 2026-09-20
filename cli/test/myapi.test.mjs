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
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
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
const packageVersion = JSON.parse(
  readFileSync(path.join(repositoryRoot, 'package.json'), 'utf8')
).version
const escapedPackageVersion = packageVersion.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
const defaultFullImagePattern = new RegExp(
  `^MYAPI_IMAGE=ghcr\\.io/forcemind/myapi:v${escapedPackageVersion}$`,
  'm'
)
const defaultLANImagePattern = new RegExp(
  `^MYAPI_IMAGE=ghcr\\.io/forcemind/myapi-lan:v${escapedPackageVersion}$`,
  'm'
)
const temporaryRoots = []

function runCli(...args) {
  return execFileSync(process.execPath, [cli, ...args], {
    cwd: repositoryRoot,
    encoding: 'utf8',
  })
}

function runCliWithEnv(env, ...args) {
  return execFileSync(process.execPath, [cli, ...args], {
    cwd: repositoryRoot,
    encoding: 'utf8',
    env: { ...process.env, ...env },
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
  assert.equal(version.version, packageVersion)
  assert.equal(version.distribution, 'MyAPI')
  assert.equal(version.machineSlug, 'my-api')
  assert.doesNotMatch(JSON.stringify(version), /New API|QuantumNous/)
})

test('generated deployment defaults follow the package release version', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  runCli('init', project)
  runCli('configure', '--project-dir', project, '--public-url', 'https://myapi.example.test')
  const env = readFileSync(path.join(project, 'deploy/.env'), 'utf8')
  const escapedVersion = packageVersion.replace(/[.*+?^${}()|[\\]\\\\]/g, '\\$&')
  assert.match(
    env,
    new RegExp(`^MYAPI_IMAGE=ghcr\\.io/forcemind/myapi:v${escapedVersion}$`, 'm'),
  )
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
  assert.match(env, defaultFullImagePattern)
  assert.match(env, /^MYAPI_BUILD_LOCAL=false$/m)
  assert.match(env, /^MYAPI_PUBLIC_URL=https:\/\/myapi\.example\.test$/m)
  assert.match(env, /^MYAPI_BRAND_NAME=MyAPI$/m)
  assert.match(env, /^MYAPI_BRAND_LOGO=\/myapi-logo-v1\.png$/m)
  assert.match(env, /^MYAPI_CPU_LIMIT=2\.0$/m)
  assert.match(env, /^MYAPI_MEMORY_LIMIT=2g$/m)
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

test('upgrade validates the release version before touching deployment state', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  runCli('init', project)
  runCli('configure', '--project-dir', project, '--public-url', 'https://myapi.example.test')

  assert.throws(
    () => runCli('upgrade', '--project-dir', project, '--version', 'nightly'),
    /semantic version/
  )
  const env = readFileSync(path.join(project, 'deploy/.env'), 'utf8')
  assert.match(env, defaultFullImagePattern)
  assert.equal(existsSync(path.join(project, 'backups')), false)
})

test('build forwards bounded CPU and memory limits to Docker', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  const fakeBin = path.join(root, 'bin')
  const dockerLog = path.join(root, 'docker.log')
  mkdirSync(fakeBin)
  const fakeDocker = path.join(fakeBin, 'docker')
  writeFileSync(
    fakeDocker,
    '#!/bin/sh\n' +
      'set -eu\n' +
      'printf "%s\\n" "$*" >> "$MYAPI_FAKE_DOCKER_LOG"\n',
  )
  chmodSync(fakeDocker, 0o700)
  runCli('init', project)
  runCli('configure', '--project-dir', project, '--public-url', 'https://myapi.example.test')
  const envPath = path.join(project, 'deploy/.env')
  let env = readFileSync(envPath, 'utf8')
  env = env.replace(/^MYAPI_IMAGE=.*$/m, 'MYAPI_IMAGE=local/myapi:test')
    .replace(/^MYAPI_CPU_LIMIT=.*$/m, 'MYAPI_CPU_LIMIT=1.5')
    .replace(/^MYAPI_MEMORY_LIMIT=.*$/m, 'MYAPI_MEMORY_LIMIT=768m')
  writeFileSync(envPath, env, { mode: 0o600 })

  runCliWithEnv(
    { PATH: `${fakeBin}:${process.env.PATH || ''}`, MYAPI_FAKE_DOCKER_LOG: dockerLog },
    'build', '--project-dir', project,
  )

  const dockerCall = readFileSync(dockerLog, 'utf8')
  assert.match(dockerCall, /build --cpu-period 100000 --cpu-quota 150000 --memory 768m --memory-swap 768m/)
  assert.match(dockerCall, /--build-arg MYAPI_BUILD_PARALLELISM=2/)
})

test('upgrade dry-run validates a copy without writing files or invoking Docker', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  runCli('init', project)
  runCli('configure', '--project-dir', project, '--public-url', 'https://myapi.example.test')

  const envPath = path.join(project, 'deploy/.env')
  const before = readFileSync(envPath, 'utf8')
  const result = JSON.parse(
    runCli(
      'upgrade',
      '--project-dir',
      project,
      '--version',
      'v0.2.0',
      '--dry-run',
      '--json'
    )
  )

  assert.equal(result.mode, 'dry-run')
  assert.equal(result.edition, 'full')
  assert.equal(result.targetImage, 'ghcr.io/forcemind/myapi:v0.2.0')
  assert.equal(result.imageSource, 'ghcr-pull')
  assert.deepEqual(result.writes, [])
  assert.deepEqual(result.dockerOperations, [])
  assert.equal(readFileSync(envPath, 'utf8'), before)
  assert.equal(existsSync(path.join(project, 'backups')), false)
})

test('upgrade dry-run rejects JSON output without dry-run mode', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  runCli('init', project)
  runCli('configure', '--project-dir', project, '--public-url', 'https://myapi.example.test')

  assert.throws(
    () => runCli('upgrade', '--project-dir', project, '--version', 'v0.2.0', '--json'),
    /only supported with --dry-run/
  )
})

test('upgrade dry-run fails closed on invalid runtime configuration', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  runCli('init', project)
  runCli('configure', '--project-dir', project, '--public-url', 'https://myapi.example.test')

  const envPath = path.join(project, 'deploy/.env')
  const before = readFileSync(envPath, 'utf8')
  writeFileSync(envPath, before.replace(/^MYAPI_CPU_LIMIT=.*$/m, 'MYAPI_CPU_LIMIT=0'), {
    mode: 0o600,
  })

  assert.throws(
    () => runCli('upgrade', '--project-dir', project, '--version', 'v0.2.0', '--dry-run'),
    /upgrade preflight failed.*MYAPI_CPU_LIMIT/
  )
  assert.equal(existsSync(path.join(project, 'backups')), false)
  assert.equal(readFileSync(envPath, 'utf8'), before.replace(/^MYAPI_CPU_LIMIT=.*$/m, 'MYAPI_CPU_LIMIT=0'))
})

test('upgrade restores the environment and reruns the old deployment after a pull failure', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  const fakeBin = path.join(root, 'bin')
  const dockerLog = path.join(root, 'docker.log')
  const dockerState = path.join(root, 'docker.failed-once')
  mkdirSync(fakeBin)
  const fakeDocker = path.join(fakeBin, 'docker')
  writeFileSync(
    fakeDocker,
    '#!/bin/sh\n' +
      'set -eu\n' +
      'printf "%s\\n" "$*" >> "$MYAPI_FAKE_DOCKER_LOG"\n' +
      'case " $* " in\n' +
      '  *" pull my-api "*)\n' +
      '    if [ ! -e "$MYAPI_FAKE_DOCKER_STATE" ]; then\n' +
      '      : > "$MYAPI_FAKE_DOCKER_STATE"\n' +
      '      exit 42\n' +
      '    fi\n' +
      '    ;;\n' +
      'esac\n',
    { mode: 0o700 },
  )
  chmodSync(fakeDocker, 0o700)

  runCli('init', project)
  runCli('configure', '--project-dir', project, '--public-url', 'https://myapi.example.test')
  const envPath = path.join(project, 'deploy/.env')
  const before = readFileSync(envPath, 'utf8')

  assert.throws(
    () =>
      runCliWithEnv(
        {
          PATH: `${fakeBin}:${process.env.PATH || ''}`,
          MYAPI_FAKE_DOCKER_LOG: dockerLog,
          MYAPI_FAKE_DOCKER_STATE: dockerState,
        },
        'upgrade',
        '--project-dir',
        project,
        '--version',
        'v0.2.0',
      ),
    (error) =>
      Boolean(
        error &&
          typeof error === 'object' &&
          'stderr' in error &&
          /previous image restored and health-checked/.test(String(error.stderr)),
      ),
  )
  assert.equal(readFileSync(envPath, 'utf8'), before)
  const backups = readdirSync(path.join(project, 'backups'))
  assert.equal(backups.length, 1)
  assert.equal(statSync(path.join(project, 'backups', backups[0])).mode & 0o777, 0o600)
  const dockerCalls = readFileSync(dockerLog, 'utf8').trim().split(/\r?\n/)
  assert.equal(dockerCalls.length, 2)
  assert.match(dockerCalls[0], /pull my-api/)
  assert.match(dockerCalls[1], /up -d --force-recreate --wait --wait-timeout 120/)
})

test('upgrade can pin the pulled image to its repository digest', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  const fakeBin = path.join(root, 'bin')
  const dockerLog = path.join(root, 'docker.log')
  mkdirSync(fakeBin)
  const fakeDocker = path.join(fakeBin, 'docker')
  const digest = 'a'.repeat(64)
  writeFileSync(
    fakeDocker,
    '#!/bin/sh\n' +
      'set -eu\n' +
      'printf "%s\\n" "$*" >> "$MYAPI_FAKE_DOCKER_LOG"\n' +
      'case " $* " in\n' +
      '  *" image inspect "*) printf \'["ghcr.io/forcemind/myapi@sha256:' + digest + '"]\\n\' ;;\n' +
      'esac\n',
    { mode: 0o700 },
  )
  chmodSync(fakeDocker, 0o700)

  runCli('init', project)
  runCli('configure', '--project-dir', project, '--public-url', 'https://myapi.example.test')
  runCliWithEnv(
    {
      PATH: `${fakeBin}:${process.env.PATH || ''}`,
      MYAPI_FAKE_DOCKER_LOG: dockerLog,
    },
    'upgrade',
    '--project-dir',
    project,
    '--version',
    'v0.2.0',
    '--pin-digest',
  )

  const env = readFileSync(path.join(project, 'deploy/.env'), 'utf8')
  assert.match(env, new RegExp(`^MYAPI_IMAGE=ghcr\\.io/forcemind/myapi@sha256:${digest}$`, 'm'))
  const dockerCalls = readFileSync(dockerLog, 'utf8').trim().split(/\r?\n/)
  assert.equal(dockerCalls.length, 3)
  assert.match(dockerCalls[0], /pull my-api/)
  assert.match(dockerCalls[1], /image inspect --format/)
  assert.match(dockerCalls[2], /up -d --force-recreate --wait --wait-timeout 120/)
})

test('upgrade rejects a missing or malformed pulled image digest and rolls back', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  const fakeBin = path.join(root, 'bin')
  const dockerLog = path.join(root, 'docker.log')
  mkdirSync(fakeBin)
  const fakeDocker = path.join(fakeBin, 'docker')
  writeFileSync(
    fakeDocker,
    '#!/bin/sh\n' +
      'set -eu\n' +
      'printf "%s\\n" "$*" >> "$MYAPI_FAKE_DOCKER_LOG"\n' +
      'case " $* " in\n' +
      '  *" image inspect "*) printf \'["ghcr.io/forcemind/myapi@sha256:not-a-digest"]\\n\' ;;\n' +
      'esac\n',
    { mode: 0o700 },
  )
  chmodSync(fakeDocker, 0o700)

  runCli('init', project)
  runCli('configure', '--project-dir', project, '--public-url', 'https://myapi.example.test')
  const envPath = path.join(project, 'deploy/.env')
  const before = readFileSync(envPath, 'utf8')
  assert.throws(
    () => runCliWithEnv(
      {
        PATH: `${fakeBin}:${process.env.PATH || ''}`,
        MYAPI_FAKE_DOCKER_LOG: dockerLog,
      },
      'upgrade', '--project-dir', project, '--version', 'v0.2.0', '--pin-digest',
    ),
    (error) => Boolean(error && typeof error === 'object' && 'stderr' in error && /valid repository digest/.test(String(error.stderr))),
  )
  assert.equal(readFileSync(envPath, 'utf8'), before)
  const dockerCalls = readFileSync(dockerLog, 'utf8').trim().split(/\r?\n/)
  assert.equal(dockerCalls.length, 3)
  assert.match(dockerCalls[0], /pull my-api/)
  assert.match(dockerCalls[1], /image inspect --format/)
  assert.match(dockerCalls[2], /up -d --force-recreate --wait --wait-timeout 120/)
})

test('up rejects unsafe Docker resource limits before invoking Compose', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  runCli('init', project)
  runCli('configure', '--project-dir', project, '--public-url', 'https://myapi.example.test')

  const envPath = path.join(project, 'deploy/.env')
  let env = readFileSync(envPath, 'utf8')
  env = env.replace(/^MYAPI_CPU_LIMIT=.*$/m, 'MYAPI_CPU_LIMIT=0')
  env = env.replace(/^MYAPI_MEMORY_LIMIT=.*$/m, 'MYAPI_MEMORY_LIMIT=not-a-size')
  writeFileSync(envPath, env, { mode: 0o600 })

  assert.throws(
    () => runCli('up', '--project-dir', project),
    /deployment preflight failed.*MYAPI_CPU_LIMIT.*MYAPI_MEMORY_LIMIT/s,
  )
})

test('up rejects a non-loopback bind without explicit LAN opt-in', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  const fakeBin = path.join(root, 'bin')
  const dockerLog = path.join(root, 'docker.log')
  mkdirSync(fakeBin)
  const fakeDocker = path.join(fakeBin, 'docker')
  writeFileSync(
    fakeDocker,
    '#!/bin/sh\n' +
      'set -eu\n' +
      'printf "%s\\n" "$*" >> "$MYAPI_FAKE_DOCKER_LOG"\n',
    { mode: 0o700 },
  )

  runCli('init', project)
  runCli('configure', '--project-dir', project, '--public-url', 'https://myapi.example.test')
  const envPath = path.join(project, 'deploy/.env')
  let env = readFileSync(envPath, 'utf8')
  env = env.replace(/^MYAPI_BIND_ADDRESS=.*$/m, 'MYAPI_BIND_ADDRESS=0.0.0.0')
    .replace(/^MYAPI_ALLOW_LAN=.*$/m, 'MYAPI_ALLOW_LAN=false')
  writeFileSync(envPath, env, { mode: 0o600 })

  assert.throws(
    () => runCliWithEnv(
      { PATH: `${fakeBin}:${process.env.PATH || ''}`, MYAPI_FAKE_DOCKER_LOG: dockerLog },
      'up', '--project-dir', project,
    ),
    /deployment preflight failed.*LAN binding is disabled by default/,
  )
  assert.equal(existsSync(dockerLog), false)
})

test('upgrade dry-run rejects a non-loopback bind without explicit LAN opt-in', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  runCli('init', project)
  runCli('configure', '--project-dir', project, '--public-url', 'https://myapi.example.test')
  const envPath = path.join(project, 'deploy/.env')
  let env = readFileSync(envPath, 'utf8')
  env = env.replace(/^MYAPI_BIND_ADDRESS=.*$/m, 'MYAPI_BIND_ADDRESS=192.168.1.20')
    .replace(/^MYAPI_ALLOW_LAN=.*$/m, 'MYAPI_ALLOW_LAN=false')
  writeFileSync(envPath, env, { mode: 0o600 })

  assert.throws(
    () => runCli('upgrade', '--project-dir', project, '--version', 'v0.2.0', '--dry-run'),
    /upgrade preflight failed.*LAN binding is disabled by default/,
  )
  assert.equal(existsSync(path.join(project, 'backups')), false)
})

test('signature verification fails closed before changing deployment state', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'source')
  runCli('init', project)
  runCli('configure', '--project-dir', project, '--public-url', 'https://myapi.example.test')

  assert.throws(
    () => runCli('upgrade', '--project-dir', project, '--version', 'v0.2.0', '--verify-signature'),
    /requires MYAPI_COSIGN_CERTIFICATE_IDENTITY/
  )
  const env = readFileSync(path.join(project, 'deploy/.env'), 'utf8')
  assert.match(env, defaultFullImagePattern)
  assert.equal(existsSync(path.join(project, 'backups')), false)
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

test('lan init creates a loopback-only LAN deployment without exposing credentials', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'lan')

  const output = runCli('lan', 'init', project)
  const env = readFileSync(path.join(project, 'deploy/.env'), 'utf8')

  assert.match(env, defaultLANImagePattern)
  assert.match(env, /^MYAPI_EDITION=lan$/m)
  assert.match(env, /^MYAPI_BIND_ADDRESS=127\.0\.0\.1$/m)
  assert.match(env, /^MYAPI_ALLOW_LAN=false$/m)
  assert.match(env, /^MYAPI_PUBLIC_URL=http:\/\/localhost:3000$/m)
  assert.match(output, /Loopback-only mode/)
  assert.match(output, /never reads local credential files/)
  assert.doesNotMatch(output, /SESSION_SECRET|sk-[A-Za-z0-9]|oauth/i)
})

test('lan init requires explicit opt-in for a private network address', () => {
  const root = temporaryRoot()
  const refused = path.join(root, 'refused')
  assert.throws(
    () => runCli('lan', 'init', refused, '--bind-address', '192.168.1.20'),
    /pass --allow-lan/
  )
  assert.equal(existsSync(refused), false)

  const project = path.join(root, 'shared')
  const output = runCli(
    'lan',
    'init',
    project,
    '--bind-address',
    '192.168.1.20',
    '--port',
    '4317',
    '--allow-lan'
  )
  const env = readFileSync(path.join(project, 'deploy/.env'), 'utf8')
  assert.match(env, /^MYAPI_BIND_ADDRESS=192\.168\.1\.20$/m)
  assert.match(env, /^MYAPI_ALLOW_LAN=true$/m)
  assert.match(env, /^MYAPI_PORT=4317$/m)
  assert.match(env, /^MYAPI_PUBLIC_URL=http:\/\/192\.168\.1\.20:4317$/m)
  assert.match(output, /Private-network mode/)

  // Once the project has recorded the explicit LAN opt-in, the documented
  // restart command should not require repeating --allow-lan. Use a fake
  // Docker executable so this remains a no-deployment unit test.
  const fakeBin = path.join(root, 'bin')
  const dockerLog = path.join(root, 'docker.log')
  mkdirSync(fakeBin)
  const fakeDocker = path.join(fakeBin, 'docker')
  writeFileSync(
    fakeDocker,
    '#!/bin/sh\n' +
      'set -eu\n' +
      'printf "%s\\n" "$*" >> "$MYAPI_FAKE_DOCKER_LOG"\n',
    { mode: 0o700 },
  )
  chmodSync(fakeDocker, 0o700)
  const startOutput = runCliWithEnv(
    { PATH: `${fakeBin}:${process.env.PATH || ''}`, MYAPI_FAKE_DOCKER_LOG: dockerLog },
    'lan',
    'start',
    '--project-dir',
    project,
  )
  assert.match(startOutput, /Private-network mode/)
  assert.match(readFileSync(dockerLog, 'utf8'), /compose .* up/)

  // An operator may have disabled LAN sharing in .env and explicitly opt in
  // again for one start. Persist that choice before Compose reads the file.
  const envPath = path.join(project, 'deploy/.env')
  writeFileSync(
    envPath,
    readFileSync(envPath, 'utf8').replace(/^MYAPI_ALLOW_LAN=.*$/m, 'MYAPI_ALLOW_LAN=false'),
    { mode: 0o600 },
  )
  runCliWithEnv(
    { PATH: `${fakeBin}:${process.env.PATH || ''}`, MYAPI_FAKE_DOCKER_LOG: dockerLog },
    'lan',
    'start',
    '--project-dir',
    project,
    '--allow-lan',
  )
  assert.match(readFileSync(envPath, 'utf8'), /^MYAPI_ALLOW_LAN=true$/m)
})

test('lan wildcard binding advertises discovered RFC1918 endpoints without a placeholder', () => {
  const root = temporaryRoot()
  const project = path.join(root, 'wildcard')

  const output = runCli(
    'lan',
    'init',
    project,
    '--bind-address',
    '0.0.0.0',
    '--port',
    '4318',
    '--allow-lan',
  )
  const env = readFileSync(path.join(project, 'deploy/.env'), 'utf8')

  // A wildcard is a socket bind address, not a usable endpoint. The CLI must
  // either print one or more RFC1918 URLs or explicitly report that discovery
  // found none, and must never suggest the old placeholder as a URL.
  assert.doesNotMatch(output, /<private-LAN-IP>/)
  assert.match(output, /MyAPI LAN endpoint(?:s)?: .*(?:RFC1918|http:\/\/)/)
  assert.match(env, /^MYAPI_BIND_ADDRESS=0\.0\.0\.0$/m)
  assert.match(env, /^MYAPI_PUBLIC_URL=http:\/\/localhost:4318$/m)
})

test('lan init rejects public and invalid listener addresses', () => {
  const root = temporaryRoot()
  assert.throws(
    () => runCli('lan', 'init', path.join(root, 'public'), '--bind-address', '8.8.8.8', '--allow-lan'),
    /private IPv4 address/
  )
  assert.throws(
    () => runCli('lan', 'init', path.join(root, 'invalid'), '--port', '70000'),
    /between 1 and 65535/
  )
  for (const address of ['10.bad', '192.168.999.1', '172.16.bad', '172.15.1.1', '172.32.1.1']) {
    assert.throws(
      () => runCli('lan', 'init', path.join(root, `invalid-${address.replaceAll('.', '-')}`), '--bind-address', address, '--allow-lan'),
      /private IPv4 address/
    )
  }
})
