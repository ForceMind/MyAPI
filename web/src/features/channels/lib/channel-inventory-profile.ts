import type { VisibilityState } from '@tanstack/react-table'

export type ChannelInventoryProfile = 'monitoring' | 'routing' | 'custom'

export const CHANNEL_INVENTORY_PROFILE_KEY = 'channels:inventory-profile'

const columns = ['id', 'name', 'type', 'status', 'models', 'group', 'tag', 'priority', 'weight', 'balance', 'response_time', 'test_time', 'actions']

/** Existing column IDs stay available; profiles only choose presentation. */
export function channelInventoryVisibility(profile: ChannelInventoryProfile): VisibilityState {
  if (profile === 'custom') return { models: false, tag: false }
  const visible = profile === 'monitoring'
    ? ['name', 'type', 'status', 'priority', 'weight', 'balance', 'response_time', 'actions']
    : ['name', 'type', 'status', 'models', 'group', 'tag', 'priority', 'weight', 'actions']
  return Object.fromEntries(columns.map((id) => [id, visible.includes(id)]))
}

export function channelInventoryStorage(profile: ChannelInventoryProfile) {
  return profile === 'custom'
    ? { visibility: 'channels:column-visibility', sizing: 'channels:column-sizing' }
    : { visibility: `channels:inventory:${profile}:column-visibility`, sizing: `channels:inventory:${profile}:column-sizing` }
}
