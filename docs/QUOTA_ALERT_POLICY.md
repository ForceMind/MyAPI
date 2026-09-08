# MyAPI quota alert policy

This policy layer is intentionally notifier-neutral. It evaluates normalized
quota history only; it never calls an SMTP server, webhook, provider billing
endpoint, or routing control. Outbound delivery remains disabled until a
future release selects an approved channel and stores its credentials through
the existing secret-management path.

## Configuration

`ChannelQuotaAlertSettings` is persisted atomically as the
`ChannelQuotaAlertSettings` option:

- `enabled`: opt-in threshold evaluation (default `false`).
- `warning_percent` / `critical_percent`: percentages of a provider-reported
  total; critical must be lower than warning.
- `cooldown_seconds`: repeated warning/critical reminder interval, bounded to
  seven days (default one hour; zero also falls back to the default for
  compatibility with older JSON options).
- `notify_on_recovery`: opt-in healthy transition after warning/critical
  (default `false`).

The equivalent environment defaults are
`CHANNEL_QUOTA_ALERT_ENABLED`, `CHANNEL_QUOTA_ALERT_WARNING_PERCENT`,
`CHANNEL_QUOTA_ALERT_CRITICAL_PERCENT`,
`CHANNEL_QUOTA_ALERT_COOLDOWN_SECONDS`, and
`CHANNEL_QUOTA_ALERT_NOTIFY_ON_RECOVERY`.

## Legacy transition preview

`common.EvaluateChannelQuotaAlertTransition` receives a redacted subject,
previous/current status, observation time, last-delivery time, and settings.
It returns one of:

- `threshold`: warning/critical status changed;
- `reminder`: the same status is still active after the cooldown;
- `recovery`: status became healthy and recovery is enabled;
- suppressed/no event: unknown, disabled, unavailable, or still inside the
  cooldown window.

The returned `dedup_key` is `<subject>:<status>` and `next_eligible_at` is
provided for suppressed reminders. This legacy preview remains compatible for
existing callers, but its permanent key is **not** a valid persistence or
delivery identity: directly persisting it would suppress a later
healthy→critical transition forever.

## P2A occurrence identity contract

`common.EvaluateChannelQuotaAlertOccurrenceV2` is a pure, notifier-neutral
contract for a later persistent pipeline. It accepts only bounded internal
references:

- `subject_ref` is exactly `channel:<positive canonical decimal id>` and
  `source_snapshot_ref` is exactly `snapshot:<positive canonical decimal id>`.
  Leading zero, zero, overflowing, URL-shaped, key-shaped, or arbitrary text
  references fail closed. The source snapshot ID represents one immutable
  normalized observation. These identifiers are internal redacted scope
  references, never names, URLs, provider payloads, API keys, credentials, or
  message bodies.
- `source_trusted` and `has_provider_total` must both be true. Failed,
  total-less, unknown, malformed, untrusted, disabled, or clock-inconsistent
  observations return an empty outcome and cannot create an event.
- previous/current statuses are restricted to `healthy`, `warning`, and
  `critical`; observation and last-delivery timestamps must be ordered.

Eligible `threshold`, `recovery`, and `reminder` outcomes receive a
`quota-alert-occurrence-v2:<sha256>` key. The digest includes the bounded
internal subject reference, immutable source-snapshot reference, resulting status, kind, and—for
reminders—the last confirmed delivery time as the cooldown-cycle identity. It
therefore converges on replay of the same trusted input while distinguishing a
later recovery/re-entry, a later source snapshot, and a later reminder cycle.
Suppressed cooldown outcomes have no event key and report only
`next_eligible_at`. If `last_delivered_at + cooldown_seconds` cannot fit in
signed 64-bit time, P2A fails closed with an empty outcome; it never wraps or
emits a negative eligibility time.

P2A has no database, outbox, worker, network, notifier, routing, configuration
write, or external provider behavior. It is not a persistent event model,
delivery guarantee, notification channel, administrator history, or real
channel validation. P2B must add scoped three-database persistence, authorized
recipient resolution, delivery state, retry/unknown semantics, audit history,
and a selected notifier before any outbound delivery is enabled.

## Deliberate boundaries

- A failed or total-less provider observation is `unavailable`; it never emits
  a low-balance event.
- No raw provider response, credential, URL, or message body is included in
  the event.
- This release does not send notifications, disable channels, or change
  routing. P2A only derives an occurrence identity. A future notifier must add
  explicit authorization, retry/backoff, rate limits, audit logging, and tests
  before enabling delivery.
