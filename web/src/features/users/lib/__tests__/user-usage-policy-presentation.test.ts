import { afterAll, expect, test, vi } from 'vitest'

import type { SystemStatus } from '@/features/auth/types'
import { resolveUserFundingCapabilities } from '@/lib/self-use-build'

import { resolveUsagePolicyPresentation } from '../user-usage-policy-presentation'

vi.hoisted(() => vi.stubEnv('VITE_SELF_USE_MINIMAL', 'true'))
afterAll(() => vi.unstubAllEnvs())
const policy = {
  user_id: 2,
  no_balance: true,
  revision: 1,
  legacy_remaining_quota: 0,
}
const status = {
  user_funding_mode: 'disabled',
  user_funding_capabilities: { mode: 'disabled', ready: true, epoch: 1 },
}
const input = {
  userId: 2,
  policy,
  policyConfirmed: true,
  status: status as SystemStatus,
  statusConfirmed: true,
}

test('minimal display override cannot replace a confirmed enabled server mode', () => {
  const enabled = {
    user_funding_mode: 'enabled',
    user_funding_capabilities: { mode: 'enabled', ready: true, epoch: 1 },
  } as SystemStatus
  expect(resolveUserFundingCapabilities(enabled).mode).toBe('disabled')
  expect(resolveUsagePolicyPresentation({ ...input, status: enabled })).toEqual(
    { mode: 'enabled', savedNoBalance: true, condition: 'inactive' }
  )
})
test('valid disabled funding and audited saved preference describe supported-path configuration only', () => {
  expect(resolveUsagePolicyPresentation(input)).toEqual({
    mode: 'disabled',
    savedNoBalance: true,
    condition: 'supported',
  })
  expect(
    resolveUsagePolicyPresentation({
      ...input,
      policy: { ...policy, no_balance: false, revision: 0 },
    })
  ).toEqual({ mode: 'disabled', savedNoBalance: false, condition: 'wallet' })
})
const invalidStatuses: Array<[string, unknown]> = [
  ['missing capabilities', { user_funding_mode: 'disabled' }],
  [
    'unknown mode',
    {
      ...status,
      user_funding_capabilities: {
        ...status.user_funding_capabilities,
        mode: 'future',
      },
    },
  ],
  ['conflicting modes', { ...status, user_funding_mode: 'enabled' }],
  [
    'not ready',
    {
      ...status,
      user_funding_capabilities: {
        ...status.user_funding_capabilities,
        ready: false,
      },
    },
  ],
  [
    'string ready',
    {
      ...status,
      user_funding_capabilities: {
        ...status.user_funding_capabilities,
        ready: 'true',
      },
    },
  ],
  ...[undefined, 0, -1, 1.5, Number.MAX_SAFE_INTEGER + 1].map(
    (epoch): [string, unknown] => [
      `invalid epoch ${epoch}`,
      {
        ...status,
        user_funding_capabilities: {
          ...status.user_funding_capabilities,
          epoch,
        },
      },
    ]
  ),
]
test.each(invalidStatuses)(
  '%s does not establish funding configuration',
  (_name, value) => {
    expect(
      resolveUsagePolicyPresentation({
        ...input,
        status: value as SystemStatus,
      })
    ).toEqual({ mode: null, savedNoBalance: true, condition: 'unknown' })
  }
)
test.each([
  ['status placeholder or error', { ...input, statusConfirmed: false }],
  ['policy refresh or error', { ...input, policyConfirmed: false }],
  ['missing policy', { ...input, policy: undefined }],
  ['different user', { ...input, userId: 3 }],
  [
    'invalid identity',
    { ...input, userId: 0, policy: { ...policy, user_id: 0 } },
  ],
  [
    'unaudited no-wallet preference',
    { ...input, policy: { ...policy, revision: 0 } },
  ],
  ['negative revision', { ...input, policy: { ...policy, revision: -1 } }],
])('%s cannot retain supported configuration claims', (_name, value) => {
  expect(resolveUsagePolicyPresentation(value).condition).toBe('unknown')
})
