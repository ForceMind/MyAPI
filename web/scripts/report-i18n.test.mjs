import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import test from 'node:test'
import { analyzeI18nSnapshot, interpolationSignature, inventoryI18nSource, i18nReportScope } from './report-i18n.mjs'

function fixture() {
  const english = { Hello: 'Hello', OpenAI: 'OpenAI', 'Ready {{count}}': 'Ready {{count}}', 'Old key': 'Old key' }
  const locales = Object.fromEntries(['en', 'zh', 'zh-TW', 'fr', 'ja', 'ru', 'vi'].map(locale => [locale, JSON.stringify({ translation: english })]))
  locales.zh = JSON.stringify({ translation: { Hello: '你好', OpenAI: 'OpenAI', 'Ready {{count}}': '准备好 {{total}}', Extra: '额外项' } })
  return { locales, sources: [
    { file: 'src/i18n/static-keys.ts', source: "export const STATIC_I18N_KEYS = ['Hello', 'Registered missing'] as const\n" },
    { file: 'src/example.tsx', source: "t('Hello')\nt(labelKey)\nt('Literal missing')\n" },
  ] }
}

test('raw missing/extras/equality candidates survive inventory without syncing or accepting translations', () => {
  const input = fixture(), before = JSON.stringify(input)
  const result = analyzeI18nSnapshot(input)
  assert.deepEqual(result.locales.zh.missing, ['Old key'])
  assert.deepEqual(result.locales.zh.extras, ['Extra'])
  assert.deepEqual(result.locales.zh.same_as_english_candidates, ['OpenAI'], 'brand equality is retained as a review candidate, not silently declared translated or wrong')
  assert.deepEqual(result.locales.fr.same_as_english_candidates, ['Hello', 'Old key', 'OpenAI', 'Ready {{count}}'])
  assert.deepEqual(result.locales.en.same_as_english_candidates, [])
  assert.deepEqual(result.locales.zh.placeholder_mismatches.map(row => row.key), ['Ready {{count}}'])
  assert.equal(result.locales.zh.input_sha256, createHash('sha256').update(input.locales.zh).digest('hex'))
  assert.deepEqual(result.source_keys_missing_from_en.map(row => row.key).sort(), ['Literal missing', 'Registered missing'])
  assert.equal(JSON.stringify(input), before)
  assert.equal(Object.hasOwn(result, 'canonical_sync'), false, 'a pure inventory cannot manufacture a canonical execution result')
  assert.match(result.limitations.join('\n'), /not translation failures/)
  assert.match(result.limitations.join('\n'), /No key.*authorized for deletion/)
})

test('interpolation order and padding are insignificant while name, count, escape mode and formatter stay exact', () => {
  assert.deepEqual(interpolationSignature('{{ a }} / {{b}}'), interpolationSignature('{{b}} / {{a}}'))
  assert.deepEqual(interpolationSignature('{{count, number}}'), [{ name: 'count', unescaped: false, format: 'number', count: 1 }])
  for (const [left, right] of [
    ['{{count}}', '{{total}}'], ['{{count}}', '{{count}} {{count}}'], ['{{html}}', '{{-html}}'], ['{{price, number}}', '{{price, currency}}'],
    ['{{user.name}}', '{{user}}'],
  ]) assert.notDeepEqual(interpolationSignature(left), interpolationSignature(right))
  assert.deepEqual(interpolationSignature('OpenAI / ordinary braces {text}'), [])
  assert.throws(() => interpolationSignature(null), /PLACEHOLDER_INPUT_INVALID/)
})

test('literal calls carry source line/hash and comments, strings and regex text do not become calls', () => {
  const source = "// t('comment')\nconst example = \"t('string')\"\nconst pattern = /t\\('regex'\\)/\nconst escaped = t('He\\u006clo')\ni18next.t(\n 'World', { count: 1 })\nt(`Static template`)\n"
  const result = inventoryI18nSource({ file: 'src/example.tsx', source })
  assert.deepEqual(result.literal.map(row => [row.key, row.line]), [['Hello', 4], ['World', 5], ['Static template', 7]])
  assert.equal(result.literal.every(row => row.binding_verified === false && row.source_role === 'production'), true)
  assert.equal(result.file_sha256, createHash('sha256').update(source).digest('hex'))
  assert.equal(result.literal.every(row => row.file_sha256 === result.file_sha256), true)
  assert.deepEqual(result.unknown, [])
})

test('dynamic keys, concatenation, fallback arrays and nested templates remain unknown, never unused', () => {
  const source = "t(labelKey)\nt('Prefix ' + value)\nt(['first', fallback])\nt(`Hello ${name}`)\nt(`Outer ${format(`Inner ${name}`)}`)\nt?.(selected)\n"
  const result = inventoryI18nSource({ file: 'src/feature.ts', source })
  assert.deepEqual(result.literal, [])
  assert.equal(result.unknown.length, 6)
  assert.deepEqual(result.unknown.map(row => row.line), [1, 2, 3, 4, 5, 6])
  assert.equal(result.unknown.filter(row => row.reason === 'template-expression').length, 2)
})

