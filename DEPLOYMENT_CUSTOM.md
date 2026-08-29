# 定制版部署说明

> 本文适用于 **MyAPI** 自建发行版（rc.25 兼容基线）。MyAPI CLI 的初始化、配置、
> 数据接管与安全检查请参阅 [`docs/MYAPI_DISTRIBUTION.md`](docs/MYAPI_DISTRIBUTION.md)。
> 新部署使用 `my-api` 与 `MYAPI_*` 命名；旧部署的目录、变量和挂载点只作为迁移输入，
> 不应继续写入新的配置。

## 环境要求

- Linux；
- Docker 与 Docker Compose v2；
- 一个反向代理或 Cloudflare Tunnel（可选）；
- 只需要对外代理 MyAPI 的 `3000` 端口。

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
| 镜像/服务名 | `ghcr.io/forcemind/myapi:v0.1.1` / `my-api` | 镜像由 GitHub Actions 生成；升级时修改 `MYAPI_IMAGE`，再按需重建容器。 |
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
```

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

## 自用精简构建

仓库 Dockerfile 默认设置 `VITE_SELF_USE_MINIMAL=true`，构建单管理员自用界面：

- 保留渠道、模型、API Key、用量与完整内容日志、游乐场、个人设置、系统设置、登录/OAuth/初始化流程；
- 保留 About、AGPL 许可证、版权和法律要求的第三方通知；站点默认品牌为 MyAPI；
- 不构建充值、订阅、兑换码、公开定价、排行榜、用户管理和内置聊天路由；
- 前端仅打包简体中文和英文；
- 使用轻量 Provider 标记和项目内置品牌图标，不再打包完整第三方图标 UI 系统；
- Alpine 仅作为静态 Go 二进制的运行层。

如需恢复完整上游业务界面，请删除 Dockerfile 构建命令中的
`VITE_SELF_USE_MINIMAL='true'`，并根据需要恢复本仓库已删除的未使用组件和依赖。

## 消耗分布粒度

消耗图现在支持分钟、小时、天、周四种服务端聚合粒度，并按浏览器为所选区间
计算出的固定时区偏移返回和显示桶边界。分钟数据最多选择最近 24 小时；页面会
禁用不兼容的 7/14/29 天组合，手工范围超限时会显示错误，不会再伪装成 0 消耗。

升级前的历史记录仍是小时精度，无法恢复成真实分钟细节；分钟级精度从部署本版
后新产生的数据开始。后端单次查询最多返回 1,500 个时间桶。

## 更新与回滚

更新前备份：

```bash
for db in deploy/data/my-api.db deploy/data/one-api.db; do
  if [ -f "$db" ]; then cp "$db" "$db.before-update"; fi
done
docker image inspect "${MYAPI_IMAGE:-ghcr.io/forcemind/myapi:v0.1.1}"
```

新安装默认使用 `my-api.db`；接管旧实例时，CLI/运行时会在未显式设置
`SQLITE_PATH` 的情况下继续使用已有的 `one-api.db`。备份时保留实际存在的文件名，
不要同时创建一个空的同名数据库。

拉取并更新：

```bash
./deploy/install.sh
```

若确实需要本地构建，请在 `deploy/.env` 中设置 `MYAPI_BUILD_LOCAL=true`。

如新镜像异常，将 `deploy/.env` 中的 `MYAPI_IMAGE` 改回已验证的旧镜像标签，然后重新执行：

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.yml up -d --force-recreate
```
