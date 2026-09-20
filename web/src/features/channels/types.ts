/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { z } from 'zod'

// ============================================================================
// Channel Schema & Types
// ============================================================================

export const channelInfoSchema = z.object({
  is_multi_key: z.boolean().default(false),
  multi_key_size: z.number().default(0),
  multi_key_status_list: z.record(z.string(), z.number()).optional(),
  multi_key_disabled_reason: z.record(z.string(), z.string()).optional(),
  multi_key_disabled_time: z.record(z.string(), z.number()).optional(),
  multi_key_polling_index: z.number().default(0),
  multi_key_mode: z.enum(['random', 'polling']).default('random'),
})

export type ChannelInfo = z.infer<typeof channelInfoSchema>

export const channelSchema = z.object({
  id: z.number(),
  type: z.number(),
  key: z.string(),
  openai_organization: z.string().nullish(),
  test_model: z.string().nullish(),
  status: z.number(), // 1: enabled, 0: manual disabled, 2: auto disabled
  name: z.string(),
  weight: z.number().nullish(),
  created_time: z.number(),
  test_time: z.number(),
  response_time: z.number(), // in milliseconds
  base_url: z.string().nullish(),
  other: z.string().default(''),
  balance: z.number().default(0), // in USD
  balance_updated_time: z.number(),
  models: z.string().default(''),
  group: z.string().default('default'),
  used_quota: z.number().default(0),
  model_mapping: z.string().nullish(),
  status_code_mapping: z.string().nullish(),
  priority: z.number().nullish(),
  auto_ban: z.number().nullish(),
  other_info: z.string().default(''),
  tag: z.string().nullish(),
  setting: z.string().nullish(),
  param_override: z.string().nullish(),
  header_override: z.string().nullish(),
  remark: z.string().default(''),
  max_input_tokens: z.number().default(0),
  channel_info: channelInfoSchema.default({
    is_multi_key: false,
    multi_key_size: 0,
    multi_key_polling_index: 0,
    multi_key_mode: 'random',
  }),
  settings: z.string().default('{}'), // other_settings JSON
})

export type Channel = z.infer<typeof channelSchema>

// ============================================================================
// Channel Settings Types
// ============================================================================

export interface ChannelSettings {
  force_format?: boolean
  thinking_to_content?: boolean
  proxy?: string
  pass_through_body_enabled?: boolean
  system_prompt?: string
  system_prompt_override?: boolean
  http_protocol?: 'auto' | 'http1' | string
  http2_connection_shards?: number
}

export interface ChannelOtherSettings {
  azure_responses_version?: string
  vertex_key_type?: 'json' | 'api_key'
  openrouter_enterprise?: boolean
  aws_key_type?: 'ak_sk' | 'api_key'
  allow_service_tier?: boolean
  disable_store?: boolean
  allow_safety_identifier?: boolean
  allow_include_obfuscation?: boolean
  allow_inference_geo?: boolean
  allow_speed?: boolean
  claude_beta_query?: boolean
  disable_task_polling_sleep?: boolean
  upstream_model_update_check_enabled?: boolean
  upstream_model_update_auto_sync_enabled?: boolean
  upstream_model_update_ignored_models?: string[]
  upstream_model_update_last_check_time?: number
  upstream_model_update_last_detected_models?: string[]
  advanced_custom?: AdvancedCustomConfig
}

export interface AdvancedCustomConfig {
  advanced_routes?: AdvancedCustomRoute[]
}

export interface AdvancedCustomRoute {
  incoming_path?: string
  upstream_path?: string
  converter?: AdvancedCustomConverter
  models?: string[]
  auth?: AdvancedCustomRouteAuth
}

export interface AdvancedCustomRouteAuth {
  type?: AdvancedCustomAuthType
  name?: string
  value?: string
}

export type AdvancedCustomConverter =
  | 'none'
  | 'anthropic_messages_to_openai_chat_completions'
  | 'openai_chat_completions_to_anthropic_messages'
  | 'openai_chat_completions_to_openai_responses'
  | 'openai_responses_to_openai_chat_completions'
  | 'openai_responses_to_gemini_generate_content'
  | 'gemini_generate_content_to_openai_chat_completions'
  | 'openai_chat_completions_to_gemini_generate_content'

export type AdvancedCustomAuthType = 'none' | 'header' | 'query'

