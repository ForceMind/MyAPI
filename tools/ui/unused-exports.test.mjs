import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { lstatSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, readlinkSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'
import { collectSourceEvidence, parseKnipReport } from './unused-exports.mjs'

function parse(value, options = { toolExitCode: 0 }) {
  return parseKnipReport(JSON.stringify(value), options)
}

function report(exports = [], types = [], file = 'src/example.ts') {
  return { issues: [{ file, files: [], exports, types }] }
}

function tree(directory) {
  const entries = []
  for (const name of readdirSync(directory).sort()) {
    const absolute = path.join(directory, name)
    const stat = lstatSync(absolute)
    if (stat.isSymbolicLink()) {
      entries.push([name, 'symlink', readlinkSync(absolute)])
    } else if (stat.isDirectory()) {
      entries.push([name, stat.mode & 0o777, tree(absolute)])
    } else {
      entries.push([name, stat.mode & 0o777, createHash('sha256').update(readFileSync(absolute)).digest('hex')])
    }
  }
  return entries
}

function fixture(t, contents = 'export const Example = 1\n') {
  const root = mkdtempSync(path.join(os.tmpdir(), 'myapi-unused-exports-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const projectDir = path.join(root, 'web')
  mkdirSync(path.join(projectDir, 'src'), { recursive: true })
  const source = path.join(projectDir, 'src/example.ts')
  writeFileSync(source, contents, { mode: 0o640 })
  return { root, projectDir, source, trackedFiles: ['src/example.ts'] }
}

test('the official empty issues array is a valid zero report with no deletion authority', () => {
  assert.deepEqual(parse({ issues: [] }), {
    analysis_only: true, deletion_authorized: false, manual_review_required: true,
    files: [], records: [],
  })
  assert.deepEqual(parse(report()).records, [], 'an explicit empty files scope remains a valid zero row')
})

test('exports and exported types preserve their actual reporter fields without granting permission', () => {
  const input = report(
    [{ name: 'Example', line: 1, col: 14, pos: 13, kind: 'function', specifier: 'Example', namespace: 'Public' }],
    [{ name: 'Shape', line: 2, col: 13, pos: 40 }],
  )
  input.issues[0].owners = [{ name: '@synthetic-owner' }]
  const parsed = parse(input)
  assert.deepEqual(parsed.records, [
    { file: 'src/example.ts', symbol: 'Example', kind: 'exports', line: 1, col: 14, pos: 13,
      namespace: 'Public', symbol_kind: 'function', specifier: 'Example', manual_review_required: true },
    { file: 'src/example.ts', symbol: 'Shape', kind: 'types', line: 2, col: 13, pos: 40,
      namespace: null, symbol_kind: null, specifier: null, manual_review_required: true },
  ])
  assert.equal(parsed.analysis_only, true)
  assert.equal(parsed.deletion_authorized, false)
  assert.equal(parsed.manual_review_required, true)
})

test('an absent or unsuccessful tool exit cannot turn empty output into a successful scan', () => {
  for (const options of [{}, { toolExitCode: 1 }, { toolExitCode: 2 }, { toolExitCode: null }, { toolExitCode: '0' }]) {
    assert.throws(() => parseKnipReport('{"issues":[]}', options), /tool-exit-not-successful/)
  }
  assert.throws(() => parseKnipReport('{"issues":[]}'), /tool-exit-not-successful/)
})

test('blank, broken, partial and unsupported JSON are rejected rather than reported as zero', () => {
  for (const text of ['', ' \n ', '{', 'null', '[]', '{}', '{"issues":null}', '{"issues":{}}']) {
    assert.throws(() => parseKnipReport(text, { toolExitCode: 0 }))
  }
  const invalid = [
    { issues: [], unexpected: true },
    { issues: [{ files: [], exports: [], types: [] }] },
    { issues: [{ file: 'src/example.ts', files: [], exports: [] }] },
    { issues: [{ file: 'src/example.ts', files: [], types: [] }] },
    { issues: [{ file: 'src/example.ts', files: [], exports: {}, types: [] }] },
    { issues: [{ file: 'src/example.ts', files: [], exports: [], types: [], dependencies: [] }] },
    report([{}]), report([{ name: '' }]), report([{ name: 'Example', unexpected: 'do not forward this' }]),
    report([{ name: 'Example', kind: 7 }]), report([{ name: 'Example', namespace: null }]),
    { issues: [{ file: 'src/example.ts', files: [], exports: [], types: [], owners: [{ name: '@owner', extra: true }] }] },
  ]
  for (const input of invalid) {
    assert.throws(() => parse(input))
  }
})

test('unsafe, nonportable or nonsource paths cannot enter source evidence', () => {
  for (const file of ['../outside.ts', '/tmp/outside.ts', 'C:/outside.ts', 'C:\\outside.ts', 'src/../example.ts',
    './src/example.ts', 'src//example.ts', 'src/example.ts\u0000', 'src/line\nbreak.ts', 'package.json']) {
    assert.throws(() => parse(report([{ name: 'Example' }], [], file)), /invalid-source-path/)
  }
})

test('explicit invalid coordinates fail while offset zero remains a known valid value', t => {
  for (const item of [{ line: 0 }, { col: 0 }, { pos: -1 }, { line: 1.5 }, { pos: Number.MAX_SAFE_INTEGER + 1 }, { col: null }]) {
    assert.throws(() => parse(report([{ name: 'Example', ...item }])), /invalid-position/)
  }
  const local = fixture(t)
  const parsed = parse(report([{ name: 'Example', line: 1, col: 1, pos: 0 }]))
  const evidence = collectSourceEvidence(parsed, local)
  assert.equal(evidence.records[0].pos, 0)
  assert.equal(evidence.records[0].position_status, 'line-column-offset-checked')
  assert.equal(evidence.records[0].symbol_declaration_verified, false, 'bounds evidence is not a declaration parser')
})

test('UTF16 positions, CRLF lines, file hashes and every fixture byte remain faithful to source', t => {
  const contents = '// 中文😀\r\nexport const 未用 = 1\nexport type Shape = { ok: boolean }\n'
  const local = fixture(t, contents)
  const before = tree(local.root)
  const parsed = parse(report(
    [{ name: '未用', line: 2, col: 14, pos: contents.indexOf('未用') }],
    [{ name: 'Shape', line: 3, col: 13, pos: contents.indexOf('Shape') }],
  ))
  const evidence = collectSourceEvidence(parsed, local)
  const expectedHash = createHash('sha256').update(contents).digest('hex')
  assert.deepEqual(evidence.source_files, [{ file: 'src/example.ts', file_sha256: expectedHash }])
  assert.deepEqual(evidence.records.map(record => [record.symbol, record.line, record.col, record.file_sha256, record.position_status]), [
    ['未用', 2, 14, expectedHash, 'line-column-offset-checked'],
    ['Shape', 3, 13, expectedHash, 'line-column-offset-checked'],
  ])
  assert.deepEqual(tree(local.root), before)
})

test('optional missing locations remain visible and unknown instead of disappearing or becoming zero', t => {
  const local = fixture(t)
  const evidence = collectSourceEvidence(parse(report([{ name: 'Example' }], [{ name: 'Shape', line: 1 }])), local)
  assert.equal(evidence.records.length, 2)
  assert.deepEqual([evidence.records[0].line, evidence.records[0].col, evidence.records[0].pos], [null, null, null])
  assert.equal(evidence.records[0].position_status, 'unavailable')
  assert.equal(evidence.records[1].line, 1)
  assert.equal(evidence.records[1].col, null)
  assert.equal(evidence.records[1].position_status, 'partial')
  assert.equal(evidence.records.every(record => record.manual_review_required && record.symbol_declaration_verified === false), true)
})

test('out-of-source and internally inconsistent locations are rejected', t => {
  const local = fixture(t)
  for (const item of [{ line: 100, col: 1 }, { line: 1, col: 1000 }, { col: 1000 }, { pos: 1000 },
    { line: 1, col: 2, pos: 0 }, { line: 2, col: 1, pos: 13 }]) {
    assert.throws(() => collectSourceEvidence(parse(report([{ name: 'Example', ...item }])), local), /position-outside-source|inconsistent-position/)
  }
})

test('missing or untracked files cannot hide behind empty arrays', t => {
  const local = fixture(t)
  assert.throws(() => collectSourceEvidence(parse(report()), { ...local, trackedFiles: [] }), /untracked-source/)
  assert.throws(() => collectSourceEvidence(parse(report([], [], 'src/missing.ts')), { ...local, trackedFiles: ['src/missing.ts'] }), /source-unavailable/)
  assert.throws(() => collectSourceEvidence(parse(report()), { projectDir: local.projectDir }), /missing-tracked-inventory/)
})

test('symlink files and symlink directories cannot redirect a tracked source outside web', t => {
  const local = fixture(t)
  const outside = path.join(local.root, 'outside')
  mkdirSync(outside)
  writeFileSync(path.join(outside, 'external.ts'), 'export const External = 1\n')
  symlinkSync(path.join(outside, 'external.ts'), path.join(local.projectDir, 'src/link.ts'))
  symlinkSync(outside, path.join(local.projectDir, 'linked'))
  const before = tree(local.root)
  for (const file of ['src/link.ts', 'linked/external.ts']) {
    assert.throws(() => collectSourceEvidence(parse(report([{ name: 'External' }], [], file)), { ...local, trackedFiles: [file] }), /unsafe-source-path/)
  }
  assert.deepEqual(tree(local.root), before)
})

test('duplicate source groups and duplicate issues are not silently merged', () => {
  const row = report([{ name: 'Example' }]).issues[0]
  assert.throws(() => parse({ issues: [row, row] }), /duplicate-file/)
  assert.throws(() => parse(report([{ name: 'Example' }, { name: 'Example' }])), /duplicate-issue/)
})

test('source hashes reflect bytes read by the library, without any Git or deletion claim', t => {
  const local = fixture(t)
  const parsed = parse(report([{ name: 'Example' }]))
  const first = collectSourceEvidence(parsed, local)
  const changed = 'export const Example = 2\n'
  writeFileSync(local.source, changed)
  const before = tree(local.root)
  const second = collectSourceEvidence(parsed, local)
  assert.notEqual(first.records[0].file_sha256, second.records[0].file_sha256)
  assert.equal(second.records[0].file_sha256, createHash('sha256').update(changed).digest('hex'))
  assert.equal(parsed.deletion_authorized, false)
  assert.deepEqual(tree(local.root), before)
})

test('a whole unused UI source file retains its hash and nonapplicable locations without deletion authority', t => {
  const local = fixture(t)
  const file = 'src/components/ui/unused.tsx'
  const contents = 'export const UnusedPrimitive = () => null\n'
  mkdirSync(path.dirname(path.join(local.projectDir, file)), { recursive: true })
  writeFileSync(path.join(local.projectDir, file), contents, { mode: 0o640 })
  const before = tree(local.root)
  const parsed = parse({ issues: [{ file, files: [{ name: file }], exports: [], types: [] }] })
  const evidence = collectSourceEvidence(parsed, { ...local, trackedFiles: [file] })
  assert.deepEqual(evidence.records, [{
    file, symbol: null, kind: 'file', line: null, col: null, pos: null,
    namespace: null, symbol_kind: null, specifier: null, manual_review_required: true,
    file_sha256: createHash('sha256').update(contents).digest('hex'),
    position_status: 'not_applicable', symbol_declaration_verified: false,
  }])
  assert.deepEqual(evidence.source_files, [{ file, file_sha256: evidence.records[0].file_sha256 }])
  assert.equal(parsed.analysis_only, true)
  assert.equal(parsed.deletion_authorized, false)
  assert.deepEqual(tree(local.root), before)
})

test('missing or malformed files scope is rejected rather than invented as an empty inventory', () => {
  const missing = report()
  delete missing.issues[0].files
  assert.throws(() => parse(missing), /missing-field/)
  assert.throws(() => parse({ files: [], issues: [] }), /unsupported-field/, 'Knip 6 has per-row files, not a legacy root files array')
  for (const files of [null, {}, 'src/example.ts', ['src/example.ts'], [{}], [{ name: '../outside.ts' }],
    [{ name: 'src/other.ts' }], [{ name: 'src/example.ts', line: 0 }],
    [{ name: 'src/example.ts' }, { name: 'src/example.ts' }]]) {
    const input = report()
    input.issues[0].files = files
    assert.throws(() => parse(input))
  }
})
