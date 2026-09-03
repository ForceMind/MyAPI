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

2026-09-03 只读核对：本机 macOS 26.2 arm64，Xcode CLT 已配置；Go 1.27.0、Bun 1.4.0、
Node 22.23.2 可用。最低/固定验证仍以 `go.mod` 和 CI 为准，不能用较新版本代替最低版本
保证。未发现 Docker CLI 或 `/Applications/Docker.app`，本机 Docker/Compose 演练仍缺前置
环境；GitHub CI 已恢复成功，与本机 Docker 安装是两个独立事项。

安装基础工具：

```bash
xcode-select --install
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
brew install git go node@22
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

Homebrew 的 `node@22` 是独立版本；在当前 shell 使用
`export PATH="$(brew --prefix node@22)/bin:$PATH"` 后再运行 Node/Bun 脚本。
不要把未锁版本的安装命令当作已符合 Go/Bun 基线，安装后必须核对实际版本。

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
npm run quota:openapi:check
npm run lan:check -- --skip-docker
npm run desktop:check
npm run upgrade:check
npm run runtime:check
npm run runtime:probe:test
npm run release:workflow:check
npm run pack:check
```

Docker 镜像测试使用 GitHub Actions 的 `Docker build smoke` workflow（手动触发）：它只在 runner
上构建并加载本地镜像（`push: false`），启动隔离 SQLite 容器检查 `/api/status`，不登录
GHCR、不创建 tag。现有 smoke 仅覆盖 LAN＋SQLite，不等于 Full/Compose/三数据库恢复。
`a36e529` 的 CI `33721694305` 已成功，历史 Billing/runner 故障不再是当前阻塞；本机仍
需 Docker Desktop 完成 Compose、资源和数据库副本演练。

2026-09-01 历史工具链备注：当时 `/opt/homebrew/bin/node` 启动时因 `merve` 依赖的
`simdutf` 动态库缺失而 SIGABRT，导致 `bun run typecheck` 和包含 `npm test` 的发行合同
无法启动。这是开发主机 Homebrew 运行时问题，不是前端断言失败；修复 Node/Homebrew
链接后应按本页命令重新执行类型检查、测试、构建和发行合同。

同日复核可使用已安装的 Node 22：
`PATH=/opt/homebrew/opt/node@22/bin:$PATH`。该路径下类型检查、前端测试（62 个文件/
280 个测试）、production build 和发行合同均通过；后续继续明确选择 Node 22。
当前 Node 22 可运行不等于默认 Node 或 Homebrew 动态库已永久修复，本轮未修改 shell 配置。

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

当前执行基线与授权见 [开发执行计划](DEVELOPMENT_EXECUTION_PLAN.md)：`6fabc98` 的
CI `33740901999` 五项均成功；`a620246`、`e0ca670` 及对应 runner 启动失败是
历史记录。Docker Desktop、真实手机、Windows 安装、MySQL/PostgreSQL 恢复、NOTICE/法律
及正式发布分别需要对应条件与授权，不能由本机静态合同替代。

S0＋S1 后续交付 `dc94e81` 已通过 CI `33740321133` 五项验证；本机 Go 全量测试、
vet/build、relaykit 独立构建/测试、Zhipu 与 profile 专项 race 通过，Node22 发行合同
通过（pack 必须在干净提交重新生成清单）。CI 另外实跑本次身份迁移/配置事务的
MySQL5.7 和 PostgreSQL9.6；它不是本机 Docker Desktop 验收，也不替代完整三库恢复。
后续本机与 CI 都按实际范围记录结果，不再统一写成“数据库完全验证”或“全部外部阻塞”。

S1-R1 本轮基线 `6fabc98`、CI `33740901999` 已核对。原六项已完成，用户已确认追加的
同进程配置保存/后台重载顺序修复已由 `8dfcfba` 交付，CI `33743669737` 五项成功；
本机 Go 全量/vet/build、relaykit 独立验证、四项 race 和干净树 release:check 通过。
验证使用低并行 Go 与临时 fixture，不操作本机生产数据。跨实例一致性、完整
数据库恢复、Docker Desktop 和真实设备验收仍按各自范围处理。

