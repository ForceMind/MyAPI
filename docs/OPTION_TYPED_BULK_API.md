# Typed Bulk Options API (C09-N3a, frozen)

Status: frozen by the C09-N3a backend batch. The frontend form merge
(C09-N3b) must code against this contract. Both endpoints sit in the existing
admin option route group and require root authentication.

## Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| PUT | `/api/option/typed-bulk` | Single-writer typed bulk update with optional revision CAS |
| GET | `/api/option/typed-bulk/revision` | Current persistent revision for form loading |

## PUT request

```json
{
  "expected_revision": 3,
  "items": [
    {"key": "general_setting.docs_link", "type": "string", "value": "https://docs.example.com"},
    {"key": "general_setting.ping_interval_enabled", "type": "boolean", "value": true},
    {"key": "general_setting.ping_interval_seconds", "type": "number", "value": 120},
    {"key": "qwen.sync_image_models", "type": "string_list", "value": ["z-image", "custom-x"]}
  ]
}
```

- The body is decoded strictly: unknown top-level fields and trailing JSON
  values are rejected (400).
- `expected_revision` is optional, an integer `>= 0`. When present, the whole
  request commits only if the persistent revision equals it; otherwise the
  request fails with 409 and writes nothing. When absent, no CAS check runs.
- `items` must contain 1..64 entries (`MaxTypedBulkItems`).
- `key` is a `<family>.<field>` managed option key, at most 256 bytes.
  Duplicate keys are rejected (400). Unknown keys are rejected (400): the
  family must be allow-listed and the field must exist in the family's
  exported configuration map. This endpoint never silently accepts
  unrecognized keys; the legacy single-key PUT keeps its permissive behavior.
- `type` is a closed union: `"string"`, `"number"`, `"boolean"`,
  `"string_list"`. The JSON type of `value` must match the declared type
  exactly (a quoted number is not a `number`).

### Canonical value forms (what gets persisted)

| type | canonical string form | bounds |
| --- | --- | --- |
| `string` | the string itself (JSON text for map/object fields) | <= 65536 bytes |
| `number` | integer lexeme as-is; float via `FormatFloat('f', -1, 64)` | finite, abs <= 1e15 |
| `boolean` | `"true"` / `"false"` | — |
| `string_list` | JSON array encoding, `[]` for null | <= 256 elements, each <= 1024 bytes, encoded <= 65536 bytes |

After canonicalization every item passes the existing per-key validator
(`validateOptionValue`) and the whole-family validator
(`config.ValidateConfigFromMap`) before anything is written. Any failure
aborts the request with zero writes to DB, runtime, and OptionMap.

## Managed families (allow-list)

`access_profile_setting`, `billing_setting`, `checkin_setting`, `claude`,
`console_setting`, `discord`, `fetch_setting`, `gemini`, `general_setting`,
`global`, `grok`, `group_ratio_setting`, `legal`, `monitor_setting`, `oidc`,
`passkey`, `perf_metrics_setting`, `qwen`, `quota_setting`, `token_setting`,
`tool_price_setting`.

Excluded on purpose:

- `performance_setting` — hot-path decision family pending D14.
- `channel_affinity_setting` — channel/retry/billing decision family pending D15.
- `payment_setting` — owned by the payment funding bulk (`PUT
  /api/option/payment-funding`); compliance fields are never editable here.
- `user_funding_setting` — owned by the funding state machine.

The legacy flat keys (`Notice`, `ModelRatio`, `GroupRatio`, rate limit keys,
payment gateway keys, `ServerAddress`, ...) are intentionally not part of this
key space. The `group_ratio_setting.group_ratio` /
`group_ratio_setting.group_group_ratio` aliases are accepted and are persisted
and published together with their canonical flat keys so readers never observe
two meanings for one setting.

## Responses

Success (200):

```json
{"success": true, "message": "", "data": {"revision": 4, "applied": ["general_setting.docs_link", "..."]}}
```

`revision` is the new persistent revision; `applied` is the sorted list of
persisted keys (group ratio writes include the canonical alias keys).

Errors:

| Status | Condition | Notes |
| --- | --- | --- |
| 400 | malformed body, unknown/duplicate key, item bounds, invalid value | i18n message (`option.typed_bulk_*`), offending key/reason embedded |
| 409 | `expected_revision` mismatch | zero writes; `data.revision` carries the current revision for reload |
| 500 | database failure | nothing was committed; DB/runtime/OptionMap all unchanged |
| 500 | post-commit publication abort (`option.typed_bulk_publish_failed`) | see below |

## Single-writer flow

1. Full normalize + validate of every item (any failure: zero writes).
2. `optionMutationLock` serializes the commit with all legacy writers and
   reloads.
3. One database transaction: revision row read (row lock where supported),
   expected-revision CAS check, per-key upserts in sorted order, single
   guarded revision bump (`UPDATE ... WHERE key=? AND value=?`, exactly one
   row must match). Any failure rolls the whole transaction back.
4. After the commit, one runtime publication per family in sorted family
   order: MapConfig families store one immutable generation; families that
   delegate to the config reflection layer apply one whole-candidate `Set`.
   OptionMap entries for a family are written only after that family's
   publication succeeded.

## Publication abort semantics (post-commit boundary)

If a family publication fails after the database commit, the whole group
aborts: the failure is recorded through `SysError`, the failed family and all
later families keep their previous runtime generation and OptionMap values,
and the endpoint returns 500 with `option.typed_bulk_publish_failed`.
Families published before the failure keep their new generation and the
committed database state stands — the same documented boundary as the payment
funding bulk. This is never presented as a cross-family atomic runtime
commit.

External side effects (the billing pricing/exposed-data cache invalidations)
run after the commit and are not part of the database atomicity: they are
idempotent in-memory projections and there is nothing to roll back. External
provider operations are never enlisted in the transaction.

## Revision mechanism

The revision is a persistent monotonic counter stored in the reserved options
row `typed_bulk_revision` (absent = 0). The options table has no
updated_at/version column and adding one would not be maintained by the
legacy writers, so a dedicated counter row is the minimal persistent
implementation. It is bumped exactly once per committed typed bulk inside the
same transaction, guarded by a conditional update so the database remains the
final CAS arbiter even across processes; `optionMutationLock` serializes
in-process writers. The counter row is never published to OptionMap and is
skipped by option reloads.

The legacy single-key PUT and the payment funding bulk do not bump this
counter: the revision versions the typed bulk write stream that merged admin
forms (C09-N3b) CAS on, and those forms must use this endpoint exclusively.
Forms that also save through legacy endpoints must reload the revision
afterwards.

## GET response

```json
{"success": true, "message": "", "data": {"revision": 4}}
```

## i18n keys

`option.typed_bulk_invalid_body`, `option.typed_bulk_unknown_key`,
`option.typed_bulk_duplicate_key`, `option.typed_bulk_invalid_value`,
`option.typed_bulk_item_count`, `option.typed_bulk_revision_conflict`,
`option.typed_bulk_publish_failed`, `option.typed_bulk_internal`
(en / zh-CN / zh-TW; template data: `Key`, `Reason`, `Max`, `Expected`,
`Actual`).
