# MyAPI rc.25 定制发行版

> **MyAPI 发行提示：** 本仓库同时提供以 MyAPI 为发行名称的自建 CLI、Logo 和
> 完整源码包。下面的功能说明与补丁清单适用于 MyAPI 发行版；许可证、NOTICE、
> 依赖许可证和法律要求的第三方通知按项目规则保留。新机器部署可先阅读
> [`docs/MYAPI_DISTRIBUTION.md`](docs/MYAPI_DISTRIBUTION.md)。

本仓库保存当前发行版源码基线，同时保留按顺序应用的补丁文件，便于在新机器重新构建或迁移；不要据此推断生产环境已经同步或重建。

本仓库中的定制代码继续遵守仓库根目录中的 AGPL-3.0 许可证及适用的第三方许可要求。
MyAPI 是用户可见的发行品牌；机器安全 slug 使用 `my-api`。API、SSE、数据库表和
字段等技术协议保持兼容，品牌迁移不会改变客户端请求契约。

## 已集成功能

### 1. Codex / Chat Completions 兼容

- Chat Completions 非流式请求可以转为 Codex Responses SSE 上游请求；
- 自动识别缺少 `Content-Type` 的 SSE；
- 将上游流式结果缓冲为标准 Chat Completions 非流式 JSON；
- Chat 流式请求继续返回标准 Chat delta SSE；
- 转发到 Codex 前自动移除不受支持的 `max_output_tokens`、`temperature`、`top_p`、`frequency_penalty` 和 `presence_penalty`，避免游乐场默认参数导致 400。

对应历史补丁：`patches/01-codex-chat-compat.patch` 和 `patches/05-codex-strip-unsupported-top-p.patch`。

### 2. Chat 附件兼容

- 将 Chat 文件内容转换为 Responses 的扁平 `input_file`；
- 支持 `filename`、`file_name`、`file_data` 和 `file_id`；
- 避免生成上游不接受的嵌套 `input_file.file`。

对应历史补丁：`patches/02-chat-attachment-compat.patch`。

### 3. 完整请求/返回日志

- 记录 Relay 请求正文、查询参数、请求头、响应头和每个响应分片；
- 自动脱敏 Authorization、Cookie、API Key、密码和 Token 等敏感字段；
- 支持 UTF-8、Base64 二进制和 SSE；
- 管理员 API 支持按时间、API Key、模型和 request ID 查询；
- 管理后台提供在线请求日志浏览器、详情、下载和删除功能；
- 日志详情默认显示清洗后的对话纯文本；
- 可切换为原始数据，检查请求头、参数、JSON 和 SSE 协议；
- 纯文本清洗支持 OpenAI Chat Completions、Responses、Claude、Gemini 及常见 SSE 增量格式。

对应补丁：`patches/03-full-content-log-explorer.patch`。

### 4. Codex 网页登录

- 在 Codex 渠道的“凭据”区域提供“使用 ChatGPT 登录”按钮；
- 登录流程在浏览器完成，不需要在服务器执行 `codex login`；
- 使用 OpenAI Codex CLI 同款 OAuth 客户端、授权端点、PKCE 和 localhost 回调；
- 浏览器跳转到 `http://localhost:1455/auth/callback` 后，即使页面打不开，也只需复制地址栏中的完整 URL 并粘贴回对话框；
- 新建渠道时，生成的 OAuth JSON 会自动填入 Key 字段；
- 编辑已有渠道时，新凭据会由后端直接写入渠道，不在响应中返回完整 JSON；
- OAuth state 与 PKCE verifier 存储在服务端 `auth_flows` 表，绑定当前管理员登录会话，10 分钟过期且只能消费一次；
- 登录、完成和刷新接口均受管理员渠道敏感写权限保护；
- access token、refresh token 和 OAuth JSON 不写入应用日志。

### 5. 精简默认推广内容

- 移除默认主页中的 Cherry Studio、CC Switch 和“更多应用”推广卡片；
- 演示模式页脚不再内置社区、文档和相关项目导流列；
- 默认第三方客户端一键导入列表为空；
- 管理员仍可在系统设置中按需配置自己的页脚列和客户端入口；
- 保留 AGPL 许可证、版权和法律要求的第三方通知；发行层页面统一使用 MyAPI。