S2-A 当前获准范围见[执行矩阵](DEVELOPMENT_EXECUTION_PLAN.md#s2-a-支付与订阅事务)，
开始基线 `c82d0f1` / CI `33744326429`。仅使用临时 SQLite 和独立 CI 数据库 fixture，
不配置真实 Stripe/Creem/Pancake 凭据或回调。新实库测试要求显式
`MYAPI_S2A_DATABASE_TESTS=1`，DSN 为字面 loopback 且数据库名必须是 `myapi_s2a_test`；
不允许远端/其他数据库或已有业务表，不 drop 表，由一次性服务生命周期清理。
它只证明本批支付/订阅单连接与并发合同，不替代完整升级恢复、真实付款或本机 Docker Desktop 验收。

S2-A `2777021` 已通过本机 Go 全量、vet/build、专项 race、relaykit 独立验证和 Node22
完整 `release:check`。CI `33749764180` 六项 success，但 MySQL 实库日志出现中文插入
`Error 1366`，缺少日志断言使 job 假绿；该次未予验收。fixture 绕过了生产启动的中文
字符集检查，不能仅靠 DSN `charset=utf8mb4` 认定 schema 正确。

S2-A-R1 交付 `767b17b` / CI `33751536908` 六项成功，MySQL5.7/PostgreSQL9.6 专用空库
各七场景的中文日志、增量和历史不变断言通过，原始输出无旧错误。本机 model 全量/vet、
七场景 SQLite、专项 race、relaykit 独立构建及 Node22 完整 release:check 通过。
字符集 DDL 只作用于已校验的空库 `myapi_s2a_test`，不属于生产 schema 迁移。
SQLite 后三项为顺序重放，不替代实库并发；本批也不替代 Docker Desktop、完整升级恢复、
真实网关付款、跨实例部署或真实设备验收。

S2-C 从 `2d705a7` / CI `33752536294` 开始。Mac 上先跑 miniredis/SQLite 和内存上游
fixture；新增真实 Redis 7 CI 使用开发 Compose 同一主版本，要求
`MYAPI_S2C_REDIS_TESTS=1`、`MYAPI_S2C_REDIS_ADDR` 为字面 loopback 与有效端口，整个
实例必须为空，仅写 DB 15，不 FLUSH 或删除键。绝对过期时间由 PEXPIRETIME 检查，
测试不靠等待时间猜测 TTL 行为。`451bee3` / CI `33760303619` 七项成功，真实 Redis 36
场景全部执行且原始日志无 skip/Lua 错误；这不是本机 Redis/Docker 验收。
批量缓存损坏后的恢复策略未改变；不得使用生产 Redis 或旧数据库快照冒充安全恢复。

本机根模块全量/vet/build、专项 race、relaykit 独立 build/test、Node22 release:check
及最终源码 pack 已通过。零差额饱和审计使用已有 System 类型，不增加消费 RPM/TPM 或
导出数据；关闭消费日志时仍保留异常审计。C03b 与任务持久化恢复尚未完成。

### B1 与无发布 Docker smoke（进行中）

B1 在现有 TaskBillingContext JSON 内记录版本及完整性，不新增 SQL 列；本机验证提交时
实际倍率、SQLite 往返与结算/审计。现有 `TestS2APaymentConfiguredDatabases` 专库入口
增加任务快照往返场景，继续使用严格空库/loopback 安全门，不连接生产补验。

Mac 当前无 Docker CLI/App，S4-01 使用已授权的 GitHub `Docker build smoke` 手动入口：
分别构建 Full/LAN 的 linux/amd64 镜像，load:true/push:false，无 registry 登录。
BuildKit 限 2 CPU/4 GiB，应用限 1 CPU/768 MiB，矩阵串行；仅发布 runner 回环端口，
SQLite 放容器临时挂载，生成合成管理员凭据不输出/不上传。新脚本拒绝已初始化或非
SQLite 目标及重定向；浏览器只允许同源请求，核对当前 SHA 的实际前端构建标识。
Node22 `npm run runtime:probe:test`、YAML/Bash 语法已通过；需推送后手动 dispatch 并核对
精确 headSha、两个 edition 的实际步骤/报告。镜像尚待验证，不以本地单测代替。
这只是新安装/认证/前端启动，不是完整 Key/Provider 流程、三库恢复或 Docker Desktop 验收。
