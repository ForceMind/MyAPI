# Personal-core slice 3: real application journey, first bounded part

Status: implementation and independent review in progress (2026-10-09). Real Docker/Chromium acceptance is pending.
Base: `23df400bce84414782fe263db87f8bae61e7994e`, normal PR #7 merge; branch `codex/personal-app-journey-20261009`.
This implements the first part of step 4 in [the personal-core card](PERSONAL_CORE_DELIVERY_CARD.md). It does not complete the full recovery/concurrency plan or publish a new version.

## Actual missing link

Existing `probeSelfUseRelayFixture` already exercised a real isolated HTTP application: a zero-wallet Root, finite Key, loopback relay, request-correlated consume log, user counters and Key balances. Existing browser fixtures instead supplied synthetic application API responses. The missing link is an actual browser using the application's setup/login, policy, Key and Playground controls before observing the real persisted result.

Reuse the existing application image and `tools/runtime/fake-openai.mjs`; simulate only the upstream. Do not intercept application APIs with fixture responses or inject authentication/selected Key state. Ordinary display quota remains price-converted internal units, not strict actual-token accounting.

## First-part contract

1. Real setup wizard and password login. Verify the running source build identity. A fresh Root sees disabled commercial funding, its explicit no-wallet policy and zero wallet.
2. Create a finite ordinary Key through the actual page, select that exact Key in Playground, set output limit through its control and send the default streaming request. Verify the response, request ID, one upstream count increment, one associated consume log, user counters and finite Key debit. Reload reads the same persisted result without sending again.
3. A separately logged-in ordinary owner (role 1) starts with zero wallet and no explicit no-wallet policy. Its finite Key request rejects without upstream dispatch/debit. Root uses the real policy confirmation UI; owner reads the new revision and the same Key then succeeds. The ordinary owner cannot mutate another user's policy.
4. A low ordinary Key allowance rejects without a new upstream call or committed usage. This is the existing allowance boundary, not a new claim of strict concurrent hard-budget semantics.
5. Enable Token and USD budgets on a test Key, then submit the unqualified loopback/smoke-model/request combination. It must reject and leave upstream count, budget reservation and persisted usage unchanged. This is not proof that the host check alone caused rejection, nor a native-TLS positive qualification.
6. Run two independent fresh owned SQLite application instances: legacy and authoritative. Real English1280 Root and Chinese320 ordinary-owner controls are the initial full-chain representatives. Existing seven-language layout/browser CI remains, but is not described as seven-language real-server accounting acceptance.

## Isolation and writer setup

- Only a newly created owned test DB/container and literal loopback targets. Existing/non-SQLite instances reject before setup writes. No user server, production credential or paid upstream call.
- The application retains the existing smoke default bridge with only explicit host-loopback HTTP mappings; before process start, the authoritative application also joins a private internal Redis network. Redis is connected only to that internal network. The authoritative case uses the official Redis 7 Alpine digest `sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499`, observed in the successful prior Redis CI. No host Redis port; ephemeral test-only password is masked, and both source/run labels guard finally cleanup.
- Select the authoritative writer only through the existing status, bounded drain, plan and apply APIs, with explicit single-instance acknowledgement and epoch readback. Preserve legacy → bridge → authoritative ordering. No DB state rewrite, audit waiver or fallback when an audit fails.
- The exact same-repository personal branch is added only to the existing build-smoke gate; fork protection, read-only workflow permissions and `push:false` remain. It receives two fresh cases, not the historical handoff case. Other branches/manual scenarios keep their existing installer/handoff paths.
- This personal job sets restore=0: browser reload is tested, but process restart/DB restore is not claimed here. Existing independent restore contracts remain separate.
- Browser traffic continues to the actual same-origin application; external browser requests abort. No trace/auth-storage dumps or Key-reveal endpoint. Screenshots are taken after login with input/Key/identity masking; reports contain fixed stages, numeric checks, source/writer and relative screenshot names, never raw external errors or credentials.

## Evidence ledger

