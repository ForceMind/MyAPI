import { readFileSync, statSync } from 'node:fs'
import { pathToFileURL } from 'node:url'

// This driver controls an explicitly configured local acceptance server. It
// never enables the mode, sends inference, reads deployment files or logs in.
export async function acceptanceControl({ baseURL, accessToken, action, runID, spec, fetchImpl = fetch }) {
  const base = new URL(baseURL)
  if (base.protocol !== 'http:' || !['127.0.0.1', '[::1]'].includes(base.hostname) ||
      base.username || base.password || base.search || base.hash || base.pathname !== '/') {
    throw new Error('An explicit HTTP loopback acceptance server is required')
  }
  if (typeof accessToken !== 'string' || !accessToken || /[\r\n]/.test(accessToken)) {
    throw new Error('A local acceptance session is required')
  }
  if (!['create', 'status', 'stop'].includes(action) || action !== 'create' && !/^[A-Za-z0-9:_-]{1,64}$/.test(runID ?? '')) {
    throw new Error('Expected create, status or stop with an explicit run ID')
  }
  if (action === 'create' && (!spec || !Number.isSafeInteger(spec.channel_id) || spec.channel_id < 1 || !spec.limits)) {
    throw new Error('An explicit channel and limits specification is required')
  }
  const path = action === 'create' ? '/api/upstream-acceptance/runs' :
    `/api/upstream-acceptance/runs/${runID}${action === 'stop' ? '/stop' : ''}`
  const response = await fetchImpl(new URL(path, base), {
    method: action === 'status' ? 'GET' : 'POST',
    redirect: 'error', signal: AbortSignal.timeout(10_000),
    headers: { Authorization: `Bearer ${accessToken}`, Origin: base.origin, 'Content-Type': 'application/json' },
    ...(action === 'create' ? { body: JSON.stringify(spec) } : {}),
  })
  const body = await response.json()
  if (!response.ok || body?.success !== true || !body.data) {
    throw new Error('Acceptance control was refused; inspect the run with the same ID')
  }
  return body.data
}

async function main() {
  const action = process.argv[2]
  const args = process.argv.slice(3)
  const options = {}
  for (let i = 0; i < args.length; i += 2) {
    if (!['--base-url', '--session-file', '--run-id', '--spec-file'].includes(args[i]) || !args[i + 1] || options[args[i]]) {
      throw new Error('Use explicit --base-url, --session-file and --spec-file or --run-id')
    }
    options[args[i]] = args[i + 1]
  }
  const sessionPath = options['--session-file']
  if (!sessionPath || (statSync(sessionPath).mode & 0o077) !== 0 || statSync(sessionPath).size > 16_384) {
    throw new Error('Use an owner-only local session file (0600), at most 16 KiB')
  }
  const session = JSON.parse(readFileSync(sessionPath, 'utf8'))
  const specPath = options['--spec-file']
  if (specPath && statSync(specPath).size > 8192) throw new Error('Limits specification is too large')
  const data = await acceptanceControl({
    action, baseURL: options['--base-url'], accessToken: session.access_token,
    runID: options['--run-id'], spec: specPath ? JSON.parse(readFileSync(specPath, 'utf8')) : undefined,
  })
  // A stopping response is deliberately reported as stopping. No cancellation
  // acknowledgment is presented as a completed drain.
  process.stdout.write(`${JSON.stringify({ success: true, data }, null, 2)}\n`)
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch(() => {
    process.stderr.write('Acceptance control failed. Check the explicit local URL, session, scope and run status.\n')
    process.exitCode = 1
  })
}
