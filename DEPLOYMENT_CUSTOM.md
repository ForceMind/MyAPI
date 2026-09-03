# 定制版部署说明

> 本文适用于 **MyAPI** 自建发行版（rc.25 兼容基线）。MyAPI CLI 的初始化、配置、
> 数据接管与安全检查请参阅 [`docs/MYAPI_DISTRIBUTION.md`](docs/MYAPI_DISTRIBUTION.md)。
> 新部署使用 `my-api` 与 `MYAPI_*` 命名；旧部署的目录、变量和挂载点只作为迁移输入，
> 不应继续写入新的配置。

## 环境要求

- 服务器部署：Linux、Docker 与 Docker Compose v2；
- macOS/Windows 开发或 LAN Lite：Docker Desktop 与 Compose v2.17+；
- 一个反向代理或 Cloudflare Tunnel（可选）；
- 只需要对外代理 MyAPI 的 `3000` 端口。

如果你的主要开发设备是 Mac，请先阅读
[`docs/DEVELOPMENT_ON_MACOS.md`](docs/DEVELOPMENT_ON_MACOS.md)。它说明源码克隆位置、
Homebrew/Bun/Go/Node、Docker Desktop、Electron、LAN Lite 和服务器数据隔离；不要把
本页的服务器 `.env` 或数据目录复制到开发机。

## 新机器安装

```bash
git clone https://github.com/ForceMind/MyAPI.git my-api
cd my-api

cp deploy/.env.example deploy/.env
```

编辑 `deploy/.env`：

- 设置随机的 `SESSION_SECRET`；
- 将 `MYAPI_PUBLIC_URL` 改为实际 HTTPS 域名；
- 如需修改宿主机端口，修改 `MYAPI_PORT`。
- `MYAPI_BRAND_NAME` 和 `MYAPI_BRAND_LOGO` 为可选的构建默认品牌，默认分别为
  `MyAPI` 和 `/myapi-logo-v1.png`；运行时站点设置中的自定义值优先。

然后执行：

```bash
chmod +x deploy/install.sh
./deploy/install.sh
```

脚本默认拉取 GitHub Actions 发布到 GHCR 的版本镜像、创建数据与日志目录、启动容器并显示健康状态。
只有设置 `MYAPI_BUILD_LOCAL=true` 或使用 `local/...` 镜像名时，脚本才会从源码构建。

### 命名迁移边界

发行层命名迁移只涉及宿主机目录、compose 服务/容器名和部署变量：

| 旧部署概念 | 新部署写法 | 说明 |
| --- | --- | --- |
| 镜像/服务名 | `ghcr.io/forcemind/myapi:<version>` / `my-api` | 镜像由 GitHub Actions 生成；升级时将 `MYAPI_IMAGE` 设为已发布标签，再按需重建容器。 |
| 公开地址变量 | `MYAPI_PUBLIC_URL` | 新配置不再新增旧前缀变量；迁移脚本可暂时读取旧值。 |
| 镜像、端口、数据、日志变量 | `MYAPI_IMAGE`、`MYAPI_PORT`、`MYAPI_DATA_DIR`、`MYAPI_LOGS_DIR` | 逐项复制值后删除旧变量，避免两个变量来源不一致。 |
| 容器内挂载点 | `/data`、`/app/logs` | 为保护现有数据库和日志格式暂不更改；只迁移宿主机目录。 |

OpenAI-compatible、Responses、Claude、Gemini 的路由、SSE 事件、请求字段和数据库
表结构属于技术协议，品牌迁移不会改变它们。升级客户端时只需确认公开地址和认证
配置；不要把 `my-api` 当作新的 API 协议或模型名称。

## 从旧机器迁移

旧实例停止写入并完成备份后，复制以下内容（路径仅为示例）：

```text
旧实例 /srv/previous-instance/data/my-api.db 或 one-api.db
    -> 新项目 deploy/data/ 中保留原文件名

旧实例 /srv/previous-instance/logs/
    -> 新项目 deploy/logs/
```

不要把数据库和日志提交到 Git。完成复制后执行：

```bash
./deploy/install.sh
```

## Cloudflare Tunnel

只保留一个入口即可：

```text
https://你的域名 -> http://127.0.0.1:3000
```

API Base URL：

```text
https://你的域名/v1
```

管理后台与 API 共用同一个端口和域名。

## 完整内容日志

默认配置：

