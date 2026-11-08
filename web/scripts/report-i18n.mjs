#!/usr/bin/env node
// Inventory only. Canonical sync runs on a private copy, never the checkout.
import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { existsSync, lstatSync, mkdirSync, readFileSync, realpathSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = fileURLToPath(new URL('../', import.meta.url)).replace(/\/$/, '')
const repositoryRoot = path.dirname(webRoot)
const locales = ['en', 'zh', 'zh-TW', 'fr', 'ja', 'ru', 'vi']
const digest = value => createHash('sha256').update(value).digest('hex')
function demand(condition, code) { if (!condition) throw new Error(code) }

function dictionary(raw) {
  demand(typeof raw === 'string' && Buffer.byteLength(raw) <= 16 * 1024 * 1024, 'I18N_DICTIONARY_INVALID')
  let value
  try { value = JSON.parse(raw) } catch { throw new Error('I18N_DICTIONARY_INVALID') }
  demand(value && !Array.isArray(value) && Object.keys(value).join() === 'translation' && value.translation &&
    typeof value.translation === 'object' && !Array.isArray(value.translation) && Object.values(value.translation).every(item => typeof item === 'string'), 'I18N_DICTIONARY_INVALID')
  // JSON.parse accepts duplicate object members. Inventory must not discard
  // an earlier value and call the surviving translation the complete input.
  const stack = [], token = /("(?:\\.|[^"\\])*")|([{}])/g
  for (const match of raw.matchAll(token)) {
    if (match[2] === '{') stack.push(new Set())
    else if (match[2] === '}') stack.pop()
    else if (/^\s*:/.test(raw.slice(match.index + match[0].length))) {
      const key = JSON.parse(match[1]), keys = stack.at(-1)
      demand(keys && !keys.has(key), 'I18N_DUPLICATE_DICTIONARY_KEY')
      keys.add(key)
    }
  }
  return value.translation
}

export function interpolationSignature(value) {
  demand(typeof value === 'string', 'I18N_PLACEHOLDER_INPUT_INVALID')
  const tokens = new Map()
  for (const match of value.matchAll(/\{\{\s*(-)?\s*([^{}]+?)\s*\}\}/g)) {
    const separator = match[2].indexOf(',')
    const name = (separator < 0 ? match[2] : match[2].slice(0, separator)).trim()
    const format = separator < 0 ? null : match[2].slice(separator + 1).trim()
    const key = JSON.stringify({ name, unescaped: Boolean(match[1]), format })
    tokens.set(key, (tokens.get(key) || 0) + 1)
  }
  return [...tokens].sort(([left], [right]) => left < right ? -1 : left > right ? 1 : 0)
    .map(([token, count]) => ({ ...JSON.parse(token), count }))
}

