# MyAPI 完整请求/返回日志

> 本文档适用于本仓库的 **MyAPI** 自建发行版。MyAPI 是运行时发行品牌；日志接口、
> 数据格式和 OpenAI-compatible/Responses 等技术协议保持兼容。许可证与第三方法律
> 通知仍以仓库中的 LICENSE、NOTICE 和依赖声明为准。

## 当前状态

当前生产容器已经启用完整内容日志：

```text
镜像：ghcr.io/forcemind/myapi:v0.1.1
容器：my-api
日志目录：<my-api-project>/logs/full-content/
日志格式：JSON Lines（每行一个 JSON 对象）
```

记录范围是经过身份验证的 AI Relay 接口，包括 OpenAI Chat/Responses、Gemini、Claude、Suno、Midjourney 和视频相关接口。管理后台、登录接口、健康检查以及身份验证失败的请求不会写入完整内容日志。

## 在管理后台查看

使用管理员账号登录后，在左侧“管理”菜单点击“API 请求日志”，或直接访问：

```text
https://myapi.example.com/full-content-logs
```

页面支持：

- 按时间、模型、API Key 名称或 ID、request_id 筛选；
- 从已有日志中直接选择模型和 API Key，不需要记忆 ID；
- 查看请求和返回的字节数、分片数、状态码与耗时；
- 点击任意请求行或固定在右侧的“查看内容”，在线打开详情；
- 详情默认使用“纯文本”模式，只显示角色文本、用户输入和模型回答；
- 可切换到“原始数据”模式，查看查询参数、请求头、JSON 包装和完整 SSE；
- 纯文本模式支持 Chat Completions、Responses、Claude、Gemini 及常见流式增量格式；
- 在线查看脱敏后的查询参数、请求头、完整请求正文和响应头；
- 自动按 sequence 拼接流式响应并查看完整返回；
- 复制请求或返回正文；
- 下载原始 JSONL 文件；
- 删除单个日志文件或清空全部完整内容日志。

页面和 `/api/full-content-logs` 管理接口均强制管理员鉴权。普通用户直接输入地址也无法读取日志。

## 每个请求如何记录

同一个请求的所有日志拥有相同的 `request_id`，按以下阶段记录：

1. `request`：客户端发给 MyAPI 的脱敏查询参数、请求头和完整请求正文；
2. `response_chunk`：MyAPI 实际返回客户端的每个分片；
3. `response_end`：最终 HTTP 状态、响应总字节数、分片数和耗时。

流式响应不会只记录最终文字，而是保留发送给客户端的所有 SSE 分片。按照 `sequence` 排序并拼接 `body`，可以还原客户端收到的完整响应。

文本按 UTF-8 保存，非 UTF-8 二进制内容按 Base64 保存，并在 `encoding` 中标记为 `base64`。因此 multipart 附件和二进制返回也不会被静默丢弃。

## 安全处理

完整日志可能包含提示词、模型回复和附件，请只允许管理员读取。

当前实现的安全边界：

- 记录 HTTP 请求头和响应头，`Authorization`、Cookie、API Key 等敏感字段的值统一替换为 `[REDACTED]`；
- 记录 URL Query，但敏感查询参数的值统一替换为 `[REDACTED]`；
- 管理界面按 API Key 的名称和数据库 ID 分类，不会显示完整 Key；
- JSON 请求正文中以下字段会递归替换为 `[REDACTED]`：
  `api_key`、`x_api_key`、`key`、`authorization`、`proxy_authorization`、`x_auth_token`、`cookie`、`set_cookie`、`token`、`access_token`、`refresh_token`、`password`、`secret`、`client_secret`、`session_secret`、`jwt`；
- 日志目录权限强制为 `0700`；
- 日志文件权限强制为 `0600`；
- 日志写入失败不会中断正常 API 请求。

注意：模型回复本身按原样记录。如果模型主动输出了某个秘密，该内容仍会出现在响应日志中。

## Docker Compose 配置

生产配置位于部署项目的 `docker-compose.yml`（以下仅为片段）：

```yaml
environment:
  FULL_CONTENT_LOG_ENABLED: "true"
  FULL_CONTENT_LOG_MAX_MB: "100"
  FULL_CONTENT_LOG_MAX_FILES: "10"
```

变量含义：

