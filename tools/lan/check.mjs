#!/usr/bin/env node

/*
 * Cross-platform LAN Lite acceptance harness.
 *
 * This command is intentionally dependency-free and non-destructive. It
 * exercises the CLI's LAN initializer in a temporary directory, validates the
 * generated deployment contract, and optionally checks Docker Compose's
 * configuration parser. It never starts, stops, pulls, or rebuilds a service.
 */
import { execFileSync, spawnSync } from 'node:child_process'
import {
  existsSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
  statSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const cli = path.join(repositoryRoot, 'cli', 'myapi.mjs')
const keepTemporary = process.argv.includes('--keep-temp')
const jsonOutput = process.argv.includes('--json')
const skipDocker = process.argv.includes('--skip-docker')

function optionValue(name) {
  const index = process.argv.indexOf(name)
  if (index < 0) return undefined
  const value = process.argv[index + 1]
  if (!value || value.startsWith('-')) throw new Error(`${name} requires a value`)
  return value
}

function record(checks, name, ok, detail = '') {
  checks.push({ name, ok: Boolean(ok), detail })
}

function parseEnv(contents) {
  const values = {}
  for (const line of contents.split(/\r?\n/)) {
    const separator = line.indexOf('=')
    if (separator <= 0 || line.trimStart().startsWith('#')) continue
    values[line.slice(0, separator).trim()] = line.slice(separator + 1).trim()
  }
  return values
}

function commandAvailable(command, args = ['--version']) {
  const result = spawnSync(command, args, { encoding: 'utf8', stdio: 'ignore', shell: false })
  return !result.error && result.status === 0
}

function runCli(...args) {
  return execFileSync(process.execPath, [cli, ...args], {
    cwd: repositoryRoot,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  })
}

function runCliFailure(...args) {
  const result = spawnSync(process.execPath, [cli, ...args], {
    cwd: repositoryRoot,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
    shell: false,
  })
  return {
    status: result.status,
    output: `${result.stdout || ''}${result.stderr || ''}`,
  }
}

function checkStaticContracts(checks) {
  const requiredFiles = [
    'cli/myapi.mjs',
    'deploy/docker-compose.yml',
    'deploy/.env.example',
    'electron/main.js',
    'electron/runtime-config.js',
    'docs/LAN_LITE.md',
    '.github/workflows/electron-build.yml',
    '.github/workflows/docker-build.yml',
  ]
  for (const relative of requiredFiles) {
    record(checks, `file: ${relative}`, existsSync(path.join(repositoryRoot, relative)))
  }

  const lanDoc = readFileSync(path.join(repositoryRoot, 'docs/LAN_LITE.md'), 'utf8')
  record(checks, 'docs explain loopback default', /loopback-only|回环/.test(lanDoc))
  record(checks, 'docs forbid credential-file import', /credential-file importer|local credential store|never reads by the CLI/.test(lanDoc))
  record(checks, 'docs require explicit LAN opt-in', /--allow-lan/.test(lanDoc))
  record(checks, 'docs describe safe stop/update', /lan stop/.test(lanDoc) && /pinned/.test(lanDoc))

  const runtime = readFileSync(path.join(repositoryRoot, 'electron', 'runtime-config.js'), 'utf8')
  record(checks, 'Electron validates private IPv4', /private IPv4/.test(runtime))
  record(checks, 'Electron relaunches instead of live rebind', /relaunch|重新启动/.test(runtime))

  const workflow = readFileSync(path.join(repositoryRoot, '.github/workflows/electron-build.yml'), 'utf8')
  record(checks, 'CI builds macOS and Windows artifacts', /macos-latest/.test(workflow) && /windows-latest/.test(workflow))
  const images = readFileSync(path.join(repositoryRoot, '.github/workflows/docker-build.yml'), 'utf8')
  record(checks, 'CI builds the LAN GHCR image', /ghcr\.io\/forcemind\/myapi-lan/.test(images))
}

function validateGeneratedProject(
  checks,
  project,
  output,
  { expectLoopback = true, expectFresh = true } = {},
) {
  const envPath = path.join(project, 'deploy', '.env')
  record(checks, expectFresh ? 'LAN init creates deploy/.env' : 'LAN project contains deploy/.env', existsSync(envPath))
  if (!existsSync(envPath)) return

  const envContents = readFileSync(envPath, 'utf8')
  const values = parseEnv(envContents)
  record(checks, 'LAN edition is selected', values.MYAPI_EDITION === 'lan')
  record(checks, 'LAN image is version-pinned GHCR', /^ghcr\.io\/forcemind\/myapi-lan:v\d+\.\d+\.\d+/.test(values.MYAPI_IMAGE || ''))
  record(
    checks,
    expectLoopback ? 'default listener is loopback' : 'listener stays loopback or private IPv4',
    expectLoopback
      ? values.MYAPI_BIND_ADDRESS === '127.0.0.1'
      : /^(?:127\.0\.0\.1|localhost|0\.0\.0\.0|10\.|192\.168\.|172\.(?:1[6-9]|2\d|3[01])\.)/.test(values.MYAPI_BIND_ADDRESS || ''),
  )
  record(
    checks,
    expectLoopback ? 'default public URL is local HTTP' : 'public URL stays local/private HTTP(S)',
    expectLoopback
      ? /^http:\/\/localhost:\d+$/.test(values.MYAPI_PUBLIC_URL || '')
      : /^(?:https?:\/\/)(?:localhost|127\.0\.0\.1|10\.|192\.168\.|172\.(?:1[6-9]|2\d|3[01])\.)[^\s]*$/.test(values.MYAPI_PUBLIC_URL || ''),
  )
  record(checks, 'resource guardrails are bounded', values.MYAPI_CPU_LIMIT === '2.0' && values.MYAPI_MEMORY_LIMIT === '2g')
  record(checks, 'session secret is generated without exposing it', /^[a-f0-9]{64}$/i.test(values.SESSION_SECRET || '') && !output.includes(values.SESSION_SECRET || '__missing__'))
  record(checks, 'generated environment is private', process.platform === 'win32' || (statSync(envPath).mode & 0o777) === 0o600)
  if (expectFresh) {
    const runtimeData = path.join(project, 'deploy', 'data')
    record(checks, 'runtime data is not copied into initializer', !existsSync(path.join(project, '.codex')) && (!existsSync(runtimeData) || readdirSync(runtimeData).length === 0))
  } else {
    record(checks, 'LAN project contains no local credential directory', !existsSync(path.join(project, '.codex')))
  }

  if (!skipDocker && commandAvailable('docker', ['compose', 'version', '--short'])) {
    const result = spawnSync('docker', ['compose', '--env-file', envPath, '-f', path.join(project, 'deploy', 'docker-compose.yml'), 'config', '--quiet'], {
      cwd: project,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
      shell: false,
    })
    // Compose config is parser-only: no daemon, pull, volume, or container operation.
    record(checks, 'Docker Compose LAN template parses', result.status === 0, result.status === 0 ? 'parser-only' : 'unavailable or invalid')
  } else {
    record(checks, 'Docker Compose LAN template parses', true, 'skipped (Docker Compose unavailable or --skip-docker)')
  }
}

function checkPrivateLANOptIn(checks) {
  const project = mkdtempSync(path.join(tmpdir(), 'myapi-lan-opt-in-check-'))
  try {
    const output = runCli(
      'lan',
      'init',
      project,
      '--bind-address',
      '192.168.1.20',
      '--port',
      '4317',
      '--allow-lan',
    )
    validateGeneratedProject(checks, project, output, {
      expectLoopback: false,
      expectFresh: true,
    })

    // A non-loopback bind without the explicit opt-in must fail before the
    // destination is created or any environment is written.
    const rejectedProject = path.join(project, 'missing-opt-in')
    const rejectedResult = runCliFailure(
      'lan',
      'init',
      rejectedProject,
      '--bind-address',
      '192.168.1.20',
    )
    record(
      checks,
      'private LAN binding requires explicit opt-in',
      rejectedResult.status !== 0,
      'CLI exits before writing the project',
    )
    record(
      checks,
      'rejected LAN bind does not create a project',
      !existsSync(rejectedProject),
    )
  } finally {
    if (!keepTemporary) rmSync(project, { recursive: true, force: true })
  }
}

function main() {
  const checks = []
  checkStaticContracts(checks)
  const requestedProject = optionValue('--project-dir')
  let temporaryProject
  const project = requestedProject
    ? path.resolve(requestedProject)
    : (temporaryProject = mkdtempSync(path.join(tmpdir(), 'myapi-lan-check-')))

  try {
    let output = ''
    if (requestedProject) {
      const envPath = path.join(project, 'deploy', '.env')
      record(checks, 'requested LAN project exists', existsSync(envPath), envPath)
      if (existsSync(envPath)) {
        const values = parseEnv(readFileSync(envPath, 'utf8'))
        record(checks, 'requested project selects LAN edition', values.MYAPI_EDITION === 'lan')
      }
    } else {
      output = runCli('lan', 'init', project)
    }
    validateGeneratedProject(checks, project, output, {
      expectLoopback: !requestedProject,
      expectFresh: !requestedProject,
    })
    if (!requestedProject) checkPrivateLANOptIn(checks)
  } finally {
    if (temporaryProject && !keepTemporary) rmSync(temporaryProject, { recursive: true, force: true })
  }

  const failed = checks.filter((check) => !check.ok)
  if (jsonOutput) {
    console.log(JSON.stringify({ command: 'lan:check', passed: failed.length === 0, checks }, null, 2))
  } else {
    for (const check of checks) console.log(`${check.ok ? 'PASS' : 'FAIL'}  ${check.name}${check.detail ? ` (${check.detail})` : ''}`)
    console.log(`LAN Lite acceptance: ${failed.length === 0 ? 'PASS' : 'FAIL'} (${checks.length - failed.length}/${checks.length})`)
  }
  if (failed.length > 0) process.exitCode = 1
}

try {
  main()
} catch (error) {
  console.error(`LAN Lite acceptance: ${error instanceof Error ? error.message : String(error)}`)
  process.exitCode = 1
}
