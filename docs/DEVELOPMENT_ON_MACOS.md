# MyAPI macOS 开发环境

本文说明如何把 MyAPI 的开发从 Linux 服务器迁移到 macOS。macOS 是源码、测试、
Electron 和 LAN Lite 试用的主机；Linux 服务器只作为远程仓库、CI/GHCR 或经明确
批准的部署环境。不要从服务器复制 `.env`、数据库、日志、OAuth JSON、API Key、JWT
或 `SESSION_SECRET`。

## 推荐环境

- macOS 13 Ventura 或更新版本；Apple Silicon（M 系列）优先，Intel 也受支持；
- Xcode Command Line Tools；
- Homebrew（Apple Silicon 通常在 `/opt/homebrew`，Intel 通常在 `/usr/local`）；
- Docker Desktop for Mac，使用 Compose v2.17 或更新版本；
- Go 版本以 `go.mod` 为准（当前 `1.25.1`）；
- Bun 使用项目 lockfile 对应版本（CI 当前 `1.3.14`）；
- Node.js `22.x`，用于 CLI、脚本和 Electron 测试。

安装基础工具：

```bash
xcode-select --install
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
brew install git go node
curl -fsSL https://bun.sh/install | bash
```

重启终端后确认版本：

```bash
sw_vers
uname -m
git --version
go version
node --version
bun --version
docker compose version
```

Docker Desktop 设置建议：给 Docker 至少 4GB 内存，构建时通过项目参数限制并行度；
不要共享整个用户目录、`~/.codex`、Keychain 导出目录或其他凭据目录。

## 克隆私有仓库

建议将源码放在本机普通目录（例如 `~/src/MyAPI`），不要放在 iCloud Drive、Dropbox、
OneDrive、网络盘或带自动同步的目录中：

```bash
mkdir -p ~/src
cd ~/src
git clone https://github.com/ForceMind/MyAPI.git MyAPI
cd MyAPI
git status --short --branch
git remote -v
git log --oneline --decorate -5
```

私有仓库认证使用 GitHub CLI、SSH agent 或 macOS Keychain；不要把 PAT 写入 remote URL、
脚本、`.env` 或 Codex 提示词。一个工作树只由一个 shell/编辑器负责写入，避免多个
终端同时执行 checkout、格式化或依赖安装。

## 依赖、测试和构建

```bash
cd ~/src/MyAPI/web
bun install --frozen-lockfile
bun run typecheck
bun run test -- --run
bun run build
cd ..
go mod download
go test ./relay/channel/gemini ./relay/channel/claude
```

发行合同检查（从仓库根目录）：

```bash
npm test
npm run brand:test
npm run brand:check
npm run website:check
npm run lan:check -- --skip-docker
npm run desktop:check
npm run upgrade:check
npm run runtime:check
npm run runtime:probe:test
npm run release:workflow:check
npm run pack:check
```

MacBook 资源有限时使用 `MYAPI_BUILD_PARALLELISM=1`、`GOMAXPROCS=1`，不要并行运行多
个完整前端构建或 Docker 构建。Linux 专用的 `taskset` 不适用于 macOS；Docker Desktop
的 CPU/内存限制和项目的 `MYAPI_CPU_LIMIT`/`MYAPI_MEMORY_LIMIT` 是主要资源边界。

## 本机 Full 运行

不要复制服务器配置。在仓库中生成独立开发配置：

```bash
cp deploy/.env.example deploy/.env
```

本机 HTTP 调试至少设置：

```text
MYAPI_IMAGE=local/myapi:dev
MYAPI_EDITION=full
MYAPI_BUILD_LOCAL=true
MYAPI_BIND_ADDRESS=127.0.0.1
MYAPI_ALLOW_LAN=false
MYAPI_SESSION_COOKIE_SECURE=false
MYAPI_PORT=3000
MYAPI_PUBLIC_URL=http://127.0.0.1:3000
MYAPI_DATA_DIR=./data
MYAPI_LOGS_DIR=./logs
SESSION_SECRET=<只写入本机 .env 的随机字符串，至少 48 个字符>
```

启动与停止：

```bash
node cli/myapi.mjs build --project-dir .
node cli/myapi.mjs up --project-dir .
curl -fsS http://127.0.0.1:3000/api/status
node cli/myapi.mjs stop --project-dir .
```

登录管理员后，概览页的「账户额度变化趋势」面板会在每分钟变化列表下显示
「Codex · Usage trend」趋势卡；有成功采样时绘制选中 Codex 账户/窗口在最近 24 小时的
可用额度百分比，多个账户或窗口可以在图表右上角切换。首次使用若显示暂无历史，请在
渠道页成功查询一次 Codex 用量，或等待已启用的后台采样器完成下一次采样。

