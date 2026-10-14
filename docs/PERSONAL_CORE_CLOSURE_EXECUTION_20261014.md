# 个人核心流程收口与下一阶段

## 范围与基线

按维护者最新指示，先完成个人核心卡步骤4，再推进 Key 共享百分比。历史账务继续冻结；
本轮不部署、不发布、不合并 main，不访问或修改生产服务、数据库和凭据。

开发基线为 `963fabe521e9905c450b2e45968d0d7402488594`，应用源码仍为
`e613350eb2a96b1d59a66c0613f22b9aee9f2477` / beta.12。
基线 CI `38450353211` 十项、Docker `38450365161` 五项已通过，最新文档 CI
`38518589155` 已通过；旧验证按各自源码及范围复用，不替代本批准确提交验收。

独立开发分支 `codex/personal-core-20261014` 从该基线建立，后续以
`codex/personal-restart-20261014` 为 base 创建 Draft PR。既有 PR #12/#13 与历史标签保持原样。
持久 Goal 已创建，无指定预算；阶段清单与验收保存在本记录。

## 本批实施方案

1. 核对已有初装、个人资料、有限 Key、两 writer、并发、撤销、未知恢复及商业关闭测试，
   建立逐项证据。已完成行为不重复实现，不以合成测试声明真实 provider 或历史账务已解决。
2. 修复工作台进入 Playground 的 Key 选择断点：目前入口只导航到 `/playground`，
   聊天页沿用自身保存的 Key，可能与工作台明确选择不同。选择交接只由显式点击产生，
   绑定当前用户与会话；聊天页重新验证本人 Key 列表及可用状态。失效选择不能回退到更宽 Key，
   交接不能发送生成请求，也不能迁移凭据、会话或聊天内容。
3. 补齐个人工作台七语言 320/1280 实际 Chromium 流程，以及进入聊天页后的选择一致性、
   无效/过期选择和无自动 POST 检查。现有 driver 仅 en/zh/fr，导航检查止于 URL。
4. 独立审查改动与实际截图，按准确 head 运行 GitHub Actions 的类型、lint、行为、
   后端与相关三库回归；需要时复用现有隔离应用验收，不新增生产或宿主机服务。
5. UI 行为交付更新下一候选为 beta.13，代码、版本、证据与文档一起正常推送 Draft PR。
6. 个人核心收口后，按已确认的新合同实现共享百分比。当前只有账户窗口安全阈值，
   不存在逐 Key 分配账本；总池计量和份额含义待维护者回答。账户前后变化不能自动算作某个 Key 用量。

## 分工与资源

主智能体负责合同、分支、版本、工作记录、CI、集成与交付。只读规划使用
`gpt-5.6-sol/ultra`；局部实现和独立审查使用可用的 `gpt-5.6-sol/high`，
不用不可用 Terra 型号。并发最多三个子任务，不递归派工；共享文件按所有权串行写入。

前端实现负责工作台/Playground 的选择交接及对应回归；浏览器实现仅负责
`tools/ui/personal-workbench-browser.mjs` 及必要测试；实应用实现仅负责
`tools/runtime/personal-browser.mjs` 和对应编排测试。版本、CI 和文档由主智能体写。
如需新文案，先读取 i18n skill，再通过指定工具维护七语言；不直接改 locale JSON。
本机仅轻量检查，依赖安装、构建、浏览器和完整 Go 测试使用既有云端工作流。

主要风险是错误复用以前保存的 Key、过期身份/列表、异步加载覆盖用户新选择，以及长译文布局。
用用户可观察的选择、取消/过期、权限、没有生成 POST 和实际像素证据验证；
不改资金/结算模型、协议或历史义务。业务合同变化、权限受限或必要验证无法完成时记录具体缺口，
已获授权的独立工作继续推进。