- [x] Streaming fixture contract first rejected the real default streaming shape (400); the bounded upstream now returns fixed text, complete 10/5/15 usage and `[DONE]`, while retaining the existing nonstream response and rejecting missing usage opt-in.
- [x] Writer fixture unit contracts preserve API transition order, epoch checks, freshness, no-inflight/audit refusal and secret-free reports.
- [x] Runtime contracts: 74/74, including 28 browser orchestration/security contracts using doubles only. They are not counted as browser or ledger acceptance. Existing CLI tests and workflow YAML/shell syntax checks also passed.
- [x] Independent final review of cleanup, isolation, writer gates and browser/API bindings. Initial review caught swallowed browser-close failures; after a red test, successful-journey cleanup failures now produce a fixed safe error, while an earlier failure remains primary. Independent browser-driver contracts: 28/28 passed.
- [ ] Exact candidate ordinary CI and real two-writer Docker/Chromium runs; preserve any first failure and original artifacts.
- [ ] Verify actual checkout/merge tree against PR source, source-bearing reports, artifact ZIP digests and representative pixels.

No production Go/React behavior is intentionally changed. If real integration reveals a product defect, record a separate red regression and review the smallest correction before claiming success.

## Remaining parent-card work

Concurrent ordinary admission must follow its existing contract, not silently acquire strict semantics. Policy revocation during in-flight work, unknown usage held across restart and exactly-once recovery remain subsequent bounded parts. A strict positive real-browser fixture requiring a special test binary/private CA/proxy needs a separately frozen isolation design; no production hook or system trust change is included. Commercial history and existing obligations stay intact, and the existing FundingSource interface is retained.

## First real Docker attempt (2026-10-09)

Source `ed85840ea57bb3a20fce3aaec2aaa9c6ca0b0460`, PR merge checkout `f0fb1eb5da602fa1b8f03012168ff8ca3db22220`, run [37974629688](https://github.com/ForceMind/MyAPI/actions/runs/37974629688): both writer cases failed host-loopback fake-upstream readiness before setup or Chromium. Application and fixture containers remained running without OOM; cleanup completed. No application journey passed. An internal-only network suppressing published mappings is a working diagnosis, not a production finding; the first logs did not collect namespace-local fixture health or effective port mappings. The correction restores the established application bridge topology, retains internal-only Redis, verifies actual loopback mappings immediately, and emits only safe namespace-local readiness comparison on failure.

## Second real Docker attempt (2026-10-09)

Source `54acf7e050dafe64cf1207da0f2303bac714ac78`, run [37975745814](https://github.com/ForceMind/MyAPI/actions/runs/37975745814): effective loopback port mappings and fake-upstream readiness passed in both writer cases. Real Chromium then failed during setup, before writer transition or any personal relay assertions. Both original logs are retained. Inspection and a same-version Base UI Radio DOM reproduction found that the setup mode's explicit ID addresses a visually hidden input; the driver now clicks the visible named radio role and checks its selected state. Fixed, allowlisted setup substages improve diagnosis without logging page contents, credentials or arbitrary errors. Real setup success remains pending the next exact-head run.

## Third real Docker attempt (2026-10-09)

Source `8bc3328d5324e7470fecdf71067334aae57c3db0`, merge checkout `39310e0b5054784e9babb2cf29e5ad61990025cb` has the same tree `7c554abe36fd40393a19384baf7f1e4db3b54d92`. Run [37976914483](https://github.com/ForceMind/MyAPI/actions/runs/37976914483) passed real setup/login, writer setup and the existing HTTP relay probes in both cases, then failed the first personal Key-create assertion. The driver had required explicit `group=default` without selecting it, while the real form starts with the inherited empty group. The correction selects Standard access through the actual control and retains precise payload/persisted allowance checks; controlled switches are awaited before reading state. Fixed Key-create substages and a report-whitelist equality test keep future failures diagnosable without raw data. Both original failed logs and the first actual policy screenshot are retained; personal browser relay acceptance is still pending.

The same pass checked remaining driver contracts against current Key, Playground, policy and budget components, including ordinary-owner separate login/context and model selection; backend Token/self/policy/budget/log DTOs and writer-specific refusal codes were inspected separately. These source/double checks do not substitute for the next real Docker/Chromium run.
