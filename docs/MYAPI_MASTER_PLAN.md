# MyAPI 总体产品与工程计划

> 2026-09-08 最新：额度页首屏布局修正源码 0.1.4 已验证，真实双渠道折线可见，线上仍为 0.1.2、尚未部署。已清理约 11.77 GB Go 构建/旧 Codex 缓存；每批收尾清理要求见 AGENTS.md。详情见[渠道分析执行记录](CHANNEL_ANALYTICS_EXECUTION.md)。

> 2026-09-08 最新交付：渠道分页签与多图额度分析源码 `0.1.3` 已验收，尚未部署或推送；线上仍为 `0.1.2`。功能、验证、历史数据扫描边界及接续状态见[渠道分析执行记录](CHANNEL_ANALYTICS_EXECUTION.md)。恢复时保留两批未提交改动。

> 文档状态：执行基线与路线图
>
> 基线复核日期：2026-09-06（历史阶段记录保留；S5-P 与发行/安装/更新 P0 合同已纳入；仅有未接线的 S5-P P1a/P1b 纯内核、S5-Q P2A occurrence identity、C09-N1 legal/perf/general/console/checkin immutable generation/C09-N5a 只读诊断、B2-2B0/B1a/B1b gate-off 基元、Release Manifest schema-1/未受信 raw-bytes evidence/输入硬化与 D2A 纯安装状态子范围，完整功能尚未实现）
>
> 2026-09-08 状态更正：`codex/b2-durable-submissions` 已合并并删除，当前开发基线为 `main` / `f6536ca`，PR #1 已合并，CI `34186651121` 十项通过。下文日期较早的分支/未同步记录属于历史。当前新增工作见[多渠道分配执行记录](CHANNEL_ROUTING_EXECUTION.md)，源码候选 `0.1.2` 已验收，尚未部署或推送。

> 2026-09-08 部署更新：用户已授权部署，多渠道分配版本 `0.1.2` 已上线并通过健康、HTTPS 版本/资源及数据库检查；智能策略仍关闭。回滚备份和证据见 [多渠道执行记录](CHANNEL_ROUTING_EXECUTION.md)。源码尚未提交或推送。

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
- [发行制品、安装与更新合同](./RELEASE_MANIFEST.md)
- [Lite 与 Legacy LAN 迁移](./LAN_LITE.md)
- [提示词学习与版本中心（S5-P）](./PROMPT_LEARNING.md)
- [TokenHub 集成边界](./TOKENHUB_INTEGRATION.md)
- [Google Antigravity 边界](./ANTIGRAVITY_INTEGRATION.md)
- [Antigravity 公共 Relay 接入闸门](./ANTIGRAVITY_PUBLIC_RELAY_GATE.md)
- [完整内容日志](./FULL_CONTENT_LOGGING_CUSTOM.md)
- [Claude 组织用量观测边界](./CLAUDE_USAGE_REPORT.md)
- [认证和 Cookie 安全](./authentication.md)
- [实机与副本验收清单](./REAL_DEVICE_ACCEPTANCE.md)
- [开发执行计划与缺陷矩阵](./DEVELOPMENT_EXECUTION_PLAN.md)
- [全项目完成执行计划](./PROJECT_COMPLETION_EXECUTION_PLAN.md)
- [Linux 新对话交接](./NEXT_SESSION_HANDOFF.md)
- [macOS 开发迁移指南](./DEVELOPMENT_ON_MACOS.md)
- [新设备 Codex 交接提示词](./CODEX_HANDOFF_PROMPT.md)
- [定制说明](../CUSTOMIZATION.md)
- [部署说明](../DEPLOYMENT_CUSTOM.md)

## 1. 产品目标

My API 是独立的 AI API 网关发行版和运行时品牌，提供三个交付选择：

1. **Full 完整版**：面向团队、组织和完整管理需求，主要部署在服务器，提供渠道、用户、权限、额度、订阅、支付、日志和运维管理。
2. **Lite 轻量版**：面向个人和小规模使用；正式支持个人服务器、云服务器、VPS 与个人电脑。轻量化指安装、依赖、默认配置和资源占用，而不是把 Lite 限制为局域网或删除可靠转发、鉴权、Key、日志、额度安全、必要备份恢复等核心能力。
3. **Desktop 桌面版**：Lite 的桌面安装和管理形态，复用 Lite 的业务核心和数据模型，提供安装、初始化、后台服务、托盘、升级、备份恢复和本机集成体验，不维护第二套业务实现。

功能版、安装形态和访问范围必须分别建模：`Full|Lite`、`server-native|server-container|personal-native|personal-container|desktop`、`local|lan|public`。服务器可以选择 Full 或 Lite；没有服务器的用户可以在个人电脑运行 Lite 或安装 Desktop。初始个人电脑安装只开放本机访问，LAN 与公网均须独立、明确开启；公网可达必须经过环境检测和外部验证，不能由本机健康检查推断。

核心原则：

- MyAPI 是发行版品牌和运行时品牌。
- 上游凭据只在 My API 服务端配置。既有 Full 原生 Codex 凭据导入边界不因 S5-P 扩大；提示词学习中的指令文件读取、模型外发和文件写入分别授权，永不扫描主目录、凭据库或模型内置指令。
- 本机、LAN 与公网访问均默认最小暴露；扩大访问范围必须显式确认，并保持用户、Key、权限、额度和限流规则。
- Full、Lite 与 Desktop 共享可靠的请求转发、日志、权限和安全边界；功能差异由能力矩阵明确展示，不能因更名静默删除既有核心功能或历史数据。
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
- 概览额度面板已收敛为最多四个最近观测账户的紧凑分析卡：当前剩余额度、最近区间速度、
  最近一小时加权平均的每分钟/每小时消耗、reset-aware 预计可用时长、观测覆盖率和最多
  48 点的服务端剩余额度折线。概览不再复制渠道详情的范围、颗粒、指标、图形和算法控件，
  也不再为每个序列追加历史请求；完整分析入口保留在管理员渠道页。
- 本机测试副本已在明确授权下切换到当前构建的 `local/new-api:myapi-quota-ui-20260902` 并通过健康检查；SQLite 迁移和旧镜像回滚也已在临时副本完成。其他生产环境仍需由部署方按 `docs/UPGRADE_REHEARSAL.md` 完成副本验证并明确批准后再操作。

### 统计和品牌

- 用量分布支持分钟、小时、天、周，以及时区偏移和边界处理。
- Codex 网页 OAuth 登录入口已存在。
- Codex 本机登录导入的后端边界已实现：只解析当前进程的 `CODEX_HOME/auth.json` 或
  用户目录 `.codex/auth.json`，不接受客户端路径、不修改原文件、不向浏览器返回 token；
  新建渠道与 abilities 同事务，既有渠道/刷新使用跨实例命名租约、条件更新和缓存发布
  generation，避免并发轮换或旧快照覆盖。Full 原生允许显式自动导入；官方容器镜像固定
  `MYAPI_RUNTIME_ENV=container` 并返回宿主凭据不可见，LAN edition 后端硬拒绝自动扫描。
  管理界面向导、Node 手工导出 fallback、七语言、焦点/窄屏滚动和实际 Chromium 页面已
  完成本机验收；真实账号操作与同提交 CI 仍按执行计划收尾。
