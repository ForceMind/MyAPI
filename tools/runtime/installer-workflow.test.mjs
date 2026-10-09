import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

test('candidate metadata and fresh image defaults name the same immutable version', () => {
  const read = (file) => readFileSync(new URL(`../../${file}`, import.meta.url), 'utf8')
  const version = read('VERSION').trim()
  assert.equal(JSON.parse(read('package.json')).version, version)
  const image = `ghcr.io/forcemind/myapi:v${version}`
  for (const file of ['docker-compose.yml', 'deploy/docker-compose.yml']) {
    assert.ok(read(file).includes(`image: \${MYAPI_IMAGE:-\${NEW_API_IMAGE:-${image}}}`), `${file} keeps explicit and legacy image overrides before the versioned fallback`)
  }
  assert.ok(read('deploy/.env.example').split('\n').includes(`MYAPI_IMAGE=${image}`))
})

test('settlement patch keeps bounded Docker acceptance and durable race coverage', () => {
  const docker = readFileSync(new URL('../../.github/workflows/docker-smoke.yml', import.meta.url), 'utf8')
  const gates = docker.split('\n').filter(line => line.startsWith('    if:'))
  assert.equal(gates.length, 2)
  for (const [index, gate] of gates.entries()) {
    assert.match(gate, /github\.event_name == 'workflow_dispatch'/)
    assert.match(gate, /head\.repo\.full_name == github\.repository && contains/)
    const allowed = JSON.parse(gate.match(/fromJSON\('([^']+)'\)/)[1])
    assert.deepEqual(allowed, ['codex/r1-usage-review-20261002', 'codex/settlement-review-status-20261009', ...(index === 1 ? ['codex/personal-app-journey-20261009'] : [])])
  }
  assert.doesNotMatch(docker, /packages: write|contents: write|id-token: write|push: true|secrets\./)
  assert.match(docker, /push: false/)
  const ci = readFileSync(new URL('../../.github/workflows/ci.yml', import.meta.url), 'utf8')
  const raceLine = ci.split('\n').find(line => line.includes('go test -race') && line.includes('TokenBudgetChat'))
  const selected = new RegExp(raceLine.match(/-run '([^']+)'/)[1])
  for (const name of [
    'TestChatStreamTerminalCancellationSettlementBothWriters',
    'TestStrictChatCancellationTerminalWriteBoundary',
    'TestStrictChatCompletedCancelledSettlementConcurrentReplay',
    'TestBillingOperationContextRemainsBoundedAfterClientCancellation',
    'TestQuotaBatchStartupConfiguration',
    'TestIncidentSyntheticZeroReserveBatchWithoutRedis',
    'TestUsageReviewSettlementReadProjection',
    'TestUsageReviewZeroReserveSettlementJournal',
    'TestTokenBudgetRecoveryRejectsAppliedAmountBeforePrepare',
  ]) assert.ok(selected.test(name), `${name} must remain selected for race regression`)
  assert.match(raceLine, /-count=1 -timeout=180s/)
})

test('funding admission regressions remain selected by the existing bounded race command', () => {
  const ci = readFileSync(new URL('../../.github/workflows/ci.yml', import.meta.url), 'utf8')
  const raceLine = ci.split('\n').find(line => line.includes('go test -race') && line.includes('TokenBudgetChat'))
  const selected = new RegExp(raceLine.match(/-run '([^']+)'/)[1])
  for (const name of [
    'TestSubscriptionTransactionPreConsumeAdmissionErrorIdentity',
    'TestLegacyBillingSessionSubscriptionErrorTextCannotAuthorizeWalletFallback',
    'TestLegacyBillingSessionWrappedSubscriptionAdmissionErrors',
    'TestLegacyBillingSessionSubscriptionAdmissionPreferences',
    'TestBillingSessionSubscriptionStorageFailureAcrossWriters',
    'TestLegacyBillingSessionSubscriptionOverflowToWallet',
    'TestLegacyBillingSessionSubscriptionOverflowBlocked',
  ]) assert.ok(selected.test(name), `${name} must be selected for race regression`)
  assert.match(raceLine, /-count=1 -timeout=180s/)
})

test('source installer smoke exercises the real script without publishing or borrowing an existing instance', () => {
  const workflow = readFileSync(new URL('../../.github/workflows/docker-smoke.yml', import.meta.url), 'utf8')
  const job = workflow.split('  installer-local-build:\n')[1]?.split('  build-and-healthcheck:\n')[0]
  assert.ok(job)
  assert.match(job, /persist-credentials: false/)
  assert.match(job, /head\.repo\.full_name == github\.repository/)
  assert.match(job, /codex\/r1-usage-review-20261002/)
  assert.match(job, /timeout-minutes: 25/)
  assert.match(job, /\[\[ ! -e deploy\/\.env \]\]/)
  assert.match(job, /Refusing to replace an existing installer target/)
  assert.match(job, /umask 077/)
  assert.match(job, /MYAPI_BUILD_LOCAL=true/)
  assert.match(job, /MYAPI_BIND_ADDRESS=127\.0\.0\.1/)
  assert.match(job, /run: bash deploy\/install\.sh/)
  assert.match(job, /probeFreshSQLite/)
  assert.match(job, /edition: 'lan'/)
  assert.match(job, /com\.docker\.compose\.project/)
  assert.match(job, /== "\$COMPOSE_PROJECT_NAME"/)
  assert.match(job, /\.State\.Health\.Status/)
  assert.match(job, /\.HostConfig\.Memory/)
  assert.match(job, /\.HostConfig\.NanoCpus/)
  assert.doesNotMatch(job, /docker (?:push|login)|packages: write|upload-artifact|--privileged|--network[= ]host/)
  // runner.temp is not a valid context in a job-level env block.
  assert.doesNotMatch(job.split('    steps:')[0], /runner\.temp/)
  for (const file of ['deploy/install.sh', 'deploy/docker-compose.yml', 'deploy/.env.example']) {
    assert.ok(workflow.split('permissions:')[0].includes(`'${file}'`))
  }
})

test('cache-free and Redis-backed templates declare compatible explicit batch choices', () => {
  const deploy = readFileSync(new URL('../../deploy/docker-compose.yml', import.meta.url), 'utf8')
  const example = readFileSync(new URL('../../deploy/.env.example', import.meta.url), 'utf8')
  assert.match(example, /^BATCH_UPDATE_ENABLED=false$/m)
  assert.match(example, /^MYAPI_ACCOUNTING_CONFIG_VERSION=1$/m)
  assert.match(deploy, /MYAPI_ACCOUNTING_CONFIG_VERSION: \$\{MYAPI_ACCOUNTING_CONFIG_VERSION:\?[^}]+\}/)
  assert.match(deploy, /BATCH_UPDATE_ENABLED: \$\{BATCH_UPDATE_ENABLED:\?[^}]+\}/)
  assert.doesNotMatch(deploy, /BATCH_UPDATE_ENABLED:\s*"true"|BATCH_UPDATE_ENABLED:-false/)
  assert.doesNotMatch(deploy, /REDIS_CONN_STRING:/)
  for (const filename of ['docker-compose.yml', 'docker-compose.dev.yml']) {
    const source = readFileSync(new URL(`../../${filename}`, import.meta.url), 'utf8')
    assert.match(source, /BATCH_UPDATE_ENABLED=true/)
    assert.match(source, /REDIS_CONN_STRING=redis:\/\//)
    assert.match(source, /\n  redis:/)
  }
  const workflow = readFileSync(new URL('../../.github/workflows/docker-smoke.yml', import.meta.url), 'utf8')
  assert.match(workflow, /BATCH_UPDATE_ENABLED=false/)
  assert.match(workflow, /MYAPI_ACCOUNTING_CONFIG_VERSION=1/)
})