对应补丁：`patches/04-codex-oauth-minimal-ui.patch`。

### 6. 游乐场按通道显示参数能力

- 用户模型接口返回当前分组中模型优先通道的游乐场参数限制；
- Codex 模型会自动将 `temperature`、`top_p`、`max_tokens`、`frequency_penalty` 和 `presence_penalty` 显示为关闭并禁用；
- 切换到受限模型时弹出一次提示，参数面板内持续显示 Codex 不支持的参数列表；
- 切回支持这些参数的模型后恢复用户原有开关偏好；
- 请求构建使用界面显示的有效参数状态，不会继续发送已禁用参数；
- 后端 Codex 适配器仍保留参数过滤，作为外部客户端和旧前端的安全兜底。

对应补丁：`patches/06-playground-codex-parameter-capabilities.patch`。

### 7. Full 与 LAN Lite 构建及有限日志轮转

- Docker 构建通过 `MYAPI_EDITION=full|lan` 选择发行版，Full 默认不启用精简前端，LAN Lite 启用 `VITE_SELF_USE_MINIMAL=true`；
- 保留渠道、模型、Key、游乐场、用量日志、完整内容日志、系统设置、登录/OAuth、初始化和 About；
- 不构建充值、订阅、兑换码、公开定价、排行榜、用户管理和内置聊天路由；
- 前端只打包简体中文和英文；
- 移除会连带打包 Ant Design、Mermaid 等组件的完整 Provider 图标依赖，以及未使用的图表、画布、轮播和可调整面板组件；
- 运行层从 Debian 改为 Alpine，同时保留许可证与第三方声明；
- 所有 Relay 路由复用同一个完整内容日志 writer，安全启用 10 文件轮转；
- Docker 标准输出日志限制为 10 MiB × 3 文件。

## 两种使用方式

### 直接使用完整源码（推荐）

本仓库根目录就是可构建的完整源码，无需再应用补丁：

```bash
docker build -t local/my-api:custom-rc25 .
```

完整部署步骤见 `DEPLOYMENT_CUSTOM.md`。

### 在同版本原始源码上依次应用补丁

仅当目标源码与本定制版的 rc.25 基线一致时使用：

```bash
patch -p1 < patches/01-codex-chat-compat.patch
patch -p1 < patches/02-chat-attachment-compat.patch
patch -p1 < patches/03-full-content-log-explorer.patch
patch -p1 < patches/04-codex-oauth-minimal-ui.patch
patch -p1 < patches/05-codex-strip-unsupported-top-p.patch
patch -p1 < patches/06-playground-codex-parameter-capabilities.patch
```

兼容基线文件发生变化后补丁可能产生冲突。升级时，推荐把这六个补丁作为迁移清单
逐项移植，并重新运行全部测试，而不是强制应用失败的补丁。

## 主要测试

```bash
go test ./service ./middleware ./controller ./model ./router ./relay ./relay/channel/openai ./relay/channel/codex

cd relaykit
GOWORK=off go test ./relayconvert/internal/oai_chat ./dto
GOWORK=off go build ./...

cd ../web
bun install --frozen-lockfile
bun run typecheck
bun run test
bun run build
```

## 重要安全说明

- 不要提交 `.env`、数据库、API Key、OAuth JSON 或生产日志；
- 默认 `FULL_CONTENT_LOG_MAX_FILES=10`，配合 100 MiB 单文件形成约 1 GiB 软上限；设为 `0` 才表示永久保留；
- 日志正文仍可能包含用户输入、模型输出和附件，仅允许管理员访问；
- 完整 API Key 不会显示在日志界面，界面只按 Key 名称和数据库 ID 分类。
- Codex OAuth 登录只支持真实的浏览器管理员会话，不接受 PAT 代替登录会话；
- OAuth 回调 URL 含短期授权码和 state，不要发送给其他人；
- ChatGPT 订阅、工作区权限和数据处理规则仍由 OpenAI 账户策略决定。
