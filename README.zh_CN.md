<p align="center">
  <img src="web/public/myapi-logo-v1.png" alt="My API logo" width="144" />
</p>

# My API

统一管理模型服务、应用接入、权限与用量的自建 AI API 网关。

[English](README.md) · [简体中文](README.zh_CN.md) · [繁體中文](README.zh_TW.md) · [Français](README.fr.md) · [日本語](README.ja.md)

My API 将渠道接入、应用 API Key、权限和用量记录集中在一个管理界面中，优先满足个人自用，也支持受控地分享给少量用户。新安装默认关闭商业模块，使用自己的模型服务账号不需要先充值内部钱包；用户管理、权限、Key 限制和用量统计仍然保留。

## 技术栈

![Go](https://img.shields.io/badge/Go-00ADD8?style=flat-square&logo=go&logoColor=white)
![Gin](https://img.shields.io/badge/Gin-008ECF?style=flat-square)
![GORM](https://img.shields.io/badge/GORM-607D8B?style=flat-square)
![React 19](https://img.shields.io/badge/React-19-149ECA?style=flat-square&logo=react&logoColor=white)
![TypeScript](https://img.shields.io/badge/TypeScript-3178C6?style=flat-square&logo=typescript&logoColor=white)
![Rsbuild](https://img.shields.io/badge/Rsbuild-FF6B35?style=flat-square)

![Tailwind CSS 4](https://img.shields.io/badge/Tailwind_CSS-4-06B6D4?style=flat-square&logo=tailwindcss&logoColor=white)
![Base UI](https://img.shields.io/badge/Base_UI-111827?style=flat-square)
![Bun](https://img.shields.io/badge/Bun-14151A?style=flat-square&logo=bun&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-003B57?style=flat-square&logo=sqlite&logoColor=white)
![MySQL](https://img.shields.io/badge/MySQL-4479A1?style=flat-square&logo=mysql&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-4169E1?style=flat-square&logo=postgresql&logoColor=white)
![Redis](https://img.shields.io/badge/Redis-DC382D?style=flat-square&logo=redis&logoColor=white)
![Docker Compose](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker&logoColor=white)

- **后端：** Go（`go.mod` 声明 1.25.1）、Gin HTTP 路由、GORM 数据访问
- **管理界面：** React 19、TypeScript、Rsbuild、Tailwind CSS 4、Base UI；使用 Bun 管理前端依赖和脚本
- **数据存储：** 默认 SQLite，也可选择 MySQL 或 PostgreSQL；Redis 可选，用于共享缓存和限流
- **部署：** Docker 镜像与 Docker Compose；镜像安装不需要在本机准备 Go 或前端构建工具链

### 请求如何流转

```text
应用 + API Key → My API 身份认证与权限检查
              → 模型与渠道选择 → 模型服务
              ← 响应 / 流式输出 ←
                用量与错误记录
```

管理界面负责配置渠道、模型、用户和 Key；Go 服务检查请求权限，选择符合条件的渠道，调用已配置的模型服务，并记录可取得的用量证据。模型服务凭据保留在服务端，协议支持和预算资格遵循下文边界。

以上版本来自仓库依赖声明，不表示对所有未来依赖版本的兼容承诺。详见[后端依赖](go.mod)、[前端依赖](web/package.json)和[容器构建](Dockerfile)。

## 能做什么

- **接入模型服务。** 配置渠道及其开放的模型。现有适配器包括 OpenAI 兼容 API、Responses、Claude Messages、Gemini 和 Codex；具体端点和功能取决于适配器及模型服务账号。
- **分配访问权限。** 为不同应用或用户创建独立的应用 API Key，设置可用模型、访问方案和适用的用量限制，模型服务凭据保留在服务端。
- **看清使用情况。** 查看调用、错误、用量及受支持供应商的额度观测。账户图表区分缺失、失败和窗口重置，不把未知状态画成零消耗。
- **验证渠道。** 明确选择模型、端点和流式模式进行测试，在适用时复用最近成功的测试选项。
- **选择存储。** 默认使用 SQLite，也可按部署需要配置 MySQL 或 PostgreSQL，三者是备选方案，不是必须同时安装的服务。

管理界面支持简体中文、繁体中文、英文、法语、日语、俄语和越南语。端点格式见 [Relay API 规范](docs/openapi/relay.json)；规范中存在某个端点，不表示所有模型服务都支持它。

### 使用边界

严格 Token/USD 预算目前只覆盖已取得资格的**官方原生 Responses 纯文本路径**；USD 还要求匹配的冻结价格和受支持服务档位。模型别名、协议转换、工具及多模态不会自动获得精确预算资格。

Codex 百分比是账户/窗口剩余量的安全阈值，不是共享订阅在各 Key 之间的消耗账本。订阅渠道的 API 等价成本仅供参考，不是供应商实际账单。缺失或估算用量不等于实际为零；结果不明的请求应保留证据并核对，不能直接假定退款。

## 选择版本

截至 2026-10-04：

- **已发布预览版：`v0.2.0-beta.3`。** 提供 Linux amd64/arm64 的 Full 和旧版 LAN 镜像。源码、制品、镜像 digest、签名及验证范围见 [beta.3 发布记录](docs/RELEASE_BETA_3.md)。
- **beta.4：源码与限定自动化已通过，未发布。** 包含限定范围的模型发现、显式映射、路由预览、实际分发和日志流程，不包含在 beta.3 镜像里。
- **beta.5：仅本地开发候选。** 多账户调度、临时冷却、有界故障切换及尝试说明尚未发布；该候选自己的远端三数据库及 Chromium 验证仍待完成。

预览版不代表生产就绪。真实账号 OAuth、额度重置/429、供应商账单对照及目标服务器 HTTPS 验收仍有限或未完成；构建成功、容器健康不能代替这些结果。默认镜像仍指向 beta.3，检出开发分支不会自动运行开发版镜像。

## 安装已发布预览版

推荐从 **Linux 上的 Full 镜像部署**开始：使用固定版本镜像和仓库安装脚本，默认 SQLite，仅绑定本机回环地址，由自己的 HTTPS 反向代理提供访问。

### 准备条件

- Linux amd64 或 arm64、Git、Bash，以及可用的 Docker daemon
- 支持 `up --wait --wait-timeout` 的 Docker Compose v2
- 自己控制的 HTTPS 地址，以及指向 `http://127.0.0.1:3000` 的反向代理
- 仓库读取权限、GitHub/GHCR 网络连接；后续调用所需的合法模型服务凭据
- 用于数据库、日志及备份的持久存储；下面生成密钥示例使用 OpenSSL

这条镜像安装路径**不需要** Go、Bun、Node.js、Redis 或独立数据库服务器。模板默认将容器限制为 2 CPU / 2 GiB 内存；这是资源上限，不是经过测定的最低硬件要求，应为宿主系统、业务负载和存储另留空间。

### 1. 获取对应版本的部署文件

以下操作只用于**新目录中的全新安装**：

```bash
git clone --branch v0.2.0-beta.3 --single-branch https://github.com/ForceMind/MyAPI.git my-api
cd my-api
umask 077
cp deploy/.env.example deploy/.env
chmod 600 deploy/.env
```

已有实例请先阅读升级部分，保留原配置，不要用示例覆盖已有 `.env`。

### 2. 配置实例

编辑 `deploy/.env`，保留以下选项，将示例地址改为自己的准确 HTTPS Origin，不带 API 路径：

```dotenv
MYAPI_IMAGE=ghcr.io/forcemind/myapi:v0.2.0-beta.3
MYAPI_BUILD_LOCAL=false
MYAPI_EDITION=full
MYAPI_BIND_ADDRESS=127.0.0.1
MYAPI_ALLOW_LAN=false
MYAPI_SESSION_COOKIE_SECURE=true
MYAPI_PUBLIC_URL=https://api.example.com
FULL_CONTENT_LOG_ENABLED=false
```

安装器会拒绝 `example.com` 占位地址。生成新的随机会话密钥，将结果通过私有编辑器写入 `SESSION_SECRET`：

```bash
openssl rand -hex 32
```

安装器要求至少 48 字符。若使用多账户额度采样，再生成一份**独立**身份密钥：

```bash
openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n'
```

将 `CHANNEL_QUOTA_IDENTITY_KEYS` 设置为 `active:v1:` 加上该结果。完整密钥环须随数据库安全备份，共用该数据库的实例须使用相同密钥环；留空会关闭依赖身份的额度采样。已有数据库升级时不要重新生成会话密钥或身份密钥，也不要将它们贴进反馈、聊天或 Git。安装器只读取字面的 `KEY=VALUE`，不要在 `.env` 中写 shell 命令替换。

不修改路径时，持久数据和日志分别位于 `deploy/data/` 与 `deploy/logs/`，挂载到容器内 `/data` 和 `/app/logs`，不要只保存在可丢弃的容器层中。

### 3. 启动并检查

```bash
bash deploy/install.sh
```

脚本校验 Compose 配置、拉取固定镜像、启动服务，并最多等待 120 秒的容器健康检查。它不会安装 Docker、申请证书、配置反代或开放防火墙端口。

打开配置的 **HTTPS 地址**，完成初始化并创建管理员账号。Full 使用 Secure Cookie，不应把 `http://localhost:3000` 当作推荐登录地址。加入真实账户前，先确认能登录，并在「系统信息」核对运行版本及 revision。

只在本机或私网使用时，另按[旧版 LAN 指南](docs/LAN_LITE.md)选择已发布的 `ghcr.io/forcemind/myapi-lan:v0.2.0-beta.3`。局域网共享需要明确开启。统一 Lite/Desktop 安装器及更新器尚未交付，桌面构建产物也不代表实机验收完成。

## 完成第一次调用

1. **添加模型服务渠道。** 在「渠道」(`/channels`) 选择真实供应商类型，填写合法授权的地址及凭据。容器内 Codex 使用现有网页登录流程，容器无法读取宿主机登录文件。
2. **启用模型。** 在支持时获取模型列表，或手动填写模型服务提供的准确模型 ID。确认启用的模型、分组/访问方案及显式映射。发现模型不等于已授权或调用成功；增强的 beta.4 发现与路由流程需要对应开发源码，不在 beta.3 镜像中。
3. **做一次小规模渠道测试。** 明确选择模型、受支持端点和流式模式。测试会联系模型服务，可能消耗额度或产生费用，先使用非敏感输入。
4. **创建应用 API Key。** 为应用单独建 Key，只授予所需模型和访问方案，选择该渠道支持的限制。不要分发模型服务密钥或管理员登录令牌。
5. **配置客户端。** OpenAI 兼容客户端的 Base URL 使用自己的 HTTPS 地址加 `/v1`，密钥使用應用程式 Key，模型填写实际启用的公共名称。其他协议使用相应文档中的端点。先发一条简短请求，再接入正式任务。
6. **查看结果。** 在「用量日志」(`/usage-logs/common`) 核对状态、模型、用量证据及适用费用。管理员可查看更多详情；具备路由证据的开发版还可核对渠道和映射目标。路由预览不保证下一次随机选择相同，也不保证某个 Key 一定获准调用。

失败时先检查供应商类型、端点、凭据、模型、Key 权限和模型服务额度，不要先增加重试次数。没有可靠证据时，保留待核对状态。

## 配置与维护

高级配置见[部署说明](DEPLOYMENT_CUSTOM.md)、[部署模板](deploy/.env.example)和[运行时环境变量](.env.example)。部署配置和运行时变量是不同层：把任意变量加到 `deploy/.env`，不代表它会自动传入容器。配置外部数据库或 Redis 时请核对 [Compose 文件](deploy/docker-compose.yml)。

推荐路径默认 SQLite。MySQL ≥ 5.7.8、PostgreSQL ≥ 9.6 是兼容性基线，不是继续使用失去维护版本的建议；应选用适当维护的版本并演练迁移。Redis 可选，多节点需单独核对会话、缓存和限流语义，见[认证与登录会话](docs/authentication.md)。

源码 CLI 需要 Node.js ≥ 20。本指南使用检出仓库中的 CLI，不依赖从 NPM 安装软件包：

```bash
node cli/myapi.mjs help
node cli/myapi.mjs doctor --project-dir .
node cli/myapi.mjs status --project-dir .
node cli/myapi.mjs logs --project-dir .
```

`doctor` 检查配置及工具是否可用，不验证真实模型服务。日志可能包含敏感信息。规划中的 `install`、`switch`、`rollback` 尚不是当前 CLI 命令。

### 升级、备份与恢复

1. 记录正在运行的镜像 tag/digest、源码 revision、数据库类型、路径和私有配置。目标必须是实际发布版本，不使用规划中的 beta 标签或 `latest`。
2. 按对应数据库流程制作一致性备份；SQLite 按需保留 WAL 状态，同时保存配置、会话密钥、完整身份密钥环及必要日志。在隔离副本验证恢复后再升级真实实例。
3. 在副本中演练目标版本的登录、渠道、Key、受控请求、日志和恢复。CLI 支持只读预检；例如为旧实例演练升级至已发布 beta.3：

   ```bash
   node cli/myapi.mjs upgrade --project-dir ./upgrade-copy --version v0.2.0-beta.3 --dry-run --json
   ```

4. 实际切换按[升级演练与恢复指南](docs/UPGRADE_REHEARSAL.md)执行。CLI 保存的 `.env` **不是数据库备份**。一旦尝试启动目标版本，数据库可能已经迁移，CLI 不会自动启动旧镜像；启动不兼容旧程序前，必须恢复经过验证的升级前数据库备份。

不要通过删除数据卷、清空待核对记录或让旧程序直接打开新数据库来“回滚”。接管或迁移实例时保留原数据和访问范围。

## 安全与使用责任

- 首次部署保留回环监听，只开放预期 HTTPS 代理。分享前检查可信代理、注册入口、用户角色和 Key 权限。
- 只接入已获授权的账户/API，遵守供应商条款及适用法律。对外服务、转售等可能涉及额外合规义务，安装本项目不代表已满足这些义务。
- 部署模板默认开启完整内容日志，上面的示例特意将其关闭。若需开启，先阅读[完整内容日志](docs/FULL_CONTENT_LOGGING_CUSTOM.md)，确认权限、保留期限与备份范围；脱敏不能保证提示词和输出中没有个人或机密信息。
- 后台额度采样可能联系模型服务。加入真实凭据前检查监控设置及供应商支持边界。
- 密钥、OAuth 文件、数据库和私有日志不要进入 Git、截图、问题反馈或共享压缩包，诊断信息分享前先脱敏。

## 文档与帮助

- [beta.3 制品与发布证据](docs/RELEASE_BETA_3.md)
- [部署配置](DEPLOYMENT_CUSTOM.md) · [旧版 LAN](docs/LAN_LITE.md)
- [升级与恢复](docs/UPGRADE_REHEARSAL.md) · [安装验收记录](docs/R1_INSTALLATION_CHECK.md)
- [Relay API](docs/openapi/relay.json) · [管理 API](docs/openapi/api.json)
- [用量与额度分析](docs/QUOTA_ANALYTICS.md) · [Claude 组织用量边界](docs/CLAUDE_USAGE_REPORT.md)
- [认证与会话](docs/authentication.md) · [内容日志](docs/FULL_CONTENT_LOGGING_CUSTOM.md)
- [问题反馈](https://github.com/ForceMind/MyAPI/issues)：请提供版本/revision、部署方式、脱敏复现步骤、预期与实际结果

参与开发请看[开发计划与实现记录](docs/MYAPI_MASTER_PLAN.md)。开发历史单独保存，不作为本页安装步骤。

## 许可证与法律声明

项目采用 [GNU AGPLv3](LICENSE)。[NOTICE](NOTICE) 说明第 7 节附加条款及必须保留的法律和界面署名，[第三方许可证](THIRD-PARTY-LICENSES.md) 收录依赖通知。分发或通过网络提供修改版本时，请保留适用通知、标明修改，并履行对应源码提供义务；桌面发行还须保留适用的 Electron/Chromium 通知。使用或再分发前请阅读完整条款。
