/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { PoolUsage } from '../pool-usage-api'

export function poolUsageFixture(
  overrides: Partial<PoolUsage> = {}
): PoolUsage {
  return {
    pool_id: 7,
    snapshot_id: 9,
    revision: 1,
    channel_id: 1,
    channel_name: 'Subscription account',
    window_slot: 'primary',
    window_type: 'five_hour',
    window_seconds: 18000,
    plan_type: 'pro',
    generation: 1,
    state: 'active',
    reason: 'eligible',
    observed_at: 1100,
    reset_at: 2000,
    max_age_seconds: 300,
    sample_age_seconds: 60,
    allocated_bps: 6000,
    approximate: true,
    baseline_time: 1000,
    baseline_used_micro: 43000000,
    initial_baseline_time: 900,
    initial_baseline_used_micro: 35000000,
    observed_used_micro: 55000000,
    estimated_used_micro: 15000000,
    unattributed_micro: 40000000,
    unattributed_growth_micro: 5000000,
    decomposition_state: 'available',
    pending_attempts: 2,
    unsampled_attempts: 2,
    uncertain_attempts: 1,
    allocations: [
      {
        token_id: 21,
        token_name: 'Desktop app',
        token_state: 'enabled',
        share_bps: 1000,
        estimated_used_micro: 5000000,
        pending_attempts: 2,
        unsampled_attempts: 1,
        uncertain_attempts: 0,
      },
      {
        token_id: 22,
        token_name: 'Shared device',
        token_state: 'paused',
        share_bps: 4000,
        estimated_used_micro: 7000000,
        pending_attempts: 0,
        unsampled_attempts: 1,
        uncertain_attempts: 0,
      },
      {
        token_id: 23,
        token_name: 'Mobile app',
        token_state: 'enabled',
        share_bps: 1000,
        estimated_used_micro: 3000000,
        pending_attempts: 0,
        unsampled_attempts: 0,
        uncertain_attempts: 1,
      },
    ],
    ...overrides,
  }
}