test('startup publishes validated batch configuration before business workers and HTTP', () => {
  const source = readFileSync(new URL('../../main.go', import.meta.url), 'utf8')
  const initBody = source.slice(source.indexOf('func InitResources()'))
  const mainBody = source.slice(source.indexOf('func main()'), source.indexOf('func InitResources()'))
  assert.ok(initBody.indexOf('service.ValidateQuotaBatchStartupConfiguration') > initBody.indexOf('common.InitRedisClient()'))
  assert.ok(initBody.indexOf('common.BatchUpdateEnabled = batchEnabled') > initBody.indexOf('service.ValidateQuotaBatchStartupConfiguration'))
  assert.ok(mainBody.indexOf('InitResources()') < mainBody.indexOf('service.StartSystemTaskRunner()'))
  assert.ok(mainBody.indexOf('InitResources()') < mainBody.indexOf('srv.ListenAndServe()'))
  assert.doesNotMatch(mainBody, /os\.Getenv\("BATCH_UPDATE_ENABLED"\)/)
  assert.match(mainBody, /if common\.BatchUpdateEnabled \{/)
})


test('personal application smoke stays on an exact trusted branch with two owned writer fixtures', () => {
  const workflow = readFileSync(new URL('../../.github/workflows/docker-smoke.yml', import.meta.url), 'utf8')
  const job = workflow.split('  build-and-healthcheck:\n')[1]
  assert.match(job, /head\.repo\.full_name == github\.repository && contains/)
  assert.match(job, /writer: \[legacy\]/)
  assert.match(job, /"scenario":"fresh","writer":"authoritative"/)
  assert.match(job, /matrix\.writer/)
  const prepare = job.split('      - name: Prepare owned personal test network and Redis\n')[1].split('      - name: Start isolated SQLite container\n')[0]
  assert.match(prepare, /docker network create --internal/)
  assert.match(prepare, /redis:7-alpine@sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499/)
  assert.match(prepare, /--network "\$SMOKE_NETWORK" --network-alias smoke-redis/)
  assert.doesNotMatch(prepare, /--publish|(?:^|\s)-p\s|--network[= ]host|--privileged/)
  assert.match(prepare, /--requirepass "\$redis_password"/)
  assert.match(prepare, /::add-mask::/)
  assert.match(prepare, /HostConfig\.PortBindings/)
  const start = job.split('      - name: Start isolated SQLite container\n')[1].split('      - name: Wait for health and verify status\n')[0]
  assert.match(start, /docker create "\$\{personal_args\[@\]\}"/)
  assert.doesNotMatch(start, /personal_args\+=\(--network|--network[= ]host|--privileged/)
  assert.match(start, /MYAPI_SMOKE_WRITER" == 'authoritative'/)
  assert.ok(start.indexOf('docker network connect') < start.indexOf('docker start'))
  assert.match(start, /docker network connect "\$SMOKE_NETWORK" "\$SMOKE_CONTAINER"/)
  assert.match(start, /docker port "\$SMOKE_CONTAINER" 3000\/tcp/)
  assert.match(start, /docker port "\$SMOKE_CONTAINER" 19090\/tcp/)
  assert.match(start, /127\.0\.0\.1:18080/)
  assert.match(start, /127\.0\.0\.1:19090/)
  const readiness = job.split('      - name: Wait for fake upstream fixture\n')[1].split('      - name: Install isolated browser test runtime\n')[0]
  assert.match(readiness, /docker exec "\$SMOKE_FAKE_UPSTREAM_CONTAINER" bun/)
  assert.match(readiness, /AbortSignal.timeout\(2000\)/)
  assert.match(readiness, /namespace-local readiness also failed/)
  assert.doesNotMatch(readiness, /docker logs|docker inspect/)
  const cleanup = job.split('      - name: Remove only owned personal Redis and network\n')[1]
  assert.match(cleanup, /if: always\(\)/)
  assert.match(cleanup, /io\.myapi\.smoke\.sha/)
  assert.match(cleanup, /io\.myapi\.personal\.run/)
  assert.match(cleanup, /GITHUB_RUN_ID-\$GITHUB_RUN_ATTEMPT/)
  assert.match(cleanup, /docker network rm "\$SMOKE_NETWORK"/)
  assert.doesNotMatch(cleanup, /prune|rm -rf/)
  assert.match(job, /name: myapi-personal-\$\{\{ matrix.writer \}\}-\$\{\{ github.sha \}\}/)
  assert.doesNotMatch(workflow, /packages: write|contents: write|id-token: write|push: true|secrets\./)
})
