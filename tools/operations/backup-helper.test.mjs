import assert from 'node:assert/strict'
import test from 'node:test'
import { python } from './python.mjs'

test('backup helper failures expose only the defined safe rejection code', async () => {
  await assert.rejects(python('backup_data.py', { action: 'bundle_prepare' }), { code: 'BUNDLE_REQUEST_INVALID' })
  await assert.rejects(python('writer_guard.py', { action: 'unrecognized' }), {
    code: process.platform === 'linux' && process.arch === 'x64' && process.getuid() === 0 ? 'INVALID_REQUEST' : 'ROOT_LINUX_AMD64_REQUIRED',
  })
  await assert.rejects(python('writer_guard.py', { action: 'archive_scan', verified: true }), {
    code: process.platform === 'linux' && process.arch === 'x64' && process.getuid() === 0 ? 'INVALID_REQUEST' : 'ROOT_LINUX_AMD64_REQUIRED',
  })
  await assert.rejects(python('backup_data.py', { action: 'backup' }), { code: 'HELPER_FAILED' })
  await assert.rejects(python('unregistered.py', {}), { code: 'INVALID_HELPER' })
})
