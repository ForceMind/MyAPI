import { render, within } from '@testing-library/react'
import { createInstance } from 'i18next'
import { createElement } from 'react'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, test } from 'vitest'

import { TokenBudgetEvidence } from '@/features/usage-logs/components/token-budget-evidence'
import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import vi from '@/i18n/locales/vi.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zh from '@/i18n/locales/zh.json'

const localeResources = [
  ['en', en],
  ['zh', zh],
  ['zh-TW', zhTW],
  ['fr', fr],
  ['ru', ru],
  ['ja', ja],
  ['vi', vi],
] as const

const chatFeeRequirements =
  'Chat USD budgets require service_tier=default, exact published short/long prices frozen at dispatch, and explicit cache-read and cache-write usage counters, including zero. Missing evidence retains the reservation for review.'
const chatConservativeBound =
  'Chat reserves 1,050,000 total tokens for input and completion, including reasoning. This is a conservative bound, not measured usage or a tokenizer estimate. Even a small request can fail if the remaining budget cannot cover this bound.'
const chatEvidenceExplanation =
  'Chat reserves the 1,050,000-token context ceiling, not a measured input count. The total includes input and completion; the output cap is not added twice.'
const requestRequirements =
  'Responses requires explicit max_output_tokens. Chat requires explicit max_completion_tokens from 1 to 128,000 and n omitted or 1; streaming requires stream_options.include_usage=true.'
const sourceSchedule =
  'Scheduled checks are off by default. When enabled, checks run every 24 hours; evidence is stale after 72 hours without a successful check.'
const supportedModel =
  'Strict budgets support official OpenAI Responses text and native Chat text for exact model gpt-6.1-sol with per-token pricing.'

const requiredKeys = [
  'Added',
  'An existing source check was reused.',
  'Candidate price digest',
  'Changed',
  chatFeeRequirements,
  chatConservativeBound,
  chatEvidenceExplanation,
  'Check official source now',
  'Checks save source evidence for Root review. They never publish prices or override locks.',
  'Compared models',
  'Confirmed total tokens',
  'Could not load source check status. Refresh before making changes.',
  'Current price digest',
  'Daily source checks',
  'Disable daily source checks',
  'Enable daily source checks',
  'Fresh source evidence',
  'Input reservation bound',
  'Last check attempt',
  'Last successful check',
  'Last successful check is separate from source creation and price publication. An unchanged source can have a new successful check.',
  'Next scheduled check',
  'No pending source differences. Saved source does not mean budget qualification.',
  'Not confirmed',
  'Official source checks',
  'Output reservation cap',
  'Pending review',
  'Price publication revision',
  'Refresh check status',
  'Reservation and actual usage',
  'Reservation bound source',
  requestRequirements,
  'Review saved source',
  sourceSchedule,
  'Source check differences',
  'Source check queued.',
  'Source check request failed. Refresh status before retrying; saved evidence and effective prices are unchanged.',
  'Source created at',
  'Source differences await Root review. Saved source does not mean published price or budget qualification.',
  'Source freshness',
  'Stale source evidence',
  supportedModel,
  'The last source check failed. The last good source and effective prices are retained.',
  'This bounded preview is incomplete. Counts cover displayed rows only; review all source models before publication.',
  'Total reservation bound',
  'Unchanged',
] as const

// Numeric grouping is localized; the limits and API identifiers must not change.
const technicalContracts = [
  [chatFeeRequirements, [/USD/, /service_tier=default/]],
  [chatConservativeBound, [/Chat/, /1[,.\s]050[,.\s]000/]],
  [chatEvidenceExplanation, [/Chat/, /1[,.\s]050[,.\s]000/]],
  [
    requestRequirements,
    [
      /Responses/,
      /max_output_tokens/,
      /Chat/,
      /max_completion_tokens/,
      /(?:^|\D)1(?:\D|$)/,
      /128[,.\s]000/,
      /\bn\b/,
      /stream_options\.include_usage=true/,
    ],
  ],
  [sourceSchedule, [/(?:^|\D)24(?:\D|$)/, /(?:^|\D)72(?:\D|$)/]],
  [supportedModel, [/OpenAI/, /Responses/, /Chat/, /gpt-6\.1-sol/]],
  [
    'Checks save source evidence for Root review. They never publish prices or override locks.',
    [/Root/],
  ],
  [
    'Source differences await Root review. Saved source does not mean published price or budget qualification.',
    [/Root/],
  ],
] as const

describe('beta.6 source and strict-budget translations', () => {
  test.each(localeResources)(
    '%s provides all 46 non-empty beta.6 translations without English fallback',
    (locale, resource) => {
      for (const key of requiredKeys) {
        expect(Object.hasOwn(resource.translation, key), key).toBe(true)
        expect(resource.translation[key].trim(), key).not.toBe('')
        if (locale !== 'en') {
          expect(resource.translation[key], key).not.toBe(en.translation[key])
        }
      }
    }
  )

  test.each(localeResources)(
    '%s preserves model, API field and numeric safety requirements',
    (_locale, resource) => {
      for (const [key, patterns] of technicalContracts) {
        for (const pattern of patterns) {
          expect(resource.translation[key], key).toMatch(pattern)
        }
      }
    }
  )

  test.each(localeResources)(
    '%s renders localized reservation evidence and preserves confirmed zero usage',
    async (locale, resource) => {
      const i18n = createInstance()
      await i18n.init({
        lng: locale,
        fallbackLng: false,
        keySeparator: false,
        nsSeparator: false,
        interpolation: { escapeValue: false },
        resources: { [locale]: resource },
      })
      const view = render(
        createElement(
          I18nextProvider,
          { i18n },
          createElement(TokenBudgetEvidence, {
            evidence: {
              bound_source: 'openai_chat_context_window',
              input_tokens_bound: 1050000,
              max_output_tokens: 128000,
              reserved: 1050000,
              fee_reserved_usd: '12.34',
              actual_input: 0,
              actual_output: 0,
              actual_fee_usd: '0.000000',
            },
          })
        )
      )
      try {
        const evidence = within(
          view.getByRole('region', {
            name: resource.translation['Reservation and actual usage'],
          })
        )
        expect(
          evidence.getByText(resource.translation[chatEvidenceExplanation])
        ).toBeInTheDocument()
        expect(
          evidence.getByText(resource.translation['Confirmed input tokens'])
        ).toBeInTheDocument()
        expect(
          evidence.getByText(resource.translation['Confirmed output tokens'])
        ).toBeInTheDocument()
        expect(
          evidence.getByText(resource.translation['Confirmed total tokens'])
        ).toBeInTheDocument()
        expect(evidence.getAllByText('0', { exact: true })).toHaveLength(3)
        expect(
          evidence.getByText('0.000000', { exact: true })
        ).toBeInTheDocument()
        expect(
          evidence.queryByText(resource.translation['Not confirmed'])
        ).not.toBeInTheDocument()
        if (locale !== 'en') {
          expect(
            view.queryByRole('region', {
              name: en.translation['Reservation and actual usage'],
            })
          ).not.toBeInTheDocument()
          expect(
            evidence.queryByText(en.translation[chatEvidenceExplanation])
          ).not.toBeInTheDocument()
        }
      } finally {
        view.unmount()
      }
    }
  )
})
