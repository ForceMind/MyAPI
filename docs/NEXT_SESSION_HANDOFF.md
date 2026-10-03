# My API 当前开发交接（2026-10-02）

2026-10-03 09:16 北京时间：7072dbb已同步，新人工恢复三库实际PASS（联合SQLite0.43s/MySQL1.26s/PostgreSQL5.56s）。573前端测试/构建通过，Chromium发现中文窗口缺少本地化关闭入口（共用图标名是英文Close）。本小批仅在新窗口提供既有翻译Close按钮、保留Escape并补不写入回归；不改英文选择器掩盖问题，不改后端/数据库。前端最终122文件574测试/类型/lint/构建通过，新SHA浏览器待复验，不称前置十项全过。详见R1_CURRENT_ITERATION首节，真实验收仍未完成。

2026-10-03 08:58 北京时间：c7ab856 的 CI37081017788 十项全过，新增双writer派送三库和568前端/Chromium及精确树已核验。本批接用量日志Root待核对列表（有界keyset、含无完成日志的持久派送）与确认请求结束后的人工恢复。现有确定结算不得被覆盖，迟到自动意图/事实在事务内受生命周期锁保护；接管先保存首次金额/操作者/证据摘要，失败后相同命令重试。无新表/支付/协议扩展，旧无证据open/prepared不猜零退款。新增模型/路由race通过，前端122文件573测试/类型/lint/构建通过；补修人工终态后仍可创建迟到新结算工作的缺口，最终Go/vet与模型race6.877s复验通过，秘密扫描无发现；新SHA三库/320和1280px Chromium待核验；详情以R1_CURRENT_ITERATION首节为准，真实验收与未覆盖供应商边界保留。

2026-10-03 08:09 北京时间：a299d6a 的 CI37078122556 十项全过，三库/568前端/Chromium与合并树一致已核验。下一有界候选把普通HTTP文本的发送标记及既有计价证据保存到现有双writer行，保存失败不发上游；重启后的直接退款、后台退款和扩展也需遵守标记，已知拒绝经CAS才清除。无新表或UI，原missing/estimated不变；本地定向及相关race、最终完整Go/vet通过，畸形标记模型race复验4.942s通过，秘密扫描无发现；新SHA三库CI待验。完整崩溃记录发现和Root接管入口下一批再接，旧无证据open/prepared不猜零、不退款，所有实例须支持新语义，真实环境仍未验。详见R1_CURRENT_ITERATION首节。

2026-10-03 07:29 北京时间：3697c89 的 CI37072061927 十项全过，既有三库恢复、568前端测试和Chromium已核验。本批仅补普通Key标准HTTP Chat/Responses按Token计量的发送不确定性：发送后解析/网络等不确定失败保留预留，进入既有待核对并禁止自动重试；发送前取消与原始明确拒绝（含429）保持原语义。双writer/自用/旧限额、Root幂等恢复、写入失败保护及真实HTTP断连单次发送测试通过；本地错误防伪先红后绿，最终根Go全测/vet和相关race通过，原missing/estimated文件未改。无新表/UI/预算资格扩展；源码文档同批同步后必须核验新SHA CI。本轮不是普通Key持久派送或完整崩溃恢复，prepared/open恢复及真实环境边界仍未闭合。详见R1_CURRENT_ITERATION最新节。

2026-10-03 06:17 北京时间：8121b9e的CI37066909385十项全过，三库恢复与响应式用户策略页面已核验。本批继续同一R1，新增Claude有效JSON字段存在性/流终态证据，缺失/null/估算/累计输入输出倒退进入既有待核对；明确0不估算，缓存-only不再误作零消耗，TTL超界不饱和为确定扣费。适配器与结算合同先红后绿，原始missing/estimated文件未改；最终根Go/vet、独立relaykit全测/build/vet与定向race通过，无UI/数据库结构/费用预算资格扩展。源码文档待同批同步，新SHA CI待验；详情见R1_CURRENT_ITERATION首节，其他供应商与真实环境边界仍保留。

2026-10-03 05:24 北京时间：2694048 已同步，CI37065906993 的新增自用/Token/USD/旧限额/审计恢复三库实际通过（SQLite0.35s、MySQL0.94s、PostgreSQL5.31s）。Chromium发现用户策略弹窗随表格从桌面切到移动卡片时卸载，导致未保存状态丢失；本补修将弹窗移到稳定页面层，沿用既有Provider目标用户/弹窗状态，新增跨320/1280px保留勾选的浏览器断言。原失败保留，不降低超时或跳过断言；新SHA仍须浏览器复验。预算/结算/数据库规则不变，真实验收与独立迁移race限制保留。