```text
FULL_CONTENT_LOG_ENABLED=true
FULL_CONTENT_LOG_MAX_MB=100
FULL_CONTENT_LOG_MAX_FILES=10
# Keep normalized channel quota snapshots for this many days. 0 disables
# automatic cleanup; cleanup runs daily in bounded batches.
CHANNEL_QUOTA_SNAPSHOT_RETENTION_DAYS=0
# Provider account balance sampler (enabled by default; can be adjusted in the admin UI).
CHANNEL_QUOTA_SYNC_ENABLED=true
CHANNEL_QUOTA_SYNC_INTERVAL=1m
CHANNEL_QUOTA_SYNC_MAX_CHANNELS=100
# Read-only quota threshold status (disabled by default; no notifications or
# routing changes are triggered). Percentages are relative to provider total.
CHANNEL_QUOTA_ALERT_ENABLED=false
CHANNEL_QUOTA_ALERT_WARNING_PERCENT=20
CHANNEL_QUOTA_ALERT_CRITICAL_PERCENT=10
# Optional keyless cosign verification for `myapi upgrade` (requires cosign).
# MYAPI_VERIFY_IMAGE_SIGNATURE=false
# MYAPI_COSIGN_CERTIFICATE_IDENTITY=https://github.com/ForceMind/MyAPI/.github/workflows/docker-build.yml@refs/tags/v<version>
# MYAPI_COSIGN_CERTIFICATE_OIDC_ISSUER=https://token.actions.githubusercontent.com
```

后台采样默认开启，默认间隔为 1 分钟，并限制每轮请求数量和总运行时间。管理员可在
“设置 → 运维 → 监控与告警”中调整采样开关、间隔和每轮最大渠道数；部署环境变量
`CHANNEL_QUOTA_SYNC_ENABLED=false` 可作为强制关闭覆盖。额度告警默认关闭，只影响
额度历史中的只读状态，不发送通知、不停用渠道，也不改变路由。

同一进程内的 Relay 路由共享一个日志写入器，默认最多保留 10 个
100 MiB 文件，约为 1 GiB 软上限。单条超大记录可能暂时超过该值。

管理页面：

```text
https://你的域名/full-content-logs
```

只有管理员可以读取。详细说明见 `docs/FULL_CONTENT_LOGGING_CUSTOM.md`。

## Codex 渠道网页登录

1. 登录管理后台，打开“渠道”；
2. 新建渠道或编辑现有渠道，渠道类型选择 `ChatGPT Subscription (Codex)`；
3. 在“凭据”区域点击“使用 ChatGPT 登录”；
4. 在新页面完成 ChatGPT 登录和授权；
5. 浏览器会跳转到 `http://localhost:1455/auth/callback?...`，该页面无法打开是正常现象；
6. 复制地址栏中的完整 URL，返回管理后台并点击“从剪贴板粘贴”；
7. 点击“完成登录”。

新建渠道时还需要填写渠道名称、模型和分组，然后保存；编辑渠道时 OAuth 凭据会直接保存。

该流程不依赖服务器安装 Codex CLI。OAuth 临时状态保存在 `auth_flows` 表，部署时必须继续提供稳定且保密的 `SESSION_SECRET`，否则旧的未完成授权会话会失效。

## 默认推广精简

本发行版默认不展示第三方客户端推广或一键导入入口。如需恢复可信客户端入口，可在
“系统设置 → 控制台内容 → 聊天设置”中自行添加。许可证、NOTICE、版权和法律要求的
第三方通知仍按原文件保留，发行层页面统一使用 MyAPI。

## Full 与 LAN Lite 构建

Dockerfile 通过 `MYAPI_EDITION` 选择发行版，默认是完整的 Full 版。两版都保留渠道、
模型、API Key、用量、日志、游乐场、系统设置和 MyAPI 品牌；LAN Lite 另外启用精简
前端与后端路由边界，不构建充值、订阅、兑换码、公开定价、排行榜、用户管理和内置
聊天路由。两版都保留许可证、NOTICE、版权和法律要求的第三方通知。

Full 版构建：

```bash
docker build --build-arg MYAPI_EDITION=full -t local/myapi:full .
```

LAN Lite 构建：

```bash
docker build --build-arg MYAPI_EDITION=lan -t local/myapi:lan .
```

