import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

import en from '../locales/en.json'
import fr from '../locales/fr.json'
import ja from '../locales/ja.json'
import ru from '../locales/ru.json'
import vi from '../locales/vi.json'
import zh from '../locales/zh.json'
import zhTW from '../locales/zh-TW.json'

const baseKeys = Object.keys(en.translation).sort()
const localeResources = [
  ['zh', zh],
  ['fr', fr],
  ['ja', ja],
  ['ru', ru],
  ['vi', vi],
  ['zh-TW', zhTW],
] as const
const allLocaleResources = [
  ['en', en],
  ['zh', zh],
  ['fr', fr],
  ['ja', ja],
  ['ru', ru],
  ['vi', vi],
  ['zh-TW', zhTW],
] as const

// Check the actual shipped quota surfaces, not only a manually curated key
// list. This catches a new parent-card label being missed while shared chart
// translations still pass. Dynamic enum labels remain covered below.
const quotaSourceFiles = [
  'features/channels/components/quota-history-trend.tsx',
  'features/channels/components/channel-quota-changes-panel.tsx',
  'features/channels/components/channel-quota-detail-chart.tsx',
  'features/channels/components/dialogs/channel-quota-history.tsx',
  'features/channels/components/dialogs/codex-usage-dialog.tsx',
  'features/channels/lib/quota-history.ts',
  'features/dashboard/components/overview/account-quota-changes-panel.tsx',
  'features/dashboard/components/overview/codex-account-quota-chart.tsx',
  'features/system-settings/integrations/monitoring-settings-section.tsx',
]