2026-10-03 05:07 北京时间：前置0b60819的十项CI、阈值三库与Chromium已通过。当前新增非充值自用准入候选：Root独立审计策略、新安装Root零余额、冻结self_use来源、双writer/Key限额/严格预算/未知恢复兼容及用户管理页。现有用户不自动取消限额，新注册普通用户不自动无限；首批仅Chat/Responses，其他入口对新策略拒绝。前端568测试/类型/lint/构建与最终完整Go/vet通过，审计详情及冻结来源路径二次门禁先红后修、定向race复验通过；另删除全仓无调用的历史固定默认口令初始化函数，保留正式Setup流程；新SHA三库与浏览器尚待同步后验收。详见R1_CURRENT_ITERATION首节，不重做前批，不称R1完成或可部署。

2026-10-03 03:18 北京时间：44c0ca1 的 CI37051741438 十项全过，新增阈值三库/Chromium通过。核看截图发现取消冲突开关后旧提示残留，本小批修立即校验并新增组件/浏览器回归；不改后端，待新SHA验证。详见 R1_CURRENT_ITERATION 最新节，真实验收与批迁移race限制不变。

2026-10-03 02:28 北京时间：本批账户/窗口安全阈值已接原生 Codex 可信新采样、路由/固定账户/发送前门禁，以及 Root 设置与所有者只读页；不与当前直连 Token/USD 模式混开。旧样本不回填资格，缺失百分比不再默认满额。根模块测试/vet与前端562测试/类型/lint/构建已过；阈值定向 race 通过，但既有批迁移测试单独 race 仍超时，已如实登记。源码/文档待同批同步，新 SHA 三库与浏览器待验。最新边界与证据以 R1_CURRENT_ITERATION 首节为准；真实环境和最终自用验收仍未完成。

2026-10-03 01:26 北京时间：`b0b7a3f` 费用候选的 CI `37037635553` 十项全过，三库与费用 Chromium 已核验。下一有界迭代为逐账户/窗口剩余百分比避让，限定可信 Codex 采样；不分摊逐 Key 百分比，不与当前直连 API 严格 Token/USD 模式混开，不将旧不完整样本当新证据。具体范围、默认新鲜度与验收见 R1_CURRENT_ITERATION 最新节。真实环境及最终自用验收仍未完成。

2026-10-03 00:52 北京时间：基于 `afaf250` 的费用预算候选已接精确 USD 预留/结算、Root 恢复和 Key 页；费用与 Token 独立开关，复用同一未决请求和 writer。当前本地验证已过，新 SHA 三库/Chromium 待同步后验收。费用模式必须显式 service_tier=default、匹配模型和完整缓存分类证据，只计冻结官网价格下的实际 Token 成本，不等于上游发票。先读 R1_CURRENT_ITERATION 最新节，随后推进账户/窗口安全阈值；真实环境与最终自用模式验收未完成。

23:53 北京时间：`afaf250` 的 CI `37028335783` 十项全过，新增 Token/双账本恢复三库及 Key 页 Chromium 合成操作已通过。下一有界迭代为精确 USD 费用预算，复用现有预留/恢复与冻结官方价格，当前费用预算尚未实现；先读 R1_CURRENT_ITERATION 最新节。账户百分比与真实部署验收仍未完成，不把上一批候选验收等同 R1 封版。

23:12 北京时间更新：前置 `36e72a2` 的 CI `37011043799` 十项全部成功，独立 Token 模型合同已在 SQLite/MySQL/PostgreSQL 实库通过。本批继续同一严格 Token 迭代，新增原生官方 Responses 的精确输入计数/发送上限/持久准入、正常或未知结算、两种 quota writer 的审计恢复，以及 Key 管理页设置/恢复入口。当前代码的限制与验证见 R1_CURRENT_ITERATION 最新节；本批 exact-head CI/新增三库耦合与 Chromium 页面回归须同步后核验，不得复用前批绿色。实际费用预算与账户百分比仍未接，不称 R1 完成或可部署。

## 2026-10-02 后续接管状态（本节优先）

21:00 北京时间更新：`7295ce1` 的 CI `37006732955` 十项全部成功，Chat 零值/缺字段证据已收口；前置价格发布检查点也已通过。当前同一 Token 预算迭代新增独立整数预算、持久请求预留/待核对/Root 审计恢复的模型基础、迁移及隔离合同，尚未接运行时准入、公开 API/UI，不开放配置开关。SQLite 定向与 race 通过，新三库合同待本批 SHA 的 CI；不能说 Token 预算已可用。完整范围与证据见 R1_CURRENT_ITERATION 首节，下文旧待验状态为历史。

