# v0.2.0-beta.11 unpublished candidate

**2026-10-14 部署准备与下一片：** 用户已授权部署，历史账务仍冻结。固定候选 `0000e865`
准确 CI `38393556593` 十项及 Docker smoke `38430541538` 五项通过，Full 制品身份已核验。
原实例只读检查未满足 writer/cache 与历史 execution 启动条件，实际部署数据方式待用户选择；
尚未加载/启动候选，没有创建 beta.11 标签或公开镜像，不把合成恢复当生产升级资格。
独立分支补齐新建 Responses 严格 Token unknown 跨 SQLite 重开的组合，静态独审已通过，
准确新 head Actions 待验证。此片是回归证据，不新增协议资格或版本号；见
[本轮执行记录](PERSONAL_RESTART_EXECUTION_20261014.md)。下方保留各批次当时授权与验收范围。

**2026-10-13 工作台与个人核心批次，未发布：** 独立个人工作台已实现，使用真实接口显示显式选定
Key 的内部额度/Token/USD/账户窗口、本人记录及提醒；未确认模式仅显示中性加载/重试。
个人资料的金额与签到入口只在已确认商业启用时显示，retirement 继续保留历史入口。
严格预算 prepared 预留在最终检查之后、生成请求派送前才提交 sent；确定未发送可走原释放路径，
提交后未知仍保留。候选 `af79f57` 十项 CI 通过；实际独立视觉发现的一处未选 Key 提示截断已由
`d305bdc` 修正，新准确源码前端整项、1,264 前端测试、10 个工作台浏览器旅程及修正后 13 图
独立视觉复验通过；[CI 38390080664](https://github.com/ForceMind/MyAPI/actions/runs/38390080664)
十项全部 success，含完整 Go、独立 relaykit、两 writer、三库及 race。新工作台截图与报告已随文档归档。
后继仅同步文档/截图，复用准确应用源码 `d305bdc` 的验收，不称文档后继在途 CI 已过。
无 schema/金额迁移，不操作历史记录；详细身份与失败范围见[当前执行记录](PERSONAL_WORKBENCH_EXECUTION_20261013.md)，
不沿用下方旧批次绿灯。
仍不合并、发布或部署，不能把本批写成完整个人核心或生产问题已解决。

**个人状态与日志追加批次，已验但未发布：** 应用候选 `3c78ceb` 为新的 OpenAI Chat 文本请求分别记录
响应完成证据、用量来源和结算待确认状态；回答完整但 usage 未知的日志采用待核对提示，
不会将未知用量作为零值或估算结算。管理员 Users 的非商业视图改为请求次数及实际策略，
无钱包生效仍需要已确认的原始服务端模式和用户审计 revision。个人模式用量区在手机卡片中全宽展示。
[CI 38264888897](https://github.com/ForceMind/MyAPI/actions/runs/38264888897) 十项通过，包含后端
完整测试/race、独立 relaykit、1,206 前端测试及全部合成浏览器。实现与新布局八图独立审查通过。
上轮 `895d1a6` 的分词器测试夹具失败仅补初始化，失败记录保留；收口仅更新文档并复用准确应用验收。
本批失败记录、范围和独立审查见[个人状态执行记录](PERSONAL_STATUS_EXECUTION_20261012.md)。
这不代表独立工作台重新设计或剩余个人核心已完成，也不证明生产结算问题已解决。
仍沿用 Draft PR #12、未发布 beta.11，不合并或部署；下方安装批次证据仅证明各自源码。

**安装页追加批次，已验但未发布：** `21511e5` 修复步骤标题拆词及数据库徽标压缩，增加初始化
POST 15 秒期限与未知结果只读确认。只有明确业务失败可重新提交；未确认结果保持本页写锁。
七语文案同步完成；[CI 38250319423](https://github.com/ForceMind/MyAPI/actions/runs/38250319423)
十项、1,181 条前端测试、25 个新增合成浏览器场景及独立代码/截图审查通过。
原个人资料 9 个、安装/安全恢复 10 个场景通过，合成安装后登录与侧栏个人资料正常。
未知锁只限本次页面挂载，不提供跨刷新幂等；真实发行包/三数据库安装及生产未验证，
本分支 Docker smoke 跳过。收口只改交付文档，沿用 Draft PR #12，不合并、发布或部署。
下段的 `8a482c4` 为上一批已验应用，不代替本追加批次验收。

**2026-10-12 上一批结论：** 优先修复全新安装与日常 UI，历史账务冻结，恢复缺口不阻塞独立开发。
原始数据与证据保留，仍暂不合并、发布或部署。[UI 执行记录](UI_STABILITY_EXECUTION_20261012.md)
与 [Draft PR #12](https://github.com/ForceMind/MyAPI/pull/12) 覆盖安装检查、通知保存偏好及安全状态误报。
最终应用候选 `8a482c4` 的 [CI 38228160614](https://github.com/ForceMind/MyAPI/actions/runs/38228160614)
十项通过，独立代码与关键视觉审查通过；手机 2FA 截断已修复，安装页 P3 窄屏细节仍另记。
收口仅更新五份交付文档，复用准确应用源码验证。本批没有版本标签、Release 或镜像发布。

**2026-10-11 冻结记录：** 暂不发布或部署。当时先按[历史账务兼容核查记录](HISTORICAL_ACCOUNTING_COMPATIBILITY.md)
核清历史账务；下述构建、安装与升级验收只覆盖合成环境，不能代替用户历史数据库的一致性验收。

**冻结时开发状态：** 1608f85基线的[CI38142050913](https://github.com/ForceMind/MyAPI/actions/runs/38142050913)十项success、独立终审通过。当前补原金额方案中的Key显式调整持久幂等和派生总额显示：成功收据与名称/权限/金额同事务，重复请求返回原结果和当前Key；合法组件的总和超M时不再令组件整体不可读。新增表及三库、18项386、前端契约等待本批准确后继验证，不沿用基线绿灯。下文旧“本批后继待验”保留当时事实，最新结果以[云端验收记录](CLOUD_VALIDATION_PROGRESS.md)首节和PR正文为准。版本元数据仍是未发布beta.11候选；本批不改发布标签、镜像、资产或生产。

修复候选已提交[草稿PR #11](https://github.com/ForceMind/MyAPI/pull/11)，包含新请求准入保护、个人资料加载修复、历史金额表示兼容、迁移前只读探针、持久执行许可及Key编辑并发保护。被测源码 `09ef63268b5c6742fe73e246f6c5e26e30559903` 的[CI十项](https://github.com/ForceMind/MyAPI/actions/runs/38128779978)和[Docker五项](https://github.com/ForceMind/MyAPI/actions/runs/38128779248)成功；独立CI提交 `c8e7bbc04676e46f2ad0c806a9ac78562923a4ef` 与其非CI源码相同，[MySQL 5.7/PostgreSQL 9.6专项](https://github.com/ForceMind/MyAPI/actions/runs/38132921489)串行成功、零skip，独立审查通过。个人资料9个Chromium合成场景和既有Redis 7.4.11的44个quota-script子项通过，后者不覆盖全部缓存生命周期。实际Go副本演练仍只持有合成许可，全部副本继续DO_NOT_RESTORE。文档后继3feff8e的前端CI曾被15分钟总时限取消；预算后继3a1d50a的[CI 38135269862](https://github.com/ForceMind/MyAPI/actions/runs/38135269862)十项success且Playground完整完成，旧取消保留。真实Redis补验af18853的[CI 38138227403](https://github.com/ForceMind/MyAPI/actions/runs/38138227403)十项success，Redis7.4.11原44项及新3组/3拒绝子项实际PASS、零skip/fail，独立复核通过。本批继续金额方案的386定向行为要求：修复订阅通知int64→int窄化，新增纯DTO测试；在现有backend386build后增加五包14项严格JSON门禁，无新服务、权限或上传。准确后继SHA/终态以[PR Checks](https://github.com/ForceMind/MyAPI/pull/11/checks)回读，旧386全包失败及mock限制保留。新增应用差异限通知组装，旧09ef Docker/c8e7 native继续只按自身身份及未变路径复用，不能称新整棵业务tree等于旧源码。候选未进入main、发布镜像或资产；beta.3来源、资金/预扣/缓存/全部writer、504事件、旧终态凭证来源和实际回滚资格仍缺，生产账务未核清。详细证据及历史失败范围见[云端验收记录](CLOUD_VALIDATION_PROGRESS.md)。

**Not published. Do not pull or deploy the beta.11 example image yet.**
The currently available prerelease remains [beta.10](https://github.com/ForceMind/MyAPI/releases/tag/v0.2.0-beta.10), fixed at
`631b3205ab2ad28df3f9372bd00f02bd27cb4c31`. Do not move that tag or overwrite its assets or images.
The current local repair baseline is main `92fa1182e265924312d70917a0a637965aebc617`, tree
`01664d716e57f3fc99e2d92c61b6f31217c5a92b`. The earlier release-preparation baseline was
`b8eec75d135f5489fff884820539b5abe99ae312`, tree `c51954e8abae5e251f050b04426d921af2a8402b`.
Its own final source head, merge checkout,
CI and release identities must be recorded after execution; earlier green commits do not qualify it.

下面保留的是历史发布准备记录，其旧候选SHA和合成验收不覆盖上述修复候选。main仍为 `92fa1182e265924312d70917a0a637965aebc617`；以本文开头的部署禁令和核查记录为准。


c94e5ed的[CI38140987775](https://github.com/ForceMind/MyAPI/actions/runs/38140987775)后端在386编译旧service夹具时失败：三处超出int范围的常量，其他13项/四包实际PASS，service未运行，不能称14项或完整CI成功。MySQL/PostgreSQL的完整schema子项各一次RUN/PASS、零skip/fail，21表38列、13个宽字段及七类匹配行回滚检查通过；Redis7.4.11共51次RUN/PASS各一次、零skip/fail，两job容器/网络清理成功且独立复核通过。本批只修两份测试的架构输入，保留64位原极值与原断言，并将三项改动测试加入同一386门禁，共五包17项；生产代码和其他CI配置不变。后继准确结果待[PR Checks](https://github.com/ForceMind/MyAPI/pull/11/checks)，失败记录保留，旧c8e7仍限子集，当前schema成功不证明旧窄列升级或历史账务已核清，详见[云端记录](CLOUD_VALIDATION_PROGRESS.md)。

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
