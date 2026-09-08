/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

export type RoutingQuotaState = 'fresh' | 'stale' | 'unknown' | 'exhausted'

export interface ChannelRoutingPolicy {
  enabled: boolean
  sticky_enabled: boolean
  session_ttl_seconds: number
  quota_max_age_seconds: number
}

export interface ChannelRoutingQuota {
  state: RoutingQuotaState
  available?: number
  unit?: string
  observed_at?: number
  comparison_key?: string
}

export interface RoutingChannel {
  id: number
  name: string
  type: number
  status: number
  priority: number
  legacy_priority?: string
  legacy_weight?: string
  weight: number
  quota: ChannelRoutingQuota
}

export interface ChannelRoutingData {
  policy: ChannelRoutingPolicy
  channels: RoutingChannel[]
}

export interface ChannelRoutingResponse {
  success: boolean
  message?: string
  data?: ChannelRoutingData
}

export interface RoutingPreviewRequest {
  model: string
  group: string
  path: string
}

export interface RoutingPreviewCandidate {
  id: number
  name: string
  share: number
  reason: string
  available?: number
  unit?: string
}

export interface RoutingPreviewResponse {
  success: boolean
  message?: string
  data?: {
    workload: 'chat' | 'work' | 'balanced'
    reason: string
    candidates: RoutingPreviewCandidate[]
  }
}

export interface RoutingChannelUpdate {
  priority: number
  weight: number
  expected_priority: number
  expected_legacy_priority?: string
  expected_legacy_weight?: string
  expected_weight: number
}

export interface RoutingMutationResponse {
  success: boolean
  message?: string
}
