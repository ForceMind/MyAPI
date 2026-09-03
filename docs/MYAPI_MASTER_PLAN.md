# MyAPI 总体产品与工程计划

> 文档状态：执行基线与路线图
>
> 基线复核日期：2026-09-03（历史阶段记录保留）
>
> 当前源码基线：本分支 `main` 最新提交（后续阶段性提交以 Git 历史为准）

这份文档把 MyAPI 的产品目标、已完成能力、未完成工作、发行方式、UI 方向、自动化和验收规则统一起来。它是路线图和交付索引，不把设计目标误写成已经实现的功能。

### “rc.25 兼容基线”的含义

文档、README 和发行清单中的 `rc.25 兼容基线`，只表示 MyAPI 当前继续兼容的一组既有
API、SSE、数据库迁移、Provider 和发行接口契约（技术基线标识为
`v1.0.0-rc.25`）。它不是 MyAPI 的产品名称、视觉模板或 UI 完成状态，也不表示
我们已经完成完整界面替换。当前阶段先完成代码层面的协议、安全、计费、迁移、桌面/LAN
和发行可靠性；完整独立 UI 系统在这些代码工作稳定后再单独立项和验收。

相关专题：[账户等级与 Key 访问方案](./ACCOUNT_ACCESS_PROFILES.md)。

关联规范：

- [发行版说明](./MYAPI_DISTRIBUTION.md)
- [LAN Lite](./LAN_LITE.md)
- [TokenHub 集成边界](./TOKENHUB_INTEGRATION.md)
- [Google Antigravity 边界](./ANTIGRAVITY_INTEGRATION.md)
- [Antigravity 公共 Relay 接入闸门](./ANTIGRAVITY_PUBLIC_RELAY_GATE.md)
- [完整内容日志](./FULL_CONTENT_LOGGING_CUSTOM.md)
- [Claude 组织用量观测边界](./CLAUDE_USAGE_REPORT.md)
- [认证和 Cookie 安全](./authentication.md)
- [实机与副本验收清单](./REAL_DEVICE_ACCEPTANCE.md)
- [开发执行计划与缺陷矩阵](./DEVELOPMENT_EXECUTION_PLAN.md)
- [macOS 开发迁移指南](./DEVELOPMENT_ON_MACOS.md)
- [新设备 Codex 交接提示词](./CODEX_HANDOFF_PROMPT.md)
- [定制说明](../CUSTOMIZATION.md)
- [部署说明](../DEPLOYMENT_CUSTOM.md)

## 1. 产品目标

MyAPI 是独立的 AI API 网关发行版和运行时品牌，面向三类使用场景：

1. Full：服务器或团队使用的完整管理版。
2. LAN Lite：macOS/Windows 工作站上的局域网极简版，同事使用自己的 MyAPI API Key 调用。
3. Desktop/LAN：围绕 LAN Lite 的桌面化安装、升级和状态体验。

核心原则：

- MyAPI 是发行版品牌和运行时品牌。
- 上游凭据只在 MyAPI 服务端配置；LAN 客户端不读取本机 Codex、Claude 或其他凭据文件。
- 局域网版默认本机回环监听，扩大到局域网必须显式确认。
- Full、LAN Lite、桌面版共享可靠的请求转发、日志、权限和安全边界，但不强行共享不适用的功能。
- TokenHub 和原 New API 都只是参考样本：可以研究其架构取舍、用户流程、信息层级和产品表达，但不得复制其代码、页面结构、视觉资产、文案、品牌、链接、容器/环境约定或内部协议。
- MyAPI 的领域模型、接口契约、UI 信息架构、视觉系统、静态官网和运行时行为必须独立设计与实现；“借鉴”只表示吸收可验证的设计思路，不表示逐项仿制。
- 许可证和法定通知另行进行合规审查，不把参考项目的署名或产品归属混入 MyAPI 的品牌展示。

## 2. 不可违反的边界

### 安全和数据

- 不在日志、测试输出、文档或提交中暴露 API Key、JWT、Cookie、OAuth JSON、SESSION_SECRET 或上游完整敏感响应。
- 生产数据库、日志、`.env`、`node_modules`、`dist` 不加入 Git。
- 完整内容日志必须继续脱敏，并受文件大小、文件数量和内存上限约束。
- 额度历史只保存规范化数值和状态，不保存完整余额响应。
- 生产重建、重启、发布、推送和删除操作必须有明确授权，并在执行前说明目标和影响。

### 品牌、归属和许可证

- 产品名称、About 页面、默认页脚、Logo、网站和运行时展示统一使用 MyAPI。
- 必须专项清理旧产品名称、旧链接、容器名、数据路径、环境变量和内部协议中的非兼容遗留项。
- 当前仓库仍可静态检索到大量旧 `New API`、`QuantumNous` 和旧元数据引用，因此品牌清理不能标记为已完成。
- About 默认空态已改为 MyAPI 归属，并将必要的第三方通知集中链接到发行版 `NOTICE`；NOTICE 原文和源码法律头部仍需法务确认后再决定是否调整，不能用全局替换破坏许可证义务。
- Web 默认 Logo、favicon 与 Electron 打包图标已统一为 MyAPI 资产；法律 NOTICE、源码头部和兼容 fallback 仍需合规审查。
- 六种 README 已移除旧项目名称、链接和“基于原项目”的公开产品表述，改为既有数据/API 契约兼容说明；源码法律头、NOTICE、wire header、数据库/缓存/env 兼容标识保留在迁移矩阵内。
- 许可证和法定版权通知必须进行合规审查后处理，不能通过删除必要法律信息来伪造独立归属。

## 3. 当前已完成并验证的能力

### API 兼容

- Chat Completions 转 Responses。
- Codex SSE、流式和非流式响应兼容。
- 自动移除 Codex 不支持的参数。
- Chat 附件支持 `file_id`、`file_data`、`filename`/`file_name`，转换为 Responses `input_file`。

### 日志

