import { readFileSync } from 'node:fs'
import path from 'node:path'

/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { render, screen, within } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import type { UsageLog } from '../../data/schema'
import { DetailsDialog } from '../dialogs/details-dialog'

const log: UsageLog = {
  id: 1,
  user_id: 2,
  created_at: 1756560000,
  type: 5,
  content: '',
  username: 'user',
  token_name: 'key',
  model_name: 'public',
  quota: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  use_time: 1,
  is_stream: false,
  channel: 3,
  channel_name: 'Test',
  token_id: 4,
  group: 'default',
  ip: '',
  request_id: '',
  upstream_request_id: '',
  other: '{}',
}

function renderAttempts(attempts: unknown, isAdmin = true, type = 5) {
  return render(
    <DetailsDialog
      log={{
        ...log,
        type,
        other: JSON.stringify({ admin_info: { relay_attempts: attempts } }),
      }}
      isAdmin={isAdmin}
      open
      onOpenChange={() => {}}
    />
  )
}

const refused = {
  channel_id: 3,
  key_index: 0,
  upstream_model: 'private-first',
  outcome: 'retryable_refusal',
  status: 429,
}

describe('relay attempt evidence', () => {
  test('a recorded cooldown explains a future-request hold without claiming quota exhaustion or extending this request', () => {
    renderAttempts([{ ...refused, cooldown_seconds: 12 }])
    const attempt = screen.getByRole('listitem')
    expect(
      within(attempt).getByText('Future-request account hold: 12 seconds')
    ).toBeInTheDocument()
    expect(
      within(attempt).getByText(
        "This temporary hold is recorded for later requests. It does not prove quota exhaustion or extend this request's retry window."
      )
    ).toHaveClass('text-muted-foreground')
  })

  test.each([undefined, 0])(
    'missing or disabled cooldown %s does not invent an account hold',
    (cooldown_seconds) => {
      renderAttempts([{ ...refused, cooldown_seconds }])
      expect(
        screen.queryByText(/Future-request account hold:/)
      ).not.toBeInTheDocument()
    }
  )

  test('cooldown evidence stays hidden from nonadministrators', () => {
    renderAttempts([{ ...refused, cooldown_seconds: 12 }], false)
    expect(
      screen.queryByText(/Future-request account hold:/)
    ).not.toBeInTheDocument()
  })

  test.each(['en', 'zh', 'zh-TW', 'fr', 'ja', 'ru', 'vi'])(
    '%s provides translated cooldown explanations with their duration placeholder',
    (locale) => {
      const translations = JSON.parse(
        readFileSync(
          path.resolve(process.cwd(), `src/i18n/locales/${locale}.json`),
          'utf8'
        )
      ).translation as Record<string, string>
      const duration = 'Future-request account hold: {{seconds}} seconds'
      const explanation =
        "This temporary hold is recorded for later requests. It does not prove quota exhaustion or extend this request's retry window."
      expect(translations[duration]).toContain('{{seconds}}')
      expect(translations[explanation]).toBeTruthy()
      if (locale !== 'en') {
        expect(translations[duration]).not.toBe(duration)
        expect(translations[explanation]).not.toBe(explanation)
      }
    }
  )

  test.each([2, 5])(
    'log type %s shows refusal followed by the selected attempt in recorded order',
    (type) => {
      renderAttempts(
        [
          refused,
          {
            ...refused,
            key_index: 2,
            upstream_model: 'private-second',
            outcome: 'selected',
            status: undefined,
          },
        ],
        true,
        type
      )
      const attempts = within(
        screen.getByRole('list', { name: 'Relay attempts' })
      ).getAllByRole('listitem')
      expect(attempts).toHaveLength(2)
      expect(within(attempts[0]).getByText('Attempt 1')).toBeInTheDocument()
      expect(within(attempts[0]).getByText('private-first')).toBeInTheDocument()
      expect(within(attempts[0]).getByText('0')).toBeInTheDocument()
      expect(within(attempts[0]).getByText('429')).toBeInTheDocument()
      expect(
        within(attempts[0]).getByText('Refused; retry allowed')
      ).toBeInTheDocument()
      expect(within(attempts[1]).getByText('Attempt 2')).toBeInTheDocument()
      expect(
        within(attempts[1]).getByText('private-second')
      ).toBeInTheDocument()
      expect(within(attempts[1]).getByText('2')).toBeInTheDocument()
      expect(
        within(attempts[1]).getByText('Selected at log time')
      ).toBeInTheDocument()
      expect(
        within(attempts[1]).queryByText('Failure HTTP status')
      ).not.toBeInTheDocument()
      expect(screen.queryByText('Completed')).not.toBeInTheDocument()
    }
  )

  test.each([
    ['refused', 'Refused'],
    ['failed', 'Failed'],
    ['completed', 'Completed'],
    ['future_outcome', 'Unknown outcome'],
  ])(
    'recorded outcome %s has the correct label without exposing unknown raw values',
    (outcome, label) => {
      renderAttempts([{ ...refused, outcome }])
      expect(screen.getByText(label)).toBeInTheDocument()
      expect(screen.queryByText('future_outcome')).not.toBeInTheDocument()
    }
  )

  test.each([undefined, null, []])(
    'legacy or empty evidence %s omits the attempt section',
    (attempts) => {
      renderAttempts(attempts)
      expect(
        screen.queryByRole('list', { name: 'Relay attempts' })
      ).not.toBeInTheDocument()
      expect(screen.queryByText('Relay attempts')).not.toBeInTheDocument()
    }
  )

  test('nonadministrator cannot see attempt evidence even if the response includes it', () => {
    renderAttempts([refused], false)
    expect(screen.queryByText('Relay attempts')).not.toBeInTheDocument()
    expect(screen.queryByText('private-first')).not.toBeInTheDocument()
    expect(screen.queryByText('Refused; retry allowed')).not.toBeInTheDocument()
  })

  test('long upstream names wrap in shrinkable rows instead of widening the drawer', () => {
    const upstreamModel = 'private-target-'.repeat(50)
    renderAttempts([{ ...refused, upstream_model: upstreamModel }])
    expect(screen.getByRole('list', { name: 'Relay attempts' })).toHaveClass(
      'min-w-0'
    )
    expect(screen.getByRole('listitem')).toHaveClass('min-w-0')
    expect(screen.getByText(upstreamModel)).toHaveClass(
      'min-w-0',
      'max-w-full',
      'break-all'
    )
  })

  test('extra credential data is never rendered from an attempt', () => {
    renderAttempts([
      {
        ...refused,
        api_key: 'credential-must-stay-hidden',
        credential_digest: 'digest-must-stay-hidden',
      },
    ])
    expect(
      screen.queryByText('credential-must-stay-hidden')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByText('digest-must-stay-hidden')
    ).not.toBeInTheDocument()
  })
})
