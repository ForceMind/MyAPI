# My API 自建发行版、统一入口与 NPM 包

My API 是面向自建部署的发行品牌，提供可审计的完整源码、部署模板和
`@forcemind/myapi` CLI。源码仓库：
<https://github.com/ForceMind/MyAPI>（当前为私有仓库）。本文只描述发行层
的命名和迁移边界；OpenAI、Anthropic、Google 等第三方协议名称仍按其官方
名称保留。

**发行状态：** `@forcemind/myapi` 当前尚未正式发布到 NPM；统一 Release Manifest、服务器 Lite、个人电脑 Lite、Desktop updater 和形态切换仍未实现。现有 `cli/lib/release-manifest.mjs` 与 `installation-state.mjs` 仅为未接线的 schema-1 结构选择和内存安装状态/cleanup plan；`legacy-installation-profile.mjs` 仅对显式旧 Docker 配置作 fail-closed 的 Full/Lite/local/LAN/needs_manual 画像映射。三者都不能获取或验签资产、读写文件、安装、更新、切换或回退。源码检出状态下请将
文中的 `npx @forcemind/myapi ...` 替换为 `node cli/myapi.mjs ...`，或使用维护者
审核过的本地 tarball；正式发布后再按版本和 registry 复核 `npx` 示例。

产品定位、制品关系、更新状态机和验收见 [发行制品、安装与更新合同](RELEASE_MANIFEST.md)；提示词学习数据与 Codex 应用备份的保留规则见 [S5-P 专题](PROMPT_LEARNING.md)。本文保留当前可用 Legacy CLI/Docker 事实，不能把未来命令写成当前可执行命令。

## 品牌与兼容性边界

新部署统一使用以下发行层标识：

| 范围 | MyAPI 规范 | 迁移与兼容说明 |
| --- | --- | --- |
| 人类可见品牌 | `MyAPI` | 用于站点名称、Logo、CLI 帮助、文档和发行说明。 |
| 机器安全 slug | `my-api` | 用于服务名、容器名、默认镜像名和部署目录。 |
| Legacy 部署变量 | `MYAPI_IMAGE`、`MYAPI_EDITION`、`MYAPI_BIND_ADDRESS`、`MYAPI_ALLOW_LAN`、`MYAPI_PORT`、`MYAPI_PUBLIC_URL`、`MYAPI_DATA_DIR`、`MYAPI_LOGS_DIR` | 当前实现仍使用这些字段；`MYAPI_EDITION=lan` 混合了旧功能版/安装形态/访问范围，不能当作新架构规范。 |
| 目标安装记录 | `product_edition`、`installation_shape`、`access_mode`、实际监听与外部验证状态 | 后续在受控安装记录中分开保存 Full/Lite、服务器/个人/桌面和 local/LAN/public；新旧冲突 fail closed，旧 LAN 不自动公网。 |
| 服务与容器 | `my-api` | 重命名容器前先备份并确认 compose 项目，避免误删卷。 |
| 当前官方镜像 | `ghcr.io/forcemind/myapi:vX.Y.Z`（Full）或 `ghcr.io/forcemind/myapi-lan:vX.Y.Z`（Legacy LAN） | 由本仓库 GitHub Actions 生成；`myapi-lan` 暂作 Lite 兼容 identity，长期 Lite 坐标尚未决定，不能擅自新增或改名。 |
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

## 统一交付模型（目标，尚未实现）

同一个 `myapi` 入口将在检测操作系统、CPU 架构、容器/原生能力和现有安装后，让用户选择：

1. Full 服务器完整版；
2. Lite 服务器轻量版；
3. 个人电脑上的 Lite；
4. Desktop，即 Lite 的桌面安装形态。

Full、Lite、Desktop 共享核心业务和必要资源；功能版、安装形态、访问模式独立。Lite 可以部署到个人服务器、云服务器、VPS 或个人电脑，绝不因 Lite 名称被限制到局域网。Desktop 与 Lite 的业务和数据模型一致，只增加安装、后台服务、托盘、升级、备份恢复与受限 OS 集成。

推荐 NPM 包长期作为轻量 control-plane CLI：它保存安装向导、manifest 校验、信任根和指南索引，不在 package install 时下载或启动运行制品。每个正式版本由不可变 `RELEASE_MANIFEST.json` 连接源码 SHA、NPM 包、Full/Lite OCI、原生二进制、Desktop 安装包、平台/架构、hash/digest、签名、数据库/配置 schema 和回退条件。安装器只下载/保留用户选中的制品；`SOURCE_MANIFEST.json` 继续只校验源码包，不能取代 Release Manifest。