- `FULL_CONTENT_LOG_ENABLED`：是否启用，设为 `true` 才记录；
- `FULL_CONTENT_LOG_DIR`：可选，容器内日志路径；未设置时使用 `/app/logs/full-content`；
- `FULL_CONTENT_LOG_MAX_MB`：单个 JSONL 文件最大容量；当前为 100 MiB，达到后创建新文件；设为 `0` 表示单文件不限大小；
- `FULL_CONTENT_LOG_MAX_FILES`：最多保留的日志文件数；当前为 `10`，设为 `0` 才表示不自动删除任何旧文件。

所有 Relay 路由会复用同一个进程级 writer，因此轮转是全局的，不会误删其他路由仍在写入的文件。当前配置约为 1 GiB 软上限；单条超大记录可能暂时超过该值。

## 常用检查命令

检查服务和日志文件：

```bash
cd /path/to/my-api
docker compose ps
find logs/full-content -maxdepth 1 -type f \
  -printf '%TY-%Tm-%Td %TH:%TM:%TS %m %s %p\n' | sort
```

查看最后 20 条记录：

```bash
tail -n 20 logs/full-content/full-content-*.jsonl
```

只看请求和结束摘要（安装了 `jq` 时）：

```bash
jq -c 'select(.phase == "request" or .phase == "response_end")' \
  logs/full-content/full-content-*.jsonl
```

按 request_id 查看一次调用的全部记录：

```bash
REQUEST_ID='把 request_id 填在这里'
jq -c --arg request_id "$REQUEST_ID" \
  'select(.request_id == $request_id)' \
  logs/full-content/full-content-*.jsonl
```

还原某次 UTF-8 流式响应：

```bash
REQUEST_ID='把 request_id 填在这里'
jq -j --arg request_id "$REQUEST_ID" \
  'select(.request_id == $request_id and .phase == "response_chunk") | .body' \
  logs/full-content/full-content-*.jsonl
```

如果某个分片的 `encoding` 是 `base64`，需要对该分片的 `body` 做 Base64 解码，不能直接用上面的 UTF-8 拼接命令。

旧日志是在请求头和查询参数采集功能上线前生成的，因此其详情中的这两个区域可能显示“未记录”。新请求会正常显示。

## 已完成验证（2026-08-28）

- 中间件、控制器与路由测试通过；
- 前端类型检查、Lint 和 12 项日志页面测试通过；
- 容器健康状态为 `healthy`；
- 管理 API 返回 21 个请求、2 个模型和 2 个 API Key 筛选维度；
- 在线详情确认包含请求正文、完整返回、请求头、响应头和查询参数字段；
- 未登录读取日志 API 返回 HTTP 401；
- 真实 `POST /v1/chat/completions` 流式请求返回 `CHAT_LOG_OK`；
- 真实 `POST /v1/responses` 流式请求返回 `RESPONSES_LOG_OK`；
- Chat 的 14 个分片、2,478 字节可从日志逐字节还原；
- 记录了 52 个响应分片，共 6,668 字节；
- 从日志重建的响应与客户端实际响应逐字节相同；
- JSON 正文中的 `api_key` 和 `password` 未泄漏；
- 部署者应在自己的公开地址执行 `/api/status` 健康检查；本文不记录任何生产域名或
  生产请求正文。

## 停用日志

修改部署项目中的 `docker-compose.yml`：

```yaml
FULL_CONTENT_LOG_ENABLED: "false"
```

然后执行：

```bash
cd /path/to/my-api
docker compose up -d
```

停用不会删除已有日志。

## 回滚本次部署

部署前备份目录（示例）：

```text
/path/to/my-api/backups/full-content-logging/
```

回滚配置并重建旧容器：

```bash
cp /path/to/my-api/backups/full-content-logging/docker-compose.yml.before \
  /path/to/my-api/docker-compose.yml
cd /path/to/my-api
docker compose up -d
docker compose ps
```

旧镜像仍保留为：

```text
local/my-api:custom-rc25-before-logging
```

数据库没有因为本功能增加新表或修改结构，通常不需要回滚数据库。备份目录中仍保存了部署前的 `one-api.db.before`，只应在确认需要数据库级回滚并停止服务后使用。

## 源码位置

- 日志中间件：`middleware/full_content_logger.go`
- 中间件测试：`middleware/full_content_logger_test.go`
- 日志管理 API：`controller/full_content_log.go`
- 管理页面：`web/src/features/full-content-logs/`
- 页面路由：`web/src/routes/_authenticated/full-content-logs/index.tsx`
- Relay 路由挂载：`router/relay-router.go`
- 视频路由挂载：`router/video-router.go`
