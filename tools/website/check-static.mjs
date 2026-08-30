/*
 * MyAPI static website validation.
 * Copyright (C) 2026 ForceMind
 *
 * Licensed under the GNU Affero General Public License version 3 or later.
 */
import { createHash } from 'node:crypto'
import { readFileSync, statSync } from 'node:fs'
import { resolve } from 'node:path'

const root = resolve(import.meta.dirname, '..', '..')
const website = resolve(root, 'website')

function read(name) {
  return readFileSync(resolve(website, name), 'utf8')
}

function requireFile(name) {
  const path = resolve(website, name)
  if (!statSync(path, { throwIfNoEntry: false })?.isFile()) {
    throw new Error(`website asset is missing: ${name}`)
  }
}

function sha256(path) {
  return createHash('sha256').update(readFileSync(path)).digest('hex')
}

const html = read('index.html')
const css = read('styles.css')
const script = read('script.js')
const publicWebsite = `${html}\n${css}\n${script}`

for (const name of ['index.html', 'styles.css', 'script.js', 'myapi-logo-v1.png']) {
  requireFile(name)
}

const maintainedLogo = resolve(root, 'web', 'public', 'myapi-logo-v1.png')
if (!statSync(maintainedLogo, { throwIfNoEntry: false })?.isFile()) {
  throw new Error('web/public/myapi-logo-v1.png is missing')
}
if (sha256(resolve(website, 'myapi-logo-v1.png')) !== sha256(maintainedLogo)) {
  throw new Error('website/myapi-logo-v1.png must match web/public/myapi-logo-v1.png')
}

if (!/<html[^>]+lang=["'](?:zh-CN|en)["']/i.test(html)) {
  throw new Error('website/index.html must declare a supported document language')
}
if (!/<meta[^>]+name=["']viewport["']/i.test(html)) {
  throw new Error('website/index.html must include a mobile viewport')
}
if (!/<script[^>]+src=["']\.\/script\.js["']/i.test(html)) {
  throw new Error('website/index.html must load the website interaction script')
}
if (!/<button[^>]+data-theme-toggle[^>]+aria-label=/i.test(html)) {
  throw new Error('theme toggle must expose an accessible label')
}
if (!/<button[^>]+data-menu-toggle[^>]+aria-expanded=["'](?:true|false)["']/i.test(html)) {
  throw new Error('mobile menu toggle must expose aria-expanded')
}
if (!/myapi-logo-v1\.png/.test(html)) {
  throw new Error('website/index.html must reference the MyAPI logo asset')
}
if (/QuantumNous|New API|NewAPI|github\.com\/QuantumNous\/new-api/i.test(publicWebsite)) {
  throw new Error('public website contains a legacy product or attribution reference')
}
if (!/theme|menu|install|release/i.test(script)) {
  throw new Error('website/script.js does not contain the expected website interactions')
}
if (!/aria-pressed/.test(script) || !/matchMedia/.test(script)) {
  throw new Error('website theme interaction must expose state and respect system preference')
}
if (css.trim().length < 200) {
  throw new Error('website/styles.css is unexpectedly small')
}
if (!/:focus-visible/.test(css)) {
  throw new Error('website/styles.css must define a visible keyboard focus state')
}

console.log('Website static check passed: structure, responsive metadata, assets, and interactions.')
