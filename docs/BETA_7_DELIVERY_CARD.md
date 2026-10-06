# beta.7 delivery card: assigned access and in-app quota events

## Bounded PostgreSQL catalog correction (2026-10-06)

The owner-menu correction `e33be806` passed the unchanged real Chromium access
and event journey, alongside the existing quota and routing journeys. Database
qualification remains a separate gate.

The PostgreSQL experiment identified the expensive metadata join directly:
the old constraint-column branch scanned 6,214 catalog attribute rows 34 times,
rejecting about 8.93 million constraint/column pairs. The upstream tightened
join limits it to the target relation's 40 attributes once. This preserves the
returned metadata while removing the measured catalog scan amplification.

The repository now selects PostgreSQL driver v1.5.4 with its declared GORM
v1.25.5 requirement. The dependency patch changes only those two version pins
and adds their four checksum entries; pgx remains v5.9.2. An earlier isolated
v1.5.3 experiment was rejected during review because its quoted-default parser
adds repeated SET DEFAULT operations for existing nonempty string defaults,
including quota units/status and fee strings. No unsupported GORM downgrade or
local driver fork is used.

The unchanged PostgreSQL 9.6 B2 contract passes in 27.247 seconds with the
selected pair under the same 180-second deadline. The new real-model migration
contract fails v1.5.3 and passes v1.5.4: repeated AutoMigrate preserves raw server
default metadata, complete stored rows and values supplied by omitted-column
INSERTs, with no repeated SET DEFAULT for unchanged fields. It covers quota
snapshots, Token/USD budgets and reservations, including empty-string defaults.
Its owned schema cannot clean up existing fixture tables. The same contract is
included in CI; complete root, SQLite/MySQL 5.7/PostgreSQL 9.6 and exact-head
qualification are still required before closure. Fee formulas, reservation,
settlement and recovery authorization are unchanged; no prior failure is erased.

## Exact-head findings and owner-menu correction (2026-10-06)

The complete source candidate `03d0c67f` was published through separate fixture,
assignment, event and UI checkpoints. Its first real Chromium access journey
completed administrator preview/save/conflict/revocation and account-event
screens, then failed on the ordinary owner's first Key menu opening. The
existing asynchronous key reveal updated provider state; an inline table-cell
component was recreated and unmounted the open menu. Three real-provider/table/
dialog regressions reproduce the failure before the correction. A stable
`DataTableRowActions` cell reference preserves the menu, first-click action and
keyboard focus through pending, successful and failed reveal. The owner-only
reveal request and authorization behavior are unchanged. All 97 related tests,
typecheck, scoped lint/format and build passed; independent review cleared the
two-file change. The unchanged real browser journey must still pass on its new
published head; no automatic reclick, pre-reveal, timeout increase or reduced
assertion was introduced.

The intermediate event checkpoint `616b00b6` passed nine CI jobs, but B2 failed
at its original 180-second limit both initially and in its single same-head
retry. Both traces wait in the PostgreSQL driver's catalog introspection during
AutoMigrate after earlier startup migrations have populated the schema. The
first AutoMigrate inside the B2 helper is line 132; its repeat is line 136.
The later quota/event database group was skipped. The full UI candidate also
failed B2, so no complete beta.7 database or CI pass is claimed.

A disposable local PostgreSQL 9.6.24 reproduction also exhausted the unchanged
180-second deadline. Server logs show many completed catalog queries taking
roughly one to two seconds each and sustained backend CPU, rather than evidence
of a single query blocked for the whole deadline. On the same catalog, the
original metadata join took 1509 ms, the upstream tightened join took 5 ms and
the original again took 2495 ms; returned metadata rows were identical. A
separate alternate-modfile driver experiment completed the existing B2 contract
in 44.585 seconds. Dependency/default-normalization review and final three-engine
qualification remain pending; no timeout, migration assertion or financial rule
was relaxed. These new observations do not retroactively prove the cause of
beta.6's separately retained catalog-query timeout.

## Recovery and fresh local qualification (2026-10-06)

The original local candidate and verification logs were lost in a workspace
rollback. All 82 candidate files were reconstructed from durable uploaded Git
blobs and the recorded final corrections. Their computed tree exactly matches
`71c8bccebecd0a37fcc9c27621518c7825c3b75f`. No beta.7 commit or branch update had
been published; the durable branch baseline remains `0f4829d`. Earlier local
results below are historical reports, not surviving logs or current evidence.

Fresh verification of the recovered source passed the 62 focused frontend tests,
136 files / 746 full frontend tests, typecheck, scoped lint/format, production
build and all seven locale key sets. The first full Vitest command passed, but
its logging wrapper had an EOF error; that attempt is retained separately and
an immutable-wrapper full rerun completed with an overall zero exit. The
formatter helper's initial invocation error was also retained before its clean
read-only 33-file check. No frontend source or dependency lockfile changed.