- 请求正文、响应正文和响应分片记录。
- Authorization、Cookie、API Key、JWT、密码等脱敏。
- 按时间、Key、模型、Request ID 查询。
- 在线查看、纯文本视图、原始 JSON/SSE 视图切换。
- 移动端 API 日志卡片和详情操作已修复。
- 响应/SSE 日志内存边界已加固。
- 概览中的账户额度变化面板同时展示每分钟变化和 Codex 账户趋势图；支持 24h/7d/30d/90d 时间范围、auto/raw/hour/day/week 颗粒度、可用/已用/总量指标，以及折线/面积/柱状显示模式。顶部变化卡片在额度稳定时显示 0，并明确说明没有观察到增减，避免把稳定数据误认为缺失；多组 Codex 渠道/窗口可选择查看。没有窗口身份的旧 Codex 传输失败记录在出现更新成功采样后不再作为第二个实时账户展示，但仍保留在历史数据质量统计中。
- 本机测试副本已在明确授权下切换到当前构建的 `local/new-api:myapi-quota-ui-20260902` 并通过健康检查；SQLite 迁移和旧镜像回滚也已在临时副本完成。其他生产环境仍需由部署方按 `docs/UPGRADE_REHEARSAL.md` 完成副本验证并明确批准后再操作。

### 统计和品牌

- 用量分布支持分钟、小时、天、周，以及时区偏移和边界处理。
- Codex 网页 OAuth 登录入口已存在。
- 默认推广内容已精简。
- MyAPI 品牌运行时参数：`VITE_BRAND_NAME`、`VITE_BRAND_LOGO`、`MYAPI_BRAND_NAME`、`MYAPI_BRAND_LOGO`。
- Full/LAN 构建参数和 Logo 配置已接入 Docker 与部署脚本。
- API Key 已开始返回兼容旧 `group` 的访问方案元数据，创建和列表 UI 已显示“Access profile”及用途说明；`GET /api/user/self/groups` 另返回独立的 `account_tier` 元数据，Key 创建时会同时解释“账户等级”和“访问方案”的边界。
- 设置引导已按用户和版本隔离；完成后自动移除引导卡片，不再显示“设置引导已完成”或重复打开入口。
- 渠道余额对话框已接入额度历史折线图，支持 24h/7d/30d/90d、自定义日期范围、自动/raw/hour/day/week 聚合、浏览器时区偏移、加载/失败/空数据、多 Key 解释和失败采样断点；手动刷新会使趋势查询失效并重新读取。快照可通过 `CHANNEL_QUOTA_SNAPSHOT_RETENTION_DAYS` 启用每日限批清理。
- Codex OAuth 渠道的 Account Info 对话框已增加“当前窗口 / 历史趋势”切换；历史只保存官方 WHAM usage 响应中规范化的 primary/secondary 使用百分比、窗口和重置时间，不保存原始响应或凭据。后台有界采样任务默认开启，会按渠道锁调用同一官方 usage 接口并记录成功/失败状态；管理员可在「系统设置 → 运维 → 监控与告警」中调整，部署环境显式设置 `CHANNEL_QUOTA_SYNC_ENABLED=false` 时强制关闭。
- 概览页和管理员渠道页已增加“账户额度变化”面板，共用 `GET /api/channel/quota/changes` 聚合接口；按渠道/上游账户、指标和窗口分组，以相邻有效快照计算带符号的每分钟变化，并默认按绝对变化最大值排序。概览趋势图支持时间范围、颗粒度、指标和折线/面积/柱状模式；渠道页在筛选列表下提供可选账户系列的详细历史图表、摘要卡和同样的图表控制。`GET /api/channel/quota/status` 只读返回采样开关、间隔和每轮上限，帮助空态解释部署配置。跨重置边界、失败或不足样本不会伪造变化值；面板只显示脱敏后的渠道名称和额度数值，不包含凭据或原始响应，移动端使用纵向卡片布局。
- 额度变化聚合项同时返回与渠道历史一致的只读 `alert` 状态（`disabled`、`unavailable`、`healthy`、`warning`、`critical`）和阈值元数据；失败、无总量或不支持的渠道不会继承旧状态，也不会触发通知、停用渠道或改变路由。
- 额度概览和渠道详情的查询会使用正常认证刷新流程（不再跳过 `401` 后的 session refresh）；TanStack Query key 同时包含用户 ID、session SID 和能力状态，避免同一标签页切换登录身份后短暂复用上一位管理员的额度或采样状态。`a2528a2` 增加了跨登录身份回归测试；它不改变后端权限边界，也不缓存凭据。
- 额度采样状态区分 `success`、`error`、`unsupported` 和 `unavailable`：没有官方余额端点的 Claude/Azure 等渠道记录为 `unsupported`，不会被误报为可修复的网络故障；管理员渠道面板可独立筛选该状态。
- 当前“账户”展示语义是脱敏后的渠道/指标序列：`Accounts tracked` 统计分组序列数量，不保证等于真实上游订阅账户数；多 Key 渠道在 MVP 中不合并或拆分各 Key 的余额。后续若要区分同一渠道的多个订阅账户，必须先取得 provider 返回的稳定非敏感账户标识（或由管理员显式配置别名），再扩展快照主键、聚合 API、权限和迁移，不得使用 API Key 原文或未经验证的响应字段。
- 渠道类型选择已将 Codex 置首，并在 Codex、Claude、Gemini/Antigravity 入口显示实际能力边界。
- Claude adaptor 的边界防护已在 `1827358` 固化：缺失 `RelayInfo` 或空 base URL 会被拒绝，尾部斜杠会规范化，出站请求默认补齐 JSON `Content-Type` 与 `anthropic-version`；这些保护只影响请求构造，不改变 Claude Messages/Responses 兼容范围，也不提供账户余额读取。

### TokenHub、Antigravity 和发行基础

