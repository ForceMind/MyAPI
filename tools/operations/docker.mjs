import http from 'node:http'
import { createReadStream } from 'node:fs'
import { lstat } from 'node:fs/promises'
import path from 'node:path'
import { check, digest, OperationsError } from './safety.mjs'

// No environment-selected endpoint, CLI, exec, build, delete, mount or URL API.
export class DockerClient {
  constructor(socket = '/var/run/docker.sock') { check(socket === '/var/run/docker.sock', 'INVALID_DOCKER_SOCKET'); this.socket = socket }
  async request(method, route, { json, stream, timeout = 30_000, max = 4 * 1024 * 1024 } = {}) {
    return new Promise((resolve, reject) => {
      const body = json === undefined ? undefined : Buffer.from(JSON.stringify(json))
      const request = http.request({ socketPath: this.socket, path: '/v1.45' + route, method,
        headers: { ...(body ? { 'Content-Type': 'application/json', 'Content-Length': body.length } : {}), ...(stream ? { 'Content-Type': 'application/x-tar' } : {}) } }, response => {
        let length = 0; const chunks = []
        response.on('data', chunk => { length += chunk.length; if (length > max) response.destroy(); else chunks.push(chunk) })
        response.on('error', () => reject(new OperationsError('DOCKER_FAILED', 503)))
        response.on('end', () => {
          if (response.statusCode < 200 || response.statusCode >= 300) return reject(new OperationsError('DOCKER_FAILED', 503))
          const data = Buffer.concat(chunks).toString('utf8')
          if (!data) return resolve(null)
          try { const parsed = JSON.parse(data); check(!parsed?.error, 'DOCKER_FAILED', 503); resolve(parsed) } catch {
            // /images/load returns newline-delimited progress, never expose it.
            if (route.startsWith('/images/load?')) {
              try { for (const line of data.trim().split('\n')) check(!JSON.parse(line).error, 'DOCKER_FAILED', 503); resolve(null) }
              catch { reject(new OperationsError('DOCKER_FAILED', 503)) }
            } else reject(new OperationsError('DOCKER_FAILED', 503))
          }
        })
      })
      request.setTimeout(timeout, () => request.destroy())
      request.on('error', () => reject(new OperationsError('DOCKER_FAILED', 503)))
      if (stream) { stream.on('error', () => request.destroy()); stream.pipe(request) } else request.end(body)
    })
  }
  inspectContainer(id) { check(/^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$/.test(id), 'INVALID_CONTAINER'); return this.request('GET', `/containers/${encodeURIComponent(id)}/json`) }
  inspectImage(id) { check(/^sha256:[a-f0-9]{64}$/.test(id) || /^myapi(?:-smoke-fixture)?:[A-Za-z0-9_.-]+$/.test(id), 'INVALID_IMAGE'); return this.request('GET', `/images/${encodeURIComponent(id)}/json`) }
  disableRestart(id) { check(/^[a-f0-9]{64}$/.test(id), 'INVALID_CONTAINER'); return this.request('POST', `/containers/${id}/update`, { json: { RestartPolicy: { Name: 'no', MaximumRetryCount: 0 } } }) }
  stop(id) { check(/^[a-f0-9]{64}$/.test(id), 'INVALID_CONTAINER'); return this.request('POST', `/containers/${id}/stop?t=30`, { timeout: 45_000 }) }
  start(id) { check(/^[a-f0-9]{64}$/.test(id), 'INVALID_CONTAINER'); return this.request('POST', `/containers/${id}/start`) }
  create(name, config) { check(/^myapi-operations-[a-z0-9-]+$/.test(name), 'INVALID_CONTAINER'); return this.request('POST', `/containers/create?name=${name}`, { json: config }) }
  async load(filename) { return this.request('POST', '/images/load?quiet=1&platform=linux%2Famd64', { stream: createReadStream(filename), timeout: 15 * 60_000, max: 8 * 1024 * 1024 }) }
}