The fresh root run found one existing beta.6 fixture error under two CPUs:
`TestManagedGlobalSettingsPublishesWholeGeneration` omitted `all_channels` in
its first update. After the second update set it true, correct partial-update
semantics retained true, creating a legitimate state outside the fixture's two
expected generations. Unchanged baseline hashes, an untouched failing run and
a deterministic second-to-first overlay reproduce this cause. The test-only
correction explicitly supplies false and asserts both complete transitions
before the original concurrent checks. Production settings, partial-merge
semantics, the writer barrier, write/observation counts and the whole-generation
assertion are unchanged. Independent review and focused/full-package/race
checks passed; this is not a production torn-publication fix.

After that fixture correction, a fresh complete root run passed all 69 tested
packages. Root vet/build, affected access/event races and independent
`GOWORK=off` relaykit tests/build/vet passed. The pre-correction model-proof
overlay again reproduced HTTP 403 for strict Responses and Chat, including the
unknown-hold case; the recovered correction passed the service/router/transport
checks without restoring `GetBody` or changing settlement rules. SQLite
lifecycle, reopen, event persistence and concurrent contracts ran locally.

Current Chromium launch independently failed before page creation with
`socket() failed: Operation not permitted` in its process-singleton setup.
No local browser journey or screenshot is claimed, and that route was stopped
without escalation or retries. Actual disposable MySQL 5.7 / PostgreSQL 9.6,
real Chromium journeys and all exact-published-head CI remain required.
Publication is still pending; recovery and local checks do not close beta.7.

## Baseline and bounded outcome

Source baseline: `0f4829db1df43e906522c1f52869db79d9dea7de`, original
`codex/r1-usage-review-20261002` / Draft PR #2. This is a new source candidate,
not a release, deployment, main merge or a claim about real accounts.
The beta.6 final PostgreSQL catalog-query timeout and successful same-head retry
remain in its evidence; no test deadline or financial rule is relaxed here.

One task: an administrator narrows user/Key public-model and upstream scope;
the owner can see available public models and safe rejection reasons; a real
request obeys the public and final target constraints; revocation blocks later
admission/dispatch despite stale caches; administrators can inspect same-account
quota-window low, exhausted and recovery events without configuring a webhook.

## Source inspection and decisions

- Existing account-tier/profile policy already implements absent/inherit,
  explicit-empty deny and disabled/unknown reference behavior, with an intentional
  off/audit/scoped-enforce migration gate. It is reused and not silently enabled.
- Generic owner Key editing can change `access_profile_id` and model limits.
  Therefore administrator assignments use one separate, admin-only per-user or
  per-Key constraint record rather than repurposing those editable fields.
  This is an additional narrowing input to the existing accesspolicy machinery,
  not a second route, entitlement, account or billing engine.
- No assignment inherits existing behavior. Missing/null dimension inherits;
  explicit `[]` denies all in that dimension; a disabled or malformed assignment
  denies. User and Key constraints intersect. Public model, actual upstream model
  and actual channel are distinct dimensions. Assignment changes use revision
  compare-and-swap; only authorized administrators can remove a constraint.
- Explicit assignments apply regardless of the legacy profile mode. They cannot
  grant access that legacy Key/group/tier/profile/channel rules deny. Preview
  performs no writes, migration, enablement or permission grants. Old Keys are
  not auto-enrolled.
- Existing final failover validation is protocol-scoped. Assigned-policy support
  is bounded to the existing OpenAI-compatible Chat/Responses and Codex Responses
  routes with verifiable final target evidence. Assigned requests require a final
  unambiguous JSON body no larger than the existing 1 MiB canonical-proof bound;
  duplicate/case-variant model keys fail closed. Other transports with assignments
  fail closed; unassigned compatible requests retain their existing behavior.
- Current token-cache mutation fences expire after ten seconds. Authorization
  cannot rely on those fences. The supported admission and final send boundaries
  read durable token/user/current assignments and reject unavailable evidence.
  A completed local check cannot undo an upstream request already accepted.
- Existing quota snapshot ingestion transactionally records an outbox and keeps
  retries/quarantine. It already works without an external endpoint. The missing
  in-app view is event meaning and durable account/window evidence, rather than
  another notification subsystem. Event state derives only from normalized,
  trusted account samples; requests succeeding, errors, absence or mixed account
  percentages cannot imply recovery or zero remaining quota.

## Ownership

- Access implementation: `model/assigned_access_policy*`, existing migration
  registration, `service/accesspolicy/`, assigned-policy service/controller/router,
  minimal auth/distribution/model-discovery integration and shared final-send hook
- In-app events: existing quota alert common/model/service/controller paths,
  channel event read route, and corresponding model/service/router contracts
- UI/integration: existing Users, Keys and Channels entries, one shared assignment
  dialog and model availability view, seven locales, browser fixture and CI groups
- Delivery coordination: this card and current plan/handoff documents; no unrelated
  source, recovery checkout, index, dependency, fee or version changes

## Verification and stop

