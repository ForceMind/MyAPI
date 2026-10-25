import { describe, expect, test } from 'vitest'

import { isRequestFailure } from '../request-facts'
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
    ).toMatchObject({
      label: 'Completed · settlement pending',
      color: 'orange',
    })
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

  test('completion evidence remains successful without a pending settlement marker', () => {
    expect(getLogTypeConfig(2)).toMatchObject({ label: 'Consume' })
    expect(getLogTypeConfig(5, { request_outcome: 'completed' })).toMatchObject(
      {
        label: 'Request succeeded',
        color: 'green',
      }
    )
    expect(getLogTypeConfig(5, { request_outcome: 'failed' })).toMatchObject({
      label: 'Error',
      color: 'red',
    })
  })
})

describe('request failures match the server result=failed filter', () => {
  test.each([
    [
      'an error record',
      { type: 5, content: 'status_code=502', other: '{}' },
      true,
    ],
    ['a consume record', { type: 2, content: '', other: '{}' }, false],
    [
      'a usage review',
      { type: 5, content: 'usage_pending_review', other: '{}' },
      false,
    ],
    [
      'a settlement review',
      { type: 5, content: 'usage_settlement_pending', other: '{}' },
      false,
    ],
    [
      'a preflight rejection',
      {
        type: 5,
        content: 'input limit',
        other: '{"request_outcome":"rejected","preflight_rejected":true}',
      },
      false,
    ],
    [
      'an explicit non-rejection',
      { type: 5, content: 'x', other: '{"preflight_rejected":false}' },
      true,
    ],
    ['unreadable metadata', { type: 5, content: 'x', other: '{broken' }, true],
  ])('%s', (_name, log, expected) => {
    expect(isRequestFailure(log)).toBe(expected)
  })
})
