import type { ChannelFormValues } from './channel-form'

/** Connection inputs only. Never serialize this credential-bearing value outside the editor. */
export const CHANNEL_MODEL_SOURCE_FIELDS = [
  'type', 'base_url', 'key', 'openai_organization', 'other', 'setting', 'settings',
  'advanced_custom', 'header_override', 'proxy', 'http_protocol', 'http2_connection_shards',
  'aws_key_type', 'vertex_key_type', 'multi_key_mode', 'multi_key_type', 'key_mode',
] as const satisfies readonly (keyof ChannelFormValues)[]

export type ChannelModelSource = Pick<ChannelFormValues, (typeof CHANNEL_MODEL_SOURCE_FIELDS)[number]>

export function channelModelSource(values: ChannelFormValues): ChannelModelSource {
  return Object.fromEntries(CHANNEL_MODEL_SOURCE_FIELDS.map((field) => [field, values[field]])) as ChannelModelSource
}

export function sameChannelModelSource(left: ChannelModelSource, right: ChannelModelSource): boolean {
  return CHANNEL_MODEL_SOURCE_FIELDS.every((field) => Object.is(left[field], right[field]))
}

/** Preview only inputs represented by the existing draft endpoint; Advanced Custom also accepts its route, headers and proxy. */
export function canPreviewBasicChannelModels(source: ChannelModelSource, advanced = false): boolean {
  const empty = (value: unknown) => value == null || value === '' || value === '{}' || value === 'null'
  return [source.openai_organization, source.other, source.setting, source.settings, ...(advanced ? [] : [source.header_override, source.proxy, source.advanced_custom])].every(empty) &&
    (source.http_protocol === undefined || source.http_protocol === 'auto') &&
    (source.http2_connection_shards === undefined || source.http2_connection_shards === 1) &&
    (source.multi_key_mode === undefined || source.multi_key_mode === 'single')
}

/** Saved Advanced Custom preview retains the stored credential and unsupported transport inputs. */
export function canPreviewSavedAdvancedModels(source: ChannelModelSource, saved: ChannelModelSource): boolean {
  const supported = new Set<string>(['base_url', 'advanced_custom', 'header_override', 'proxy'])
  return CHANNEL_MODEL_SOURCE_FIELDS.every((field) => supported.has(field) || Object.is(source[field], saved[field]))
}
