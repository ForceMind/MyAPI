# MyAPI Desktop：本机服务与接入 / Local service and access

本目录包含 MyAPI 的 Lite 桌面封装。首发范围为 macOS、Windows；保留的 Linux 开发打包目标不代表正式支持。
This directory contains the Lite desktop wrapper for MyAPI. Initial platforms are macOS and Windows; retained Linux development packaging targets are not a support claim.

## Desktop 使用教程 / Desktop task guide

**先看身份：** 当前源码是 `0.2.0-beta.47` / `90cb0f6c`，自身完整 CI 与这次教程命令尚待验收。已有安装构件绑定 **beta.40 / `13eedad9`**：macOS arm64 DMG/ZIP（[Actions 42379249054](https://github.com/ForceMind/MyAPI/actions/runs/42379249054)）和 Windows x64 NSIS/portable（[Actions 42379624002](https://github.com/ForceMind/MyAPI/actions/runs/42379624002)）。它们不是 beta.47；旧包不包含后来源码的全部能力。未正式发布或确认签名，未做真实 GUI、安装、托盘、系统防火墙或跨机验收。不要把 Actions 测试构件当作已受信任的正式安装包。Linux Desktop 不纳入首发。

**Identity first:** current source is `0.2.0-beta.47` / `90cb0f6c`; its full CI and these exact tutorial commands are pending. Existing macOS arm64 DMG/ZIP and Windows x64 NSIS/portable artifacts are **beta.40 / `13eedad9`**, linked above. They are not beta.47 and do not include every later source feature. Publication, signing, real GUI/install/tray, firewall and second-device acceptance are unverified. Linux Desktop is not a first-release platform.

下面说明的是**当前源码行为**，供维护者审核和后续受控包验收；没有与目标源码匹配并审核过的包时，不提供猜测下载链接、绕过 Gatekeeper/SmartScreen 的命令或自动安装步骤。源码构建只在明确的构建任务中执行，不代表已安装到你的电脑。

The following describes **current source behavior** for review and later package acceptance. Without an exact-source reviewed package, do not invent a download URL or bypass OS trust prompts. Building source is a separate task, not installation on your computer.

### 1. 选择形态与打开面板 / Choose the shape and open the panel

Desktop 是 Lite 的桌面安装与管理形态，使用同一套连接、Key、模型和请求功能。需要完整计费等服务端功能时看 [Docker Full](../docs/MYAPI_DISTRIBUTION.md)；个人 Docker 与系统限制见[个人电脑说明](../docs/PERSONAL_COMPUTER.md)。它不会读取 Codex、Claude、Keychain 或其他本机供应商凭据。

Desktop packages and manages the Lite core: connections, Keys, models and requests. Use Full for the complete server feature set. Desktop does not discover host provider credentials or import Codex, Claude or Keychain secrets.

运行经审核的对应平台包后，从托盘打开“本机服务面板”。面板是本地资源，内置服务停止或启动失败时仍可查看状态和帮助；只有“运行中”且本机探测已确认时才按所见地址打开控制台。开发模式的外部服务会明确标为外部管理，不由面板启动或停止。服务状态不证明真实供应商或公网健康。

In a reviewed matching package, open the local service panel from the tray. It remains available when the bundled service is stopped or fails. Open the console using an address actually shown after local readiness is confirmed. Development servers are externally managed and are not controlled by this panel. Local readiness is not provider or public-network verification.

托盘、服务面板及相关确认支持中文和英文；可在托盘明确选择语言。其他系统语言回退英文；偏好写入失败时只对本次有效并提示，不应误认为永久保存。以下界面操作尚未在真实 GUI 验收，不是本次命令测试的替代物。

Tray, panel and confirmations support Chinese and English. Select the language explicitly; other OS languages fall back to English. A failed preference write is reported as session-only. These UI steps still require real GUI acceptance.

### 2. 本机、LAN、已保存 HTTPS 地址 / Local, LAN and saved HTTPS entry

| 任务 / Task | 当前可做与结果 / Current action and result |
| --- | --- |
| 本机 / Local | 默认回环监听；复制面板显示的 OpenAI-compatible Base URL（含 `/v1`），在控制台选择 Key 后配置客户端。控制台根地址不是 API Base URL。 / Loopback is default. Copy the displayed `/v1` Base URL and select a Key in the console; the console root is a different URL. |
| 私网 / LAN | 从 LAN 帮助选择当前可用私网地址，阅读并确认受控重启。它先确认旧 owned child 已关，再重启本应用，不热切换、不配置防火墙。停止未确认则保留现场，不继续创建第二个服务。 / Select an available private address and confirm the controlled relaunch. The owned child must close first; no hot rebind or firewall configuration is performed. An unconfirmed stop blocks another launch. |
| 已有 HTTPS 反代 / Existing HTTPS reverse proxy | 在本地公网引导里选择反代位置、核对外部条件、输入 HTTPS 根 origin，再明确保存。保存只是本机入口记录，状态始终未验证；不是 DNS/TLS/鉴权/可达性或模型成功证据。 / Choose the proxy location, review prerequisites, enter an HTTPS root origin and explicitly save. This records a user-provided entry on this computer; verification remains unknown. |

已有 HTTPS 向导不会创建域名、监听公网、开端口、安装隧道、修改 LAN 或替用户选择收费厂商。输入不带 `/v1`、路径前缀、凭据、query 或 fragment；当前只接受 HTTPS 根 origin，本机/私网地址应回到对应分支。客户端复制的是所见已保存地址对应的 `/v1`，不会混入本机地址列表。保存失败、旧记录不可读或确认已过期时保留草稿并显示未知，不假称已保存；不可读旧记录须先明确确认清除，再重新填写。清除只解除这台电脑的记录，外部反代、DNS、隧道和权限必须由操作者另行撤销。

The guide does not create DNS, a public listener, port mappings or tunnels, change LAN scope, or choose a paid vendor. Enter an HTTPS root origin without `/v1`, a prefix path, credentials, query or fragment. Local/LAN inputs belong in their own branches. Copy uses the displayed saved entry plus `/v1`, separately from local addresses. Failed or stale actions preserve the draft and report uncertainty. An unreadable record must be explicitly cleared before saving another. Clearing changes only this computer's record; revoke external DNS/proxy/tunnel/access separately.

个人电脑休眠、合盖、关机、断网、退出、地址变化都会影响访问；面板不承诺后台永远在线、不自动补发请求。上行带宽、资源与额度消耗由使用者控制。公网能力的外部验证和自动更新仍属于后续范围，保存记录不代表已完成它们。

Sleep, closing a laptop, power loss, disconnection, quitting or address changes can interrupt access. The panel promises neither permanent availability nor request replay. Consider upstream bandwidth, resources and quota. External public-access verification and automatic updates remain future work.

### 3. 关窗、停服务、退出 / Close, stop or quit

| 操作 / Action | 含义 / Meaning |
| --- | --- |
| 关闭窗口 / Close window | 隐藏窗口、留在托盘；不是停止服务。Dock/任务栏是否显示窗口也不能作为服务已停的证据。 / Hides the window and retains the tray; it does not stop the service. Dock/taskbar visibility is not stop evidence. |
| 停止内置服务 / Stop bundled service | 默认取消的确认后，只停止本应用持有的 child；应用和面板保留，可明确再次开始。 / After a default-cancel confirmation, stops only this app's owned child. The app and panel remain available for an explicit start. |
| 退出 MyAPI / Quit MyAPI | 默认取消的确认后，先确认 owned child 真正关闭再退出；未知停止结果不会被当成安全退出。 / After confirmation, verifies the owned child closed before exiting. An unknown stop is not treated as a completed exit. |

这些动作不按端口杀进程，不操作开发模式外部 server，不自动重启、回灌数据库或重放模型请求。只有前置探测与实际 close 证据成立时才显示对应状态。未知时保留面板和日志入口，避免反复点开始。

These actions do not kill by port, control external development servers, automatically restart, restore databases or replay model requests. Unknown outcomes retain the panel and log access; do not repeatedly start another process.

### 4. 保存数据与排查 / Preserve data and inspect failures

在面板展开数据/日志位置，核对实际 `userData`、数据库、日志和身份文件。数据库优先采用明确的 `MYAPI_SQLITE_PATH`、其次 `SQLITE_PATH`，否则才用 `userData/data` 下既有 legacy 或 canonical 文件；显式外部路径不会因为在应用外就被自动包含。`SESSION_SECRET` 可来自环境或本应用固定私有文件，不能重新生成来“修复”已有实例。不要删除 WAL/SHM、数据库或身份文件，也不要把它们提交到 Git。

Expand data/log locations to inspect actual paths. Database selection prefers explicit `MYAPI_SQLITE_PATH`, then `SQLITE_PATH`, then the preserved legacy/canonical file under `userData/data`. An external database path is not automatically included in an app-folder copy. Preserve the configured session secret or private app identity file; regenerating it is not a recovery procedure. Never delete DB/WAL/SHM or identity files to bypass a failure.

只复制一个目录不保证一致或完整备份。先停止并确认服务，再由操作者核对全部真实数据、配置、密钥和外部路径。当前 [备份与恢复 CLI](../docs/BACKUP_RESTORE.md) 的资格范围是 root Linux amd64 Docker Full/SQLite，不能当作 Desktop 的恢复工具；Desktop 自动备份/恢复、更新器和公网外部验证尚未交付。日志只在明确打开时写出原始诊断，可能含敏感细节，分享前应脱敏。

Copying one directory alone does not guarantee a complete consistent backup. Confirm the service stopped and account for all actual data/configuration/identity paths. The linked backup CLI is qualified for root Linux amd64 Docker Full/SQLite, not Desktop recovery. Automatic Desktop backup/restore, updater and external public verification are not delivered. Explicitly opened local diagnostics can contain sensitive details; redact before sharing.

### 5. 当前可在 CI 执行的合同 / Current CI command boundaries

以下从仓库根运行，不安装或启动 GUI。第一条检查资源/打包合同，第二条执行 Node/VM/合成 child 合同；它们不能证明托盘显示、真实安装、签名或防火墙访问。本批 exact 命令仍待 Actions 记录。

Run from the repository root. The first command checks packaging/resource contracts; the second exercises Node/VM/synthetic-child contracts without launching Electron GUI. Neither verifies a real tray, installation, signing or firewall. This tutorial's exact command evidence is still pending Actions.

<!-- tutorial-command: desktop-source-contract -->
```bash
node tools/desktop/check.mjs --json
```

<!-- tutorial-command: desktop-source-tests -->
```bash
node --test electron/test/*.test.mjs
```

以下开发与打包内容继续保留为维护参考。只有匹配源码的 Actions 构建、平台签名和真实主机步骤分别完成后，才可扩大支持声明；未标记为 `tutorial-command` 的构建/GUI例子不属于本批待验的教程命令集合。
The following development and packaging references remain available. Source-matched Actions builds, signing and real-host acceptance are separate gates; reference examples are not current qualified installation steps.

## 开发和打包参考 / Development and packaging reference

以下命令要求维护者另外准备构建工具和依赖，或明确启动 GUI；尚未作为本批用户安装教程执行。 / These references require build dependencies or explicit GUI launch and are not qualified user-install steps.

## Prerequisites

### 1. Go Binary (Required)
The Electron app requires the compiled Go binary to function. You have two options:

**Option A: Use existing binary (without Go installed)**
<!-- tutorial-reference: desktop-development-1 -->
开发/平台参考，需额外条件与验证，勿当作当前安装步骤。 / Development/platform reference requiring separate prerequisites and acceptance.
```text
# If you have a pre-built binary (e.g., my-api-macos)
cp ../my-api-macos ../my-api
```

**Option B: Build from source (requires Go)**
From the repository root, build the frontend and the platform-specific
backend/package with the bounded helper (the default is two parallel workers):

<!-- tutorial-reference: desktop-development-2 -->
开发/平台参考，需额外条件与验证，勿当作当前安装步骤。 / Development/platform reference requiring separate prerequisites and acceptance.
```text
cd electron
MYAPI_BUILD_PARALLELISM=2 ./build.sh
```

On Windows, run the same script from Git Bash (or build the Go binary with
`go build -o my-api.exe` and then run `npm run build:win`). The helper never
starts Docker or publishes artifacts; it only writes `my-api[.exe]` and the
installer files under `electron/dist/`.

### 2. Electron Dependencies
<!-- tutorial-reference: desktop-development-3 -->
开发/平台参考，需额外条件与验证，勿当作当前安装步骤。 / Development/platform reference requiring separate prerequisites and acceptance.
```text
cd electron
npm install
```

## Development

Start the backend, the frontend, and Electron in separate terminals:
<!-- tutorial-reference: desktop-development-4 -->
开发/平台参考，需额外条件与验证，勿当作当前安装步骤。 / Development/platform reference requiring separate prerequisites and acceptance.
```text
# Repository root
go run main.go

# Repository root
make dev-web

# electron/
npm run dev-app
```

This will:
- Use the Go backend on port 3000
- Use the Rsbuild frontend development server on port 5173
- Open an Electron window with DevTools enabled
- Create a system tray icon (menu bar on macOS)
- Store database in `../data/my-api.db` (existing `new-api.db`/`one-api.db` files are adopted automatically)

## Building for Production

The helper limits Go/npm build parallelism to two workers by default so a
desktop build does not monopolize a shared workstation. Set
`MYAPI_BUILD_PARALLELISM` to a positive integer for a dedicated build host.

### Quick Build
<!-- tutorial-reference: desktop-development-5 -->
开发/平台参考，需额外条件与验证，勿当作当前安装步骤。 / Development/platform reference requiring separate prerequisites and acceptance.
```text
# From electron/, build the frontend, Go binary, and desktop package
./build.sh

# Or package an existing binary for the current platform
npm run build

# Platform-specific builds
npm run build:mac    # Creates .dmg and .zip
npm run build:win    # Creates .exe installer
npm run build:linux  # Retained development target; Linux Desktop is not a first-release platform
```

### Build Output
- Built applications are in `electron/dist/`
- macOS: `.dmg` (installer) and `.zip` (portable)
- Windows: `.exe` (installer) and portable exe
- Linux: retained `.AppImage` and `.deb` development targets; no initial-release acceptance

Before a platform owner runs a build, the repository-level contract check can
be run without Electron or Go:

<!-- tutorial-reference: desktop-development-6 -->
开发/平台参考，需额外条件与验证，勿当作当前安装步骤。 / Development/platform reference requiring separate prerequisites and acceptance.
```text
npm run desktop:check
```

The check covers the macOS/Windows targets, bundled native binary and license
resources, versioned tag validation, SHA256 checksum generation, and explicit
release approval gates in GitHub Actions. It is deterministic and does not
start Docker, contact GHCR, or publish artifacts. Installer launch, firewall
prompt, and LAN request acceptance still require real macOS and Windows hosts.

The current workflow produces unsigned desktop artifacts unless the release
environment supplies platform signing credentials. macOS users may see
Gatekeeper warnings (DMG/ZIP), and Windows users may see SmartScreen warnings
for NSIS/portable packages. Apple Developer ID signing/notarization and
Windows code-signing certificates are external release prerequisites; do not
treat a successful build or checksum as proof that an installer is trusted.

## Configuration

### Port
Default port is 3000. Pass `--port 4317` when launching the desktop app to
override it. The process binds to `127.0.0.1` by default.

To share with colleagues, explicitly pass a private bind address and opt in:

```text
MyAPI --allow-lan --bind-address 192.168.1.20 --port 4317
```

Public addresses are rejected, and `--allow-lan` is required for any
non-loopback listener. The tray menu shows the effective endpoint. The desktop
process runs the Lite edition (with the legacy LAN-compatible access limits) and never imports local Codex, Claude, or other
provider credential files.

Use the bilingual local service panel and **LAN status and help** tray action to inspect effective endpoints and firewall guidance. Current source also offers a confirmed private-address relaunch: it waits for the owned child to close before restarting, never performs a hot rebind and does not modify firewall rules. If stop is unconfirmed, relaunch is blocked. Real tray/OS behavior remains unverified.

Before a server process is spawned, Electron performs a platform-independent
configuration preflight. Invalid/public addresses fail with a visible error;
the default remains `127.0.0.1:3000`. The checks are covered by
`npm test` in this directory and intentionally do not inspect or import any
local provider credential files.

### Database Location (defaults; inspect actual paths first)
- **Development**: `../data/my-api.db` (project directory; legacy database names are read for migration)
- **Production**:
  - macOS: `~/Library/Application Support/MyAPI/data/`
  - Windows: `%APPDATA%/MyAPI/data/`
  - Linux development target only: `~/.config/MyAPI/data/`; not an initial-release platform