19:15 北京时间更新：前置 `3094e5e` 的 CI `36994474226` 十项全过，新增发布合同三库实际 PASS。其后本批已接 Root 预览/确认 API、来源页发布/锁/回滚、请求版本冻结及日志参考成本标识，本地必要验证通过；本批 exact-head CI 与新增 Chromium 操作尚待同步后验证。当前仅支持受限 Standard 纯文本参考计价，费用/Token 预算未接，不能冒充上游账单或 R1 完成。详见 [当前迭代最新节](R1_CURRENT_ITERATION.md)，优先于紧随其后的基础检查点描述。

已验证绿色检查点为 `9410b02`，位于 `codex/r1-usage-review-20261002`；[草稿 PR #2](https://github.com/ForceMind/MyAPI/pull/2) 的 CI `36989602580` 10 项全部成功，包含恢复表单的新增 Chromium 合成操作及三库合同。当前继续有效价格发布迭代，已加入来源候选表达式、数据库原子发布/锁/回滚基础与回归；尚未接通公开 API、页面和请求冻结，不可当成可操作发布功能。本批 exact-head CI 待同步后验证。下文“未推送”和旧红灯是早期记录，最新边界见 [R1_CURRENT_ITERATION.md](R1_CURRENT_ITERATION.md)。R1 新预算和真实部署验收尚未完成。

负责人已恢复 R1 范围内的实现、隔离验证与文档更新，并授权选择安全可落地的业务规则。已明确：严格费用/Token 预算的未知用量暂停后续请求、保留预留等待证据或授权核对；百分比采用逐账户/窗口的剩余安全阈值，不伪造逐 Key 百分比消耗。**这是确定的规则，实际费用/Token 预算及准入联锁仍未完成。**

当前候选从远端 `codex/r1-handoff-20261002` 的 `71277bf6055ce68b8cd1d11f1d10f1907d7696fd` 接管，在当前实际 checkout 开发，不使用下文旧设备路径。原 missing/estimated 合同已经在本地候选转绿；原断言保持。增加持久待核对、授权恢复、幂等预留准入、统计/日志投影与界面，并继续第一轮验收；详见 [当前迭代与证据](R1_CURRENT_ITERATION.md)。

负责人于 2026-10-02 07:03 UTC 授权自主推进既有 R1 开发、源码同步、草稿 PR 和 CI 修复；开发分支为 `codex/r1-usage-review-20261002`，基线为交接分支。远端实际同步和 CI 以回读为准。`9410b02` 的 MySQL/PostgreSQL 隔离实库合同与 CI 已验收；新增价格发布代码需重新验证自己的 exact head。真实浏览器/容器/OAuth/账单及升级回滚仍未验收。正式上线与真实账户/凭据操作另行确认。每次源码与文档同步后主动汇报，附北京时间及距上次汇报间隔；不得用旧快照红灯或局部绿色代替当前事实。

本节是当前入口；下方旧 Linux 模板只保留历史，不作为当前目录、授权或完成状态。启动提示词见 [CODEX_HANDOFF_PROMPT.md](CODEX_HANDOFF_PROMPT.md)，R1 需求及逐批证据见 [NEXT_USABLE_VERSION.md](NEXT_USABLE_VERSION.md)。不重新规划 F1–F8，不重做已完成批次。

**同步结果（2026-10-02）：** 源码快照 `f93695873c4413d42a48fe1af302bee3bc70be98` 共 232 个文件（含继承商业支付 WIP），随后交接文档提交 `17ba246466e285e73b914f9a80fc58af9b347aec`。首次公开推送被安全审核拒绝；向负责人披露范围后，负责人再次明确要求推送，原命令重试成功，没有绕过审核。[GitHub 交接分支](https://github.com/ForceMind/MyAPI/tree/codex/r1-handoff-20261002) 已存在；后续同步状态文档提交以远端同名分支回读为准。不写 main、不创建 PR/tag、不发布或部署，CI 和可部署资格仍未验收。

## Git 与工作树

- 仓库 `https://github.com/ForceMind/MyAPI`；交接分支 `codex/r1-handoff-20261002`，未封版源码快照。提交通过 `git rev-parse HEAD` 与远端同名分支回读，本文不填写自引用 SHA。
- 基线 `57ec31a58fc737ba2be0601e4102123436c3d938`；交接前远端 main 是 `f6536ca96126415f74d239165fd688589bd54af9`，不是该基线。不要先 pull/rebase main，也不要强推或直接覆盖 main；集成与合并另行授权。
- 候选 `/Users/wxx110/.codex/worktrees/next-usable-candidate/MyAPI`：交接开始时 detached HEAD，229 个修改/新增文件条目。快照包含从原工作树继承的商业支付 WIP 及候选 R1 改动，不把全部差异称为本次新增或已验收。
- 主目录 `/Users/wxx110/工作/Prive/MyAPI` 当时干净，分支 `codex/mac-durable-accounting`，仍在基线，没有被候选覆盖。
- 原 `/Users/wxx110/.codex/worktrees/2daa/MyAPI` 保留全部 WIP，只读保护，不 reset/stash/clean、归档或整包搬入。候选不等于原工作树全部成果。
- `VERSION` 仍为 `0.2.0-beta.1`；历史 tag 不代表本候选发布。此次只提交/推送交接源码与文档，不创建 tag、PR、合并、镜像或部署；不包含 `.env`、数据库、真实日志、凭据、依赖缓存和构建产物。
- 本轮读取 Goal 返回 null，没有创建自动化或子智能体；下一模型先查目标，不声称旧 Goal 仍持续运行。

## 已有成果与边界

|流程|已有候选成果|未完成/待验|
|---|---|---|
|自用 UI|新安装商业默认 disabled；关闭模式隐藏购买推广/当前余额；首页概览简化，用户/Key/历史恢复入口保留|旧模式不强改；三种新预算未实现；商业启用资格未验收|
|额度及渠道|同图细线与系列选择，多 Key 采样/身份隔离，Codex 历史按 series_id，测试复用成功 model/endpoint/stream，确切耗尽 429 避让|专用历史无新 UI 选择入口；真实账户、容器 OAuth/刷新、上游测试/路由计数待验|
|usage/实时|Responses 明确零值不覆盖，冻结 usage/reasoning/cache 明细；日志来源标签；冻结单位/模型/工具倍率；实时累计预留及重复扣费修正|未知/部分/畸形 usage 未收口；reported 不等于账单核对；实时有界汇总不是持久逐 response 证据|
|官网价格|固定官网抓取/严格十进制解析；Root 保存不可变 SHA256 来源版本及按摘要读取；独立参考成本计算和 Root 页面|保存不等于发布；未应用有效价格或 Key 预算；差异确认、管理员锁、原子发布/回滚、档位/上下文/地区/工具/多模态资格未完成|

商业支付代码仅保留默认关闭模块及历史恢复，不继续扩展支付商，不宣称可以安全收款，不建设动态插件框架。

## 红灯和规则确认

`go test -p 1 ./service -run '^TestPerTokenSettlementRequiresReportedUsage$' -count=1` 中缺 usage、Estimated usage 合同失败：预留 100 后仍确定扣费 100/120；reported 对照正常扣费 120。service 全包不能宣称通过，历史绿色已被新合同推翻。禁止删测试/放宽断言、猜零价或自动退款制造绿色。

先集中确认两条规则，未确认前不实施关键副作用：

1. 严格费用/Token 预算 Key 遇到未知 usage，是否暂停后续请求、保留预留，等待可靠 usage 或授权人工恢复？少量预留或继续估算不保证硬限额；固定按次/独立工具费用分别处理。
2. 百分比是账户剩余安全阈值，还是每个 Key 在各账户窗口内独立消耗预算？共享账户差值不能可靠归属 Key，不伪造逐请求百分比。

原 2daa 的 usage_unknown 冻结/审计/恢复未纳入候选，legacy 保留仍有进程内边界；不得整包复制来冒充持久保证。改结算前读 `pkg/billingexpr/expr.md`，核对预留生命周期、writer 模式与权限。

## 验证与下一迭代

- 2026-10-02 交接复核：Gitleaks 8.30.1 官方校验和通过，扫描暂存补丁约 1.27MB 未发现秘密（只覆盖本次差异，不是全历史无秘密保证）；暂存 diff 检查通过。relaykit 使用 `GOWORK=off GOMAXPROCS=1 GOMEMLIMIT=768MiB go build ./...` 独立构建退出 0。
- GitHub API 确认基线 `57ec31a58fc737ba2be0601e4102123436c3d938` 已存在于公开仓库；源码快照仅在其上新增一个提交，另有交接文档提交，不夹带额外未核查的祖先历史。未因推送拒绝改变仓库可见性、换工具外发或隐藏商业 WIP。
- 同日重跑上述未知结算合同，1.770s 退出 1，missing/estimated 两项仍失败、reported 对照通过；未修改生产行为或隐藏失败。GitHub API 回读仓库为 PUBLIC，快照范围为候选源码与交接文档，不发布制品。
- 逐批命令/日期见 R1 卡，按变化范围复用，不是当前全部仓库绿色或独立审查。最后 controller/router 全包 2.048s/1.017s、相关 race/vet 通过；最后 service 全包 6.977s 退出 1，仅上述两项红灯，后续该包源码未变。
- 最后官网来源 UI 14/14 合同、typecheck/lint、构建 2.06s 通过；新增七语言键 missingCount=0，历史未翻译项未整体补完。沿用既有 UI 组件、会话隔离与指定 i18n 脚本，未增加项目依赖。
- 320/390/768/1440 合成浏览器稳定布局、保存反馈、失败隐藏旧表、按摘要读取及键盘滚动已观察。`http://127.0.0.1:8769/system-settings/models/openai-pricing-source` 只是本机静态构建+合成 API，进程可能结束，不是后端持久化证明。
- SQLite 和跨库声明/合同代码有部分证据；MySQL 5.7/PostgreSQL 9.6 实库、当前交接 SHA 的 CI、真实容器/OAuth/上游账单、升级/恢复/回滚未验收。relaykit 独立构建历史证据在 R1 卡，新改其 API 后须 `GOWORK=off go build ./...`。
- 官网曾实取不等于当前价格相同。截图留在本机会话可视化目录，未入 Git：`r1-saved-price-source.jpg` / `r1-saved-price-source-mobile.jpg`。

接管先执行 `pwd`、`git status --short --branch`、`git remote -v`、`git log -5 --oneline`、`git rev-parse HEAD`；确认远端交接分支，不切 main 丢失候选。读取本节、R1 卡和 AGENTS 后只定位本轮必需代码。

下一迭代优先“正常结算 → 未知待核对 → 权限恢复 → 幂等限额”的完整可操作闭环，依赖上述规则确认。缺确认先安全诊断，不继续无限增加价格辅助能力/支付功能。随后接价格发布和费用/Token 预算，百分比按确认口径最后接入。每版固定范围、查看入口、验收和停止条件，额外需求放下一版。

## 历史归档：旧 Linux 模板（不用于当前接管）

# My API 新对话启动提示词（Linux 源码开发）

编制日期：2026-09-05；2026-09-06 增补产品、发行与 S5-P 交接事项。将下方文本作为负责人指令复制到在 `/root/myapi` 打开的新对话。
这份模板只有在负责人实际发送后才构成其中描述的授权；文档本身不授予额外权限。
完整任务拆分见 [全项目执行计划](PROJECT_COMPLETION_EXECUTION_PLAN.md)。

本提示词用于接管已本地提交的 B2-1 候选，不使用旧 Mac 交接提示词，不重新要求 HEAD 必须停在旧 main。

## 2026-09-06 使用前补充

模板中 B2-1 的历史状态仅用于说明当时授权，实时 Git、[全项目完成执行计划](PROJECT_COMPLETION_EXECUTION_PLAN.md) 和 [完成度审计](COMPLETION_AUDIT.md) 优先。当前已知还有未提交的本任务代码/文档；新对话必须先按模板核对工作树，不得覆盖或假定归属。新对话还必须完整读取：

- [提示词学习与版本中心（S5-P）](PROMPT_LEARNING.md)：默认关闭、三层授权、合成样本/预算/版本/Codex 文件边界；不得把模板当作读取真实日志、调用模型或覆盖宿主指令的授权。
- [发行制品、安装与更新合同](RELEASE_MANIFEST.md)：Full/Lite/Desktop、功能版/安装形态/访问模式分离、Legacy LAN 映射、统一入口、更新/切换/恢复边界；不得把合同当作发布、tag、生产升级、防火墙或公网开放的授权。
- [Lite 与 Legacy LAN 迁移](LAN_LITE.md)：旧 `MYAPI_EDITION=lan` 只映射 Lite/local 或 Lite/lan，升级不能自动 public。

当前局部证据也应按范围读取：Ali、Doubao、Gemini、Hailuo、Jimeng、Kling、Sora、Suno、Vertex、Vidu 的 B2-2A parser 在 HTTP 200 时已由 legacy `DoResponse` 调用，gate-off 非 200 安全兼容 bridge 在 parser 前处理；Task 单个 outbound attempt 的 one-shot body、3xx 不跟随、客户幂等头隔离和 Vertex OAuth JWT 换取无重定向均已有受限制最终测试与独立审查，但没有 durable 提交接线。B2-2B0 的设计确认不能包裹旧 retry/failover，真正 POST 必须等 D02 与 B3-A；当前另有无 caller 的严格 protocol、T0 owner/replay 原子基元、Full Content 字段级幂等脱敏，以及 owner-scoped、no-store 的只读 `GET /v1/task-operations/:id`（DTO/router/controller 已接线），B1a/B1b 另有 JSON 与仅 video form/multipart canonical request fingerprint，仍无 POST、账务或上游。`setting/system_setting/legal.go` 与 `setting/perf_metrics_setting/config.go` 是 C09-N1 的私有 immutable generation：分别保持 legal 原键/默认/controller 输出，以及 perf metrics 四键、整值浮点解析和读时 fallback，仅返回 detached snapshot；另有 `setting/payment_runtime.go` 的 C09-N2a immutable payment generation，它涵盖现有支付 option 与 `TopupGroupRatio`，三者都不读写 legacy global、OptionMap、DB、SDK 或网络，不能被当作付款/回调/账务已迁移。C09-N5a 的 `/api/option/diagnostics` 是 RootAuth/no-store 的有界主库只读诊断，业务层不触碰 OptionMap/runtime/LOG_DB/Redis，但平台鉴权/限流仍可使用自己的 cache；不回显 values 或不可信 key，MySQL/PG/CI 仍待。`service/promptlearning` 是未接线的 P1a/P1b 纯准入/脱敏/指纹内核：仅 package-private 完整 server-observed turn 可提取一个可信新增用户段，排除段/客户端来源声明不进入样本或指纹，仍不能读取真实日志或计数。`service/accesspolicy` 是 S3-PRE1 无 caller 的 detached snapshot/diagnostic，所有 mode 都不应用策略、不触碰路由/价格/账务/缓存；D04、audit ingress 和 enforce 仍待。`cli/lib/release-manifest.mjs` 与 `installation-state.mjs` 分别是 schema-1 结构选择和内存安装状态/cleanup plan，且拒绝 Proxy/访问器等 hostile JS 输入；它们不能获取/验证资产、读写文件、安装、更新、切换、回退。上述子范围均不改变 C03b gate、生产、真实模型或宿主文件边界。

本轮的其他有限本地证据如下：B2-2B1a/B1b 已补严格 JSON 与仅 video form/multipart canonical digest/版本化请求 HMAC，仍未接入 POST/账务/上游；C09-N1 现有 22/24 个注册族采用受控快照/复制，新增 `gemini`、`billing_setting`、`payment_setting` 与 `global` 后，仅 `performance_setting`/`channel_affinity_setting` 分别等待 D14/D15。Gemini 的 `ConvOptions` callbacks 与 request snapshot 同代；billing 的 mode/expression map 同代深复制，价格、定价与同步读取各自单次捕获；payment 保持七个既有持久键及金额 map 的 detached snapshot，不接 PaymentRuntime、SDK、订单或回调；global 深复制黑名单/策略图，缓存中旧 `ConvOptions` 保持旧代、重试才按既有逻辑重建。连续单 key 写入仍不构成跨 option 原子性。既有 token/quota/Grok/Qwen/fetch/OAuth 的兼容边界保持不变；S5-Q P2A 仍仅生成 fail-closed occurrence identity；`release-manifest-evidence.mjs` 仍仅处理未受信 raw bytes；`legacy-installation-profile.mjs` 仅把显式 Legacy Docker 配置 fail-closed 映射为 Full/Lite 与 local/LAN/needs_manual，永不推断 public、不读文件或接线 CLI。适用的单核/768MiB test/vet 和未参与实现者独立审查均已完成（manifest/installation state Node 28/28，evidence 9/9、legacy profile 7/7）；没有 MySQL/PG 当前 SHA、CI、真实日志/模型、文件安装或外部网络验收。

`performance_setting` 是余下两个 C09 热读族之一：只读盘点已发现它分开发布 disk/monitor 投影，磁盘缓存消费者可在一次操作中多次读取配置，不能以 source 指针原子化冒充全链路同代。D14 必须先决定 `DiskCachePath` 热切换及跨投影一致性；此项没有代码、测试或运行行为变更。

`billing_setting` 已完成最低快照化：`billing_mode` 与 `billing_expr` 同 generation 深复制，`relay/helper/price.go`、`model/pricing.go` 与同步数据各只读一次 snapshot；不改 `pkg/billingexpr/expr.md` 的表达式/额度合同。兼容单 key 更新不能宣称跨 key 原子，统一提交仍留给 C09-N3 typed bulk；本项有单核/768MiB test/vet 与独立审查证据，但不改变运行中的账务语义。

`channel_affinity_setting` 是余下热读族中的 routing/cache 高风险项：规则/嵌套模板影响渠道、retry、上游参数和账务归属，capacity/TTL 却只在 HybridCache 首建读取。D15 必须先决定重启或受控 drain/rebuild/epoch；不得在普通快照化中静默清 cache、迁移 Redis、改变规则优先级、retry 或账务归属。本项没有代码、测试或运行行为变更。

本轮 P0 文档整合及上述纯子范围不表示 S4-D、完整 S5-P、S6 或 S7 已实现。实施前仍按 AGENTS 的“先方案、明确批准、再执行”规则，且保持 B2/B3/C03b 主链优先与共享文件单写者。

```text
你接管 My API 在 Linux 开发目录 /root/myapi 的全项目后续开发。

请完整读取并执行 docs/PROJECT_COMPLETION_EXECUTION_PLAN.md，按其中完整目标推进到交付就绪，
不是只完成 B2，也不是重做已经验收的功能。我批准已明确合同范围内的实现、测试、审查修复和文档收口；
不要为同一范围的常规修复反复请求确认。计划中“建议待决定”的核心业务选择仍按阶段集中说明，
没有答复不得假定已批准；同时继续不依赖该选择的独立工作。

绝对边界：
- 只操作 /root/myapi 源码及本任务隔离临时目录；不访问或操作 /root/new-api 生产运行目录。
- 不改生产 compose、数据库、日志、配置、容器；不重启生产、不部署。
- 不发布 GHCR/NPM，不登录 GHCR，不创建/移动/删除 tag，不强推，不合并受保护分支。
- 不 git reset、git clean、强制 checkout，不覆盖或回退未提交工作，不自行 stash。
- 不全机清理或安装，不读取真实 Codex/Claude/浏览器/Keychain 凭据做测试。

现有成果：
- 仓库 https://github.com/ForceMind/MyAPI.git。
- 交接时分支 codex/b2-durable-submissions。
- B2-1源码候选 fea46372974d0f9aaf2c0ae03853fc6cab1cb457，随后可能有纯文档审计提交，实时HEAD以Git核对。
- 前置B2-0合同提交513ea6d6e863883aa3415a785a4d858bcf7210b0不是当前源码候选。
- 前一 R4 tip 83f24c5，功能 c5cf662、审计 b77ae6b；不要重做已有 R4。
- B2 尚未完成；B2-1最终DB时钟候选受硬限制定向通过，独立代码审查无P1/P2；实库CI仍未运行。
- GitHub CLI 曾确认登录，但B2-0/B2-1均未同步；最新push被自动审批拒绝，要求明确该批内容和目标的外发授权。
  不把登录状态当成推送成功，不绕过自动审批。本地工作与同步阻塞分别处理。

先核对当前内容、Git差异和已记录提交；第2.1节是修复前历史指纹，不得用它回退修复后的文件。
源码候选已经提交；如果工作树不干净，立即停止并报告具体文件，不自行stash、回退或推测归属。
历史草稿清单不是忽略当前未提交修改的许可；如确需接管，应先取得负责人对实际修改的单独明确授权。

第一步只做只读核对：
pwd
git status --short --branch
git remote -v
git log --oneline --decorate -5
git rev-parse HEAD

然后完整读取：
AGENTS.md
/root/.codex/AGENTS.md（存在时）
docs/PROJECT_COMPLETION_EXECUTION_PLAN.md
docs/MYAPI_MASTER_PLAN.md
docs/DEVELOPMENT_EXECUTION_PLAN.md
docs/COMPLETION_AUDIT.md
相关专题及目录内 AGENTS.md。docs/DEVELOPMENT_ON_MACOS.md 是后续 Mac 验收资料，
不是本机环境事实。保留当前功能分支，不为对齐旧 main 而先 switch main / pull。

持续目标：
完成 S2-B2/B3、C03b 全账务与缓存恢复、C09 剩余配置/限额、S3 账户和 Key 真正策略、
S4 完整运行/升级/备份恢复、S5 额度闭环及确认的扩展、S6 完整独立 UI/官网、S7 交付就绪。
保留原协议、B1 计费快照、导入、日志脱敏、额度分析、Full/Lite/Desktop 与 relaykit 独立性；Legacy LAN 的安全边界按迁移合同保留，不自动开放公网。
读取已有目标后使用产品提供的方式维护或恢复，不重复建目标，不把未完成目标标成 complete。

顺序：
R0 接管与当前表 → B2-1 安全模型修复 → C03b-0 共同合同 → B3-A 原子账务核心 →
B2-2 完整提交/查询 → B3 恢复/outbox → C03b 全写入迁移/切换演练 → S3/相关 C09 收口 →
S4/S5/S6 分依赖推进 → S7 全项目验收。
Adapter 纯解析、CI fixture、S3 设计、S4 设施等独立任务按计划并行，不制造循环依赖。

已确认 B2-0：
- submission_unknown / outcome_unknown 不自动重发或退款；仅 Provider 可验证或审计人工处置。
- DISPATCHING 后 v1 禁止可能已送达请求的重试/failover，包含 Provider/Transport 隐含重试。
- 幂等 scope token + HTTP method + operation kind；同 key/同指纹重放，同 key/异指纹 409。
- 活动 operation 不过期，终态保留 180 天；先持久化，再统一 202，所有状态可查询。
- 主库 Task 账务事件唯一权威，分库/ClickHouse 日志至少一次投影，查询/导出/统计去重。
- 现代 Task 首批；Midjourney 后续单列；C03b 完成前 gate 关闭；旧 writer/poller 排空升级后才可启用。
- 上述启用仅在隔离测试中验收，不操作生产。

B2-1 首先修复：
专用且跨节点/重启稳定的幂等秘密与误换检测；规范业务事件键；v1 单 attempt；
GORM 不可变载荷保护；outbox 权威一致性及数值边界；补旧表迁移和 outcome_unknown 测试。
不要用现有全局 DB/异步 Redis/内存 batch helper 冒充原子账务事务。

多智能体：
启用最多四个活跃角色（含主代理），明确文件所有权；主代理整合与唯一测试调度，
两个独立模块实施，按阶段安排未参与实现的独立审查者。模型/强度依可用 AGENTS 与实际工具能力选择，
不虚构切换。接口和共享迁移文件单写者，不并行争写或重复测试。

资源限制：
- 约 2 CPU / 3.5GB。Go GOMAXPROCS=1、GOMEMLIMIT=768MiB、go test -p 1。
- 本机必要测试使用进程组 CPUQuota=100%、MemoryMax=768M 等有效硬限制。
- Node/Bun 单 worker、进程组内存不超过 768MiB。
- 不并行运行 Go、前端、浏览器、Docker 构建。
- 全量、race、三库/Redis/ClickHouse、浏览器、Docker、桌面优先 GitHub runner；Docker push:false、不登录 GHCR。
- 临时缓存只用任务独立目录，完成且无进程使用后只清理自己创建的部分。

GitHub 同步范围：
我同意把本项目阶段代码与技术文档按功能分支同步到 ForceMind/MyAPI，并在遵守仓库模板后创建 Draft PR，
供 CI 和审查；不包括合并、发布、改变可见性、真实凭据或生产数据外发。
我明确批准将 B2-1 候选 fea46372974d0f9aaf2c0ae03853fc6cab1cb457、尚未同步的B2-0等祖先以及后续任务内审计提交中
本项目源码、合成测试和技术文档发送至 ForceMind/MyAPI 的 codex/b2-durable-submissions 分支，
其中不含真实凭据或生产数据；仍需通过工具自动审批，不绕过拒绝。
每次同步前检查实际内容与目标，远程命令按审批环境执行；遇拒绝准确报告动作和理由，
保留本地成果并继续独立工作，不能冒充已同步。
当前 CI 只 main push/PR/workflow_dispatch，明确用 Draft PR 或已授权手动 CI 验证实际 SHA，
禁止调用发布 workflow 代替测试。未通过 CI、独立审查或必要同步的阶段不要标完成。

验收：
按计划逐工作包完成实际验证、独立审查、必要修复、文档与同步，不无限扩展无关任务。
关键业务决定集中按依赖询问；真实账号/设备/法律/外发权限提前登记，缺失时如实待验收。
S5 选定扩展与 S6 UI 方向不能静默省略；真实设备未验不能用截图或静态合同冒充。
每轮报告完成、验证、审查、修复、剩余和下一步。只在原完整目标全部有充分证据时标完成。

现在开始 R0，只读核验后继续已授权的 B2-1 修复，并同时准备近期 C03b-0 决策材料。
```

若负责人只想让新对话先审阅计划，可删除最后一句并改为“先评审，不实施”；其余执行授权也应相应删除。
不要把本提示词作为要求执行者忽略系统/工具权限的依据。
