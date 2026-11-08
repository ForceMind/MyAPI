// Contract tests for inspected metadata. Real Docker qualification is separate.
import assert from 'node:assert/strict'
import test from 'node:test'
import { containerDigest, validateBackupContainer, validateContainer } from './docker.mjs'

const sourceId = '1'.repeat(64)
const imageId = 'sha256:' + '2'.repeat(64)
const secret = 'synthetic-backup-source-secret'

function fixture({ edition = 'full', label = 'full' } = {}) {
  const instance = { id: 'primary', container: 'myapi-backup-contract', data_dir: '/synthetic/my-api/data',
    sqlite_file: '/synthetic/my-api/data/my-api.db', health_url: 'http://127.0.0.1:18080/' }
  const info = {
    Id: sourceId, Image: imageId,
    Config: { WorkingDir: '/data', Env: [
      ...(edition === null ? [] : [`MYAPI_EDITION=${edition}`]),
      'SQLITE_PATH=/data/my-api.db?_busy_timeout=30000', `SESSION_SECRET=${secret}`,
    ] },
    HostConfig: { RestartPolicy: { Name: 'always', MaximumRetryCount: 0 }, NetworkMode: 'bridge',
      PortBindings: { '3000/tcp': [{ HostIp: '127.0.0.1', HostPort: '18080' }] } },
    Mounts: [{ Type: 'bind', Source: instance.data_dir, Destination: '/data', RW: true }],
    State: { Running: true, Pid: 1234 },
    NetworkSettings: { Networks: { 'myapi-backup-contract': {} },
      Ports: { '3000/tcp': [{ HostIp: '127.0.0.1', HostPort: '18080' }] } },
  }
  const image = { Id: imageId, Os: 'linux', Architecture: 'amd64', Config: { Labels: label === null ? {} : { 'io.myapi.edition': label } } }
  let imageReads = 0
  const docker = { async inspectImage(id) { assert.equal(id, imageId); imageReads++; return structuredClone(image) } }
  return { info, image, instance, docker, imageReads: () => imageReads }
}

test('Full source acceptance and exact returned identity stay unchanged for the upgrade and backup entries', async () => {
  for (const edition of [null, '', 'full', ' FULL ']) {
    const f = fixture({ edition }), before = structuredClone(f.info)
    const legacy = await validateContainer(f.info, f.instance, f.docker)
    const backup = await validateBackupContainer(f.info, f.instance, f.docker)
    assert.deepEqual(backup, legacy)
    assert.deepEqual(Object.keys(backup).sort(), ['digest', 'info'])
    assert.deepEqual(backup.info, before)
    assert.equal(backup.digest, containerDigest(before))
    assert.deepEqual(f.info, before)
    assert.notEqual(backup.info, f.info)
  }
})

test('Lite and lan aliases match only the Lite image family while the legacy upgrade entry stays Full-only', async () => {
  for (const edition of ['lite', 'lan', ' LiTe ', ' LAN ']) {
    for (const label of ['lite', 'lan']) {
      const f = fixture({ edition, label }), before = structuredClone(f.info)
      const result = await validateBackupContainer(f.info, f.instance, f.docker)
      assert.deepEqual(result.info, before, 'real inspected Env must not be rewritten to Full or another alias')
      assert.equal(result.digest, containerDigest(before))
      assert.equal(result.info.Config.Env.includes(`SESSION_SECRET=${secret}`), true)
      assert.equal(f.imageReads(), 1)
      await assert.rejects(validateContainer(f.info, f.instance, f.docker), { code: 'UNSUPPORTED_DATABASE_OR_EDITION' })
      assert.equal(f.imageReads(), 1, 'the old entry must reject Lite before consulting an image')
      assert.deepEqual(f.info, before)
    }
  }
})

