#!/usr/bin/env node

/*
 * Deterministic desktop distribution contract check.
 *
 * This is intentionally dependency-free and parser-light: it validates the
 * files and explicit strings that define the Electron macOS/Windows build
 * contract without installing Electron, invoking Go, contacting GitHub, or
 * starting an application. Real installer launches remain a platform-owner
 * acceptance step.
 */
import { existsSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const checks = []
const jsonOutput = process.argv.includes('--json')

function record(name, ok, detail = '') {
  checks.push({ name, ok: Boolean(ok), detail })
}

function read(relative) {
  const file = path.join(repositoryRoot, relative)
  record(`file: ${relative}`, existsSync(file))
  return existsSync(file) ? readFileSync(file, 'utf8') : ''
}

function includes(text, fragments) {
  return fragments.every((fragment) => text.includes(fragment))
}

function checkPackage() {
  const raw = read('electron/package.json')
  if (!raw) return
  let pkg
  try {
    pkg = JSON.parse(raw)
  } catch {
    record('Electron package.json is valid JSON', false)
    return
  }
  record('Electron package.json is valid JSON', true)
  record('desktop product is MyAPI', pkg.build?.productName === 'MyAPI' && pkg.build?.appId === 'com.forcemind.myapi')
  record('macOS targets include DMG and ZIP', includes(pkg.build?.mac?.target || [], ['dmg', 'zip']))
  record('Windows targets include NSIS and portable', includes(pkg.build?.win?.target || [], ['nsis', 'portable']))
  record('runtime preflight is packaged', (pkg.build?.files || []).includes('runtime-config.js'))
  const allResources = [pkg.build?.mac?.extraResources || [], pkg.build?.win?.extraResources || []]
    .flat()
    .map((resource) => resource.from)
  record('platform binaries are explicit', allResources.includes('../my-api') && allResources.includes('../my-api.exe'))
  record(
    'license resources are bundled',
    allResources.includes('../LICENSE') && allResources.includes('../NOTICE') && allResources.includes('../THIRD-PARTY-LICENSES.md'),
  )
  record('desktop build scripts are present', Boolean(pkg.scripts?.build && pkg.scripts?.['build:mac'] && pkg.scripts?.['build:win']))
  record('macOS and Windows build commands are defined', pkg.scripts?.['build:mac'] === 'electron-builder --mac' && pkg.scripts?.['build:win'] === 'electron-builder --win')
}

function checkWorkflow() {
  const workflow = read('.github/workflows/electron-build.yml')
  if (!workflow) return
  record('Electron workflow is tag/manual gated', workflow.includes("tags:\n      - 'v*.*.*'") && workflow.includes('workflow_dispatch:'))
  record('workflow matrix covers macOS and Windows', workflow.includes('macos-latest') && workflow.includes('windows-latest'))
  record('workflow validates exact SemVer tag commit', includes(workflow, ['Validate requested tag', 'refs/tags/$BUILD_TAG^{commit}', 'checked out commit does not match tag']))
  record('workflow builds frontend and native binary per platform', includes(workflow, ['Build frontend', 'Build Go binary (macOS)', 'Build Go binary (Windows)', 'go build']))
  record('workflow builds platform installers', includes(workflow, ['npm run build:mac', 'npm run build:win']))
  record('workflow emits SHA256 checksums', workflow.includes('SHA256SUMS-${process.env.RUNNER_OS}.txt') && workflow.includes("createHash('sha256')"))
  record('workflow uploads separate platform artifacts', workflow.includes('name: macos-build') && workflow.includes('name: windows-build'))
  record('release upload has explicit approval gates', includes(workflow, ["inputs.confirm == 'PUBLISH'", "vars.MYAPI_ENABLE_RELEASE == 'true'", "inputs.tag != ''"]))
}

function checkDocs() {
  const lan = read('docs/LAN_LITE.md')
  const master = read('docs/MYAPI_MASTER_PLAN.md')
  record('LAN guide documents platform data locations', includes(lan, ['macOS:', 'Windows:', 'Cross-platform acceptance']))
  record('LAN guide distinguishes parser checks from real-device rehearsal', includes(lan, ['does not prove that macOS/Windows firewall rules', 'real macOS and Windows rehearsal']))
  record(
    'master plan keeps real platform rehearsal as an explicit gate',
    master.includes('真实跨平台安装/局域网请求演练和系统防火墙自动配置仍待完成') ||
      master.includes('跨平台安装演练和系统防火墙自动配置仍待完成'),
  )
}

checkPackage()
checkWorkflow()
checkDocs()

const failed = checks.filter((check) => !check.ok)
if (jsonOutput) {
  console.log(JSON.stringify({ command: 'desktop:check', passed: failed.length === 0, checks }, null, 2))
} else {
  for (const check of checks) console.log(`${check.ok ? 'PASS' : 'FAIL'}  ${check.name}${check.detail ? ` (${check.detail})` : ''}`)
  console.log(`Desktop distribution contract: ${failed.length === 0 ? 'PASS' : 'FAIL'} (${checks.length - failed.length}/${checks.length})`)
}
if (failed.length > 0) process.exitCode = 1
