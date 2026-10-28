import assert from 'node:assert/strict'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import test from 'node:test'

const workflow = readFileSync(new URL('../../.github/workflows/docker-smoke.yml', import.meta.url), 'utf8')
const producer = workflow.split('  prepare-smoke-images:\n')[1].split('  installer-local-build:\n')[0]
const consumer = workflow.split('  build-and-healthcheck:\n')[1]
const installer = workflow.split('  installer-local-build:\n')[1].split('  build-and-healthcheck:\n')[0]
function shellStep(name) {
  const step = producer.split(`      - name: ${name}\n`)[1].split(/\n      - name:|\n  [a-z]/)[0]
  return step.split('        run: |\n')[1].split('\n').map(line => line.replace(/^          /, '')).join('\n')
}
function outcomes(full, lan, manual = true) {
  return { FULL_BUILD: full[0], FULL_EXPORT: full[1], FULL_UPLOAD: full[2], FULL_MANIFEST: 'a'.repeat(64),
    LAN_BUILD: lan[0], LAN_EXPORT: lan[1], LAN_UPLOAD: lan[2], LAN_MANIFEST: 'b'.repeat(64),
    GITHUB_EVENT_NAME: manual ? 'workflow_dispatch' : 'pull_request' }
}

test('one current build per edition shares only immutable bundle bytes across independent writer jobs', () => {
  assert.equal((producer.match(/uses: docker\/build-push-action@/g) || []).length, 2)
  assert.match(producer, /id: build-lan\n[\s\S]*?if: github.event_name == 'workflow_dispatch'/)
  assert.equal((producer.match(/docker pull --platform linux\/amd64 oven\/bun:/g) || []).length, 1)
  assert.equal((producer.match(/docker pull --platform linux\/amd64 redis:/g) || []).length, 1)
  assert.doesNotMatch(producer, /docker (?:run|create|commit|push|login)\b|SESSION_SECRET|REDIS_CONN_STRING/)
  assert.match(producer, /timeout-minutes: 50/)
  assert.equal((producer.match(/timeout-minutes: 20/g) || []).length, 2)
  assert.match(producer, /cache-from: type=gha,scope=smoke-full/)
  assert.match(producer, /cache-from: type=gha,scope=smoke-lan/)
  assert.match(consumer, /needs: prepare-smoke-images/)
  assert.match(consumer, /runs-on: ubuntu-latest/)
  assert.match(consumer, /max-parallel: 1/)
  assert.match(consumer, /fail-fast: false/)
  assert.match(consumer, /"scenario":"fresh","writer":"authoritative"/)
  assert.doesNotMatch(consumer, /Build local smoke image|docker pull/)
  assert.match(consumer, /name: Set up Docker Buildx\n\s+if: matrix.scenario == 'handoff'/)
  assert.match(installer, /run: bash deploy\/install.sh/)
  assert.doesNotMatch(installer, /download-artifact|needs:/)
})

test('consumer uses exact original producer identity across failed-job reruns and never a latest artifact', () => {
  assert.match(producer, /prefix=myapi-smoke-images-\$GITHUB_RUN_ID-\$GITHUB_RUN_ATTEMPT-\$GITHUB_SHA/)
  assert.match(consumer, /name: \$\{\{ needs.prepare-smoke-images.outputs.prefix \}\}-\$\{\{ matrix.edition \}\}/)
  assert.match(consumer, /--producer-attempt "\$\{\{ needs.prepare-smoke-images.outputs.attempt \}\}"/)
  assert.match(consumer, /--manifest-sha256 "\$BUNDLE_MANIFEST_SHA"/)
  assert.match(consumer, /--tree-sha "\$\(git rev-parse 'HEAD\^\{tree\}'\)"/)
  assert.doesNotMatch(consumer.split('      # Historical source')[0], /pattern:|merge-multiple:|github-token:|run-id:|overwrite:/)
  assert.match(producer, /full-manifest: \$\{\{ steps.publish.outputs.full_manifest_sha256 \}\}/)
  assert.match(producer, /lan-manifest: \$\{\{ steps.publish.outputs.lan_manifest_sha256 \}\}/)
  assert.equal((producer.match(/retention-days: \$\{\{ inputs.api_only && 7 \|\| 30 \}\}/g) || []).length, 2)
  assert.equal((producer.match(/if-no-files-found: error/g) || []).length, 2)
  for (const edition of ['full', 'lan']) {
    assert.match(producer, new RegExp(`smoke-bundle-${edition}/manifest\\.json`))
    assert.match(producer, new RegExp(`smoke-bundle-${edition}/images\\.tar`))
  }
})

