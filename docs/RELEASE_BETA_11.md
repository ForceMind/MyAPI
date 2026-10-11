# v0.2.0-beta.11 unpublished candidate

**2026-10-11 当前任务：** 暂不发布或部署。先按[历史账务兼容核查记录](HISTORICAL_ACCOUNTING_COMPATIBILITY.md)
核清历史账务；下述构建、安装与升级验收只覆盖合成环境，不能代替用户历史数据库的一致性验收。

修复候选已提交[草稿PR #11](https://github.com/ForceMind/MyAPI/pull/11)，包含新请求准入保护、个人资料加载修复、历史金额表示兼容、迁移前只读探针、持久执行许可及Key编辑并发保护。被测源码 `09ef63268b5c6742fe73e246f6c5e26e30559903` 的[CI十项](https://github.com/ForceMind/MyAPI/actions/runs/38128779978)和[Docker五项](https://github.com/ForceMind/MyAPI/actions/runs/38128779248)成功；独立CI提交 `c8e7bbc04676e46f2ad0c806a9ac78562923a4ef` 与其非CI源码相同，[MySQL 5.7/PostgreSQL 9.6专项](https://github.com/ForceMind/MyAPI/actions/runs/38132921489)串行成功、零skip，独立审查通过。个人资料9个Chromium合成场景和既有Redis 7.4.11的44个quota-script子项通过，后者不覆盖全部缓存生命周期。实际Go副本演练仍只持有合成许可，全部副本继续DO_NOT_RESTORE。文档后继3feff8e的前端CI曾被15分钟总时限取消；预算后继3a1d50a的[CI 38135269862](https://github.com/ForceMind/MyAPI/actions/runs/38135269862)十项success且Playground完整完成，旧取消保留。当前只新增真实Redis宽金额、精确版本排序及冷发布/信任合同测试与文档，复用现有服务、保留原44断言；准确后继CI结果以PR Checks为准，应用源码不变，不将新增测试后的tree混写为旧运行检出对象。前述运行的源码身份保持独立记录。候选未进入main、发布镜像或资产；beta.3来源、资金/预扣/缓存/全部writer、504事件、旧终态凭证来源和实际回滚资格仍缺，生产账务未核清。详细证据及历史失败范围见[云端验收记录](CLOUD_VALIDATION_PROGRESS.md)。

**Not published. Do not pull or deploy the beta.11 example image yet.**
The currently available prerelease remains [beta.10](https://github.com/ForceMind/MyAPI/releases/tag/v0.2.0-beta.10), fixed at
`631b3205ab2ad28df3f9372bd00f02bd27cb4c31`. Do not move that tag or overwrite its assets or images.
The current local repair baseline is main `92fa1182e265924312d70917a0a637965aebc617`, tree
`01664d716e57f3fc99e2d92c61b6f31217c5a92b`. The earlier release-preparation baseline was
`b8eec75d135f5489fff884820539b5abe99ae312`, tree `c51954e8abae5e251f050b04426d921af2a8402b`.
Its own final source head, merge checkout,
CI and release identities must be recorded after execution; earlier green commits do not qualify it.

下面保留的是历史发布准备记录，其旧候选SHA和合成验收不覆盖上述修复候选。main仍为 `92fa1182e265924312d70917a0a637965aebc617`；以本文开头的部署禁令和核查记录为准。

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
