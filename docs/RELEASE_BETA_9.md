# v0.2.0-beta.9 source release candidate

## Status and authorization

This is the beta.9 Draft source candidate, not a published release, image, or
production-upgrade instruction. The latest published prerelease remains
[v0.2.0-beta.8](https://github.com/ForceMind/MyAPI/releases/tag/v0.2.0-beta.8)
(published 2026-10-08). Work and exact-head evidence are in
[Draft PR #4](https://github.com/ForceMind/MyAPI/pull/4).

`VERSION`, the root package, and root/deploy default Full image references name
`0.2.0-beta.9`. Those defaults are candidate metadata, not proof that the image
exists. README installation examples deliberately pin the published beta.8.
Do not deploy the beta.9 candidate defaults before separately authorized
publication and verified artifact readback. Existing beta.8 artifacts stay unchanged.

Normal scoped fixes, tests, documentation, commits, pushes, and Draft PR upkeep
are authorized. Main merge, a new tag, GitHub prerelease, GHCR/NPM publication,
production deployment, credential expansion, and the user's computer are not
included. Preserve protected workflow/environment gates; no force pushes,
tag replacement, alternate publishing workflow, or stable/latest promotion.

## Included journey

1. Open Playground and explicitly choose an existing Key owned by the signed-in
   user. No automatic first-Key selection, temporary unlimited token, or browser
   credential disclosure. Model discovery follows that selected Key.
2. Attach supported images/PDF, inspect or remove the draft, and send once through
   the existing shared authentication, routing, admission, Relay, and usage path.
   Unsupported media or policy returns an error; it is not stripped or replayed.
3. Stop cancels the active request without automatic resubmission. Failed/cancelled
   drafts remain available for an explicit next action. After reload, attachment
   history has an explicit unavailable marker rather than sending degraded text.
4. Open **Usage Logs → Common** (`/usage-logs/common`) to inspect the selected
   Key's record; the existing Token Name column/filter identifies it. Backend
   HTTP/relay tests prove selected `TokenId`, quota/log attribution and an unchanged
   unselected Key; synthetic browser logs validate only the display/navigation.

Seven-language mobile controls show the entire selected name/group/status in a
wrapped readout with a full-width 44px selector. Key identity remains readable
while controls are individually disabled. An available Stop control is not dimmed
because another descendant is disabled.

## Actual support matrix

| Input / capability | beta.9 boundary |
| --- | --- |
| Ordinary text | Existing selected-Key/model/provider policy applies. No new provider capability is promised. |
| PNG/JPEG/WEBP/GIF | Inline `image_url`; actual type-1 OpenAI-compatible and type-57 Codex paths only, checked on each attempt. The upstream model must support images. |
| PDF | Inline `file_data` through an actual type-1 OpenAI-compatible adapter only. Existing Chat→Responses mapping preserves `input_file`; upstream model support is still required. |
| Attachment limits | At most four files across conversation history and draft, 10 MiB each, 20 MiB decoded aggregate; 32 MiB server envelope. Signature, filename, role and unambiguous protocol keys are validated. Smaller deployment and existing 1 MiB assigned-access proof limits remain authoritative. |
| Strict Token/USD budgets | Media is rejected. For an explicitly selected strict Key and exact `gpt-6.1-sol`, the client emits the existing strict native-Chat envelope: required `max_completion_tokens` (1..128000), `stream_options.include_usage=true` only while streaming, and `service_tier=default`. Incompatible sampling controls are visibly disabled and ordinary-Key preferences are preserved. The server still requires its existing official type-1 route/model/funding/price qualification; fee-qualified routes also need their existing service-tier configuration. Unsupported combinations fail closed with accurate guidance; no Key fallback or broader protocol qualification. |
| File persistence | Bytes remain in memory; no attachment bytes in localStorage or diagnostics. Text/history metadata is owner-scoped. Legacy ownerless history is retained but not automatically loaded or assigned. |
| Not included | `/v1/files` upload/read/download/delete lifecycle, retention, upstream file-ID mapping, remote file URLs, opaque `file_id`, TXT/DOCX, media generation, arbitrary uploads, new billing, or new credential management. |

Payload-preservation tests use synthetic upstreams. They do not prove live
provider/reseller acceptance, actual invoices, production migrations/rollback,
physical-device native-picker behavior, or production readiness of stable 0.2.0.

## Evidence and remaining release gates

The reviewed implementation checkpoint
[`f008dc79eef64e31b0e8c7f30140f8ca797b6336`](https://github.com/ForceMind/MyAPI/commit/f008dc79eef64e31b0e8c7f30140f8ca797b6336)
passed [all ten CI jobs](https://github.com/ForceMind/MyAPI/actions/runs/37895495612)
and [website checks](https://github.com/ForceMind/MyAPI/actions/runs/37895495717).
Its explicit Playground Token SQLite/MySQL 5.7/PostgreSQL 9.6 subtests all passed.
Frontend was 178 files/1007 tests; root and independent relaykit checks passed.
Clean-checkout `release:check` included source-manifest and 2928-file pack validation.

That checkpoint's [browser artifact](https://github.com/ForceMind/MyAPI/actions/runs/37895495612/artifacts/11600308724)
contains 8 Playground journeys/24 PNGs/48 identity measurements plus 19 shared UI
journeys/42 settings deep links/171 PNGs. All seven 320px languages and desktop
1280px were inspected; identity opacity was 1 and contrast 12.79:1. Native touch
is dispatched to the control; native option selection uses Playwright
`selectOption`, not physical-device OS picker automation.

The closeout source adds actual browser multiple-image, invalid-file,
remove/Stop-cancel and owner usage-log inspection scenarios, strict-text client/server parameter alignment with accurate rejection wording,
synchronized docs, and the final head's own applicable checks.
The final PR evidence must name that head and artifacts. Earlier green results
never substitute for the subsequent commit, and a skipped Docker smoke does not
count as a pass. The card's stopping condition is not satisfied until these
closeout checks succeed.

After separate publication authorization:

1. Merge only the approved exact candidate, qualify the resulting main commit,
   and use a new `v0.2.0-beta.9` tag with matching version/source identity.
2. Use existing `release.yml` and protected image workflows for the already-defined
   platform assets and Full/LAN images. Verify prerelease/not-latest status,
   filenames, checksums, immutable image digests, signatures/provenance and source SHA.
3. NPM publication requires its own explicit inclusion and package verification;
   `npm pack` validation is not publication.
4. Return artifact links and pause before actual deployment. Real account/billing,
   installation/upgrade/rollback and production acceptance remain separate evidence.

Persistent Files API lifecycle or any later roadmap node needs a separate frozen
scope. Do not silently expand this candidate into it.
