import { lstat } from 'node:fs/promises'
import path from 'node:path'
import { absolute, check, exactKeys, noSymlinks, privateDirectory, protectedRead } from './safety.mjs'

export async function validateConfig(config, { uid = process.getuid() } = {}) {
  exactKeys(config, ['listen', 'unix_socket', 'bridge_token_file', 'github_token_file', 'docker_socket', 'state_dir', 'backups_dir', 'instances'], 'INVALID_CONFIG')
  exactKeys(config.listen, ['host', 'port', 'tls_cert_file', 'tls_key_file'], 'INVALID_CONFIG')
  check(typeof config.listen.host === 'string' && /^(127\.0\.0\.1|0\.0\.0\.0|::1|::)$/.test(config.listen.host) &&
    Number.isInteger(config.listen.port) && config.listen.port >= 1024 && config.listen.port <= 65535, 'INVALID_LISTENER')
  check(config.docker_socket === '/var/run/docker.sock', 'INVALID_DOCKER_SOCKET')
  check([config.unix_socket, config.state_dir, config.backups_dir].every(absolute) && config.state_dir !== config.backups_dir &&
    !config.state_dir.startsWith(config.backups_dir + path.sep) && !config.backups_dir.startsWith(config.state_dir + path.sep), 'INVALID_CONFIG')
  await privateDirectory(config.state_dir, uid)
  await privateDirectory(config.backups_dir, uid)
  await privateDirectory(path.dirname(config.unix_socket), uid)
  check(Array.isArray(config.instances) && config.instances.length > 0 && config.instances.length <= 32, 'INVALID_INSTANCES')
  const ids = new Set(), containers = new Set(), data = new Set()
  for (const instance of config.instances) {
    exactKeys(instance, ['id', 'container', 'data_dir', 'sqlite_file', 'health_url'], 'INVALID_INSTANCE')
    check(typeof instance.id === 'string' && /^[a-z0-9][a-z0-9-]{0,47}$/.test(instance.id) && !ids.has(instance.id) &&
      typeof instance.container === 'string' && /^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$/.test(instance.container) && !containers.has(instance.container) &&
      absolute(instance.data_dir) && absolute(instance.sqlite_file) && instance.sqlite_file.startsWith(instance.data_dir + path.sep) && ![...data].some(directory => directory === instance.data_dir || directory.startsWith(instance.data_dir + path.sep) || instance.data_dir.startsWith(directory + path.sep)), 'INVALID_INSTANCE')
    await noSymlinks(instance.data_dir)
    await noSymlinks(instance.sqlite_file)
    const st = await lstat(instance.sqlite_file)
    check(st.isFile() && st.nlink === 1, 'UNSAFE_SQLITE')
    for (const directory of [config.state_dir, config.backups_dir, path.dirname(config.unix_socket)]) {
      check(directory !== instance.data_dir && !directory.startsWith(instance.data_dir + path.sep) && !instance.data_dir.startsWith(directory + path.sep), 'DATA_OVERLAP')
    }
    let url
    try { url = new URL(instance.health_url) } catch { check(false, 'INVALID_HEALTH_URL') }
    check(url.protocol === 'http:' && ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname) &&
      /^[1-9][0-9]{0,4}$/.test(url.port) && Number(url.port) <= 65535 &&
      !url.username && !url.password && url.pathname === '/' && !url.search && !url.hash, 'INVALID_HEALTH_URL')
    ids.add(instance.id); containers.add(instance.container); data.add(instance.data_dir)
  }
  return structuredClone(config)
}

export async function loadConfig(filename) {
  check(Number(process.versions.node.split('.')[0]) >= 22, 'NODE_22_REQUIRED')
  check(process.platform === 'linux' && process.arch === 'x64' && process.getuid() === 0, 'ROOT_LINUX_AMD64_REQUIRED')
  let config
  try { config = JSON.parse(await protectedRead(filename)) } catch { check(false, 'INVALID_CONFIG') }
  await validateConfig(config, { uid: 0 })
  for (const file of [config.bridge_token_file, config.github_token_file, config.listen.tls_cert_file, config.listen.tls_key_file]) await protectedRead(file)
  return config
}
