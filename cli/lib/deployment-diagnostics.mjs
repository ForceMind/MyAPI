/*
 * MyAPI distribution and self-hosting tooling.
 * Copyright (C) 2026 ForceMind
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published
 * by the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 */

/**
 * Read-only environment checks behind `myapi doctor`.
 *
 * Every probe that touches the host (external commands, sockets, the file
 * system) goes through an injected environment so the checks can be tested
 * without Docker, the network, or a particular disk. Nothing here writes a
 * file, changes a setting, or starts a container.
 */

import { spawnSync } from 'node:child_process'
import { accessSync, constants, lstatSync, statfsSync } from 'node:fs'
import { createServer } from 'node:net'
import path from 'node:path'

export const MIN_COMPOSE_VERSION = Object.freeze([2, 17, 0])
export const MIN_FREE_BYTES = 1024 ** 3
export const COMMAND_TIMEOUT_MS = 10_000

const KNOWN_EDITIONS = Object.freeze(['full', 'lite', 'lan'])
const GIB = 1024 ** 3

export function parseComposeVersion(text) {
  const match = /(\d+)\.(\d+)(?:\.(\d+))?/.exec(String(text || ''))
  if (!match) return null
  return [Number(match[1]), Number(match[2]), Number(match[3] || 0)]
}

function isOlderVersion(version, minimum) {
  for (let index = 0; index < minimum.length; index++) {
    if (version[index] !== minimum[index]) return version[index] < minimum[index]
  }
  return false
}

function formatGiB(bytes) {
  return `${(bytes / GIB).toFixed(1)} GiB`
}

function isLoopbackHost(address) {
  return address === 'localhost' || address === '127.0.0.1' || address === '::1' || address === '[::1]'
}

/** Production probes. Tests pass their own environment instead. */
export function createDiagnosticsEnvironment({ timeoutMs = COMMAND_TIMEOUT_MS } = {}) {
  return {
    exec(command, args) {
      const result = spawnSync(command, args, {
        encoding: 'utf8',
        timeout: timeoutMs,
        maxBuffer: 64 * 1024,
        shell: false,
        stdio: ['ignore', 'pipe', 'pipe'],
      })
      return {
        status: result.status,
        stdout: result.stdout || '',
        stderr: result.stderr || '',
        errorCode: result.error?.code,
      }
    },
    probePort(host, port) {
      return new Promise((resolve) => {
        const server = createServer()
        server.once('error', (error) => {
          resolve({ state: error.code === 'EADDRINUSE' ? 'in-use' : 'unavailable', code: error.code })
        })
        server.once('listening', () => server.close(() => resolve({ state: 'free' })))
        server.listen({ host, port, exclusive: true })
      })
    },
    // Reports what exists at `directory`; when it is missing, which existing
    // ancestor would receive it. Nothing is created.
    inspectDirectory(directory) {
      let current = directory
      for (;;) {
        let stat
        try {
          stat = lstatSync(current)
        } catch (error) {
          if (error.code !== 'ENOENT' && error.code !== 'ENOTDIR') throw error
          const parent = path.dirname(current)
          if (parent === current) return { kind: 'missing', nearest: current, writable: false }
          current = parent
          continue
        }
        let writable = true
        try {
          accessSync(current, constants.W_OK)
        } catch {
          writable = false
        }
        const kind = current === directory ? (stat.isDirectory() ? 'directory' : 'file') : 'missing'
        return { kind, nearest: current, writable }
      }
    },
    freeBytes(directory) {
      const stats = statfsSync(directory)
      return Number(stats.bavail) * Number(stats.bsize)
    },
  }
}

function checkEdition(rawEdition) {
  const edition = String(rawEdition ?? '').trim().toLowerCase() || 'full'
  if (KNOWN_EDITIONS.includes(edition)) {
    const note = edition === 'lan' ? ' (Legacy alias of lite)' : ''
    return { name: 'MYAPI_EDITION', status: 'ok', detail: `${edition}${note}` }
  }
  return {
    name: 'MYAPI_EDITION',
    status: 'fail',
    detail: `${JSON.stringify(String(rawEdition).slice(0, 40))} is not a supported edition`,
    fix: 'set MYAPI_EDITION to full, lite, or lan in deploy/.env (the application refuses to start with any other value)',
  }
}