- 默认推广内容已精简。
- MyAPI 品牌运行时参数：`VITE_BRAND_NAME`、`VITE_BRAND_LOGO`、`MYAPI_BRAND_NAME`、`MYAPI_BRAND_LOGO`。
- 当前 Full/Legacy LAN 构建参数和 Logo 配置已接入 Docker 与部署脚本；这不是新 Full/Lite 三维解析器。
- API Key 已开始返回兼容旧 `group` 的访问方案元数据，创建和列表 UI 已显示“Access profile”及用途说明；`GET /api/user/self/groups` 另返回独立的 `account_tier` 元数据，Key 创建时会同时解释“账户等级”和“访问方案”的边界。
- 设置引导已按用户和版本隔离；完成后自动移除引导卡片，不再显示“设置引导已完成”或重复打开入口。
- 渠道余额对话框已接入额度历史折线图，支持 24h/7d/30d/90d、自定义日期范围、自动/raw/hour/day/week 聚合、浏览器时区偏移、加载/失败/空数据、多 Key 解释和失败采样断点；手动刷新会使趋势查询失效并重新读取。快照可通过 `CHANNEL_QUOTA_SNAPSHOT_RETENTION_DAYS` 启用每日限批清理。
- Codex OAuth 渠道的 Account Info 对话框已增加“当前窗口 / 历史趋势”切换；历史只保存官方 WHAM usage 响应中规范化的 primary/secondary 使用百分比、窗口和重置时间，不保存原始响应或凭据。后台有界采样任务默认开启，会按渠道锁调用同一官方 usage 接口并记录成功/失败状态；管理员可在「系统设置 → 运维 → 监控与告警」中调整，部署环境显式设置 `CHANNEL_QUOTA_SYNC_ENABLED=false` 时强制关闭。
- 概览页和管理员渠道页共用 `GET /api/channel/quota/changes` 的精确系列身份与服务端分析。
  概览固定 24 小时查询、1 小时分析窗口、30 分钟 EWMA 半衰期、最多四个最近系列和
  48 个折线点；渠道页继续提供 1h/6h/24h/7d/30d/90d/自定义范围、完整颗粒度、
  available/used/total/consumption/rate 指标、折线/面积/柱形，以及 latest interval、
  observed window、EWMA 三种计算方法和独立分析窗口。`range`、`granularity` 与
  `rate_window` 互不替代；图形和方法切换不触发无意义请求。失败、恢复、重置、长缺口、
  基准变化或样本不足不会被插值，ETA 若晚于重置则明确返回“先重置”，不跨窗口外推。
- 额度变化聚合项同时返回与渠道历史一致的只读 `alert` 状态（`disabled`、`unavailable`、`healthy`、`warning`、`critical`）和阈值元数据；失败、无总量或不支持的渠道不会继承旧状态，也不会触发通知、停用渠道或改变路由。
- 额度概览和渠道详情的查询会使用正常认证刷新流程（不再跳过 `401` 后的 session refresh）；TanStack Query key 同时包含用户 ID、session SID 和能力状态，避免同一标签页切换登录身份后短暂复用上一位管理员的额度或采样状态。`a2528a2` 增加了跨登录身份回归测试；它不改变后端权限边界，也不缓存凭据。
- 额度采样状态区分 `success`、`error`、`unsupported` 和 `unavailable`：没有官方余额端点的 Claude/Azure 等渠道记录为 `unsupported`，不会被误报为可修复的网络故障；管理员渠道面板可独立筛选该状态。
- 当前“账户”展示语义是脱敏后的渠道/指标序列：`Accounts tracked` 统计分组序列数量，不保证等于真实上游订阅账户数；多 Key 渠道在 MVP 中不合并或拆分各 Key 的余额。后续若要区分同一渠道的多个订阅账户，必须先取得 provider 返回的稳定非敏感账户标识（或由管理员显式配置别名），再扩展快照主键、聚合 API、权限和迁移，不得使用 API Key 原文或未经验证的响应字段。
- 渠道类型选择已将 Codex 置首，并在 Codex、Claude、Gemini/Antigravity 入口显示实际能力边界。
- Claude adaptor 的边界防护已在 `1827358` 固化：缺失 `RelayInfo` 或空 base URL 会被拒绝，尾部斜杠会规范化，出站请求默认补齐 JSON `Content-Type` 与 `anthropic-version`；这些保护只影响请求构造，不改变 Claude Messages/Responses 兼容范围，也不提供账户余额读取。

### TokenHub、Antigravity 和发行基础

- TokenHub 有独立边界文档和 provider-neutral 适配边界；后续 UI 和官网只借鉴其产品叙事与信息组织，不复制实现或页面。
- Google Antigravity 已按官方 Gemini Interactions API 建立独立的非持久化客户端边界（创建、有限轮询、取消、删除和 usage 提取）。`1827358` 进一步固定 dynamic agent 类型、仅通过 `environment` 承载 continuation 的环境标识、限制 interaction ID/输入/响应大小、将 `requires_action` 视为轮询终态并在错误中去除上游响应正文；但尚未接入普通 relay/channel 或账户额度，不应宣传为完整 Antigravity 账户或额度支持。
- Legacy LAN CLI、SQLite-first 项目初始化和局域网安全边界已存在；服务器 Lite 与个人电脑 Lite 的正式交付仍未实现。
- Electron 桌面版默认回环监听、单实例和持久会话密钥已加固；`--allow-lan` 加私网绑定地址才可共享，并在托盘菜单显示生效端点。
- Electron 生产后端就绪探针请求 `/api/status`，要求 HTTP 2xx 且 JSON `success=true`；开发前端探针仍使用 `/` 并只校验 HTTP 状态。该行为由 runtime-config、探针合同测试和 desktop check 固化，尚未替代真实 macOS/Windows 安装与局域网演练。
- 新开发环境的 PostgreSQL 默认数据库标识已统一为 `myapi`（`docker-compose.dev.yml`、`makefile`）；接管旧数据必须显式设置 `MYAPI_DEV_POSTGRES_DB` 或 `DEV_POSTGRES_DB`，不自动重命名或迁移既有数据库。
- `deploy/install.sh` 与 CLI 使用相同的回环/RFC1918 绑定边界；安装脚本拒绝格式错误或公网地址，非回环监听必须显式设置 `MYAPI_ALLOW_LAN=true`，并以非执行方式读取 `.env`。
- GitHub Actions 已支持 SemVer tag 构建并推送 Full/Legacy LAN GHCR 镜像；多架构 manifest 使用构建任务产出的、经过格式和仓库校验的架构 digest 组装，不再以可变架构 tag 作为 manifest 输入。它不是后续统一 Release Manifest。
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

