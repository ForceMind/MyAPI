/*
 * MyAPI static website validation.
 * Copyright (C) 2026 ForceMind
 *
 * Licensed under the GNU Affero General Public License version 3 or later.
 */
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

const html = read('index.html')
const css = read('styles.css')
const script = read('script.js')

for (const name of ['index.html', 'styles.css', 'script.js', 'myapi-logo-v1.png']) {
  requireFile(name)
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
if (!/myapi-logo-v1\.png/.test(html)) {
  throw new Error('website/index.html must reference the MyAPI logo asset')
}
if (!/theme|menu|install|release/i.test(script)) {
  throw new Error('website/script.js does not contain the expected website interactions')
}
if (css.trim().length < 200) {
  throw new Error('website/styles.css is unexpectedly small')
}

console.log('Website static check passed: structure, responsive metadata, assets, and interactions.')
