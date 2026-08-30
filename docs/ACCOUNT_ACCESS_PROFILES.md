# MyAPI account tiers and Key access profiles

MyAPI keeps two concepts separate even while old installations continue to
send and store `group`:

- **Account tier** (`users.account_tier_id`) describes what a user account is
  entitled to: quota, eligible channels and enabled features.
- **Key access profile** (`tokens.access_profile_id`) describes how one API Key
  is allowed to call: routing pool, model scope and fallback guidance.

The legacy values `default`, `vip` and `auto` remain accepted and are mapped to
the stable profile/tier IDs by `model.ResolveAccessProfile` and
`model.ResolveAccountTier`. Existing routing still uses the legacy group until
an explicit migration enables policy enforcement.

## Configure profile guidance

Root administrators can open **Billing → Group Pricing → Key access profile
policies**. The JSON object is keyed by stable profile ID:

```json
{
  "priority": {
    "label": "Priority access",
    "description": "Reserved team pool",
    "route_groups": ["vip"],
    "model_allowlist": ["gpt-5"],
    "fallback_profiles": ["standard"],
    "enabled": true
  }
}
```

The registry is persisted as `access_profile_setting.profiles`. Invalid JSON,
empty IDs/labels and self-referential fallbacks are rejected atomically. The
configured metadata is returned by `/api/user/self/groups` and shown in Key
creation/list views; it does not expose credentials and does not silently
reject existing Keys.

