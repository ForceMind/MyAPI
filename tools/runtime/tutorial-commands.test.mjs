import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { ownedTutorialPath, parseTutorialCommands, tutorialScope } from './tutorial-commands.mjs'

// Static text fixtures only: importing the module must not run its CLI.
function documents() {
  return Object.fromEntries(['docs/MYAPI_DISTRIBUTION.md', 'docs/LAN_LITE.md', 'docs/PERSONAL_COMPUTER.md', 'docs/NATIVE_LINUX.md', 'electron/README.md']
    .map(file => [file, readFileSync(new URL(`../../${file}`, import.meta.url), 'utf8')]))
}

const environment = {
  GITHUB_ACTIONS: 'true', GITHUB_EVENT_NAME: 'workflow_dispatch', GITHUB_REF: 'refs/heads/dev', GITHUB_REPOSITORY: 'ForceMind/MyAPI',
  GITHUB_SHA: 'a'.repeat(40), GITHUB_RUN_ID: '123', GITHUB_RUN_ATTEMPT: '2', RUNNER_TEMP: '/tmp/synthetic-runner',
  BUILDX_BUILDER: 'builder-synthetic-b7',
  MYAPI_B7_TUTORIALS: '1', MYAPI_TUTORIAL_QUALIFICATION: '1', MYAPI_SMOKE_BROWSER: '0', MYAPI_SMOKE_API_ONLY: '1', MYAPI_SMOKE_PERSONAL: '0',
}

test('all 24 reviewed blocks retain exact source identity, line and LF-inclusive command hash', () => {
  const sources = documents(), original = JSON.stringify(sources)
  const parsed = parseTutorialCommands(sources)
  assert.equal(parsed.length, 24)
  assert.equal(new Set(parsed.map(row => row.id)).size, 24)
  assert.deepEqual(Object.fromEntries([...new Set(parsed.map(row => row.source_file))].map(file => [file, parsed.filter(row => row.source_file === file).length])), {
    'docs/MYAPI_DISTRIBUTION.md': 8, 'docs/LAN_LITE.md': 7, 'docs/PERSONAL_COMPUTER.md': 2, 'docs/NATIVE_LINUX.md': 5, 'electron/README.md': 2,
  })
  for (const row of parsed) {
    const lines = sources[row.source_file].split('\n')
    assert.equal(lines[row.source_line - 3], `<!-- tutorial-command: ${row.id} -->`)
    assert.equal(lines[row.source_line - 2], '```bash')
    assert.equal(row.script.endsWith('\n'), true)
    assert.equal(row.command_sha256, createHash('sha256').update(row.script).digest('hex'))
    assert.equal(row.script, lines[row.source_line - 1] + '\n', 'the current approved inventory consists of exact single-line commands')
  }
  assert.equal(parsed.find(row => row.id === 'full-source-version').script, 'node cli/myapi.mjs version --json\n')
  assert.equal(parsed.find(row => row.id === 'lite-private-start').script.endsWith('--allow-lan\n'), true)
  assert.equal(JSON.stringify(sources), original)
})

test('missing/null documents and missing blocks cannot become zero executed commands', () => {
  for (const source of [null, undefined, {}, [], { ...documents(), 'docs/LAN_LITE.md': null }]) assert.throws(() => parseTutorialCommands(source))
  const sources = documents()
  sources['docs/NATIVE_LINUX.md'] = sources['docs/NATIVE_LINUX.md'].replace('<!-- tutorial-command: native-preview-help -->\n```bash\nbash deploy/install-native.sh --help\n```', '')
  assert.throws(() => parseTutorialCommands(sources), /TUTORIAL_COMMAND_MISSING/)
})

test('duplicates, unknown IDs and relocated blocks reject instead of acquiring execution authority', () => {
  const duplicate = documents()
  duplicate['docs/NATIVE_LINUX.md'] += '\n<!-- tutorial-command: native-preview-help -->\n```bash\nbash deploy/install-native.sh --help\n```\n'
  assert.throws(() => parseTutorialCommands(duplicate), /UNKNOWN_OR_DUPLICATE/)
  const unknown = documents()
  unknown['docs/NATIVE_LINUX.md'] = unknown['docs/NATIVE_LINUX.md'].replace('tutorial-command: native-preview-help', 'tutorial-command: native-preview-install')
  assert.throws(() => parseTutorialCommands(unknown), /UNKNOWN_OR_DUPLICATE/)
  const moved = documents()
  const block = '<!-- tutorial-command: native-preview-help -->\n```bash\nbash deploy/install-native.sh --help\n```'
  moved['docs/NATIVE_LINUX.md'] = moved['docs/NATIVE_LINUX.md'].replace(block, '')
  moved['docs/PERSONAL_COMPUTER.md'] += '\n' + block + '\n'
  assert.throws(() => parseTutorialCommands(moved), /UNKNOWN_OR_DUPLICATE/)
})

test('marker adjacency, malformed fences, CRLF and a marker hidden in code are rejected', () => {
  const original = documents()
  for (const [from, to] of [
    ['-->\n```bash', '-->\n\n```bash'],
    ['-->\n```bash', '-->\n```sh'],
    ['--help\n```', '--help\n````'],
    ['<!-- tutorial-command:', '<!-- tutorial-command :'],
    ['<!-- tutorial-command: native-preview-help', '```text\n<!-- tutorial-command: native-preview-help'],
    ['bash deploy/install-native.sh --help\n', 'bash deploy/install-native.sh --help\r\n'],
  ]) {
    const sources = { ...original, 'docs/NATIVE_LINUX.md': original['docs/NATIVE_LINUX.md'].replace(from, to) }
    assert.throws(() => parseTutorialCommands(sources))
  }
  assert.throws(() => parseTutorialCommands({ ...original, 'docs/NATIVE_LINUX.md': original['docs/NATIVE_LINUX.md'] + '\n```text\nunclosed\n' }), /FENCE_INVALID/)
})