// This is deliberately a lexical inventory, not a TypeScript binding or
// reachability analysis. Unsupported expressions remain unknown references.
function sourceTokens(source) {
  const tokens = []
  let offset = 0, line = 1
  const advance = end => { line += (source.slice(offset, end).match(/\n/g) || []).length; offset = end }
  const quoted = (start, quote) => {
    let value = '', cursor = start + 1, dynamic = false
    while (cursor < source.length) {
      const character = source[cursor++]
      if (character === quote) return { end: cursor, value, dynamic }
      if (quote === '`' && character === '$' && source[cursor] === '{') {
        dynamic = true; cursor++
        let depth = 1
        while (cursor < source.length && depth > 0) {
          const inside = source[cursor]
          if (["'", '"', '`'].includes(inside)) { cursor = quoted(cursor, inside).end; continue }
          if (source.startsWith('//', cursor)) { const end = source.indexOf('\n', cursor); cursor = end < 0 ? source.length : end; continue }
          if (source.startsWith('/*', cursor)) { const end = source.indexOf('*/', cursor + 2); demand(end >= 0, 'I18N_SOURCE_STRING_INVALID'); cursor = end + 2; continue }
          if (inside === '{') depth++
          else if (inside === '}') depth--
          cursor++
        }
        demand(depth === 0, 'I18N_SOURCE_STRING_UNTERMINATED')
        continue
      }
      demand(quote === '`' || character !== '\n' && character !== '\r', 'I18N_SOURCE_STRING_INVALID')
      if (character !== '\\') { value += character; continue }
      demand(cursor < source.length, 'I18N_SOURCE_STRING_UNTERMINATED')
      const escaped = source[cursor++]
      if (escaped === '\n') continue
      if (escaped === '\r') { if (source[cursor] === '\n') cursor++; continue }
      const simple = { n: '\n', r: '\r', t: '\t', b: '\b', f: '\f', v: '\v', '0': '\0' }
      if (Object.hasOwn(simple, escaped)) value += simple[escaped]
      else if (escaped === 'x' || escaped === 'u') {
        let length = escaped === 'x' ? 2 : 4
        if (escaped === 'u' && source[cursor] === '{') {
          const end = source.indexOf('}', cursor + 1)
          demand(end >= 0 && /^[a-fA-F0-9]{1,6}$/.test(source.slice(cursor + 1, end)), 'I18N_SOURCE_STRING_INVALID')
          const point = Number.parseInt(source.slice(cursor + 1, end), 16)
          demand(point <= 0x10ffff, 'I18N_SOURCE_STRING_INVALID')
          value += String.fromCodePoint(point); cursor = end + 1; continue
        }
        const digits = source.slice(cursor, cursor + length)
        demand(new RegExp(`^[a-fA-F0-9]{${length}}$`).test(digits), 'I18N_SOURCE_STRING_INVALID')
        value += String.fromCharCode(Number.parseInt(digits, 16)); cursor += length
      } else value += escaped
    }
    throw new Error('I18N_SOURCE_STRING_UNTERMINATED')
  }
  while (offset < source.length) {
    const character = source[offset], start = offset, startLine = line
    if (/\s/.test(character)) { advance(offset + 1); continue }
    if (source.startsWith('//', offset)) { const end = source.indexOf('\n', offset); advance(end < 0 ? source.length : end); continue }
    if (source.startsWith('/*', offset)) { const end = source.indexOf('*/', offset + 2); demand(end >= 0, 'I18N_SOURCE_COMMENT_UNTERMINATED'); advance(end + 2); continue }
    if (character === '"' || character === "'" || character === '`') {
      try {
        const literal = quoted(offset, character)
        tokens.push({ kind: literal.dynamic ? 'dynamic-template' : 'string', value: literal.value, offset: start, line: startLine })
        advance(literal.end)
      } catch {
        // JSX text and unsupported syntax are not proof of an empty call set.
        tokens.push({ kind: 'unknown', value: null, offset: start, line: startLine })
        const end = source.indexOf('\n', offset)
        advance(end < 0 ? source.length : end)
      }
      continue
    }
    if (/[\p{L}_$]/u.test(character)) {
      let end = offset + 1
      while (end < source.length && /[\p{L}\p{N}_$]/u.test(source[end])) end++
      tokens.push({ kind: 'identifier', value: source.slice(offset, end), offset: start, line: startLine }); advance(end); continue
    }
    // Opaque regular expressions avoid treating their text as call sites.
    const previous = tokens.at(-1)?.value
    if (character === '/' && (!previous || ['=', '(', '[', ',', ':', '!', ';', 'return', '>'].includes(previous))) {
      let cursor = offset + 1, inClass = false
      while (cursor < source.length && source[cursor] !== '\n') {
        const current = source[cursor++]
        if (current === '\\') { cursor++; continue }
        if (current === '[') inClass = true
        else if (current === ']') inClass = false
        else if (current === '/' && !inClass) {
          while (/[a-z]/i.test(source[cursor] || '') && cursor < source.length) cursor++
          tokens.push({ kind: 'regex', value: null, offset: start, line: startLine }); advance(cursor); break
        }
      }
      if (offset !== start) continue
    }
    tokens.push({ kind: 'punctuation', value: character, offset: start, line: startLine }); advance(offset + 1)
  }
  return tokens
}

