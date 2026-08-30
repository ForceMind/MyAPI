# MyAPI 自建发行版与 NPM 包

MyAPI 是面向自建部署的发行品牌，提供可审计的完整源码、部署模板和
`@forcemind/myapi` CLI。源码仓库：
<https://github.com/ForceMind/MyAPI>（当前为私有仓库）。本文只描述发行层
的命名和迁移边界；OpenAI、Anthropic、Google 等第三方协议名称仍按其官方
名称保留。

## 品牌与兼容性边界

新部署统一使用以下发行层标识：

| 范围 | MyAPI 规范 | 迁移与兼容说明 |
| --- | --- | --- |
| 人类可见品牌 | `MyAPI` | 用于站点名称、Logo、CLI 帮助、文档和发行说明。 |
| 机器安全 slug | `my-api` | 用于服务名、容器名、默认镜像名和部署目录。 |
| 部署变量 | `MYAPI_IMAGE`、`MYAPI_EDITION`、`MYAPI_BIND_ADDRESS`、`MYAPI_ALLOW_LAN`、`MYAPI_PORT`、`MYAPI_PUBLIC_URL`、`MYAPI_DATA_DIR`、`MYAPI_LOGS_DIR` | 新配置只写这些变量；旧部署变量只在一次性迁移时读取，完成迁移后应删除。非回环绑定还必须显式设置 `MYAPI_ALLOW_LAN=true`。 |
| 服务与容器 | `my-api` | 重命名容器前先备份并确认 compose 项目，避免误删卷。 |
| 官方镜像 | `ghcr.io/forcemind/myapi:vX.Y.Z`（Full）或 `ghcr.io/forcemind/myapi-lan:vX.Y.Z`（LAN Lite） | 由本仓库 GitHub Actions 生成；升级时固定目标版本 tag，不强制移动旧 tag。 |
| 容器挂载点 | `/data`、`/app/logs` | 这是数据格式兼容边界，容器内挂载点暂不变；宿主机目录可迁移到新的 `my-api` 项目目录。 |
| API、SSE 和数据库协议 | 现有 OpenAI-compatible、Responses、Claude、Gemini 路由及表结构 | 本发行版不因品牌重命名改变线协议、路由、字段或数据库结构；客户端可继续使用原协议。 |

品牌迁移只改变发行层名称，不会把第三方供应商（例如 OpenAI 或 Claude）改名，
也不会把 API 字段、响应事件或数据库表改成品牌专用协议。需要迁移技术标识时，
应同时更新 compose、脚本、客户端示例和回滚说明，并先在副本上验证读写。

### Go 模块身份

源码发行版的 Go 模块路径已统一为 `github.com/ForceMind/MyAPI`，内置
`relaykit` 子模块对应 `github.com/ForceMind/MyAPI/relaykit`。这是源码导入路径的
有意变更；使用 Go 包的外部项目需要同步更新 import，并在升级说明中记录这一点。
它不会改变 HTTP 协议、数据库表、JWT 兼容校验或已有部署数据。

## 可见实例品牌

仓库提供 `web/public/myapi-logo-v1.png`。构建默认值可以通过以下参数覆盖，
但管理员在站点设置中填写的自定义名称和 Logo 始终优先：

```text
MYAPI_BRAND_NAME=MyAPI
MYAPI_BRAND_LOGO=/myapi-logo-v1.png
```

运行时站点设置示例：

```text
SystemName = MyAPI
Logo = https://你的域名/myapi-logo-v1.png
ServerAddress = https://你的域名
Footer = 留空
About = 留空
```

默认部署会拉取 GitHub Actions 生成的 GHCR 镜像。私有 GHCR 包需要先登录：

```bash
echo "$GHCR_TOKEN" | docker login ghcr.io -u YOUR_GITHUB_USER --password-stdin
```

然后在 `deploy/.env` 中固定要运行的版本，例如：

```text
MYAPI_IMAGE=ghcr.io/forcemind/myapi:<version>
MYAPI_EDITION=full
MYAPI_BIND_ADDRESS=127.0.0.1
MYAPI_BUILD_LOCAL=false
```

局域网极简版使用同一套源码但不同的镜像和后端发行策略：

```text
MYAPI_IMAGE=ghcr.io/forcemind/myapi-lan:<version>
MYAPI_EDITION=lan
MYAPI_BIND_ADDRESS=127.0.0.1
MYAPI_ALLOW_LAN=false
```

LAN 版默认只绑定本机；确认局域网访问范围后，再将 `MYAPI_BIND_ADDRESS` 设置为
私网接口地址并将 `MYAPI_ALLOW_LAN=true`，然后为每位同事创建独立的下游 API Key。
`deploy/install.sh` 会拒绝公网/格式错误的地址，也会在缺少显式 opt-in 时停止；上游凭据不会显示给下游用户。
该脚本按 `KEY=VALUE` 读取 `.env`，不会执行其中的 shell 命令替换或函数；请不要依赖
变量展开语法，敏感值应直接写入并将文件保持为 `0600`。

执行 `myapi up` 或 `deploy/install.sh` 时会先拉取该镜像。只有明确设置
`MYAPI_BUILD_LOCAL=true`（或使用 `local/...` 镜像名）才会从源码构建。

需要切换到新版本时，推荐显式执行：

```bash
npx @forcemind/myapi upgrade --project-dir ./my-api --version v0.2.0
```

该命令按 Full/LAN 发行版选择 GHCR 仓库，备份部署环境文件，等待健康检查，并在
失败时恢复旧镜像配置。它不会在安装包时自动运行，也不会替换或删除数据卷。