当前代码 `02a0aaf` / [CI 33805743908](https://github.com/ForceMind/MyAPI/actions/runs/33805743908)
七项成功；最新适用镜像证据是 `237c0da` 的
[Docker smoke 33790513455](https://github.com/ForceMind/MyAPI/actions/runs/33790513455)，
Full/LAN 两项均成功。**B1、C06/C07、S4-01/S4-02 与 D01–D06 已完成当前范围**：
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
#### 2026-09-05 B2-0 恢复账务合同（已冻结，完整流程尚未实现）

B2-0 的未知态、幂等和 Task 业务事件权威合同已确认；可消费余额权威源与历史不明余额处置（D02/D03）仍是建议待决定，不能误写为 B3 已确认。B2-1 模型/迁移基础已完成当前范围的受限本机验证、独立审查和隔离实库 CI。
Ali、Doubao、Gemini、Hailuo、Jimeng、Kling、Sora、Suno、Vertex、Vidu 的 B2-2A 纯响应 parser 在 HTTP 200 时已由 legacy `DoResponse` 调用，gate-off 非 200 安全兼容桥则在 parser 前处理；Task 单个 outbound attempt 的 one-shot body、3xx 不跟随、客户幂等头隔离及 Vertex OAuth JWT 换取无重定向也已有合成测试与独立审查。这些本地子范围不等于
durable submission。B2-2B0 现有无 caller 的严格 protocol、operation+attempt T0 原子基元和 Full Content 字段级幂等脱敏，并有 owner-scoped、只读的 `GET /v1/task-operations/:id`；B1a/B1b 另有 JSON 与仅 video form/multipart 的严格 canonical request fingerprint，但都不含 HTTP POST 提交、原子账务或恢复闭环，不能启用生产开关；controller 的 legacy retry/failover 也仍待 B2-2B/C durable dispatcher 接管。现代 Task 的 `submission_unknown` 与已受理后轮询结果未知的
`outcome_unknown` 不自动重发或退款，只能由上游可验证的结果或带审计记录的人工处置结束；v1 在进入
`DISPATCHING` 后禁止一切可能已送达请求的重试及跨渠道 failover，包含 Provider 内重试。

幂等范围为 `token + HTTP method + operation kind`：同一 scope/key/请求指纹复用原 operation，同一
scope/key 但指纹不同返回 `409`；活动 operation 不自动过期，终态后保留 180 天。adaptor 必须先持久化
operation，随后统一返回 `202` 和稳定 operation/task ID，所有状态可查询，不能先写成功响应。主库账务事件
是唯一权威账本；分库/ClickHouse 日志为带稳定 `billing_event_id` 的至少一次投影，用户查询、导出、统计
必须去重，重投不得成为重复用量。

首版仅覆盖现代 Task，Midjourney 保持在独立后续阶段。C03b 完成前 gate 保持关闭；开启前须排空并升级全部
旧 writer/poller，不承诺新旧 worker 混跑。后续应复用既有 CAS/租约/快照，不重建平行系统；三库加法迁移、
故障恢复验证及生产启用申请仍须分阶段完成。见[完整执行矩阵](DEVELOPMENT_EXECUTION_PLAN.md)。

根模块 JSON wrapper 复审重新识别出 67 个直接序列化调用/27 个生产文件；此前 Provider
小批完成不等于全仓合规。已按协议风险拆为 D01–D10，排除 `common/json.go`、测试、
relaykit 和合法类型/`json.Valid` 使用。D01 已在本机将 GitHub/Discord/OIDC/Linux DO 的
9 处清零，GitHub 合成 transport 回归、OAuth race、根模块全量及独立审查通过；随后 D02–D10
逐批收敛，D10 已清除 `pkg/cachex/codec.go` 最后两处调用，根模块生产实际 JSON
Marshal/Unmarshal/Decoder/Encoder 直调余量为 **0/0**。扫描仍排除 `common/json.go`、测试、relaykit、
合法类型/`json.Valid` 与一条注释；不整体重写、不访问真实 OAuth 凭据，详见
[执行计划](DEVELOPMENT_EXECUTION_PLAN.md#s2-d-根模块-json-wrapper-合规)。

C07/D02 本地修复了 Kling/Jimeng 兼容入口的旧正文缓存遮蔽和 Provider metadata 二次选模/
时长旁路：正文缓存、直接 Body、GetBody 与 ContentLength 原子切换；Full Content 保存鉴权
后的原始客户端请求并冻结同一入口身份，下游只读统一 envelope；模型别名不能覆盖已映射
模型，Kling duration/mode 和 Jimeng frames 不能绕过顶层验证。Jimeng 官方 frames 仅
121/241，正规化为 5/10 秒。红绿、全包 race、根模块全量及两轮独立 Sol 审查已通过；
`f7cc5c3` 的七项 CI 同样成功，JSON 余量降至 56/21，C07/D02 当前范围完成。未连接真实
Provider，也未处理 Jimeng 查询 handler 候选或全局 metadata 系统；详情见
[执行计划](DEVELOPMENT_EXECUTION_PLAN.md#s2-d-根模块-json-wrapper-合规)。

D03 本地把 OpenRouter Anthropic thinking、Replicate output format 与模型映射三处解码
等价迁移到 `common.Unmarshal`，保留 RawMessage 类型和各自既有静默/错误/循环/覆盖顺序。
定向与 race 三包、vet 和独立 Sol 审查通过；`615fbbd` 七项 CI 成功，结构余量为 53/18，
D03 当前范围完成。不借 wrapper 迁移改变 Provider 协议、模型 mapping 算法或全局设置。

D04 本地完成 SiliconFlow/Tencent/Vertex 五处 wrapper 收敛，并修复 Vertex token 边界：
非 2xx、provider error、malformed、缺失/错误类型/空白 token 只返回固定安全错误，不再把
完整上游响应 map 带入 API 错误。三包 handler/parser 普通与 race、vet、独立 Sol 审查及
`544f83b` 七项 CI 通过，结构余量为 48/15，D04 当前范围完成；没有真实 Google、代理、
JWT 或凭据访问。

D05 本地完成 Midjourney 单文件 8 处 wrapper 收敛；SQLite 真实 handler 回归锁定空
VideoUrls 持久化、Buttons/Properties 历史兼容、camelCase 单对象/条件数组/空 `[]` 响应。
relay 全包普通/race、vet、独立 Sol 审查及 `b2b60fd` 七项 CI 通过，余量为 40/14，D05
当前范围完成；不改变上游、计费、转发 URL 或历史错误字符串。

D06 本地完成 Controller 8 处 wrapper 收敛，并让规则模型 endpoint JSON 稳定排序；保留
合法 `json.Valid`/RawMessage。Vertex key 与 Uptime helper 离线测试、Controller 全包
普通/race、vet、独立 Sol 审查及 `02a0aaf` 七项 CI 通过，余量为 32/11，D06 当前范围
完成；Ollama/endpoint 专用集成测试为非阻断剩余，不改变其现有流式或集合合同。

S2-C08 / D07 已完成当前范围：Settings 五个目标文件 13 处直接
JSON 调用清零，根余量降为 19 处/6 文件；同时收紧设置 fresh 发布、倍率和非有限值、模型
DB 前验证/批量 rate 聚合、generic config 全对象原子验证、ConfigManager 回调锁、集合 null
规范化与模型成功限流的快照、溢出、动态 key 过期和缩小 prune 合同。独立 Sol 首审修复后
复审无 P1/P2。本机完成同 CI race、限流 race `-count=2`、根测试/vet/build、relaykit 独立
vet/build/test、格式/diff/YAML 检查。最终 `2d6acab` /
[CI 33814136556](https://github.com/ForceMind/MyAPI/actions/runs/33814136556) 七项成功，Backend 原始日志确认新 D07 十包
race 全部实跑。无页面或 schema 变更，版本保持 0.1.1；未执行真实上游、生产、真实设备或发布。
同 SHA 常规 MySQL/PostgreSQL job 成功，但本批无专用三数据库 Settings 行为场景。

这不关闭独立 S2-C09：generic config 热读尚无统一快照/锁；内存和 Redis 成功限额仍是
check→execute→record 的近似合同，并发可能超发；跨配置族 reload 非全量事务；历史 DB raw
`null`、未知分层 key、Passkey 懒写和 `GroupRatioSetting` 可变指针待审计。C09-N1 已盘点 24 个注册族：
22 个已有受控快照/复制语义，仅 `performance_setting` 与 `channel_affinity_setting` 分别等待 D14/D15；除既有族外，`gemini`、`billing_setting`、`payment_setting` 与 `global` 也在不改变原键/默认/宽松 parser 或业务校验的前提下完成私有原子 generation 与 detached getter 子范围。仍待的是两族受决策阻塞热读、跨族事务、N2b/N3/N4/N5b 接线和三数据库验收。D09 与 D10 均已完成当前范围并通过同提交 CI。

S2-C09-R1 已完成当前范围；独立最终复审确认无 P1/P2。最终 `ae07527` /
[CI 33824814509](https://github.com/ForceMind/MyAPI/actions/runs/33824814509) 八项成功，包含独立真实 Redis 7
limiter lifecycle Job、S1 MySQL 5.7/PostgreSQL 9.6 fixture 与扩展 Backend race。
当前范围包括：payment compliance 五字段以单次 `UpdateOptionsBulk` 写入；SQLite 覆盖成功及保留旧值的
rollback，同一 helper 已由 MySQL 5.7/PostgreSQL 9.6 CI engine 流程执行。工具价格配置改为 source 与 index 在同一
代际不可变原子发布，公开 DTO 保持兼容，严格 `MapConfig` 与历史宽松 loader 分离。`ConfigManager.SaveToDB`
在完整快照完成后于锁外回调，覆盖重入和错误释放。Redis limiter 取消首个 client singleton，Lua 通过
`redis.Script` 在 `NOSCRIPT` 后恢复；TTL 按自然复满时间设置，不再截断为 24 小时；Go 在触碰 Redis 前拒绝
非正、超过 2^53 或 `Requested > Capacity` 的参数，Lua 还在写入前拒绝非整数，拒绝路径零写入；miniredis 回归已通过，新增独立
真实 Redis 7 CI job 已实跑。`common/limiter` 已导出并复用 `MaxExactInteger`/`ValidateConfig`；Settings 对默认及每个
group 在 DB 写入前均按实际 `capacity=total*durationSeconds`、`rate=total`、`requested=durationSeconds` 使用同一
2^53-1 精确边界，覆盖大于 2^53 且不超过 MaxInt64 的拒绝，并确保 runtime/OptionMap 不发布；保留 `total=0` 和
disabled `duration=0`。仍不完成：成功限额仍为 check→execute→record，未引入 reservation/rollback；
总量仍令牌桶，未改变产品语义；R1 当时 24 个注册族中尚余 6 个通用热读，且跨族事务、Passkey/null/未知 key/
`GroupRatioSetting`、payment runtime 的 legacy 逐字段读取与活指针接线仍待；后续 N1 子范围已将热读余量收敛为 D14/D15 的两个族。真实付款、生产、设备及发布仍未做。`VERSION` 保持 0.1.1。

`channel_affinity_setting` 的后续收口必须单列：复杂规则/模板会改变渠道、retry、上游参数和账务归属，且 capacity/TTL 当前只在 HybridCache 首建时读取。D15 需先确定重启或受控 drain/rebuild/epoch 的 cache 生命周期；不能把普通设置保存实现成静默清 cache、Redis 迁移或路由语义变更。

2026-09-06 新增的 C09-N2a 只完成一个无副作用的 PaymentRuntime 内核：显式 seed 的 immutable generation、typed set/clear/keep、detached snapshot、候选 CAS/abort 与 canonical option copy 均留在 `setting/payment_runtime.go`，且把所有现有支付 option 与 `TopupGroupRatio` 纳入同一代际。它没有 caller，不读取或替换 legacy global、`OptionMap`、数据库、Provider SDK 或网络，因此不会改变真实支付行为。`go test -p 1 -count=1 -timeout=180s -v ./setting`、`go vet -p 1 ./setting` 在单核/768MiB 隔离组成功；独立审查发现并修复 `TopupGroupRatio` 的代际遗漏后复审无 P1/P2。后续仍须完成 N2b 请求快照、N3 typed DB/runtime writer、webhook keyring 窗口、Provider 补偿和三数据库验收，不能把本段当作支付接线或 C09 完结。

同日 C09-N5a 增加了 root-only 的 `/api/option/diagnostics` 本地诊断子范围：精确路径在全局限流前写 no-store，`DisableCache → RootAuth` 覆盖业务 401/403/200；query 先以 `url.ParseQuery` fail-closed；业务层只做主库 `options` 的 1024 行有界 SELECT，key 仅读取前 256 字符和一字符截断探针，value 仅读取前 65537 字符，按类型元数据诊断 raw-null/空/非法 UTF-8/截断、注册 schema 和 GroupRatio alias，不读写 OptionMap、配置 generation、LOG_DB 或 Redis。超长/未知/畸形/dynamic external key、值和 parser error 一律不回显；MapConfig 不调用 validator，截断 source 或覆盖不全只给不确定状态。平台继承的鉴权/限流仍可使用自身 Redis cache，不能把该业务限制错误描述为全 HTTP 请求零 Redis。SQLite 合成 test/vet 与独立审查已完成；MySQL 5.7/PostgreSQL 9.6 实库证据为 P2，race、CI 和 N5b 修复流程仍待。

同日 C09-N1 继续以低风险批次收口 `general_setting`、`console_setting` 与 `checkin_setting`：三者只替换原地反射发布为 immutable generation、完整 partial candidate 和 detached getter，保留注册名、键、默认值、unknown/partial/`null`/标量解析语义。`GetStatus` 对 general/console 字段各使用单代 snapshot；checkin 没有新增额度范围业务校验，负数、反向范围和极端差值风险仍待独立决定。单核/768MiB 下相关定向 Go test/vet 通过，独立审查无 P1/P2/P3；这不形成跨族 runtime 原子性、三数据库热更新或成功硬限额证据。

同日对余下的 `performance_setting` 只做了盘点：它除 source 配置外还分别发布 disk/monitor 两个 `common` 投影，批量/重载可逐字段发布中间代，磁盘缓存一次操作也会多次读取配置且路径创建会重新读取全局 path。因此不能将其作为普通低风险 generation 批次接入；D14 需先决定 `DiskCachePath` 热切换生命周期，以及是否要求八个性能字段跨所有消费者同一 runtime generation。本段没有代码、测试或运行行为变更。

同日 `token_setting` 完成了低风险快照化：保持注册名、`max_user_tokens`、默认 `1000` 和 generic parser 的宽松语义，私有 immutable generation 与 detached getter 只关闭 live-pointer race；它没有改变后端对 `0`/负值的历史接受行为，也没有处理 Key 数量检查与创建之间的竞态。单核/768MiB 定向 Go test/vet 和独立审查通过。

同日 `grok`、`discord` 与 `oidc` 完成独立 immutable generation 子范围。Grok 保持违规收费开关/金额、有限 float parser 和 `service/violation_fee.go` 的既有单次读取，不新增收费规则；Discord/OIDC 保持全部 OAuth 持久键、HTTP/endpoint/redirect 语义和 secret 边界，`GetStatus` 对各族只取一次 snapshot。随后 `quota_setting` 保持免费模型预扣开关与默认 `true`，`qwen` 对同步图片模型列表做深复制并保留 `null`/空列表和既有 `Contains` 匹配；`fetch_setting` 对 domain/IP/port 列表深复制、保持默认 SSRF 策略并取消 getter 对 live policy 的可写暴露。三者均不改变 relay、账务或 Provider 语义。上述批次完成时尚余 6 个热读族；后续的 `gemini`、`billing_setting`、`payment_setting` 与 `global` 已进一步收口，目前仅余 D14/D15 所阻塞的两个热读族，以及跨族发布和服务端 Key 限额事务。

`gemini` 的 request-private `ConvOptions` 现在连同 `SafetySetting` 与 `SupportsImagine` 回调绑定到同一 generation；`billing_setting` 的 mode/expression 两张 map 同代深复制，定价/同步读取不混代，但连续单 key 更新仍非跨 option 原子合同。`payment_setting` 保持七个既有持久键、金额 option/discount map 与合规判断的 detached snapshot，不接 PaymentRuntime、订单、SDK 或回调。`global` 深复制模型黑名单和策略图，`PreserveThinkingSuffix` 绑定捕获的 snapshot，缓存中旧 options 仍是旧代且重试按既有路径重建。四项单核/768MiB 相关 Go test/vet 均通过；独立审查在补齐 Gemini callback、Global partial-policy 回归后无 P1/P2/P3。它们不改变 Provider、账务、支付、路由或三数据库持久化语义。
本机实际通过扩展后的 Settings/C09 同 CI race、根模块全量 test/vet/build、relaykit 独立 vet/build/test、
gofmt、diff-check、YAML 与根 JSON 静态门禁。CI 原始日志确认 Redis 7 的短/25 小时 TTL、Go/Lua 拒绝和
`SCRIPT FLUSH` 恢复均实际执行；MySQL/PostgreSQL engine 测试及 11 包扩展 race 均成功，不以 miniredis 或
SQLite 替代这些结果。

S2-C09-R2 已完成当前范围。最终 `2575f5b` / [CI 33828024982](https://github.com/ForceMind/MyAPI/actions/runs/33828024982)
八项成功；Backend 原始日志确认改名后的 `Verify settings, rate-limit, and request snapshots` 12 包均为 `ok`，
包括 `setting/model_setting`、`operation_setting` 与 `relay/common`，不是 no tests；其余七个 job 亦成功。
`ValidatingMapConfig` 只作纯验证，既有
`MapConfig` 仍保持 unsupported。Claude 采用私有 atomic 完整代，getter 对三层 map/slice 深拷贝并保留 `[]`/`null`
形状；`null`/`{}` 仅在读取副本补 8192，不污染 export，严格失败不发布。`GenRelayInfo` 捕获 request-private Claude
代，handler/header/converter 使用同一代。Monitor 使用私有 atomic 代，保留 env frequency 优先于 enabled 的顺序；
移除 env 后恢复 DB，mode/concurrency 只影响 effective 值且不污染 export，`runChannelTestTask` 取一次快照并发 partial
不丢。测试改为单次屏障，无 sleep 或概率循环。独立 Sol 最终复审当前范围无 P1/P2；P3 是 `GlobalConfig.Get` 的动态
具体类型转为私有 manager，公开 DTO/getter 签名兼容且仓内无断言。

本机已通过受影响包普通测试、workflow 同款 12 包 race（`common`、`common/limiter`、`types`、五个 setting
相关包、`model`、`middleware`、`controller`、`relay/common`）、`go test -p 1 ./...`、vet、build、relaykit
`GOWORK=off` vet/build/test、gofmt、diff、YAML/JSON 门禁。R2 不需且未做真实 Redis、三数据库、前端或上游；无页面/
schema 改动，`VERSION` 仍为 0.1.1。仍待：Passkey/ServerAddress、payment runtime/密钥轮换、`GroupRatioSetting` alias/
前端 bulk、成功 hard limit 与跨族事务。R1 文档收尾 `3af738e` / [CI 33825533002](https://github.com/ForceMind/MyAPI/actions/runs/33825533002)
八项成功。

R2 文档收尾 `3c63e02` / [CI 33828752473](https://github.com/ForceMind/MyAPI/actions/runs/33828752473) 八项成功。
S2-C09-R3 已完成当前范围。最终 `e3cd185` / [CI 33831492021](https://github.com/ForceMind/MyAPI/actions/runs/33831492021)
八项成功。原始 S1 日志明确 MySQL 5.7 与 PostgreSQL 9.6 均执行
`group-ratio-alias-contract/create-rollback/{mixed-existing-canonical-and-missing-alias,both-missing-second-create}`，全 PASS 且无 skip；
Backend `Verify settings, rate-limit, and request snapshots` 13 包均为 `ok`，包括 ratio/model/controller/service/relay-common。
GroupRatio 三图由私有 writer 加 atomic 单快照发布，嵌套值深拷贝；
detached 公开 DTO 保持三字段 unkeyed/JSON 兼容、receiver-local，NaN/Inf 导出错误继续传播；注册表动态类型改为私有
manager，但公开 DTO/函数不变且仓内无生产类型断言。special 的空 user/target，
以及 direct、`+:`/`-:` 同目标冲突均在写前拒绝；service 读取 detached special getter，`+`/`-` 语义不变。基于当前 UI，
平面 `GroupRatio`/`GroupGroupRatio` 为 canonical，分层键仅兼容；新 JSON 语义规范化并在事务中双写，bulk 冲突写前拒绝，
OptionMap/runtime 双键同值。历史加载有效 canonical 优先；invalid canonical fallback 有效 alias；双方 invalid 保留最后有效
runtime，明确 warning 且不写回 DB。SQLite 覆盖反向行序、alias-only/conflict、update/mixed/create rollback；同一合同已接入
现有 MySQL 5.7/PostgreSQL 9.6 gated 子测试，并已由同提交 CI 通过。

独立 Sol 最终复审无 P1/P2/P3。本机已通过 ratio/model/service/controller 普通测试、workflow 同款 13 包 race、
`go test -p 1 ./...`、vet、build、relaykit `GOWORK=off` vet/build/test、gofmt、diff、YAML/JSON 门禁。首次 service/controller
race 仅因磁盘满导致链接失败；只清理 7.9GB 可重建的 `/private/tmp/myapi-gocache` 后原命令通过，未触碰仓库或 DB。R3 不完成
前端 bulk/跨多 HTTP 事务、历史 DB 清理、payment/Passkey/hard limit/global config；无页面/schema，`VERSION` 仍为 0.1.1。

D08 本地将 `pkg/ionet/client.go`、`pkg/ionet/jsonutil.go` 各 4 处实际 stdlib JSON 调用迁移至
`common` wrapper，根模块余量由 19/6 降为 11/4。无网络 fake client 覆盖请求 body/headers/method/URL、
NaN marshal、transport/API detail fallback、query slices/HTML escape/空值/零值/false/`time.Time`/
`*time.Time`，以及 flexible time 的对象/数组、直接或 `data` 包装、无时区 UTC、带时区 offset、未知或普通
字符串、malformed、错误类型和尾随值。普通测试、race `-count=2`、vet、gofmt、diff-check、根全量
test/vet/build 及 relaykit 独立 vet/build/test 已通过。最终 `9193ada` /
[CI 33816756504](https://github.com/ForceMind/MyAPI/actions/runs/33816756504) 七项成功。独立 Sol 无 P1/P2，数组和 `*time.Time` 的 P3
已补；保留 `interface{}`→`float64` 大整数精度风险与递归时间字符串识别的既有语义。无页面/schema、真实
io.net、凭据或网络访问，版本保持 0.1.1；D09 的 endpoint path 逃逸、nil response 等边界另审。

D09 本地完成 `pkg/ionet` container/deployment/hardware 三文件 9 处 `Unmarshal` 到 `common.Unmarshal`
的收敛，根余量由 11/4 降至 **2/1**（仅 D10 `cachex` codec）。所有动态 deployment/container/cluster
path segment 经 `PathEscape`，stream options 仅局部复制；`makeRequest` 的 nil、仅 2xx 成功和非 2xx 固定
`APIError` 合同已锁定，错误不回显 raw response/detail，controller 用 `errors.As` 识别。默认 HTTP client
拒绝 301/302/303/307/308，防 `X-API-KEY` 及敏感 body 跟随重定向泄露。离线 fake 与 loopback `httptest`
覆盖合法/malformed、null/空/缺 ID 防伪成功、mutation/hardware/location、合法 `false`、五类重定向、path
逃逸与 stream options 不变性；未访问真实 io.net、凭据或公网。`pkg/ionet` race `-count=2`、controller
定向 race、vet、gofmt、diff-check、根全量 test/vet/build 及 relaykit 独立 vet/build/test 已通过。最终 `8228203` /
[CI 33819117410](https://github.com/ForceMind/MyAPI/actions/runs/33819117410) 七项成功，新 io.net race `-count=2` 门禁实跑通过。
独立 Sol 无 P1/P2；P3 为 hardware/location 必填回显尚需真实脱敏响应或官方 schema 补验。无页面/schema
变更，版本仍为 0.1.1。

D10 已完成当前范围：`pkg/cachex/codec.go` 的最后两处 JSON wrapper 收敛为编码走 `common.Marshal`、解码走
`common.Unmarshal([]byte(s), ...)`，并保留 string→`[]byte` 的复制。审查发现直接改用 `UnmarshalJsonStr` 的 unsafe
别名会允许自定义 `UnmarshalJSON` 改写调用者字符串，codec 路径已避免该回归；测试覆盖嵌套 round trip（含 `0`/`false`）、空白、
malformed、类型错误、多个 JSON 值、尾随空白、func 不可编码及 mutating unmarshaler 输入不变性。cachex race
`-count=2`、根全量 test/vet/build、relaykit `GOWORK=off` vet/build/test、gofmt、diff-check、静态门禁均通过，
独立复审最终无 P1/P2。最终 `bf03cba` /
[CI 33821142971](https://github.com/ForceMind/MyAPI/actions/runs/33821142971) 七项成功；Backend 原始步骤确认根生产代码
`git grep` 静态门禁及 io.net/cachex race `-count=2` 均实际成功。无页面/schema/Redis/真实缓存服务改动，
`VERSION` 仍为 0.1.1。

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
| 独立 UI 系统 | 尚未开始（代码层先行） | 不依赖旧 New API 信息架构，Full/Lite/Desktop/移动端完成真实画面审查；当前已有的 Logo、品牌文案和局部功能面板不计为完整 UI 替换 |
| TokenHub 风格静态官网 | Chromium smoke 与审阅制品已实现 | 形成独立产品叙事、安装入口、发行版选择、安全说明、响应式菜单无障碍/主题交互、静态资源自动校验、390px/320px Chromium 移动 smoke（含窄屏水平溢出断言）和 main 变更自动生成的确定性 artifact workflow；真实移动视觉审查与绑定域名的独立发布仍待完成 |
| 渠道额度历史 | 后端和前端初版已实现 | 普通渠道与 Codex OAuth 渠道均已有历史查询、折线图、失败状态、可选保留清理和只读健康指标；历史聚合按计划、单位、币种和窗口系列隔离，未指定系列时锁定最新系列；普通渠道与 Codex OAuth 均支持可选、有界后台采样；告警阈值和 notifier-neutral 去重策略已支持默认关闭、原子持久化和只读状态展示，外部通知通道仍待业务决策 |
| 账户额度变化聚合 | 初版已实现 | 概览和管理员渠道页均可查看每分钟变化及最大变化排序；概览页在前台每 60 秒自动刷新，并显示 provider plan type 与错误采样状态；普通渠道与 Codex OAuth 后台采样已接入系统任务并避免与旧轮询重复，跨账户订阅账单同步和通知仍待后续迭代 |
| 账户等级/Key 访问方案 | 独立策略注册表、S3-0 盘点与 PRE1 纯预检内核已完成当前子范围 | 管理员可在计费设置的“Key access profile policies”编辑稳定 profile ID 的显示名、说明、路由组、模型白名单、回退方案和启用状态；Key 表单会显式提交 `access_profile_id` 并同时保留 legacy `group`，显式 `account_tier_id`/`access_profile_id` 会持久化，旧客户端省略时按现有记录或变更后的 `group` 兼容回退。PRE1 只形成 detached diagnostic，永不强制；路由/模型强制执行仍需 D04 后的单独迁移评审 |
| 设置引导生命周期 | 初版已实现 | 完成后自动消失、按用户和版本保存；真实多设备视觉审查仍待完成 |
| Claude 支持 | Messages 原生转发与 Responses→Messages 兼容转换已实现；官方组织用量报告已确认存在 | 只实现有明确官方协议的能力；普通 Claude 渠道仍不读取账户余额，组织 Usage Report 只有在管理员显式配置受保护的 Admin 凭据并完成权限/保留策略后才接入 |
| Google Antigravity 专用 relay | 第一阶段 transport 代码、边界测试和有界 Docker Go 回归已交付 | `AntigravityClient` 已覆盖官方 preview 的创建、状态读取、有限轮询、取消、删除和 usage 提取；`1827358` 增加 dynamic agent/continuation 字段约束、请求/响应大小上限、`requires_action` 终态、nil context 兜底和错误正文脱敏测试；`GOWORK=off go test ./relay/channel/gemini ./relay/channel/claude` 已通过。公开 relay/channel 接入按 [公共 Relay 闸门](./ANTIGRAVITY_PUBLIC_RELAY_GATE.md) 进行持久化、权限、计费和工具策略评审，余额端点不存在时显示 `unsupported` |
| Legacy LAN/当前 Desktop 安全边界 | 合同检查已实现，不是正式 Lite/Desktop 完成交付 | 当前 Electron 默认回环、单实例、显式 `--allow-lan` 私网绑定和私网地址提示已验证；它仍以 `lan` 混合功能版/安装形态/访问范围，未提供服务器 Lite、公共访问向导、统一更新器或正式跨平台交付。P0 已将迁移、安装与更新合同纳入计划，真实安装/网络验证仍待完成。 |
| 新开发环境数据库默认值 | 代码与模板已验证 | `docker-compose.dev.yml`、`makefile` 及多语言 README 的新开发示例默认使用 `myapi`；显式 `MYAPI_DEV_POSTGRES_DB`/`DEV_POSTGRES_DB` 可接管既有数据库，未执行自动迁移或生产改名 |
| Legacy Docker 升级 | CLI 预检与有限 Docker 回退已实现 | 生产端显式拉取、可选签名验证、可选 digest 固定、健康检查、环境文件备份和失败回滚已有；它不证明数据库回退、原生/Desktop 更新、持久 journal 或形态切换。完整目标见 [发行制品、安装与更新合同](RELEASE_MANIFEST.md)。 |
| 统一 NPM 发行与更新 | 未完成；schema-1 结构选择/输入硬化、未受信 raw-bytes evidence 与 D2A 纯安装状态子范围已验证 | 保留 `@forcemind/myapi` / `myapi`，新增同源受信 Release Manifest、平台制品选择、受控清理、更新/回退和切换；现有纯 selector/evidence/installation state 不获取/验签资产、不读写文件或安装，且对 hostile JS 输入 fail closed；正式 NPM/GHCR/tag 发布仍需单独授权。 |
| S5-P 提示词学习与版本中心 | P0 合同与 P1a/P1b 纯内核子范围已完成；完整 P1 未实现 | 默认关闭、授权范围、脱敏派生样本、触发/预算、版本/差异、Codex 文件受限应用、备份/回滚和三形态支持见 [专题](PROMPT_LEARNING.md)；纯内核不接真实日志、模型、文件或存储。 |

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

4. S3-PRE1 现提供不接运行路径的 `service/accesspolicy` envelope：固定 reference/list presence/legacy outcome 经有界校验形成确定性诊断快照，所有模式均不应用结果。它不能读取或更改 legacy 路由、价格、缓存或账务，也不替代 D04 对权益交集、disabled/fallback 和计费快照的决定。

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

### 5.3 S5-P 提示词学习与版本中心

S5-P 是与 S5-Q 额度闭环并列的必选模块，不替代渠道提示词、Full Content 日志、Codex 凭据导入或现有权限模型。目标是让用户在手动启用、限定用户/项目/Key 范围、明确外发渠道/模型和预算后，从**已脱敏且可审阅的新增用户需求样本**生成可复用的 Codex 自定义指令文档。

当前未接线的 P1a/P1b 纯内核负责包内候选的来源类别、文本边界、二层脱敏、scope 隔离指纹，以及完整 server-observed turn 的单一新用户段准入。P1b 明确排除历史/system/developer/助手/工具/附件/响应/internal/automatic-analysis 段，并以独立 observation HMAC 绑定可信物理回执和规范化 eligible payload；它不读取或保存实际日志，不调用模型，也不形成用户可用功能。真实采集、用户隔离、DB 去重、调度、版本中心和文件应用仍按专题计划推进。

核心不变量如下：

- 默认关闭；没有新样本时不调用模型。时间与请求数触发独立启用、可按任一/同时条件组合，双条件同批命中只生成一个 run。
- 原始 Full Content JSONL 不直接作为训练集或模型输入；排除 system/developer、助手、工具、附件、凭据、响应正文、重试、重复上传和自动分析任务，并在发送前进行第二层正文脱敏。
- 需求候选区分长期偏好、重复需求、纠正、禁止、项目规则、一次性任务、已替代规则和冲突/证据不足；高频不自动变全局规则，锁定规则不被静默删除。
- 自动生成、人工编辑和模型再生成都创建不可变版本；冻结样本范围与基线，保留来源摘要、渠道/模型、token、费用、父版本和差异；并发编辑不能被旧结果覆盖。
- 读取 Codex 指令、向分析模型外发、应用到目标文件三者单独授权。定时任务只生成草稿，绝不自动应用；文件应用需检查路径/链接/外部修改、原子备份/替换/校验并可回滚。
- Lite 服务器、个人电脑 Lite 与 Desktop 都可支持本模块，但公网访问者永远不获得宿主文件或 Codex 权限；容器只能经受限宿主集成，LAN 浏览器不能直接写访问者电脑。

S5-P 的数据模型、调度、费用、Codex 集成、阶段和验收见 [提示词学习与版本中心](PROMPT_LEARNING.md)。付费自动分析调用依赖 B3 原子账务、C03b 权威余额/receipt、S3 相关权限和 S4 的恢复合同；在此前仅可进行合成/假上游验证。

## 6. UI 与交互路线

### 6.1 Key 创建

表单按以下区域组织：

1. 基本信息：名称、有效期、IP 限制。
2. Key 访问方案：显示名称、用途、倍率、路由范围和资格说明。
3. 自动路由设置：仅在选择自动路由时展示。
4. 使用范围：模型、额度和速率限制。

账户等级在用户页面单独展示，并明确说明：账户等级决定“用户拥有什么”，访问方案决定“这个 Key 怎么调用”。

### 6.2 设置引导

设置引导按用户与版本记录完成状态；完成所有步骤后应自动移除引导卡片，不能继续渲染“设置引导已完成”提示，也不重复打开已完成入口。当前 Legacy Full 构建的管理员还会看到“配置上游渠道”步骤，该步骤仅在具备 `channel.read` 权限时查询渠道数量；普通用户和 Legacy LAN/极简构建不显示管理员步骤。当前实现已覆盖基础生命周期，Lite/Desktop 的目标引导在 S6 重建。

- 已完成：按当前 Legacy 发行版和角色显示相关步骤；Full 管理员在具备 `channel.read` 时看到渠道配置步骤，普通用户与 Legacy LAN 不显示管理员步骤。
- 已完成：整个引导卡片移除，不占首屏空间。
- 需要帮助：从帮助菜单或“重新查看快速开始”主动打开。
- 状态按用户 ID 和引导版本保存，不使用跨用户的单一 localStorage key。
- 步骤使用稳定 ID，并区分未开始、进行中、完成、不适用、错误和权限不足。

目标 Full 版可包含渠道、额度和日志步骤；目标 Lite/Desktop 保留核心调用与安全流程，具体页面差异由 S6 能力矩阵决定，不能继续以 Legacy LAN 的极简路由替代。

### 6.3 额度趋势

概览页只承担快速判断，不复制详细分析工作台。固定展示剩余额度、最近区间速度、
最近分析窗口的每分钟/小时加权平均、预计耗尽或先重置状态、覆盖率和简洁折线；高级
控件及数据质量诊断只放在渠道页。

渠道详情页增加：

- 当前可用额度。
- 24 小时和 7 天变化。
- 最后采集时间和采集状态。
- 可用额度折线图。
- 失败断点、重置标记和触摸 tooltip。
- 1 小时、6 小时、24 小时、7 天、30 天、90 天和自定义范围。
- 服务端支持 `granularity=raw|minute|5m|15m|hour|day|week|auto` 与 `timezone_offset`（分钟）；前端默认按浏览器时区自动聚合。展示颗粒不改变采样间隔，完整契约见 [额度分析](QUOTA_ANALYTICS.md)。
- 服务端另支持 `rate_window` 与 `ewma_half_life`；速率方法为最后有效区间、有效观测时间
  加权平均和连续时间 EWMA。ETA 以 `analysis.as_of` 为分析基准，过去的预测不能冒充
  当前剩余时间；有未来 `reset_at` 时优先判断当前窗口是否会先重置。
- 手机端纯文本摘要和无图表降级视图。

渠道上游账户余额、MyAPI 用户余额和单个 API Key 限额必须使用不同标题，不能统称为“可用额度”。

2026-09-04 代码阶段收口：Codex 本机导入和额度分析整合提交 `6f8f85b` 已通过
[CI 33861244725](https://github.com/ForceMind/MyAPI/actions/runs/33861244725) 八项检查；
[Docker smoke 33862317233](https://github.com/ForceMind/MyAPI/actions/runs/33862317233)
在同一提交上完成 Full/LAN 两种 `push:false` 镜像、隔离 SQLite、认证和真实前端冒烟。
因此该批代码与自动化合同标记为已完成；真实账号长期刷新、真实管理员/手机视觉、桌面
安装、三数据库恢复及生产升级仍按外部验收处理，完整独立 UI 替换仍未启动。

## 7. Provider 支持路线

优先级和边界：

1. 先统一 OpenAI、DeepSeek、OpenRouter、Moonshot、SiliconFlow 等已有余额适配器。
2. Codex 只接入官方可验证的 usage/quota 接口，支持窗口额度时记录 `window_type` 和 `reset_at`。
3. Claude 只实现官方稳定 API 能力；没有公开账户余额接口时显示“不支持额度查询”。
4. Google Antigravity 不通过普通 Gemini 路由伪装实现，不读取本地凭据；只有官方稳定额度接口出现后才增加额度适配。
5. Advanced Custom 使用受限的 JSON 映射，不保存原始敏感响应。

## 8. Full、Lite、Desktop、安装形态与访问模式

Full、Lite、Desktop 的产品定位以第 1 节为准。下面的能力矩阵用于产品、后端门禁、安装器、界面和文档；它不能用当前 `MYAPI_EDITION=lan` 的实现替代。

| 范围 | Full | Lite | Desktop |
| --- | --- | --- | --- |
| 典型用户 | 团队/组织与完整治理 | 个人/小规模，服务器或个人电脑 | 希望本机安装和管理 Lite 的个人 |
| 核心业务 | 完整 | 可靠转发、鉴权、Key、日志、额度安全、必要备份恢复 | 与 Lite 相同 |
| 管理复杂度 | 完整用户、权限、订阅、支付与运维 | 精简默认流程；任何不提供能力须显示影响且保留历史数据 | 只增加安装、托盘、后台服务与 OS 集成 |
| 默认数据与运行 | 由部署拓扑决定 | SQLite-first、低依赖 | 平台 `userData`，同 Lite 模型 |
| 访问模式 | local/LAN/public 独立配置 | local/LAN/public 独立配置 | local/LAN/public 独立配置 |
| 指令学习与 Codex 集成 | 经授权支持 | 经授权支持 | 经授权本机支持 |

个人电脑 Lite/Desktop 初始只绑定本机。LAN 分享和公网开放是两个独立操作：公网向导检查监听、端口、权限、防火墙、NAT/CGNAT、IPv4/IPv6、域名/DNS/HTTPS、反向代理或用户选择的隧道，并以外部网络验证区分“已验证”“仍需配置”“不支持”“无法自动判断”。电脑休眠、合盖、关机、断网、退出后台服务、上行带宽和动态地址都会影响公网可用性，界面和平台教程必须如实说明。

现有 `lan` 配置在过渡期只作为 Legacy 映射：回环为 Lite/local，私网显式 `ALLOW_LAN=true` 为 Lite/lan，绝不自动推导 public。新旧字段冲突必须拒绝；不改 GHCR/NPM 地址、包名、tag 或用户数据，直到负责人决定兼容命名与弃用窗口。完整映射与发行约束见 [发行制品、安装与更新合同](RELEASE_MANIFEST.md)。

## 9. 发行、安装、更新与恢复

现有 CI 已能构建 Full 与 Legacy LAN OCI、原生后端和部分 Electron 产物；现有 CLI 只能执行有限 Docker 升级。另有 schema-1 Release Manifest 的纯结构校验/fresh-install 选择和只冻结未受信原始 bytes 的 evidence 合同；selector/installation state 会在反射前拒绝 Proxy、访问器、非 plain object/array 等 hostile JS 输入，但都不获取、解析或验签资产，不能安装或更新。它们是可保留基础，不代表统一发行、Lite 服务器、Desktop updater、数据库回退或产品切换已完成。

后续使用一个经验证的 `RELEASE_MANIFEST.json` 将 NPM 包、源码 SHA、Full/Lite OCI digest、原生二进制、Desktop 产物、签名、SBOM/provenance、安装器兼容、数据库/配置 schema 和回退条件绑定。NPM 包、Full/Lite 运行制品和 Desktop 共享同一版本清单，但安装器只保留已选形态的运行文件；安装、配置、数据、日志、暂存和回退备份必须分开存放。

安装、切换和更新必须有持久 journal 与单安装锁：预检 → 校验 → 暂存 → 备份 → 排空/维护 → 切换 → 健康/业务验证 → 提交 → 受控清理。失败或崩溃进入恢复、回退或人工处理状态；程序回退与数据库恢复明确分开。自动检查、自动下载、自动安装默认均关闭，且检查、下载、安装、维护窗口和更新范围分别配置。更新不得改变访问模式、用户授权、Key、额度、S5-P 数据或 Codex 文件。

NPM bootstrap 只能由用户通过包管理器更新；应用不能自行改写全局 NPM。Docker 只能管理已登记的 Compose 项目和卷，不能 `down -v` 或接管未知目录；Desktop 必须把窗口关闭、退出应用和停止后台服务区别展示，并在更新后按用户原有运行状态恢复。所有细节、目录和验收见 [发行制品、安装与更新合同](RELEASE_MANIFEST.md)。

CI 自动构建不等于自动重启生产服务。正式 NPM/GHCR/tag/发布、系统权限、外部网络配置和生产升级均保持单独授权。

## 10. NPM 和版本策略

正式发布前，除既有 `VERSION`、`package.json`、`SOURCE_MANIFEST.json`、测试与 tag 一致性检查外，还必须聚合并校验同一源码 SHA 的 `RELEASE_MANIFEST.json`、全部声明制品、hash/digest、签名和兼容矩阵。任何制品缺失或错误均不得标记为完整 release；未决定的 Lite 公开 OCI 坐标继续使用 Legacy 兼容记录，不静默创建新 namespace。

当前 `@forcemind/myapi` 尚未正式发布。未来统一入口可以通过交互式安装向导显示 Full 服务器、Lite 服务器、个人电脑 Lite 和 Desktop，但文档不能在实现/制品/签名/平台验证之前把设计命令写成已发布命令。正式发布仍需目标版本、tag、维护者确认和独立的 NPM/GHCR 发布授权。

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
- 额度告警配置（默认关闭；警告/严重阈值、重复通知冷却时间和恢复通知偏好均原子持久化、校验并只读展示；另有 P2A 纯 occurrence identity，以可信内部 channel/snapshot 引用和冷却周期生成 fail-closed key，不创建持久事件或发送外部通知；具体通道和凭据仍待业务决策）。

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

- 当前阶段策略：先完成后端、协议、安全、计费、迁移、Lite/Desktop 和发行代码；完整 UI
  替换暂缓，待代码合同稳定后再建立独立的信息架构、视觉系统和真实设备验收批次。
- 在 TokenHub 和原 New API 的参考研究基础上，完成 MyAPI 独立管理 UI；不复制任一项目的代码、页面、资产、文案或品牌。
- 静态官网和发行版选择页：Full、Lite、Desktop 及服务器/个人电脑安装选择，明确 local/LAN/public 是独立访问模式；基础交互、静态资源校验和 main 变更 artifact workflow 已完成，真实浏览器/移动视觉审查与绑定域名发布仍待完成。
- 统一 NPM 入口、Release Manifest、安装器所有权清单、服务器 Lite、个人电脑 Lite、Desktop 安装/升级/状态/回滚，以及中英文验证过的教程。
- 产品形态切换、手动/自动更新、持久 journal、数据库恢复边界、S5-P 数据保留与受限宿主集成。
- S5-P 的样本治理、调度/预算、版本中心、差异/导出、人工文件应用/回滚；真实日志、付费调用与宿主写入另行授权。

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
- Full/Lite/Desktop、管理员/普通用户及 local/LAN/public 状态差异。
- 中文、英文及其他支持语言。

### 运行和发行

- TypeScript 类型检查、前端测试和生产构建。
- Go 单元测试、格式检查和 API 回归。
- Full/Lite OCI、原生和 Desktop 与同一 Release Manifest 的版本、SHA/digest、签名和平台矩阵校验。
- Docker/原生/Desktop 安装、健康、清理、切换、更新中断、回退与三数据库恢复副本。
- 本机、LAN、公网状态准确性和外部验证；升级不得扩大访问范围。
- S5-P 数据、授权、预算、任务恢复和 Codex 应用备份保持正确。
- 不触碰生产数据的升级演练；实际设备、网络、签名与正式发布单独记录。

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
- `@forcemind/myapi` 是否演进为轻量安装入口、Release Manifest 的信任根，以及 Lite OCI 的长期兼容坐标。
- 原生安装的标准数据/配置/日志目录、system/user service、最低可升级版本、RPO/RTO 与数据库 downgrade 规则。
- Desktop 首发平台/架构、Linux Desktop 的正式支持条件和自动更新策略。
- S5-P 的用户/项目/Key scope、指令正文加密、允许的分析渠道/预算与历史样本导入范围。

## 15. 实施文件映射

以下是首轮实现时的模块边界。具体文件以实现前的现状检查为准，不因路线图而整体重写已经稳定的模块。

| 能力 | 后端/配置 | 前端 | 文档/测试 |
| --- | --- | --- | --- |
| 额度历史 | `model/`、`controller/channel-billing.go`、`router/channel-router.go`、迁移 | `web/src/features/channels/`、图表组件 | OpenAPI、模型/控制器测试、移动端测试 |
| Key 访问方案 | `model/token.go`、`model/user.go`、`controller/token.go`、设置项 | `web/src/features/keys/`、用户管理和设置页 | 多语言、兼容性和权限测试 |
| 设置引导 | `controller/setup.go`、系统状态接口 | `web/src/features/dashboard/components/overview/overview-dashboard.tsx`、`web/src/features/setup/` | 引导状态、角色、版本迁移测试 |
| 移动端日志 | 日志查询和完整内容日志控制器 | `web/src/features/usage-logs/`、`web/src/features/full-content-logs/` | Vitest、真实移动浏览器检查 |
| Provider 支持 | `relay/channel/codex/`、`relay/channel/claude/`、`relay/channel/gemini/`、额度适配器 | 渠道创建和状态页 | 官方接口 fixture、失败和安全测试 |
| Lite/Desktop 与访问模式 | `cli/myapi.mjs`、`deploy/`、edition/access resolver、受限安装管理器 | 安装、访问、公网向导、状态与托盘 | 服务器/个人电脑/桌面、端口、外部可达性与回滚演练 |
| 统一发行与更新 | `package.json`、`tools/npm/`、`tools/upgrade/`、`tools/release/`、Dockerfile、版本脚本 | 安装/更新/切换/备份页面、Electron 打包配置 | Release Manifest、签名、制品、journal、清理、三数据库恢复与发布检查 |
| S5-P 提示词学习 | `model/`、`service/`、`controller/`、专用 worker/授权/文件适配 | `web/src/features/prompt-learning/`、设置/版本/差异/授权页面 | 合成日志、预算/未知态、三数据库、文件安全、七语言与真实设备验收 |
| 品牌/官网 | 构建参数、元数据和部署变量 | `web/src/lib/build-branding.ts`、`website/` | 旧引用扫描、视觉审查、许可证审查 |