// ============================================================================
// API Response Types
// ============================================================================

export interface GetChannelsResponse {
  success: boolean
  message?: string
  data?: {
    items: Channel[]
    total: number
    page: number
    page_size: number
    type_counts?: Record<string, number>
  }
}

export interface SearchChannelsResponse {
  success: boolean
  message?: string
  data?: {
    items: Channel[]
    total: number
    type_counts?: Record<string, number>
  }
}

export interface GetChannelResponse {
  success: boolean
  message?: string
  data?: Channel
}

export interface ChannelOpsResponse {
  success: boolean
  message?: string
  data?: {
    retry_times: number
  }
}

export interface ChannelCommitState {
  committed?: boolean
  cache_pending?: boolean
  cache_enabled?: boolean
  data_generation?: number
  published_generation?: number
  cluster_committed_epoch?: number
  local_published_epoch?: number
  code?: string
}

export interface ChannelMutationResponse<T = never> extends ChannelCommitState {
  success: boolean
  message?: string
  data?: T
}

export interface ChannelRoutingPreviewParams {
  group: string
  model: string
  request_path?: string
}

export interface ChannelRoutingPreviewCandidate {
  id: number
  name: string
  type: number
  weight: number
  effective_weight: number
  expected_share: number
}

export interface ChannelRoutingPreviewTier {
  priority: number
  fallback_index: number
  channels: ChannelRoutingPreviewCandidate[]
}

export interface ChannelRoutingPreviewResponse {
  success: boolean
  code?: string
  message?: string
  data?: {
    group: string
    model: string
    request_path: string
    tiers: ChannelRoutingPreviewTier[]
    source: 'cache' | 'database'
    generation: number
    data_generation: number
    published_generation: number
    cluster_committed_epoch: number
    local_published_epoch: number
    cache_enabled: boolean
    cache_pending: boolean
    affinity: {
      evaluated: false
      precedence: 'before_priority_weight'
      explanation_code: 'routing_preview_affinity_not_evaluated'
    }
  }
}

export interface ChannelTestResponse {
  success: boolean
  message?: string
  error_code?: string
  time?: number
  data?: {
    response_time?: number
    error?: string
  }
}

export interface ChannelBalanceResponse {
  success: boolean
  message?: string
  balance?: number
  currency?: string
  raw_response?: string
}

export type ChannelQuotaHistoryRange =
  | '1h'
  | '6h'
  | '24h'
  | '7d'
  | '30d'
  | '90d'
  | 'custom'

export type ChannelQuotaHistoryGranularity =
  | 'raw'
  | 'minute'
  | '5m'
  | '15m'
  | 'hour'
  | 'day'
  | 'week'
  | 'auto'

export type ChannelQuotaHistoryMetric =
  | 'available'
  | 'used'
  | 'total'
  | 'consumption'
  | 'rate_per_minute'

export interface ChannelQuotaHistoryPoint {
  timestamp: number
  /** Actual time of the observation retained for this display bucket. */
  observed_at?: number
  status: string
  available?: number
  used?: number
  /** Whether `used` came from the provider or was safely derived. */
  used_source?: 'reported' | 'derived'
  total?: number
  reset_at?: number
  error_code?: string
  /** Redacted origin of a non-success point, for diagnostics only. */
  event_source?: string
  /** Number of raw observations represented by this display bucket. */
  sample_count?: number
  success_count?: number
  failed_count?: number
  unsupported_count?: number
  /** The bucket crosses a provider-reported quota reset boundary. */
  reset?: boolean
  /** A value must not be connected to its predecessor in the chart. */
  continuity_break?: boolean
  /** Server-derived intervals, assigned to the bucket containing their end. */
  consumption?: number
  rate_per_minute?: number
  peak_rate_per_minute?: number
  observed_seconds?: number
  interval_count?: number
  gap?: boolean
  recovery?: boolean
  baseline_change?: boolean
}

export interface ChannelQuotaHistoryMetricSummary {
  start?: number
  end?: number
  change?: number
  change_percent?: number
  minimum?: number
  maximum?: number
  samples?: number
}

