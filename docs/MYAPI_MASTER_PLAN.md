# MyAPI 总体产品与工程计划

> 文档状态：执行基线与路线图
>
> 基线日期：2026-08-30
>
> 当前源码基线：本分支 `main` 最新提交（后续阶段性提交以 Git 历史为准）

这份文档把 MyAPI 的产品目标、已完成能力、未完成工作、发行方式、UI 方向、自动化和验收规则统一起来。它是路线图和交付索引，不把设计目标误写成已经实现的功能。

关联规范：

- [发行版说明](./MYAPI_DISTRIBUTION.md)
- [LAN Lite](./LAN_LITE.md)
- [TokenHub 集成边界](./TOKENHUB_INTEGRATION.md)
- [Google Antigravity 边界](./ANTIGRAVITY_INTEGRATION.md)
- [完整内容日志](./FULL_CONTENT_LOGGING_CUSTOM.md)
- [认证和 Cookie 安全](./authentication.md)
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
- 生产容器重建与健康检查尚未在本轮执行；需由部署方按 `docs/UPGRADE_REHEARSAL.md` 完成副本验证并明确批准后再操作。

### 统计和品牌

- 用量分布支持分钟、小时、天、周，以及时区偏移和边界处理。
- Codex 网页 OAuth 登录入口已存在。
- 默认推广内容已精简。
- MyAPI 品牌运行时参数：`VITE_BRAND_NAME`、`VITE_BRAND_LOGO`、`MYAPI_BRAND_NAME`、`MYAPI_BRAND_LOGO`。
- Full/LAN 构建参数和 Logo 配置已接入 Docker 与部署脚本。
- API Key 已开始返回兼容旧 `group` 的访问方案元数据，创建和列表 UI 已显示“Access profile”及用途说明；`GET /api/user/self/groups` 另返回独立的 `account_tier` 元数据，Key 创建时会同时解释“账户等级”和“访问方案”的边界。
- 设置引导已按用户和版本隔离；完成后自动移除引导卡片，不再显示“设置引导已完成”或重复打开入口。
- 渠道余额对话框已接入额度历史折线图，支持 24h/7d/30d/90d、自定义日期范围、自动/raw/hour/day/week 聚合、浏览器时区偏移、加载/失败/空数据、多 Key 解释和失败采样断点；手动刷新会使趋势查询失效并重新读取。快照可通过 `CHANNEL_QUOTA_SNAPSHOT_RETENTION_DAYS` 启用每日限批清理。
- Codex OAuth 渠道的 Account Info 对话框已增加“当前窗口 / 历史趋势”切换；历史只保存官方 WHAM usage 响应中规范化的 primary/secondary 使用百分比、窗口和重置时间，不保存原始响应或凭据。启用 `CHANNEL_QUOTA_SYNC_ENABLED` 后，后台有界采样任务会按渠道锁调用同一官方 usage 接口并记录成功/失败状态；未启用时仍可由管理员查询当前 Codex 用量产生首个样本。
- 概览页和管理员渠道页已增加“账户额度变化”面板，共用 `GET /api/channel/quota/changes` 聚合接口；按渠道/上游账户、指标和窗口分组，以相邻有效快照计算带符号的每分钟变化，并默认按绝对变化最大值排序。`GET /api/channel/quota/status` 只读返回采样开关、间隔和每轮上限，帮助空态解释部署配置。跨重置边界、失败或不足样本不会伪造变化值；面板只显示脱敏后的渠道名称和额度数值，不包含凭据或原始响应，移动端使用纵向卡片布局。
- 渠道类型选择已将 Codex 置首，并在 Codex、Claude、Gemini/Antigravity 入口显示实际能力边界。

### TokenHub、Antigravity 和发行基础

