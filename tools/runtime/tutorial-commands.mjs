#!/usr/bin/env node
// Runs only the fixed, source-bound B7 tutorial blocks on a disposable runner.
import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { existsSync, lstatSync, mkdirSync, readFileSync, readlinkSync, realpathSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:net'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const repository = fileURLToPath(new URL('../../', import.meta.url)).replace(/\/$/, '')
const specifications = [
  ['docs/MYAPI_DISTRIBUTION.md', 'full-source-version', 'node cli/myapi.mjs version --json'],
  ['docs/MYAPI_DISTRIBUTION.md', 'full-source-init', 'node cli/myapi.mjs init "${MYAPI_PROJECT_DIR:?}"'],
  ['docs/MYAPI_DISTRIBUTION.md', 'full-source-configure', 'node cli/myapi.mjs configure --project-dir "${MYAPI_PROJECT_DIR:?}" --public-url "${MYAPI_TUTORIAL_PUBLIC_URL:?}"'],
  ['docs/MYAPI_DISTRIBUTION.md', 'full-source-doctor', 'node cli/myapi.mjs doctor --project-dir "${MYAPI_PROJECT_DIR:?}"'],
  ['docs/MYAPI_DISTRIBUTION.md', 'full-source-build', 'node cli/myapi.mjs build --project-dir "${MYAPI_PROJECT_DIR:?}"'],
  ['docs/MYAPI_DISTRIBUTION.md', 'full-source-up', 'node cli/myapi.mjs up --project-dir "${MYAPI_PROJECT_DIR:?}"'],
  ['docs/MYAPI_DISTRIBUTION.md', 'full-source-status', 'node cli/myapi.mjs status --project-dir "${MYAPI_PROJECT_DIR:?}"'],
  ['docs/MYAPI_DISTRIBUTION.md', 'full-source-down', 'node cli/myapi.mjs down --project-dir "${MYAPI_PROJECT_DIR:?}"'],
  ['docs/LAN_LITE.md', 'lite-source-init', 'node cli/myapi.mjs lan init "${MYAPI_PROJECT_DIR:?}" --port "${MYAPI_TUTORIAL_PORT:?}"'],
  ['docs/LAN_LITE.md', 'lite-source-doctor', 'node cli/myapi.mjs doctor --project-dir "${MYAPI_PROJECT_DIR:?}"'],
  ['docs/LAN_LITE.md', 'lite-source-build', 'node cli/myapi.mjs build --project-dir "${MYAPI_PROJECT_DIR:?}"'],
  ['docs/LAN_LITE.md', 'lite-source-start', 'node cli/myapi.mjs lan start --project-dir "${MYAPI_PROJECT_DIR:?}"'],
  ['docs/LAN_LITE.md', 'lite-source-status', 'node cli/myapi.mjs lan status --project-dir "${MYAPI_PROJECT_DIR:?}"'],
  ['docs/LAN_LITE.md', 'lite-private-start', 'node cli/myapi.mjs lan start --project-dir "${MYAPI_PROJECT_DIR:?}" --bind-address "${MYAPI_TUTORIAL_BIND_ADDRESS:?}" --port "${MYAPI_TUTORIAL_PORT:?}" --allow-lan'],
  ['docs/LAN_LITE.md', 'lite-source-stop', 'node cli/myapi.mjs lan stop --project-dir "${MYAPI_PROJECT_DIR:?}"'],
  ['docs/PERSONAL_COMPUTER.md', 'personal-source-cli-help', 'node cli/myapi.mjs --help'],
  ['docs/PERSONAL_COMPUTER.md', 'personal-lite-project-init', 'node cli/myapi.mjs lan init "${MYAPI_PROJECT_DIR:?Set a new empty project directory first}"'],
  ['docs/NATIVE_LINUX.md', 'native-preview-help', 'bash deploy/install-native.sh --help'],
  ['docs/NATIVE_LINUX.md', 'native-preview-system-plan', 'bash deploy/install-native.sh plan --scope system'],
  ['docs/NATIVE_LINUX.md', 'native-preview-system-render', 'bash deploy/install-native.sh render-unit --scope system'],
  ['docs/NATIVE_LINUX.md', 'native-preview-user-plan', 'bash deploy/install-native.sh plan --scope user'],
  ['docs/NATIVE_LINUX.md', 'native-preview-user-render', 'bash deploy/install-native.sh render-unit --scope user'],
  ['electron/README.md', 'desktop-source-contract', 'node tools/desktop/check.mjs --json'],
  ['electron/README.md', 'desktop-source-tests', 'node --test electron/test/*.test.mjs'],
]
const sha256 = value => createHash('sha256').update(value).digest('hex')
function demand(value, code) { if (!value) throw new Error(code) }

export function parseTutorialCommands(sources) {
  const files = [...new Set(specifications.map(([file]) => file))]
  demand(sources && typeof sources === 'object' && !Array.isArray(sources) && Object.keys(sources).sort().join('\n') === files.sort().join('\n'), 'TUTORIAL_DOCUMENT_SET_INVALID')
  const found = new Map()
  for (const file of files) {
    const source = sources[file]
    demand(typeof source === 'string' && Buffer.byteLength(source) <= 1024 * 1024 && !source.includes('\r'), 'TUTORIAL_DOCUMENT_INVALID')
    const lines = source.split('\n')
    let fence = null
    for (let index = 0; index < lines.length; index++) {
      const line = lines[index]
      if (!/<!--\s*tutorial-command\b/.test(line)) {
        if (/^```/.test(line)) {
          if (fence) { demand(line === '```', 'TUTORIAL_FENCE_INVALID'); fence = null }
          else fence = line
        }
        continue
      }
      demand(!fence, 'TUTORIAL_MARKER_IN_CODE')
      const match = /^<!-- tutorial-command: ([a-z][a-z0-9]*(?:-[a-z0-9]+)*) -->$/.exec(line)
      demand(match && lines[index + 1] === '```bash', 'TUTORIAL_MARKER_INVALID')
      const id = match[1], expected = specifications.find(row => row[1] === id)
      demand(expected && expected[0] === file && !found.has(id), 'TUTORIAL_MARKER_UNKNOWN_OR_DUPLICATE')
      let end = index + 2
      while (end < lines.length && !lines[end].startsWith('```')) end++
      demand(lines[end] === '```', 'TUTORIAL_FENCE_INVALID')
      const script = lines.slice(index + 2, end).join('\n') + '\n'
      demand(script === expected[2] + '\n', 'TUTORIAL_COMMAND_CHANGED')
      found.set(id, { id, source_file: file, source_line: index + 3, command_sha256: sha256(script), script })
      index = end
    }
    demand(fence === null, 'TUTORIAL_FENCE_INVALID')
  }
  demand(found.size === specifications.length, 'TUTORIAL_COMMAND_MISSING')
  return specifications.map(([, id]) => found.get(id))
}

export function tutorialScope(env) {
  demand(env && env.GITHUB_ACTIONS === 'true' && env.GITHUB_EVENT_NAME === 'workflow_dispatch' && env.GITHUB_REF === 'refs/heads/dev' &&
    env.GITHUB_REPOSITORY === 'ForceMind/MyAPI' && env.MYAPI_B7_TUTORIALS === '1' && env.MYAPI_TUTORIAL_QUALIFICATION === '1' &&
    env.MYAPI_SMOKE_BROWSER === '0' && env.MYAPI_SMOKE_API_ONLY === '1' && env.MYAPI_SMOKE_PERSONAL === '0', 'TUTORIAL_SCOPE_REJECTED')
  demand(typeof env.GITHUB_SHA === 'string' && /^[a-f0-9]{40}$/.test(env.GITHUB_SHA) && typeof env.GITHUB_RUN_ID === 'string' &&
    /^[1-9][0-9]{0,19}$/.test(env.GITHUB_RUN_ID) && typeof env.GITHUB_RUN_ATTEMPT === 'string' && /^[1-9][0-9]{0,19}$/.test(env.GITHUB_RUN_ATTEMPT), 'TUTORIAL_IDENTITY_REJECTED')
  demand(typeof env.RUNNER_TEMP === 'string' && path.isAbsolute(env.RUNNER_TEMP) && path.normalize(env.RUNNER_TEMP) === env.RUNNER_TEMP &&
    env.RUNNER_TEMP !== '/' && !/[,\u0000-\u001f\u007f-\u009f]/u.test(env.RUNNER_TEMP), 'TUTORIAL_PATH_REJECTED')
  demand((env.DOCKER_HOST === undefined || env.DOCKER_HOST === 'unix:///var/run/docker.sock') && env.DOCKER_CONTEXT === undefined &&
    ['MYAPI_PROJECT_DIR', 'MYAPI_TUTORIAL_PUBLIC_URL', 'MYAPI_TUTORIAL_IMAGE', 'MYAPI_TUTORIAL_PORT', 'MYAPI_TUTORIAL_BIND_ADDRESS'].every(key => env[key] === undefined), 'TUTORIAL_EXTERNAL_INPUT_REJECTED')
  demand(typeof env.BUILDX_BUILDER === 'string' && /^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$/.test(env.BUILDX_BUILDER), 'TUTORIAL_BUILDER_UNBOUND')
  const suffix = `${env.GITHUB_SHA}-${env.GITHUB_RUN_ID}-${env.GITHUB_RUN_ATTEMPT}`
  return { sourceSha: env.GITHUB_SHA, runId: env.GITHUB_RUN_ID, attempt: env.GITHUB_RUN_ATTEMPT,
    root: path.join(env.RUNNER_TEMP, `myapi-b7-${suffix}`), project: `myapi-b7-${suffix}`, bridge: `myapi-b7-private-${suffix}` }
}

export function ownedTutorialPath(root, target) {
  demand(typeof root === 'string' && typeof target === 'string' && path.isAbsolute(root) && path.isAbsolute(target) &&
    path.normalize(root) === root && path.normalize(target) === target && root !== '/' && target.startsWith(root + '/') &&
    !/[,\u0000-\u001f\u007f-\u009f]/u.test(root + target), 'TUTORIAL_OWNED_PATH_REJECTED')
  return target
}

function execute(command, args, options = {}) {
  const result = spawnSync(command, args, { shell: false, encoding: 'utf8', maxBuffer: 32 * 1024 * 1024, timeout: 60000, ...options })
  demand(!result.error && result.signal === null && result.status === 0, 'TUTORIAL_SUBPROCESS_FAILED')
  return result.stdout.trim()
}

function git(args) { return execute('git', ['-c', 'core.fileMode=false', ...args], { cwd: repository }) }

function sourceIdentity(sha) {
  demand(git(['rev-parse', 'HEAD']) === sha && git(['status', '--porcelain', '--untracked-files=no']) === '', 'TUTORIAL_SOURCE_CHANGED')
  const files = git(['ls-files', '-z']).split('\0').filter(Boolean).sort()
  demand(files.length > 0, 'TUTORIAL_SOURCE_UNKNOWN')
  const digest = createHash('sha256')
  for (const file of files) {
    demand(!path.isAbsolute(file) && !file.split('/').includes('..'), 'TUTORIAL_SOURCE_UNKNOWN')
    const absolute = path.join(repository, file), info = lstatSync(absolute)
    demand(info.isFile() || info.isSymbolicLink(), 'TUTORIAL_SOURCE_UNKNOWN')
    digest.update(file + '\0' + (info.isSymbolicLink() ? 'link:' + readlinkSync(absolute) : sha256(readFileSync(absolute))) + '\0')
  }
  return digest.digest('hex')
}

function freePort(host, port) {
  return new Promise((resolve, reject) => {
    const server = createServer()
    server.once('error', () => reject(new Error('TUTORIAL_PORT_UNAVAILABLE')))
    server.listen({ host, port, exclusive: true }, () => server.close(error => error ? reject(new Error('TUTORIAL_PORT_UNAVAILABLE')) : resolve()))
  })
}

export async function runTutorialQualification(env) {
  const report = { schema_version: 1, source_sha: null, run_id: null, run_attempt: null, passed: false, phase: 'guard', error_code: null,
    commands: [], cleanup: { confirmed: false, owned_containers_removed: false, owned_networks_removed: false, owned_image_tags_removed: false, fixture_retained: true },
    local_synthetic_alias: { used: false, ghcr_published: false } }
  let scope, ownership, rootBinding, beforeSource, childEnv, bridgeId, activeRole, activeContainer, aliasId, creating = false
  const images = new Map(), networks = new Map()
  const docker = args => execute('docker', args, { env: childEnv })
  const inspect = (kind, name) => JSON.parse(docker([kind, 'inspect', name]))[0]
  const noContainer = () => demand(docker(['container', 'ls', '-a', '--filter', 'name=^/my-api$', '--format', '{{.ID}}']) === '', 'TUTORIAL_EXISTING_CONTAINER')
  const noImage = name => demand(docker(['image', 'ls', '--quiet', '--no-trunc', name]) === '', 'TUTORIAL_EXISTING_IMAGE')
  const owned = () => {
    const info = lstatSync(scope.root, { bigint: true }), filename = path.join(scope.root, 'ownership.json'), marker = lstatSync(filename)
    demand(info.isDirectory() && !info.isSymbolicLink() && info.uid === BigInt(process.getuid()) && (info.mode & 0o777n) === 0o700n &&
      `${info.dev}:${info.ino}` === rootBinding && realpathSync(scope.root) === scope.root && marker.isFile() && !marker.isSymbolicLink() &&
      marker.nlink === 1 && marker.uid === process.getuid() && (marker.mode & 0o777) === 0o600 && readFileSync(filename, 'utf8') === ownership, 'TUTORIAL_OWNERSHIP_CHANGED')
  }
  const projects = new Map()
  const configuration = role => {
    owned()
    const filename = ownedTutorialPath(scope.root, path.join(projects.get(role).directory, 'deploy/.env'))
    const info = lstatSync(filename)
    demand(info.isFile() && !info.isSymbolicLink() && info.nlink === 1 && info.uid === process.getuid() && (info.mode & 0o777) === 0o600, 'TUTORIAL_CONFIG_INVALID')
    const values = {}
    for (const line of readFileSync(filename, 'utf8').split('\n')) {
      if (!line || line.startsWith('#')) continue
      const match = /^([A-Z][A-Z0-9_]*)=(.*)$/.exec(line)
      demand(match && !Object.hasOwn(values, match[1]), 'TUTORIAL_CONFIG_INVALID')
      values[match[1]] = match[2]
    }
    demand(/^[a-f0-9]{64}$/.test(values.SESSION_SECRET || '') && values.BATCH_UPDATE_ENABLED === 'false' && values.MYAPI_ACCOUNTING_CONFIG_VERSION === '1' &&
      values.MYAPI_DATA_DIR === './data' && values.MYAPI_LOGS_DIR === './logs' && values.MYAPI_EDITION === (role === 'full' ? 'full' : 'lan'), 'TUTORIAL_CONFIG_INVALID')
    return { filename, values }
  }
  const bindConfiguration = role => {
    const current = projects.get(role), { filename, values } = configuration(role)
    current.secretHash = sha256(values.SESSION_SECRET)
    const replacements = { MYAPI_IMAGE: current.image, MYAPI_BUILD_LOCAL: 'true', MYAPI_PORT: String(current.port), MYAPI_BIND_ADDRESS: '127.0.0.1',
      MYAPI_CPU_LIMIT: '2.0', MYAPI_MEMORY_LIMIT: '2g', MYAPI_BUILD_PARALLELISM: '2' }
    let text = readFileSync(filename, 'utf8')
    for (const [key, value] of Object.entries(replacements)) {
      const pattern = new RegExp(`^${key}=.*$`, 'm')
      text = pattern.test(text) ? text.replace(pattern, `${key}=${value}`) : text + `\n${key}=${value}\n`
    }
    writeFileSync(filename, text, { mode: 0o600 })
    demand(configuration(role).values.SESSION_SECRET === values.SESSION_SECRET, 'TUTORIAL_SECRET_CHANGED')
  }
  const runtimeIdentity = (role, requireHealthy = true) => {
    owned()
    const current = projects.get(role), config = configuration(role).values
    demand(sha256(config.SESSION_SECRET) === current.secretHash, 'TUTORIAL_SECRET_CHANGED')
    const container = inspect('container', 'my-api')
    const labels = container.Config?.Labels || {}, mounts = container.Mounts || []
    demand(/^[a-f0-9]{64}$/.test(container.Id || '') && container.Image === images.get(role) && container.Config?.Image === config.MYAPI_IMAGE && container.Name === '/my-api' &&
      labels['com.docker.compose.project'] === scope.project && labels['com.docker.compose.service'] === 'my-api' &&
      labels['com.docker.compose.project.working_dir'] === path.join(current.directory, 'deploy') && labels['com.docker.compose.project.config_files'] === path.join(current.directory, 'deploy/docker-compose.yml') &&
      mounts.length === 2 && mounts.every(mount => mount.Type === 'bind' && mount.RW === true &&
        mount.Source === path.join(current.directory, 'deploy', mount.Destination === '/data' ? 'data' : mount.Destination === '/app/logs' ? 'logs' : '__invalid__')), 'TUTORIAL_CONTAINER_IDENTITY')
    const entries = container.Config.Env
    const exactEnv = (key, value) => demand(Array.isArray(entries) && entries.filter(entry => entry.startsWith(key + '=')).join('\n') === `${key}=${value}`, 'TUTORIAL_RUNTIME_ENV_MISMATCH')
    exactEnv('MYAPI_EDITION', role === 'full' ? 'full' : 'lan')
    exactEnv('SESSION_COOKIE_SECURE', role === 'full' ? 'true' : 'false')
    exactEnv('SESSION_SECRET', config.SESSION_SECRET)
    exactEnv('BATCH_UPDATE_ENABLED', 'false')
    if (requireHealthy) demand(container.State?.Running === true && container.State?.Health?.Status === 'healthy', 'TUTORIAL_RUNTIME_NOT_HEALTHY')
    const bindings = container.HostConfig?.PortBindings?.['3000/tcp']
    demand(Array.isArray(bindings) && bindings.length === 1 && bindings[0].HostIp === current.bind && bindings[0].HostPort === String(current.port), 'TUTORIAL_RUNTIME_PORT_MISMATCH')
    const names = Object.keys(container.NetworkSettings?.Networks || {})
    demand(names.length === 1 && names[0] === `${scope.project}_default`, 'TUTORIAL_NETWORK_UNKNOWN')
    const network = inspect('network', names[0])
    demand(network.Labels?.['com.docker.compose.project'] === scope.project && network.Labels?.['com.docker.compose.network'] === 'default', 'TUTORIAL_NETWORK_UNKNOWN')
    networks.set(network.Id, names[0])
    return container
  }
  const status = async role => {
    const current = projects.get(role)
    const container = runtimeIdentity(role)
    activeRole = role; activeContainer = container.Id; creating = false
    const response = await fetch(`http://${current.bind}:${current.port}/api/status`, { redirect: 'error', signal: AbortSignal.timeout(10000) })
    demand(response.ok && Number(response.headers.get('content-length') || 0) <= 1024 * 1024, 'TUTORIAL_STATUS_INVALID')
    const text = await response.text()
    demand(text.length <= 1024 * 1024, 'TUTORIAL_STATUS_INVALID')
    const value = JSON.parse(text)
    demand(value.success === true && value.data?.product_edition === (role === 'full' ? 'full' : 'lite') && value.data?.version === readFileSync(path.join(repository, 'VERSION'), 'utf8').trim() && value.data.setup === false, 'TUTORIAL_STATUS_INVALID')
    return { product_edition: value.data.product_edition, version: value.data.version, setup: false, secure_cookie_configured: role === 'full', tls_ingress_verified: false }
  }
  const stopped = async role => {
    noContainer()
    const current = projects.get(role)
    let unreachable = false
    try { await fetch(`http://${current.bind}:${current.port}/api/status`, { redirect: 'error', signal: AbortSignal.timeout(3000) }) } catch { unreachable = true }
    demand(unreachable && existsSync(path.join(current.directory, 'deploy/data/my-api.db')) && existsSync(path.join(current.directory, 'deploy/logs')), 'TUTORIAL_STOP_UNCONFIRMED')
    activeRole = null; activeContainer = null
    for (const [id, name] of networks) {
      demand(docker(['network', 'ls', '--filter', `id=${id}`, '--format', '{{.ID}}']) === '', 'TUTORIAL_NETWORK_NOT_REMOVED')
      networks.delete(id)
    }
  }
  try {
    scope = tutorialScope(env)
    demand(process.platform === 'linux' && process.arch === 'x64' && Number(process.versions.node.split('.')[0]) >= 22 && process.argv.length === 2, 'TUTORIAL_PLATFORM_OR_ARGUMENTS_REJECTED')
    Object.assign(report, { source_sha: scope.sourceSha, run_id: scope.runId, run_attempt: scope.attempt })
    beforeSource = sourceIdentity(scope.sourceSha)
    const commands = parseTutorialCommands(Object.fromEntries([...new Set(specifications.map(([file]) => file))].map(file => [file, readFileSync(path.join(repository, file), 'utf8')])))
    const temp = lstatSync(env.RUNNER_TEMP)
    demand(temp.isDirectory() && !temp.isSymbolicLink() && temp.uid === process.getuid() && realpathSync(env.RUNNER_TEMP) === env.RUNNER_TEMP && !existsSync(scope.root), 'TUTORIAL_TEMP_UNSAFE')
    mkdirSync(scope.root, { mode: 0o700 })
    const created = lstatSync(scope.root, { bigint: true })
    rootBinding = `${created.dev}:${created.ino}`
    ownership = JSON.stringify({ schema_version: 1, source_sha: scope.sourceSha, run_id: scope.runId, run_attempt: scope.attempt })
    writeFileSync(path.join(scope.root, 'ownership.json'), ownership, { flag: 'wx', mode: 0o600 })
    childEnv = Object.fromEntries(['PATH', 'HOME', 'LANG', 'LC_ALL', 'BUILDX_BUILDER'].filter(key => env[key] !== undefined).map(key => [key, env[key]]))
    Object.assign(childEnv, { DOCKER_HOST: 'unix:///var/run/docker.sock', DOCKER_BUILDKIT: '1', COMPOSE_PROJECT_NAME: scope.project,
      TMPDIR: scope.root, RUNNER_TEMP: scope.root, GITHUB_ACTIONS: 'true', GOMAXPROCS: '2', MYAPI_BUILD_PARALLELISM: '2', MYAPI_TUTORIAL_PUBLIC_URL: 'https://tutorial.myapi.invalid' })
    const socket = lstatSync('/var/run/docker.sock')
    demand(socket.isSocket() && !socket.isSymbolicLink(), 'TUTORIAL_DOCKER_SOCKET_REJECTED')
    noContainer()
    demand(docker(['container', 'ls', '-a', '--filter', `label=com.docker.compose.project=${scope.project}`, '--format', '{{.ID}}']) === '' &&
      docker(['network', 'ls', '--filter', `label=com.docker.compose.project=${scope.project}`, '--format', '{{.ID}}']) === '' &&
      docker(['network', 'ls', '--filter', `name=^${scope.bridge}$`, '--format', '{{.ID}}']) === '', 'TUTORIAL_EXISTING_RESOURCE')
    const subnetByte = Number.parseInt(sha256(scope.project).slice(0, 2), 16)
    const privateAddress = `10.254.${subnetByte}.1`
    const subnet = `10.254.${subnetByte}.0/24`
    bridgeId = docker(['network', 'create', '--label', `io.myapi.b7.source=${scope.sourceSha}`, '--label', `io.myapi.b7.run=${scope.runId}-${scope.attempt}`, '--subnet', subnet, '--gateway', privateAddress, scope.bridge])
    const bridge = inspect('network', bridgeId)
    demand(bridge.Id === bridgeId && bridge.Name === scope.bridge && bridge.IPAM?.Config?.length === 1 && bridge.IPAM.Config[0].Subnet === subnet && bridge.IPAM.Config[0].Gateway === privateAddress, 'TUTORIAL_BRIDGE_UNKNOWN')
    for (const [role, port] of [['full', 18331], ['lite', 18332], ['personal', 18333]]) {
      const directory = ownedTutorialPath(scope.root, path.join(scope.root, role))
      const image = `local/myapi-tutorial-${role}:${scope.sourceSha}-${scope.runId}-${scope.attempt}`
      noImage(image)
      projects.set(role, { directory, image, port, bind: '127.0.0.1' })
      await freePort('127.0.0.1', port)
    }
    await freePort(privateAddress, 18332)
    const alias = `ghcr.io/forcemind/myapi-lan:v${readFileSync(path.join(repository, 'VERSION'), 'utf8').trim()}`
    noImage(alias)
    for (const block of commands) {
      report.phase = block.id
      owned()
      const role = block.id.startsWith('full-') ? 'full' : block.id.startsWith('lite-') ? 'lite' : 'personal'
      const current = projects.get(role)
      const blockEnv = { ...childEnv, MYAPI_PROJECT_DIR: current.directory, MYAPI_TUTORIAL_PORT: String(current.port), MYAPI_TUTORIAL_IMAGE: current.image, MYAPI_TUTORIAL_BIND_ADDRESS: privateAddress }
      for (const [key, suffix] of [['XDG_DATA_HOME', 'xdg-data'], ['XDG_CONFIG_HOME', 'xdg-config'], ['XDG_STATE_HOME', 'xdg-state'], ['XDG_CACHE_HOME', 'xdg-cache']]) blockEnv[key] = ownedTutorialPath(scope.root, path.join(scope.root, suffix))
      if (block.id.endsWith('-build')) { noContainer(); noImage(current.image) }
      if (['full-source-up', 'lite-source-start'].includes(block.id)) {
        noContainer()
        await freePort(current.bind, current.port)
        const values = configuration(role).values
        demand(values.MYAPI_IMAGE === current.image && values.MYAPI_BUILD_LOCAL === 'true' && inspect('image', current.image).Id === images.get(role), 'TUTORIAL_IMAGE_CHANGED')
      }
      if (['full-source-status', 'full-source-down', 'lite-source-status', 'lite-source-stop', 'lite-private-start'].includes(block.id)) runtimeIdentity(role)
      if (block.id === 'lite-private-start') {
        await freePort(privateAddress, current.port)
        noImage(alias)
        demand(inspect('image', current.image).Id === images.get('lite'), 'TUTORIAL_IMAGE_CHANGED')
        docker(['image', 'tag', images.get('lite'), alias])
        aliasId = images.get('lite'); report.local_synthetic_alias.used = true
        demand(inspect('image', alias).Id === aliasId && configuration(role).values.MYAPI_BUILD_LOCAL === 'true', 'TUTORIAL_IMAGE_CHANGED')
      }
      if (['full-source-up', 'lite-source-start', 'lite-private-start'].includes(block.id)) {
        if (block.id === 'lite-private-start') current.bind = privateAddress
        activeRole = role; creating = true
      }
      const row = { id: block.id, source_file: block.source_file, source_line: block.source_line, command_sha256: block.command_sha256,
        exit_code: null, checks: { executed: false, exit_zero: false, postcondition_verified: false }, runtime: null }
      report.commands.push(row)
      const result = spawnSync('bash', ['--noprofile', '--norc', '-e', '-o', 'pipefail', '-c', block.script], {
        cwd: repository, env: blockEnv, encoding: 'utf8', timeout: block.id.endsWith('-build') ? 25 * 60 * 1000 : 5 * 60 * 1000, maxBuffer: 32 * 1024 * 1024,
      })
      row.checks.executed = !result.error || typeof result.pid === 'number' && result.pid > 0
      row.exit_code = Number.isInteger(result.status) ? result.status : null
      demand(!result.error && result.signal === null && result.status === 0, 'TUTORIAL_MARKED_COMMAND_FAILED')
      row.checks.exit_zero = true
      if (block.id === 'full-source-version') demand(JSON.parse(result.stdout).version === readFileSync(path.join(repository, 'VERSION'), 'utf8').trim(), 'TUTORIAL_VERSION_MISMATCH')
      else if (block.id === 'full-source-init') demand(existsSync(path.join(current.directory, 'deploy/docker-compose.yml')) && !existsSync(path.join(current.directory, 'deploy/.env')), 'TUTORIAL_INIT_INVALID')
      else if (['full-source-configure', 'lite-source-init'].includes(block.id)) bindConfiguration(role)
      else if (block.id === 'personal-lite-project-init') demand(configuration(role).values.MYAPI_EDITION === 'lan' && configuration(role).values.MYAPI_BIND_ADDRESS === '127.0.0.1', 'TUTORIAL_PERSONAL_INIT_INVALID')
      else if (block.id.endsWith('-build')) {
        const image = inspect('image', current.image)
        demand(/^sha256:[a-f0-9]{64}$/.test(image.Id) && image.Os === 'linux' && image.Architecture === 'amd64' && image.Config?.Labels?.['io.myapi.edition'] === (role === 'full' ? 'full' : 'lan'), 'TUTORIAL_IMAGE_UNKNOWN')
        demand(role !== 'lite' || image.Id !== images.get('full'), 'TUTORIAL_IMAGE_UNKNOWN')
        images.set(role, image.Id)
      } else if (['full-source-up', 'full-source-status', 'lite-source-start', 'lite-source-status', 'lite-private-start'].includes(block.id)) {
        row.runtime = await status(role)
      } else if (['full-source-down', 'lite-source-stop'].includes(block.id)) await stopped(role)
      else if (block.id.endsWith('-doctor')) demand(/Docker Compose configuration/.test(result.stdout) && !/\[fail\]/i.test(result.stdout), 'TUTORIAL_DOCTOR_INCOMPLETE')
      else if (block.id === 'personal-source-cli-help') demand(result.stdout.includes('MyAPI self-hosting CLI'), 'TUTORIAL_HELP_INVALID')
      else if (block.id === 'desktop-source-contract') demand(JSON.parse(result.stdout).passed === true, 'TUTORIAL_DESKTOP_CONTRACT_FAILED')
      else if (block.id === 'desktop-source-tests') demand(/# fail 0\b/.test(result.stdout) && /# skipped 0\b/.test(result.stdout) && /# tests [1-9][0-9]*\b/.test(result.stdout), 'TUTORIAL_DESKTOP_TESTS_INCOMPLETE')
      else if (block.id.startsWith('native-preview-')) {
        if (block.id.endsWith('-render')) demand(result.stdout.includes('[Service]') && result.stdout.includes('TimeoutStopSec=infinity') && !result.stdout.includes('@DATA@'), 'TUTORIAL_NATIVE_RENDER_INVALID')
        else demand(result.stdout.includes('预览') && result.stdout.includes(block.id.endsWith('-plan') ? 'scope:' : 'render-unit'), 'TUTORIAL_NATIVE_PREVIEW_INVALID')
      } else throw new Error('TUTORIAL_UNHANDLED_MARKER')
      row.checks.postcondition_verified = true
    }
    demand(report.commands.length === 24 && sourceIdentity(scope.sourceSha) === beforeSource, 'TUTORIAL_SOURCE_CHANGED')
    report.passed = true
  } catch (error) {
    report.error_code = /^TUTORIAL_[A-Z_]+$/.test(error.message || '') ? error.message : 'TUTORIAL_EXECUTION_UNCONFIRMED'
  } finally {
    if (scope && childEnv && ownership) {
      try {
        owned()
        if (docker(['container', 'ls', '-a', '--filter', 'name=^/my-api$', '--format', '{{.ID}}'])) {
          demand(activeRole && (activeContainer || creating), 'TUTORIAL_CLEANUP_OWNERSHIP_UNKNOWN')
          const container = runtimeIdentity(activeRole, false)
          demand(creating || container.Id === activeContainer, 'TUTORIAL_CLEANUP_OWNERSHIP_UNKNOWN')
          if (container.State?.Running === true) {
            docker(['container', 'stop', '--time', '150', container.Id])
            const closed = inspect('container', container.Id)
            demand(closed.State?.Running === false && closed.State?.ExitCode === 0 && closed.State?.OOMKilled === false, 'TUTORIAL_CLEANUP_STOP_UNCONFIRMED')
          } else demand(container.State?.Status === 'exited' || container.State?.Status === 'created', 'TUTORIAL_CLEANUP_STOP_UNCONFIRMED')
          docker(['container', 'rm', container.Id])
        }
        noContainer()
        report.cleanup.owned_containers_removed = true
        for (const [id, name] of networks) {
          const network = inspect('network', id)
          demand(network.Id === id && network.Name === name && network.Labels?.['com.docker.compose.project'] === scope.project && Object.keys(network.Containers || {}).length === 0, 'TUTORIAL_CLEANUP_OWNERSHIP_UNKNOWN')
          docker(['network', 'rm', id])
        }
        if (bridgeId) {
          const bridge = inspect('network', bridgeId)
          demand(bridge.Id === bridgeId && bridge.Name === scope.bridge && bridge.Labels?.['io.myapi.b7.source'] === scope.sourceSha && bridge.Labels?.['io.myapi.b7.run'] === `${scope.runId}-${scope.attempt}` && Object.keys(bridge.Containers || {}).length === 0, 'TUTORIAL_CLEANUP_OWNERSHIP_UNKNOWN')
          docker(['network', 'rm', bridgeId])
        }
        demand(docker(['network', 'ls', '--filter', `name=^${scope.bridge}$`, '--format', '{{.ID}}']) === '' &&
          docker(['network', 'ls', '--filter', `label=com.docker.compose.project=${scope.project}`, '--format', '{{.ID}}']) === '', 'TUTORIAL_CLEANUP_OWNERSHIP_UNKNOWN')
        report.cleanup.owned_networks_removed = true
        const alias = `ghcr.io/forcemind/myapi-lan:v${readFileSync(path.join(repository, 'VERSION'), 'utf8').trim()}`
        if (aliasId) { demand(inspect('image', alias).Id === aliasId, 'TUTORIAL_CLEANUP_IMAGE_UNKNOWN'); docker(['image', 'rm', alias]) }
        else noImage(alias)
        for (const [role, id] of images) { demand(inspect('image', projects.get(role).image).Id === id, 'TUTORIAL_CLEANUP_IMAGE_UNKNOWN'); docker(['image', 'rm', projects.get(role).image]) }
        for (const current of projects.values()) noImage(current.image)
        noImage(alias)
        report.cleanup.owned_image_tags_removed = true
        demand(sourceIdentity(scope.sourceSha) === beforeSource, 'TUTORIAL_SOURCE_CHANGED')
        report.cleanup.confirmed = true
      } catch {
        report.passed = false
        report.error_code = report.error_code || 'TUTORIAL_OWNED_CLEANUP_UNCONFIRMED'
      }
    }
  }
  if (report.passed) report.phase = 'completed'
  return report
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const report = await runTutorialQualification(process.env)
  process.stdout.write(JSON.stringify(report) + '\n')
  process.exitCode = report.passed && report.cleanup.confirmed ? 0 : 1
}