export interface ChannelQuotaHistoryConsumptionSummary {
  /** Consumption observed only across continuous, successful sample pairs. */
  observed?: number
  basis?: 'used' | 'available' | string
  pair_count?: number
  reset_boundaries?: number
  recovery_count?: number
  interrupted_count?: number
  unit?: string
  observed_seconds?: number
  average_rate_per_minute?: number
  peak_rate_per_minute?: number
  peak_rate_observed_at?: number
  gap_count?: number
  baseline_change_count?: number
  allocation?: 'interval_end'
}

export type ChannelQuotaAnalysisMethod =
  | 'latest_interval'
  | 'observed_window'
  | 'ewma'

export type ChannelQuotaETAOutcome =
  | 'depletes_before_reset'
  | 'reset_before_depletion'
  | 'stable_or_no_observed_consumption'
  | 'insufficient_data'

export interface ChannelQuotaETA {
  outcome: ChannelQuotaETAOutcome
  estimated_depletion_at?: number
  seconds_to_depletion?: number
  reset_at?: number
  seconds_until_reset?: number
}

export interface ChannelQuotaRateMethodAnalysis {
  rate_per_minute: number | null
  rate_per_hour: number | null
  coverage: number
  observed_seconds: number
  interval_count: number
  observed_at: number | null
  eta: ChannelQuotaETA
}

export interface ChannelQuotaAnalysis {
  default_method: 'observed_window'
  rate_window_seconds: number
  window_start: number
  window_end: number
  as_of: number
  ewma_half_life_seconds: number
  complete: boolean
  methods: Record<ChannelQuotaAnalysisMethod, ChannelQuotaRateMethodAnalysis>
}

export interface ChannelQuotaOverviewPoint {
  timestamp: number
  available: number | null
  continuity_break: boolean
}

export interface ChannelQuotaHistorySummary {
  start_available: number
  end_available: number
  change: number
  change_percent: number
  minimum: number
  maximum: number
  drop_rate_per_day?: number | null
  forecast_zero_at?: number | null
  forecast_confidence?: 'high' | 'low' | 'insufficient'
  data_quality?: ChannelQuotaHistoryDataQuality
  /** Metric-specific figures prevent a selected chart metric using availability figures. */
  available?: ChannelQuotaHistoryMetricSummary
  used?: ChannelQuotaHistoryMetricSummary
  total?: ChannelQuotaHistoryMetricSummary
  consumption?: ChannelQuotaHistoryConsumptionSummary
}

export interface ChannelQuotaHistoryDataQuality {
  sample_count?: number
  success_count: number
  error_count: number
  unsupported_count?: number
  invalid_count: number
  reset_boundaries: number
  observed_span_seconds?: number
  span_seconds: number
}

export interface ChannelQuotaHistoryAlert {
  enabled: boolean
  status: 'disabled' | 'unavailable' | 'healthy' | 'warning' | 'critical'
  ratio_percent?: number
  warning_percent: number
  critical_percent: number
  cooldown_seconds?: number
  notify_on_recovery?: boolean
}

export interface ChannelQuotaHistoryData {
  channel_id: number
  start: number
  end: number
  limit: number
  granularity?: ChannelQuotaHistoryGranularity
  timezone_offset?: number
  /** Stable, redacted identifier of the resolved provider quota series. */
  series_id?: string
  /** Raw observations found in the requested period before chart bucketing. */
  raw_observations?: number
  raw_observation_limit?: number
  /** Buckets available before and after the response point cap. */
  available_points?: number
  returned_points?: number
  /** No raw observations were skipped while producing the response. */
  source_complete?: boolean
  /** No display buckets were skipped while producing the response. */
  points_complete?: boolean
  /** True only when the requested range was represented in full. */
  complete?: boolean
  truncated?: boolean
  truncation_reason?: 'raw_observation_limit' | 'point_limit' | string
  points: ChannelQuotaHistoryPoint[]
  analysis?: ChannelQuotaAnalysis
  summary?: ChannelQuotaHistorySummary
  data_quality?: ChannelQuotaHistoryDataQuality
  alert?: ChannelQuotaHistoryAlert
  current?: {
    available?: number
    used?: number
    used_source?: 'reported' | 'derived'
    total?: number
    reset_at?: number
    observed_at: number
    status: string
    error_code?: string
    event_source?: string
  } | null
  unit?: string
  currency?: string
  metric_type?: string
  window_type?: string
  source?: string
  plan_type?: string
  window_seconds?: number
  series?: {
    id: string
    metric_type?: string
    window_type?: string
    source?: string
    plan_type?: string
    unit?: string
    currency?: string
    window_seconds?: number
  }
}