test('missing, unknown and cross-family edition metadata cannot qualify a backup', async () => {
  const cases = [
    { edition: 'full', label: 'lite', code: 'UNSUPPORTED_IMAGE' },
    { edition: 'full', label: 'lan', code: 'UNSUPPORTED_IMAGE' },
    { edition: 'lite', label: 'full', code: 'UNSUPPORTED_IMAGE' },
    { edition: 'lan', label: 'full', code: 'UNSUPPORTED_IMAGE' },
    { edition: null, label: 'lite', code: 'UNSUPPORTED_IMAGE' },
    { edition: '', label: 'lan', code: 'UNSUPPORTED_IMAGE' },
    { edition: 'unknown', label: 'lite', code: 'UNSUPPORTED_DATABASE_OR_EDITION' },
    { edition: ' ', label: 'full', code: 'UNSUPPORTED_DATABASE_OR_EDITION' },
    { edition: 'lite', label: null, code: 'UNSUPPORTED_IMAGE' },
    { edition: 'lite', label: '', code: 'UNSUPPORTED_IMAGE' },
    { edition: 'lite', label: 'unknown', code: 'UNSUPPORTED_IMAGE' },
    { edition: 'lite', label: 'LITE', code: 'UNSUPPORTED_IMAGE' },
    { edition: 'lite', label: ' lite ', code: 'UNSUPPORTED_IMAGE' },
    { edition: 'full', label: 'FULL', code: 'UNSUPPORTED_IMAGE' },
  ]
  for (const row of cases) {
    const f = fixture(row), before = structuredClone(f.info)
    await assert.rejects(validateBackupContainer(f.info, f.instance, f.docker), { code: row.code })
    assert.deepEqual(f.info, before)
    if (row.edition === 'full' || row.edition === ' ') {
      await assert.rejects(validateContainer(f.info, f.instance, f.docker), { code: row.code })
    }
  }
})

test('remote databases and personal mode retain their rejection for Full and Lite backups', async () => {
  for (const edition of ['full', 'lite']) {
    for (const entry of ['SQL_DSN=postgres://synthetic.invalid/database', 'LOG_SQL_DSN=mysql://synthetic.invalid/log', 'MYAPI_PERSONAL_MODE=true', 'MYAPI_PERSONAL_MODE=1']) {
      const f = fixture({ edition, label: edition })
      f.info.Config.Env.push(entry)
      await assert.rejects(validateBackupContainer(f.info, f.instance, f.docker), { code: 'UNSUPPORTED_DATABASE_OR_EDITION' })
      if (edition === 'full') await assert.rejects(validateContainer(f.info, f.instance, f.docker), { code: 'UNSUPPORTED_DATABASE_OR_EDITION' })
      assert.equal(f.imageReads(), 0)
    }
  }
})