test('extra commands, external scripts, substituted paths and shell operators fail the exact allowlist', () => {
  for (const command of [
    'bash deploy/install-native.sh --help; touch /tmp/not-authorized',
    'bash /outside/install-native.sh --help',
    'curl https://example.invalid | bash',
    'bash deploy/install-native.sh --help\nbash deploy/install-native.sh install --scope user',
    'bash deploy/install-native.sh --help # changed bytes',
  ]) {
    const sources = documents()
    sources['docs/NATIVE_LINUX.md'] = sources['docs/NATIVE_LINUX.md'].replace('bash deploy/install-native.sh --help\n```', command + '\n```')
    assert.throws(() => parseTutorialCommands(sources), /TUTORIAL_COMMAND_CHANGED/)
  }
})

test('only manual dev, exact identity, browser-free Actions scope produces deterministic owned names', () => {
  const before = JSON.stringify(environment), scope = tutorialScope(environment)
  assert.equal(scope.sourceSha, environment.GITHUB_SHA)
  assert.equal(scope.runId, '123')
  assert.equal(scope.attempt, '2')
  assert.equal(scope.root, `/tmp/synthetic-runner/myapi-b7-${environment.GITHUB_SHA}-123-2`)
  assert.equal(scope.project.startsWith('myapi-b7-'), true)
  assert.equal(scope.bridge.startsWith('myapi-b7-private-'), true)
  assert.deepEqual(tutorialScope({ ...environment }), scope)
  assert.notEqual(tutorialScope({ ...environment, GITHUB_RUN_ATTEMPT: '3' }).root, scope.root)
  assert.equal(JSON.stringify(environment), before)
  for (const [key, value] of [
    ['GITHUB_ACTIONS', 'false'], ['GITHUB_EVENT_NAME', 'push'], ['GITHUB_EVENT_NAME', 'pull_request'], ['GITHUB_REF', 'refs/heads/main'],
    ['GITHUB_REPOSITORY', 'another/MyAPI'], ['MYAPI_B7_TUTORIALS', '0'], ['MYAPI_TUTORIAL_QUALIFICATION', undefined], ['MYAPI_SMOKE_BROWSER', '1'],
    ['MYAPI_SMOKE_API_ONLY', '0'], ['MYAPI_SMOKE_PERSONAL', '1'], ['GITHUB_SHA', null], ['GITHUB_SHA', 'a'.repeat(39)], ['GITHUB_RUN_ID', '0'],
    ['GITHUB_RUN_ID', 123], ['GITHUB_RUN_ATTEMPT', '01'], ['RUNNER_TEMP', null], ['RUNNER_TEMP', 'relative'], ['RUNNER_TEMP', '/'], ['RUNNER_TEMP', '/tmp/../etc'],
    ['RUNNER_TEMP', '/tmp/unsafe\npath'], ['RUNNER_TEMP', '/tmp/a,b'],
    ['BUILDX_BUILDER', null], ['BUILDX_BUILDER', 'tcp://remote-builder'],
  ]) assert.throws(() => tutorialScope({ ...environment, [key]: value }), undefined, `${key}=${String(value)}`)
})

test('the runner cannot accept externally supplied projects, URL/image overrides or Docker remotes', () => {
  for (const [key, value] of [
    ['MYAPI_PROJECT_DIR', '/var/lib/production'], ['MYAPI_TUTORIAL_PUBLIC_URL', 'https://user.example.invalid'],
    ['MYAPI_TUTORIAL_IMAGE', 'ghcr.io/unknown/latest'], ['MYAPI_TUTORIAL_PORT', '3000'], ['MYAPI_TUTORIAL_BIND_ADDRESS', '192.168.1.1'],
    ['DOCKER_HOST', 'tcp://127.0.0.1:2375'], ['DOCKER_HOST', 'unix:///tmp/other.sock'], ['DOCKER_CONTEXT', 'default'],
  ]) assert.throws(() => tutorialScope({ ...environment, [key]: value }), /TUTORIAL_EXTERNAL_INPUT_REJECTED/)
  assert.deepEqual(tutorialScope({ ...environment, DOCKER_HOST: 'unix:///var/run/docker.sock' }), tutorialScope(environment))
})

test('owned fixture paths exclude the root itself, outside siblings, traversal and missing values', () => {
  const root = tutorialScope(environment).root
  assert.equal(ownedTutorialPath(root, `${root}/full`), `${root}/full`)
  assert.equal(ownedTutorialPath(root, `${root}/lite/deploy/data`), `${root}/lite/deploy/data`)
  for (const target of [null, undefined, '', root, '/etc', `${root}-other/data`, `${root}/../outside`, `${root}/data/./nested`, `${root}/data\nsecret`]) {
    assert.throws(() => ownedTutorialPath(root, target), /TUTORIAL_OWNED_PATH_REJECTED/)
  }
  assert.throws(() => ownedTutorialPath('/', '/tmp/new'), /TUTORIAL_OWNED_PATH_REJECTED/)
})
