import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { lstatSync, readFileSync, realpathSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

// Knip 6 JSON reporter: https://knip.dev/features/reporters
// Confirmed against the frozen 6.27.0 dist/reporters/{json,util/util}.js.
// Optional locations remain unknown; this tool does not parse TS declarations
// or prove that a public, dynamic or external consumer does not exist.
function reject(code) {
  throw new Error(code)
}

function objectKeys(value, required, optional = []) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) reject('invalid-object')
  if (required.some(key => !Object.hasOwn(value, key))) reject('missing-field')
  if (Object.keys(value).some(key => !required.includes(key) && !optional.includes(key))) reject('unsupported-field')
}

function safeText(value) {
  return typeof value === 'string' && value.length > 0 && !/[\u0000-\u001f\u007f-\u009f]/u.test(value)
}

function sourcePath(value) {
  if (!safeText(value) || value.includes('\\') || value.startsWith('/') || /^[A-Za-z]:/.test(value) ||
    value.split('/').some(segment => segment === '' || segment === '.' || segment === '..') ||
    !/\.(?:[cm]?[jt]sx?)$/u.test(value)) reject('invalid-source-path')
  return value
}

export function parseKnipReport(inputText, { toolExitCode } = {}) {
  if (toolExitCode !== 0) reject('tool-exit-not-successful')
  if (typeof inputText !== 'string' || inputText.trim() === '') reject('empty-input')
  let input
  try {
    input = JSON.parse(inputText)
  } catch {
    reject('invalid-json')
  }
  objectKeys(input, ['issues'])
  if (!Array.isArray(input.issues)) reject('invalid-issues')
  const files = []
  const records = []
  const seenFiles = new Set()
  const seenRecords = new Set()
  for (const row of input.issues) {
    objectKeys(row, ['file', 'files', 'exports', 'types'], ['owners'])
    const file = sourcePath(row.file)
    if (seenFiles.has(file)) reject('duplicate-file')
    seenFiles.add(file)
    files.push(file)
    if (!Array.isArray(row.files)) reject('invalid-file-list')
    for (const item of row.files) {
      objectKeys(item, ['name'])
      if (sourcePath(item.name) !== file) reject('inconsistent-file-issue')
      const record = {
        file, symbol: null, kind: 'file', line: null, col: null, pos: null,
        namespace: null, symbol_kind: null, specifier: null, manual_review_required: true,
      }
      const identity = JSON.stringify(record)
      if (seenRecords.has(identity)) reject('duplicate-issue')
      seenRecords.add(identity)
      records.push(record)
    }
    if (Object.hasOwn(row, 'owners')) {
      if (!Array.isArray(row.owners)) reject('invalid-owners')
      for (const owner of row.owners) {
        objectKeys(owner, ['name'])
        if (!safeText(owner.name)) reject('invalid-owner')
      }
    }
    for (const kind of ['exports', 'types']) {
      if (!Array.isArray(row[kind])) reject('invalid-issue-list')
      for (const item of row[kind]) {
        objectKeys(item, ['name'], ['namespace', 'kind', 'specifier', 'line', 'col', 'pos'])
        if (!safeText(item.name)) reject('invalid-symbol')
        for (const key of ['namespace', 'kind', 'specifier']) {
          if (Object.hasOwn(item, key) && !safeText(item[key])) reject('invalid-symbol-metadata')
        }
        for (const key of ['line', 'col', 'pos']) {
          if (Object.hasOwn(item, key) && (!Number.isSafeInteger(item[key]) || item[key] < (key === 'pos' ? 0 : 1))) reject('invalid-position')
        }
        const record = {
          file, symbol: item.name, kind,
          line: item.line ?? null, col: item.col ?? null, pos: item.pos ?? null,
          namespace: item.namespace ?? null, symbol_kind: item.kind ?? null, specifier: item.specifier ?? null,
          manual_review_required: true,
        }
        const identity = JSON.stringify(record)
        if (seenRecords.has(identity)) reject('duplicate-issue')
        seenRecords.add(identity)
        records.push(record)
      }
    }
  }
  return { analysis_only: true, deletion_authorized: false, manual_review_required: true, files, records }
}

function sha256(bytes) {
  return createHash('sha256').update(bytes).digest('hex')
}

