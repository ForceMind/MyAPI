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
上构建并加载本地镜像（`push: false`），启动隔离 Full/LAN SQLite 容器验证状态、登录、
合成计费与日志合同，不登录 GHCR、不创建 tag。现有 smoke 不等于本机 Compose、三数据库
恢复、真实 Provider 或真实设备验收。
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

登录管理员后，概览额度面板应直接显示最近观测账户的剩余额度、最近区间速度、最近
一小时有效观测的每分钟/小时平均、预计耗尽或先重置状态、覆盖率和简洁折线；概览不再
出现范围、颗粒、指标、图形或算法选择器。完整控制位于「管理员 → 渠道」：可选择
1h/6h/24h/7d/30d/90d/自定义范围、raw/minute/5m/15m/hour/day/week/auto 颗粒度、
available/used/total/consumption/rate 指标、折线/面积/柱状，以及 latest interval、
observed window、EWMA、独立分析窗口和半衰期。

本机回归使用临时 Playwright 1.61.1 和系统 Chrome 加载真实 `web/dist`，验证概览单次
有界查询、48 点分段折线、详情请求参数、最新失败、320/390 像素和低高度滚动。真实账号
仍需等待至少两个成功采样再核对；合成 fixture 不证明生产数据正确。

`MYAPI_SESSION_COOKIE_SECURE=false` 只用于本机 HTTP；真实 HTTPS 部署必须恢复安全值。
开发数据和日志只留在本机目录；不要使用 `down -v`，不要把数据库、日志、`dist`、
`node_modules` 或 `.env` 加入 Git。

## Full 原生进程导入本机 Codex

只有直接运行在 macOS 上的 Full 进程可以自动检查该进程用户的 Codex 登录文件。默认
读取 `~/.codex/auth.json`；若启动 MyAPI 时显式设置了绝对路径 `CODEX_HOME`，则读取
`$CODEX_HOME/auth.json`。接口不接受任意路径，不写回原文件，也不在响应、日志或审计中
返回 token。自动导入要求 Root 的实时 dashboard 会话、同源请求和 Passkey/2FA 安全复核。

Docker Desktop 中的 MyAPI 只能看到容器文件系统。官方镜像固定
`MYAPI_RUNTIME_ENV=container`，因此页面应显示“无法访问宿主机 Codex 登录”，不能解释成
宿主机未登录，也不要为此挂载整个 home 或 `.codex` 目录。需要在容器/LAN 环境配置账号时，
使用管理界面的显式手工凭据或现有 ChatGPT OAuth 流程。

自动化测试必须使用临时 `CODEX_HOME` 和合成 JWT；不得读取开发者真实 `auth.json`、
macOS Keychain 或其他凭据库。真实账号验收只能由用户在本机界面明确触发。

渠道编辑器中的「导入本机 Codex」向导先显示脱敏账号，再进入 Passkey/2FA 安全复核和
后端导入；容器/文件不可用时可以切到网页 OAuth 或手工模式。手工模式给出 macOS/Linux
和 Windows Node 命令，粘贴内容只保存在组件内存，切换、取消、关闭或成功后清空。
390 像素浏览器回归已验证输入框获得焦点、弹窗内部可真实滚动到底部操作且无横向溢出；
这仍不等于 macOS/Windows 安装包实机验收。

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

### B1、C06 与无发布 Docker smoke（2026-09-04 验收）

