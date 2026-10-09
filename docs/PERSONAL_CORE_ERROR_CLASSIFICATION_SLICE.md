# Personal-core slice 2: typed funding-admission failures

Status: scoped implementation and regression verification in progress (2026-10-09).
Base: `ee97e2e8e8000b2d081449bc38998dd867c6e175`, the normal merge of PR #6.
Branch: `codex/subscription-admission-errors-20261009`.

## Actual boundary to improve

`FundingSource` already provides Source, PreConsume, Settle and Refund. This slice does not introduce a duplicate interface or call file movement architectural simplification.

The remaining concrete coupling is in `service/billing_session.go`: subscription business failures are recognized by substrings in `err.Error()`, and the resulting API error class can permit the existing wallet fallback. The corresponding normal business errors originate in `model/subscription.go`. A database failure whose diagnostic text contains a matching phrase must remain a failure, not become permission to charge another source.

The first gate is a deterministic regression proving this control-flow problem with an isolated database fixture. A suspected path or a screenshot is not proof of a production incident.

## Existing call graph and preserved boundaries

1. Token authentication checks the saved user policy against the authoritative funding mode, then the Key's independent strict-budget configuration.
2. `PreConsumeBilling` enters `NewBillingSession`.
3. Legacy mode claims a durable request and freezes its self-use source/revision before selecting a commercial funding fallback. Authoritative mode enters `ReserveAccountQuota`, whose transaction freezes source identity and its receipt/head.
4. The selected self-use source is checked against supported request paths before dispatch. Strict Token/USD qualification separately inspects the final transformed payload and selected upstream before reserving its budget.
5. Reliable returned usage settles the applicable budgets and ordinary internal quota; unknown evidence retains the existing hold. Settlement/refund/recovery uses the frozen source, rather than recomputing a new wallet policy.

Primary code: `middleware/auth.go`, `middleware/user_usage_policy.go`, `middleware/token_budget.go`, `controller/relay.go`, `service/billing_session.go`, `service/funding_source.go`, `service/token_budget_bound.go`, `service/text_quota.go`, `relay/channel/api_request.go`, `model/legacy_usage_reservation.go`, `model/account_quota_mutation.go`.

## Minimal implementation contract

- Define stable identities for no active subscription and insufficient subscription quota at their existing model origin; preserve their current error text.
- Use `errors.Is` at the service classification boundary. Trace existing callers and wrapping paths; preserve genuine business-error identity through wrapping.
- Preserve all four billing preferences, existing overflow permission checks, pre-consumption/compensation ordering, writer routing and durable transaction boundaries.
- Database and unknown errors must remain fail-closed even when their text resembles a business failure.
- No new persistent fields, billing arithmetic, protocol eligibility, user-data migration, payment flow, commercial-history removal, version/tag or deployment changes.

## Acceptance ledger

- [x] Red test proves both business-phrase collisions incorrectly enter wallet fallback: wallet 1000→900, Key 500→400 / used 100, durable journal prepared with wallet source and reserved 100, subscription records zero. The synthetic overflow query actually ran once. This is isolated regression evidence, not a production-cause claim.
- [x] Typed and wrapped genuine business failures preserve their existing eligible fallback behavior; forbidden overflow remains forbidden.
- [x] Unrelated database/unknown failures do not charge another source; existing compensation remains correct.
- [ ] Relevant self-use and both-writer contracts, targeted race checks and SQLite/MySQL/PostgreSQL CI pass on the exact candidate.
- [ ] Independent review checks every affected call/wrapping path and the unchanged transaction boundary.
- [ ] Accurate-source Draft PR CI and existing browser/release contracts pass; preserve failed evidence rather than rerunning unchanged for green.

## Local evidence and CI selection

The red collision test reaches the actual legacy subscription transaction and records the old wallet/Key/journal mutation before asserting failure. After typed classification, both text collisions and a generic storage outage return `update_data_error`, leave wallet 1000 / Key 500 / used 0, and retain only the failed-admission journal with zero reservation. Authoritative-writer storage failures are preservation coverage, not a claim that the same text-classification defect existed there.

All 32 related top-level regression tests passed locally, including the S2-A SQLite matrix. Targeted race passed for model and service after bounded test identities were corrected: user names are at most 20 characters and affiliate codes at most 32, with explicit fixture assertions. MySQL 5.7 and PostgreSQL 9.6 are still pending the exact candidate CI; SQLite is not substituted for them.

The four-case model contract (absent, exhausted, query failure, write failure) runs inside the existing `TestS2APaymentConfiguredDatabases` matrix. Ordinary root tests cover all new tests. The existing bounded race command explicitly selects the new model/service cases and old allowed/blocked overflow cases; a workflow contract test prevents accidental omission. No workflow permission, service, credential, timeout or publication gate changes.

## What this does not complete

PR #6 delivered personal-policy and Key-limit explanation/discoverability. The strict limits themselves already existed with their narrower model/protocol qualifications. This slice removes an actual error-text dependency, but does not complete a separate personal-core module or the full personal page → real relay → limit rejection → unknown recovery application journey. Those remaining items stay under [the parent card](PERSONAL_CORE_DELIVERY_CARD.md). No broad frontend dependency prohibition is added merely to claim more architectural change.
