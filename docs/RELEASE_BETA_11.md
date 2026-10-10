# v0.2.0-beta.11 unpublished candidate

**Not published. Do not pull or deploy the beta.11 example image yet.**
The currently available prerelease remains [beta.10](https://github.com/ForceMind/MyAPI/releases/tag/v0.2.0-beta.10), fixed at
`631b3205ab2ad28df3f9372bd00f02bd27cb4c31`. Do not move that tag or overwrite its assets or images.
This candidate starts from main `b8eec75d135f5489fff884820539b5abe99ae312`, tree
`c51954e8abae5e251f050b04426d921af2a8402b`. Its own final source head, merge checkout,
CI and release identities must be recorded after execution; earlier green commits do not qualify it.

## Included scope since beta.10

- PR #6: explain the effective personal usage policy and Key units/qualification,
  expose the existing policy/budget entry points, and improve the specific seven-language
  narrow-screen controls. This adds no universal strict token budget or new protocol eligibility.
- PR #7: use typed subscription-admission errors and wrapping identity instead of matching
  error text. Genuine business insufficiency retains its existing allowed fallback;
  database and unknown failures cannot authorize a different funding source merely because
  their text resembles an insufficiency message. Existing writer and accounting contracts remain.
- PR #8/#9: real application tests for fresh Root and an ordinary owner, finite normal Key
  quota, explicit policy confirmation, strict-ineligible loopback rejection and persistent
  accounting; bounded, verified image reuse between independent test VMs; corrected test
  navigation and diagnostic capture. These changes are test/workflow/documentation, not new
  production billing behavior.
- All beta.10 fixes remain included: automatic-settlement ownership, the limited complete-usage
  post-terminal cancellation case, and the accounting startup/upgrade preflight protections.

Normal Key quota is price-converted internal units, not a universal hard token counter.
Strict Token/USD qualifications remain limited to their existing supported native request paths.
The controlled loopback negative test does not prove a positive native-provider budget path.
This version does not complete the personal-core card: in-flight policy revocation, concurrency,
unknown/restart recovery, remaining structure simplification and broader reference adoption
remain separately tracked. No held-provider feature is added by this release preparation.

## Upgrade without changing user data or accounting choices

This preparation changes version metadata, fresh-install image defaults and release/test records.
It does not edit any running instance, channel, API Key, database, wallet, quota, used amount,
pending review or migration marker. PR #6/#7 introduce no new persistent schema migration.
Existing channels, groups, keys and accounting history must remain attached to the same database
and volumes. A source merge or new image is not evidence that an old production error is fixed.

- Preserve the existing `.env`, explicit `MYAPI_IMAGE` / legacy `NEW_API_IMAGE` override,
  database DSN/name, secrets, volumes and reviewed writer/cache configuration. The Compose
  fallback changes only where neither image override is supplied. Do not copy the example
  `.env` over an existing installation or create an empty replacement database.
- Keep the beta.10 [accounting preflight and incident guidance](BETA_9_PENDING_USAGE_INCIDENT_CARD.md).
  Inspect actual running container configuration before pull/stop. A legacy batch/cache
  incompatibility requires an explicit reviewed choice; do not disable batching to bypass
  Redis or abandon outstanding generations. Never add the configuration marker blindly.
- Take a verified backup/snapshot using the established deployment procedure before an
  authorized upgrade; retain the old image digest and original settings. A rollback must use
  compatible database state and reviewed recovery procedure, not casually replace newer
  accounting data. This candidate performs no production backup, restore or migration.
- Historical pending usage does not automatically disappear. Preserve automatic settlement
  facts/intents and the existing recovery rules; never repeat a manual debit just to clear a badge.
- Existing personal policies are not silently enabled or revoked. Root changes still require
  the real confirmation, CAS and idempotency protections. Retained old allowance is not erased.

## Evidence and remaining gates

The integrated pre-version source passed candidate CI and two independent real Docker writer
journeys at merge `f1d9ff7a7d6f06cf64554569d41031ae7aa94076`, with source tree `c51954e8...`:
[CI](https://github.com/ForceMind/MyAPI/actions/runs/38019193773),
[Docker](https://github.com/ForceMind/MyAPI/actions/runs/38019193602).
Each writer recorded five checks: fresh Root stream and persisted 10/5 usage /15 internal quota;
ordinary owner refusal followed by explicit Root confirmation and same-Key success; finite-quota
refusal without another upstream dispatch; unqualified strict Token/USD refusal without reservation;
and a reload showing remaining985/used15 without replay. No real paid provider or user data was used.

The 14 screenshots per writer are auxiliary. Some 320px long error identifiers are clipped in the
initial viewport, some captures contain a transition or an earlier message. This is not complete
visual qualification, and the evidence does not establish that horizontal scrolling is unavailable.
Legitimate earlier errors are preserved in conversation history. Full request/accounting assertions,
not a cropped screenshot, establish subsequent success. Earlier fixture failures and their original
logs remain documented in [the image-reuse record](DOCKER_SMOKE_IMAGE_REUSE.md).

A version change requires fresh complete candidate CI and Docker installation/upgrade checks,
independent review, then normal merge and exact-main CI. Record the actual checked-out commit/tree,
including merged-closed workflows whose API metadata can still name the former PR head.
No old-green substitution, force push, mutable release tag or unverified deployment is allowed.

## Publication sequence, only after gates and applicable authorization

1. Verify final `VERSION`, root `package.json`, source manifest and new tag all agree on
   `0.2.0-beta.11`; existing web/Electron private-package versions have separate contracts.
2. After exact-main acceptance, create a new immutable `v0.2.0-beta.11` tag at that exact commit.
   Confirm no existing tag/release first. Never repurpose beta.10.
3. Use the existing gated Release and GHCR workflows with the exact tag, enabled repository
   publication gates and `PUBLISH` confirmation. Preserve protected-environment/platform checks.
4. Independently verify the actual release asset inventory, bytes/checksums/source manifests,
   Full/LAN linux amd64/arm64 indexes/configs, provenance/SBOM and cosign identities. Inspect any
   tag-triggered Electron build and whether it actually adds assets; do not assume an inventory.
5. Keep the release a prerelease. Do not promote stable/latest, publish NPM or deploy a user server.
   Report availability only after public readback and verification; record publication separately
   without moving the immutable tag.

The candidate's exact same-repository branch adds no general branch permission. Its Docker
matrix reuses Full fresh as the legacy personal journey, adds one authoritative Full fresh
journey, and retains the fixed historical Full handoff/restore plus the independent real LAN
source installer. The current Full image is built once; both personal consumers and handoff
verify the same producer bundle. Handoff does not enable the personal fixture or skip recovery
merely because that shared bundle also contains the pinned Redis image. Existing manual Full/LAN
and prior branch behavior stay intact. Producer/required-edition failures and missing artifacts
remain failures; skipped dependent tests cannot count as acceptance.
