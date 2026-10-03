import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

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
