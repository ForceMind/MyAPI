import { createHash } from 'node:crypto'
import { lstat, readFile, realpath, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const TOOL = { module: 'golang.org/x/tools/cmd/deadcode', version: 'v0.38.0' }
const SCANS = ['root_production', 'root_with_tests', 'relaykit_with_tests']
const TARGETS = new Set(['linux/amd64', 'darwin/arm64', 'windows/amd64'])
const IDENTIFIER = /^[\p{L}_][\p{L}\p{Nd}_]*$/u

function required(condition, message) {
  if (!condition) throw new Error(message)
}

function inside(root, file) {
  const relative = path.relative(root, file)
  return relative !== '' && relative !== '..' && !relative.startsWith(`..${path.sep}`) && !path.isAbsolute(relative)
}

async function sourceFile(root, filename) {
  required(typeof filename === 'string' && filename.length > 0 && !filename.includes('\0'), 'Missing source file')
  const full = path.resolve(root, filename)
  required(inside(root, full), 'Source path is outside the repository')
  const parts = path.relative(root, full).split(path.sep)
  let current = root
  for (const part of parts) {
    current = path.join(current, part)
    const info = await lstat(current)
    required(!info.isSymbolicLink(), 'Source path contains a symbolic link')
  }
  required((await lstat(full)).isFile() && await realpath(full) === full, 'Source is not a regular repository file')
  return { full, relative: path.relative(root, full).split(path.sep).join('/') }
}

// This is a conservative lexical check, not a replacement Go compiler. Strings,
// runes and comments cannot manufacture declarations or matching brace ranges.
function goTokens(buffer) {
  const text = buffer.toString('utf8')
  required(Buffer.from(text, 'utf8').equals(buffer), 'Source is not valid UTF-8')
  const byteOffsets = new Map()
  let bytes = 0
  for (let index = 0; index < text.length;) {
    byteOffsets.set(index, bytes)
    const character = String.fromCodePoint(text.codePointAt(index))
    index += character.length
    bytes += Buffer.byteLength(character)
  }
  byteOffsets.set(text.length, bytes)
  const tokens = []
  let depth = 0
  for (let index = 0; index < text.length;) {
    const start = index
    const character = String.fromCodePoint(text.codePointAt(index))
    if (/\s/u.test(character) || character === '\ufeff') { index += character.length; continue }
    if (text.startsWith('//', index)) { const end = text.indexOf('\n', index); index = end < 0 ? text.length : end; continue }
    if (text.startsWith('/*', index)) { const end = text.indexOf('*/', index + 2); required(end >= 0, 'Unterminated Go comment'); index = end + 2; continue }
    if (character === '"' || character === "'" || character === '`') {
      index++
      let closed = false
      while (index < text.length) {
        if (character !== '`' && text[index] === '\\') { index += 2; continue }
        if (text[index++] === character) { closed = true; break }
      }
      required(closed, 'Unterminated Go literal')
      continue
    }
    let value = character
    index += character.length
    if (/^[\p{L}_]$/u.test(character)) {
      while (index < text.length) {
        const next = String.fromCodePoint(text.codePointAt(index))
        if (!/^[\p{L}\p{Nd}_]$/u.test(next)) break
        value += next
        index += next.length
      }
    }
    const token = { value, start: byteOffsets.get(start), end: byteOffsets.get(index), depth }
    tokens.push(token)
    if (value === '{') depth++
    if (value === '}') depth--
    required(depth >= 0, 'Unbalanced Go source braces')
  }
  required(depth === 0, 'Unbalanced Go source braces')
  return { tokens, text }
}

function closing(tokens, start, left, right) {
  let depth = 0
  for (let index = start; index < tokens.length; index++) {
    if (tokens[index].value === left) depth++
    if (tokens[index].value === right && --depth === 0) return index
  }
  return -1
}

function declarations(tokens) {
  const result = []
  for (let index = 0; index < tokens.length; index++) {
    if (tokens[index].value !== 'func' || tokens[index].depth !== 0) continue
    let cursor = index + 1
    let receiver = null
    if (tokens[cursor]?.value === '(') {
      const end = closing(tokens, cursor, '(', ')')
      if (end < 0) continue
      let received = tokens.slice(cursor + 1, end)
      if (received.length === 0) continue
      if (IDENTIFIER.test(received[0]?.value || '') && (received[1]?.value === '*' || IDENTIFIER.test(received[1]?.value || ''))) received = received.slice(1)
      if (received[0]?.value === '*') received = received.slice(1)
      receiver = IDENTIFIER.test(received[0]?.value || '') ? received[0].value : null
      cursor = end + 1
    }
    const name = tokens[cursor]
    if (!name || !IDENTIFIER.test(name.value)) continue
    cursor++
    if (tokens[cursor]?.value === '[') { cursor = closing(tokens, cursor, '[', ']') + 1; if (cursor === 0) continue }
    if (tokens[cursor]?.value !== '(') continue
    cursor = closing(tokens, cursor, '(', ')') + 1
    if (cursor === 0) continue
    if (tokens[cursor]?.value === '(') cursor = closing(tokens, cursor, '(', ')') + 1
    let body = -1
    for (; cursor > 0 && cursor < tokens.length; cursor++) {
      const value = tokens[cursor].value
      if (['struct', 'interface', 'func', 'type', 'var', 'const', 'import', 'package', ';'].includes(value)) break
      if (value === '{') { body = cursor; break }
      if (value === '[') { cursor = closing(tokens, cursor, '[', ']'); if (cursor < 0) break }
    }
    const end = body < 0 ? -1 : closing(tokens, body, '{', '}')
    result.push({ name: name.value, nameOffset: name.start, funcOffset: tokens[index].start, receiver, bodyEnd: end < 0 ? null : tokens[end].end })
  }
  return result
}

function lineAt(buffer, offset) {
  let line = 1
  for (let index = 0; index < offset; index++) if (buffer[index] === 10) line++
  return line
}

function positionOffset(buffer, line, column) {
  required(Number.isSafeInteger(line) && line > 0 && Number.isSafeInteger(column) && column > 0, 'Invalid source position')
  let start = 0
  for (let current = 1; current < line; current++) { const end = buffer.indexOf(10, start); required(end >= 0, 'Source line is outside the file'); start = end + 1 }
  const newline = buffer.indexOf(10, start)
  const end = newline < 0 ? buffer.length : newline
  const offset = start + column - 1
  required(offset < end, 'Source column is outside the line')
  required((buffer[offset] & 0xc0) !== 0x80, 'Source column splits a UTF-8 code point')
  return offset
}

function symbolEvidence(buffer, source, name, position) {
  const offset = positionOffset(buffer, position.Line, position.Col)
  const declaration = declarations(source.tokens).find((item) => item.nameOffset === offset || item.funcOffset === offset)
  const method = name.includes('.')
  const last = name.split('.').at(-1)
  const init = /^init(?:#\d+)?$/.test(last)
  const ordinary = IDENTIFIER.test(name) && !method
  const receiver = name.slice(0, name.lastIndexOf('.')).replace(/^\(\*?(.+)\)$/, '$1')
  const matches = declaration && (declaration.name === last || init && declaration.name === 'init') &&
    (method ? receiver === declaration.receiver : declaration.receiver === null)
  if (ordinary || init) required(matches, 'Tool symbol does not match its source declaration')
  else required(declaration || source.tokens.some((token) => token.start === offset && token.value === 'func'), 'Unrecognized symbol has no function source position')
  return {
    method, init, exported: /^\p{Lu}/u.test(last),
    symbol_verified: Boolean(matches),
    source_range: matches && declaration.bodyEnd !== null ? { start_line: lineAt(buffer, declaration.funcOffset), end_line: lineAt(buffer, declaration.bodyEnd - 1) } : null,
  }
}

function parseScan(scan, label) {
  required(scan && scan.exit_code === 0 && typeof scan.json === 'string' && scan.json.trim() !== '', `Missing or failed scan: ${label}`)
  const packages = JSON.parse(scan.json)
  required(packages === null || Array.isArray(packages), `Invalid deadcode package array: ${label}`)
  for (const pkg of packages || []) {
    required(pkg && typeof pkg.Name === 'string' && pkg.Name.length > 0 && typeof pkg.Path === 'string' && pkg.Path.length > 0 && Array.isArray(pkg.Funcs) && pkg.Funcs.length > 0, 'Invalid deadcode package')
    for (const fn of pkg.Funcs) required(fn && typeof fn.Name === 'string' && fn.Name.length > 0 && typeof fn.Generated === 'boolean' && fn.Position && typeof fn.Position.File === 'string', 'Invalid deadcode function')
  }
  return packages || []
}

export async function createReachabilityReport(input) {
  required(input && /^[a-fA-F0-9]{40}$/.test(input.sourceSha || ''), 'Invalid source SHA')
  required(input.tool?.module === TOOL.module && input.tool.version === TOOL.version, 'Unexpected deadcode tool pin')
  required(TARGETS.has(`${input.target?.goos}/${input.target?.goarch}`), 'Unsupported target')
  required(typeof input.run?.id === 'string' && /^[1-9]\d*$/.test(input.run.id) && typeof input.run.attempt === 'string' && /^[1-9]\d*$/.test(input.run.attempt), 'Invalid run identity')
  const root = await realpath(input.repositoryRoot)
  const modules = {}
  for (const [module, filename] of [['root', 'go.mod'], ['relaykit', 'relaykit/go.mod']]) {
    const file = await sourceFile(root, filename)
    const text = await readFile(file.full, 'utf8')
    const match = /^module\s+(\S+)\s*$/m.exec(text)
    required(match && !match[1].startsWith('"'), 'Cannot establish module identity')
    modules[module] = match[1]
  }
  const scans = Object.fromEntries(SCANS.map((name) => [name, parseScan(input.scans?.[name], name)]))
  const records = new Map()
  const positions = new Map()
  const generatedFlags = new Map()
  const sources = new Map()
  for (const name of SCANS) {
    const seen = new Set()
    for (const pkg of scans[name]) for (const fn of pkg.Funcs) {
      const filename = path.isAbsolute(fn.Position.File) ? fn.Position.File : path.join(name === 'relaykit_with_tests' ? 'relaykit' : '', fn.Position.File)
      const file = await sourceFile(root, filename)
      required(file.relative.endsWith('.go'), 'Tool position is not a Go source file')
      const module = file.relative.startsWith('relaykit/') ? 'relaykit' : 'root'
      required(name !== 'relaykit_with_tests' || module === 'relaykit', 'Independent library scan points outside relaykit')
      const moduleFile = module === 'relaykit' ? file.relative.slice('relaykit/'.length) : file.relative
      const directory = path.posix.dirname(moduleFile)
      const expectedPackage = modules[module] + (directory === '.' ? '' : `/${directory}`)
      required(pkg.Path === expectedPackage || file.relative.endsWith('_test.go') && pkg.Path === `${expectedPackage}_test`, 'Package and source module do not match')
      if (!sources.has(file.relative)) {
        const buffer = await readFile(file.full)
        const lexical = goTokens(buffer)
        const packageIndex = lexical.tokens.findIndex((token) => token.value === 'package' && token.depth === 0)
        required(packageIndex >= 0 && IDENTIFIER.test(lexical.tokens[packageIndex + 1]?.value || ''), 'Cannot establish source package clause')
        const header = buffer.subarray(0, lexical.tokens[packageIndex].start).toString('utf8')
        sources.set(file.relative, { buffer, ...lexical, packageName: lexical.tokens[packageIndex + 1].value, generated: /^\/\/ Code generated .* DO NOT EDIT\.$/m.test(header), hash: createHash('sha256').update(buffer).digest('hex') })
      }
      const source = sources.get(file.relative)
      required(source.packageName === pkg.Name, 'Package name does not match the source clause')
      const evidence = symbolEvidence(source.buffer, source, fn.Name, fn.Position)
      const positionKey = JSON.stringify([module, file.relative, fn.Position.Line, fn.Position.Col])
      const symbolKey = JSON.stringify([pkg.Path, pkg.Name, fn.Name])
      required(!positions.has(positionKey) || positions.get(positionKey) === symbolKey, 'Conflicting symbols at the same source position')
      positions.set(positionKey, symbolKey)
      required(!generatedFlags.has(positionKey) || generatedFlags.get(positionKey) === fn.Generated, 'Conflicting generated flags at the same source position')
      generatedFlags.set(positionKey, fn.Generated)
      const key = JSON.stringify([module, pkg.Path, fn.Name, file.relative, fn.Position.Line, fn.Position.Col])
      required(!seen.has(key), 'Duplicate tool function in a scan')
      seen.add(key)
      if (!records.has(key)) records.set(key, {
        module, package: pkg.Path, package_name: pkg.Name, name: fn.Name, file: file.relative,
        line: fn.Position.Line, column: fn.Position.Col, file_sha256: source.hash,
        ...evidence, generated: fn.Generated || source.generated,
        scan_presence: { root_production: false, root_with_tests: false, relaykit_with_tests: false },
      })
      const record = records.get(key)
      record.generated ||= fn.Generated
      record.scan_presence[name] = true
    }
  }
  const result = [...records.values()].map((record) => {
    const test = record.file.endsWith('_test.go')
    record.unreachable_in_production = record.module === 'root' && !test ? record.scan_presence.root_production : null
    record.unreachable_with_tests = record.module === 'root' ? record.scan_presence.root_with_tests : record.scan_presence.relaykit_with_tests
    record.test_reachable_only = record.module === 'root' && !test ? record.unreachable_in_production && !record.unreachable_with_tests : null
    record.protections = []
    if (record.exported) record.protections.push('exported-api')
    if (record.method) record.protections.push('method-or-hook')
    if (record.init) record.protections.push('init')
    if (record.generated) record.protections.push('generated')
    if (test) record.protections.push('test-source')
    if (record.test_reachable_only) record.protections.push('test-reachable-only')
    if (record.module === 'relaykit') record.protections.push('relaykit-public-boundary')
    if (!record.symbol_verified || record.source_range === null) record.protections.push('unknown-symbol')
    record.billing_adjacent = /quota|billing|price|cost|consume|funding|subscription|topup|token/i.test(`${record.package}/${record.file}/${record.name}`)
    record.billing_basis = 'heuristic-only'
    record.manual_review_candidate = record.module === 'root' && record.unreachable_in_production === true && record.unreachable_with_tests === true && record.protections.length === 0
    record.deletion_authorized = false
    return record
  }).sort((a, b) => a.module.localeCompare(b.module) || a.file.localeCompare(b.file) || a.line - b.line || a.column - b.column || a.name.localeCompare(b.name))
  return {
    schema_version: 1, source_sha: input.sourceSha.toLowerCase(), tool: { ...TOOL },
    target: { goos: input.target.goos, goarch: input.target.goarch },
    run: { id: input.run.id, attempt: input.run.attempt }, analysis_only: true, deletion_authorized: false,
    unsupported_checks: ['external-consumers', 'non-default-build-tags', 'targets-outside-the-three-requested-pairs', 'runtime-or-browser-behavior', 'relaykit-public-api-death', 'historical-37-candidate-membership', 'general-Go-parser-proof-for-complex-signatures'],
    scans: Object.fromEntries(SCANS.map((name) => [name, { exit_code: 0, reported_functions: scans[name].reduce((count, pkg) => count + pkg.Funcs.length, 0) }])),
    summary: { records: result.length, manual_review_candidates: result.filter((record) => record.manual_review_candidate).length }, records: result,
  }
}

async function main() {
  const args = process.argv.slice(2)
  const values = {}
  for (let index = 0; index < args.length; index += 2) {
    required(['--repository-root', '--evidence-dir', '--output'].includes(args[index]) && args[index + 1] && !values[args[index]], 'Usage: summarize.mjs --repository-root PATH --evidence-dir PATH --output PATH')
    values[args[index]] = args[index + 1]
  }
  required(Object.keys(values).length === 3, 'Missing report arguments')
  const directory = values['--evidence-dir']
  const metadata = JSON.parse(await readFile(path.join(directory, 'metadata.json'), 'utf8'))
  const scans = {}
  for (const name of SCANS) {
    const filename = name.replaceAll('_', '-')
    const status = (await readFile(path.join(directory, `${filename}.exit`), 'utf8')).trim()
    required(/^\d+$/.test(status), 'Missing actual tool exit status')
    scans[name] = { json: await readFile(path.join(directory, `${filename}.json`), 'utf8'), exit_code: Number(status) }
  }
  const report = await createReachabilityReport({ ...metadata, repositoryRoot: values['--repository-root'], scans })
  await writeFile(values['--output'], JSON.stringify(report, null, 2) + '\n')
  console.log(JSON.stringify({ type: 'reachability_metadata', ...report, records: undefined }))
  for (const record of report.records) console.log(JSON.stringify({ type: 'reachability_candidate', target: report.target, ...record }))
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch((error) => { console.error(`Reachability evidence rejected: ${error.message}`); process.exitCode = 1 })
}
