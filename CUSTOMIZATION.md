# New API rc.25 定制版

本仓库保存当前生产环境实际运行的完整源码，同时保留按顺序应用的补丁文件，便于在新机器重新构建或迁移。

上游项目、许可证、版权和署名均保持原样。本仓库中的定制代码继续遵守仓库根目录中的 AGPL-3.0 许可证及原项目许可要求。

## 已集成功能

### 1. Codex / Chat Completions 兼容

- Chat Completions 非流式请求可以转为 Codex Responses SSE 上游请求；
- 自动识别缺少 `Content-Type` 的 SSE；
- 将上游流式结果缓冲为标准 Chat Completions 非流式 JSON；
- Chat 流式请求继续返回标准 Chat delta SSE。

对应历史补丁：`patches/01-codex-chat-compat.patch`。

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

## 两种使用方式

### 直接使用完整源码（推荐）

本仓库根目录就是可构建的完整源码，无需再应用补丁：

```bash
docker build -t local/new-api:custom-rc25 .
```

完整部署步骤见 `DEPLOYMENT_CUSTOM.md`。

### 在同版本原始源码上依次应用补丁

仅当目标源码与本定制版的 rc.25 基线一致时使用：

```bash
patch -p1 < patches/01-codex-chat-compat.patch
patch -p1 < patches/02-chat-attachment-compat.patch
patch -p1 < patches/03-full-content-log-explorer.patch
```

上游文件发生变化后补丁可能产生冲突。升级 New API 时，推荐把这三个补丁作为迁移清单逐项移植，并重新运行全部测试，而不是强制应用失败的补丁。

## 主要测试

```bash
go test ./middleware ./controller ./router ./relay ./relay/channel/openai ./relay/channel/codex

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
- `FULL_CONTENT_LOG_MAX_FILES=0` 表示日志永久保留，磁盘占用会持续增长；
- 日志正文仍可能包含用户输入、模型输出和附件，仅允许管理员访问；
- 完整 API Key 不会显示在日志界面，界面只按 Key 名称和数据库 ID 分类。
