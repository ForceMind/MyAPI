# Bounded Docker smoke image reuse

2026-10-10 development card. Based on frozen personal application candidate `c24779ccd4f925752bfba963c717283cdfdadd0f`; this card does not claim that PR #8 or the new transport has passed real Docker acceptance. The original two attempts, including Docker Hub 429, authentication timeout and 504 failures before application execution, remain evidence rather than application test failures.

## Change boundary

- Build the current exact merge checkout once per requested edition, then load that same image into the existing independent writer VMs. PR personal runs still require fresh legacy and authoritative cases; manual Full/LAN and fixed-source historical handoff remain. The source installer independently builds from source because that is its acceptance contract.
- Fetch the existing digest-pinned Bun runtime once in the producer; the personal two-writer path additionally fetches Redis once. Standard/manual paths retain their previous absence of a Redis dependency. This does not remove producer upstream access, BuildKit base-image metadata requests, the source installer's own build, or the historical handoff build. It does not solve an upstream outage or quota exhaustion by itself.
- No production Go/React/Dockerfile behavior, package version, registry publication, credentials, host networking or runner identity changes. The CI database jobs remain outside this first bounded change.

## Producer and transport

A single producer avoids matrix-output collisions and mixed producer attempts. It builds Full once; manual dispatch additionally builds LAN once. Each build is bounded at 20 minutes, fixture fetch at eight, export/upload at five each, and the producer at 50 minutes. Existing GHA build caches, resource limits, linux/amd64 and push:false remain.

Each artifact name binds workflow run ID, producer attempt, exact checkout SHA and edition. It contains only `manifest.json` and `images.tar`, created before any application or database container starts. Retention is seven days. No container commit, runtime data, logs, auth state or test credentials enter this artifact.

The manifest binds source SHA, PR head SHA, source tree, run ID, producer attempt, edition, personal/standard fixture mode, platform, tar SHA256 and image identities. The consumer first checks the manifest against the digest passed through the producer's successful job output, then checks its schema/identity and tar digest. After Docker load, it independently verifies image IDs, tags, architecture and application source labels. Fixed upstream digest and Docker image ID are distinct identities; load is not assumed to preserve RepoDigests.

Only successful build, verified export and successful artifact upload publish an edition identity. Continue-on-error exists only to allow a manual request's other edition to prepare; a final required-outcome check makes any requested edition failure fail the producer. Available editions may still run their original consumers. A missing edition fails closed; if none is available, consumers do not run and the failed producer is not acceptance success.

Consumer-only retries use the original successful producer attempt, not the consumer's new attempt number. A whole producer retry creates a new immutable name. Missing, expired, corrupt or mismatched artifacts fail without registry fallback or cross-run lookup.

## Runtime invariants

The consumer preserves independent fresh SQLite, session state, authoritative private Redis network, loopback HTTP ports, explicit writer migration APIs, six-minute personal application step, and always-run ownership-checked cleanup. Loaded fixture IDs and the verified application tag use `--pull=never`; the existing restore helper likewise cannot implicitly pull. Historical handoff source continues to build from its existing fixed commit in the handoff consumer.

Trusted same-repository branch guards and manual invocation remain explicit. The new development branch is added to the exact personal branch list, including both writers and restore-off behavior. Forks do not gain this Docker job. Permissions remain contents:read with checkout credentials disabled.

## Acceptance before integration

- Local helper negative cases: wrong source/head/tree/run/producer attempt/edition/platform, manifest/tar tampering, image mismatch, missing/extra/symlink files, invalid app labels, and no output before full verification.
- Workflow contracts: only one current build per edition; two independent writer VMs; immutable producer binding on retries; manual one-edition failure still permits the other while overall fails; exact trust gates; no implicit consumer pull; installer/handoff preservation.
- Parse workflow and shell blocks; run all runtime contracts and independent review.
- Then coordinate one explicit remote candidate run, recording actual merge checkout/tree, producer identities, both real writer outcomes and representative browser evidence. Local mocks and static contracts do not prove Docker save/load or actual Actions dependency behavior.

Do not run the old frozen candidate concurrently with this new candidate merely to seek a green result. Integration, merge and any release remain coordinated separately.

## Local checkpoint

The frozen implementation passed 194 runtime contracts, including 54 bundle-helper cases and four workflow-reuse contracts. The workflow outcome test executes the actual publication/aggregate shell steps for seven success/failure combinations. Removing the external manifest-hash guard makes its negative test fail. Actionlint 1.7.12, YAML parsing, 24 embedded shell syntax checks and diff checks passed. This executor has no Docker binary; image transport and Actions rerun behavior remain explicitly unverified until the coordinated remote candidate run.

## First real run and bounded failure investigation

PR #9 head `9342acb226a573fcaa17bc55e47383df18e496ef` was tested by Docker run `38014865417` at merge checkout `ecc2d7c73756539854c938c7fc886f6fc77167f1`, tree `396a8112ce97a87eb77c18955ab2ec7922fe24f6`. The producer built Full once. Both independent writer VMs successfully downloaded, checked and loaded the same application, Bun and Redis image identities. The image artifact's downloaded ZIP digest, manifest digest and tar digest were independently checked. This establishes real image transport for this run; a consumer-only retry retaining the original successful producer has not yet been exercised.

Both complete application cases still failed. Fresh Root setup, finite Key creation, real Playground relay and persisted usage passed. The ordinary owner's zero-wallet rejection, Root's explicit policy confirmation and the same owner's subsequent same-Key relay and accounting also passed. Opening that owner's filtered usage list then displayed the application's generic 500 error boundary while the URL remained `/usage-logs/common`. The screenshot alone does not establish an HTTP 500 response or its cause. Later low-quota, strict-eligibility and final-reload stages were not reached and are not accepted by this run.

A local investigation rendered the actual table and complete usage page in eight combinations of ordinary owner/Root and mobile/desktop, with Chinese and internal-quota display. Those component checks passed and narrow the investigation; they do not prove the real application navigation works. Production route code splitting can also route a chunk-loading exception to the generic error boundary. The original evidence does not contain enough resource/error classification to distinguish this from a render exception.

The next diagnostic change keeps the failed run and every identity/accounting assertion. It separates waiting for the filtered API/identity result from waiting for the visible list. Browser diagnostics are bounded fixed categories and numeric statuses only, including console-reported route errors and same-origin asset failures. Raw errors, stacks, URLs, query values, headers and response bodies must not enter reports. No navigation retry, production behavior or acceptance threshold is changed to make this case pass. A new exact candidate and real two-writer run remain required.