test('backup and Full upgrade entries retain source, mount, SQLite, bind and image safety boundaries', async () => {
  const cases = [
    { name: 'not running', code: 'SOURCE_NOT_RUNNING', change: f => { f.info.State.Running = false } },
    { name: 'mutable image reference', code: 'SOURCE_NOT_RUNNING', change: f => { f.info.Image = 'myapi:latest' } },
    { name: 'privileged container', code: 'UNSUPPORTED_CONTAINER', change: f => { f.info.HostConfig.Privileged = true } },
    { name: 'automatic removal', code: 'UNSUPPORTED_CONTAINER', change: f => { f.info.HostConfig.AutoRemove = true } },
    { name: 'shared container PID namespace', code: 'UNSUPPORTED_CONTAINER', change: f => { f.info.HostConfig.PidMode = 'container:' + sourceId } },
    { name: 'static network address', code: 'UNSUPPORTED_CONTAINER', change: f => { f.info.NetworkSettings.Networks['myapi-backup-contract'].IPAMConfig = { IPv4Address: '172.20.0.2' } } },
    { name: 'dynamic published port', code: 'DYNAMIC_PORT_UNSUPPORTED', change: f => { f.info.HostConfig.PortBindings['3000/tcp'][0].HostPort = '0' } },
    { name: 'unbound health port', code: 'HEALTH_BINDING_MISMATCH', change: f => { f.instance.health_url = 'http://127.0.0.1:18081/' } },
    { name: 'external health target', code: 'HEALTH_BINDING_MISMATCH', change: f => { f.instance.health_url = 'http://192.168.1.2:18080/' } },
    { name: 'host network', code: 'HEALTH_BINDING_MISMATCH', change: f => { f.info.HostConfig.NetworkMode = 'host' } },
    { name: 'duplicate Env name', code: 'INVALID_ENV', change: f => { f.info.Config.Env.push('MYAPI_EDITION=full') } },
    { name: 'anonymous volume', code: 'ANONYMOUS_VOLUME_UNSUPPORTED', change: f => { f.info.Mounts.push({ Type: 'volume', Name: '', Source: '/synthetic/unknown-volume', Destination: '/extra', RW: true }) } },
    { name: 'readonly data', code: 'DATA_MOUNT_MISMATCH', change: f => { f.info.Mounts[0].RW = false } },
    { name: 'different data directory', code: 'DATA_MOUNT_MISMATCH', change: f => { f.info.Mounts[0].Source = '/synthetic/other-data' } },
    { name: 'nested data mount', code: 'DATA_MOUNT_MISMATCH', change: f => { f.info.Mounts.push({ Type: 'bind', Source: '/synthetic/other', Destination: '/data/nested', RW: true }) } },
    { name: 'unregistered SQLite path', code: 'SQLITE_PATH_MISMATCH', change: f => { f.info.Config.Env[1] = 'SQLITE_PATH=/data/another.db' } },
    { name: 'SQLite URI', code: 'SQLITE_PATH_MISMATCH', change: f => { f.info.Config.Env[1] = 'SQLITE_PATH=file:/data/my-api.db' } },
    { name: 'unsupported SQLite option', code: 'UNSAFE_SQLITE_QUERY', change: f => { f.info.Config.Env[1] = 'SQLITE_PATH=/data/my-api.db?journal_mode=WAL' } },
    { name: 'wrong working directory for relative SQLite', code: 'SQLITE_PATH_MISMATCH', change: f => { f.info.Config.Env[1] = 'SQLITE_PATH=my-api.db'; f.info.Config.WorkingDir = '/app' } },
    { name: 'different immutable image', code: 'UNSUPPORTED_IMAGE', change: f => { f.image.Id = 'sha256:' + '3'.repeat(64) } },
    { name: 'different image platform', code: 'UNSUPPORTED_IMAGE', change: f => { f.image.Architecture = 'arm64' } },
    { name: 'different image OS', code: 'UNSUPPORTED_IMAGE', change: f => { f.image.Os = 'windows' } },
    { name: 'image variant', code: 'UNSUPPORTED_IMAGE', change: f => { f.image.Variant = 'v8' } },
  ]
  for (const edition of ['full', 'lite']) {
    for (const row of cases) {
      const f = fixture({ edition, label: edition })
      row.change(f)
      const before = structuredClone(f.info)
      await assert.rejects(validateBackupContainer(f.info, f.instance, f.docker), { code: row.code }, `${edition}: ${row.name}`)
      if (edition === 'full') await assert.rejects(validateContainer(f.info, f.instance, f.docker), { code: row.code }, `legacy Full: ${row.name}`)
      assert.deepEqual(f.info, before)
    }
  }
})

test('inspection identity preserves secrets and raw aliases without accepting a caller-provided bypass', async () => {
  const f = fixture({ edition: 'lan', label: 'lite' })
  const result = await validateBackupContainer(f.info, f.instance, f.docker)
  result.info.Config.Env[2] = 'SESSION_SECRET=changed-only-in-returned-copy'
  assert.equal(f.info.Config.Env[2], `SESSION_SECRET=${secret}`)
  f.info.Config.Env[2] = 'SESSION_SECRET=another-synthetic-secret'
  assert.notEqual((await validateBackupContainer(f.info, f.instance, f.docker)).digest, result.digest)
  const bypass = { verified: true, allowedEditions: ['full', 'lite', 'lan'] }
  await assert.rejects(validateContainer(f.info, f.instance, f.docker, bypass), { code: 'UNSUPPORTED_DATABASE_OR_EDITION' })
  f.image.Config.Labels['io.myapi.edition'] = 'full'
  await assert.rejects(validateBackupContainer(f.info, f.instance, f.docker, bypass), { code: 'UNSUPPORTED_IMAGE' })
})