export interface ChannelQuotaHistoryResponse {
  success: boolean
  message?: string
  data?: ChannelQuotaHistoryData
}

/**
 * One provider-account quota change, normalized by the server from the latest
 * two trustworthy snapshots. This is intentionally distinct from MyAPI user
 * balance/quota; it describes the upstream account attached to a channel.
 */
export interface ChannelQuotaChangeItem {
  channel_id: number
  name: string
  account_label?: string
  metric_type?: string
  window_type?: string
  source?: string
  plan_type?: string
  window_seconds?: number
  unit?: string
  currency?: string
  current_available?: number | null
  current_total?: number | null
  previous_available?: number | null
  change_per_minute?: number | null
  abs_change_per_minute?: number | null
  direction?: 'increase' | 'decrease' | 'stable' | 'unknown' | string
  sample_span_seconds?: number | null
  observed_at?: number
  previous_observed_at?: number | null
  status?: 'success' | 'unavailable' | 'unsupported' | 'error' | string
  /** Read-only threshold state derived from the latest normalized snapshot. */
  alert?: ChannelQuotaHistoryAlert
  data_quality?: ChannelQuotaHistoryDataQuality
  consumption?: ChannelQuotaHistoryConsumptionSummary
  analysis?: ChannelQuotaAnalysis
  overview_points?: ChannelQuotaOverviewPoint[]
  peak_abs_change_per_minute?: number
  peak_drop_per_minute?: number
  peak_increase_per_minute?: number
}

export interface ChannelQuotaChangesData {
  items: ChannelQuotaChangeItem[]
  range?: string
  generated_at?: number
  rate_window_seconds?: number
  ewma_half_life_seconds?: number
  data_quality?: ChannelQuotaHistoryDataQuality
  total_items?: number
  returned_items?: number
  source_complete?: boolean
  items_complete?: boolean
}

export interface ChannelQuotaChangesResponse {
  success: boolean
  message?: string
  data?: ChannelQuotaChangesData
}

export interface ChannelQuotaSamplingStatusData {
  enabled: boolean
  interval_seconds: number
  max_channels: number
}

export interface ChannelQuotaSamplingStatusResponse {
  success: boolean
  message?: string
  data?: ChannelQuotaSamplingStatusData
}

export interface ChannelQuotaAlertDeliveryStatus {
  policy_enabled: boolean
  configured: boolean
  endpoint_host?: string
  https_only: boolean
  redirects_allowed: boolean
  timeout_ms: number
  max_attempts: number
}

export interface ChannelQuotaAlertDeliveryEvent {
  id: number
  event_key: string
  snapshot_id: number
  channel_id: number
  status: string
  kind: string
  state: string
  attempt_count: number
  next_attempt_at?: number
  last_error_code?: string
  last_error_at?: number
  delivered_at?: number
  observed_at: number
  created_at: number
  updated_at: number
}

export interface ChannelQuotaAlertDeliveryEventsResponse {
  success: boolean
  message?: string
  data?: {
    items: ChannelQuotaAlertDeliveryEvent[]
    total: number
    page: number
    page_size: number
  }
}

export interface ChannelQuotaAlertDeliveryStatusResponse {
  success: boolean
  message?: string
  data?: ChannelQuotaAlertDeliveryStatus
}

export interface ChannelQuotaAlertDeliveryRunSummary {
  enabled: boolean
  claimed: number
  delivered: number
  retryable: number
  quarantined: number
}

export interface ChannelQuotaAlertDeliveryRunResponse {
  success: boolean
  message?: string
  data?: ChannelQuotaAlertDeliveryRunSummary
}

export interface FetchModelsResponse {
  success: boolean
  message?: string
  data?: string[]
}

export type CopyChannelResponse = ChannelMutationResponse<{
  id: number
  ids: number[]
  replayed: boolean
}>

// ============================================================================
// Multi-Key Management Types
// ============================================================================

export interface KeyStatus {
  index: number
  status: number // 1: enabled, 2: manual disabled, 3: auto disabled
  disabled_time?: number
  reason?: string
  key_preview?: string
}