describe('shipped quota page translation coverage', () => {
  const keys = new Set<string>()
  for (const source of quotaSourceFiles) {
    const text = readFileSync(resolve(process.cwd(), 'src', source), 'utf8')
    for (const match of text.matchAll(/\bt\(\s*(['"])(.*?)\1/g)) {
      keys.add(match[2])
    }
  }
  it.each(allLocaleResources)('%s covers actual quota surface literals', (_locale, resource) => {
    expect([...keys].filter((key) => !(key in resource.translation))).toEqual([])
  })
  it('renders the new quota consumption explanations in Simplified Chinese', () => {
    const labels = zh.translation as Record<string, string>
    for (const key of keys) {
      if (/consumption|sampling gap|quota baseline|Quota series|quota window|Weekly window|Daily window/i.test(key)) {
        expect(labels[key], key).toMatch(/[\u3400-\u9fff]/)
      }
    }
  })
})

const quotaTrendKeys = [
  '1 minute',
  '5 minutes',
  '15 minutes',
  'Area',
  'Auto',
  'Available quota',
  'Bar',
  'Change',
  'Change since reset',
  'Chart granularity',
  'Chart style',
  'Day',
  'Error',
  'Failed or discontinuous samples remain visible as chart gaps.',
  'Hour',
  'Incomplete quota history',
  'Last plotted value',
  'Latest observed interval',
  'Latest raw sample',
  'Line',
  'Maximum',
  'Metric',
  'Minimum',
  'No quota history data yet',
  'Observed consumption',
  'Observed coverage',
  'Please try again later.',
  'Quota history chart',
  'Quota trend',
  'Raw',
  'Refresh',
  'Response was truncated',
  'Samples',
  'Selected metric is unavailable',
  'Start',
  'Success',
  'Summary values start after the latest reset boundary.',
  'The upstream did not return a usable value for this metric.',
  'Time range',
  'Total quota',
  'Unable to load quota history',
  'Unavailable',
  'Unknown',
  'Unsupported',
  'Used quota',
  'Week',
  '{{count}} failed sample',
  '{{count}} interrupted interval',
  '{{count}} invalid sample',
  '{{count}} recovery event',
  '{{count}} reset boundary',
  '{{count}} unsupported sample',
] as const

const quotaAnalyticsKeys = [
  'Quota consumption',
  'Consumption trend',
  'Consumption',
  'Consumption rate',
  'Observed consumption',
  'Remaining quota',
  'Used quota',
  'Total quota',
  'Display mode',
  'Raw samples',
  '1 minute',
  '5 minutes',
  '15 minutes',
  'Hour',
  'Day',
  'Week',
  'Data coverage',
  'Successful samples',
  'Failed samples',
  'Reset detected',
  'Sampling failure',
  'Latest observation',
  'No usable quota observations in this range',
  'Sampling failures are shown as gaps.',
  'Change is measured in percentage points, not tokens or currency.',
  'Raw',
  'Latest',
  'Samples',
  'Last sample',
  'Minimum',
  'Maximum',
  'No quota history data yet',
  'Unable to load quota history',
  'Quota history chart',
  'Time range',
  'Chart granularity',
  'Metric',
  'Chart style',
  'Line',
  'Area',
  'Bar',
  'Detailed quota history',
  'Historical Codex rate-limit usage. Missing samples remain gaps.',
  'Codex usage history chart',
  'Provider quota sampling',
  'Enable automatic quota sampling',
  'Disable only if upstream balance requests should never run automatically.',
  'Sampling interval (minutes)',
  'Minimum 1 minute; the default is 1 minute.',
  'Maximum channels per run',
  'Caps each run to protect upstream services and local resources.',
  'Notification cooldown (seconds)',
  'Deduplicate repeated warning or critical notifications for this period. No notification is sent until a notifier is configured.',
  'Notify when quota recovers',
  'Record a healthy transition after warning or critical status; outbound delivery remains disabled.',
  'Automatically record provider and Codex account quota snapshots. Sampling is enabled by default and runs in the background.',
  'Critical threshold must be lower than warning threshold',
  'Enter a non-negative number or leave empty',
  'Account',
  'Bucket',
  'Change %',
  'Data quality',
  'Failed samples are never treated as zero.',
  'Largest provider account quota movements per minute',
  'Multi-key channels do not expose one combined account quota.',
  'Provider account',
  'Quota history',
  'Quota history is unavailable',
  'Quota history unavailable',
  'Some samples failed and are shown as gaps.',
  'The upstream did not return a usable quota value.',
  'The upstream did not return usable usage percentages.',
  'Window',
  'Codex account',
] as const

const quotaAnalyticsLabelsRequiringLocalization = [
  'Quota consumption',
  'Consumption trend',
  'Consumption rate',
  'Observed consumption',
  'Display mode',
  'Raw samples',
  'Data coverage',
  'Successful samples',
  'Failed samples',
  'Reset detected',
  'Sampling failure',
  'Latest observation',
  'No usable quota observations in this range',
  'Sampling failures are shown as gaps.',
  'Change is measured in percentage points, not tokens or currency.',
  'Raw',
  'Latest',
  'Samples',
  'Last sample',
  'No quota history data yet',
  'Unable to load quota history',
  'Quota history chart',
  'Time range',
  'Chart granularity',
  'Metric',
  'Chart style',
  'Line',
  'Area',
  'Bar',
  'Detailed quota history',
  'Historical Codex rate-limit usage. Missing samples remain gaps.',
  'Codex usage history chart',
  'Provider quota sampling',
  'Enable automatic quota sampling',
  'Disable only if upstream balance requests should never run automatically.',
  'Sampling interval (minutes)',
  'Minimum 1 minute; the default is 1 minute.',
  'Maximum channels per run',
  'Caps each run to protect upstream services and local resources.',
  'Notification cooldown (seconds)',
  'Deduplicate repeated warning or critical notifications for this period. No notification is sent until a notifier is configured.',
  'Notify when quota recovers',
  'Record a healthy transition after warning or critical status; outbound delivery remains disabled.',
  'Automatically record provider and Codex account quota snapshots. Sampling is enabled by default and runs in the background.',
  'Critical threshold must be lower than warning threshold',
  'Enter a non-negative number or leave empty',
  'Account',
  'Bucket',
  'Change %',
  'Data quality',
  'Failed samples are never treated as zero.',
  'Largest provider account quota movements per minute',
  'Multi-key channels do not expose one combined account quota.',
  'Provider account',
  'Quota history',
  'Quota history is unavailable',
  'Quota history unavailable',
  'Some samples failed and are shown as gaps.',
  'The upstream did not return a usable quota value.',
  'The upstream did not return usable usage percentages.',
  'Window',
  'Codex account',
] as const

describe('locale key parity', () => {
  it.each(localeResources)('%s contains every English translation key', (_locale, resource) => {
    const missing = baseKeys.filter((key) => !(key in resource.translation))
    expect(missing).toEqual([])
  })

  it.each(allLocaleResources)(
    '%s contains every quota analytics key',
    (_locale, resource) => {
      const missing = quotaAnalyticsKeys.filter(
        (key) => !(key in resource.translation)
      )

      expect(missing).toEqual([])
    }
  )

  it.each(allLocaleResources)(
    '%s contains every quota trend key',
    (_locale, resource) => {
      const missing = quotaTrendKeys.filter(
        (key) => !(key in resource.translation)
      )

      expect(missing).toEqual([])
    }
  )

  it.each(localeResources)(
    '%s does not fall back to English for quota analytics labels',
    (_locale, resource) => {
      const copiedEnglish = quotaAnalyticsLabelsRequiringLocalization.filter(
        (key) => resource.translation[key] === en.translation[key]
      )

      expect(copiedEnglish).toEqual([])
    }
  )

  it.each(localeResources)(
    '%s does not fall back to English for quota trend labels',
    (_locale, resource) => {
      const copiedEnglish = quotaTrendKeys.filter(
        (key) => resource.translation[key] === en.translation[key]
      )

      expect(copiedEnglish).toEqual([])
    }
  )
})
