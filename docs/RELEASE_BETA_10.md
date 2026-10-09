# v0.2.0-beta.10 published prerelease

## Verified publication, 2026-10-09

**[beta.10 is published](https://github.com/ForceMind/MyAPI/releases/tag/v0.2.0-beta.10).**
PR #5 was normally merged; immutable release source is
`631b3205ab2ad28df3f9372bd00f02bd27cb4c31`, tree
`28b235eb612e98ca97672485305bc377b1232fc8`.
This post-publication document is a later documentation commit. The copy inside the
immutable beta.10 tag is the pre-publication candidate snapshot, whose old warning is
historical. Do not move the tag to incorporate this evidence.

- [Candidate CI](https://github.com/ForceMind/MyAPI/actions/runs/37940360062): ten successful jobs;
  [actual Docker smoke](https://github.com/ForceMind/MyAPI/actions/runs/37940360343): LAN source
  installation/fresh SQLite, Full fresh SQLite, and fixed-old-source migration/original-backup recovery passed.
- [Exact-main CI](https://github.com/ForceMind/MyAPI/actions/runs/37942794654): ten successful jobs.
  This is the merged-PR closed event: API event metadata names the former PR head, but all ten
  checkout logs and downloaded browser reports name `631b3205ab2ad28df3f9372bd00f02bd27cb4c31`.
  The duplicate push run was cancelled by the overlapping run; it is not counted as success.
- Frontend: 182 files / 1070 tests, typecheck/build and all browser stages passed. Shared UI:
  22 journeys / 195 screenshots; Playground: eight journeys / 96 screenshots. Three-database
  contracts and the expanded race group passed. Synthetic evidence does not establish live-provider billing.
- [Release workflow](https://github.com/ForceMind/MyAPI/actions/runs/37944886993): all five jobs passed;
  [GHCR workflow](https://github.com/ForceMind/MyAPI/actions/runs/37944949604): four builds and two
  multi-architecture manifest jobs passed, stable/latest promotion explicitly skipped.
- The tag-triggered [Electron workflow](https://github.com/ForceMind/MyAPI/actions/runs/37944814995)
  built both desktop packages successfully. Its separate release-attachment job was skipped.
  These CI build artifacts were not added to the seven release assets and do not prove real-device acceptance.

## Assets and independent byte verification

All seven public assets were downloaded and matched their GitHub SHA256/byte size. The four
binaries also matched the corresponding Linux/macOS/Windows checksum lists. Binaries were
inspected, not executed: Linux amd64/arm64 are dynamically linked ELF; Windows is x86-64 PE;
macOS is arm64 Mach-O. No universal/static-binary compatibility claim is made.

| Asset | SHA256 |
| --- | --- |
| [checksums-linux.txt](https://github.com/ForceMind/MyAPI/releases/download/v0.2.0-beta.10/checksums-linux.txt) | `f6195ec693de173162b68d5c5972b66d3ab8b3e86cf7b00e56517e00d3419d17` |
| [checksums-macos.txt](https://github.com/ForceMind/MyAPI/releases/download/v0.2.0-beta.10/checksums-macos.txt) | `28fdc687e5337ca499782dcb7b52ec312e6bb2cd647694415e111fc90214d0c5` |
| [checksums-windows.txt](https://github.com/ForceMind/MyAPI/releases/download/v0.2.0-beta.10/checksums-windows.txt) | `6896e4f50516cf6ff0ea1f8ee4025bd2f6769d3c58da78869a42bbc59e780089` |
| [my-api-0.2.0-beta.10](https://github.com/ForceMind/MyAPI/releases/download/v0.2.0-beta.10/my-api-0.2.0-beta.10) | `55a772160376f23b662b852141075c6c8ee813636ea93bcc97109c2a29a3d28d` |
| [my-api-0.2.0-beta.10.exe](https://github.com/ForceMind/MyAPI/releases/download/v0.2.0-beta.10/my-api-0.2.0-beta.10.exe) | `5abae486825f5b5b908a5bc28f6aa2d926b8519e1dd233cd7cb402dcfbdffe5a` |
| [my-api-arm64-0.2.0-beta.10](https://github.com/ForceMind/MyAPI/releases/download/v0.2.0-beta.10/my-api-arm64-0.2.0-beta.10) | `ef42d7a7d0213906ac5b8ac7460a984ed1745ea78363bebfe27c07270c1304c9` |
| [my-api-macos-0.2.0-beta.10](https://github.com/ForceMind/MyAPI/releases/download/v0.2.0-beta.10/my-api-macos-0.2.0-beta.10) | `be5276f414006875abae47c50b3f5e21e21c1992b45ab98057323c39dec30e6f` |

## Container identity, signatures and unchanged older release

- Full: `ghcr.io/forcemind/myapi:v0.2.0-beta.10`
  digest `sha256:bea70e5d806b03222b89e981144bbff38708ed1003a01e15c289fef110ef0476`.
- LAN: `ghcr.io/forcemind/myapi-lan:v0.2.0-beta.10`
  digest `sha256:d849b349a59a121b558f881207879c65246dfbf7592e5d2ef12d55bd2e23a716`.

Both indexes contain Linux amd64 and arm64. All four image configs match the edition,
version and source SHA. Four SLSA provenance statements and four SPDX inventories bind to
those image digests; the four workflow digest artifacts match the architecture roots.
All six aggregate/architecture roots passed independent cosign verification with exact
certificate identity
`https://github.com/ForceMind/MyAPI/.github/workflows/docker-build.yml@refs/tags/v0.2.0-beta.10`
and issuer `https://token.actions.githubusercontent.com`.
Source binding combines the verified tag-bound workflow, source checkout, provenance and
signed digest chain; it is not a claim of hermetic/reproducible dependency resolution.

Public-registry before/after checks confirm all six beta.9 roots unchanged. The six
Full/LAN `latest`, `latest-amd64` and `latest-arm64` tags remain absent; GitHub's stable
latest-release endpoint also remains absent. No NPM publication or user-server deployment occurred.

## Upgrade and next work

Read [the accounting preflight guide](PENDING_USAGE_RECOVERY_GUIDE.md) before an existing
installation is changed. Preserve explicit image and accounting settings; never copy the
new marker or disable batching just to bypass an old-instance rejection. Preflight precedes
container changes. Historical pending badges are not automatically cleared, and production
root cause still requires minimal redacted evidence.

This release contains the bounded incident fixes below. Personal-core policy/Key explanation
and structural simplification continue separately under the [next delivery card](PERSONAL_CORE_DELIVERY_CARD.md);
they are not new capabilities of beta.10. Release tags and artifacts remain frozen.

## Historical pre-publication candidate snapshot

The following text records the plan before publication. Its unpublished warnings and pending
checks describe that earlier stage; the verified results above are the current release record.

# v0.2.0-beta.10 incident-fix candidate

2026-10-09. **Unpublished candidate. Do not deploy or pull the beta.10 image tag yet.**
This document prepares the next prerelease; it is not a publication, merge, or production
acceptance record. The immutable beta.9 release remains available. No personal-core new
feature, protocol expansion, `/v1/files` library, or accounting migration is included.

## Bounded fixes

- Usage-review controls follow durable automatic-settlement ownership. Applied quota
  cannot be replaced with a second manual amount. A settlement that already committed
  but still needs journal finalization is identified separately; legitimate strict-budget
  evidence recovery remains available behind an explicit Advanced action.
- Strict native Chat can settle trusted, complete upstream usage after a successful
  terminal write even when the downstream client cancels during or after that write.
  Cancellation before write entry, incomplete/contradictory usage, missing upstream
  terminal evidence, short writes and write errors keep their previous fail-closed holds.
  Server-accepted output is not proof of physical client receipt or upstream cost.
- Cache-free fresh deployment defaults explicitly select `BATCH_UPDATE_ENABLED=false`
  and `MYAPI_ACCOUNTING_CONFIG_VERSION=1`. Legacy/bridge batch-without-Redis is rejected
  before business workers and HTTP start. Authoritative SQL settlement remains eligible.
- Upgrade helpers inspect the existing effective batch setting before changing
  configuration, pulling or replacing containers. Ambiguous, conflicting or unreviewed
  old settings stop the upgrade while the existing service remains untouched.

These defects were reproduced with synthetic requests. They do not establish the cause
of a particular production request without its minimal, redacted diagnostic evidence.
Saved historical log badges and accounting facts are not rewritten by this patch.

## Version and compatibility contract

`VERSION`, root `package.json`, both Compose image fallbacks and the deployment example
name beta.10. CLI and installer image defaults derive from that metadata. Existing
`MYAPI_IMAGE` and legacy `NEW_API_IMAGE` overrides retain priority; no existing environment
file is silently replaced. Private web/Electron package versions are independent.

The candidate's default image does not exist merely because these files name it. Use
the verified released version until a separately authorized beta.10 release completes.
Local builds are explicit test operations, not a production upgrade recommendation.

Read [the diagnostic and upgrade guide](PENDING_USAGE_RECOVERY_GUIDE.md) before any later
upgrade. Do not add the configuration marker or turn batching off to bypass a failed
preflight. Redis loss, outstanding increments and other running nodes need review;
absence of pending SQL records alone cannot prove that all quota work was drained.
Raw Compose only rejects missing/empty markers, so use the helper preflight rather than
stopping an old container first. No writer switch, queue clearing, balance adjustment,
automatic refund, or retry with a different Key is part of this release.

## Acceptance evidence and remaining gates

The [incident card](BETA_9_PENDING_USAGE_INCIDENT_CARD.md) and
[status patch card](SETTLEMENT_REVIEW_STATUS_PATCH_CARD.md) define the contracts.
[Draft PR #5](https://github.com/ForceMind/MyAPI/pull/5) records current exact-head results.

- Preserve the incident-source run for `e6ebb20e0b21098acb312c2e11a61057eaff5fa7`
  ([CI](https://github.com/ForceMind/MyAPI/actions/runs/37937933457),
  [Docker smoke](https://github.com/ForceMind/MyAPI/actions/runs/37937933477)) separately
  from the later version-candidate run. Results must be read to terminal status.
- All 24 new Go regressions are selected by ordinary CI and the bounded race group.
  Three-database review contracts, full frontend tests/typecheck/build, real Chromium,
  release/package contracts and actual isolated Docker installation/migration are required.
- The earlier e1a8e6c frontend job failed because old quota helpers tried to fill a
  collapsed manual form. All three remaining entry points now assert the closed state,
  explicitly expand Advanced and preserve validation/replay assertions. Other successful
  browser journeys in that failed job are not a passing frontend result.
- Independent reviews cover automatic-settlement ownership, both-writer idempotency,
  observable write/cancellation boundaries, existing-configuration preservation,
  before-stop preflight, browser transitions and bounded CI permissions.
- A version change requires a fresh complete candidate CI and Docker smoke. PR checks
  run the GitHub merge checkout; evidence must bind its parents and tree to the exact
  source head. A previous green head is not final candidate acceptance.

After explicit approval, the existing process is: normal merge, exact-main CI, a new
immutable `v0.2.0-beta.10` tag, and gated Release/GHCR workflows. They require matching
tag/VERSION/package/source, the existing repository gates and `PUBLISH` confirmation.
Verify prerelease assets/checksums, Full/LAN amd64/arm64 manifests, cosign identities and
source/provenance before reporting publication. Prereleases do not advance stable/latest.
No NPM publication or server deployment is included; either requires its own applicable
authorization. Do not replace beta.9 artifacts or weaken platform confirmation gates.
