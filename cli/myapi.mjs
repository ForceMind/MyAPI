#!/usr/bin/env node

/*
MyAPI distribution tooling for the New API based custom source release.
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { randomBytes } from 'node:crypto'
import {
  chmodSync,
  cpSync,
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  renameSync,
  rmSync,
  statSync,
  writeFileSync,
} from 'node:fs'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const packageMetadata = JSON.parse(
  readFileSync(path.join(packageRoot, 'package.json'), 'utf8')
)

const excludedSourceParts = new Set([
  '.git',
  '.github',
  '.codex',
  '.agents',
  'node_modules',
  'dist',
  'coverage',
])

function fail(message) {
  console.error(`MyAPI: ${message}`)
  process.exitCode = 1
}

function argumentValue(args, name) {
  const index = args.indexOf(name)
  if (index === -1 || index === args.length - 1) return undefined
  return args[index + 1]
}

function validateArguments(
  args,
  { values = [], flags = [], positional = 0 } = {}
) {
  const valueOptions = new Set(values)
  const flagOptions = new Set(flags)
  let positionalCount = 0
  for (let index = 0; index < args.length; index++) {
    const argument = args[index]
    if (!argument.startsWith('-')) {
      positionalCount += 1
      continue
    }
    if (flagOptions.has(argument)) continue
    if (!valueOptions.has(argument)) {
      throw new Error(`unknown option ${argument}`)
    }
    const value = args[index + 1]
    if (!value || value.startsWith('-')) {
      throw new Error(`${argument} requires a value`)
    }
    index += 1
  }
  if (positionalCount > positional) {
    throw new Error('too many positional arguments')
  }
}

function validatePublicOrigin(value) {
  try {
    const url = new URL(value)
    const hostname = url.hostname.toLowerCase()
    return (
      url.protocol === 'https:' &&
      !url.username &&
      !url.password &&
      url.pathname === '/' &&
      !url.search &&
      !url.hash &&
      hostname !== 'localhost' &&
      !hostname.endsWith('.localhost') &&
      hostname !== 'example.com' &&
      !hostname.endsWith('.example.com')
    )
  } catch {
    return false
  }
}

function validateRuntimeConfiguration(values) {
  const errors = []
  const publicUrl = values.NEW_API_PUBLIC_URL || ''
  if (!validatePublicOrigin(publicUrl)) {
    errors.push('NEW_API_PUBLIC_URL must be an exact non-placeholder HTTPS origin')
  }
  const sessionSecret = values.SESSION_SECRET || ''
  if (sessionSecret.length < 48 || sessionSecret.includes('replace-with')) {
    errors.push('SESSION_SECRET must be a generated secret of at least 48 characters')
  }
  const port = Number(values.NEW_API_PORT || '3000')
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    errors.push('NEW_API_PORT must be an integer between 1 and 65535')
  }
  return errors
}

function projectRootFromArgs(args) {
  const explicit = argumentValue(args, '--project-dir')
  return path.resolve(explicit || process.cwd())
}

function parseEnvFile(filePath) {
  if (!existsSync(filePath)) return {}
  const values = {}
  for (const rawLine of readFileSync(filePath, 'utf8').split(/\r?\n/)) {
    const line = rawLine.trim()
    if (!line || line.startsWith('#')) continue
    const separator = line.indexOf('=')
    if (separator <= 0) continue
    const key = line.slice(0, separator).trim()
    if (!/^[A-Z][A-Z0-9_]*$/.test(key)) continue
    values[key] = line.slice(separator + 1).trim()
  }
  return values
}

function setEnvValue(contents, key, value) {
  const safeValue = String(value).replace(/[\r\n]/g, '')
  const line = `${key}=${safeValue}`
  const pattern = new RegExp(`^${key}=.*$`, 'm')
  if (pattern.test(contents)) return contents.replace(pattern, line)
  return `${contents.trimEnd()}\n${line}\n`
}

function deploymentPaths(projectRoot) {
  const deployDir = path.join(projectRoot, 'deploy')
  return {
    projectRoot,
    deployDir,
    composeFile: path.join(deployDir, 'docker-compose.yml'),
    envFile: path.join(deployDir, '.env'),
    envExample: path.join(deployDir, '.env.example'),
  }
}

function assertDeploymentSource(paths) {
  if (!existsSync(paths.composeFile) || !existsSync(paths.envExample)) {
    throw new Error(
      `deployment files were not found under ${paths.deployDir}; run "myapi init" first`
    )
  }
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd,
    encoding: 'utf8',
    stdio: options.capture ? 'pipe' : 'inherit',
    shell: false,
  })
  if (options.capture) return result
  if (result.error) throw result.error
  if (result.status !== 0) {
    throw new Error(`${command} exited with status ${result.status ?? 'unknown'}`)
  }
  return result
}

function composeArguments(paths, args) {
  return [
    'compose',
    '--env-file',
    paths.envFile,
    '-f',
    paths.composeFile,
    ...args,
  ]
}

function shouldCopySource(sourcePath) {
  const relative = path.relative(packageRoot, sourcePath)
  if (!relative) return true
  const parts = relative.split(path.sep)
  if (parts.some((part) => excludedSourceParts.has(part))) return false
  const portableRelative = parts.join('/')
  if (
    /^(?:data|logs|backups|cache)(?:\/|$)/.test(portableRelative) ||
    /^deploy\/(?:data|logs|backups|cache)(?:\/|$)/.test(portableRelative)
  ) {
    return false
  }
  const basename = parts.at(-1) || ''
  if (basename === '.env' || basename.startsWith('.env.')) {
    return basename === '.env.example'
  }
  return !/\.(?:db(?:-.*)?|sqlite3?|log|tgz)$/i.test(basename)
}

function initProject(args) {
  const positional = args.find((arg) => !arg.startsWith('-'))
  const destination = path.resolve(positional || 'myapi-source')
  if (existsSync(destination) && readdirSync(destination).length > 0) {
    throw new Error(`refusing to overwrite non-empty directory ${destination}`)
  }

  const staging = `${destination}.myapi-init-${process.pid}`
  if (existsSync(staging)) rmSync(staging, { recursive: true, force: true })
  mkdirSync(path.dirname(destination), { recursive: true })
  cpSync(packageRoot, staging, {
    recursive: true,
    filter: shouldCopySource,
  })
  // An initialized deployment is an application workspace, not a package
  // publishing checkout. Mark it private so a later `npm publish` cannot leak
  // a generated deploy/.env or any other local runtime material.
  const initializedPackagePath = path.join(staging, 'package.json')
  const initializedPackage = JSON.parse(
    readFileSync(initializedPackagePath, 'utf8')
  )
  initializedPackage.private = true
  delete initializedPackage.publishConfig
  writeFileSync(
    initializedPackagePath,
    `${JSON.stringify(initializedPackage, null, 2)}\n`
  )
  writeFileSync(
    path.join(staging, '.gitignore'),
    readFileSync(path.join(packageRoot, 'cli/templates/gitignore'), 'utf8')
  )
  writeFileSync(
    path.join(staging, '.npmignore'),
    readFileSync(path.join(packageRoot, 'cli/templates/npmignore'), 'utf8')
  )
  if (existsSync(destination)) rmSync(destination, { recursive: true })
  renameSync(staging, destination)
  console.log(`MyAPI source initialized at ${destination}`)
  console.log(`Next: myapi configure --project-dir ${destination}`)
}

function configureProject(args) {
  const paths = deploymentPaths(projectRootFromArgs(args))
  assertDeploymentSource(paths)
  if (existsSync(paths.envFile) && !args.includes('--force')) {
    throw new Error(`${paths.envFile} already exists; use --force to replace it`)
  }

  let contents = readFileSync(paths.envExample, 'utf8')
  const publicUrl = argumentValue(args, '--public-url')
  contents = setEnvValue(contents, 'SESSION_SECRET', randomBytes(32).toString('hex'))
  if (publicUrl) contents = setEnvValue(contents, 'NEW_API_PUBLIC_URL', publicUrl)

  const dataDir = argumentValue(args, '--data-dir')
  const logsDir = argumentValue(args, '--logs-dir')
  if (dataDir) contents = setEnvValue(contents, 'NEW_API_DATA_DIR', path.resolve(dataDir))
  if (logsDir) contents = setEnvValue(contents, 'NEW_API_LOGS_DIR', path.resolve(logsDir))

  writeFileSync(paths.envFile, contents, { mode: 0o600 })
  chmodSync(paths.envFile, 0o600)
  const values = parseEnvFile(paths.envFile)
  mkdirSync(path.resolve(paths.deployDir, values.NEW_API_DATA_DIR || './data'), {
    recursive: true,
  })
  mkdirSync(path.resolve(paths.deployDir, values.NEW_API_LOGS_DIR || './logs'), {
    recursive: true,
  })

  console.log(`Created ${paths.envFile} with mode 0600.`)
  if (!publicUrl) {
    console.log('Set NEW_API_PUBLIC_URL in that file before running myapi up.')
  }
}

function adoptDataPaths(args) {
  const paths = deploymentPaths(projectRootFromArgs(args))
  assertDeploymentSource(paths)
  if (!existsSync(paths.envFile)) {
    throw new Error('run "myapi configure" before adopting data paths')
  }
  const dataDir = argumentValue(args, '--data-dir')
  const logsDir = argumentValue(args, '--logs-dir')
  if (!dataDir || !logsDir) {
    throw new Error('adopt requires both --data-dir and --logs-dir')
  }
  const resolvedDataDir = path.resolve(dataDir)
  const resolvedLogsDir = path.resolve(logsDir)
  for (const target of [resolvedDataDir, resolvedLogsDir]) {
    if (!existsSync(target) || !statSync(target).isDirectory()) {
      throw new Error(`adopt target is not an existing directory: ${target}`)
    }
  }

  let contents = readFileSync(paths.envFile, 'utf8')
  contents = setEnvValue(contents, 'NEW_API_DATA_DIR', resolvedDataDir)
  contents = setEnvValue(contents, 'NEW_API_LOGS_DIR', resolvedLogsDir)
  writeFileSync(paths.envFile, contents, { mode: 0o600 })
  chmodSync(paths.envFile, 0o600)
  console.log('Updated deployment data and log paths without moving any files.')
}

function doctor(args) {
  const paths = deploymentPaths(projectRootFromArgs(args))
  const checks = []
  const record = (name, ok, detail) => checks.push({ name, ok, detail })

  record('Node.js >= 20', Number(process.versions.node.split('.')[0]) >= 20, process.version)
  const docker = run('docker', ['version', '--format', '{{.Server.Version}}'], {
    capture: true,
  })
  record('Docker daemon', docker.status === 0, docker.status === 0 ? docker.stdout.trim() : 'unavailable')
  const compose = run('docker', ['compose', 'version', '--short'], {
    capture: true,
  })
  record('Docker Compose', compose.status === 0, compose.status === 0 ? compose.stdout.trim() : 'unavailable')
  record('Compose template', existsSync(paths.composeFile), paths.composeFile)
  record('Environment file', existsSync(paths.envFile), paths.envFile)

  const values = parseEnvFile(paths.envFile)
  const publicUrl = values.NEW_API_PUBLIC_URL || ''
  record(
    'Public HTTPS URL',
    validatePublicOrigin(publicUrl),
    publicUrl ? 'configured' : 'missing'
  )
  const sessionSecret = values.SESSION_SECRET || ''
  record(
    'Session secret',
    sessionSecret.length >= 48 && !sessionSecret.includes('replace-with'),
    sessionSecret ? 'configured' : 'missing'
  )
  const composeConfig = existsSync(paths.envFile)
    ? run(
        'docker',
        composeArguments(paths, ['config', '--quiet']),
        { cwd: paths.projectRoot, capture: true }
      )
    : { status: 1, stderr: 'environment file is missing' }
  record(
    'Docker Compose configuration',
    composeConfig.status === 0,
    composeConfig.status === 0
      ? 'valid'
      : (composeConfig.stderr || 'invalid').trim()
  )

  for (const check of checks) {
    console.log(`${check.ok ? 'OK' : 'FAIL'}  ${check.name}: ${check.detail}`)
  }
  if (checks.some((check) => !check.ok)) process.exitCode = 1
}

function deploymentCommand(command, args) {
  const paths = deploymentPaths(projectRootFromArgs(args))
  assertDeploymentSource(paths)
  if (!existsSync(paths.envFile)) {
    throw new Error('run "myapi configure" before Docker commands')
  }
  const values = parseEnvFile(paths.envFile)

  if (command === 'build') {
    run(
      'docker',
      [
        'build',
        '--build-arg',
        `MYAPI_BRAND_NAME=${values.MYAPI_BRAND_NAME || 'MyAPI'}`,
        '--build-arg',
        `MYAPI_BRAND_LOGO=${values.MYAPI_BRAND_LOGO || '/myapi-logo-v1.png'}`,
        '-t',
        values.NEW_API_IMAGE || 'local/new-api:custom-rc25',
        '.',
      ],
      { cwd: paths.projectRoot }
    )
    return
  }

  if (command === 'up') {
    const errors = validateRuntimeConfiguration(values)
    if (errors.length > 0) {
      throw new Error(`deployment preflight failed: ${errors.join('; ')}`)
    }
    const composeConfig = run(
      'docker',
      composeArguments(paths, ['config', '--quiet']),
      { cwd: paths.projectRoot, capture: true }
    )
    if (composeConfig.status !== 0) {
      throw new Error(
        `Docker Compose configuration is invalid: ${(composeConfig.stderr || '').trim()}`
      )
    }
  }

  let composeCommand
  if (command === 'up') composeCommand = ['up', '-d', '--force-recreate']
  else if (command === 'down') composeCommand = ['down']
  else if (command === 'status') composeCommand = ['ps']
  else composeCommand = ['logs', '--tail', '100', ...(args.includes('--follow') ? ['--follow'] : [])]
  run('docker', composeArguments(paths, composeCommand), { cwd: paths.projectRoot })
}

function printVersion(args) {
  const value = {
    package: packageMetadata.name,
    version: packageMetadata.version,
    upstream: 'New API v1.0.0-rc.25',
    repository: packageMetadata.repository.url,
  }
  if (args.includes('--json')) console.log(JSON.stringify(value))
  else console.log(`${value.package} ${value.version} (${value.upstream})`)
}

function printHelp() {
  console.log(`MyAPI self-hosting CLI (based on New API)

Usage:
  myapi init [directory]
  myapi configure [--project-dir DIR] [--public-url URL] [--data-dir DIR] [--logs-dir DIR]
  myapi doctor [--project-dir DIR]
  myapi adopt --project-dir DIR --data-dir DIR --logs-dir DIR
  myapi build|up|down|status|logs [--project-dir DIR] [--follow]
  myapi version [--json]

The CLI never deploys during npm install, never removes Docker volumes, and
never prints SESSION_SECRET or API credentials.`)
}

const [command = 'help', ...args] = process.argv.slice(2)

try {
  if (command === 'init') {
    validateArguments(args, { positional: 1 })
    initProject(args)
  } else if (command === 'configure') {
    validateArguments(args, {
      values: [
        '--project-dir',
        '--public-url',
        '--data-dir',
        '--logs-dir',
      ],
      flags: ['--force'],
    })
    configureProject(args)
  } else if (command === 'doctor') {
    validateArguments(args, { values: ['--project-dir'] })
    doctor(args)
  } else if (command === 'adopt') {
    validateArguments(args, {
      values: ['--project-dir', '--data-dir', '--logs-dir'],
    })
    adoptDataPaths(args)
  }
  else if (['build', 'up', 'down', 'status', 'logs'].includes(command)) {
    validateArguments(args, {
      values: ['--project-dir'],
      flags: command === 'logs' ? ['--follow'] : [],
    })
    deploymentCommand(command, args)
  } else if (command === 'version' || command === '--version' || command === '-v') {
    validateArguments(args, { flags: ['--json'] })
    printVersion(args)
  } else if (command === 'help' || command === '--help' || command === '-h') {
    printHelp()
  } else {
    fail(`unknown command "${command}"`)
    printHelp()
  }
} catch (error) {
  fail(error instanceof Error ? error.message : String(error))
}