export function inventoryI18nSource({ file, source }) {
  demand(typeof file === 'string' && /^src\/.+\.(?:[jt]sx?)$/.test(file) && !file.split('/').some(part => !part || part === '.' || part === '..') &&
    !/[\\\u0000-\u001f\u007f]/u.test(file) && typeof source === 'string', 'I18N_SOURCE_INVALID')
  const fileSha = digest(source), tokens = sourceTokens(source), literal = [], unknown = []
  const location = token => ({ file, line: token.line, file_sha256: fileSha, source_role: /(?:__tests__|\.(?:test|spec)\.)/.test(file) ? 'test' : 'production' })
  for (let index = 0; index < tokens.length; index++) {
    const token = tokens[index]
    if (token.kind === 'unknown') unknown.push({ ...location(token), kind: 'lexical-unknown', reason: 'unrecognized-string-or-jsx-fragment' })
    if (token.kind !== 'identifier' || token.value !== 't') continue
    let opening = index + 1
    if (tokens[opening]?.kind === 'string' || tokens[opening]?.kind === 'dynamic-template' || tokens[opening]?.value === '<') {
      unknown.push({ ...location(token), kind: 'dynamic-t', reason: 'tagged-template-or-generic-syntax' }); continue
    }
    if (tokens[opening]?.value === '?' && tokens[opening + 1]?.value === '.') opening += 2
    if (tokens[opening]?.value !== '(') continue
    const first = tokens[opening + 1], next = tokens[opening + 2]
    if (first?.kind === 'string' && [',', ')'].includes(next?.value)) literal.push({ ...location(token), kind: 'literal-t', key: first.value, binding_verified: false })
    else unknown.push({ ...location(token), kind: 'dynamic-t', reason: first?.kind === 'dynamic-template' ? 'template-expression' : 'nonliteral-or-expression-argument' })
  }
  if (file === 'src/i18n/static-keys.ts') {
    const declaration = tokens.findIndex(token => token.kind === 'identifier' && token.value === 'STATIC_I18N_KEYS')
    demand(declaration >= 0 && tokens[declaration + 1]?.value === '=' && tokens[declaration + 2]?.value === '[', 'I18N_STATIC_REGISTRY_UNKNOWN')
    let index = declaration + 3, depth = 0, entry = []
    const registration = () => {
      if (entry.length === 1 && entry[0].kind === 'string') literal.push({ ...location(entry[0]), kind: 'static-key', key: entry[0].value, binding_verified: false })
      else if (entry.length) unknown.push({ ...location(entry[0]), kind: 'static-registry-unknown', reason: 'nonliteral-registry-entry' })
      entry = []
    }
    for (; index < tokens.length; index++) {
      const token = tokens[index]
      if (token.kind === 'punctuation' && depth === 0 && token.value === ']') { registration(); break }
      if (token.kind === 'punctuation' && depth === 0 && token.value === ',') { registration(); continue }
      if (token.kind === 'punctuation' && ['[', '(', '{'].includes(token.value)) depth++
      else if (token.kind === 'punctuation' && [']', ')', '}'].includes(token.value)) depth--
      entry.push(token)
    }
    demand(tokens[index]?.kind === 'punctuation' && tokens[index].value === ']' && depth === 0, 'I18N_STATIC_REGISTRY_UNKNOWN')
  }
  return { file, file_sha256: fileSha, literal, unknown }
}

