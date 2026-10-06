import { z } from 'zod'

import type { AssignedPolicy, PolicyCandidate } from '../types'

function parsePolicyScope(text: string): string[] {
  return [
    ...new Set(
      text
        .split(/[\n,]/)
        .map((item) => item.trim())
        .filter(Boolean)
    ),
  ]
}

const dimension = z.object({ inherit: z.boolean(), text: z.string() })
export const policyFormSchema = z
  .object({
    enabled: z.boolean(),
    public: dimension,
    upstream: dimension,
    channels: dimension,
  })
  .superRefine((value, ctx) => {
    for (const name of ['public', 'upstream', 'channels'] as const) {
      if (value[name].inherit) continue
      const items = parsePolicyScope(value[name].text)
      if (items.length > 128) {
        ctx.addIssue({
          code: 'custom',
          path: [name, 'text'],
          message: 'scope_limit',
        })
      }
      if (name === 'channels') {
        if (
          items.some(
            (item) =>
              !/^[1-9]\d*$/.test(item) || !Number.isSafeInteger(Number(item))
          )
        ) {
          ctx.addIssue({
            code: 'custom',
            path: [name, 'text'],
            message: 'invalid_channels',
          })
        }
      } else if (
        items.some(
          (item) =>
            new TextEncoder().encode(item).length > 255 ||
            [...item].some((character) => {
              const code = character.codePointAt(0) ?? 0
              return (
                code <= 0x1f ||
                (code >= 0x7f && code <= 0x9f) ||
                /\p{Cf}/u.test(character)
              )
            })
        )
      ) {
        ctx.addIssue({
          code: 'custom',
          path: [name, 'text'],
          message: 'invalid_models',
        })
      }
    }
  })
export type PolicyFormValues = z.infer<typeof policyFormSchema>

export function policyFormDefaults(policy: AssignedPolicy): PolicyFormValues {
  return {
    enabled: policy.assigned ? policy.enabled : true,
    public: {
      inherit: policy.public_models === null,
      text: policy.public_models?.join('\n') ?? '',
    },
    upstream: {
      inherit: policy.upstream_models === null,
      text: policy.upstream_models?.join('\n') ?? '',
    },
    channels: {
      inherit: policy.channel_ids === null,
      text: policy.channel_ids?.join('\n') ?? '',
    },
  }
}

export function policyCandidate(values: PolicyFormValues): PolicyCandidate {
  const list = (dimension: PolicyFormValues['public']): string[] | null => {
    if (dimension.inherit) return null
    return parsePolicyScope(dimension.text)
  }
  const channels = list(values.channels)
  return {
    enabled: values.enabled,
    public_models: list(values.public),
    upstream_models: list(values.upstream),
    channel_ids: channels?.map(Number) ?? null,
  }
}
