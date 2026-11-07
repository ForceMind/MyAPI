import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

test('production desktop startup probes the backend status endpoint and validates HTTP status', async () => {
  const source = await readFile(new URL('../main.js', import.meta.url), 'utf8')
  assert.match(source, /checkServerAvailability\(\s*PORT,\s*30,\s*1000,\s*healthCheckHost,\s*['"]\/api\/status['"],\s*\{\s*signal,?\s*\},?\s*\)/)
  assert.match(source, /isSuccessfulHttpStatus\(statusCode\)/)
  assert.match(source, /isSuccessfulStatusResponse\(statusCode, responseBody\)/)
  assert.match(source, /Unexpected HTTP response from \$\{requestPath\}/)
  assert.match(source, /checkServerAvailability\(\s*DEV_FRONTEND_PORT,\s*30,\s*1000,\s*['"]127\.0\.0\.1['"],\s*['"]\/['"],\s*\{\s*signal:\s*developmentProbe\.signal,?\s*\},?\s*\)/)
})

test('production desktop runs the backend as the lite edition', async () => {
  const source = await readFile(new URL('../main.js', import.meta.url), 'utf8')
  assert.match(source, /MYAPI_EDITION:\s*['"]lite['"],/)
  assert.doesNotMatch(source, /MYAPI_EDITION:\s*['"]lan['"]/)
})

test('production desktop stores backend and full-content logs under userData', async () => {
  const source = await readFile(new URL('../main.js', import.meta.url), 'utf8')
  assert.match(source, /const logsDir = path\.join\(userDataPath, ['"]logs['"]\)/)
  assert.match(source, /FULL_CONTENT_LOG_DIR: path\.join\(logsDir, ['"]full-content['"]\)/)
  assert.match(source, /spawn\(binaryPath, \[['"]--log-dir['"], logsDir\]/)
})