export function analyzeI18nSnapshot({ locales: raw, sources }) {
  demand(raw && typeof raw === 'object' && !Array.isArray(raw) && Object.keys(raw).sort().join() === [...locales].sort().join() &&
    Array.isArray(sources) && sources.length > 0 && sources.some(item => item?.file === 'src/i18n/static-keys.ts'), 'I18N_INPUT_SET_INVALID')
  const dictionaries = Object.fromEntries(locales.map(locale => [locale, dictionary(raw[locale])]))
  const base = dictionaries.en, baseKeys = Object.keys(base).sort(), seen = new Set()
  const references = sources.map(item => {
    demand(item && !seen.has(item.file), 'I18N_DUPLICATE_SOURCE')
    seen.add(item.file)
    return inventoryI18nSource(item)
  }).sort((left, right) => left.file < right.file ? -1 : left.file > right.file ? 1 : 0)
  const localeReports = Object.fromEntries(locales.map(locale => {
    const target = dictionaries[locale], keys = Object.keys(target).sort()
    return [locale, { input_sha256: digest(raw[locale]), key_count: keys.length,
      missing: baseKeys.filter(key => !Object.hasOwn(target, key)), extras: keys.filter(key => !Object.hasOwn(base, key)),
      same_as_english_candidates: locale === 'en' ? [] : baseKeys.filter(key => Object.hasOwn(target, key) && target[key] === base[key]),
      placeholder_mismatches: locale === 'en' ? [] : baseKeys.filter(key => Object.hasOwn(target, key)).flatMap(key => {
        const expected = interpolationSignature(base[key]), actual = interpolationSignature(target[key])
        return JSON.stringify(expected) === JSON.stringify(actual) ? [] : [{ key, expected, actual }]
      }) }]
  }))
  return { locales: localeReports, references,
    source_keys_missing_from_en: references.flatMap(file => file.literal.filter(row => !Object.hasOwn(base, row.key))),
    limitations: ['Same-as-English values are review candidates, including legitimate brands and cognates; they are not translation failures.',
      'Source references are lexical candidates; bindings, aliases, JSX text, regular-expression ambiguities and runtime reachability are not verified.',
      'Nested template expressions and runtime registries may contain additional references. Dynamic calls are unknown, never evidence that a key is unused.',
      'Placeholder comparison covers i18next {{...}} interpolation names, escape mode, formatter text and multiplicity; nesting, plural/context behavior and rendered layout require separate review.',
      'Dictionary coverage does not verify language availability in Full, Lite or Desktop bundles, native-language quality or browser layout.',
      'Test references are labelled and do not establish production use. No key, translation, file or export is authorized for deletion.'] }
}

export function i18nReportScope(env) {
  demand(env && env.GITHUB_ACTIONS === 'true' && env.MYAPI_I18N_INVENTORY === '1' && env.GITHUB_EVENT_NAME === 'workflow_dispatch' &&
    env.GITHUB_REF === 'refs/heads/dev' && env.GITHUB_REPOSITORY === 'ForceMind/MyAPI', 'I18N_SCOPE_REJECTED')
  demand(typeof env.GITHUB_SHA === 'string' && /^[a-f0-9]{40}$/.test(env.GITHUB_SHA) && typeof env.GITHUB_RUN_ID === 'string' && /^[1-9][0-9]{0,19}$/.test(env.GITHUB_RUN_ID) &&
    typeof env.GITHUB_RUN_ATTEMPT === 'string' && /^[1-9][0-9]{0,19}$/.test(env.GITHUB_RUN_ATTEMPT), 'I18N_IDENTITY_REJECTED')
  demand(typeof env.RUNNER_TEMP === 'string' && path.isAbsolute(env.RUNNER_TEMP) && path.normalize(env.RUNNER_TEMP) === env.RUNNER_TEMP && env.RUNNER_TEMP !== '/' &&
    !/[\u0000-\u001f\u007f-\u009f]/u.test(env.RUNNER_TEMP), 'I18N_TEMP_REJECTED')
  return { source_sha: env.GITHUB_SHA, run_id: env.GITHUB_RUN_ID, run_attempt: env.GITHUB_RUN_ATTEMPT,
    directory: path.join(env.RUNNER_TEMP, `myapi-i18n-inventory-${env.GITHUB_SHA}-${env.GITHUB_RUN_ID}-${env.GITHUB_RUN_ATTEMPT}`) }
}

function git(args) {
  const result = spawnSync('git', ['-c', 'core.fileMode=false', ...args], { cwd: repositoryRoot, encoding: 'utf8', timeout: 30000, maxBuffer: 8 * 1024 * 1024 })
  demand(!result.error && result.status === 0, 'I18N_SOURCE_IDENTITY_UNAVAILABLE')
  return result.stdout
}

function readSource(file) {
  const absolute = path.join(webRoot, file), info = lstatSync(absolute)
  demand(info.isFile() && !info.isSymbolicLink() && info.size <= 16 * 1024 * 1024 && realpathSync(absolute) === absolute, 'I18N_SOURCE_FILE_UNSAFE')
  return readFileSync(absolute, 'utf8')
}

