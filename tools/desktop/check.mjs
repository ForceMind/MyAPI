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
  record('desktop lifecycle, native translations and local panel are packaged', includes(pkg.build?.files || [], [
    'service-controller.js', 'service-diagnostics.js', 'service-panel-ipc.js', 'service-panel-preload.js',
    'service-panel.html', 'service-panel.js', 'service-panel.css', 'public-access-config.js', 'public-access-check.js',
    'native-i18n.js', 'locales/en.json', 'locales/zh.json',
  ]))
  record('desktop publishing and macOS signing remain disabled by default', pkg.build?.publish === null && pkg.build?.mac?.identity === null)
  record('desktop does not add an auto-updater dependency', !pkg.dependencies?.['electron-updater'] && !pkg.devDependencies?.['electron-updater'])
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
  const ciWorkflow = read('.github/workflows/ci.yml')
  if (!workflow) return
  record('Electron workflow is tag/manual gated', workflow.includes("tags:\n      - 'v*.*.*'") && workflow.includes('workflow_dispatch:'))
  record('workflow matrix covers macOS and Windows', workflow.includes('macos-latest') && workflow.includes('windows-latest'))
  record('manual builds can select macOS before Windows without changing the default', includes(workflow, ['platform:', 'default: both', 'options: [both, macos, windows]', "inputs.platform == 'macos'", "inputs.platform == 'windows'"]))
  record(
    'workflow validates exact SemVer tag commit and version files',
    includes(workflow, [
      'Validate requested tag',
      'EVENT_NAME: ${{ github.event_name }}',
      'GITHUB_REF_NAME',
      'refs/tags/$BUILD_TAG^{commit}',
      'checked out commit does not match tag',
      'FILE_VERSION',
      'PACKAGE_VERSION',
      'VERSION and package.json must match',
    ]),
  )
  record('workflow builds frontend and native binary per platform', includes(workflow, ['Build frontend', 'Build Go binary (macOS)', 'Build Go binary (Windows)', 'GOMAXPROCS=2 go build -p 2']))
  record('workflow builds platform installers', includes(workflow, ['npm run build:mac', 'npm run build:win']))
  record('workflow emits SHA256 checksums', workflow.includes('SHA256SUMS-${process.env.RUNNER_OS}.txt') && workflow.includes("createHash('sha256')"))
  record('workflow uploads separate platform artifacts', workflow.includes('name: macos-build') && workflow.includes('name: windows-build'))
  record('release upload has explicit approval gates', includes(workflow, ["inputs.confirm == 'PUBLISH'", "vars.MYAPI_ENABLE_RELEASE == 'true'", "inputs.tag != ''"]))
  record(
    'workflow runs Electron runtime tests',
    workflow.includes('node --test electron/test/*.test.mjs') ||
      ciWorkflow.includes('node --test electron/test/*.test.mjs'),
  )
}

function checkBuildInputs() {
  const helper = read('electron/build.sh')
  const workflow = read('.github/workflows/electron-build.yml')
  const version = read('VERSION').trim()
  const root = JSON.parse(read('package.json'))
  record('desktop root VERSION matches the root package version', version === root.version)
  for (const [name, source] of [['local helper', helper], ['Actions', workflow]]) {
    record(`${name} compiles the Lite minimal frontend and canonical brand`, includes(source, ["VITE_SELF_USE_MINIMAL='true'", "VITE_BRAND_NAME='MyAPI'", "VITE_BRAND_LOGO='/myapi-logo-v1.png'"]))
    record(`${name} embeds the checked-out source identity`, source.includes('git -c core.fileMode=false rev-parse HEAD') && source.includes('VITE_BUILD_ID=') && !source.includes('VITE_BUILD_ID=${{ github.sha }}'))
    record(`${name} uses root VERSION for frontend, backend and effective Electron version`, includes(source, ['< VERSION', 'VITE_REACT_APP_VERSION=', '/common.Version=', '--config.extraMetadata.version=']))
  }
  record('Actions build identity comes from the resolved checkout, not the trigger event', workflow.includes("VITE_BUILD_ID='${{ steps.source.outputs.sha }}'") && workflow.includes('echo "version=$FILE_VERSION"'))
  record('Electron packaging does not rewrite package or lock versions', !workflow.includes('npm version ') && helper.includes('MYAPI_DESKTOP_VERSION'))
}

function checkRuntime() {
  const main = read('electron/main.js')
  const runtime = read('electron/runtime-config.js')
  record(
    'packaged UI follows the effective LAN bind host',
    main.includes('getHealthCheckAddress(BIND_ADDRESS)') &&
      main.includes('mainWindow.loadURL(`http://${loadHost}:${loadPort}`)'),
  )
  record(
    'wildcard LAN status exposes discovered endpoints without placeholders',
    runtime.includes('getPrivateIPv4Candidates(networkInterfaces)') &&
      runtime.includes('No RFC1918 address detected') &&
      !main.includes("'<private-LAN-IP>'"),
  )
  record(
    'production startup probes the backend status endpoint and rejects non-2xx or unsuccessful JSON',
    /checkServerAvailability\(\s*PORT,\s*30,\s*1000,\s*healthCheckHost,\s*['"]\/api\/status['"],\s*\{\s*signal,?\s*\},?\s*\)/.test(main) &&
      main.includes('isSuccessfulHttpStatus(statusCode)') &&
      main.includes('isSuccessfulStatusResponse(statusCode, responseBody)') &&
      runtime.includes('function isSuccessfulHttpStatus(statusCode)') &&
      runtime.includes('function isSuccessfulStatusResponse(statusCode, body)'),
  )
  record(
    'production logs stay under the writable Electron userData root',
    /const logsDir = path\.join\(userDataPath, ['"]logs['"]\)/.test(main) &&
      /FULL_CONTENT_LOG_DIR: path\.join\(logsDir, ['"]full-content['"]\)/.test(main) &&
      /spawn\(binaryPath, \[['"]--log-dir['"], logsDir\]/.test(main),
  )
}

function checkDocs() {
  const lan = read('docs/LAN_LITE.md')
  const master = read('docs/MYAPI_MASTER_PLAN.md')
  record(
    'LAN guide documents platform data locations',
    includes(lan, ['macOS:', 'Windows:']) &&
      (lan.includes('Cross-platform acceptance') || lan.includes('跨平台合同验收')),
  )
  record('LAN guide distinguishes parser checks from real-device rehearsal', includes(lan, ['does not prove that macOS/Windows firewall rules', 'real macOS and Windows rehearsal']))
  // The gate is that the plan still calls real-device work outstanding. Accept
  // any wording that says so; reject a plan that quietly drops the claim.
  record(
    'master plan keeps real platform rehearsal as an explicit gate',
    master.includes('真实跨平台安装/局域网请求演练和系统防火墙自动配置仍待完成') ||
      master.includes('跨平台安装演练和系统防火墙自动配置仍待完成') ||
      master.includes('真实安装/网络验证仍待完成'),
  )
}

checkPackage()
checkWorkflow()
checkBuildInputs()
checkRuntime()
checkDocs()

const failed = checks.filter((check) => !check.ok)
if (jsonOutput) {
  console.log(JSON.stringify({ command: 'desktop:check', passed: failed.length === 0, checks }, null, 2))
} else {
  for (const check of checks) console.log(`${check.ok ? 'PASS' : 'FAIL'}  ${check.name}${check.detail ? ` (${check.detail})` : ''}`)
  console.log(`Desktop distribution contract: ${failed.length === 0 ? 'PASS' : 'FAIL'} (${checks.length - failed.length}/${checks.length})`)
}
if (failed.length > 0) process.exitCode = 1
