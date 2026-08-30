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

## Deterministic event rules

`common.EvaluateChannelQuotaAlertTransition` receives a redacted subject,
previous/current status, observation time, last-delivery time, and settings.
It returns one of:

- `threshold`: warning/critical status changed;
- `reminder`: the same status is still active after the cooldown;
- `recovery`: status became healthy and recovery is enabled;
- suppressed/no event: unknown, disabled, unavailable, or still inside the
  cooldown window.

The returned `dedup_key` is `<subject>:<status>` and `next_eligible_at` is
provided for suppressed reminders. Callers must persist delivery state with a
channel-scoped key and update it only after a notifier confirms delivery.

## Deliberate boundaries

- A failed or total-less provider observation is `unavailable`; it never emits
  a low-balance event.
- No raw provider response, credential, URL, or message body is included in
  the event.
- This release does not send notifications, disable channels, or change
  routing. A future notifier must add explicit authorization, retry/backoff,
  rate limits, audit logging, and tests before enabling delivery.