实际工作台路由为 `/dashboard/overview`，公开 `/` 不是工作台；浏览器与真实应用均以实际路由验收。
工作台普通读取已通过 userId/sessionId 查询键、会话切换清缓存及响应归属校验隔离；
目前未取得其越权或串会话证据，不把缺 AbortSignal 的普通查询当成本轮已发现漏洞扩大改动。
新交接的 ownKeys 校验则需要当前会话、取消信号和新鲜 HTTP 请求，不能只换 React Query 键
却继续复用旧的底层请求。

## 既有个人合同与本批验收对应

| 个人流程 | 已有直接行为证据 | 本批补验 |
| --- | --- | --- |
| 初装 Root/现有用户显式策略、个人资料 | `tools/runtime/personal-browser.mjs` 的初装/Root确认/owner拒绝后允许；`tools/ui/setup-browser.mjs`、`profile-browser.mjs` | 准确候选完整 CI；隔离应用两 writer |
| 零钱包与有限 Key、保留 Key 记账 | `TestSelfUseSessionKeepsKeyAccountingWithoutWalletAcrossWriters` | 保留真实调用与有限额度拒绝，不改后端 |
| 到限、并发与派送前撤销 | `TestTokenBudgetConcurrentAdmissionAndAtomicFailure`、`TestChatBudgetActualApplicationReservationExhaustion`、`TestChatBudgetActualApplicationFreezesInflightPublicationAndRollback` | 准确 CI、race及配置三库 |
| 明确零值、未知、重开、幂等核对 | `TestTokenBudgetReportedZeroAndPreDispatchCancellation`、`TestAssignedAccessStrictResponsesUnknownReopenRecoveryBothWriters` | 沿用既有持久事实回归，不操作历史库 |
| owner/Root 权限与商业关闭 | `TestSelfUsePolicyActualRoutesProtectAuthorityAndUnsupportedPaths`、`TestPersonalUsageAdmissionAvoidsCommercialTablesAcrossWriters` | 准确 CI；商业隔离 canary 仍只证明 SQLite 准入/预扣 |
| 工作台选择→聊天页同 Key | 原 driver 仅到 URL，没有覆盖，属于实际缺口 | 新交接、失效/跨身份不回退、列表新鲜校验、无自动 POST |
| 手机/桌面、长译文及操作 | 原工作台 driver 仅 en/zh/fr | 七语言 320/1280 Chromium、截图与独立视觉审查 |

GitHub Actions 的 Docker smoke 增加手动 `personal` 布尔输入，默认 false；开启时复用现有
隔离 Redis/合成上游和原两 writer 流程、单次不可变镜像构建，不推送镜像，也不部署。
原安装、历史拒绝/原备份恢复及 LAN 源码测试保留。前端 job 原准确源码耗时15m57s，
本批新增七语言两尺寸流程，作业上限由20改25分钟，单项测试截止时间保持不变。

## 阶段状态

| 阶段 | 状态 | 完成证据 |
| --- | --- | --- |
| 基线与剩余合同核对 | 已完成 | 远端、基线 CI 与已有行为证据逐项对应 |
| 工作台 Key 交接修复 | 待验证 | 一次性会话绑定交接、新鲜请求、失效空选、取消消费、手动恢复已实现 |
| 七语言/响应式流程 | 待验证 | driver 已覆盖23条流程/46张截图目标，含七语言两尺寸及阻断反例 |
| 准确提交 CI/独立审查 | 进行中 | 首轮静态独审问题已修；待准确 head 云端行为/截图 |
| 共享百分比最小合同 | 待澄清 | 已询问总池计量和份额含义 |
| 共享百分比实现 | 未开始 | 依赖个人核心验收与新合同 |

完成本批仍不代表真实 OAuth、订阅窗口429、provider收费、公开入口或历史恢复已验证。

## 本机实施与审查检查点

UI 交接仅存内存、30秒失效，绑定用户与会话。进入 Playground 即清空当前用户旧选择，
成功校验才持久化候选；卸载消费 pending 意图，取消后的迟响应不能恢复选择。
测试入口改为真正按钮再导航，避免改键/新标签页链接跳过交接。直接进入页面且无交接仍恢复本人配置。
列表错误或目标失效后，用户明确选择有效 Key 可以恢复，拒绝提示随有效选择清除。

