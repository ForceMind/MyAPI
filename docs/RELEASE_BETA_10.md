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