function checkDocker(environment) {
  const daemon = environment.exec('docker', ['version', '--format', '{{.Server.Version}}'])
  if (daemon.errorCode === 'ENOENT') {
    return {
      dockerFound: false,
      checks: [{
        name: 'Docker daemon',
        status: 'warn',
        detail: 'docker was not found on PATH (Docker checks skipped)',
        fix: 'install Docker Desktop or Docker Engine to run the container deployment, then run myapi doctor again',
      }],
    }
  }
  const checks = []
  if (daemon.errorCode === 'ETIMEDOUT') {
    checks.push({
      name: 'Docker daemon',
      status: 'fail',
      detail: 'docker did not answer within 10 seconds',
      fix: 'check that Docker is running and responsive (restart Docker Desktop or the docker service)',
    })
  } else if (daemon.status !== 0) {
    checks.push({
      name: 'Docker daemon',
      status: 'fail',
      detail: 'unavailable',
      fix: 'start Docker Desktop, or on Linux run "sudo systemctl start docker"; check that your user may access the Docker socket',
    })
  } else {
    checks.push({ name: 'Docker daemon', status: 'ok', detail: daemon.stdout.trim() || 'running' })
  }

  const compose = environment.exec('docker', ['compose', 'version', '--short'])
  const minimum = `v${MIN_COMPOSE_VERSION.slice(0, 2).join('.')}`
  const installFix = `install Docker Compose ${minimum} or newer (current Docker Desktop releases include it)`
  if (compose.errorCode === 'ETIMEDOUT') {
    checks.push({ name: 'Docker Compose', status: 'fail', detail: 'docker compose did not answer within 10 seconds', fix: installFix })
  } else if (compose.status !== 0) {
    checks.push({ name: 'Docker Compose', status: 'fail', detail: 'unavailable', fix: installFix })
  } else {
    const version = parseComposeVersion(compose.stdout)
    if (!version) {
      checks.push({
        name: 'Docker Compose',
        status: 'warn',
        detail: `could not read the version from ${JSON.stringify(compose.stdout.trim().slice(0, 40))}`,
        fix: `confirm that "docker compose version" reports ${minimum} or newer`,
      })
    } else if (isOlderVersion(version, MIN_COMPOSE_VERSION)) {
      checks.push({
        name: 'Docker Compose',
        status: 'fail',
        detail: `${version.join('.')} is older than the required ${minimum} (myapi up relies on "--wait")`,
        fix: installFix,
      })
    } else {
      checks.push({ name: 'Docker Compose', status: 'ok', detail: version.join('.') })
    }
  }
  return { dockerFound: true, checks }
}

async function checkListenPort({ bindAddress, port, dockerFound }, environment) {
  if (!Number.isInteger(port) || port < 1 || port > 65535) return []
  const host = bindAddress === 'localhost' ? '127.0.0.1' : bindAddress
  const probe = await environment.probePort(host, port)
  const target = `${bindAddress}:${port}`
  if (probe.state === 'free') return [{ name: 'Listen port', status: 'ok', detail: `${target} is free` }]
  if (probe.state === 'in-use') {
    if (dockerFound) {
      const running = environment.exec('docker', [
        'container', 'ls', '--filter', 'name=^/my-api$', '--filter', 'status=running', '--format', '{{.ID}}',
      ])
      if (running.status === 0 && running.stdout.trim() !== '') {
        return [{ name: 'Listen port', status: 'ok', detail: `${target} is in use by the running my-api container` }]
      }
    }
    return [{
      name: 'Listen port',
      status: 'warn',
      detail: `${target} is already in use by another process`,
      fix: `choose a free port by setting MYAPI_PORT in deploy/.env (currently ${port}), or stop the process that holds it`,
    }]
  }
  if (probe.code === 'EADDRNOTAVAIL') {
    return [{
      name: 'Listen port',
      status: 'warn',
      detail: `${bindAddress} is not an address of this machine`,
      fix: 'set MYAPI_BIND_ADDRESS to 127.0.0.1, 0.0.0.0, or an address assigned to this host',
    }]
  }
  if (probe.code === 'EACCES') {
    return [{
      name: 'Listen port',
      status: 'warn',
      detail: `${target} needs elevated privileges`,
      fix: 'use a port above 1023 in MYAPI_PORT, or put a reverse proxy in front',
    }]
  }
  return [{
    name: 'Listen port',
    status: 'warn',
    detail: `${target} could not be probed (${probe.code || 'unknown error'})`,
    fix: 'check MYAPI_BIND_ADDRESS and MYAPI_PORT in deploy/.env',
  }]
}

