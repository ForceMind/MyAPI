import type { TFunction } from 'i18next'

import type { AccessProfileMetadata } from '../types'

/**
 * Shape used by the key profile picker when it needs to retain an existing
 * profile which is no longer returned by the current user's selectable-group
 * endpoint.  This is deliberately a read-only compatibility affordance: it
 * never makes the profile available during key creation.
 */
export type PreservedAccessProfileOption = {
  value: string
  label: string
  desc: string
  profileId?: string
}

/**
 * Build an option for an already persisted profile that is temporarily absent
 * from the selectable profile list.  Editing an old key must not silently
 * replace its routing group merely because policy metadata or eligibility has
 * changed.  Returning null for empty/already-present values keeps creation and
 * normal update behaviour unchanged.
 */
export function getPreservedAccessProfileOption(
  options: Array<{ value: string }>,
  group: string | null | undefined,
  profile: AccessProfileMetadata | null | undefined,
  t: TFunction
): PreservedAccessProfileOption | null {
  const value = group ?? ''
  if (!value.trim() || options.some((option) => option.value === value)) {
    return null
  }

  return {
    value,
    label: `${getAccessProfileLabel(value, profile ?? undefined, t)} · ${t(
      'Existing key'
    )}`,
    desc: [
      getAccessProfileDescription(value, profile ?? undefined, t),
      t(
        'Retained for compatibility; choose another profile only if you intend to change this key.'
      ),
    ]
      .filter(Boolean)
      .join(' · '),
    profileId: profile?.id,
  }
}

/**
 * Translate the stable access-profile identity while keeping legacy group
 * values usable. The backend deliberately exposes metadata as a compatibility
 * layer; unknown/custom groups remain readable instead of being hidden.
 */
export function getAccessProfileLabel(
  group: string | undefined,
  profile: AccessProfileMetadata | undefined,
  t: TFunction
): string {
  const id = normalizeAccessProfileId(group, profile)
  switch (id) {
    case 'standard':
      return `${t('Standard access')} (default)`
    case 'priority':
      return `${t('Priority access')} (vip)`
    case 'automatic':
    case 'auto':
      // Keep the legacy value visible in a compact suffix so existing keys
      // can still be identified while the product language stays explicit.
      return `${t('Automatic routing')} (auto)`
    default:
      return profile?.label || group || t('Standard access')
  }
}

export function getAccessProfileDescription(
  group: string | undefined,
  profile: AccessProfileMetadata | undefined,
  t: TFunction
): string {
  const id = normalizeAccessProfileId(group, profile)
  const configuredDescription = profile?.description?.trim()
  if (configuredDescription) return configuredDescription
  switch (id) {
    case 'standard':
      return t('Uses the standard channel pool and billing rules.')
    case 'priority':
      return t('Uses the priority channel pool when your account allows it.')
    case 'automatic':
    case 'auto':
      return t(
        'Tries eligible channel groups in order and can fail over when enabled.'
      )
    default:
      return (
        profile?.description || t('Uses the channels assigned to this profile.')
      )
  }
}

export function getAccessProfilePolicyHint(
  profile: AccessProfileMetadata | undefined,
  t: TFunction
): string {
  if (!profile) return ''
  const parts: string[] = []
  if (profile.route_groups?.length) {
    parts.push(`${t('Routes')}: ${profile.route_groups.join(', ')}`)
  }
  if (profile.model_allowlist?.length) {
    parts.push(`${t('Models')}: ${profile.model_allowlist.join(', ')}`)
  }
  if (profile.fallback_profiles?.length) {
    parts.push(`${t('Fallback')}: ${profile.fallback_profiles.join(', ')}`)
  }
  if (profile.enabled === false) parts.push(t('Disabled'))
  return parts.join(' · ')
}

function normalizeAccessProfileId(
  group: string | undefined,
  profile: AccessProfileMetadata | undefined
): string {
  const rawId = profile?.id || group || 'standard'
  if (rawId === 'default' || rawId === '') return 'standard'
  if (rawId === 'vip') return 'priority'
  return rawId
}
