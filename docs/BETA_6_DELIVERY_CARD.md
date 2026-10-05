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


## First exact-head CI and bounded follow-up (2026-10-05)

First source commit [`6b9ec98`](https://github.com/ForceMind/MyAPI/commit/6b9ec987a7cdb8fccdcd4f3fa2c9e5824eb04bff),
tree `49933c30e43b553eec960f2c123728f7151e7c40`, was pushed and read back.
Its PR test merge `6e2d929d1550fcf52b8ae8137bf15452eadf8920` has the same tree.

- [CI run](https://github.com/ForceMind/MyAPI/actions/runs/37343269172): nine jobs
  succeeded; Backend was cancelled at 15m17s, so this run is **not** an all-CI pass
- [Backend log](https://github.com/ForceMind/MyAPI/actions/runs/37343269172/job/111875406691):
  root vet 179s, root/relaykit tests 260s, publication-order race 168s,
  settings/request race 134s, route-identity race 85s, new beta.6 race 36s.
  All those stages passed. The following legacy billing/logging race was cancelled
  after 21s and the final durable-submission race skipped. No assertion failure was
  reported before cancellation. The serial job reached its former 15-minute budget
- The follow-up raises only Backend's bounded job orchestration allowance from
  15 to 20 minutes. Every test group, assertion, race detector, individual 180-second
  test deadline, production timeout, resource setting and permission is retained
- [Three-database job](https://github.com/ForceMind/MyAPI/actions/runs/37343269172/job/111875407207)
  passed actual SQLite/MySQL/PostgreSQL TokenBudget (including Chat context/zero/
  concurrency/hold/recovery) and OpenAIPriceCheck lease/fence/save/cancel contracts,
  plus existing R1, publication, migration, routing and account contracts
- [Frontend job](https://github.com/ForceMind/MyAPI/actions/runs/37343269172/job/111875407449)
  passed 131 files / 683 tests, typecheck/build and both real Chromium journeys.
  [Quota screenshots](https://github.com/ForceMind/MyAPI/actions/runs/37343269172/artifacts/11360155320)
  are synthetic. Independent pixel review passed the visible regions; the first
  320px source screenshot did not include the lower diff/review controls
- [Docker three jobs](https://github.com/ForceMind/MyAPI/actions/runs/37343269180)
  and [static website](https://github.com/ForceMind/MyAPI/actions/runs/37343269196)
  succeeded; no image was released or deployed

The bounded follow-up also makes existing translated labels visibly separate the
unchanged Responses scope from the single exact Chat model. Its semantic test,
34 focused frontend tests, typecheck/lint/format/build passed without locale or
money-path changes. The existing browser journey now actually wheel-scrolls to
the diff models/status/actions at 320/1280, opens saved-source review without a
publication write, and exercises schedule enable/disable at 320 with final state
off. Additional lower-viewport screenshots are required from its new exact-head
CI. Node syntax passed locally; local Chromium was not run.

The new accurate commit must complete **all** CI, including the previously cancelled
and skipped legacy groups. Preserve the first-run record; do not re-label cancellation
or a prior screenshot as final acceptance. Real account/invoice/deployment limitations
and the one-protocol stop boundary remain unchanged.


## Follow-up CI evidence and retained failure (2026-10-05)

Commit [`320b2e1`](https://github.com/ForceMind/MyAPI/commit/320b2e11a32db595dcdf9cdac1faec0c5db1d6ba)
completed [all Backend groups](https://github.com/ForceMind/MyAPI/actions/runs/37346213329/job/111885307582)
in 15m32s, including the previously cancelled billing/logging race (33s) and
skipped durable-submission race (27s). The 20-minute orchestration allowance is
therefore supported by an actual complete run, without weakening test deadlines
or production limits. Its three-database contracts, Docker and website also passed.
Frontend's 131 files / 684 tests and build/typecheck passed, but the newly added
browser reachability helper incorrectly required a fixed 75px bottom margin:
a visible review button ending at 826.5px failed the artificial 825px cutoff in a
900px viewport. The routing browser step was consequently skipped, not passed.

Commit [`5bf8382`](https://github.com/ForceMind/MyAPI/commit/5bf83828c430e641b7f43120a5b459ad088acacd)
changed only that browser helper. Actual viewport/ancestor clipping plus five
hit-test points replace the fixed margin; bounded native wheel interaction,
normal clicks, joint status/action visibility and no-publication assertions remain.
Independent static review found no material blocker. Its [Frontend job](https://github.com/ForceMind/MyAPI/actions/runs/37348369636/job/111892648608)
passed all 684 tests, typecheck/build and both real Chromium journeys. The
[40 synthetic quota screenshots](https://github.com/ForceMind/MyAPI/actions/runs/37348369636/artifacts/11360829834)
include lower diff models/status/actions, saved-source review at 320/1280, and
mobile source-check enable/disable controls ending disabled. Independent pixel
review found the previously missing lower mobile controls readable and unobscured.
Token recovery screenshots show reservation versus actual values, but omit the
top protocol scope and conservative small-request warning; dedicated top captures
are added in the following candidate, not claimed from these images.

The same 5bf8382 run passed nine CI jobs, actual three-database contracts,
[Docker three jobs](https://github.com/ForceMind/MyAPI/actions/runs/37348369517) and
[website](https://github.com/ForceMind/MyAPI/actions/runs/37348369593), but is **not**
an all-CI pass. [Backend](https://github.com/ForceMind/MyAPI/actions/runs/37348369636/job/111892648168)
failed in the complete root module test: the synthetic Chat EOF scenario expected
200 and received pre-dispatch 503 `token_budget_unavailable`; an adjacent async
audit message reported SQLite `readonly` (1032). Later race groups were skipped.
This failure remains recorded and must be diagnosed before acceptance; prior
Backend success is not a substitute for the final exact-head result.

The bounded diagnosis reproduced a specific test-isolation defect at CI's
`GOMAXPROCS=1`: the preceding channel permission fixture left six asynchronous
admin-audit workers running, owned only `model.DB` but not `model.LOG_DB`, and
closed/restored state before those workers finished. The following Chat fixture
accepted that nonzero worker baseline during its own cleanup, allowing work to
cross fixture lifetimes. Deleted-file audit writes (1032) were reproduced; the
original admission 503 was not independently reproduced and is not attributed
solely from an adjacent log line.

The next candidate changes only those two test fixtures: the predecessor owns a
synthetic Log table and both database handles, blocks real audit inserts until
cleanup, then drains and verifies all six rows before teardown; Chat drains to
zero before restoring its settings and database. The deterministic audit regression
was red when the new drain was removed (expected six rows, found zero). Endpoint,
origin, rate-limit, budget and unknown-retention assertions remain unchanged;
no production guard, reservation rule, test timeout or CI race group is relaxed.

Final fixture verification passed: complete router package at CI's single-CPU
setting (7.314s), three fresh-process predecessor-to-Chat runs (0.729s / 0.725s /
0.878s), Chat-only ten repetitions (7.313s), and affected race checks (6.958s,
180-second test ceiling). Temporary diagnostics were removed. The top protocol
scope/warning screenshot extension also passed Node syntax and independent static
review. These local checks do not replace the next exact-head full CI.
