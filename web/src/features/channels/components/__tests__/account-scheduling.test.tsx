import { readFileSync } from 'node:fs'
import path from 'node:path'

/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, test, vi } from 'vitest'

import { getChannelRoutingPreview } from '../../api'
import type { ChannelRoutingPreviewResponse } from '../../types'
import { ChannelRoutingPreview } from '../channel-routing-preview'

vi.mock('../../api', () => ({ getChannelRoutingPreview: vi.fn() }))

function previewFixture(): NonNullable<ChannelRoutingPreviewResponse['data']> {
  return {
    group: 'default',
    model: 'public-model',
    request_path: '/v1/responses',
    source: 'database',
    generation: 1,
    data_generation: 1,
    published_generation: 1,
    cluster_committed_epoch: 1,
    local_published_epoch: 1,
    cache_enabled: false,
    cache_pending: false,
    tiers: [],
    scheduling: { failover_timeout_seconds: 30, failure_cooldown_seconds: 10 },
    affinity: {
      evaluated: false,
      precedence: 'before_priority_weight',
      explanation_code: 'routing_preview_affinity_not_evaluated',
      account_binding: 'confirmed_success_only',
    },
  }
}

function renderPreview() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ChannelRoutingPreview />
    </QueryClientProvider>
  )
}

async function submitPreview() {
  const user = userEvent.setup()
  await user.type(
    screen.getByLabelText('Routing preview model'),
    'public-model'
  )
  await user.click(screen.getByRole('button', { name: 'Preview routing' }))
  return user
}