- TokenHub 有独立边界文档和 provider-neutral 适配边界；后续 UI 和官网只借鉴其产品叙事与信息组织，不复制实现或页面。
- Google Antigravity 已按官方 Gemini Interactions API 建立独立的非持久化客户端边界（创建、有限轮询、取消、删除和 usage 提取）。`1827358` 进一步固定 dynamic agent 类型、仅通过 `environment` 承载 continuation 的环境标识、限制 interaction ID/输入/响应大小、将 `requires_action` 视为轮询终态并在错误中去除上游响应正文；但尚未接入普通 relay/channel 或账户额度，不应宣传为完整 Antigravity 账户或额度支持。
- LAN Lite CLI、SQLite-first 项目初始化和局域网安全边界已存在。
- Electron 桌面版默认回环监听、单实例和持久会话密钥已加固；`--allow-lan` 加私网绑定地址才可共享，并在托盘菜单显示生效端点。
- Electron 生产后端就绪探针请求 `/api/status`，要求 HTTP 2xx 且 JSON `success=true`；开发前端探针仍使用 `/` 并只校验 HTTP 状态。该行为由 runtime-config、探针合同测试和 desktop check 固化，尚未替代真实 macOS/Windows 安装与局域网演练。
- 新开发环境的 PostgreSQL 默认数据库标识已统一为 `myapi`（`docker-compose.dev.yml`、`makefile`）；接管旧数据必须显式设置 `MYAPI_DEV_POSTGRES_DB` 或 `DEV_POSTGRES_DB`，不自动重命名或迁移既有数据库。
- `deploy/install.sh` 与 CLI 使用相同的回环/RFC1918 绑定边界；安装脚本拒绝格式错误或公网地址，非回环监听必须显式设置 `MYAPI_ALLOW_LAN=true`，并以非执行方式读取 `.env`。
- GitHub Actions 已支持 SemVer tag 构建并推送 Full/LAN GHCR 镜像；多架构 manifest 使用构建任务产出的、经过格式和仓库校验的架构 digest 组装，不再以可变架构 tag 作为 manifest 输入。
- `myapi upgrade` CLI 已支持按发行版拉取 GHCR 镜像、可选 cosign 签名校验、可选拉取后 digest 固定、环境文件备份、健康等待和失败回滚；生产启用仍需人工审批与数据备份演练。
- SemVer tag 可触发 macOS/Windows Electron 构建产物并生成 SHA256 校验和；发布上传需显式开启。
- 静态官网已补齐移动菜单关闭、outside-click、Escape、焦点回归与 Tab 约束、主题偏好持久化等基础交互，并加入资源/结构自动校验与 390px/320px Chromium 移动 smoke workflow；完成度记录中的最近一次成功 Chromium smoke run 为 `33334940499`，真实设备/移动视觉审查与独立发布 workflow 仍待完成。
- 管理员「系统信息」新增只读 Runtime build 标识，可复制构建 revision；Docker、Release 和 Electron 构建会注入 commit SHA，避免同一版本不同提交显示相同标识；本机测试副本已完成更新并核对健康状态，真实管理员手机现场核对仍待完成。

## 4. 当前未完成或仅有边界设计的工作

### 当前执行批次目标（代码优先）

本批次先完成不依赖完整 UI 重做、生产凭据或正式发布权限的工作，验收目标如下：

1. **代码合同收敛**：后端、计费、Provider、迁移、LAN、Electron 和发行脚本无已知阻断；
   quota 计算必须有饱和保护，relaykit 保持独立构建。
2. **本机验证**：Node 22、Go、Bun 环境下通过相关 typecheck、测试、构建和合同检查；
   Docker/数据库/真实设备结果必须单独记录，不能由静态检查替代。
3. **阶段同步**：每个阶段更新本计划、完成度审计和迁移文档，形成提交并推送 GitHub，
   记录对应 CI；不创建或移动受保护 tag，不执行 GHCR/NPM/生产操作。
4. **外部验收准备**：为 Docker Compose、SQLite/MySQL/PostgreSQL 副本、手机、
   macOS/Windows、局域网和防火墙演练定义证据、回滚和阻塞条件。
5. **UI 后置闸门**：只有代码合同稳定、外部运行边界明确后，才启动完整独立 UI 的
   信息架构、视觉和交互替换；此前的 Logo、文案和局部面板不计为 UI 全量替换。

#### 2026-09-04 当前代码验收基线