function regularFileWithin(projectDir, relative) {
  let current = projectDir
  const segments = relative.split('/')
  for (let index = 0; index < segments.length; index++) {
    current = path.join(current, segments[index])
    let stat
    try {
      stat = lstatSync(current)
    } catch {
      reject('source-unavailable')
    }
    if (stat.isSymbolicLink() || (index < segments.length - 1 ? !stat.isDirectory() : !stat.isFile())) reject('unsafe-source-path')
  }
  if (realpathSync(current) !== current) reject('unsafe-source-path')
  return readFileSync(current)
}

// The library accepts an explicit tracked-file inventory so its filesystem
// contracts can use temporary TS fixtures without creating a Git repository.
// Only the CLI below obtains that inventory and clean-HEAD evidence from Git.
export function collectSourceEvidence(parsed, { projectDir, trackedFiles }) {
  const root = realpathSync(projectDir)
  if (!Array.isArray(trackedFiles)) reject('missing-tracked-inventory')
  const tracked = new Set(trackedFiles)
  const sources = new Map()
  for (const relative of parsed.files) {
    sourcePath(relative)
    if (!tracked.has(relative)) reject('untracked-source')
    const bytes = regularFileWithin(root, relative)
    let text
    try {
      text = new TextDecoder('utf-8', { fatal: true }).decode(bytes)
    } catch {
      reject('invalid-source-encoding')
    }
    sources.set(relative, { text, file_sha256: sha256(bytes) })
  }
  const records = parsed.records.map(record => {
    const source = sources.get(record.file)
    if (!source) reject('source-unavailable')
    if (record.kind === 'file') {
      return { ...record, file_sha256: source.file_sha256, position_status: 'not_applicable', symbol_declaration_verified: false }
    }
    const starts = [0]
    for (let index = 0; index < source.text.length; index++) {
      if (source.text[index] === '\n') starts.push(index + 1)
    }
    if (record.line !== null && record.line > starts.length) reject('position-outside-source')
    const longestLine = source.text.split('\n').reduce((longest, line) => Math.max(longest, line.replace(/\r$/u, '').length), 0)
    if (record.col !== null && record.col > longestLine + 1) reject('position-outside-source')
    if (record.col !== null && record.line !== null) {
      const line = source.text.slice(starts[record.line - 1], starts[record.line] ?? source.text.length).replace(/\r?\n$/u, '')
      if (record.col > line.length + 1) reject('position-outside-source')
    }
    if (record.pos !== null) {
      if (record.pos >= source.text.length) reject('position-outside-source')
      const before = source.text.slice(0, record.pos)
      const line = before.split('\n').length
      const col = record.pos - (before.lastIndexOf('\n') + 1) + 1
      if ((record.line !== null && record.line !== line) || (record.col !== null && record.col !== col)) reject('inconsistent-position')
    }
    let positionStatus = 'partial'
    if (record.line === null && record.col === null && record.pos === null) positionStatus = 'unavailable'
    else if (record.line !== null && record.col !== null) positionStatus = record.pos === null ? 'line-column-checked' : 'line-column-offset-checked'
    else if (record.pos !== null) positionStatus = 'offset-checked'
    return { ...record, file_sha256: source.file_sha256, position_status: positionStatus, symbol_declaration_verified: false }
  })
  records.sort((left, right) => left.file.localeCompare(right.file) || left.kind.localeCompare(right.kind) || (left.line ?? 0) - (right.line ?? 0) || (left.col ?? 0) - (right.col ?? 0) || (left.symbol ?? '').localeCompare(right.symbol ?? ''))
  return { records, source_files: [...sources].map(([file, value]) => ({ file, file_sha256: value.file_sha256 })) }
}

function git(directory, args) {
  const result = spawnSync('git', ['-c', 'core.fileMode=false', '-C', directory, ...args], { env: { ...process.env, GIT_OPTIONAL_LOCKS: '0' }, encoding: 'utf8', maxBuffer: 16 * 1024 * 1024, timeout: 10_000 })
  if (result.error || result.signal || result.status !== 0) reject('git-source-check-failed')
  return result.stdout
}