独审发现并修复：未完成交接在离开页面后可能重试、新标签页链接可沿用旧 Key、
真实浏览器 Key 身份文本同时匹配 option/p、baseline红测先命中请求参数而非选择断言、
Full/LAN personal artifact 重名、新增 workflow inputs 未接静态测试，以及根 Compose 文档链接落后。
每项按实际问题修正，未放宽权限、金额或结算断言。

本机 `git diff --check`、Bun 外部依赖解析、i18n sync、runtime/installer/immutable-bundle Node
针对性测试通过；未安装前端依赖，Vitest/TypeScript/完整构建/Chromium/真实应用留给 GitHub Actions。
手动准确 CI 的可选 `baseline_red` 先在固定旧实现上运行同一选择回归，要求预期可观察断言失败；
恢复候选原文件并确认无差异后继续原全部检查。仅测试环境执行，不改部署。

首轮提交 `7211d90f0698ac3fd477f17cfd830eec373fe0f7` 已同步 Draft PR #16。
准确 CI `38523579318` 成功执行旧实现红测与文件恢复；候选 Typecheck 发现新增测试的
`initialProps.keyHandoff` 推断过窄，无法 rerender 为 undefined；交付合同发现运行报告阶段白名单
未同步新工作台阶段。均为真实失败，修订显式测试类型并同步固定阶段集合，不删除测试或弱化断言。
同轮 Docker `38523582111` 属于首轮源码，后继以新准确提交重验；首轮记录保留。

修订 `bb1c1866a9feeaba86038fc758258de061acdf51` 已正常推送，准确 CI `38524169925`
与 Docker `38524172311` 已启动。前端类型/lint/测试、交付合同及 LAN 源码安装已成功；
完整后端、浏览器及镜像应用验收仍在运行。旧过期作业与重复 PR CI 已取消，
保留首轮原始失败及红测合成报告；本机四文件 Node 专项153项已在允许 loopback 的环境通过。

同轮真实 Full legacy 应用已走到工作台、目标 Key 与 fresh GET 校验，随后在 `key-selection`
等待20秒而失败。驱动对关闭的原生 `option:checked` 使用默认 visible 等待；修订为 attached，
仍要求可见 select 的精确值与可见 Key 身份文本。修订不手动选 Key、不更改应用、不跳过断言。
真实重验前不把该定位假设或 Node double 通过写成应用成功；原 Docker 失败记录保留。

## 共享百分比：已核事实与待定输入

当前 Codex 请求的凭据账户身份可以与最终发送绑定；可信 WHAM 样本仅提供同一账户的整体
窗口百分比、时长和重置时间。已有阈值因此只做请求前安全避让，不能证明本次请求使用多少百分比。
样本没有本地 Key/request_id 或单请求窗口单位，也没有已验证的 Token 到订阅窗口百分比换算。
并发 Key、外部客户端、延迟、取整和窗口重置都可能影响账户快照差值。

依据为 `docs/R1_CURRENT_ITERATION.md` 账户阈值合同、`docs/NEXT_USABLE_VERSION.md` 的
逐 Key 可归属事实要求，以及 `service/codex_wham_usage.go`、`model/account_quota_threshold.go`
和 `service/account_quota_threshold.go` 的实际采样与发送前校验。Codex 会话头不是计量事实。

下一阶段必须先确定“共享100%”的总池单位与预期控制：本地 Token/费用池，还是每个订阅账户
的原生窗口；需要硬限额，还是只读观测/路由策略。不能把静态分配比例或现有安全阈值
标为已经实现的逐 Key 消费上限。用户问题仍待回答；在此前不新增分摊表或推算用量。
如果引入持久池、并发分额或恢复状态，单独列明三库事务、CAS/审计、两 writer 及未知用量合同，
仍不解冻历史账务，也不自动部署。
