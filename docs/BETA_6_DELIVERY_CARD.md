# beta.6: native Chat text budget qualification

## Status and boundary

2026-10-05: qualification and implementation received independent static review;
local backend/frontend verification passed; exact-new-head CI acceptance remains pending. Base `ab8da0ef2feb429022f7b0e4ae1447c66d9f1b41`
is the verified beta.5 documentation head. Preserve beta.3–5 behavior and evidence.
The development branch and Draft PR #2 continue; no main merge, tag, release,
deployment, real account or paid API call is part of this candidate. VERSION and
default images remain the published beta.3 until separately authorized release.

This is the bounded next step in [the master plan](MYAPI_MASTER_PLAN.md#beta6实际费用与严格预算扩展),
not an additional gateway, ledger, provider pricing service or full UI redesign.

## Qualification contract

- First additional protocol: native OpenAI `POST https://api.openai.com/v1/chat/completions`.
  First exact model: `gpt-6.1-sol`. Compatible proxies, other models, model-prefix
  guesses, tools, media, prediction and alternate service tiers are not qualified.
- The final outgoing request must retain verified TLS, exact official destination,
  exact routed model identity, no parameter/header overrides, plain-text messages,
  `n` omitted or `1`, and explicit positive `max_completion_tokens <= 128000`.
  Streaming requires `stream_options.include_usage=true`. USD also requires final
  `service_tier=default`; channel filtering cannot be mistaken for that evidence.
  Unsupported strict requests fail before dispatch. Ordinary keys keep their
  existing compatible behavior.
- Chat has no documented Responses-style input-token counting endpoint. Local
  tokenizers and message-framing estimates do not become actual usage. The model's
  documented context ceiling is **1,050,000 total input plus completion tokens**;
  completion includes reasoning and hidden formatting. With one completion,
  reserve that conservative total ceiling, retaining a separate explicit output
  cap. A small request can be refused when the remaining budget cannot cover the
  ceiling; the UI must distinguish reservation bounds from measured usage.
- The existing reservation journal gains a distinct bound source. Chat settlement
  requires nonnegative actual input <= context, actual output <= cap, and their
  sum <= context. Existing Responses exact-count equality and frozen v1 evidence
  stay unchanged. Unknown, incomplete, contradictory, out-of-bound or interrupted
  usage retains the original reservation and uses existing Root-audited idempotent
  recovery. No timeout, absent field or failed parse means automatic zero/refund.
- Native response JSON must be unambiguous, retain raw counter presence, and match
  the exact model and applicable tier. Explicit zero is evidence; missing/null is
  not. Cache read and cache write are disjoint input subsets and must add to no
  more than prompt tokens. Reasoning is already included in completion. USD
  requires explicit cache-read and cache-write counters, including valid zeros.
- Reuse the existing saved official source, Root publication, lock and rollback
  contract. Saved source != published price != budget qualification. This model
  requires both current short/long price profiles, with the existing 272,000-input
  boundary. A Chat v2 evidence envelope freezes both; actual prompt count selects
  the settlement profile, never today's mutable publication.
- For context ceiling W and output cap M, let I be the greatest frozen input,
  cached-input or cache-write rate across both profiles and O the greatest output
  rate. The USD reservation is `(W*I + M*max(0,O-I))/1,000,000`, using exact decimals.
  This follows from P+C<=W and C<=M. The checked 2026-10-05 source gives I=5 and O=15,
  or $6.53 at M=128,000. These are source-based Standard usage costs, not reconciled
  invoices. Subscription channels retain equivalent-cost reference status only.
- Public model aliases must still pass original public/actual-target permissions,
  exact final target qualification and exact frozen pricing. Discovery alone
  grants no model permission or financial qualification.

## Price source checks

Reuse the existing SystemTask scheduler, database lease, fixed official URL and
bounded safe client. Checks are default-off; once explicitly enabled, cadence is
24 hours. A manual check uses the same deduplicated task. Freshness becomes stale
after 72 hours without a successful check. Last successful check time is distinct
from immutable source creation time and price publication time: an unchanged hash
can still have a new successful check.

Checks save source evidence and expose differences for Root review. Fetch failure,
lost lease or late completion preserves the last good evidence and all effective
prices/locks. No automatic price publication, zero-price fallback, lock override
or rollback. Local/CI tests use synthetic sources; development does not enable a
live schedule or create an external automation.

## Source evidence

Public documentation checked 2026-10-05, without account/API calls:

- [Exact model and limits](https://developers.openai.com/api/docs/models/gpt-6.1-sol)
- [Chat request and usage schema](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)
- [Context includes input, output and reasoning](https://developers.openai.com/api/docs/guides/conversation-state#managing-the-context-window)
- [Output-token categories](https://developers.openai.com/api/docs/guides/token-counting#understand-output-token-counts)
- [Official Standard prices](https://developers.openai.com/api/docs/pricing)
- [Prompt cache categories](https://developers.openai.com/api/docs/guides/prompt-caching)

The fixed [Markdown source](https://developers.openai.com/api/docs/pricing.md) was
retrieved with bounded HTTPS on 2026-10-05: SHA-256
`0d9fb6b240209bb7b5a05d1d6ce0b8a796e0924b7e34c5ab183a3f1f890febe6`.
The existing runtime fetcher/parser and publication-candidate builder subsequently
passed against the same hash (40 source models, 7.32 seconds). Exact headings, unit,
bare model label, eight quoted rates and the single 272K qualification were checked.
Runtime saved-source/publication validation remains mandatory for each admission;
this public documentation read did not publish any running price.

## Acceptance and stop condition

The actual application flow must protect explicit zero, cache overlap, exact target
and tier, alias permissions, absent or contradictory usage, stream interruption,
concurrent limit exhaustion, frozen in-flight short/long prices, publication changes,
rollback and Root idempotent recovery. Source checks must cover disabled/due/manual,
unchanged/new/failed source, freshness, lease loss and permission isolation.

Required evidence on the exact final head: root tests/build/vet and affected race,
independent relaykit, SQLite/MySQL/PostgreSQL contracts, frontend tests/typecheck/build,
seven languages and 320/1280 Chromium on the existing `/keys`, `/usage-logs/common`,
`/channels` and `/system-settings/models/openai-pricing-source` entries. Do not
reuse beta.5 green CI as new-head evidence.

Stop after this one protocol's qualified closed loop and source-review workflow.
Real provider bill reconciliation, real accounts, production upgrade/rollback,
media/audio/image/Realtime, third-party automatic estimates and per-Key Codex
percentage accounting remain outside the candidate and explicitly unverified.

## Implementation review and local evidence

Independent review verified the bounded contract and then the money path. The
review corrections are protected by direct regressions: swallowed final SSE writes
(and deadline forwarding), Chat-to-Responses qualification bypass, unexpected audio
settlement bypass, non-null error envelopes, and mismatched service tiers.

The new actual application HTTP fixture exposed an existing initialization-order
problem: strict selected-channel validation ran before channel metadata existed.
The controller now captures the already-selected channel for strict keys before
that validation; final outbound qualification remains unchanged. Independent
review found no weakened guard. The fixture covers native Chat and existing native
Responses v1 through real Gin admission, ephemeral verified-TLS synthetic upstream,
Root publication and recovery APIs, native adapters, reservation and logs.

Current completed local evidence (not exact-head CI):

- 19 synthetic HTTP scenarios passed, including cache/reasoning, explicit zero,
  unknown and non-text holds, exact target/permissions, budget exhaustion,
  in-flight publication and rollback, and Responses v1 count/price preservation
- Full root Go tests (69 tested packages), vet/build, independent `GOWORK=off`
  relaykit tests/build/vet, and affected model/service/controller/adapter/router
  race checks passed. SQLite configured contracts passed; MySQL/PostgreSQL were
  explicitly skipped locally until the disposable database CI runs
- 76 affected frontend tests, seven-language real rendering, typecheck, scoped
  lint/format, i18n sync and production build passed; final small status-label and
  request-field wording adjustments passed a further 21-test/typecheck/build run
- The final-usage/DONE write-failure regression was first red, then green after the
  checked writer observer; existing ordinary formatting is preserved
- Initial combined SQLite fixture reused a Token ID from the old fee contract;
  assigning a separate fixture ID fixed it without removing assertions. An old
  admission-only route test was updated to expect Chat admission while preserving
  pending/exhausted/query/media rejection; native financial gates have separate
  actual application coverage. Missing local godotenv cache was restored from the
  official Go registry at the existing locked version; dependency manifests did not change

MySQL/PostgreSQL, actual Chromium and complete exact-new-head CI are not yet claimed.
No synthetic fixture is evidence of a paid upstream call, reconciled provider invoice
or target deployment. All source-check schedules remain disabled during development.


## Operator entry points

- Root enables `OpenAIOfficialPriceCheckEnabled` in the existing official source
  panel. It defaults to false. `GET /api/ratio_sync/openai/check` reads status;
  `POST /api/ratio_sync/openai/check` with `{}` requests the deduplicated manual
  check. Existing Root authentication, origin checks and write rate limits apply
- Enable the channel's existing `allow_service_tier` setting when sending the
  explicitly qualified `service_tier=default`; filtered/absent tier cannot acquire
  strict USD qualification. No channel option is changed automatically
- Native Chat messages currently require role/content string pairs; `max_tokens`,
  tools, media, arbitrary overrides, extra request controls and automatic
  Chat-to-Responses conversion stay outside this strict slice. Existing ordinary
  compatible requests keep their behavior
- A source-check freshness warning does not overwrite or automatically publish a
  price. Inspect the retained version and differences, then use the existing
  separate Root publication/lock/rollback controls
- Read reservation versus actual counts in Key pending details, administrator
  usage-log details and the existing recovery panel. Absent actual values remain
  unconfirmed, including after interrupted or contradictory provider evidence
