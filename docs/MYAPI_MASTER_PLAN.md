# MyAPI 总体产品与工程计划

> 文档状态：执行基线与路线图
>
> 基线日期：2026-08-30
>
> 当前源码基线：`dc4b076`

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
- 参考 TokenHub 的架构思维和信息表达方式，不复制其代码、品牌、页面或协议。

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
- 最新镜像已在生产容器中重建，健康检查通过。

### 统计和品牌

- 用量分布支持分钟、小时、天、周，以及时区偏移和边界处理。
- Codex 网页 OAuth 登录入口已存在。
- 默认推广内容已精简。
- MyAPI 品牌运行时参数：`VITE_BRAND_NAME`、`VITE_BRAND_LOGO`、`MYAPI_BRAND_NAME`、`MYAPI_BRAND_LOGO`。
- Full/LAN 构建参数和 Logo 配置已接入 Docker 与部署脚本。

### TokenHub、Antigravity 和发行基础

- TokenHub 有独立边界文档和 provider-neutral 适配边界。
- Google Antigravity 当前只建立了官方 Gemini Interactions API 的兼容边界，不应宣传为完整 Antigravity 账户或额度支持。
- LAN Lite CLI、SQLite-first 项目初始化和局域网安全边界已存在。
- GitHub Actions 已支持 SemVer tag 构建并推送 Full/LAN GHCR 镜像。
- SemVer tag 可触发 macOS/Windows Electron 构建产物；发布上传需显式开启。

## 4. 当前未完成或仅有边界设计的工作

| 领域 | 当前状态 | 完成定义 |
| --- | --- | --- |
| 品牌和旧元数据清理 | 未完成 | 代码、文档、容器、环境变量、About、网站完成审计并通过合规检查 |
| 独立 UI 系统 | 部分完成 | 不依赖旧 New API 信息架构，Full/LAN/移动端完成真实画面审查 |
| TokenHub 风格静态官网 | 有基础目录，未完成 | 形成独立产品叙事、安装入口、发行版选择和安全说明 |
| 渠道额度历史 | 未开始 | 自动采集、历史查询、折线图、失败状态和保留策略可用 |
| 账户等级/Key 访问方案 | 未开始 | 用户账户等级和 Key 路由/计费策略在模型、API、UI 中分离 |
| 设置引导生命周期 | 未完成 | 完成后自动消失，按用户和版本保存，可主动重新打开 |
| Claude 支持 | 需按官方接口推进 | 只实现有明确官方协议的能力，不读取本地凭据 |
| Google Antigravity 完整能力 | 未完成 | 只有官方稳定接口存在时才实现；无接口时明确显示不支持 |
| LAN Lite 桌面体验 | CLI/镜像基础已有 | macOS/Windows 安装、升级、回滚、端口和防火墙提示完整 |
| GHCR 自动升级 | 构建已自动化 | 生产端显式拉取、健康检查、回滚和不破坏数据 |
| NPM 正式发布 | 未完成 | 版本、Tag、清单、测试和用户确认齐备后发布 |

## 5. 领域模型重构方向

### 5.1 账户等级与 Key 访问方案分离

当前 `User.Group` 和 `Token.Group` 共用同一批名称，导致 `default`、`vip` 既像账户等级又像路由策略。目标模型如下：

- **账户等级（Account Tier）**：描述用户身份、权益、可用功能、用户额度和可见模型。
- **Key 访问方案（Access Profile）**：描述一个 API Key 使用的路由池、计费倍率、模型范围和故障转移策略。
- **自动路由策略（Auto Routing Policy）**：定义多个访问方案的尝试顺序和跨方案重试。
- **渠道标签（Channel Tag）**：描述渠道路由归属，不作为用户账户名称。

迁移策略：

1. 第一阶段只改 UI、说明和返回的展示元数据，保留旧 `group` 字段兼容。
2. 第二阶段增加访问方案显示名、描述、倍率、可选范围和稳定标识。
3. 第三阶段再评估 `access_profile_id` 独立字段，旧 `group` 作为兼容回退。

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
CHANNEL_QUOTA_SYNC_ENABLED=true
CHANNEL_QUOTA_SYNC_INTERVAL=15m
CHANNEL_QUOTA_HISTORY_RETENTION_DAYS=180
CHANNEL_QUOTA_MAX_POINTS=2000
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

当前首页在完成所有步骤后仍渲染“设置引导已完成”卡片，这是错误的生命周期设计。目标规则：

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
- 完成 Full/LAN 镜像健康检查和升级回滚说明。
- 完成品牌、旧名称、旧链接、版权头和环境变量专项审计。
- 补齐真实手机浏览器验证。

### P1：账户和管理体验

- 账户等级与 Key 访问方案拆分。
- 访问方案配置和清晰说明。
- 创建 Key 表单重做。
- 设置引导完成后自动消失。
- 引导状态按用户和版本隔离。
- 多语言文案和移动端回归。

### P2：额度观测

- 渠道额度快照表和迁移。
- 手动/自动采集接入。
- 历史查询 API、聚合和权限。
- 渠道详情折线图和移动端降级视图。
- 采集失败、重置和不支持状态。

### P3：Provider 和预警

- Codex 官方 usage/quota 适配。
- Claude 官方接口评估和适配。
- Antigravity 官方接口出现后再实现。
- 额度阈值、下降速度和预计耗尽预警。

### P4：独立产品体验

- TokenHub 思路启发下的 MyAPI 独立管理 UI。
- 静态官网和发行版选择页。
- LAN Lite macOS/Windows 安装、升级、状态和回滚。
- GHCR 版本拉取自动化。

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