LAN 版的公开注册、支付、订阅、兑换和外部 OAuth 路由由后端拒绝，不能只依赖前端
隐藏菜单。局域网部署默认绑定回环地址，确认网络边界后再显式开放私网接口。

## 消耗分布粒度

消耗图现在支持分钟、小时、天、周四种服务端聚合粒度，并按浏览器为所选区间
计算出的固定时区偏移返回和显示桶边界。分钟数据最多选择最近 24 小时；页面会
禁用不兼容的 7/14/29 天组合，手工范围超限时会显示错误，不会再伪装成 0 消耗。

升级前的历史记录仍是小时精度，无法恢复成真实分钟细节；分钟级精度从部署本版
后新产生的数据开始。后端单次查询最多返回 1,500 个时间桶。

## 更新与回滚

> **NPM 发行状态：** 当前 `@forcemind/myapi` 尚未正式发布。源码检出时请使用
> `node cli/myapi.mjs ...`（或审核过的本地 tarball）替代下面示例中的
> `npx @forcemind/myapi ...`；不要从未知公共包解析同名命令。

更新前备份：

```bash
for db in deploy/data/my-api.db deploy/data/one-api.db; do
  if [ -f "$db" ]; then cp "$db" "$db.before-update"; fi
done
docker image inspect "${MYAPI_IMAGE:-ghcr.io/forcemind/myapi:<version>}"
```

新安装默认使用 `my-api.db`；接管旧实例时，CLI/运行时会在未显式设置
`SQLITE_PATH` 的情况下继续使用已有的 `one-api.db`。备份时保留实际存在的文件名，
不要同时创建一个空的同名数据库。

拉取并更新：

```bash
./deploy/install.sh
```

也可以使用 CLI 执行带健康等待和自动回滚的版本升级。命令只更新部署目录的
`MYAPI_IMAGE`，不会移动数据目录；升级前会在项目 `backups/` 下以 0600 权限保存
原环境文件：

```bash
npx @forcemind/myapi upgrade --project-dir . --version v0.2.0
```

GitHub Actions signs release images with keyless cosign. Operators who have
installed `cosign` can require verification before the environment file is
changed:

```bash
npx @forcemind/myapi upgrade --project-dir . --version v0.2.0 --verify-signature
```

Set `MYAPI_COSIGN_CERTIFICATE_IDENTITY` in `deploy/.env` to the exact trusted
workflow identity (and optionally override
`MYAPI_COSIGN_CERTIFICATE_OIDC_ISSUER`). Verification is opt-in; a missing
identity, missing cosign binary, or failed signature stops the upgrade before
any deployment state is changed.

CLI 会按 `MYAPI_EDITION` 选择 `ghcr.io/forcemind/myapi` 或
`ghcr.io/forcemind/myapi-lan`，拉取目标镜像并等待 Compose 健康检查。拉取、启动或
健康检查失败时自动恢复旧环境并重新启动旧镜像；若回滚也失败，应保留现场并按输出
中的错误进行人工处理。生产环境仍应先在副本上演练，不会因为安装 NPM 包而自动升级。

升级副本前可先执行只读预检。它检查版本、发行版镜像映射、环境文件、URL、会话密钥和
资源限制，不会创建备份、修改 `deploy/.env`、调用 Docker、拉取 GHCR 或执行 `cosign`：

```bash
npx @forcemind/myapi upgrade \
  --project-dir ./copy --version v0.2.0 --dry-run --json
```

只有副本预检通过后，才执行不带 `--dry-run` 的升级命令；SQLite/PostgreSQL 数据库恢复
仍须使用独立的脱敏副本和组织批准的备份工具。

部署模板默认设置 `MYAPI_CPU_LIMIT=2.0` 和 `MYAPI_MEMORY_LIMIT=2g`，用于限制
Docker Desktop 或小型服务器的资源占用。`myapi up` 与 `deploy/install.sh` 会先执行
Compose 配置校验，再拉取镜像，并等待最多 120 秒的健康检查；限制值无效或容器未健康
时不会把部署报告为成功。需要提高上限时，只修改 `deploy/.env` 中的这两个变量并先
在副本上验证。

若确实需要本地构建，请在 `deploy/.env` 中设置 `MYAPI_BUILD_LOCAL=true`。

如新镜像异常，将 `deploy/.env` 中的 `MYAPI_IMAGE` 改回已验证的旧镜像标签，然后重新执行：

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.yml up -d --force-recreate
```