export type MultiKeyConfirmAction = {
  type:
    | 'enable'
    | 'disable'
    | 'delete'
    | 'enable-all'
    | 'disable-all'
    | 'delete-disabled'
  keyIndex?: number
}

export interface MultiKeyStatusResponse {
  success: boolean
  message?: string
  data?: {
    keys: KeyStatus[]
    total: number
    page: number
    page_size: number
    total_pages: number
    enabled_count: number
    manual_disabled_count: number
    auto_disabled_count: number
  }
}

// ============================================================================
// API Request Parameters
// ============================================================================

export type ChannelSortBy =
  | 'id'
  | 'name'
  | 'priority'
  | 'weight'
  | 'balance'
  | 'response_time'
  | 'test_time'

export type ChannelSortOrder = 'asc' | 'desc'

export interface GetChannelsParams {
  p?: number
  page_size?: number
  status?: string // 'enabled', 'disabled', or empty for all
  type?: number
  group?: string
  id_sort?: boolean
  tag_mode?: boolean
  sort_by?: ChannelSortBy
  sort_order?: ChannelSortOrder
}

export interface SearchChannelsParams {
  keyword?: string
  group?: string
  model?: string
  status?: string
  type?: number
  id_sort?: boolean
  tag_mode?: boolean
  sort_by?: ChannelSortBy
  sort_order?: ChannelSortOrder
  p?: number
  page_size?: number
}

export interface ChannelTestParams {
  test_model?: string
}

export interface CopyChannelParams {
  suffix?: string
  reset_balance?: boolean
  operation_key?: string
}

export interface MultiKeyManageParams {
  channel_id: number
  action:
    | 'get_key_status'
    | 'disable_key'
    | 'enable_key'
    | 'enable_all_keys'
    | 'disable_all_keys'
    | 'delete_key'
    | 'delete_disabled_keys'
  key_index?: number
  page?: number
  page_size?: number
  status?: number // 1=enabled, 2=manual_disabled, 3=auto_disabled
}

export interface BatchDeleteParams {
  ids: number[]
}

export interface BatchSetTagParams {
  ids: number[]
  tag: string | null
}

export interface TagOperationParams {
  tag: string
  new_tag?: string
  priority?: number
  weight?: number
  model_mapping?: string
  models?: string
  groups?: string
}

// ============================================================================
// Form Data Types
// ============================================================================

export interface ChannelFormData {
  name: string
  type: number
  base_url: string
  key: string
  openai_organization?: string
  models: string
  group: string
  model_mapping?: string
  priority?: number
  weight?: number
  test_model?: string
  auto_ban?: number
  status: number
  status_code_mapping?: string
  tag?: string
  remark?: string
  setting?: string
  param_override?: string
  header_override?: string
  settings?: string
  other?: string
  // Multi-key specific
  multi_key_mode?: 'single' | 'batch' | 'multi_to_single'
  multi_key_type?: 'random' | 'polling'
  batch_add_set_key_prefix_2_name?: boolean
}

// ============================================================================
// Add Channel Request (special structure)
// ============================================================================

export interface AddChannelRequest {
  mode: 'single' | 'batch' | 'multi_to_single'
  multi_key_mode?: 'random' | 'polling'
  batch_add_set_key_prefix_2_name?: boolean
  channel: Partial<Channel>
}

export type CodexLocalAuthState =
  | 'ready'
  | 'container_host_unavailable'
  | 'home_unavailable'
  | 'not_found'
  | 'unreadable'
  | 'unsafe_file'
  | 'too_large'
  | 'changed_during_read'
  | 'invalid_json'
  | 'unsupported_auth_method'
  | 'incomplete_credential'
  | 'lan'
  | 'disabled'
  | 'unknown'

/** Non-secret state of the Codex login owned by the MyAPI process user. */
export interface CodexLocalAuthStatus {
  state: CodexLocalAuthState
  platform: 'darwin' | 'linux' | 'windows' | 'unknown'
  environment: 'native' | 'container' | 'lan' | 'unknown'
  codex_installed: boolean
  cli_version?: string
  auth_file_exists: boolean
  auth_readable: boolean
  logged_in: boolean
  auto_import_available: boolean
  manual_import_available: boolean
  account_hint?: string
  email_hint?: string
  last_refresh?: string
  can_refresh: boolean
}