本地构建示例（不会发布任何远端镜像）：

```bash
docker build \
  --build-arg MYAPI_BRAND_NAME=MyAPI \
  --build-arg MYAPI_BRAND_LOGO=/myapi-logo-v1.png \
  --build-arg MYAPI_EDITION=full \
  -t local/my-api:custom-rc25 .
```

使用部署脚本时，在 `deploy/.env` 中填写上述品牌参数。它们不是密钥；不要把
Token、OAuth JSON 或会话密钥写入品牌变量。

## NPM 发行包

独立发行包名称为：

```text
@forcemind/myapi
```

它不会在安装时自动运行 Docker，也不会读取或上传密钥。包内包括完整源码、
Dockerfile、部署模板、历史补丁、许可证和零运行时依赖 CLI。

### 初始化

```bash
npx @forcemind/myapi init ./my-api
npx @forcemind/myapi configure \
  --project-dir ./my-api \
  --public-url https://myapi.example.com
npx @forcemind/myapi doctor --project-dir ./my-api
npx @forcemind/myapi build --project-dir ./my-api
```

`configure` 会生成随机 `SESSION_SECRET`，并将 `deploy/.env` 权限设为 `0600`。
初始化后的项目会标记为 `private: true`，同时生成忽略密钥、数据库、日志、缓存、
`dist` 和 `node_modules` 的忽略文件。`myapi up` 只在 HTTPS 地址、会话密钥、端口
和 compose 配置检查通过后启动；检查失败不会启动容器。

直接执行 `docker compose` 时请先确保 `deploy/.env` 已写入 `MYAPI_PUBLIC_URL`；
只存在旧 `NEW_API_PUBLIC_URL` 的配置先运行 `myapi migrate --project-dir DIR`。
这样可以避免 Compose 在变量解析阶段把受信任 Origin 留空。

### 接管已有数据

CLI 不会复制、删除或自动迁移数据，只把用户明确给出的目录写入配置。示例：

```bash
npx @forcemind/myapi adopt \
  --project-dir ./my-api \
  --data-dir /srv/previous-instance/data \
  --logs-dir /srv/previous-instance/logs
```

接管前应停止旧实例、校验备份并确认数据库文件可读；`down` 不附带 `-v`，不会删除
Docker volume。

## 发布检查与人工门禁

发布前先在本地完成所有检查，推荐只做 dry-run：

```bash
npm test
npm run release:state
npm run source:manifest
npm run pack:check
npm pack --dry-run --json
npm publish --dry-run --access public --registry=https://registry.npmjs.org/
```

Docker 镜像 workflow 会在推送符合 `vX.Y.Z` 的版本 tag 后自动运行，矩阵构建并推送
Full 与 LAN Lite 两个多架构镜像；稳定版本分别更新各自的 `latest`，预发布 tag
不会覆盖 `latest`。也可以通过 `workflow_dispatch` 指定已有 tag 手动重跑，但必须在
`confirm` 输入中选择 `PUBLISH`；默认的 `NO` 会跳过所有推送 job。workflow 会校验
tag、`VERSION` 和 `package.json.version` 完全一致，不会移动既有 tag。
GitHub Actions 只负责生成镜像，不直接连接或重启生产主机；部署端更新
`MYAPI_IMAGE` 后由 `myapi up` 或 `deploy/install.sh` 拉取新版本。

同一版本 tag 也会自动触发 Electron macOS/Windows 构建并上传 Actions artifacts；
只有在维护者显式填写 `PUBLISH` 且开启 release 环境变量时，才会附加到 GitHub
Release，不会因为推送 tag 自动发布桌面安装包。

正式发布前还必须确认：

- 工作树干净，tag、`VERSION` 与 `package.json.version` 一致；
- Go、relaykit、前端、普通构建和精简构建全部通过；
- tarball 中没有 `.env`、数据库、日志、OAuth JSON、token、缓存、`dist` 或
  `node_modules`；
- NPM scope、Trusted Publishing、2FA 和 provenance 已由维护者单独确认；
- 发布镜像的 registry、仓库和签名策略已明确填写，不使用未确认的默认远端。

`SOURCE_MANIFEST.json` 是生成文件，发布前重新生成并检查内容即可，不要把它或
生产数据加入 Git。

截至 2026-08-30，远端 `v0.1.0` 仍指向历史提交 `9c618d3`，`v0.1.1` 指向
`5007c6c`，当前 `main` 为后续提交。`npm run release:state` 因当前 HEAD 尚未
匹配 `v0.1.1` 而有意失败；不得移动或覆盖已有 tag。下一次正式发布必须先确定
新的版本号，创建指向当前提交的新 SemVer tag，再重新运行全部发布检查。

## MyAPI 静态官网

`website/` 是独立的无依赖静态产品官网，借鉴 TokenHub 的产品叙事方式，使用 MyAPI
自己的文案、图形和发行版说明。它不依赖后台或运行时密钥，可直接用静态文件服务器
预览：

```bash
python3 -m http.server 4173 --directory website
```

官网发布暂不绑定生产部署或 Docker 镜像流程；确定域名和托管目标后，再增加单独、
经过审阅的静态站点发布 workflow。

## 法律与第三方通知

发行层品牌可以使用 MyAPI，但仓库中的许可证、NOTICE、依赖许可证和法律要求的
第三方通知必须原样保留。若某个历史归属条款的适用范围不确定，应单独进行法律
审阅，不要用品牌替换许可证文本，也不要把第三方代码宣传为 MyAPI 独立原创。