未来公开的 `myapi install`、`myapi switch`、`myapi rollback` 等命令目前不存在；教程和 UI 在实现、签名和真实验证前只能标为设计，不能当作可复制命令。正式 NPM/GHCR 发布、tag 与发布坐标仍需单独授权。

### Legacy LAN 迁移

现有 `MYAPI_EDITION=lan` 不是新的产品定义。升级时：回环/`MYAPI_ALLOW_LAN=false` 映射为 Lite/local，私网显式 `MYAPI_ALLOW_LAN=true` 映射为 Lite/lan；两者均保留原监听、数据、权限、镜像和访问范围。任何新旧配置冲突必须停止并说明，不能自动开放公网、改变防火墙或访问宿主文件。

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

以下是**当前 Legacy LAN Docker** 的兼容配置，不是新的 Lite 安装方式：

```text
MYAPI_IMAGE=ghcr.io/forcemind/myapi-lan:<version>
MYAPI_EDITION=lan
MYAPI_BIND_ADDRESS=127.0.0.1
MYAPI_ALLOW_LAN=false
```

当前 Legacy LAN 默认只绑定本机；确认局域网访问范围后，再将 `MYAPI_BIND_ADDRESS` 设置为
私网接口地址并将 `MYAPI_ALLOW_LAN=true`，然后为每位同事创建独立的下游 API Key。
`deploy/install.sh` 会拒绝公网/格式错误的地址，也会在缺少显式 opt-in 时停止；上游凭据不会显示给下游用户。
该脚本按 `KEY=VALUE` 读取 `.env`，不会执行其中的 shell 命令替换或函数；请不要依赖
变量展开语法，敏感值应直接写入并将文件保持为 `0600`。

执行 `myapi up` 或 `deploy/install.sh` 时会先拉取该镜像。只有明确设置
`MYAPI_BUILD_LOCAL=true`（或使用 `local/...` 镜像名）才会从源码构建。

当前 Docker 升级只能显式执行：

```bash
npx @forcemind/myapi upgrade --project-dir ./my-api --version v0.2.0
```

该命令按 Full/Legacy LAN 发行版选择 GHCR 仓库，备份部署环境文件，等待健康检查，并在
失败时恢复旧镜像配置。它不会在安装包时自动运行，也不会替换或删除数据卷；它还不是包含数据库恢复、持久 journal、服务器 Lite、Desktop 或产品形态切换的统一更新器。

对需要固定不可变镜像的升级，可增加 `--pin-digest`（或在 `deploy/.env` 设置
`MYAPI_PIN_IMAGE_DIGEST=true`）；CLI 会在拉取后解析并校验 RepoDigest，再让 Compose
使用 `repo@sha256:...` 完成切换。普通 `up`/首次安装仍按版本 tag 拉取，生产环境应在
升级演练中明确选择是否启用 digest 固定。

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

## 当前 NPM 源码包与未来统一安装入口

独立发行包名称为：

```text
@forcemind/myapi
```

当前包不会在安装时自动运行 Docker，也不会读取或上传密钥。当前源码分发包括完整源码、
Dockerfile、部署模板、历史补丁、许可证和零运行时依赖 CLI。

未来统一入口仍使用相同的包名和 CLI 名；推荐它成为按 Release Manifest 选择制品的 bootstrap，而不是把所有 Full/Lite/Desktop 平台制品放进一个 NPM tarball。安装完成后保留当前运行制品、一个受控回退制品和明确的恢复材料；未选择制品、安装暂存和已确认可删除的自有文件仅在健康检查提交后按 `owned-files` 清单清理。数据库、配置、日志、上传、S5-P 记录/版本和 Codex 应用备份永不属于该清理范围。

### 当前源码初始化

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

### 当前接管已有数据

CLI 不会复制、删除或自动迁移数据，只把用户明确给出的目录写入配置。示例：

```bash
npx @forcemind/myapi adopt \
  --project-dir ./my-api \
  --data-dir /srv/previous-instance/data \
  --logs-dir /srv/previous-instance/logs
```

接管前应停止旧实例、校验备份并确认数据库文件可读；`down` 不附带 `-v`，不会删除
Docker volume。

## 更新、切换、清理与教程（目标，尚未实现）

