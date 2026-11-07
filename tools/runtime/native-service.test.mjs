import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { chmodSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const repository = fileURLToPath(new URL('../../', import.meta.url))
const script = path.join(repository, 'deploy/install-native.sh')
const secret = 'SYNTHETIC_NATIVE_CONFIG_SECRET_DO_NOT_PRINT'
const xdgKeys = ['XDG_DATA_HOME', 'XDG_CONFIG_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME']

function snapshotTree(directory, relative = '') {
  const entries = []
  for (const name of readdirSync(path.join(directory, relative)).sort()) {
    const entry = path.join(relative, name)
    const absolute = path.join(directory, entry)
    const stat = lstatSync(absolute)
    assert.equal(stat.isSymbolicLink(), false, 'the synthetic fixture contains no symbolic links')
    entries.push({ path: entry, mode: stat.mode & 0o777, type: stat.isDirectory() ? 'directory' : 'file',
      digest: stat.isDirectory() ? null : createHash('sha256').update(readFileSync(absolute)).digest('hex') })
    if (stat.isDirectory()) {
      entries.push(...snapshotTree(directory, entry))
    }
  }
  return entries
}

function fixture(t) {
  const root = mkdtempSync(path.join(os.tmpdir(), 'myapi-native-contract-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const home = path.join(root, 'home')
  const guards = path.join(root, 'command-guards')
  const commandLog = path.join(root, 'forbidden-command-attempts')
  mkdirSync(guards)
  writeFileSync(commandLog, '')
  for (const command of ['systemctl', 'sudo', 'curl', 'wget', 'cp', 'mkdir', 'mv', 'rm', 'install', 'ln', 'chmod', 'chown', 'tee', 'touch', 'tar', 'unzip', 'dd']) {
    const guard = path.join(guards, command)
    writeFileSync(guard, `#!/bin/sh\nprintf '%s\\n' '${command}' >> "$NATIVE_GUARD_LOG"\nexit 97\n`)
    chmodSync(guard, 0o755)
  }
  for (const [relative, bytes] of [
    ['.config/my-api/my-api.env', `SQLITE_PATH=existing-private.db\nSESSION_SECRET=${secret}\n`],
    ['.local/share/my-api/data/my-api.db', Buffer.from(`SQLite format 3\0synthetic-accounting-bytes:${secret}`)],
    ['.local/state/my-api/state/install-journal.json', '{"schema":1,"phase":"existing"}\n'],
    ['.local/state/my-api/backups/old.sqlite', Buffer.from([0, 17, 255, 31])],
    ['.local/state/my-api/logs/existing.log', 'synthetic-existing-log\n'],
    ['.cache/my-api/existing-cache', 'synthetic-existing-cache\n'],
  ]) {
    const target = path.join(home, relative)
    mkdirSync(path.dirname(target), { recursive: true })
    writeFileSync(target, bytes, { mode: 0o600 })
  }
  const before = snapshotTree(root)
  const environment = { ...process.env, HOME: home, PATH: `${guards}${path.delimiter}${process.env.PATH || '/usr/bin:/bin'}`, NATIVE_GUARD_LOG: commandLog }
  for (const key of xdgKeys) {
    delete environment[key]
  }
  return {
    root, home, commandLog, environment,
    run(args, overrides = {}) {
      const env = { ...environment }
      for (const [key, value] of Object.entries(overrides)) {
        if (value === undefined) {
          delete env[key]
        } else {
          env[key] = value
        }
      }
      const result = spawnSync('bash', [script, ...args], { cwd: root, env, encoding: 'utf8', timeout: 10_000, maxBuffer: 1024 * 1024 })
      assert.equal(result.error, undefined)
      assert.equal(result.signal, null)
      assert.equal(readFileSync(commandLog, 'utf8'), '', 'preview and rejected commands must not invoke mutating tools')
      assert.deepEqual(snapshotTree(root), before, 'configuration, accounting, backups, logs and directory contents must remain unchanged')
      assert.equal(`${result.stdout}${result.stderr}`.includes(secret), false, 'preview must not disclose configuration or accounting contents')
      return result
    },
  }
}

function parsePlan(output) {
  const fields = {}
  for (const line of output.split('\n')) {
    const match = /^(scope|account|binary|releases|config|data|state|backups|logs|cache|unit): (.+)$/.exec(line)
    if (match) {
      assert.equal(Object.hasOwn(fields, match[1]), false, `duplicate plan field: ${match[1]}`)
      fields[match[1]] = match[2]
    }
  }
  assert.match(output, /preview/i)
  assert.match(output, /预览/)
  assert.match(output, /not (?:been )?(?:inspected|checked)/i)
  assert.match(output, /未检查/)
  assert.match(output, /installation unavailable|(?:does not|will not|not) install/i)
  assert.match(output, /安装不可用|不(?:会)?安装/)
  return fields
}

function userPaths(home, overrides = {}) {
  const data = overrides.XDG_DATA_HOME ?? path.join(home, '.local/share')
  const config = overrides.XDG_CONFIG_HOME ?? path.join(home, '.config')
  const state = overrides.XDG_STATE_HOME ?? path.join(home, '.local/state')
  const cache = overrides.XDG_CACHE_HOME ?? path.join(home, '.cache')
  return {
    scope: 'user', account: 'current-user',
    binary: path.join(data, 'my-api/current/my-api'), releases: path.join(data, 'my-api/releases'),
    config: path.join(config, 'my-api/my-api.env'), unit: path.join(config, 'systemd/user/my-api.service'),
    data: path.join(data, 'my-api/data'), state: path.join(state, 'my-api/state'),
    backups: path.join(state, 'my-api/backups'), logs: path.join(state, 'my-api/logs'), cache: path.join(cache, 'my-api'),
  }
}

const systemPaths = {
  scope: 'system', account: 'my-api', binary: '/opt/my-api/current/my-api', releases: '/opt/my-api/releases',
  config: '/etc/my-api/my-api.env', unit: '/etc/systemd/system/my-api.service', data: '/var/lib/my-api/data',
  state: '/var/lib/my-api/state', backups: '/var/lib/my-api/backups', logs: '/var/log/my-api', cache: '/var/cache/my-api',
}

// Decode the emitted systemd words rather than treating a quoted path as a
// shell expression. Only the escapes used by these fixed templates are needed.
function unitWords(value, exec = false) {
  const words = []
  let word = ''
  let quote = null
  let started = false
  for (let index = 0; index < value.length; index++) {
    const character = value[index]
    if (!quote && /\s/.test(character)) {
      if (started) {
        words.push(word)
        word = ''
        started = false
      }
      continue
    }
    started = true
    if (character === '"' || character === "'") {
      if (quote === character) {
        quote = null
      } else if (quote === null) {
        quote = character
      } else {
        word += character
      }
    } else if (character === '\\') {
      const escaped = value[++index]
      assert.ok(escaped === '\\' || escaped === '"' || escaped === "'", 'path escaping must be a valid systemd quoted-word escape')
      word += escaped
    } else if (character === '%') {
      assert.equal(value[++index], '%', 'literal percent must not become a systemd specifier')
      word += '%'
    } else if (exec && character === '$') {
      assert.equal(value[++index], '$', 'literal dollar in ExecStart must not expand an environment variable')
      word += '$'
    } else {
      word += character
    }
  }
  assert.equal(quote, null, 'systemd words must have balanced quotes')
  if (started) {
    words.push(word)
  }
  return words
}

function unitDirectives(output) {
  const directives = new Map()
  for (const line of output.split('\n')) {
    if (!line || /^\s*[#;]/.test(line) || /^\[.+\]$/.test(line)) {
      continue
    }
    const match = /^([A-Za-z][A-Za-z0-9]*)=(.*)$/.exec(line)
    assert.ok(match, 'render-unit must emit only unit syntax on stdout')
    const values = directives.get(match[1]) || []
    values.push(match[2])
    directives.set(match[1], values)
  }
  return directives
}

function assertRenderedUnit(output, expected) {
  const directives = unitDirectives(output)
  const one = key => {
    const values = directives.get(key)
    assert.ok(values, `missing unit directive: ${key}`)
    assert.equal(values.length, 1, `ambiguous unit directive: ${key}`)
    return values[0]
  }
  const command = unitWords(one('ExecStart'), true)
  if (expected.scope === 'system') {
    assert.deepEqual(command, [expected.binary, '--log-dir', expected.logs])
  } else {
    assert.deepEqual(command, ['/bin/sh', '-c', 'exec "$1" --log-dir "$2"', 'my-api', expected.binary, expected.logs])
  }
  assert.deepEqual(unitWords(one('WorkingDirectory')), [expected.data])
  assert.deepEqual(unitWords(one('EnvironmentFile')), [expected.config], 'configuration is required, never silently optional')
  const environment = (directives.get('Environment') || []).flatMap(value => unitWords(value))
  assert.deepEqual(environment, [`SQLITE_PATH=${expected.data}/my-api.db?_busy_timeout=30000`], 'only the SQLite default belongs in the unit; secrets and feature configuration stay in the required configuration file')
  assert.equal(one('Restart'), 'on-failure')
  assert.equal(one('TimeoutStopSec'), 'infinity', 'an unconfirmed stop must not be treated as a completed stop after an arbitrary deadline')
  assert.equal(one('NoNewPrivileges'), 'true')
  assert.equal(one('UMask'), '0077')
  if (expected.scope === 'system') {
    assert.equal(one('User'), 'my-api')
    assert.equal(one('ProtectSystem'), 'strict')
    assert.equal(one('ProtectHome'), 'true')
    const writable = (directives.get('ReadWritePaths') || []).flatMap(value => unitWords(value))
    assert.deepEqual(writable.sort(), [expected.data, expected.logs, expected.cache].sort())
    assert.equal(one('WantedBy'), 'multi-user.target')
    assert.ok(unitWords(one('After')).includes('network.target'))
  } else {
    assert.equal(directives.has('User'), false, 'the user manager uses the invoking account')
    assert.equal(one('WantedBy'), 'default.target')
    for (const key of ['PrivateTmp', 'PrivateDevices', 'PrivateUsers', 'ProtectSystem', 'ProtectHome', 'ProtectKernelTunables', 'ProtectKernelModules', 'ProtectControlGroups', 'ReadWritePaths', 'ReadOnlyPaths', 'InaccessiblePaths']) {
      assert.equal(directives.has(key), false, `user scope must not depend on filesystem namespace setup: ${key}`)
    }
    assert.equal((directives.get('After') || []).some(value => unitWords(value).includes('network.target')), false)
  }
}

test('native help and fixed system preview are read-only and do not depend on HOME', t => {
  const local = fixture(t)
  const help = local.run(['--help'])
  assert.equal(help.status, 0)
  assert.match(`${help.stdout}${help.stderr}`, /render-unit/)
  assert.match(`${help.stdout}${help.stderr}`, /--scope/)
  const overrides = { HOME: undefined, XDG_DATA_HOME: 'ignored-relative', XDG_CONFIG_HOME: '' }
  const plan = local.run(['plan', '--scope', 'system'], overrides)
  assert.equal(plan.status, 0, plan.stderr)
  assert.deepEqual(parsePlan(plan.stdout), systemPaths)
  const unit = local.run(['render-unit', '--scope', 'system'], overrides)
  assert.equal(unit.status, 0, unit.stderr)
  assertRenderedUnit(unit.stdout, systemPaths)
})

test('default user plan and rendered unit agree with XDG data and separate state locations', t => {
  const local = fixture(t)
  const expected = userPaths(local.home)
  const plan = local.run(['plan', '--scope', 'user'])
  assert.equal(plan.status, 0, plan.stderr)
  assert.deepEqual(parsePlan(plan.stdout), expected)
  const unit = local.run(['render-unit', '--scope', 'user'])
  assert.equal(unit.status, 0, unit.stderr)
  assertRenderedUnit(unit.stdout, expected)
})

test('custom XDG metacharacters remain literal in plans and correctly escaped systemd values', t => {
  const local = fixture(t)
  const overrides = {
    XDG_DATA_HOME: path.join(local.root, '数据=data space "quote" \\ %n $HOME $(touch NATIVE_PWNED)'),
    XDG_CONFIG_HOME: path.join(local.root, "config 'single' %h `touch NATIVE_BACKTICK`"),
    XDG_STATE_HOME: path.join(local.root, 'state space \\ "quote" %i ${HOME}'),
    XDG_CACHE_HOME: path.join(local.root, 'cache space %t $HOME'),
  }
  const expected = userPaths(local.home, overrides)
  const plan = local.run(['plan', '--scope', 'user'], overrides)
  assert.equal(plan.status, 0, plan.stderr)
  assert.deepEqual(parsePlan(plan.stdout), expected)
  const unit = local.run(['render-unit', '--scope', 'user'], overrides)
  assert.equal(unit.status, 0, unit.stderr)
  assertRenderedUnit(unit.stdout, expected)
})

test('missing, duplicate and unknown native arguments fail without writes or service-state claims', t => {
  const local = fixture(t)
  for (const args of [[], ['plan'], ['render-unit'], ['plan', '--scope'], ['plan', '--scope', 'unknown'],
    ['plan', '--scope', 'user', '--scope', 'system'], ['plan', '--scope', 'user', '--unknown'],
    ['render-unit', '--scope', 'system', 'extra'], ['plan', '--scope=user'], ['--help', 'extra'],
    ['install', '--scope', 'system'], ['upgrade', '--scope', 'user'], ['rollback', '--scope', 'system'],
    ['uninstall', '--scope', 'user'], ['stop', '--scope', 'system'], ['restart', '--scope', 'user']]) {
    const result = local.run(args)
    assert.notEqual(result.status, 0, `must reject arguments: ${JSON.stringify(args)}`)
    assert.equal(/^\[Service\]$/m.test(result.stdout), false, 'rejection must not return an apparently usable unit')
    assert.doesNotMatch(`${result.stdout}${result.stderr}`, /(?:successfully stopped|service is stopped|已停止服务|服务已停止)/i)
  }
})

test('user paths reject unset or empty HOME, explicit empty XDG, relative paths, controls and dot segments', t => {
  const local = fixture(t)
  const invalid = [
    { HOME: undefined }, { HOME: '' }, { HOME: 'relative-home' }, { HOME: '/tmp/./home' },
    ...xdgKeys.map(key => ({ [key]: '' })),
    { XDG_DATA_HOME: 'relative-data' }, { XDG_CONFIG_HOME: '/tmp/../config' }, { XDG_STATE_HOME: '/tmp/./state' },
    { XDG_CACHE_HOME: '/tmp/line\nbreak' }, { XDG_DATA_HOME: '/tmp/tab\tpath' },
    { XDG_CONFIG_HOME: '/tmp/carriage\rreturn' }, { XDG_STATE_HOME: '/tmp/delete\x7fpath' },
    { XDG_CACHE_HOME: '/tmp/next-line\u0085path' },
  ]
  for (const overrides of invalid) {
    for (const command of ['plan', 'render-unit']) {
      const result = local.run([command, '--scope', 'user'], overrides)
      assert.notEqual(result.status, 0, 'unsafe or ambiguous selected paths must be refused')
      assert.equal(/^\[Service\]$/m.test(result.stdout), false)
    }
  }
})

test('rendered user argv transfers literal paths and inherited environment to an inert executable', t => {
  const local = fixture(t)
  const overrides = {
    XDG_DATA_HOME: path.join(local.root, '数据=bin "quote" \\ %n $HOME $(touch NATIVE_PWNED) `touch NATIVE_BACKTICK`'),
    XDG_CONFIG_HOME: path.join(local.root, '配置=config "quote" %h'),
    XDG_STATE_HOME: path.join(local.root, '状态=logs "quote" \\ %i ${HOME} $(touch NATIVE_LOG_PWNED) `touch NATIVE_LOG_BACKTICK`'),
    XDG_CACHE_HOME: path.join(local.root, '缓存=cache %t'),
  }
  const expected = userPaths(local.home, overrides)
  const rendered = local.run(['render-unit', '--scope', 'user'], overrides)
  assert.equal(rendered.status, 0, rendered.stderr)
  assertRenderedUnit(rendered.stdout, expected)
  const command = unitWords(unitDirectives(rendered.stdout).get('ExecStart')[0], true)

  // Fixture setup happens only after the final read-only render. The fixed
  // synthetic program never interpolates path data into executable source.
  mkdirSync(path.dirname(expected.binary), { recursive: true })
  mkdirSync(expected.data, { recursive: true })
  writeFileSync(expected.binary, `#!/usr/bin/env node
process.stdout.write(JSON.stringify({
  pid: process.pid,
  binary: process.argv[1],
  argv: process.argv.slice(2),
  marker: process.env.MYAPI_NATIVE_SYNTHETIC_MARKER,
  sqlite: process.env.SQLITE_PATH
}) + '\\n')
process.exitCode = 23
`, { mode: 0o755 })
  chmodSync(expected.binary, 0o755)
  const beforeExecution = snapshotTree(local.root)
  const marker = 'SYNTHETIC_NATIVE_LITERAL_HANDOFF'
  const sqlite = `${expected.data}/my-api.db?_busy_timeout=30000`
  const result = spawnSync(command[0], command.slice(1), {
    cwd: expected.data,
    env: { ...local.environment, ...overrides, MYAPI_NATIVE_SYNTHETIC_MARKER: marker, SQLITE_PATH: sqlite },
    shell: false,
    encoding: 'utf8',
    timeout: 10_000,
    maxBuffer: 1024 * 1024,
  })
  assert.equal(result.error, undefined)
  assert.equal(result.signal, null)
  assert.equal(result.status, 23, 'the fixed shell exec preserves the synthetic executable exit status')
  assert.equal(result.stderr, '')
  assert.equal(readFileSync(local.commandLog, 'utf8'), '', 'path metacharacters must not execute guarded commands')
  assert.deepEqual(snapshotTree(local.root), beforeExecution, 'the handoff must preserve every fixture byte, directory and permission')
  assert.equal(result.stdout.includes(secret), false)
  assert.deepEqual(JSON.parse(result.stdout), {
    pid: result.pid,
    binary: expected.binary,
    argv: ['--log-dir', expected.logs],
    marker,
    sqlite,
  })
})
