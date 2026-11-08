import { spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { check, OperationsError } from './safety.mjs'

let active = 0
const scripts = new Set(['sqlite_read.py', 'extract_bundle.py', 'backup_data.py', 'writer_guard.py'])
// The executable and script are fixed; secrets and registered paths use stdin only.
export async function python(script, input, timeout = 30_000) {
  check(scripts.has(script), 'INVALID_HELPER')
  check(active < 8, 'HELPER_BUSY', 503)
  active++
  try { return await new Promise((resolve, reject) => {
    const child = spawn('/usr/bin/python3', ['-I', fileURLToPath(new URL(script, import.meta.url))], { stdio: ['pipe', 'pipe', 'pipe'], env: { PATH: '/usr/bin:/bin', LANG: 'C.UTF-8' } })
    let output = '', bytes = 0
    const timer = setTimeout(() => child.kill('SIGKILL'), timeout)
    child.stdout.on('data', chunk => { bytes += chunk.length; if (bytes > 64 * 1024) child.kill('SIGKILL'); else output += chunk })
    child.stderr.resume() // Python exceptions/paths are never exposed or logged.
    child.on('error', () => { clearTimeout(timer); reject(new OperationsError('HELPER_FAILED', 503)) })
    child.on('close', code => {
      clearTimeout(timer)
      if (code !== 0) {
        // Only these new backup helpers define a bounded, payload-free error
        // envelope. Legacy failures retain their existing opaque contract.
        try {
          const result = JSON.parse(output)
          const archiveError = input?.action === 'archive_scan'
          const writerKeys = archiveError ? 'action,code,deletion_authorized,schema_version,status' : 'action,code,schema_version,status'
          if (script === 'writer_guard.py' && result?.schema_version === 1 && ['blocked', 'unknown'].includes(result.status) &&
            Object.keys(result).sort().join(',') === writerKeys &&
            (!archiveError || result.action === 'archive_scan' && result.deletion_authorized === false) &&
            /^[A-Z][A-Z0-9_]{0,79}$/.test(result.code)) {
            return reject(new OperationsError(result.code, 409))
          }
          if (script === 'backup_data.py' && result?.schema_version === 1 && result.verified === false &&
            Object.keys(result).sort().join(',') === 'error,schema_version,verified' && /^BUNDLE_[A-Z0-9_]{1,73}$/.test(result.error)) {
            return reject(new OperationsError(result.error, 409))
          }
        } catch { /* Unrecognized output is never printed or promoted. */ }
        return reject(new OperationsError('HELPER_FAILED', 503))
      }
      try { resolve(JSON.parse(output)) } catch { reject(new OperationsError('HELPER_FAILED', 503)) }
    })
    child.stdin.on('error', () => {})
    child.stdin.end(JSON.stringify(input))
  }) } finally { active-- }
}
