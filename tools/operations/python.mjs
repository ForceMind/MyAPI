import { spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { check, OperationsError } from './safety.mjs'

let active = 0
const scripts = new Set(['sqlite_read.py', 'extract_bundle.py', 'backup_data.py'])
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
      if (code !== 0) return reject(new OperationsError('HELPER_FAILED', 503))
      try { resolve(JSON.parse(output)) } catch { reject(new OperationsError('HELPER_FAILED', 503)) }
    })
    child.stdin.on('error', () => {})
    child.stdin.end(JSON.stringify(input))
  }) } finally { active-- }
}