export function configuredNetworks(info) {
  const networks = {}
  for (const [name, endpoint] of Object.entries(info.NetworkSettings?.Networks || {})) {
    networks[name] = Object.fromEntries(['IPAMConfig', 'Links', 'Aliases', 'DriverOpts', 'GwPriority'].filter(k => endpoint[k] != null).map(k => [k, structuredClone(endpoint[k])]))
  }
  return networks
}
export function configuredMounts(info) { return structuredClone(info.Mounts || []).sort((a, b) => a.Destination.localeCompare(b.Destination)) }
export function containerDigest(info) { return digest({ id: info.Id, image: info.Image, Config: info.Config, HostConfig: info.HostConfig, Mounts: configuredMounts(info), networks: configuredNetworks(info) }) }
export async function validateContainer(info, instance, docker) {
  check(info && /^[a-f0-9]{64}$/.test(info.Id) && /^sha256:[a-f0-9]{64}$/.test(info.Image) && info.State?.Running === true, 'SOURCE_NOT_RUNNING', 409)
  check(info.HostConfig && info.Config && !info.HostConfig.AutoRemove && !info.HostConfig.VolumesFrom?.length && !info.HostConfig.Privileged &&
    !info.HostConfig.PidMode?.startsWith('container:') && !info.HostConfig.NetworkMode?.startsWith('container:') && !info.HostConfig.IpcMode?.startsWith('container:') &&
    !Object.values(info.NetworkSettings?.Networks || {}).some(endpoint => endpoint.IPAMConfig?.IPv4Address || endpoint.IPAMConfig?.IPv6Address), 'UNSUPPORTED_CONTAINER')
  check(info.HostConfig.PublishAllPorts !== true && Object.values(info.HostConfig.PortBindings || {}).every(bindings => Array.isArray(bindings) &&
    bindings.every(binding => typeof binding.HostPort === 'string' && /^[1-9][0-9]{0,4}$/.test(binding.HostPort) && Number(binding.HostPort) <= 65535)), 'DYNAMIC_PORT_UNSUPPORTED')
  let health
  try { health = new URL(instance.health_url) } catch { check(false, 'HEALTH_BINDING_MISMATCH') }
  check(health.protocol === 'http:' && ['localhost', '127.0.0.1', '[::1]'].includes(health.hostname) &&
    /^[1-9][0-9]{0,4}$/.test(health.port) && Number(health.port) <= 65535 && !health.username && !health.password &&
    health.pathname === '/' && !health.search && !health.hash && info.HostConfig.NetworkMode !== 'host', 'HEALTH_BINDING_MISMATCH')
  const hosts = health.hostname === '[::1]' ? ['::1', '::'] : ['127.0.0.1', '0.0.0.0', '::']
  check(Object.entries(info.NetworkSettings?.Ports || {}).some(([containerPort, bindings]) => containerPort.endsWith('/tcp') &&
    Array.isArray(bindings) && bindings.some(binding => binding.HostPort === health.port && hosts.includes(binding.HostIp))), 'HEALTH_BINDING_MISMATCH')
  for (const mount of info.Mounts || []) {
    if (mount.Type !== 'volume') continue
    const viaBind = (info.HostConfig.Binds || []).some(bind => { const parts = bind.split(':'); return parts[0] === mount.Name && parts[1] === mount.Destination })
    const viaMount = (info.HostConfig.Mounts || []).some(configured => configured.Type === 'volume' && configured.Source === mount.Name && configured.Target === mount.Destination)
    check(typeof mount.Name === 'string' && mount.Name.length > 0 && (viaBind || viaMount), 'ANONYMOUS_VOLUME_UNSUPPORTED')
  }
  check(Array.isArray(info.Config.Env) && new Set(info.Config.Env.map(v => v.split('=')[0])).size === info.Config.Env.length, 'INVALID_ENV')
  const env = Object.fromEntries(info.Config.Env.map(value => { const i = value.indexOf('='); return [value.slice(0, i), value.slice(i + 1)] }))
  check(!env.SQL_DSN?.trim() && !env.LOG_SQL_DSN?.trim() && (!env.MYAPI_EDITION || env.MYAPI_EDITION.trim().toLowerCase() === 'full') &&
    !['1', 'true'].includes(env.MYAPI_PERSONAL_MODE?.trim().toLowerCase()), 'UNSUPPORTED_DATABASE_OR_EDITION')
  const image = await docker.inspectImage(info.Image)
  check(image.Id === info.Image && image.Os === 'linux' && image.Architecture === 'amd64' && !image.Variant && image.Config?.Labels?.['io.myapi.edition'] === 'full', 'UNSUPPORTED_IMAGE')
  check(Array.isArray(info.Mounts) && info.Mounts.filter(m => m.Source === instance.data_dir && m.Destination === '/data' && m.Type === 'bind' && m.RW === true).length === 1 &&
    !info.Mounts.some(m => m.Destination.startsWith('/data/')), 'DATA_MOUNT_MISMATCH')
  const configuredSQLite = env.SQLITE_PATH?.trim()
  if (configuredSQLite) {
    check(!/[#%\\\0]/.test(configuredSQLite) && !configuredSQLite.startsWith('file:') && configuredSQLite.split('?').length <= 2, 'SQLITE_PATH_MISMATCH')
    const query = new URLSearchParams(configuredSQLite.split('?')[1] || '')
    check([...query].length <= 1 && [...query].every(([key, value]) => key === '_busy_timeout' && /^[1-9][0-9]{0,5}$/.test(value) && Number(value) <= 300000), 'UNSAFE_SQLITE_QUERY')
  }
  let sqlite = configuredSQLite?.split('?')[0]
  if (!sqlite) {
    check(info.Config.WorkingDir === '/data', 'SQLITE_PATH_MISMATCH')
    const canonical = path.join(instance.data_dir, 'my-api.db')
    let exists = true
    try { await lstat(canonical) } catch (error) { if (error.code === 'ENOENT') exists = false; else throw error }
    sqlite = exists ? '/data/my-api.db' : '/data/one-api.db'
  }
  check(sqlite.startsWith('/data/') || ['my-api.db', 'one-api.db'].includes(sqlite) && info.Config.WorkingDir === '/data', 'SQLITE_PATH_MISMATCH')
  const resolved = path.resolve(instance.data_dir, sqlite.startsWith('/data/') ? sqlite.slice(6) : sqlite)
  check(resolved === instance.sqlite_file, 'SQLITE_PATH_MISMATCH')
  return { info: structuredClone(info), digest: containerDigest(info) }
}

export function candidateConfig(info, imageId, targetImage) {
  check(targetImage?.Id === imageId && targetImage.Os === 'linux' && targetImage.Architecture === 'amd64' && !targetImage.Variant && targetImage.Config, 'IMAGE_ID_MISMATCH')
  const defaults = targetImage.Config
  // Docker merges these defaults even with an explicit container Config. Only
  // Labels have a deliberate merged expectation; new persistent resources,
  // environment keys or exposed ports are unsupported before the source stops.
  check(Object.keys(defaults.Volumes || {}).every(destination => Object.hasOwn(info.Config.Volumes || {}, destination)), 'TARGET_IMAGE_VOLUMES_UNSUPPORTED')
  const environmentKeys = new Set((info.Config.Env || []).map(value => value.split('=')[0]))
  check((defaults.Env || []).every(value => typeof value === 'string' && value.includes('=') && environmentKeys.has(value.split('=')[0])), 'TARGET_IMAGE_ENV_UNSUPPORTED')
  check(Object.keys(defaults.ExposedPorts || {}).every(port => Object.hasOwn(info.Config.ExposedPorts || {}, port)), 'TARGET_IMAGE_PORTS_UNSUPPORTED')
  const labels = { ...structuredClone(defaults.Labels || {}), ...structuredClone(info.Config.Labels || {}) }
  check(Object.values(labels).every(value => typeof value === 'string'), 'TARGET_IMAGE_LABELS_UNSUPPORTED')
  const result = { ...structuredClone(info.Config), Image: imageId, Labels: labels, HostConfig: structuredClone(info.HostConfig) }
  // Docker's inspect NetworkSettings contains runtime IDs. Only writable endpoint
  // configuration is copied; endpoint IDs/IP leases belong to the old container.
  const endpoints = configuredNetworks(info)
  if (Object.keys(endpoints).length) result.NetworkingConfig = { EndpointsConfig: endpoints }
  return result
}
export function sameRuntimeConfig(source, target, desiredLabels) {
  const a = structuredClone(source.Config), b = structuredClone(target.Config)
  if (desiredLabels !== undefined) a.Labels = structuredClone(desiredLabels)
  delete a.Image; delete b.Image
  // Only the exact image and explicitly computed merged Labels may change.
  return digest(a) === digest(b) && digest(source.HostConfig) === digest(target.HostConfig) && digest(configuredMounts(source)) === digest(configuredMounts(target))
}

export function samePublishedPorts(source, target) {
  const normalize = info => Object.fromEntries(Object.entries(info.NetworkSettings?.Ports || {}).map(([port, bindings]) =>
    [port, bindings === null ? null : bindings.map(binding => ({ HostIp: binding.HostIp, HostPort: binding.HostPort })).sort((a, b) => (a.HostIp + ':' + a.HostPort).localeCompare(b.HostIp + ':' + b.HostPort))]))
  return digest(normalize(source)) === digest(normalize(target))
}
