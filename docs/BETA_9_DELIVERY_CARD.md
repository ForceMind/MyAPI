# beta.9 delivery card: chat as an API client

## Current publication and deployment result (2026-10-09)

This section supersedes the retained pre-publication boundary below. The owner
explicitly authorized deployment. The published beta.9 source is
`c45df8c6c6bf2e66d39b58b8c2bc8d61984f901b`; exact-source CI
[37911737178](https://github.com/ForceMind/MyAPI/actions/runs/37911737178)
passed all ten jobs after its previously cancelled run was rerun.
Seven release assets, four architecture identities and six container signatures
were independently checked. Stable latest was not promoted.

The existing Full HTTPS instance was upgraded from beta.7 to beta.9 using the
verified immutable Full digest after isolated master upgrade and original-snapshot
beta.7 restoration passed. The rehearsal preserved 26 protected tables with no
schema changes. Production local/HTTPS homepage and status, version, health,
SQLite integrity, historical identities and accounting, backups, original
session/keyring/environment and runtime limits passed. Existing failed-retry
operational metadata was checked under its exact contract; no economic/state
change was permitted, and those preexisting failures were not repaired here.
No production rollback or real OAuth/paid call was performed.

See [the release and deployment record](RELEASE_BETA_9.md) for digests, workflow
links and bounded acceptance. Playground requires an explicit existing owned Key
and applies the normal API permissions and billing path.


## Baseline and authorization

Frozen 2026-10-09 before implementation. Baseline main commit:
`4388b0dd943b0de89d25817f1a8012ac138d9f51`, tree
`ffe1f9ab1c62d6659a6ad16bc38ce96437b73876`.
The owner will deploy beta.8 and requested the next version, prioritizing image /
attachment sending and built-in chat as a client of the same API. Work proceeds
on `codex/beta9-chat-api-20261009` in the assistant cloud environment.

This card authorizes normal scoped development, tests, documentation, commits,
pushes and a Draft PR. It does not authorize merging this version, creating a
release/tag, publishing packages/images, operating the user's Mac, or deploying.
Existing beta.8 tags and artifacts remain immutable. VERSION/package/default
image references may name the beta.9 candidate; they are not proof of a
published image and must not be deployed before separate release approval.

## One user journey

Open Playground, explicitly select one of your own existing API Keys and an
available model, add text and/or supported attachments, inspect/remove attachments,
send once, receive the stream or result, and inspect usage attributed to that Key.
No Key, unavailable authorization, unsupported media, exhausted quota or a strict
budget that cannot admit the request must produce an honest actionable error.

## Existing architecture and changes

Playground already calls the common `controller.Relay`; it is not a separate
inference engine. Its legacy `/pg` path creates a temporary Token, accepts an
independent group and sets an accounting exemption. This version replaces that
identity behavior with a server-verified selection of a real owned Key.

1. User session + explicit Key ID, persistent ownership lookup, shared TokenAuth
   validation/admission, normal rate limiting/distribution/Relay. No secret is
   sent to the browser, URL or chat storage. No first-Key or unlimited fallback.
   Models are read under the selected Key. Session/PAT and CSRF limits remain.
2. Images: PNG/JPEG/WEBP/GIF, selection and paste, previews/removal, image-only
   prompts, real `image_url` payloads. Four attachments total, 10 MiB each, 20 MiB decoded
   aggregate maximum; server request limit and validation remain authoritative.
   Existing smaller deployment limits may reject requests. Unsupported model /
   provider requests are not presented as successful or silently stripped.
3. Bounded inline PDF attachments: Chat file_data, filename and application/pdf,
   only via an actual type-1 OpenAI-compatible adapter on every attempt. The
   upstream/model still must support PDF. Four attachments including images,
   10 MiB each / 20 MiB total decoded, 32 MiB request body. Opaque file_id and
   remote file URLs are not portable and are refused. TXT/DOCX and other non-PDF
   files are not claimed as Chat file inputs; see the support matrix below.
4. Failure/interruption/reentry: prevent double submission, no automatic replay
   after a stream starts, retain actionable unsent drafts and make missing
   attachment state explicit after refresh. Media bytes remain out of persistent
   browser storage and full-content/diagnostic logs. Text history and Key selection
   are scoped to the authenticated user; legacy unowned storage is not loaded
   or silently assigned. Seven-language and mobile/keyboard behavior remain.

## Safety and support boundaries

- Shared Key policy, legacy restrictions and assigned user/Key intersections;
  IP/group/model gates, revocation, quota, strict budgets and usage attribution.
- Strict Token/USD admission remains text-only on its currently qualified native
  protocols/models. Media is refused, never downgraded or bypassed for chat.
- Provider compatibility is per protocol/adapter, not a universal claim about
  all proxy resellers or models. Synthetic upstream tests verify exact payloads
  and dispatch counts; they do not establish a live reseller's capabilities.
- No new billing model, provider credential management, database schema, paid
  provider testing, remote file fetching, media generation or arbitrary upload.
- `/v1/files` persistent upload/read/delete lifecycle, retention/ownership and
  upstream file mapping remain a later separately scoped version.
- Existing Full/LAN, accounting recovery and unsupported-provider denials remain.

## Ownership and verification

Backend changes belong to Key selection/authentication, exact route normalization,
request validation and relay/accounting boundary tests. Frontend changes belong
to Playground request/state/media handling, translations and browser fixtures.
Plan and support evidence remain in existing plan/handoff files and this card.
No parallel generalized framework or new inference service is introduced.

Verify smallest regression cases first, then applicable aggregate checks:

- Real middleware/relay tests: own/foreign/missing/disabled/deleted/expired/exhausted
  Key, IP/group/model restrictions, stale cache and revocation, budget media refusal,
  same API identity/usage and no secret disclosure or group override.
- Synthetic upstream receives exact images/files; stream and nonstream results,
  denial before dispatch, interruption and no duplicate submission/charge.
- Frontend behavior tests, typecheck, changed-file lint/format, production build;
  real 320/1280 Chromium for selection, image-only/multiple images, invalid files,
  remove/cancel/error/retry, missing persisted attachments and seven languages.
- Root Go and all existing CI gates; independent relaykit build/tests. Database
  behavior changes require SQLite/MySQL 5.7/PostgreSQL 9.6 evidence, not SQLite only.
- Exact remote head/readback/CI; retain failed attempts and their corrections.

## Resources, recovery and stopping condition

No artificial time/token budget is promised. Reuse existing CI and dependencies;
bound local parallelism and synthetic payload sizes. Do not repeatedly build
images or start persistent services. Preserve unrelated files/history and use
normal non-force publication with expected-parent checks.

Stop when the bounded journey, failures, safety tests, support matrix, documents,
Draft PR and accurate-head checks are complete. New provider/file lifecycle or
later roadmap features stay deferred. Real provider billing/production acceptance
remains explicitly unverified; this is not a production-stable 0.2.0 claim.

## Evidence ledger

Implementation is tracked in [Draft PR #4](https://github.com/ForceMind/MyAPI/pull/4).
The planning checkpoint is `68d40113e0a63c319f8209b51dfab7126f5ef200`;
subsequent source and check results are tied to the PR's exact head, not inferred
from beta.8 or a prior planning run.

Reviewed implementation checkpoint `f008dc79`: 178 files / 1007 frontend tests, TypeScript, changed-file lint and
production build passed. The independent frontend review reran the repaired
account/draft/query failure cases. Independent relaykit build and complete tests
passed. Backend tests exercise the real selected-Key/Relay/HTTP transport path,
not a mocked controller: dual writers, streaming/nonstreaming PNG+PDF, actual
Codex input_image conversion, assigned-policy revocation, unsupported adapters,
and half-stream EOF with no replay or fabricated successful usage.

An opt-in disposable database contract verifies owned Key lookup, foreign-owner
and deleted-Key denial, expiry and exhaustion status persistence on SQLite,
MySQL 5.7 and PostgreSQL 9.6. All three explicitly passed in the checkpoint
[CI run](https://github.com/ForceMind/MyAPI/actions/runs/37895495612), with no
skipped Playground Token subtest. All original backend/race/database gates remain enabled.

The current source must pass its own complete CI, the four existing Chromium
suites and the new eight-session/seven-language chat attachment browser suite.
The exact check conclusions and screenshot artifacts are recorded in PR #4.
No pending or skipped stage is counted as a pass, and no live-provider bill,
account, or production acceptance is claimed.

## Protocol support matrix (verified documentation 2026-10-09)

- Images: inline image_url for type-1 OpenAI-compatible and type-57 Codex routes
  only; other actual adapters are refused on every attempt because some legacy
  converters silently drop images. Model support remains provider-specific.
  No claim of live proxy/reseller acceptance.
- PDF: inline file_data through type-1 OpenAI-compatible Chat; where existing
  type-1 routing converts Chat to Responses, the existing input_file mapping is
  preserved. Other adapter types are refused before dispatch on each attempt.
- TXT/DOCX/etc.: not offered as Chat file attachments in this version. OpenAI's
  broader non-PDF input support is a Responses API capability, not interchangeable
  with Chat. No silent extraction, format conversion or unsupported field drop.
- Strict budgets: images/PDF remain unsupported. Explicit strict Keys with exact
  `gpt-6.1-sol` use the existing native-Chat text envelope, required bounded
  `max_completion_tokens`, streaming usage inclusion and default service tier.
  The UI reflects the required/unsupported parameters without changing ordinary-Key
  preferences. Existing actual-route, model, funding, price and service-tier gates
  remain authoritative; choosing chat does not create a bypass.
- Files API IDs/upload/download/delete: deferred persistent lifecycle.

Primary protocol references: [OpenAI file inputs](https://developers.openai.com/api/docs/guides/file-inputs)
and [image inputs](https://developers.openai.com/api/docs/guides/images-vision).
Transport preservation is verified with a synthetic upstream; model capability,
provider policy and genuine usage/bill reconciliation are separate evidence.

## Independent-review hardening

Before source publication, review reproduced and fixed case-alias/duplicate-key
media validation bypasses. Browser-envelope protocol fields now use canonical
lowercase names, exact media maps and bounded unique-key validation; nested user
tool schemas retain their legitimate case-sensitive fields. Media is user-role
only, preventing converters from silently dropping system/tool attachments.
The existing 1 MiB assigned-access final-body proof remains unchanged and may
reject requests below the client media limits. No guard was relaxed for media.

Review also identified diagnostic payload/header leaks and interrupted draft /
account-switch recovery gaps. These received red/green regressions: per-user history, local metadata-query
recovery, exact failed-tail retry ownership, request credential scrubbing, and
request-marked diagnostic omission with safe error metadata. HTTP-200/partial
stream errors retain unknown usage holds and do not retry. Independent review
is followed by accurate-head CI and actual pixels; pending checks are not passes.

## Browser-history migration

New text history, configuration and selected Key IDs are scoped to the signed-in
user ID. The previous ownerless playground_* localStorage entries remain intact
but are not loaded or automatically assigned to whoever signs in next. This
version does not import those entries; any future import needs explicit ownership
verification. Live attachment bytes are never stored in those history fields;
reopened messages retain a missing-attachment marker and cannot silently send
without their original files. Existing successful text remains per-user.


## Closeout checklist and release boundary

- Identity, shared middleware/Relay, quota/log attribution: real controller tests
  include both writer modes and streaming/nonstreaming; the unselected Key remains
  unchanged. Owners inspect the existing `/usage-logs/common` Token Name column/filter.
- Media validation, multi-image/removal, invalid input and actual Stop cancellation
  require real-browser coverage in addition to the focused unit/hook tests. The
  first green checkpoint did not cover every one of these browser actions; the
  closeout harness now includes these actions; its new-head evidence must pass
  before declaring this card complete.
- Seven-language 320px identity is checked for actual line clipping, selector size,
  ancestor opacity and text contrast, not just absence of horizontal overflow.
- The shared strict-budget unsupported code applies to text as well as media;
  its user-facing message must not falsely promise that sending text alone works.
  Qualified strict text must be proved through actual middleware/Relay/settlement,
  not just successful authentication or a mocked terminal handler.
- `VERSION`, package and both compose defaults name the beta.9 candidate; README
  install examples name the published beta.8. Current handoff headers override
  retained historical status paragraphs. [Release notes](RELEASE_BETA_9.md) explain
  this distinction and link the screenshot/evidence checkpoint.
- Final accurate-head CI, final browser artifacts/pixels and clean-source package
  validation remain mandatory after closeout edits. Their authoritative result is
  [Draft PR #4](https://github.com/ForceMind/MyAPI/pull/4); earlier source evidence
  is not inherited automatically. Docker smoke is skipped while Draft and is not a pass.
- Stop at the verified Draft source candidate. Merge, tag/prerelease, GHCR/NPM
  publication, real account/provider billing checks and deployment remain separately
  authorized actions; `/v1/files` and later roadmap capabilities remain deferred.