- TokenHub 有独立边界文档和 provider-neutral 适配边界；后续 UI 和官网只借鉴其产品叙事与信息组织，不复制实现或页面。
- Google Antigravity 当前只建立了官方 Gemini Interactions API 的兼容边界，不应宣传为完整 Antigravity 账户或额度支持。
- LAN Lite CLI、SQLite-first 项目初始化和局域网安全边界已存在。
- Electron 桌面版默认回环监听、单实例和持久会话密钥已加固；`--allow-lan` 加私网绑定地址才可共享，并在托盘菜单显示生效端点。
- GitHub Actions 已支持 SemVer tag 构建并推送 Full/LAN GHCR 镜像。
- `myapi upgrade` CLI 已支持按发行版拉取 GHCR 镜像、可选 cosign 签名校验、环境文件备份、健康等待和失败回滚；生产启用仍需人工审批与数据备份演练。
- SemVer tag 可触发 macOS/Windows Electron 构建产物并生成 SHA256 校验和；发布上传需显式开启。
- 静态官网已补齐移动菜单关闭、outside-click、Escape、焦点回归与 Tab 约束、主题偏好持久化等基础交互，并加入资源/结构自动校验 workflow；真实浏览器/移动视觉审查与独立发布 workflow 仍待完成。

## 4. 当前未完成或仅有边界设计的工作

| 领域 | 当前状态 | 完成定义 |
| --- | --- | --- |
| 品牌和旧元数据清理 | 审计清单已建立 | About 默认态、PNG/ICO 资产已切换；`docs/BRAND_AUDIT.md` 区分必须替换、兼容保留和法律保留项，NOTICE/源码头部仍需合规审查 |
| 独立 UI 系统 | 部分完成 | 不依赖旧 New API 信息架构，Full/LAN/移动端完成真实画面审查 |
| TokenHub 风格静态官网 | 交互与审阅制品已实现 | 形成独立产品叙事、安装入口、发行版选择、安全说明、响应式菜单无障碍/主题交互、静态资源自动校验和 main 变更自动生成的确定性 artifact workflow；真实浏览器/移动视觉审查与绑定域名的独立发布仍待完成 |
| 渠道额度历史 | 后端和前端初版已实现 | 普通渠道与 Codex OAuth 渠道均已有历史查询、折线图、失败状态、可选保留清理和只读健康指标；普通渠道与 Codex OAuth 均支持可选、有界后台采样；告警阈值已支持默认关闭、原子持久化和只读状态展示，通知/去重仍待业务决策 |
| 账户额度变化聚合 | 初版已实现 | 概览和管理员渠道页均可查看每分钟变化及最大变化排序；普通渠道与 Codex OAuth 后台采样已接入系统任务并避免与旧轮询重复，跨账户订阅账单同步和通知仍待后续迭代 |
| 账户等级/Key 访问方案 | 显式兼容字段已实现 | 管理员界面与 `model.ResolveAccessProfile/ResolveAccountTier` 已明确两者语义；`users.account_tier_id` 与 `tokens.access_profile_id` 已增量迁移并由旧 `group` 派生，旧路由保持兼容；独立可配置策略仍待后续阶段 |
| 设置引导生命周期 | 初版已实现 | 完成后自动消失、按用户和版本保存；真实多设备视觉审查仍待完成 |
| Claude 支持 | 需按官方接口推进 | 只实现有明确官方协议的能力，不读取本地凭据 |
| Google Antigravity 完整能力 | 未完成 | 只有官方稳定接口存在时才实现；无接口时明确显示不支持 |
| LAN Lite 桌面体验 | 安全状态体验已实现 | Electron 默认回环、单实例、持久会话密钥、显式 `--allow-lan` 私网绑定、只读 LAN 状态/防火墙提示、有效地址健康检查和托盘确认后重启切换已补齐；跨平台安装演练和系统防火墙自动配置仍待完成 |
| GHCR 自动升级 | CLI 预检与执行流程已实现 | 生产端显式拉取、可选签名验证、健康检查、环境备份和失败回滚已有；`upgrade --dry-run --json` 可在副本上无写入预检，`docs/UPGRADE_REHEARSAL.md` 已补充恢复演练清单，真实数据库恢复和人工审批仍待完成 |
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
2. 第二阶段增加访问方案显示名、描述、倍率、可选范围和稳定标识（当前内置方案已完成，管理员可配置策略仍待后续阶段）。
3. 第三阶段已增加 `users.account_tier_id` 与 `tokens.access_profile_id` 独立字段，并在启动迁移中从旧 `group` 幂等回填；旧 `group` 继续作为兼容回退。下一阶段再将独立策略配置接入路由与管理 UI。

创建 Key 的界面不再只显示 `Group`，而显示“Key 访问方案”，每个选项必须展示用途、计费倍率、路由范围和是否可用。

