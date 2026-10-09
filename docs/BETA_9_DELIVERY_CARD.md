# beta.9 delivery card: chat as an API client

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
Existing beta.8 tags and artifacts remain immutable.

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
   prompts, real `image_url` payloads. Four images, 10 MiB each, 20 MiB decoded
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
   browser storage and logs. Seven-language and mobile/keyboard behavior remain.

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

Implementation and validation pending. This section will record actual commits,
checks, supported paths and any remaining blockers; planned checks are not passes.

## Protocol support matrix (verified documentation 2026-10-09)

- Images: inline image_url for existing vision-capable adapters. Model support
  remains provider-specific. No claim of live proxy/reseller acceptance.
- PDF: inline file_data through type-1 OpenAI-compatible Chat; where existing
  type-1 routing converts Chat to Responses, the existing input_file mapping is
  preserved. Other adapter types are refused before dispatch on each attempt.
- TXT/DOCX/etc.: not offered as Chat file attachments in this version. OpenAI's
  broader non-PDF input support is a Responses API capability, not interchangeable
  with Chat. No silent extraction, format conversion or unsupported field drop.
- Strict budgets: images/PDF remain unsupported under the current text-only
  qualification. Choosing chat does not create a bypass.
- Files API IDs/upload/download/delete: deferred persistent lifecycle.

Primary protocol references: [OpenAI file inputs](https://developers.openai.com/api/docs/guides/file-inputs)
and [image inputs](https://developers.openai.com/api/docs/guides/images-vision).
Transport preservation is verified with a synthetic upstream; model capability,
provider policy and genuine usage/bill reconciliation are separate evidence.
