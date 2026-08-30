import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

test('production desktop startup probes the backend status endpoint and validates HTTP status', async () => {
  const source = await readFile(new URL('../main.js', import.meta.url), 'utf8')
  assert.match(source, /checkServerAvailability\(PORT, 30, 1000, healthCheckHost, '\/api\/status'\)/)
  assert.match(source, /isSuccessfulHttpStatus\(statusCode\)/)
  assert.match(source, /Unexpected HTTP status \$\{statusCode\}/)
  assert.match(source, /checkServerAvailability\(DEV_FRONTEND_PORT, 30, 1000, '127\.0\.0\.1', '\/'\)/)
})