手动检查、下载、安装和自动更新使用同一可恢复流程，但有不同授权：检查不下载/重启，下载不安装，安装只在用户允许的维护窗口和权限下执行。自动检查、自动下载、自动安装默认均关闭。每次操作记录当前/目标版本、说明、下载/校验/备份/排空/切换/健康检查、取消、失败、回退和人工处理状态；单安装锁阻止 UI、CLI、自动任务并发修改。

从 Full 到 Lite、从服务器到个人电脑/桌面、或反向切换时，先检查制品、环境、磁盘、权限、数据库/配置兼容、功能差异和活动订单/任务/订阅；数据不删除，数据库引擎迁移和跨机器迁移单列。旧程序不能安全读取新 schema 时，“回退程序”必须明确显示仍需恢复数据库或人工处理。

正式安装教程将提供中文和英文，并分别覆盖 Linux Full 服务器、Linux Lite 服务器、个人电脑 Lite、Windows/macOS Desktop 和实际宣布支持的其他平台。每个命令须标明系统、目录、权限和已验证版本；未发布包、未实现命令或未验证平台不得伪装为教程。安装/更新 UI 将显示版本、形态、安装方式、访问状态、数据位置、下一次计划、历史、备份/恢复和教程入口。

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

当前 Docker 镜像 workflow 会在推送符合 `vX.Y.Z` 的版本 tag 后自动运行，矩阵构建并推送
Full 与 Legacy LAN 两个多架构镜像；稳定版本分别更新各自的 `latest`，预发布 tag
不会覆盖 `latest`。也可以通过 `workflow_dispatch` 指定已有 tag 手动重跑，但必须在
`confirm` 输入中选择 `PUBLISH`；默认的 `NO` 会跳过所有推送 job。不要手动重跑旧的
`v0.1.1` tag；workflow 会校验
tag、`VERSION` 和 `package.json.version` 完全一致，不会移动既有 tag。构建前还会检查
GHCR 的版本和架构 tag；任一已存在就拒绝覆盖，必须创建新的 SemVer tag。稳定版
`latest` 与分支滚动 tag 是有意保留的可变别名，生产升级应使用版本 tag 或 digest。
CLI 会从 `package.json.version` 生成新项目的 Full 默认镜像，安装脚本会从根目录
`VERSION` 生成默认 tag；发布工作流会校验 tag、`VERSION` 与
`package.json.version` 的一致性，避免版本升级后初始化流程意外拉取旧镜像。当前
源码尚未创建对应的新 release tag 时，不要把 `v0.1.1` 当作当前源码镜像：该旧 tag
仍指向历史提交，开发验证应使用本地构建镜像或负责人确认后的新版本 tag。
GitHub Actions 当前只负责生成镜像，不直接连接或重启生产主机；部署端更新
`MYAPI_IMAGE` 后由 `myapi up` 或 `deploy/install.sh` 拉取新版本。

当前同一版本 tag 也会自动触发 Electron macOS/Windows 构建并上传 Actions artifacts；
Electron workflow 同样校验 push/manual tag 对应的提交以及 `VERSION`、
`package.json.version`，避免桌面安装包与发行版本错配；
只有在维护者显式填写 `PUBLISH` 且开启 release 环境变量时，才会附加到 GitHub
Release，不会因为推送 tag 自动发布桌面安装包。

正式发布前还必须确认：

- 工作树干净，tag、`VERSION` 与 `package.json.version` 一致；
- Go、relaykit、前端、普通构建和精简构建全部通过；
- 同一 source SHA 的 Release Manifest、NPM/source、OCI、原生与 Desktop 声明制品可相互校验；缺少制品或签名状态时不把该版本称为完整发行；
- tarball 中没有 `.env`、数据库、日志、OAuth JSON、token、缓存、`dist` 或
  `node_modules`；
- NPM scope、Trusted Publishing、2FA 和 provenance 已由维护者单独确认；
- 发布镜像的 registry、仓库和签名策略已明确填写，不使用未确认的默认远端。

`SOURCE_MANIFEST.json` 是生成文件，发布前重新生成并检查内容即可，不要把它或
生产数据加入 Git。

截至 2026-08-31，本地仍保留 `v0.1.0` 指向历史提交 `9c618d3`；对当前
`origin` 的只读核对未发现远端 `v0.1.0`，远端 `v0.1.1` 指向 `5007c6c`，而
当前 `main` 为后续提交。发布前必须再次核对远端标签状态；不得移动或覆盖已有
tag。下一次正式发布必须先确定新的版本号，创建指向当前提交的新 SemVer tag，再
重新运行全部发布检查。

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