test('continue-on-error cannot turn any required build export or upload failure into producer success', t => {
  const temp = mkdtempSync(path.join(os.tmpdir(), 'myapi-bundle-outcomes-'))
  t.after(() => rmSync(temp, { recursive: true, force: true }))
  const output = path.join(temp, 'out'), summary = path.join(temp, 'summary')
  const good = ['success', 'success', 'success'], absent = ['skipped', 'skipped', 'skipped']
  const cases = [
    [outcomes(good, good), true, true, true],
    [outcomes(good, absent, false), true, true, false],
    [outcomes(good, ['failure', 'skipped', 'skipped']), false, true, false],
    [outcomes(['failure', 'skipped', 'skipped'], good), false, false, true],
    [outcomes(['success', 'failure', 'skipped'], good), false, false, true],
    [outcomes(['success', 'success', 'failure'], good), false, false, true],
    [outcomes(absent, absent), false, false, false],
    [{ ...outcomes(good, absent), BUNDLE_API_ONLY: '1' }, true, true, false],
    [{ ...outcomes(['failure', 'skipped', 'skipped'], absent), BUNDLE_API_ONLY: '1' }, false, false, false],
    [{ ...outcomes(['success', 'failure', 'skipped'], absent), BUNDLE_API_ONLY: '1' }, false, false, false],
    [{ ...outcomes(['success', 'success', 'failure'], absent), BUNDLE_API_ONLY: '1' }, false, false, false],
  ]
  for (const [extra, passed, full, lan] of cases) {
    writeFileSync(output, ''); writeFileSync(summary, '')
    const env = { ...process.env, ...extra, GITHUB_OUTPUT: output, GITHUB_STEP_SUMMARY: summary }
    assert.equal(spawnSync('bash', ['-c', shellStep('Publish only successfully uploaded edition identities')], { env }).status, 0)
    assert.equal(readFileSync(output, 'utf8').includes('full_manifest_sha256='), full)
    assert.equal(readFileSync(output, 'utf8').includes('lan_manifest_sha256='), lan)
    assert.equal(spawnSync('bash', ['-c', shellStep('Require every requested edition bundle')], { env }).status === 0, passed)
  }
  assert.match(consumer, /if: always\(\) && !cancelled\(\)/)
  assert.match(consumer, /outputs.full-manifest != '' \|\| needs.prepare-smoke-images.outputs.lan-manifest != ''/)
  assert.doesNotMatch(producer, /steps\.[\w-]+\.conclusion/)
})