### 5.2 渠道额度历史

现有 `Channel.Balance` 继续表示当前缓存值；新增 `channel_quota_snapshots` 记录历史采样。历史记录至少包含：

- `channel_id`、`observed_at`。
- 可用、已用、总量和单位/币种。
- `metric_type`：余额、订阅额度或速率限制。
- `window_type`：无窗口、5 小时、日、月或自定义窗口。
- `reset_at`、来源、成功/失败/不支持状态。

采集规则：

- 复用现有手动和自动余额查询流程。
- 默认按 15 分钟采集，使用已有渠道轮询锁并限制并发。
- 失败不覆盖最后一个有效余额，但记录失败状态。
- 不记录上游完整响应。
- 多密钥渠道在 MVP 中不合并不同 Key 的额度。
- 查询服务端聚合和降采样，默认最多返回 2000 个点。

建议配置：

```env
# Background sampling is opt-in; set true only when collection is wanted.
CHANNEL_QUOTA_SYNC_ENABLED=false
CHANNEL_QUOTA_SYNC_INTERVAL=15m
CHANNEL_QUOTA_SNAPSHOT_RETENTION_DAYS=180
CHANNEL_QUOTA_MAX_POINTS=2000
# Optional read-only threshold status (disabled by default). This only adds
# `data.alert` to quota history responses when the provider reports a total;
# it never sends notifications, disables channels, or changes routing.
CHANNEL_QUOTA_ALERT_ENABLED=false
CHANNEL_QUOTA_ALERT_WARNING_PERCENT=20
CHANNEL_QUOTA_ALERT_CRITICAL_PERCENT=10
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

设置引导按用户与版本记录完成状态；完成所有步骤后应自动移除引导卡片，不能继续渲染“设置引导已完成”提示，也不重复打开已完成入口。当前实现已覆盖基础生命周期，仍需完成真实多设备视觉审查。

- 未完成：按发行版和角色显示相关步骤。
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
- 24 小时、7 天、30 天、90 天和自定义范围。
- 服务端支持 `granularity=raw|hour|day|week|auto` 与 `timezone_offset`（分钟）；前端默认按浏览器时区自动聚合。
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
- 补齐真实手机浏览器验证。

### P1：账户和管理体验

- 账户等级与 Key 访问方案拆分（显式兼容字段、回填迁移和语义测试已完成；独立可配置策略待后续阶段）。
- 访问方案配置和清晰说明（初版元数据和 Key 创建说明已完成）。
- 创建 Key 表单重做（初版已完成，仍需完整角色/权限体验审查）。
- 设置引导完成后自动消失（已完成）。
- 引导状态按用户和版本隔离（已完成）。
- 多语言文案和移动端回归（代码测试已完成，真实设备视觉审查待完成）。

### P2：额度观测

- 渠道额度快照表和迁移（初版已完成）。
- 手动/自动采集接入（初版已完成，失败状态已记录；新增可选、有界系统任务采样，并与旧轮询互斥）。
- 历史查询 API、基础 summary 和权限（初版已完成；已补聚合/时区参数）。
- 渠道详情折线图、移动端降级视图和只读额度健康指标（代码初版已完成；真实设备审查与通知策略待完成）。
- 采集失败、重置和不支持状态。
- 额度告警配置（默认关闭；警告/严重阈值原子持久化、校验和只读展示已完成；通知外发与去重待决策）。

### P3：Provider 和预警

- Codex 官方 usage/quota 适配。
- Claude 官方接口评估和适配。
- Antigravity 官方接口出现后再实现。
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

官方文档核验（2026-08-30）：Anthropic 的 Claude Platform 文档公开了组织级
spend/rate limit 与 Console 管理入口，但未提供可供普通渠道凭据直接轮询的余额
端点；Google 的 [Antigravity agent 文档](https://ai.google.dev/gemini-api/docs/antigravity-agent)
将其定义为 Gemini Interactions API 上的 preview agent。MyAPI 因此继续保留 Claude
Messages 转发和 Antigravity 请求边界，不把 Console 限额或 interaction 响应推断成
账户余额；待官方稳定、可授权的额度 API 后再实现采样。

### P4：独立产品体验

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
