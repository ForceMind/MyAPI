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
const browserSmoke = resolve(root, 'tools', 'website', 'browser-smoke.mjs')
const browserWorkflow = resolve(root, '.github', 'workflows', 'website-browser.yml')
const artifactWorkflow = resolve(root, '.github', 'workflows', 'website-artifact.yml')
if (!statSync(browserSmoke, { throwIfNoEntry: false })?.isFile()) {
  throw new Error('website browser smoke script is missing')
}
if (!statSync(browserWorkflow, { throwIfNoEntry: false })?.isFile()) {
  throw new Error('website browser smoke workflow is missing')
}
if (!statSync(artifactWorkflow, { throwIfNoEntry: false })?.isFile()) {
  throw new Error('website artifact workflow is missing')
}
const browserSmokeSource = readFileSync(browserSmoke, 'utf8')
const artifactWorkflowSource = readFileSync(artifactWorkflow, 'utf8')
if (!/resolve\(root,/.test(browserSmokeSource) || !/startsWith\(`\$\{root\}\$\{sep\}`\)/.test(browserSmokeSource)) {
  throw new Error('website browser smoke path guard must enforce a directory boundary')
}
if (!browserSmokeSource.includes(".replace(/[\\\\/]+$/, '')")) {
  throw new Error('website browser smoke root must not retain a trailing separator')
}
if (!/push:\s+[\s\S]*?branches:\s*- main/.test(artifactWorkflowSource) || !/workflow_dispatch:/.test(artifactWorkflowSource)) {
  throw new Error('website artifact workflow must support main pushes and manual dispatch')
}
if (!/decodeURIComponent[\s\S]*catch/.test(browserSmokeSource)) {
  throw new Error('website browser smoke must reject malformed URL encoding')
}
if (!browserSmokeSource.includes('width: 320') || !browserSmokeSource.includes('horizontal overflow')) {
  throw new Error('website browser smoke must cover the narrow 320px viewport')
}
if (!readFileSync(browserWorkflow, 'utf8').includes('artifacts/myapi-website-mobile*.png')) {
  throw new Error('website browser workflow must upload all mobile screenshots')
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
if (!/<button[^>]+data-menu-toggle[^>]+aria-haspopup=["']true["']/i.test(html)) {
  throw new Error('mobile menu toggle must expose aria-haspopup')
}
if (!/myapi-logo-v1\.png/.test(html)) {
  throw new Error('website/index.html must reference the MyAPI logo asset')
}
if (/QuantumNous|New API|NewAPI|github\.com\/QuantumNous\/new-api/i.test(publicWebsite)) {
  throw new Error('public website contains a legacy product or attribution reference')
}
if (!/theme|menu|install|release/i.test(script) || !/pointerdown/.test(script) || !/event.key !== 'Tab'/.test(script)) {
  throw new Error('website/script.js does not contain the expected website interactions')
}
if (!/aria-pressed/.test(script) || !/matchMedia/.test(script)) {
  throw new Error('website theme interaction must expose state and respect system preference')
}
if (!/docs\/LAN_LITE\.md/.test(script)) {
  throw new Error('LAN Lite CTA must link to the maintained LAN guide')
}
if (/\bdocker\s+run\b|:\s*latest\b/i.test(publicWebsite)) {
  throw new Error('public website must not advertise unpinned or bare Docker commands')
}
if (css.trim().length < 200) {
  throw new Error('website/styles.css is unexpectedly small')
}
if (!/:focus-visible/.test(css)) {
  throw new Error('website/styles.css must define a visible keyboard focus state')
}
if (!/scroll-margin-top/.test(css)) {
  throw new Error('website/styles.css must offset fixed-header anchor targets')
}
if (!/\.light-preview[^}]*--surface-strong/s.test(css) || !/\.light-preview \.problem-grid article/.test(css)) {
  throw new Error('website/styles.css must cover light-theme surface variables and cards')
}

console.log('Website static check passed: structure, responsive metadata, assets, and interactions.')
