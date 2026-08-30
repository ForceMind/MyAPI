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

test('describes immutable loopback status with platform firewall guidance', () => {
  assert.deepEqual(
    config.describeRuntimeConfig({ bindAddress: '127.0.0.1', port: 3000, allowLan: false, isLan: false }, 'win32'),
    {
      bindAddress: '127.0.0.1',
      port: 3000,
      endpoint: 'http://127.0.0.1:3000',
      lanEnabled: false,
      mode: '仅本机（回环地址）',
      firewallHint: 'Windows：如同事无法连接，请在 Windows Defender 防火墙中允许 MyAPI 访问“专用网络”。',
      restartHint: '监听地址和端口在启动时确定。修改参数后请退出 MyAPI，再使用新的参数重新启动；不会在运行中动态切换。',
    },
  )
})

test('describes an opted-in private LAN endpoint and restart requirement', () => {
  const status = config.describeRuntimeConfig(
    { bindAddress: '192.168.1.20', port: 4317, allowLan: true, isLan: true },
    'darwin',
  )
  assert.equal(status.endpoint, 'http://192.168.1.20:4317')
  assert.equal(status.lanEnabled, true)
  assert.match(status.mode, /LAN/)
  assert.match(status.firewallHint, /系统设置/)
  assert.match(status.restartHint, /重新启动/)
})

test('electron-builder packages the shared preflight module', async () => {
  const packageJson = JSON.parse(await readFile(new URL('../package.json', import.meta.url), 'utf8'))
  assert.ok(packageJson.build.files.includes('runtime-config.js'))
})