`MYAPI_SESSION_COOKIE_SECURE=false` 只用于本机 HTTP；真实 HTTPS 部署必须恢复安全值。
开发数据和日志只留在本机目录；不要使用 `down -v`，不要把数据库、日志、`dist`、
`node_modules` 或 `.env` 加入 Git。

## LAN Lite 试用

LAN Lite 不读取 macOS Keychain、`~/.codex`、Claude/Antigravity 凭据文件或任何本地
授权文件。上游渠道在 MyAPI 管理界面显式配置，同事使用各自的下游 API Key。

源码 checkout 尚未正式发布 NPM 时使用：

```bash
node cli/myapi.mjs lan init "$HOME/Library/Application Support/MyAPI/lan-project"
node cli/myapi.mjs lan start \
  --project-dir "$HOME/Library/Application Support/MyAPI/lan-project"
```

默认只监听 `127.0.0.1`。确认可信私网后才显式开放：

```bash
node cli/myapi.mjs lan start \
  --project-dir "$HOME/Library/Application Support/MyAPI/lan-project" \
  --bind-address 192.168.1.20 \
  --port 3000 \
  --allow-lan
```

更改绑定地址、端口或 LAN 开关必须停止后重新启动；不要把端口直接暴露到公网。
在第二台机器调用 `/api/status` 和受限下游 Key，记录结果时不要记录任何密钥。
完整规则见 [LAN Lite 指南](./LAN_LITE.md)。

## Electron macOS 开发和打包

```bash
cd ~/src/MyAPI/electron
npm ci
npm test
npm run build:mac
```

打包前从仓库根目录运行 `npm run desktop:check`。DMG/ZIP 产物在 `electron/dist/`；没有
Apple Developer ID 签名和 notarization 时只是测试制品，可能触发 Gatekeeper。Windows
安装包应交给 GitHub Actions 的 Windows runner 或真实 Windows 设备构建和验收，macOS
不能证明 Windows 安装体验。

## GitHub Actions、GHCR 和服务器边界

- 普通 `main` 提交只运行 CI/合同检查，不发布镜像；
- 只有新的、未占用的 SemVer tag（例如 `v0.2.0`）自动构建 Full/LAN 多架构 GHCR 镜像
  和 macOS/Windows Electron 制品；不得移动或覆盖 `v0.1.0`/`v0.1.1`；
- GHCR manifest 使用经过校验的不可变架构 digest，已存在 tag 会 fail-closed；
- macOS 开发机不保存服务器 `.env`，不直接重启 Linux 服务器；升级先在副本运行
  `myapi upgrade --dry-run`，实际升级需要负责人批准；
- NPM 发布、GHCR 发布、生产切换、签名、notarization、法律审查不是开发命令的隐式副作用。

参考：[发行指南](./MYAPI_DISTRIBUTION.md)、[升级演练](./UPGRADE_REHEARSAL.md)、
[真实设备验收](./REAL_DEVICE_ACCEPTANCE.md)、[总体计划](./MYAPI_MASTER_PLAN.md)。

## 迁移完成检查表

截至 2026-08-31，本仓库当前工作树已完成 macOS Bash 3.2 installer 兼容修复、`MYAPI_PORT` 边界校验和计费 quota saturation 防护；CLI/LAN/Desktop/Upgrade/Runtime/Release 合同与 Node 22、Go 定向回归已有本机证据。以下清单仍按“新 Mac 实际环境/真实设备”勾选，不以静态合同测试代替外部验收。

- [ ] Xcode CLT、Homebrew、Go、Bun、Node、Docker Desktop 版本已记录；
- [ ] 仓库在 `~/src/MyAPI` 等非同步目录，remote 指向 `ForceMind/MyAPI`；
- [ ] 前端 typecheck、Vitest、build 和 provider/额度 Go 测试通过；
- [ ] CLI、品牌、官网、LAN、Desktop、升级和打包合同通过；
- [ ] 本机 `.env`、数据库、日志、凭据和构建产物未进入 Git；
- [ ] Full/LAN Lite 默认回环，开放私网时明确使用 `--allow-lan`；
- [ ] Electron DMG/ZIP 未签名时只作为测试制品；
- [ ] 任何服务器升级、GHCR/NPM 发布和生产操作均另行确认。

当前阶段边界：本轮工作树修改尚未推送到 GitHub；完成阶段后应先提交并由负责人确认是否允许 push。即使提交后，GitHub runner/Billing、Docker Desktop、真实手机、Windows 设备、PostgreSQL 恢复、NOTICE/法律审查及正式发布仍分别需要对应外部条件和明确授权。
