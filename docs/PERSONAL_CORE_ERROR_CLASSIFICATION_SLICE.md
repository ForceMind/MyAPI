# Personal-core slice 2: typed funding-admission failures

Status: bounded slice accepted and normally merged by PR #7 (2026-10-09); no new release or deployment.
Base: `ee97e2e8e8000b2d081449bc38998dd867c6e175`, the normal merge of PR #6.
Branch: `codex/subscription-admission-errors-20261009`.

## Actual boundary to improve

`FundingSource` already provides Source, PreConsume, Settle and Refund. This slice does not introduce a duplicate interface or call file movement architectural simplification.

The concrete coupling removed by this slice was in `service/billing_session.go`: subscription business failures were recognized by substrings in `err.Error()`, and the resulting API error class can permit the existing wallet fallback. The corresponding normal business errors originate in `model/subscription.go`. A database failure whose diagnostic text contains a matching phrase must remain a failure, not become permission to charge another source.

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
- [x] Relevant self-use and both-writer contracts, targeted race checks and SQLite/MySQL/PostgreSQL CI pass on the exact candidate.
- [x] Independent review checks every affected call/wrapping path and the unchanged transaction boundary.
- [x] Accurate-source Draft PR CI and existing browser/release contracts pass; preserve failed evidence rather than rerunning unchanged for green.

## Local evidence and CI selection

The red collision test reaches the actual legacy subscription transaction and records the old wallet/Key/journal mutation before asserting failure. After typed classification, both text collisions and a generic storage outage return `update_data_error`, leave wallet 1000 / Key 500 / used 0, and retain only the failed-admission journal with zero reservation. Authoritative-writer storage failures are preservation coverage, not a claim that the same text-classification defect existed there.

All 32 related top-level regression tests passed locally, including the S2-A SQLite matrix. Targeted race passed for model and service after bounded test identities were corrected: user names are at most 20 characters and affiliate codes at most 32, with explicit fixture assertions. The exact candidate CI subsequently passed the real MySQL 5.7 and PostgreSQL 9.6 four-case contract; SQLite was also rerun independently after recovery.

The four-case model contract (absent, exhausted, query failure, write failure) runs inside the existing `TestS2APaymentConfiguredDatabases` matrix. Ordinary root tests cover all new tests. The existing bounded race command explicitly selects the new model/service cases and old allowed/blocked overflow cases; a workflow contract test prevents accidental omission. No workflow permission, service, credential, timeout or publication gate changes.

## What this does not complete

PR #6 delivered personal-policy and Key-limit explanation/discoverability. The strict limits themselves already existed with their narrower model/protocol qualifications. This slice removes an actual error-text dependency, but does not complete a separate personal-core module or the full personal page → real relay → limit rejection → unknown recovery application journey. Those remaining items stay under [the parent card](PERSONAL_CORE_DELIVERY_CARD.md). No broad frontend dependency prohibition is added merely to claim more architectural change.


## Exact-source closure

- Source `026905e1cc9a3197c51bc6e80f81b07bdeac170d`, actual PR checkout `e34209dba8676ce3a0355066a5900b4f2ba45313`, equal tree `24698dc3f6e25d6215cc5ca6305e7c6d854520a8`.
- [Candidate CI 37967769004](https://github.com/ForceMind/MyAPI/actions/runs/37967769004): all 10 jobs successful; PR Check also successful. The expanded race command actually selected the new error-identity, preference and both-writer storage-failure cases. Root/relaykit tests, ordinary build/vet and the real MySQL/PostgreSQL contract passed.
- Normal merge `23df400bce84414782fe263db87f8bae61e7994e`, same tree. [Main CI 37970164427](https://github.com/ForceMind/MyAPI/actions/runs/37970164427): all 10 jobs passed; every checkout log and clean browser report identifies that main commit. Concurrent push run 37970160374 was cancelled by the existing concurrency policy, not manually rerun for green.
- Frontend 184 files / 1109 tests; shared browser 31 journeys / 236 screenshots, Playground 8 / 96. Main UI artifact `11636595376` original ZIP SHA256 `b655cb8684b121df0701f39d4b4f7e1a3381428d32f4419d7a23e6f4448c7c4d` was verified. No UI production change belongs to this slice. Docker smoke was skipped by its existing exact-branch gate; source distribution/release contracts passed.
- Executor recovery lost the original local implementation RED bytes. The tracked patch was byte-identical to the independently reviewed patch, and the independent race log survived. A new reproduction at 2026-10-09 17:43 UTC used unchanged `ee97` production plus the exact same regression function: both text collisions reproduced the old debit, while the generic storage-error control rejected. Fixed source passed all three cases without debit. New RED SHA256 `2c66007eb5b8f19a53e9bb4ef3598e85d859d282d7ba696e952ec88626dcfbd3`; GREEN `a6b03e95e85a0623dc130acdeebbc30380fab9f0fa19dce9e9bf7f69a0166649`. These are new observations, not claimed recovered originals.