test('static registrations retain repeated source positions and unsupported entries are explicit unknowns', () => {
  const result = inventoryI18nSource({ file: 'src/i18n/static-keys.ts', source: "export const STATIC_I18N_KEYS = [\n 'Hello',\n 'Hello',\n dynamicKey,\n] as const\n" })
  assert.deepEqual(result.literal.map(row => [row.key, row.line]), [['Hello', 2], ['Hello', 3]])
  assert.equal(result.unknown.some(row => row.kind === 'static-registry-unknown' && row.line === 4), true)
  const expression = inventoryI18nSource({ file: 'src/i18n/static-keys.ts', source: "export const STATIC_I18N_KEYS = ['Prefix ' + suffix, ...list, ']', ','] as const" })
  assert.deepEqual(expression.literal.map(row => row.key), [']', ','])
  assert.equal(expression.unknown.filter(row => row.kind === 'static-registry-unknown').length, 2)
  const unparsed = inventoryI18nSource({ file: 'src/label.tsx', source: "<p>Don't assume this is a quoted JavaScript string</p>\n" })
  assert.equal(unparsed.unknown.some(row => row.kind === 'lexical-unknown'), true)
  assert.throws(() => inventoryI18nSource({ file: 'src/i18n/static-keys.ts', source: 'export const other = []' }), /STATIC_REGISTRY_UNKNOWN/)
})

test('test references are labelled and cannot silently become production-use evidence', () => {
  const result = inventoryI18nSource({ file: 'src/components/__tests__/labels.test.tsx', source: "t('Fixture label')" })
  assert.equal(result.literal[0].source_role, 'test')
  assert.equal(result.literal[0].binding_verified, false)
})

test('missing inputs, malformed flat dictionaries, duplicate keys and unsafe source paths fail instead of inventing zero', () => {
  for (const raw of [null, '', '{', '{}', '{"translation":[]}', '{"translation":{"Hello":null}}', '{"translation":{"Hello":"old","Hello":"new"}}', '{"translation":{},"translation":{}}']) {
    const input = fixture(); input.locales.zh = raw
    assert.throws(() => analyzeI18nSnapshot(input))
  }
  const missing = fixture(); delete missing.locales.vi
  assert.throws(() => analyzeI18nSnapshot(missing), /INPUT_SET_INVALID/)
  assert.throws(() => analyzeI18nSnapshot({ ...fixture(), sources: [] }), /INPUT_SET_INVALID/)
  const duplicate = fixture(); duplicate.sources.push(duplicate.sources[0])
  assert.throws(() => analyzeI18nSnapshot(duplicate), /DUPLICATE_SOURCE/)
  for (const file of ['/outside.ts', '../outside.ts', 'src/../outside.ts', 'src/a\\b.ts', 'src/label.json', `src/invalid${String.fromCodePoint(0)}.ts`, `src/invalid${String.fromCodePoint(0x7f)}.ts`]) assert.throws(() => inventoryI18nSource({ file, source: "t('Hello')" }), /SOURCE_INVALID/)
})

const env = { GITHUB_ACTIONS: 'true', MYAPI_I18N_INVENTORY: '1', GITHUB_EVENT_NAME: 'workflow_dispatch', GITHUB_REF: 'refs/heads/dev', GITHUB_REPOSITORY: 'ForceMind/MyAPI',
  GITHUB_SHA: 'a'.repeat(40), GITHUB_RUN_ID: '123', GITHUB_RUN_ATTEMPT: '2', RUNNER_TEMP: '/tmp/synthetic-runner' }

test('manual dev scope binds a deterministic private destination and rejects missing/null/remote identity', () => {
  const before = JSON.stringify(env), scope = i18nReportScope(env)
  assert.equal(scope.directory, `/tmp/synthetic-runner/myapi-i18n-inventory-${'a'.repeat(40)}-123-2`)
  assert.equal(scope.source_sha, env.GITHUB_SHA)
  assert.deepEqual(i18nReportScope({ ...env }), scope)
  assert.notEqual(i18nReportScope({ ...env, GITHUB_RUN_ATTEMPT: '3' }).directory, scope.directory)
  for (const [key, value] of [['GITHUB_ACTIONS', 'false'], ['MYAPI_I18N_INVENTORY', undefined], ['GITHUB_EVENT_NAME', 'push'], ['GITHUB_REF', 'refs/heads/main'],
    ['GITHUB_REPOSITORY', 'someone/MyAPI'], ['GITHUB_SHA', null], ['GITHUB_SHA', 'a'.repeat(39)], ['GITHUB_RUN_ID', 123], ['GITHUB_RUN_ATTEMPT', '0'],
    ['RUNNER_TEMP', null], ['RUNNER_TEMP', 'relative'], ['RUNNER_TEMP', '/'], ['RUNNER_TEMP', '/tmp/../etc'], ['RUNNER_TEMP', '/tmp/unsafe\npath'],
    ['RUNNER_TEMP', `/tmp/unsafe${String.fromCodePoint(0x80)}path`], ['RUNNER_TEMP', `/tmp/unsafe${String.fromCodePoint(0x9f)}path`]]) assert.throws(() => i18nReportScope({ ...env, [key]: value }))
  assert.equal(JSON.stringify(env), before)
})
