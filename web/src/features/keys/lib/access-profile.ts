import type { TFunction } from 'i18next'

import type { AccessProfileMetadata } from '../types'

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