test('API-only deployment requests Full fresh legacy only and rejects private or browser-capable runs before build', t => {
  assert.match(workflow, /api_only:\n[\s\S]*?default: false/)
  assert.match(workflow, /run_browser:\n[\s\S]*?default: false/)
  assert.match(producer, /if: github.event_name == 'workflow_dispatch' && !inputs.api_only/)
  assert.match(installer, /&& !inputs.api_only/)
  assert.match(consumer, /\(github.event_name == 'pull_request' \|\| inputs.api_only\) && '\["full"\]'/)
  assert.match(consumer, /include:.*inputs.api_only && '\[\]'/)
  assert.match(consumer, /if: matrix.scenario == 'fresh' && env.MYAPI_SMOKE_BROWSER == '1'/)
  assert.match(consumer, /MYAPI_SMOKE_API_ONLY:/)
  assert.match(consumer, /MYAPI_SMOKE_RESTORE:.*'0' \|\| '1'/)
  const temp = mkdtempSync(path.join(os.tmpdir(), 'myapi-api-only-identity-'))
  t.after(() => rmSync(temp, { recursive: true, force: true }))
  const source = spawnSync('git', ['-c', 'core.fileMode=false', 'rev-parse', 'HEAD'], { encoding: 'utf8' })
  assert.equal(source.status, 0)
  const output = path.join(temp, 'output')
  for (const [changes, expected] of [[{}, true], [{ PUBLIC_REPOSITORY: '0' }, false],
    [{ RUN_BROWSER: '1' }, false], [{ REQUEST_PERSONAL: '1' }, false]]) {
    writeFileSync(output, '')
    const env = { ...process.env, BUNDLE_API_ONLY: '1', PUBLIC_REPOSITORY: '1', RUN_BROWSER: '0', REQUEST_PERSONAL: '0',
      GITHUB_SHA: source.stdout.trim(), GITHUB_RUN_ID: '123', GITHUB_RUN_ATTEMPT: '1', GITHUB_OUTPUT: output, ...changes }
    const result = spawnSync('bash', ['-c', shellStep('Bind exact source and producer attempt')], { env })
    assert.equal(result.status === 0, expected)
    assert.equal(readFileSync(output, 'utf8').includes('prefix=myapi-smoke-images-'), expected)
  }
})

test('push CI never installs or starts browsers without an explicit manual opt-in', () => {
  const ci = readFileSync(new URL('../../.github/workflows/ci.yml', import.meta.url), 'utf8')
  assert.match(ci, /run_browser:\n[\s\S]*?type: boolean\n\s+default: false/)
  const dependencies = ci.split('      - name: Install isolated browser test dependencies\n')[1].split('      - name:')[0]
  assert.match(dependencies, /if:.*github.event_name == 'workflow_dispatch' && inputs.run_browser/)
  const credential = ci.split('  credential-maintenance:\n')[1]
  assert.match(credential, /if:.*github.event_name == 'workflow_dispatch' && inputs.run_browser/)
  for (const name of ['Qualify advanced personal maintenance in Chromium', 'Verify real quota charts and responsive interactions',
    'Verify model discovery and routing workflow in real Chromium', 'Verify assigned access and account events in real Chromium',
    'Qualify personal console pages and native-window shares in Chromium', 'Qualify shared UI, roles, settings and responsive pages in Chromium']) {
    const step = ci.split(`      - name: ${name}\n`)[1].split('      - name:')[0]
    assert.match(step, /if:.*steps.browser-dependencies.outcome == 'success'/)
  }
  assert.match(ci, /浏览器未验证/)
})

test('consumer runtime cannot implicitly pull and preserves isolated source restore and personal cleanup', () => {
  const runOrCreate = consumer.split('\n').filter(line => /docker (run|create)\b/.test(line))
  assert.equal(runOrCreate.length, 3)
  for (const line of runOrCreate) assert.match(line, /--pull=never/)
  assert.match(consumer, /"\$SMOKE_BUN_IMAGE_ID"/)
  assert.match(consumer, /"\$SMOKE_REDIS_IMAGE_ID"/)
  assert.match(consumer, /== "\$SMOKE_APP_IMAGE_ID"/)
  assert.match(consumer, /127\.0\.0\.1:18080:3000/)
  assert.match(consumer, /127\.0\.0\.1:19090:19090/)
  assert.match(consumer, /timeout-minutes: \$\{\{ env.MYAPI_SMOKE_PERSONAL == '1' && 6 \|\| 25 \}\}/)
  assert.match(consumer, /name: Remove only owned personal Redis and network\n\s+if: always\(\)/)
  assert.match(consumer, /name: Remove smoke containers\n\s+if: always\(\)/)
  const restore = readFileSync(new URL('./sqlite-restore-smoke.mjs', import.meta.url), 'utf8')
  assert.match(restore, /\['run', '--pull=never', '--detach'/)
})