function logInventory(report) {
  const references = report.references || []
  const sourceMissing = report.source_keys_missing_from_en || []
  console.log(JSON.stringify({ type: 'i18n-source-summary', source_files: references.length,
    literal_references: references.reduce((count, file) => count + file.literal.length, 0),
    unknown_references: references.reduce((count, file) => count + file.unknown.length, 0),
    missing_key_candidates: new Set(sourceMissing.map(row => row.key)).size, missing_reference_candidates: sourceMissing.length }))
  for (const reference of sourceMissing) console.log(JSON.stringify({ type: 'i18n-source-missing-key', ...reference }))
  for (const [locale, data] of Object.entries(report.locales || {})) {
    console.log(JSON.stringify({ type: 'i18n-locale-summary', locale, input_sha256: data.input_sha256, key_count: data.key_count,
      missing_count: data.missing.length, extras_count: data.extras.length, same_english_candidate_count: data.same_as_english_candidates.length, placeholder_mismatch_count: data.placeholder_mismatches.length }))
    for (const [kind, keys] of Object.entries({ missing: data.missing, extras: data.extras, same_english_candidates: data.same_as_english_candidates, placeholder_mismatches: data.placeholder_mismatches.map(item => item.key) })) {
      let chunk = [], offset = 0
      for (const key of keys) {
        if (chunk.length && JSON.stringify([...chunk, key]).length > 12000) {
          console.log(JSON.stringify({ type: 'i18n-candidate-keys', locale, kind, offset, keys: chunk })); offset += chunk.length; chunk = []
        }
        chunk.push(key)
      }
      console.log(JSON.stringify({ type: 'i18n-candidate-keys', locale, kind, offset, keys: chunk }))
    }
  }
}

