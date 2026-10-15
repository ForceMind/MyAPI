/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { describe, expect, it } from 'vitest'

import { usageLogSchema } from '../../data/schema'
import { getPersonalCallEvidence } from '../personal-call-evidence'

const completed = usageLogSchema.parse({ id: 1, user_id: 1, created_at: 1, type: 5, content: '', other: JSON.stringify({ request_outcome: 'completed', usage_outcome: 'missing' }) })

describe('personal call evidence', () => {
  it.each(['missing', 'estimated', 'partial', 'invalid', 'unknown', 'unavailable'])('keeps completed requests successful with unknown %s token usage', (usageOutcome) => {
    expect(getPersonalCallEvidence({ ...completed, prompt_tokens: 42, other: JSON.stringify({ request_outcome: 'completed', usage_outcome: usageOutcome }) })).toMatchObject({ completed: true, usageKnown: false })
  })
  it('shows explicit reported zero token usage without changing successful completion', () => {
    expect(getPersonalCallEvidence({ ...completed, other: JSON.stringify({ request_outcome: 'completed', usage_outcome: 'reported' }) })).toMatchObject({ completed: true, usageKnown: true })
  })
  it('keeps a failed request failed when its usage is missing', () => {
    expect(getPersonalCallEvidence({ ...completed, other: JSON.stringify({ request_outcome: 'failed', usage_outcome: 'missing' }) })).toMatchObject({ completed: false, usageKnown: false })
  })
  it('does not infer completion from an error log with malformed metadata', () => {
    expect(getPersonalCallEvidence({ ...completed, other: '{broken' })).toMatchObject({ completed: false, usageKnown: false })
  })
  it('preserves historical successful logs with known positive token counts', () => {
    expect(getPersonalCallEvidence({ ...completed, type: 2, prompt_tokens: 10, completion_tokens: 4, other: '' })).toMatchObject({ completed: true, usageKnown: true })
  })
  it('does not turn a historical all-zero log into known usage', () => {
    expect(getPersonalCallEvidence({ ...completed, type: 2, other: '' })).toMatchObject({ completed: true, usageKnown: false })
  })
  it.each([-1, Infinity, Number.MAX_SAFE_INTEGER + 1])('does not present invalid reported tokens %s as known', (tokens) => {
    expect(getPersonalCallEvidence({ ...completed, prompt_tokens: tokens, other: JSON.stringify({ usage_outcome: 'reported' }) }).usageKnown).toBe(false)
  })
})