代码 `7f1913e` 的 CI `33781560507` 七项成功、Docker smoke `33781637372` Full/LAN
两项成功。B1/C06/S4-01 当前范围完成，详细命令、失败历史和 image ID 见
[完成度审计](COMPLETION_AUDIT.md#s2-b1-与-s4-01)。这些结果不替代完整 Key/Provider
流程、三库恢复、Docker Desktop、arm64 或真实 macOS/Windows/手机验收。

B1 在现有 TaskBillingContext JSON 内记录版本及完整性，不新增 SQL 列；三库快照往返、
显式零费率与结算/审计已验证。现有 `TestS2APaymentConfiguredDatabases` 继续使用严格
空库/loopback 安全门；最终 MySQL5.7/PG9.6 各五种快照和 NULL 读回、原七支付场景均实跑通过。
`e7fffc2` 曾出现 PG SQLSTATE 22P02；真实 GORM 参数与 pgx codec 红测定位到 Valuer
字节数组被编码为 bytea，已改传 JSON 文本。保留 NULL/错误、schema、协议和旧读取兼容，
不是给 fixture 补假数据。本机 codec 只作诊断，验收以最终原生 CI 为准。

Mac 仍无 Docker CLI/App，测试使用已授权的 GitHub 手动入口：两种 edition 分别构建，
linux/amd64、load:true/push:false，无 registry 登录。BuildKit 限 2 CPU/4 GiB，应用限
1 CPU/768 MiB，矩阵串行；只暴露 runner 回环端口、临时 SQLite 和合成管理员。
拒绝已初始化、非 SQLite 及重定向目标；浏览器仅同源，实际登录表单就绪后校验
三处精确 SHA revision。成功项不再携错误码，登录要求 HTTP 2xx＋success:true＋非空 token。
凭据、原始响应和数据库不上传，容器清理核对本次 SHA 标签。

首轮 `540cf32` / `33766140801` 两镜像因构建标识丢失失败，记录未删除；直接 env 属性
读取修复后，最终镜像已通过。前端本机 `bun run test --maxWorkers=2` 为 64 文件/336 项
通过（101.96s），typecheck、文件 oxlint 与 production build 通过。合成构建命令
`VITE_REACT_APP_VERSION=0.1.1 VITE_BUILD_ID=fixture-s4-build RAYON_NUM_THREADS=2 bun run build`
耗时 6.64s，仅证明本地注入，不冒充提交 SHA，不提交 dist。

日志计数/轮转状态和轮询 fixture 竞争已修，保留并发行为、共同 500ms 门限与原日志格式。
本机根模块全量/vet/build、relaykit 独立 build/test 及独立复审通过；此前整合 race 通过，
但封版按 7f1913e 追加重跑时 Kling 测试清理与后台 cache 回调竞争。测试生命周期及 CI
race 接线已修；本机四包同一命令和独立复审通过。`6fd8ae4` / CI `33783792231` 七项成功，
新增 backend race 原始日志确认四包实跑，无 DATA RACE/FAIL；没有用前一次绿色替代失败。
最终代码的 Node22 release:check 全链通过：探针 13 项、干净源码 manifest、pack 2190 文件。
### S4-02 合成业务链（已完成当前范围）

已实现 Full/LAN 测试用的合成普通用户、受限 Key、普通 OpenAI 渠道和固定 usage=10+5
假上游。sidecar 使用 Dockerfile 已有的固定 Bun digest，与应用共享网络 namespace；两个
宿主端口均只绑定 127.0.0.1。SQLite/full-content 日志位于应用 `/data` tmpfs，sidecar
只读且受 UID/capability/CPU/内存/PID 限制；没有 Redis、真实 Provider 或本机凭据读取。

Node22 `npm run runtime:probe:test` 最终为 17/17；Bun 假上游已在本机 127.0.0.1:19090 实际
启动，health 与零状态读回后停止。Ruby YAML、全部 workflow shell 块、runtime/release
workflow 合同和独立审查通过。本机仍无 Docker；容器网络、资源余量及 Full/LAN 的
API/账务/日志合同由后述 GitHub 临时 runner 验收，不能写成本机 Docker Desktop 通过。

首个 `b54ce36` Full job 的 app/sidecar 均运行，但宿主 NAT 请求无法命中只绑定 namespace
loopback 的 fake，连续 connection reset，业务探针未开始；LAN 随后取消以节省资源。
仅 CI sidecar 改为显式监听 0.0.0.0，宿主端口仍绑定 127.0.0.1，本机默认监听不变。
监听修复的 Node 17 项、YAML/Bash 和独立复审通过。

最终 `237c0da` 本机完整 `npm run release:check` 通过，包检查为 2191 文件/
20,024,635 bytes；[CI 33790468336](https://github.com/ForceMind/MyAPI/actions/runs/33790468336)
七项成功。[Docker 33790513455](https://github.com/ForceMind/MyAPI/actions/runs/33790513455)
在精确同一 SHA 上 Full 4m21s、LAN 4m30s 均成功，容器联通、合成 15 quota 账本、日志
权限与脱敏、匿名不上游、真实登录表单、三处一致 revision、image identity 和 SHA 清理均
通过；失败诊断因无失败而跳过。Full/LAN image ID 分别为
`sha256:5bff88fc38c973f86d0966aff1539ddae5e1c658627a7bb7a9e8bf471343d1ae` 与
`sha256:6eb322e85047c9bb6f5087717527c5b9b0e1d91cf3c631fff7fb373f539ba1ce`。

这是测试工具与 workflow 扩展，不改产品页面；可见发行版本保持 0.1.1，GitHub 镜像用
提交 SHA 作为构建标识；临时 image ID 不是发布 digest。C03b、B2/B3、三库完整恢复、
本机 Docker Desktop 和真实设备仍单列。

### S2-D01 OAuth JSON wrapper（已完成当前范围）

根模块生产代码结构扫描从 67 个直接 JSON 序列化调用/27 文件开始；测试、wrapper 实现、
relaykit 及合法类型/`json.Valid` 使用不计。D01 将 GitHub、Discord、OIDC、Linux DO 四个
适配器的 9 处调用统一到 `common.Marshal` / `common.DecodeJson`，保持非严格单值 decoder
语义；当前余 58 处/23 文件。GitHub 回归通过合成 `http.DefaultTransport` 截取固定 URL，
不绑定端口、不读取真实 OAuth 配置或发起外网请求，测试结束恢复全局状态。

本机低并行实际通过 `go test ./common ./oauth`、`go test -race ./oauth`、
`go vet ./oauth` 和 `go test -p 1 ./... -count=1`。Go 首次编译需要写用户 build/module cache；
受限沙箱拒绝该写入时应在获准的本机执行环境运行，不能把权限错误写成测试失败。
独立审查无 P1/P2；`6b3042a` / [CI 33793219733](https://github.com/ForceMind/MyAPI/actions/runs/33793219733)
七项成功，D01 当前范围完成。该批不改前端页面，版本保持 0.1.1；后续 D02–D10 逐批处理，
不让 `relaykit` 导入根模块 `common`。当前镜像验收仍对应 `237c0da`，没有把常规 CI 写成
`6b3042a` 的 Docker 证明。

### S2-C07 / D02 视频正文缓存与 Provider 边界（已完成当前范围）

Kling/Jimeng 兼容 adapter 过去先缓存原始正文，再只改直接 Body；后续日志、Distributor、
controller 和 validator 仍优先读旧 `KeyBodyStorage`，导致统一 envelope 不可达。现在通过
`common.ReplaceRequestBody` 原子同步 storage、Body、GetBody、ContentLength，替换成功后
立即关闭旧 storage，最终由 `BodyStorageCleanup` 幂等清理；内存与强制磁盘路径均有测试。

两路由在 Token 鉴权后先记录原始客户端请求，再转换并分发；日志所有阶段冻结入口身份。
metadata 不再获得第二个模型或资源乘数入口：Kling/Jimeng 出站模型固定为已映射模型，
Kling mode/duration/image 和 Jimeng frames 由已验证顶层字段决定。Jimeng 官方 frames 只接收
121/241，并正规化为 5/10 秒。两个 middleware 的 JSON 调用也已使用 `common.Marshal`，
全仓结构余量由 58/23 降为 56/21。

本机在允许 Go cache、临时文件和回环 fixture 的环境中通过受影响包、common/middleware
全包 race、受影响 vet 与根模块低并行全量测试。首次沙箱 cache/监听拒绝不是代码失败；
红测和两轮独立 Sol 复审记录见完成度审计；`f7cc5c3` /
[CI 33798508808](https://github.com/ForceMind/MyAPI/actions/runs/33798508808) 七项成功，C07/D02
当前范围完成。未连接真实 Provider、数据库或凭据，不证明真实任务费用/结果；Jimeng 查询
handler 候选另审。该批无页面改动，版本保持 0.1.1；最新 Docker 验收仍对应 `237c0da`。

### S2-D03 Relay 输入 JSON wrapper（已完成当前范围）

OpenRouter Anthropic thinking、Replicate output format 和模型映射的三个直接解码已等价改为
`common.Unmarshal`；OpenAI 仍保留 RawMessage 类型 import，model helper 使用明确别名。
确定性测试锁定 thinking 门控/错误、Replicate 静默忽略与 Extra 覆盖、模型映射链/循环/
空值及错误部分状态，不连接网络或数据库，也不修改全局模型设置。

本机低并行三包普通测试、同三包 race、vet、gofmt 和 diff-check 均通过；全仓结构余量
由 56/21 降至 53/18，独立 Sol 审查无 P1/P2。`615fbbd` /
[CI 33800052236](https://github.com/ForceMind/MyAPI/actions/runs/33800052236) 七项成功，D03 当前
范围完成。该批不改页面或版本，不触碰 relaykit，不用常规 CI 冒充 Docker/真实 Provider
验收。

### S2-D04 Provider 响应与 Vertex token（已完成当前范围）

SiliconFlow rerank、Tencent 非流响应和 Vertex token 的五处 JSON 调用已统一到 `common`
wrapper。Vertex 两个 exchange 共用离线可测的安全 parser：非 2xx/provider error、畸形或
缺失/错误类型/空白 token 只产生固定错误，不把响应正文、token、description 或 map 返回给
客户端/日志；合法首个 JSON、未知字段和尾随值的宽松语义保留。

三包普通与 race、vet、gofmt、diff-check 均通过，结构余量由 53/18 降至 48/15，独立 Sol
审查无 P1/P2。测试只构造内存 HTTP response，不生成真实 JWT、不读取 service account、
不连接 Google/代理/cache。`544f83b` /
[CI 33802572396](https://github.com/ForceMind/MyAPI/actions/runs/33802572396) 七项成功，D04 当前
范围完成；无页面/版本/relaykit 变化，常规 CI 不能替代真实 Vertex 账户或代理验收。

### S2-D05 Midjourney JSON wrapper（已完成当前范围）

`relay/mjproxy_handler.go` 的 8 处 JSON 调用已统一到 `common` wrapper，保留 Notify 忽略
marshal error、持久字段 malformed 静默和历史错误字符串。测试使用单连接内存 SQLite，
结束时恢复 `model.DB` 并关闭本地连接；实际验证空 VideoUrls 持久化、单 object/条件 array/
空 `[]`、camelCase 和 JSON Content-Type，不访问真实 Midjourney、渠道或凭据。

本机 relay 定向、全包普通/race、vet、gofmt 和 diff-check 通过，结构余量由 48/15 降至
40/14，独立 Sol 审查无 P1/P2。`b2b60fd` /
[CI 33804146311](https://github.com/ForceMind/MyAPI/actions/runs/33804146311) 七项成功，D05 当前
范围完成；无页面/版本/relaykit 变化，不把 SQLite fixture 写成三数据库或真实上游验收。

### S2-D06 Controller JSON wrapper（已完成当前范围）

Controller 三文件的 8 处编解码已统一到 `common` wrapper，合法 `json.Valid`/RawMessage
继续保留；规则模型 endpoint 并集在编码前稳定排序。Vertex key 和 Uptime helper 测试只用
内存数据与实例级 HTTP transport，不连接外网或修改全局 client。

本机定向、Controller 全包普通/race、vet、gofmt 和 diff-check 通过，结构余量由 40/14
降至 32/11，独立 Sol 审查无 P1/P2。`02a0aaf` /
[CI 33805743908](https://github.com/ForceMind/MyAPI/actions/runs/33805743908) 七项成功，D06 当前
范围完成；没有新增 Ollama/endpoint 完整集成测试，不把机械 wrapper 与全包回归写成真实
Uptime/Ollama 运行验收。无页面/版本/relaykit 变化。

### S2-C08 / D07 Settings（已完成当前范围）

Settings 五个目标文件的 13 处直接 JSON 调用已清零，根余量为 19 处/6 文件。该批还覆盖设置
fresh 发布、RWMap/10 倍率、集合 `null` 规范化、负倍率/NaN/Inf 拒绝、模型 DB 前完整验证和
批量 rate 聚合、generic config 全对象原子验证、Claude/Gemini 语义及 ConfigManager registry
回调锁。模型成功请求限流使用单请求快照；0 分钟不能在 Enabled 状态启用；成功才写入额度；
动态窗口按每个 key 自身过期并在 count 缩小时 prune，不做巨额预分配。

本机已通过受影响包、同 CI race、限流 race `-count=2`、低并行根 test/vet/build、relaykit
独立 vet/build/test、gofmt、diff-check 和 YAML 解析。独立 Sol 复审无 P1/P2。最终 `2d6acab` /
[CI 33814136556](https://github.com/ForceMind/MyAPI/actions/runs/33814136556) 七项成功，新增 D07 十包 race 原始日志均为 `ok`。
无页面/schema 改动，版本保持 0.1.1；没有真实上游、生产、真实设备或发布验证。同 SHA 常规
MySQL/PostgreSQL CI 成功，但本批无专用三数据库 Settings 行为场景，不替代热更新验收。
C09 的热读统一快照/锁、硬限额 reservation/rollback、
跨族 reload 事务与历史数据审计未在本批解决；C09 只读设计已完成、实施仍待分批收敛；D09 与 D10
均已完成当前范围并通过同提交 CI。

### S2-C09-R1（已完成当前范围）

最终 `ae07527` / [CI 33824814509](https://github.com/ForceMind/MyAPI/actions/runs/33824814509)
八项成功。payment compliance 的五字段统一由一次 `UpdateOptionsBulk` 提交；SQLite 回归覆盖
成功写入和保留旧值的 rollback，同一 helper 已由 MySQL 5.7/PostgreSQL 9.6 CI engine 流程执行。工具价格以 source 与 index
不可变同代际原子发布，公开 DTO 不变；严格 `MapConfig` 与历史宽松 loader 明确分离。`ConfigManager.SaveToDB`
先完成完整快照，随后锁外 callback，覆盖重入和错误释放。Redis limiter 不再把第一个 client 固化为 singleton；
`redis.Script` 可在 `NOSCRIPT` 后恢复，TTL 自然复满、无 24 小时截断；Go 在触碰 Redis 前拒绝非正、超过
2^53 或 `Requested > Capacity` 的参数，Lua 还在写前拒绝非整数，且拒绝路径零写入。`common/limiter` 导出并复用 `MaxExactInteger`/
`ValidateConfig`；Settings 默认及每个 group 在 DB 写入前以实际 `capacity=total*durationSeconds`、`rate=total`、
`requested=durationSeconds` 共用 2^53-1 精确边界，覆盖大于 2^53 且不超过 MaxInt64 的拒绝，并保证 runtime/OptionMap
不发布；`total=0` 与 disabled `duration=0` 保留。miniredis 已通过；独立真实 Redis 7 CI job 已实跑。独立最终复审
确认当前范围无 P1/P2。

本机已运行扩展后的 Settings/C09 同 CI race、低并行根模块全量 test、vet、build，以及 relaykit 的
`GOWORK=off` vet/build/test；gofmt、diff-check、YAML 与根 JSON 静态门禁通过。没有在本机连接 Redis、MySQL
或 PostgreSQL；同提交 GitHub CI 已实际验证。原始 Redis 日志包含短/25 小时 TTL、Go/Lua 拒绝与
`SCRIPT FLUSH` 恢复，S1 Job 的 MySQL/PostgreSQL engine 均通过，Backend 扩展 race 的 11 包均为 `ok`。

未在本轮关闭的合同：成功限额仍是 check→execute→record，未实现 reservation/rollback；总量继续令牌桶，产品
语义不变；generic 21 模块热读、跨族事务、Passkey/null/未知 key/`GroupRatioSetting` 审计仍待；payment runtime
逐字段读取及活指针未解决；未运行真实付款、生产、真实设备或发布。`VERSION` 保持 0.1.1。

### S2-C09-R2（已完成当前范围）

最终 `2575f5b` / [CI 33828024982](https://github.com/ForceMind/MyAPI/actions/runs/33828024982) 八项成功。Backend 原始日志确认
改名后的 `Verify settings, rate-limit, and request snapshots` 12 包均为 `ok`，包含 `setting/model_setting`、
`operation_setting`、`relay/common`，不是 no tests；其余七个 job 均成功。`ValidatingMapConfig` 仅纯验证，既有 `MapConfig` 保持 unsupported。Claude 私有 atomic 完整代，getter
深拷贝三层 map/slice 并保留 `[]`/`null`；`null`/`{}` 只在读取副本补 8192，不污染 export，严格失败不发布。`GenRelayInfo`
捕获 request-private Claude 代，handler/header/converter 同代。Monitor 私有 atomic 代保留 env frequency 再 enabled
优先序；env 移除恢复 DB，mode/concurrency 仅 effective 不污染 export，`runChannelTestTask` 一次快照且并发 partial 不丢。
测试使用单次屏障，不使用 sleep 或概率循环。独立 Sol 最终复审当前范围无 P1/P2；P3 为 `GlobalConfig.Get` 动态具体类型变为
私有 manager，公开 DTO/getter 签名兼容、仓内无断言。

本机通过受影响包普通、workflow 同款 12 包 race（`common`、`common/limiter`、`types`、五个 setting 相关包、`model`、
`middleware`、`controller`、`relay/common`）、`go test -p 1 ./...`、vet、build、relaykit `GOWORK=off` vet/build/test、
gofmt、diff、YAML/JSON 门禁。R2 不需且未做真实 Redis、三数据库、前端、上游；无页面/schema，`VERSION` 0.1.1。
Passkey/ServerAddress、payment runtime/密钥轮换、`GroupRatioSetting` alias/前端 bulk、成功 hard limit、跨族事务仍待。
R1 文档收尾 `3af738e` / [CI 33825533002](https://github.com/ForceMind/MyAPI/actions/runs/33825533002) 八项成功。
R2 文档收尾 `3c63e02` / [CI 33828752473](https://github.com/ForceMind/MyAPI/actions/runs/33828752473) 八项成功。

### S2-C09-R3（已完成当前范围）

最终 `e3cd185` / [CI 33831492021](https://github.com/ForceMind/MyAPI/actions/runs/33831492021) 八项成功。原始 S1 日志明确
MySQL 5.7 与 PostgreSQL 9.6 均执行 `group-ratio-alias-contract/create-rollback/{mixed-existing-canonical-and-missing-alias,both-missing-second-create}`，
全 PASS、无 skip；Backend `Verify settings, rate-limit, and request snapshots` 13 包均为 `ok`，含 ratio/model/controller/service/relay-common。
GroupRatio 三图通过私有 writer 加 atomic 单快照发布，嵌套深拷贝；detached 公开 DTO 保持三字段
unkeyed/JSON 兼容、receiver-local，NaN/Inf 导出错误传播；注册表动态类型改为私有 manager，公开 DTO/函数不变且仓内
无生产类型断言。special 空 user/target 及 direct、`+:`/`-:` 同目标冲突均写前拒绝；
service 使用 detached special getter，`+`/`-` 语义保持。当前 UI 的平面 `GroupRatio`/`GroupGroupRatio` 为 canonical，分层键兼容；
新 JSON 语义规范化后事务双写，bulk 冲突写前拒绝，OptionMap/runtime 双键同值。历史加载以有效 canonical 优先，invalid canonical
fallback 有效 alias；双方 invalid 留最后有效 runtime、显式 warning、不改 DB。SQLite 覆盖反向行序、alias-only/conflict、
update/mixed/create rollback；同一合同接入现有 MySQL 5.7/PostgreSQL 9.6 gated 子测试，并已由同提交 CI 通过。

独立 Sol 最终复审无 P1/P2/P3。本机通过 ratio/model/service/controller 普通、workflow 同款 13 包 race、
`go test -p 1 ./...`、vet、build、relaykit `GOWORK=off` vet/build/test、gofmt、diff、YAML/JSON 门禁。首次 service/controller
race 只因磁盘满链接失败；仅清理 7.9GB 可重建 `/private/tmp/myapi-gocache` 后原命令通过，未触碰仓库/DB。R3 不完成前端 bulk/
跨多 HTTP 事务、历史 DB 清理、payment/Passkey/hard limit/global config；无页面/schema，`VERSION` 0.1.1。

### S2-D08 io.net 核心（已完成当前范围）

`pkg/ionet/client.go`、`pkg/ionet/jsonutil.go` 的各 4 处实际 stdlib JSON 调用已迁移至 `common`
wrapper，根余量由 19/6 降为 11/4。无网络 fake client 覆盖请求 body/headers/method/URL、NaN marshal、
transport/API detail fallback、query slices/HTML escape/空值/零值/false/`time.Time`/`*time.Time`；
flexible time 覆盖对象/数组、直接或 `data` 包装、无时区 UTC、带时区 offset、未知/普通字符串、malformed、
错误类型与尾随值。普通测试、race `-count=2`、vet、gofmt、diff-check，根模块全量 test/vet/build 及
relaykit 独立 vet/build/test 已通过。最终 `9193ada` /
[CI 33816756504](https://github.com/ForceMind/MyAPI/actions/runs/33816756504) 七项成功。

独立 Sol 审查无 P1/P2，数组和 `*time.Time` 两个 P3 已补。保留 `interface{}`→`float64` 大整数精度风险与
递归时间字符串识别的既有语义；endpoint path 逃逸、nil response 等 D09 边界另审。C09 设计仍独立保留，
D09 已完成本地范围并待同提交 CI，D10 为下一批。本批无页面/schema、真实 io.net、凭据或网络，`VERSION` 保持 0.1.1。

### S2-D09 io.net endpoints（已完成当前范围）

`pkg/ionet` 的 container/deployment/hardware 三文件 9 处直接 `Unmarshal` 已迁移到 `common.Unmarshal`，
根模块余量由 11 处/4 文件降至 2 处/1 文件（仅 D10 的 `pkg/cachex` codec）。动态 deployment/container/
cluster path segment 均使用 `PathEscape`，stream options 局部复制；`makeRequest` 在 nil、仅 2xx 成功、非 2xx
固定 `APIError`（不回显 raw/detail）上的边界已覆盖，controller 以 `errors.As` 分类。默认 HTTP client 禁止
301/302/303/307/308 重定向，防止 `X-API-KEY` 和敏感 body 泄露。

离线 fake 和 loopback `httptest` 覆盖合法/malformed、mutation/hardware/location、null/空/缺 ID 防伪成功、
合法 `false`、五类重定向、path 逃逸和 stream options 不变性；没有真实 io.net、凭据或公网 I/O。本机
`pkg/ionet` race `-count=2`、controller 定向 race、vet、gofmt、diff-check、根全量 test/vet/build 及
relaykit 独立 vet/build/test 已通过。最终 `8228203` /
[CI 33819117410](https://github.com/ForceMind/MyAPI/actions/runs/33819117410) 七项成功，新 io.net race `-count=2` 门禁实跑通过。
独立 Sol 无 P1/P2；P3 是 hardware/location 必填回显尚需真实
脱敏响应或官方 schema 补验。无页面/schema 改动，`VERSION` 仍为 0.1.1。

### S2-D10 cachex JSON wrapper（已完成当前范围）

`pkg/cachex/codec.go` 的最后两处调用已迁移为 `common.Marshal` 与
`common.Unmarshal([]byte(s), ...)`，根模块生产实际 JSON Marshal/Unmarshal/Decoder/Encoder 直调余量为
0/0。扫描排除 `common/json.go`、测试、relaykit、合法类型/`json.Valid` 及一条注释。解码保留 `[]byte(s)`
复制：独立审查指出直接使用 `UnmarshalJsonStr` 的 unsafe 别名会使自定义 `UnmarshalJSON` 可能改写调用者
string；codec 路径已改回复制，并新增 mutating unmarshaler 输入不变性测试。

codec 回归覆盖嵌套 round trip（含 `0`/`false`）、空白、malformed、类型错、多个值、尾随空白及 func
不可编码。本机 cachex race `-count=2`、根全量 test/vet/build、relaykit `GOWORK=off` vet/build/test、
gofmt、diff-check 和静态门禁均通过，独立复审最终无 P1/P2。最终 `bf03cba` /
[CI 33821142971](https://github.com/ForceMind/MyAPI/actions/runs/33821142971) 七项成功；Backend 原始步骤确认新增的
根 JSON 静态门禁与 io.net/cachex race `-count=2` 均实际通过。无页面/schema/Redis/真实缓存服务改动，
`VERSION` 保持 0.1.1。
