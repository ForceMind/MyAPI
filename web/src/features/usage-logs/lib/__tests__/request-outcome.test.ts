import { describe, expect, test } from 'vitest'

import { getLogTypeConfig } from '../utils'

describe('request outcome is independent of usage settlement', () => {
  test.each(['missing', 'estimated', 'partial', 'invalid', 'unknown'])(
    'a completed request with %s usage is awaiting review, not a request error',
    (usageOutcome) => {
      expect(
        getLogTypeConfig(5, {
          request_outcome: 'completed',
          usage_outcome: usageOutcome,
          settlement_status: 'pending_review',
          actual_quota: null,
        })
      ).toMatchObject({ label: 'Completed · usage pending', color: 'orange' })
    }
  )

  test('reported usage with an unconfirmed settlement remains a separate alert', () => {
    expect(
      getLogTypeConfig(5, {
        request_outcome: 'completed',
        usage_outcome: 'reported',
        settlement_status: 'pending_review',
      })
    ).toMatchObject({ label: 'Completed · settlement pending', color: 'orange' })
  })

  test.each([undefined, 'failed', 'unknown', 'COMPLETED'])(
    'does not infer successful delivery from pending usage with outcome %s',
    (requestOutcome) => {
      expect(
        getLogTypeConfig(5, {
          request_outcome: requestOutcome,
          settlement_status: 'pending_review',
        })
      ).toMatchObject({ label: 'Error', color: 'red' })
    }
  )

  test('does not reinterpret other log types or an ordinary request error', () => {
    expect(getLogTypeConfig(2)).toMatchObject({ label: 'Consume' })
    expect(getLogTypeConfig(5, { request_outcome: 'completed' })).toMatchObject({
      label: 'Error',
    })
  })
})
