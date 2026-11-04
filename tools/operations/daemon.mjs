#!/usr/bin/env node
import https from 'node:https'
import http from 'node:http'
import { spawn } from 'node:child_process'
import { chmod, lstat, unlink } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { loadConfig } from './config.mjs'
import { createOperationsService } from './service.mjs'
import { check, protectedRead } from './safety.mjs'

export { createOperationsService } from './service.mjs'


// flock is held by a fixed helper on a stable inode. No PID checks, stale-lock
// unlink, shell or recovery race; EOF after a Node crash releases the OS lock.
export async function claimDaemon(config) {
  return new Promise((resolve, reject) => {
    const child = spawn('/usr/bin/python3', ['-I', fileURLToPath(new URL('lock_guard.py', import.meta.url))],
      { stdio: ['pipe', 'pipe', 'pipe'], env: { PATH: '/usr/bin:/bin', LANG: 'C.UTF-8' } })
    let output = '', acquired = false, released = false, exited = false, lost = false, lostHandler
    const timer = setTimeout(() => child.kill('SIGKILL'), 5000)
    const failed = () => { clearTimeout(timer); reject(new Error('OPERATIONS_DAEMON_LOCKED')) }
    child.stderr.resume()
    child.on('error', failed)
    child.stdin.on('error', () => {})
    child.stdout.on('data', chunk => {
      output += chunk
      if (output.length > 256) { child.kill('SIGKILL'); return }
      if (!output.includes('\n') || acquired) return
      let result
      try { result = JSON.parse(output.trim()) } catch { child.kill('SIGKILL'); return }
      if (result.locked !== true) { child.kill('SIGKILL'); return }
      acquired = true; clearTimeout(timer)
      resolve({
        onLost(handler) { lostHandler = handler; if (lost && !released) handler() },
        release() {
          released = true
          return new Promise(done => {
            if (exited) return done()
            child.once('close', done)
            child.stdin.end()
          })
        },
      })
    })
    child.once('close', () => {
      exited = true
      clearTimeout(timer)
      if (!acquired) failed()
      else if (!released) { lost = true; lostHandler?.() }
    })
    child.stdin.write(JSON.stringify({ lock_file: path.join(config.state_dir, 'daemon.lock') }) + '\n')
  })
}

export async function startDaemon(filename) {
  const config = await loadConfig(filename)
  const guard = await claimDaemon(config)
  guard.onLost(() => { process.stderr.write('OPERATIONS_LOCK_LOST\n'); process.exit(1) })
  let service, publicServer, bridgeServer
  const closeServers = () => Promise.all([publicServer, bridgeServer].filter(Boolean).map(server => new Promise(resolve => server.close(resolve))))
  try {
    // Exclusive OS ownership proves any existing private socket is stale.
    try {
      const socket = await lstat(config.unix_socket)
      check(socket.isSocket() && socket.uid === 0 && (socket.mode & 0o777) === 0o600, 'BRIDGE_SOCKET_UNSAFE')
      await unlink(config.unix_socket)
    } catch (error) { if (error.code !== 'ENOENT') throw error }
    const bridgeToken = (await protectedRead(config.bridge_token_file)).toString().trim()
    service = await createOperationsService(config, { bridgeToken })
    const tls = { cert: await protectedRead(config.listen.tls_cert_file), key: await protectedRead(config.listen.tls_key_file), minVersion: 'TLSv1.2' }
    publicServer = https.createServer(tls, service.publicHandler)
    bridgeServer = http.createServer(service.bridgeHandler)
    for (const server of [publicServer, bridgeServer]) { server.requestTimeout = 30_000; server.headersTimeout = 15_000; server.maxHeadersCount = 24 }
    const listen = (server, options) => new Promise((resolve, reject) => { server.once('error', reject); server.listen(options, resolve) })
    await listen(bridgeServer, config.unix_socket); await chmod(config.unix_socket, 0o600)
    await listen(publicServer, { host: config.listen.host, port: config.listen.port })
  } catch (error) {
    await closeServers(); await guard.release(); throw error
  }
  let closed = false
  return { ...service, publicServer, bridgeServer, close: async () => {
    if (closed) return
    closed = true
    await closeServers()
    await service.close()
    await unlink(config.unix_socket).catch(() => {})
    await guard.release()
  } }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  if (process.argv.length !== 3) { process.stderr.write('OPERATIONS_INVALID_CONFIG\n'); process.exitCode = 1 }
  else startDaemon(process.argv[2]).then(daemon => {
    let closing = false
    for (const signal of ['SIGTERM', 'SIGINT']) process.on(signal, () => {
      if (closing) return
      closing = true
      daemon.close().then(() => process.exit(0), () => process.exit(1))
    })
  }).catch(() => { process.stderr.write('OPERATIONS_START_FAILED\n'); process.exitCode = 1 })
}