async function main() {
  let scope, original, reportFile, outputBinding
  const report = { schema_version: 1, source_sha: null, run_id: null, run_attempt: null, passed: false, translation_accepted: false,
    analysis_only: true, deletion_authorized: false, source_files_unchanged: false, canonical_sync: { exit_code: null }, error_code: null }
  const oldUmask = process.umask()
  try {
    scope = i18nReportScope(process.env)
    Object.assign(report, { source_sha: scope.source_sha, run_id: scope.run_id, run_attempt: scope.run_attempt })
    demand(process.argv.length === 2 && process.cwd() === webRoot && Number(process.versions.node.split('.')[0]) >= 22, 'I18N_INVOCATION_REJECTED')
    demand(git(['rev-parse', 'HEAD']).trim() === scope.source_sha && git(['status', '--porcelain', '--untracked-files=no']).trim() === '', 'I18N_SOURCE_CHANGED')
    const tracked = git(['ls-files', '-z', '--', 'web/src', 'web/scripts/sync-i18n.mjs']).split('\0').filter(Boolean)
    const sourceFiles = tracked.filter(file => /^web\/src\/.+\.[jt]sx?$/.test(file)).map(file => file.slice(4)).sort()
    const localeFiles = locales.map(locale => `src/i18n/locales/${locale}.json`), canonicalFile = 'scripts/sync-i18n.mjs'
    demand([canonicalFile, ...localeFiles, ...sourceFiles].every(file => tracked.includes('web/' + file)), 'I18N_SOURCE_INCOMPLETE')
    original = Object.fromEntries([canonicalFile, ...localeFiles, ...sourceFiles].map(file => [file, readSource(file)]))
    const snapshot = analyzeI18nSnapshot({ locales: Object.fromEntries(locales.map((locale, index) => [locale, original[localeFiles[index]]])), sources: sourceFiles.map(file => ({ file, source: original[file] })) })
    const temp = lstatSync(process.env.RUNNER_TEMP)
    demand(temp.isDirectory() && !temp.isSymbolicLink() && temp.uid === process.getuid() && realpathSync(process.env.RUNNER_TEMP) === process.env.RUNNER_TEMP && !existsSync(scope.directory), 'I18N_TEMP_UNSAFE')
    process.umask(0o077)
    mkdirSync(scope.directory, { mode: 0o700 })
    const directoryInfo = lstatSync(scope.directory, { bigint: true })
    outputBinding = `${directoryInfo.dev}:${directoryInfo.ino}`
    const owner = JSON.stringify({ source_sha: scope.source_sha, run_id: scope.run_id, run_attempt: scope.run_attempt })
    writeFileSync(path.join(scope.directory, 'ownership.json'), owner + '\n', { flag: 'wx', mode: 0o600 })
    reportFile = path.join(scope.directory, 'report.json')
    Object.assign(report, snapshot, { input_files: Object.entries(original).map(([file, source]) => ({ file, sha256: digest(source) })) })
    const copy = path.join(scope.directory, 'canonical-copy')
    mkdirSync(path.join(copy, 'scripts'), { recursive: true, mode: 0o700 })
    mkdirSync(path.join(copy, 'src/i18n/locales'), { recursive: true, mode: 0o700 })
    for (const file of [canonicalFile, ...localeFiles]) writeFileSync(path.join(copy, file), original[file], { flag: 'wx', mode: 0o600 })
    const result = spawnSync(process.execPath, [canonicalFile], { cwd: copy, env: { PATH: process.env.PATH, HOME: process.env.HOME, TMPDIR: scope.directory },
      encoding: 'utf8', timeout: 120000, maxBuffer: 1024 * 1024 })
    report.canonical_sync = { exit_code: Number.isInteger(result.status) ? result.status : null, executed: !result.error || typeof result.pid === 'number' && result.pid > 0,
      input_script_sha256: digest(original[canonicalFile]), input_locale_sha256: Object.fromEntries(locales.map((locale, index) => [locale, digest(original[localeFiles[index]])])), output_locale_sha256: null, output_report_sha256: null }
    demand(!result.error && result.signal === null && result.status === 0, 'I18N_CANONICAL_SYNC_FAILED')
    report.canonical_sync.output_locale_sha256 = Object.fromEntries(locales.map((locale, index) => [locale, digest(readFileSync(path.join(copy, localeFiles[index])))]))
    report.canonical_sync.output_report_sha256 = digest(readFileSync(path.join(copy, 'src/i18n/locales/_reports/_sync-report.json')))
    demand(digest(readFileSync(path.join(copy, canonicalFile))) === digest(original[canonicalFile]), 'I18N_CANONICAL_SCRIPT_CHANGED')
    report.passed = true
  } catch (error) {
    report.error_code = /^I18N_[A-Z_]+$/.test(error.message || '') ? error.message : 'I18N_INVENTORY_UNCONFIRMED'
  } finally {
    if (original && scope) {
      try {
        report.source_files_unchanged = Object.entries(original).every(([file, text]) => digest(readSource(file)) === digest(text)) && git(['rev-parse', 'HEAD']).trim() === scope.source_sha && git(['status', '--porcelain', '--untracked-files=no']).trim() === ''
      } catch { report.source_files_unchanged = false }
    }
    if (!report.source_files_unchanged) { report.passed = false; report.error_code ||= 'I18N_SOURCE_CHANGED' }
    if (reportFile) {
      try {
        const info = lstatSync(scope.directory, { bigint: true })
        demand(info.isDirectory() && !info.isSymbolicLink() && info.uid === BigInt(process.getuid()) && (info.mode & 0o777n) === 0o700n &&
          `${info.dev}:${info.ino}` === outputBinding && realpathSync(scope.directory) === scope.directory, 'I18N_REPORT_DIRECTORY_CHANGED')
        writeFileSync(reportFile, JSON.stringify(report, null, 2) + '\n', { flag: 'wx', mode: 0o600 })
      } catch { report.passed = false; report.error_code ||= 'I18N_REPORT_WRITE_FAILED'; reportFile = null }
    }
    process.umask(oldUmask)
  }
  logInventory(report)
  console.log(JSON.stringify({ type: 'i18n-inventory-result', schema_version: 1, passed: report.passed, translation_accepted: false,
    source_sha: report.source_sha, run_id: report.run_id, run_attempt: report.run_attempt, report: reportFile || null, error_code: report.error_code }))
  process.exitCode = report.passed ? 0 : 1
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await main()
