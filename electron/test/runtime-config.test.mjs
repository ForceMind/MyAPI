import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import config from '../runtime-config.js'

test('defaults to loopback and the standard port', () => {
  assert.deepEqual(config.resolveRuntimeConfig(), {
    bindAddress: '127.0.0.1',
    port: 3000,
    allowLan: false,
    isLan: false,
  })
})

test('requires explicit opt-in for a private LAN address', () => {
  assert.throws(
    () => config.resolveRuntimeConfig({ args: ['--bind-address', '192.168.1.10'] }),
    /disabled by default/,
  )
  assert.deepEqual(
    config.resolveRuntimeConfig({
      args: ['--allow-lan', '--bind-address', '192.168.1.10', '--port', '4317'],
    }),
    { bindAddress: '192.168.1.10', port: 4317, allowLan: true, isLan: true },
  )
})

test('rejects public and non-IPv4 addresses', () => {
  assert.throws(
    () => config.resolveRuntimeConfig({ args: ['--allow-lan', '--bind-address', '8.8.8.8'] }),
    /private IPv4/,
  )
  assert.throws(
    () => config.resolveRuntimeConfig({ args: ['--allow-lan', '--bind-address', '::'] }),
    /private IPv4/,
  )
})

test('rejects an explicitly invalid port instead of silently falling back', () => {
  assert.throws(
    () => config.resolveRuntimeConfig({ args: ['--port', '70000'] }),
    /between 1 and 65535/,
  )
})

test('a saved explicit opt-in survives relaunch without weakening defaults', () => {
  assert.deepEqual(
    config.resolveRuntimeConfig({ saved: { bindAddress: '10.0.0.7', port: 4000, allowLan: true } }),
    { bindAddress: '10.0.0.7', port: 4000, allowLan: true, isLan: true },
  )
  assert.throws(
    () => config.resolveRuntimeConfig({ saved: { bindAddress: '10.0.0.7' } }),
    /disabled by default/,
  )
})

test('electron-builder packages the shared preflight module', async () => {
  const packageJson = JSON.parse(await readFile(new URL('../package.json', import.meta.url), 'utf8'))
  assert.ok(packageJson.build.files.includes('runtime-config.js'))
})