function parseArguments(args) {
  const allowed = ['--input', '--mode', '--project-dir', '--source-sha', '--config-file', '--tool-version', '--tool-exit-code']
  const values = {}
  for (let index = 0; index < args.length; index += 2) {
    const key = args[index]
    const value = args[index + 1]
    if (!allowed.includes(key) || Object.hasOwn(values, key) || !value || value.startsWith('--')) reject('invalid-arguments')
    values[key] = value
  }
  if (allowed.some(key => !Object.hasOwn(values, key))) reject('missing-arguments')
  return values
}

function main(args) {
  const values = parseArguments(args)
  const mode = values['--mode']
  const sourceSha = values['--source-sha']
  if (!['configured', 'ui-inclusive'].includes(mode) || !/^[a-f0-9]{40}$/u.test(sourceSha)) reject('invalid-source-metadata')
  if (values['--tool-version'] !== '6.27.0' || values['--tool-exit-code'] !== '0') reject('invalid-tool-metadata')
  const projectDir = realpathSync(path.resolve(values['--project-dir']))
  const repository = realpathSync(git(projectDir, ['rev-parse', '--show-toplevel']).trim())
  if (projectDir !== path.join(repository, 'web')) reject('invalid-project-root')
  if (git(repository, ['rev-parse', 'HEAD']).trim() !== sourceSha) reject('source-sha-mismatch')
  git(repository, ['diff', '--quiet', 'HEAD', '--'])
  const trackedFiles = git(repository, ['ls-files', '-z', '--', 'web']).split('\0').filter(Boolean).map(file => file.slice('web/'.length))
  const configPath = path.relative(projectDir, path.resolve(values['--config-file'])).split(path.sep).join('/')
  if (!safeText(configPath) || configPath.startsWith('/') || configPath.split('/').some(segment => segment === '' || segment === '.' || segment === '..')) reject('invalid-config-path')
  if (mode === 'configured' && (configPath !== 'knip.config.ts' || !trackedFiles.includes(configPath))) reject('invalid-config-path')
  const configBytes = regularFileWithin(projectDir, configPath)
  const installed = JSON.parse(readFileSync(path.join(projectDir, 'node_modules/knip/package.json'), 'utf8'))
  if (installed.name !== 'knip' || installed.version !== values['--tool-version']) reject('installed-tool-version-mismatch')
  const inputBytes = readFileSync(path.resolve(values['--input']))
  const parsed = parseKnipReport(new TextDecoder('utf-8', { fatal: true }).decode(inputBytes), { toolExitCode: 0 })
  const evidence = collectSourceEvidence(parsed, { projectDir, trackedFiles })
  // Recheck after all source reads; a concurrent tracked edit cannot be
  // certified using an earlier clean result. No Git mutation is performed.
  if (git(repository, ['rev-parse', 'HEAD']).trim() !== sourceSha) reject('source-sha-mismatch')
  git(repository, ['diff', '--quiet', 'HEAD', '--'])
  return {
    schema_version: 1, check: 'unused-exports', project_dir: 'web', source_sha: sourceSha, mode,
    source_validation: { head_matches: true, tracked_tree_clean: true },
    effective_config: { file: configPath, sha256: sha256(configBytes), selection: 'caller-specified', invocation_verified: false },
    input_sha256: sha256(inputBytes),
    tool: { name: 'knip', reported_version: values['--tool-version'], installed_version: installed.version, reported_exit_code: 0, invocation_verified: false, provenance: 'caller-reported' },
    analysis_only: true, deletion_authorized: false, manual_review_required: true,
    issue_scope: ['files', 'exports', 'types'],
    counts: { source_files: evidence.source_files.length, unused_files: evidence.records.filter(record => record.kind === 'file').length, exports: evidence.records.filter(record => record.kind === 'exports').length, types: evidence.records.filter(record => record.kind === 'types').length },
    ...evidence,
    limitations: ['Static candidates require manual review, including barrels, routes, generated files and UI primitives.', 'Dynamic and external consumers are not verified.', 'Locations check source bounds and reported coordinates, not TypeScript declaration semantics.', 'Reported tool exit and effective configuration do not independently prove the analysis invocation.'],
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    process.stdout.write(`${JSON.stringify(main(process.argv.slice(2)))}\n`)
  } catch (error) {
    const code = error instanceof Error && /^[a-z-]+$/u.test(error.message) ? error.message : 'input-or-source-unavailable'
    process.stderr.write(`unused-exports: ${code}\n`)
    process.exitCode = 1
  }
}
