# 定制版部署说明

## 环境要求

- Linux；
- Docker 与 Docker Compose v2；
- 一个反向代理或 Cloudflare Tunnel（可选）；
- 只需要对外代理 New API 的 `3000` 端口。

## 新机器安装

```bash
git clone <你的仓库地址>
cd new-api-custom-rc25

cp deploy/.env.example deploy/.env
```

编辑 `deploy/.env`：

- 设置随机的 `SESSION_SECRET`；
- 将 `NEW_API_PUBLIC_URL` 改为实际 HTTPS 域名；
- 如需修改宿主机端口，修改 `NEW_API_PORT`。

然后执行：

```bash
chmod +x deploy/install.sh
./deploy/install.sh
```

脚本会构建本地镜像、创建数据与日志目录、启动容器并显示健康状态。

## 从旧机器迁移

旧机器停止写入后，复制以下内容：

```text
旧机器 /root/new-api/data/one-api.db
    -> 新仓库 deploy/data/one-api.db

旧机器 /root/new-api/logs/
    -> 新仓库 deploy/logs/
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
FULL_CONTENT_LOG_MAX_FILES=0
```

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

本定制版默认不展示第三方客户端推广或一键导入入口。如需恢复可信客户端入口，可在“系统设置 → 控制台内容 → 聊天设置”中自行添加。项目版权、许可证和 New API/QuantumNous 署名仍按上游要求保留。

## 更新与回滚

更新前备份：

```bash
cp deploy/data/one-api.db deploy/data/one-api.db.before-update
docker image inspect local/new-api:custom-rc25
```

重新构建：

```bash
./deploy/install.sh
```

如新镜像异常，将 `deploy/.env` 中的 `NEW_API_IMAGE` 改回旧镜像标签，然后重新执行：

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.yml up -d --force-recreate
```