function checkStorage({ label, variable, directory }, environment) {
  const state = environment.inspectDirectory(directory)
  if (state.kind === 'file') {
    return [{
      name: `${label} directory`,
      status: 'fail',
      detail: `${directory} exists but is not a directory`,
      fix: `move that file away or point ${variable} in deploy/.env at a directory`,
    }]
  }
  const checks = []
  if (state.kind === 'directory') {
    checks.push(state.writable
      ? { name: `${label} directory`, status: 'ok', detail: `${directory} is writable` }
      : {
          name: `${label} directory`,
          status: 'warn',
          detail: `${directory} is not writable by the current user`,
          fix: 'if you manage this directory yourself, grant access with chown or chmod; a root-owned directory written by the container can be ignored',
        })
  } else if (state.writable) {
    checks.push({ name: `${label} directory`, status: 'ok', detail: `${directory} will be created on first start` })
  } else {
    checks.push({
      name: `${label} directory`,
      status: 'warn',
      detail: `${directory} does not exist and ${state.nearest} is not writable by the current user`,
      fix: `create ${directory} with the right owner, or point ${variable} in deploy/.env at a writable location`,
    })
  }
  try {
    const free = environment.freeBytes(state.nearest)
    checks.push(free < MIN_FREE_BYTES
      ? {
          name: `${label} disk space`,
          status: 'warn',
          detail: `${formatGiB(free)} free on the volume holding ${state.nearest} (below ${formatGiB(MIN_FREE_BYTES)})`,
          fix: `free up disk space or point ${variable} in deploy/.env at a larger volume before the first start`,
        }
      : { name: `${label} disk space`, status: 'ok', detail: `${formatGiB(free)} free` })
  } catch {
    checks.push({
      name: `${label} disk space`,
      status: 'warn',
      detail: `could not read free space for ${state.nearest}`,
      fix: 'check the volume manually with "df -h"',
    })
  }
  return checks
}

function checkAccessScope({ edition, bindAddress, allowLan, publicUrlIsLocal, publicUrl }) {
  const restricted = edition === 'lite' || edition === 'lan'
  const loopback = isLoopbackHost(String(bindAddress).trim().toLowerCase())
  if (loopback && allowLan === 'true') {
    return [{
      name: 'Access scope',
      status: 'warn',
      detail: 'MYAPI_ALLOW_LAN=true has no effect while MYAPI_BIND_ADDRESS is a loopback address',
      fix: 'set MYAPI_ALLOW_LAN=false, or bind a private address (or 0.0.0.0) to share on the local network',
    }]
  }
  if (!loopback && allowLan === 'true') {
    const everywhere = bindAddress === '0.0.0.0'
    return [{
      name: 'Access scope',
      status: 'warn',
      detail: everywhere
        ? 'listening on all interfaces; MyAPI cannot verify what your firewall or router exposes'
        : `listening on ${bindAddress}, reachable beyond this machine`,
      fix: 'allow only trusted networks to reach this port; do not forward it from the internet without a reviewed public-access setup, and prefer a reverse proxy with HTTPS',
    }]
  }
  if (restricted && loopback && publicUrl && !publicUrlIsLocal) {
    return [{
      name: 'Access scope',
      status: 'warn',
      detail: 'lite/lan edition listens on loopback only but MYAPI_PUBLIC_URL points at a public origin',
      fix: 'external clients cannot reach a loopback listener without a reverse proxy; use a local URL for personal use, or set up the proxy deliberately',
    }]
  }
  return [{ name: 'Access scope', status: 'ok', detail: loopback ? `bind=${bindAddress} (this machine only)` : `bind=${bindAddress}` }]
}

function checkLegacyKeys(legacyKeys, projectDir) {
  if (legacyKeys.length === 0) return [{ name: 'Legacy NEW_API_* settings', status: 'ok', detail: 'none' }]
  return [{
    name: 'Legacy NEW_API_* settings',
    status: 'warn',
    detail: `${legacyKeys.join(', ')} still in use (read as a compatibility fallback)`,
    fix: `run "myapi migrate --project-dir ${projectDir}" to write the MYAPI_* names`,
  }]
}

/**
 * Collects the host-dependent checks for a deployment. `input` carries
 * already-resolved configuration; the return value never contains secrets.
 */
export async function runDeploymentDiagnostics(input, environment = createDiagnosticsEnvironment()) {
  const edition = String(input.rawEdition ?? '').trim().toLowerCase() || 'full'
  const checks = [checkEdition(input.rawEdition)]
  const docker = checkDocker(environment)
  checks.push(...docker.checks)
  checks.push(...await checkListenPort({
    bindAddress: input.bindAddress,
    port: input.port,
    dockerFound: docker.dockerFound,
  }, environment))
  checks.push(...checkStorage({ label: 'Data', variable: 'MYAPI_DATA_DIR', directory: input.dataDir }, environment))
  checks.push(...checkStorage({ label: 'Log', variable: 'MYAPI_LOGS_DIR', directory: input.logsDir }, environment))
  checks.push(...checkAccessScope({
    edition,
    bindAddress: input.bindAddress,
    allowLan: input.allowLan,
    publicUrlIsLocal: input.publicUrlIsLocal,
    publicUrl: input.publicUrl,
  }))
  checks.push(...checkLegacyKeys(input.legacyKeys, input.projectDir))
  return { dockerFound: docker.dockerFound, checks }
}
