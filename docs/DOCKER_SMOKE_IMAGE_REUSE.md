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