`7f1913e` / [CI 33781560507](https://github.com/ForceMind/MyAPI/actions/runs/33781560507)
七项成功；[Docker smoke 33781637372](https://github.com/ForceMind/MyAPI/actions/runs/33781637372)
Full/LAN 两项均成功。**B1、C06 与 S4-01 已完成当前范围**：
任务提交费率冻结及异常快照守卫、日志并发/轮转状态修复、无发布镜像新安装/认证/真实登录表单与精确构建标识。
MySQL5.7/PG9.6 各五种快照/NULL 往返与原七支付场景实跑通过，原始日志已核对。
本机整合全量/vet/build/race、relaykit 独立验证、336 项前端测试及生产构建、13 项探针
测试、2190 文件源码发行包通过。封版追加 race 发现 Kling 测试清理与后台缓存回调
竞争；测试隔离及 CI race 接线已修。`6fd8ae4` / CI `33783792231` 七项成功，新增 backend
race 原始日志确认四包通过，本批封版闭环，不改变生产缓存恢复策略。

保留 `540cf32` 首轮镜像 BuildMismatch、`e7fffc2` PostgreSQL JSON 写入失败等历史证据。
两处 JSON Valuer 的文本绑定修复和探针严格成功判定均由最终提交重新实跑，不跳过数据库
或放宽 SHA/认证检查。详细命令、image ID 和红绿过程见[完成度审计](COMPLETION_AUDIT.md#s2-b1-与-s4-01)。

**S4-02 已完成当前范围。** 首个 `b54ce36` Full job 在业务前暴露 sidecar
loopback/NAT reset，未验收；修复保持宿主回环发布，仅令受限 CI sidecar 显式监听 namespace
全接口。最终 `237c0da` 的本机完整 `release:check`、独立复审与七项常规 CI 通过；
[Docker 33790513455](https://github.com/ForceMind/MyAPI/actions/runs/33790513455) 在同一 SHA
上 Full/LAN 均通过。usage 10+5 精确形成 15 quota，普通用户/受限 Key/渠道账本、唯一关联
日志、普通/管理员权限、请求/响应头脱敏、匿名不上游和真实登录表单均由实际临时镜像验证。
它不调用真实 Provider，也不证明 Redis/batch、MySQL/PostgreSQL 完整运行恢复或真实设备。
下一步扩展三库运行合同；完整备份恢复、真实账户/设备、独立 UI 仍未完成。
B2/B3 的提交未知状态、持久账务事件和 outbox，以及 C03b 缓存恢复策略仍需核心决定；
复用既有 CAS/租约/快照，不重建平行系统。见[完整执行矩阵](DEVELOPMENT_EXECUTION_PLAN.md)。

以下为此前阶段记录，当前状态以以上验收基线与执行矩阵为准。

S2-A 已确认并在 `c82d0f1` / CI `33744326429` 通过的基线上开始。当前仅支付/订阅六项
事务、退款、回调 ACK、状态保护与仅订阅配置合同，具体状态见[执行矩阵](DEVELOPMENT_EXECUTION_PLAN.md#s2-a-支付与订阅事务)。
不运行真实付款、不发布、不操作生产；既定目标和写入边界内持续实现、复验、同步，
仅新增架构/业务取舍、范围或外部权限需要另行决定。

S2-A 代码 `2777021` 已推送，本机完整回归/构建/race/发行合同与独立静态复审通过，
CI `33749764180` 六项 success，但当时原始实库日志发现 MySQL 中文日志插入失败及缺失
断言，未予验收。追加 S2-A-R1 由 `767b17b` / CI `33751536908` 完成：仅配置已确认空的
CI 专库字符集并补日志断言，两种实库各七场景实跑且无旧错误。A01–A06 及 R1 已完成当前
确认范围；S2-B/C 及 S3–S7 仍未完成，不调整生产付款语义或生产数据，详见[审计证据](COMPLETION_AUDIT.md)。

此前 R1 基线为 `6fabc98`，CI `33740901999` 五项成功。用户已确认执行 S1-R1：限定同进程
配置保存与后台重载的读取/提交到内存发布顺序，并验证双写、重载交错和失败释放；不
保证跨实例或全部配置读取的全局原子性。原六项已完成；R1 已由 `8dfcfba` 交付并通过
CI `33743669737` 五项验证（含专项 race）及独立复审，S1 已完成当前确认范围。

以下为此前阶段证据，精确状态以[开发执行计划](DEVELOPMENT_EXECUTION_PLAN.md)为准。

源码基线 `a36e529` 的 CI `33721694305` 四个 job 已以真实 steps 成功，包含额度浏览器
回归和截图；网站检查 `33721694330` 与制品 `33721694316` 成功。历史 Billing/runner
故障不再是当前阻塞。S0 计划/证据和 S1 六项安全/一致性修复已获明确确认，执行状态见
[集中清单](DEVELOPMENT_EXECUTION_PLAN.md)；尚未授权启用新的路由策略或扩展 Provider。

S0 与 S1 原六项已完成本批实现、回归及独立复审：`dc94e81` 的 CI `33740321133` 五个
job 成功，新增本次身份迁移/配置事务的 MySQL5.7、PostgreSQL9.6 实跑；本机 SQLite
恢复/重复启动与专项 race 通过。完整应用三库恢复和 S2-S7 仍未完成，
不得从该批通过推导整个项目已验收。

本机 Node 22.23.2 可用，Go/Bun 当前安装版本不替代最低/固定基线验收；Docker CLI/App
尚不可用。真实数据库、设备、账户与生产证据单列；完整 UI 在核心合同稳定、测试实例
可运行且外部验收条件明确后启动，不要求先正式发布或先完成全部外部设备验收。

#### 2026-09-01 历史阶段状态

Palm、Zhipu、Ali rerank、MiniMax 的 JSON wrapper 和错误边界已完成并通过定向及全量
Go 回归，relaykit 独立构建也已通过。Node 22 路径下前端类型检查、280 个 Vitest 测试、
production build 与发行合同均已复核通过；默认 Homebrew Node 26 的 simdutf 链接问题仍
在该次检查中未修复。Docker、跨数据库副本和真实设备当时仍待验证。完整 UI 替换没有开始，
仍按代码合同稳定后的后置闸门执行。

第二轮 Provider 审计已覆盖 Xunfei 与 Volcengine，并通过定向回归；继续以小范围 wrapper
收敛和错误边界为主，不整体重写稳定适配器。

第三轮 Provider 审计已覆盖 AWS、Jimeng 与 MokaAI，并通过定向回归；继续保持协议边界和
relaykit 独立性，不把 UI 替换提前到代码合同稳定之前。

| 领域 | 当前状态 | 完成定义 |
| --- | --- | --- |
| 品牌和旧元数据清理 | 审计清单已建立 | About 默认态、PNG/ICO 资产已切换；`docs/BRAND_AUDIT.md` 区分必须替换、兼容保留和法律保留项，NOTICE/源码头部仍需合规审查 |
| 独立 UI 系统 | 尚未开始（代码层先行） | 不依赖旧 New API 信息架构，Full/LAN/移动端完成真实画面审查；当前已有的 Logo、品牌文案和局部功能面板不计为完整 UI 替换 |
| TokenHub 风格静态官网 | Chromium smoke 与审阅制品已实现 | 形成独立产品叙事、安装入口、发行版选择、安全说明、响应式菜单无障碍/主题交互、静态资源自动校验、390px/320px Chromium 移动 smoke（含窄屏水平溢出断言）和 main 变更自动生成的确定性 artifact workflow；真实移动视觉审查与绑定域名的独立发布仍待完成 |
| 渠道额度历史 | 后端和前端初版已实现 | 普通渠道与 Codex OAuth 渠道均已有历史查询、折线图、失败状态、可选保留清理和只读健康指标；历史聚合按计划、单位、币种和窗口系列隔离，未指定系列时锁定最新系列；普通渠道与 Codex OAuth 均支持可选、有界后台采样；告警阈值和 notifier-neutral 去重策略已支持默认关闭、原子持久化和只读状态展示，外部通知通道仍待业务决策 |
| 账户额度变化聚合 | 初版已实现 | 概览和管理员渠道页均可查看每分钟变化及最大变化排序；概览页在前台每 60 秒自动刷新，并显示 provider plan type 与错误采样状态；普通渠道与 Codex OAuth 后台采样已接入系统任务并避免与旧轮询重复，跨账户订阅账单同步和通知仍待后续迭代 |
| 账户等级/Key 访问方案 | 独立策略注册表已实现 | 管理员可在计费设置的“Key access profile policies”编辑稳定 profile ID 的显示名、说明、路由组、模型白名单、回退方案和启用状态；Key 表单会显式提交 `access_profile_id` 并同时保留 legacy `group`，显式 `account_tier_id`/`access_profile_id` 会持久化，旧客户端省略时按现有记录或变更后的 `group` 兼容回退，旧路由保持兼容。路由/模型强制执行仍需单独迁移评审 |
| 设置引导生命周期 | 初版已实现 | 完成后自动消失、按用户和版本保存；真实多设备视觉审查仍待完成 |
| Claude 支持 | Messages 原生转发与 Responses→Messages 兼容转换已实现；官方组织用量报告已确认存在 | 只实现有明确官方协议的能力；普通 Claude 渠道仍不读取账户余额，组织 Usage Report 只有在管理员显式配置受保护的 Admin 凭据并完成权限/保留策略后才接入 |
| Google Antigravity 专用 relay | 第一阶段 transport 代码、边界测试和有界 Docker Go 回归已交付 | `AntigravityClient` 已覆盖官方 preview 的创建、状态读取、有限轮询、取消、删除和 usage 提取；`1827358` 增加 dynamic agent/continuation 字段约束、请求/响应大小上限、`requires_action` 终态、nil context 兜底和错误正文脱敏测试；`GOWORK=off go test ./relay/channel/gemini ./relay/channel/claude` 已通过。公开 relay/channel 接入按 [公共 Relay 闸门](./ANTIGRAVITY_PUBLIC_RELAY_GATE.md) 进行持久化、权限、计费和工具策略评审，余额端点不存在时显示 `unsupported` |
| LAN Lite 桌面体验 | 安全状态体验与确定性发行合同检查已实现 | Electron 默认回环、单实例、持久会话密钥、显式 `--allow-lan` 私网绑定、安装脚本 `MYAPI_ALLOW_LAN` 安全门、只读 LAN 状态/防火墙提示、请求 `/api/status` 且要求 HTTP 2xx 与 JSON `success=true` 的生产探针、有效地址健康检查和托盘确认后重启切换已补齐；通配监听仅展示发现的 RFC1918 IPv4 候选；`npm run desktop:check` 与 CI 会验证 macOS/Windows 目标、资源、校验和与发布闸门；真实跨平台安装/局域网请求演练和系统防火墙自动配置仍待完成 |
| 新开发环境数据库默认值 | 代码与模板已验证 | `docker-compose.dev.yml`、`makefile` 及多语言 README 的新开发示例默认使用 `myapi`；显式 `MYAPI_DEV_POSTGRES_DB`/`DEV_POSTGRES_DB` 可接管既有数据库，未执行自动迁移或生产改名 |
| GHCR 自动升级 | CLI 预检与执行流程已实现 | 生产端显式拉取、可选签名验证、可选 digest 固定、健康检查、环境备份和失败回滚已有；`upgrade --dry-run --json` 可在副本上无写入预检，SQLite 脱敏副本的新镜像迁移与旧镜像回滚已完成，PostgreSQL/生产数据库恢复和人工审批仍待完成 |
| NPM 正式发布 | 未完成 | 版本、Tag、清单、测试和用户确认齐备后发布 |

## 5. 领域模型重构方向

### 5.1 账户等级与 Key 访问方案分离

当前 `User.Group` 和 `Token.Group` 共用同一批名称，导致 `default`、`vip` 既像账户等级又像路由策略。目标模型如下：

- **账户等级（Account Tier）**：描述用户身份、权益、可用功能、用户额度和可见模型。
- **Key 访问方案（Access Profile）**：描述一个 API Key 使用的路由池、计费倍率、模型范围和故障转移策略。
- **自动路由策略（Auto Routing Policy）**：定义多个访问方案的尝试顺序和跨方案重试。
- **渠道标签（Channel Tag）**：描述渠道路由归属，不作为用户账户名称。

迁移策略：

1. 第一阶段只改 UI、说明和返回的展示元数据，保留旧 `group` 字段兼容（已完成）。
2. 第二阶段增加访问方案显示名、描述、倍率、可选范围和稳定标识（内置方案及管理员注册表配置已完成）。
3. 第三阶段已增加 `users.account_tier_id` 与 `tokens.access_profile_id` 独立字段，并在启动迁移中从旧 `group` 幂等回填；旧 `group` 继续作为兼容回退。当前已增加 `access_profile_setting.profiles` 注册表及计费设置管理编辑器，供管理员配置稳定 profile 的说明和候选约束；路由/模型强制执行需在兼容策略评审后再启用。

创建 Key 的界面不再只显示 `Group`，而显示“Key 访问方案”，每个选项必须展示用途、计费倍率、路由范围和是否可用。

管理员可在「计费设置 → Group Pricing → Key access profile policies」维护注册表。例如：

```json
{
  "standard": {
    "label": "团队标准",
    "description": "共享标准渠道池",
    "route_groups": ["default"],
    "model_allowlist": ["gpt-5"],
    "fallback_profiles": ["priority"],
    "enabled": true
  }
}
```

该注册表先用于 Key 创建/列表的可解释展示；`Token.Group` 和既有分组路由仍是兼容事实来源。启用路由或模型白名单强制前，必须完成迁移评审，避免已有 Key 被静默拒绝。

### 5.2 渠道额度历史

现有 `Channel.Balance` 继续表示当前缓存值；新增 `channel_quota_snapshots` 记录历史采样。历史记录至少包含：

- `channel_id`、`observed_at`。
- 可用、已用、总量和单位/币种。
- `metric_type`：余额、订阅额度或速率限制。
- `window_type`：无窗口、5 小时、日、月或自定义窗口。
- `reset_at`、来源、成功/失败/不支持状态。

采集规则：

- 复用现有手动和自动余额查询流程。
- 后台采样默认开启，默认目标间隔为 1 分钟，使用已有渠道轮询锁并限制并发；管理员可在「系统设置 → 运维 → 监控与告警」中调整开关、间隔和每轮最大渠道数。环境变量覆盖管理员设置；已有显式间隔不会被升级自动覆盖。调度存在最多一个轮询周期的抖动，多渠道限额和上游延迟也会影响单渠道实际间隔。
- 失败不覆盖最后一个有效余额，但记录失败状态。
- 不记录上游完整响应。
- 多密钥渠道在 MVP 中不合并不同 Key 的额度。
- 查询在服务端先计算原始观测区间的消耗与速率，再进行展示聚合；点数限制不得改变消费总量或抹去失败、重置标记。当前修复契约和验收见 [额度消耗分析](QUOTA_ANALYTICS.md)。

建议配置：

```env
# Background sampling is enabled by default; set false only to disable it at deployment level.
CHANNEL_QUOTA_SYNC_ENABLED=true
CHANNEL_QUOTA_SYNC_INTERVAL=1m
CHANNEL_QUOTA_SNAPSHOT_RETENTION_DAYS=180
# Optional read-only threshold status (disabled by default). This only adds
# `data.alert` to quota history responses when the provider reports a total;
# it never sends notifications, disables channels, or changes routing.
CHANNEL_QUOTA_ALERT_ENABLED=false
CHANNEL_QUOTA_ALERT_WARNING_PERCENT=20
CHANNEL_QUOTA_ALERT_CRITICAL_PERCENT=10
# Policy metadata for a future notifier. No outbound delivery is enabled by
# these values; repeated states are deduplicated by this cooldown.
CHANNEL_QUOTA_ALERT_COOLDOWN_SECONDS=3600
CHANNEL_QUOTA_ALERT_NOTIFY_ON_RECOVERY=false
```

## 6. UI 与交互路线

### 6.1 Key 创建

表单按以下区域组织：

1. 基本信息：名称、有效期、IP 限制。
2. Key 访问方案：显示名称、用途、倍率、路由范围和资格说明。
3. 自动路由设置：仅在选择自动路由时展示。
4. 使用范围：模型、额度和速率限制。

账户等级在用户页面单独展示，并明确说明：账户等级决定“用户拥有什么”，访问方案决定“这个 Key 怎么调用”。

### 6.2 设置引导

设置引导按用户与版本记录完成状态；完成所有步骤后应自动移除引导卡片，不能继续渲染“设置引导已完成”提示，也不重复打开已完成入口。Full 构建的管理员还会看到“配置上游渠道”步骤，该步骤仅在具备 `channel.read` 权限时查询渠道数量；普通用户和 LAN Lite/极简构建不显示管理员步骤。当前实现已覆盖基础生命周期，仍需完成真实多设备视觉审查。

- 已完成：按发行版和角色显示相关步骤；Full 管理员在具备 `channel.read` 时看到渠道配置步骤，普通用户与 LAN Lite 不显示管理员步骤。
- 已完成：整个引导卡片移除，不占首屏空间。
- 需要帮助：从帮助菜单或“重新查看快速开始”主动打开。
- 状态按用户 ID 和引导版本保存，不使用跨用户的单一 localStorage key。
- 步骤使用稳定 ID，并区分未开始、进行中、完成、不适用、错误和权限不足。

Full 版可包含渠道、额度和日志步骤；LAN Lite 只包含创建 Key、选择访问方案和发送测试请求。

### 6.3 额度趋势

渠道详情页增加：

- 当前可用额度。
- 24 小时和 7 天变化。
- 最后采集时间和采集状态。
- 可用额度折线图。
- 失败断点、重置标记和触摸 tooltip。
- 1 小时、6 小时、24 小时、7 天、30 天、90 天和自定义范围。
- 服务端支持 `granularity=raw|minute|5m|15m|hour|day|week|auto` 与 `timezone_offset`（分钟）；前端默认按浏览器时区自动聚合。展示颗粒不改变采样间隔，完整契约见 [额度分析](QUOTA_ANALYTICS.md)。
- 手机端纯文本摘要和无图表降级视图。

渠道上游账户余额、MyAPI 用户余额和单个 API Key 限额必须使用不同标题，不能统称为“可用额度”。

## 7. Provider 支持路线

优先级和边界：

1. 先统一 OpenAI、DeepSeek、OpenRouter、Moonshot、SiliconFlow 等已有余额适配器。
2. Codex 只接入官方可验证的 usage/quota 接口，支持窗口额度时记录 `window_type` 和 `reset_at`。
3. Claude 只实现官方稳定 API 能力；没有公开账户余额接口时显示“不支持额度查询”。
4. Google Antigravity 不通过普通 Gemini 路由伪装实现，不读取本地凭据；只有官方稳定额度接口出现后才增加额度适配。
5. Advanced Custom 使用受限的 JSON 映射，不保存原始敏感响应。

## 8. Full、LAN Lite 和桌面版

### Full

适用于服务器或团队：完整渠道管理、用户管理、日志、额度、订阅和运维功能。

### LAN Lite

适用于可信局域网：

- macOS 和 Windows 优先。
- SQLite-first，单进程，最小配置。
- 默认绑定回环地址。
- `--allow-lan` 才允许非回环监听。
- 不导入本地 Codex/Claude 凭据。
- 同事只获得自己的 MyAPI Key，不获得上游密钥、管理员密码或其他人的日志。

### Desktop

- Electron 负责安装、项目初始化、启动/停止、状态和升级提示。
- 后端仍由 MyAPI 进程或受控容器运行。
- 桌面进程默认以 `MYAPI_EDITION=lan` 和 `MYAPI_BIND_ADDRESS=127.0.0.1` 启动，避免无提示暴露到局域网；开放局域网必须显式传入 `--allow-lan` 和私网绑定地址。托盘提供确认后的重启切换，真正运行中热重绑定仍不启用。
- Electron 使用单实例锁，避免重复进程争用端口和 SQLite 数据库。
- 升级必须先拉取目标镜像、执行健康检查，再切换服务。
- 保留旧镜像和数据目录以支持回滚。

## 9. GitHub Actions、镜像和自动升级

现有 CI 负责构建和推送：

```text
ghcr.io/forcemind/myapi:<version>
ghcr.io/forcemind/myapi-lan:<version>
```

后续标准流程：

1. 提交并审查代码。
2. 升级 `VERSION`、`package.json` 和相关清单。
3. 创建新的 SemVer tag，不移动旧 tag。
4. GitHub Actions 构建并签名 Full/LAN 多架构镜像。
5. 同一 tag 触发 macOS/Windows Electron 构建。
6. 部署端显式选择版本并拉取 GHCR 镜像。
7. 健康检查通过后切换，失败则保留旧版本并回滚。

CI 自动构建不等于自动重启生产服务。生产自动升级需要单独实现明确的拉取、审批、健康检查、备份和回滚策略。

当前 `.github/workflows/ci.yml` 已在 `main` push、Pull Request 和手动触发时运行
后端 vet/build/test、前端 typecheck/test、桌面发行合同以及 CLI/品牌/官网/LAN/打包合同检查；`.github/workflows/docker-build.yml`
仍只在 SemVer tag（或显式手动输入既有 tag）时推送 GHCR，因此普通提交不会意外发布镜像。

## 10. NPM 和版本策略

正式发布前必须：

- 确认目标版本号。
- 同步修改 `VERSION`、`package.json` 和文档。
- 重新生成 `SOURCE_MANIFEST.json`。
- 执行 `npm test`、`npm run pack:check`、`npm run release:state`。
- 检查新 tag 指向当前提交。
- 不强制移动或覆盖已有旧 tag。
- 得到明确确认后再执行 `npm publish`。

当前发布状态需要继续核对旧 `v0.1.0`/`v0.1.1` tag 与当前提交的关系，不能假定旧 tag 可以复用。

## 11. 分阶段路线图

### P0：稳定性和基线

- 保持移动端 API 日志修复。
- Full/LAN 镜像健康检查和 CLI 升级回滚说明已完成；生产环境升级演练仍待完成。
- 完成品牌、旧名称、旧链接、版权头和环境变量专项审计。
- 补齐真实手机浏览器验证；前端 `tsgo -b` 与 Vitest 已有有界回归，测试规模以 [完成审计](COMPLETION_AUDIT.md) 对应提交记录为准，实机视觉不可由自动化替代。额度概览面板会在管理员会话挂载时按 `user.id + session.sid` 最多刷新一次 `/api/user/self`，避免旧权限快照导致误隐藏；真实手机仍需验收。日志查询失败时会清空旧缓存行和正文，避免权限/会话错误仍展示上一结果。

### P1：账户和管理体验

- 账户等级与 Key 访问方案拆分（显式兼容字段、回填迁移和语义测试已完成；独立可配置策略待后续阶段）。
- 访问方案配置和清晰说明（注册表、校验、管理 UI 和 Key 创建说明已完成；fallback 引用目标、空 ID、去空格后的重复 ID 与循环依赖会在保存前拒绝；强制路由策略迁移待评审）。
- 创建 Key 表单重做（初版已完成，仍需完整角色/权限体验审查）。
- 设置引导完成后自动消失（已完成）。
- 引导状态按用户和版本隔离（已完成）。
- 多语言文案和移动端回归（五个额外 locale 已补齐本轮关键 key，并由 parity 测试覆盖；真实设备视觉审查待完成）。

### P2：额度观测

- 渠道额度快照表和迁移（初版已完成；重复采样按渠道、完整系列元数据和观测时间桶幂等，并以 nullable SHA-256 唯一键抵抗并发重复写入，避免重试污染趋势）。
- 手动/自动采集接入（初版已完成，失败状态已记录；新增可选、有界系统任务采样，并与旧轮询互斥）。
- 历史查询 API、基础 summary 和权限（初版已完成；已补聚合/时区参数及 handler 参数拒绝测试）。
- 渠道详情折线图、移动端降级视图和只读额度健康指标（代码初版已完成；概览额度变化支持前台自动刷新、provider plan type、错误状态和只读 warning/critical 告警徽标；真实设备审查与通知策略待完成）。
- 采集失败、重置和不支持状态。
- 额度告警配置（默认关闭；警告/严重阈值、重复通知冷却时间和恢复通知偏好均原子持久化、校验并只读展示；策略评估已实现但不发送外部通知，具体通道和凭据仍待业务决策）。

### P3：Provider 和预警

- Codex 官方 usage/quota 适配。
- Claude 官方接口评估和适配（Messages 原生与 Responses→Messages 兼容转换已完成；组织级 Usage Report 仍需显式 Admin 凭据与业务授权）。
- Antigravity 官方 Interactions API 专用客户端已实现；公开 relay/channel 接入需另行完成计费、权限、工具策略和响应映射评审，不能复用普通 Gemini Generate Content 路径。
- 额度阈值、下降速度和预计耗尽预警。

Codex 用量历史接口（管理员 `ChannelRead` 权限）为：

```text
GET /api/channel/:id/codex/usage/history?range=24h|7d|30d|90d&limit=500
```

响应只包含 `{success, data}` 包装。`data.points` 按采样时间合并
`primary_used_percent`、`secondary_used_percent` 和对应的 `reset_at`；
`data_quality` 会区分成功、失败和不支持的样本。`GET /codex/usage` 会在
完成一次上游查询后追加规范化快照，因此读取该接口具有“采样”副作用；
网络错误、非 2xx 和无法识别的响应只记录状态标记，不覆盖历史有效值。
WHAM `/backend-api` 是 Codex CLI 使用的上游兼容接口，不将其宣传为稳定的
公开账户余额 API；字段变化时前端应显示不可用状态。

官方文档核验（2026-08-31）：Anthropic 已公开
[Get Messages Usage Report](https://platform.claude.com/docs/en/api/admin/usage_report/retrieve_messages)，
路由为 `GET /v1/organizations/usage_report/messages`，支持 `1m`、`1h`、`1d`
时间桶和按 workspace、account、API key、model 等维度分组。该接口返回组织级
token/请求用量，不等于预付费余额或订阅剩余额度；请求还需要组织级 Admin API
凭据，因此不能把普通 Claude 渠道密钥直接用于后台轮询。MyAPI 后续可在明确的
Admin 凭据存储、管理员权限、脱敏、保留周期和数据隔离方案后，增加独立的“Claude
组织用量”观测源，但不得把它混入当前“账户可用额度”折线图。

Anthropic 当前还区分 Claude Enterprise 的 Analytics API（需要 `read:analytics`
权限）以及 Claude Platform on AWS 的不可用端点；MyAPI 必须先识别组织产品形态，
不能用错误的凭据类型重试或把“接口不可用”记录为网络故障。

Google 的 [Antigravity agent 文档](https://ai.google.dev/gemini-api/docs/antigravity-agent)
将 Antigravity 定义为 Gemini Interactions API 上的托管 preview agent；该接口支持
agent 执行和远程环境，但没有 provider-neutral 的账户余额端点。MyAPI 因此继续保留
Claude Messages 转发和 Antigravity 请求边界，不从 Console 限额或 interaction
响应推断账户余额。

### P4：独立产品体验

- 当前阶段策略：先完成后端、协议、安全、计费、迁移、桌面/LAN 和发行代码；完整 UI
  替换暂缓，待代码合同稳定后再建立独立的信息架构、视觉系统和真实设备验收批次。
- 在 TokenHub 和原 New API 的参考研究基础上，完成 MyAPI 独立管理 UI；不复制任一项目的代码、页面、资产、文案或品牌。
- 静态官网和发行版选择页（基础交互、静态资源校验和 main 变更自动 artifact workflow 已完成，真实浏览器/移动视觉审查与绑定域名的发布 workflow 待完成）。
- LAN Lite macOS/Windows 安装、升级、状态和回滚。
- GHCR 版本拉取自动化（CLI 预检、显式拉取、可选签名校验和失败回滚已完成，生产审批/数据备份演练待完成）。

## 12. 测试和验收矩阵

### 后端

- 数据库迁移可重复执行。
- 快照写入成功、失败、重复、重置和币种变化。
- 采集并发、轮询锁和超时。
- 权限隔离和渠道删除/复制行为。
- API 分页、聚合、时区和最大点数。
- 不记录密钥或完整上游响应。

### 前端

- Key 访问方案表单和旧数据兼容。
- 引导未开始、进行中、完成、不适用和错误状态。
- 折线图加载、空数据、失败、触摸和响应式布局。
- Full/LAN/管理员/普通用户差异。
- 中文、英文及其他支持语言。

### 运行和发行

- TypeScript 类型检查、前端测试和生产构建。
- Go 单元测试、格式检查和 API 回归。
- Docker Full/LAN 构建和健康检查。
- macOS/Windows 构建产物验证。
- GHCR 镜像标签、签名和回滚。
- 不触碰生产数据的升级演练。

验收不能用静态代码检查替代真实画面检查，也不能用镜像构建成功替代生产升级验证。

## 13. 交付状态记录规则

每项工作使用以下状态之一：

- `未开始`
- `进行中`
- `待验证`
- `审查未通过`
- `已完成`
- `阻塞/待决策`

标记“已完成”必须同时有：代码或文档交付、适用测试、运行或视觉验证、审查结论和回滚/恢复说明。

每次版本交付记录：

1. 当前完成内容。
2. 实际验证命令和结果。
3. 审查方式及发现。
4. 已修复问题。
5. 剩余任务和风险。
6. 下一步及所需授权。

## 14. 待确认决策

以下事项不能由实现者擅自决定：

- 下一个正式版本是 `0.2.0` 还是其他版本号。
- 访问方案的默认显示名称和商业语义。
- 品牌清理与许可证/法定通知的最终合规处理方式。
- Claude 和 Antigravity 是否存在可依赖的官方额度接口。
- 生产环境是否启用 GHCR 自动拉取，以及是否需要人工审批。
- NPM 正式发布时使用哪个新 tag。

## 15. 实施文件映射

以下是首轮实现时的模块边界。具体文件以实现前的现状检查为准，不因路线图而整体重写已经稳定的模块。

| 能力 | 后端/配置 | 前端 | 文档/测试 |
| --- | --- | --- | --- |
| 额度历史 | `model/`、`controller/channel-billing.go`、`router/channel-router.go`、迁移 | `web/src/features/channels/`、图表组件 | OpenAPI、模型/控制器测试、移动端测试 |
| Key 访问方案 | `model/token.go`、`model/user.go`、`controller/token.go`、设置项 | `web/src/features/keys/`、用户管理和设置页 | 多语言、兼容性和权限测试 |
| 设置引导 | `controller/setup.go`、系统状态接口 | `web/src/features/dashboard/components/overview/overview-dashboard.tsx`、`web/src/features/setup/` | 引导状态、角色、版本迁移测试 |
| 移动端日志 | 日志查询和完整内容日志控制器 | `web/src/features/usage-logs/`、`web/src/features/full-content-logs/` | Vitest、真实移动浏览器检查 |
| Provider 支持 | `relay/channel/codex/`、`relay/channel/claude/`、`relay/channel/gemini/`、额度适配器 | 渠道创建和状态页 | 官方接口 fixture、失败和安全测试 |
| LAN Lite | `cli/myapi.mjs`、`deploy/`、发行配置 | 极简路由和安装状态页 | macOS/Windows、端口和回滚演练 |
| CI/CD | `.github/workflows/`、Dockerfile、版本脚本 | Electron 打包配置 | 镜像标签、签名、产物和发布检查 |
| 品牌/官网 | 构建参数、元数据和部署变量 | `web/src/lib/build-branding.ts`、`website/` | 旧引用扫描、视觉审查、许可证审查 |
