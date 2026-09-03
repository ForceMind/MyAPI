import assert from 'node:assert/strict'
import test from 'node:test'
import { probeFreshSQLite, validateSmokeTarget } from './docker-smoke.mjs'

const sha = 'a'.repeat(40)
const baseUrl = 'http://127.0.0.1:18080'
const json = (body, status = 200) => new Response(JSON.stringify(body), { status })

test('smoke initialization requires explicit opt-in and a literal loopback target', async () => {
  for (const url of ['invalid', 'http://127.0.0.1:0', 'https://127.0.0.1:18080', 'http://localhost:18080', 'http://192.0.2.1:18080',
    'http://127.0.0.1:18080/api', 'http://127.0.0.1:18080?secret=value', 'http://user:pass@127.0.0.1:18080']) {
    assert.throws(() => validateSmokeTarget(url, 'full', sha))
  }
  assert.throws(() => validateSmokeTarget(baseUrl, 'unknown', sha))
  assert.throws(() => validateSmokeTarget(baseUrl, 'full', 'main'))
  await assert.rejects(probeFreshSQLite({ baseUrl, edition: 'full', sha }), /OPT_IN_REQUIRED/)
})

test('existing or non-SQLite instances are rejected without writes', async () => {
  for (const data of [
    { status: true, root_init: true, database_type: 'sqlite' },
    { status: false, root_init: true, database_type: 'sqlite' },
    { status: false, root_init: false, database_type: 'mysql' },
    {},
  ]) {
    let requests = 0
    await assert.rejects(probeFreshSQLite({ baseUrl, edition: 'full', sha, isolated: true,
      fetchImpl: async (_url, options) => {
        requests += 1
        assert.equal(options.method, undefined)
        assert.equal(options.redirect, 'error')
        return json({ success: true, data })
      },
    }), /FRESH_SQLITE/)
    assert.equal(requests, 1)
  }
})

for (const edition of ['full', 'lan']) {
  test(`${edition} fresh fixture initializes, authenticates and reports no credentials`, async () => {
    let setupBody
    let initialized = false
    const report = await probeFreshSQLite({ baseUrl, edition, sha, isolated: true,
      fetchImpl: async (url, options) => {
        assert.equal(options.redirect, 'error')
        const path = new URL(url).pathname
        if (path === '/api/setup' && options.method === 'POST') {
          setupBody = JSON.parse(options.body)
          assert.equal(setupBody.password, setupBody.confirmPassword)
          assert.equal(setupBody.SelfUseModeEnabled, edition === 'lan')
          initialized = true
          return json({ success: true })
        }
        if (path === '/api/setup') return json({ success: true, data: { status: false, root_init: false, database_type: 'sqlite' } })
        assert.ok(initialized)
        if (path === '/api/status') return json({ success: true, data: { setup: true, self_use_mode_enabled: edition === 'lan' } })
        if (path === '/api/user/login') {
          assert.equal(JSON.parse(options.body).password, setupBody.password)
          return json({ success: true, data: { access_token: 'private-fixture-access-token' } })
        }
        if (path === '/api/user/self' && !options.headers?.Authorization) return json({ success: false }, 401)
        assert.equal(options.headers.Authorization, 'Bearer private-fixture-access-token')
        assert.ok(['/api/user/self', '/api/channel/quota/changes', '/api/log/'].includes(path))
        return json({ success: true, data: { private_body: 'private-fixture-response' } })
      },
    })
    assert.equal(report.passed, true)
    assert.equal(report.edition, edition)
    assert.equal(report.sha, sha)
    for (const secret of [setupBody.password, 'private-fixture-access-token', 'private-fixture-response']) {
      assert.equal(JSON.stringify(report).includes(secret), false)
    }
  })
}

for (const failure of ['setup', 'anonymous', 'mode', 'login']) {
  test(`fresh fixture rejects ${failure} failure without reporting success`, async () => {
    const paths = []
    const run = probeFreshSQLite({ baseUrl, edition: 'full', sha, isolated: true,
      fetchImpl: async (url, options) => {
        const path = new URL(url).pathname
        paths.push(path)
        if (path === '/api/setup' && options.method !== 'POST') {
          return json({ success: true, data: { status: false, root_init: false, database_type: 'sqlite' } })
        }
        if (path === '/api/setup') return json({ success: failure !== 'setup' })
        if (path === '/api/user/self') return json({ success: failure === 'anonymous' }, failure === 'anonymous' ? 200 : 401)
        if (path === '/api/status') return json({ success: true, data: { setup: true, self_use_mode_enabled: failure === 'mode' } })
        if (path === '/api/user/login') return json({ success: false, message: 'private server detail' }, 401)
        assert.fail(`unexpected request ${path}`)
      },
    })
    if (failure === 'login') {
      const report = await run
      assert.equal(report.passed, false)
      assert.equal(JSON.stringify(report).includes('private server detail'), false)
      assert.equal(paths.includes('/api/log/'), false)
    } else {
      await assert.rejects(run, new RegExp({ setup: 'SETUP_FAILED', anonymous: 'ANONYMOUS_ACCESS_NOT_REJECTED', mode: 'RUNTIME_MODE_MISMATCH' }[failure]))
      assert.equal(paths.includes('/api/user/login'), false)
    }
  })
}