Minimum meaningful tests first, then independent security review of actual role,
mutation, alias, stale-cache, final target and concurrent revocation paths.
Required final exact-head evidence: root Go tests/vet/build and affected races;
independent `GOWORK=off` relaykit; actual disposable SQLite, MySQL 5.7 and PostgreSQL
9.6 migration/reopen/concurrent contracts; frontend tests/typecheck/build/lint,
seven-language rendering and Chromium at 320/1280 including repeated/interrupted
flows and permission isolation. CI runs must be tied to the exact published tree.

Stop when configuration → preview → request → denial/revocation → in-app event
history works within this contract and required verification/docs are complete.
No full UI redesign, commercial tiers, email/SMS platforms, dynamic plugins,
paid/media/provider calls, target credentials, webhook enablement, release, tag,
main merge or deployment. Real account/invoice/production tests stay unverified.

## Review corrections and evidence still pending

Independent access/security review required corrections before publication:
ambiguous model JSON keys, trusted administrator diagnostic exceptions, live
ability/key eligibility and stale group discovery, same-second event order,
native exhausted observation provenance, final expired-delivery-attempt
quarantine and a fresh check before WebSocket dialing. Each is covered by a
focused regression; the original red tests and actual checks are retained.

Native Codex event qualification is distinct from the existing healthy admission
flag. Old snapshots stay unqualified; complete known native windows and consistent
reported allowed/reached flags can prove exhaustion. A healthy secondary window
can recover independently while the primary remains exhausted. Series are never
summed or described as per-Key consumption.

New persistence is one admin-only assignment table, monotonic removal tombstones,
event evidence and state tie-break/cooldown fields, and a native observation proof
bit on the existing quota snapshot. Default inheritance and false old proof bits
preserve safe reads. Deployments must migrate all instances together before an
administrator explicitly assigns a policy; rollback to old application code can
stop enforcement, so do not treat an old binary as a safe policy rollback.
Removing a constraint is a separate authorized admin CAS operation and can restore
access allowed by remaining legacy rules. It is not a data migration or reset.

The supported final-dispatch guarantee covers the declared OpenAI/Codex HTTP
paths. Other protocols with a present assignment fail closed on selection;
WebSocket also rechecks before dial. Unsupported direct-SDK requests admitted
while still unassigned can finish, and no local check recalls a request already
accepted by an external provider. No all-adapter guarantee is made.

Local Chromium could not launch because this environment forbids browser IPC
sockets, including after a tool-reviewed escalation. No local browser success is
claimed. Exact-head CI must execute the existing quota/routing journeys and new
access/event 320/1280 journey; screenshots must be reviewed before closure.

## Local candidate verification (2026-10-05)

- Complete root Go tests passed on the final combined candidate (69 tested
  packages), followed by root vet/build and independent `GOWORK=off` relaykit
  tests/build/vet. Affected assignment/event/model-proof/HTTP/WebSocket races
  passed across model, policy core, service, controller, router and transport
- Frontend: 136 files / 746 tests passed with two local workers; typecheck,
  scoped lint/format, production build and 78 new keys across seven languages
  passed. Eleven event-render tests include real resources in all seven languages
- Native sampler → persistent events → role-scoped history, mixed windows,
  same-second observations, eight-attempt crash quarantine and snapshot-retention
  evidence passed locally. SQLite lifecycle/migration/reopen/concurrent fixtures
  passed. Actual MySQL 5.7 / PostgreSQL 9.6 remain CI-only until verified
- Initial full-root run found legacy task, realtime and text-transport fixtures
  missing the new required assignment table. Only their migration lists were
  corrected; every original assertion, crash/replay scenario and production
  fail-closed behavior remained. Complete fresh-process root tests then passed
- Independent source review cleared the corrected security boundaries and
  static-metadata compatibility. The new browser harness also received static
  review and retains synthetic-only Key reveal fixtures. Its actual Chromium
  run remains pending; local IPC denial is not a UI pass

These are local candidate results, not the next commit's complete CI or a release.

## Combined strict-budget correction (2026-10-06)

Before any commit/ref publication, a combined actual-application regression
confirmed that strict Responses and Chat budgets deliberately remove `GetBody`
to prevent replay, while the second assigned-access proof still required it.
Both synthetic TLS paths reproduced a 403 after budget preparation.

Only the assigned strict-budget proof now reads the actual unsent body through
a 1 MiB + 1 bound, closes its original reader, and validates unambiguous exact
model bytes. On success it replaces only Body with equivalent bytes; GetBody
remains nil, and ContentLength, context, headers, frozen prices/reservations and
usage evidence stay unchanged. Other nil-GetBody paths retain their previous
fail-closed behavior. Read, close, oversize or proof failures do not become sends
or zero usage. No fee, ledger, count or refund calculation changed.

The actual Responses and Chat requests then passed with one main generation
and unchanged Token/USD settlement; Responses counts once and Chat never
probes. Missing Chat usage still retains unknown Token/USD holds. Direct
reader/framing/evidence checks, existing ambiguous-POST no-replay tests and
affected service/transport/router races passed. Independent correction review
and final exact-head CI remain separate requirements before closure.