describe('account scheduling preview', () => {
  test('submission is disabled without a model and while its scheduling snapshot is loading', async () => {
    let complete!: (value: ChannelRoutingPreviewResponse) => void
    vi.mocked(getChannelRoutingPreview).mockReturnValue(
      new Promise((resolve) => {
        complete = resolve
      })
    )
    renderPreview()
    expect(
      screen.getByRole('button', { name: 'Preview routing' })
    ).toBeDisabled()
    await submitPreview()
    expect(screen.getByRole('button', { name: 'Loading...' })).toBeDisabled()
    expect(
      screen.queryByRole('region', { name: 'Failover scheduling' })
    ).not.toBeInTheDocument()
    complete({ success: true, data: previewFixture() })
    await screen.findByRole('region', { name: 'Failover scheduling' })
    expect(
      screen.getByRole('button', { name: 'Preview routing' })
    ).toBeEnabled()
  })

  test('a configured snapshot explains bounds, separate legacy timeout, and unevaluated success-only affinity', async () => {
    vi.mocked(getChannelRoutingPreview).mockResolvedValue({
      success: true,
      data: previewFixture(),
    })
    renderPreview()
    await submitPreview()

    const scheduling = await screen.findByRole('region', {
      name: 'Failover scheduling',
    })
    expect(
      within(scheduling).getByText('Total failover deadline: 30 seconds')
    ).toBeInTheDocument()
    expect(
      within(scheduling).getByText('Failure cooldown: 10 seconds')
    ).toBeInTheDocument()
    expect(within(scheduling).getByText(/0–3600.*0–300/)).toBeInTheDocument()
    expect(
      within(scheduling).getByText(/separate from RELAY_TIMEOUT/)
    ).toBeInTheDocument()
    expect(
      within(scheduling).getByText(
        /Durable cleanup or settlement may finish after cancellation/
      )
    ).toBeInTheDocument()
    expect(
      screen.getByText(/Account affinity is saved only after confirmed success/)
    ).toHaveTextContent('does not evaluate or update account affinity')
  })

  test('explicit zero settings show disabled rather than a finite deadline or cooldown', async () => {
    const data = previewFixture()
    data.scheduling = {
      failover_timeout_seconds: 0,
      failure_cooldown_seconds: 0,
    }
    vi.mocked(getChannelRoutingPreview).mockResolvedValue({
      success: true,
      data,
    })
    renderPreview()
    await submitPreview()

    expect(
      await screen.findByText(
        'Total failover deadline is disabled (0 seconds).'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByText('Failure cooldown is disabled (0 seconds).')
    ).toBeInTheDocument()
  })

  test('an eligible channel with some cooling accounts stays in its tier and shows the next hold expiry', async () => {
    const data = previewFixture()
    data.tiers = [
      {
        priority: 10,
        fallback_index: 0,
        channels: [
          {
            id: 1,
            name: 'Healthy and cooling accounts',
            type: 1,
            weight: 1,
            effective_weight: 1,
            expected_share: 1,
            cooldown_until: 1800000000,
          },
        ],
      },
    ]
    vi.mocked(getChannelRoutingPreview).mockResolvedValue({
      success: true,
      data,
    })
    renderPreview()
    await submitPreview()

    const tiers = await screen.findByLabelText('Routing preview tiers')
    expect(
      within(tiers).getByText('Healthy and cooling accounts')
    ).toBeInTheDocument()
    expect(
      within(tiers).getByText('Some accounts are temporarily cooling down.')
    ).toBeInTheDocument()
    expect(
      within(tiers).getByText(/Next account hold expiry:/)
    ).toBeInTheDocument()
    expect(
      screen.queryByText(
        'Remaining eligible accounts are temporarily cooling down.'
      )
    ).not.toBeInTheDocument()
    expect(
      screen.getByText(
        /Its expiry does not clear authoritative Codex exhaustion/
      )
    ).toHaveTextContent(
      'does not clear authoritative Codex exhaustion or guarantee eligibility'
    )
  })

  test('cooling remaining eligible accounts and unknown exclusions are explained without exposing raw reasons or widening narrow screens', async () => {
    const data = previewFixture()
    const longName = 'Account-channel-'.repeat(40)
    data.rejected = [
      {
        id: 7,
        name: longName,
        reason: 'account_cooling_down',
        cooldown_until: 1800000000,
      },
      {
        id: 8,
        name: 'Other exclusion',
        reason: 'future_private_reason',
        cooldown_until: 0,
      },
    ]
    vi.mocked(getChannelRoutingPreview).mockResolvedValue({
      success: true,
      data,
    })
    renderPreview()
    await submitPreview()

    const rejected = await screen.findByRole('list', {
      name: 'Excluded channels',
    })
    expect(rejected).toHaveClass('min-w-0')
    expect(within(rejected).getByText(longName)).toHaveClass('break-all')
    expect(
      within(rejected).getByText(
        'Remaining eligible accounts are temporarily cooling down.'
      )
    ).toBeInTheDocument()
    expect(
      within(rejected).getAllByText(/Next account hold expiry:/)
    ).toHaveLength(1)
    expect(
      within(rejected).getByText('Excluded from this routing snapshot.')
    ).toBeInTheDocument()
    expect(screen.queryByText('future_private_reason')).not.toBeInTheDocument()
    expect(
      screen.getByText('No eligible channels remain in this routing snapshot.')
    ).toBeInTheDocument()
  })

  test('old snapshots do not invent disabled settings or confirmed account affinity', async () => {
    const data = previewFixture()
    delete data.scheduling
    delete data.affinity.account_binding
    vi.mocked(getChannelRoutingPreview).mockResolvedValue({
      success: true,
      data,
    })
    renderPreview()
    await submitPreview()
    await screen.findByText('Database snapshot')

    expect(
      screen.queryByRole('region', { name: 'Failover scheduling' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByText(
        /Account affinity is saved only after confirmed success/
      )
    ).not.toBeInTheDocument()
    expect(
      screen.getByText(
        /This preview does not evaluate, read, or write affinity state/
      )
    ).toBeInTheDocument()
  })

  test('a failed refresh clears scheduling and exclusion evidence from the old snapshot', async () => {
    const data = previewFixture()
    data.rejected = [
      {
        id: 1,
        name: 'Cooling channel',
        reason: 'account_cooling_down',
        cooldown_until: 1800000000,
      },
    ]
    vi.mocked(getChannelRoutingPreview)
      .mockResolvedValueOnce({ success: true, data })
      .mockRejectedValueOnce({ isAxiosError: true, message: 'Network Error' })
    renderPreview()
    const user = await submitPreview()
    await screen.findByRole('region', { name: 'Failover scheduling' })
    await user.click(screen.getByRole('button', { name: 'Preview routing' }))

    await screen.findByText('Unable to reach the server. Try again.')
    expect(
      screen.queryByRole('region', { name: 'Failover scheduling' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('list', { name: 'Excluded channels' })
    ).not.toBeInTheDocument()
  })

  test.each([undefined, 0, -1, 9_000_000_000_000])(
    'an absent or invalid expiry %s never renders an invalid date',
    async (cooldown_until) => {
      const data = previewFixture()
      data.rejected = [
        {
          id: 1,
          name: 'Cooling channel',
          reason: 'account_cooling_down',
          cooldown_until,
        },
      ]
      vi.mocked(getChannelRoutingPreview).mockResolvedValue({
        success: true,
        data,
      })
      renderPreview()
      await submitPreview()
      await screen.findByText(
        'Remaining eligible accounts are temporarily cooling down.'
      )
      expect(
        screen.queryByText(/Next account hold expiry:/)
      ).not.toBeInTheDocument()
      expect(screen.queryByText(/Invalid Date/)).not.toBeInTheDocument()
    }
  )

  test.each([
    ['en', 'en'],
    ['zh', 'zhCN'],
    ['zh-TW', 'zhTW'],
    ['fr', 'fr'],
    ['ja', 'ja'],
    ['ru', 'ru'],
    ['vi', 'vi'],
  ])(
    '%s renders translated scheduling, exclusions, expiry, and affinity without fallback or lost placeholders',
    async (locale, language) => {
      const translations = JSON.parse(
        readFileSync(
          path.resolve(process.cwd(), `src/i18n/locales/${locale}.json`),
          'utf8'
        )
      ).translation as Record<string, string>
      const source = readFileSync(
        path.resolve(
          process.cwd(),
          'src/features/channels/components/channel-routing-preview.tsx'
        ),
        'utf8'
      )
      const keys = [...source.matchAll(/\bt\(\s*(['"])(.*?)\1/gms)].map(
        (match) => match[2]
      )
      for (const key of keys) {
        expect(translations[key], `${locale}: ${key}`).toBeTruthy()
        expect((translations[key].match(/{{\w+}}/g) ?? []).sort()).toEqual(
          (key.match(/{{\w+}}/g) ?? []).sort()
        )
      }
      const i18n = createInstance()
      await i18n.init({
        lng: language,
        fallbackLng: false,
        resources: { [language]: { translation: translations } },
      })
      const data = previewFixture()
      data.rejected = [
        {
          id: 1,
          name: 'Cooling channel',
          reason: 'account_cooling_down',
          cooldown_until: 1800000000,
        },
      ]
      vi.mocked(getChannelRoutingPreview).mockResolvedValue({
        success: true,
        data,
      })
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })
      render(
        <I18nextProvider i18n={i18n}>
          <QueryClientProvider client={client}>
            <ChannelRoutingPreview />
          </QueryClientProvider>
        </I18nextProvider>
      )
      const user = userEvent.setup()
      await user.type(
        screen.getByLabelText(i18n.t('Routing preview model')),
        'public-model'
      )
      await user.click(
        screen.getByRole('button', { name: i18n.t('Preview routing') })
      )
      const section = await screen.findByRole('region', {
        name: i18n.t('Failover scheduling'),
      })
      expect(section).toHaveClass('min-w-0', 'wrap-break-word')
      expect(
        within(section).getByText(
          i18n.t('Total failover deadline: {{seconds}} seconds', {
            seconds: 30,
          })
        )
      ).toBeInTheDocument()
      const excluded = screen.getByRole('list', {
        name: i18n.t('Excluded channels'),
      })
      expect(
        within(excluded).getByText(
          i18n.t('Remaining eligible accounts are temporarily cooling down.')
        )
      ).toBeInTheDocument()
      const expiryPrefix =
        translations['Next account hold expiry: {{time}}'].split('{{time}}')[0]
      expect(
        within(excluded).getByText((text) => text.startsWith(expiryPrefix))
      ).not.toHaveTextContent('{{time}}')
      expect(
        screen.getByText(
          translations[
            'Account affinity is saved only after confirmed success. This preview has no session input and does not evaluate or update account affinity. Every request still checks current credentials, permissions, budget, and account windows.'
          ]
        )
      ).toBeInTheDocument()
    }
  )
})
