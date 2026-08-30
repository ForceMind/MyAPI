#!/usr/bin/env node

/*
 * MyAPI brand and legacy-reference audit.
 * Copyright (C) 2026 ForceMind
 *
 * This check reads only tracked source files and never opens .env files,
 * databases, logs, node_modules, or build output. Legal and compatibility
 * references are reported with their category; unexplained legacy branding in
 * public surfaces fails the command.
 */
import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const jsonOutput = process.argv.includes('--json')

const legalPath = /^(?:LICENSE(?:\..*)?|NOTICE|THIRD-PARTY-LICENSES(?:\/|$))/i
const compatibilityPath = /(?:^|\/)(?:patches|cli\/templates)(?:\/|$)|(?:^|\/)(?:new-api\.service|makefile|docker-compose\.dev\.yml)$/i
const sourceHeaderPath = /^web\/(?:src|scripts)\//i
const auditToolPath = /^tools\/(?:website|branding)\//i
const auditDocPath = /^docs\/(?:BRAND_AUDIT|MYAPI_MASTER_PLAN|TOKENHUB_INTEGRATION|ANTIGRAVITY_INTEGRATION)\.md$/i
const compatibilityWirePath = /(?:^|\/)(?:authentication\.md|openapi\/)|(?:^|\/)(?:middleware\/cors\.go|model\/subscription\.go)$/i
const publicPath = /^(?:README(?:\.[^/]+)?\.md|package\.json|Dockerfile[^/]*|docker-compose\.yml|deploy\/|website\/|web\/public\/|cli\/myapi\.mjs)/i

const rules = [
  { name: 'QuantumNous', pattern: /quantumnous/i },
  { name: 'legacy project name', pattern: /\bNew API\b|\bNewAPI\b/i },
  { name: 'legacy repository link', pattern: /github\.com\/QuantumNous\/new-api/i },
  { name: 'legacy machine identifier', pattern: /\bnew-api\b/i },
]

// Compatibility identifiers may remain in migration paths, but a new
// development instance must never silently select an old product database.
// Keep these checks explicit because docker-compose.dev.yml and makefile are
// otherwise classified as compatibility surfaces by the generic scanner.
const legacyDevelopmentDatabaseDefaults = [
  {
    file: 'docker-compose.dev.yml',
    patterns: [
      /SQL_DSN=.*\/new-api(?:\s|$)/m,
      /^\s*POSTGRES_DB:\s*new-api\s*$/m,
      /pg_isready[^\n]*-d\s+new-api(?:['"\s]|$)/m,
    ],
  },
  {
    file: 'makefile',
    patterns: [/^DEV_POSTGRES_DB\s*\?=\s*new-api\s*$/m],
  },
  {
    files: [
      'README.md',
      'README.en.md',
      'README.fr.md',
      'README.ja.md',
      'README.zh_CN.md',
      'README.zh_TW.md',
    ],
    patterns: [/SQL_DSN=.*\/oneapi(?:["'\s]|$)/m],
  },
]

function categoryFor(relative) {
  if (legalPath.test(relative)) return 'legal'
  if (compatibilityPath.test(relative) || compatibilityWirePath.test(relative)) return 'compatibility'
  if (sourceHeaderPath.test(relative)) return 'source-header'
  if (auditToolPath.test(relative)) return 'audit-tool'
  if (auditDocPath.test(relative)) return 'audit-document'
  if (publicPath.test(relative)) return 'public'
  return 'manual-review'
}

function trackedFiles() {
  const output = execFileSync('git', ['ls-files', '-z'], { cwd: root, encoding: 'utf8' })
  return output.split('\0').filter(Boolean)
}

function stripSourceHeader(contents, category) {
  if (category !== 'source-header') return contents
  return contents.replace(/^(?:\/\*[\s\S]*?\*\/\s*)+/, '')
}

function audit() {
  const findings = []
  const tracked = new Set(trackedFiles())
  for (const relative of tracked) {
    const category = categoryFor(relative)
    const contents = stripSourceHeader(readFileSync(path.join(root, relative), 'utf8'), category)
    for (const rule of rules) {
      if (!rule.pattern.test(contents)) continue
      const blocking = category === 'public'
        ? rule.name !== 'legacy machine identifier'
        : false
      findings.push({ file: relative, category, rule: rule.name, blocking })
    }
  }
  for (const check of legacyDevelopmentDatabaseDefaults) {
    const files = check.files ?? [check.file]
    for (const relative of files) {
      if (!tracked.has(relative)) continue
      const contents = readFileSync(path.join(root, relative), 'utf8')
      if (check.patterns.some((pattern) => pattern.test(contents))) {
        findings.push({
          file: relative,
          category: 'public-default',
          rule: 'legacy development database default',
          blocking: true,
        })
      }
    }
  }
  return findings
}

try {
  const findings = audit()
  const blocking = findings.filter((finding) => finding.blocking)
  const result = {
    command: 'brand:check',
    passed: blocking.length === 0,
    tracked_files: trackedFiles().length,
    findings,
    blocking_count: blocking.length,
  }
  if (jsonOutput) {
    console.log(JSON.stringify(result, null, 2))
  } else {
    for (const finding of findings) {
      console.log(`${finding.blocking ? 'FAIL' : 'INFO'}  ${finding.category}  ${finding.file}  ${finding.rule}`)
    }
    console.log(`Brand audit: ${result.passed ? 'PASS' : 'FAIL'} (${findings.length} classified references, ${blocking.length} blocking)`)
  }
  if (!result.passed) process.exitCode = 1
} catch (error) {
  console.error(`Brand audit: ${error instanceof Error ? error.message : String(error)}`)
  process.exitCode = 1
}
