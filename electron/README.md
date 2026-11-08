# MyAPI Desktop：本机服务与接入 / Local service and access

本目录包含 MyAPI 的 Lite 桌面封装。首发范围为 macOS、Windows；保留的 Linux 开发打包目标不代表正式支持。
This directory contains the Lite desktop wrapper for MyAPI. Initial platforms are macOS and Windows; retained Linux development packaging targets are not a support claim.

## Desktop 使用教程 / Desktop task guide

**先看身份：** 当前已验源码为 **0.2.0-beta.55 / `202d01c9`**，已通过[完整CI42487543625](https://github.com/ForceMind/MyAPI/actions/runs/42487543625)及24条教程命令。同SHA按Mac→Win顺序构建候选包：macOS arm64 DMG/ZIP（[包13620767707](https://github.com/ForceMind/MyAPI/actions/runs/42489194257/artifacts/13620767707)；[CI42489194257](https://github.com/ForceMind/MyAPI/actions/runs/42489194257)）与Windows x64 NSIS/portable（[包13621485445](https://github.com/ForceMind/MyAPI/actions/runs/42489929687/artifacts/13621485445)；[CI42489929687](https://github.com/ForceMind/MyAPI/actions/runs/42489929687)），各151合同通过、发布作业skip。构建SHA/版本/架构已核，SHA256SUMS生成步骤通过；没有独立source manifest，未下载、安装或启动真实GUI，不宣称开发者签名/公证、托盘、防火墙、账务或跨机已验。准确GitHub摘要与期限见[TASKS](../docs/evidence/redesign-20261024/TASKS.md)。旧beta.40包保留原来源，不重标。限定Native后端/Go→owned HTTPS资格仍绑定beta.54 / e06c252a / [CI42485203085](https://github.com/ForceMind/MyAPI/actions/runs/42485203085)；Root Linux Docker Full/Lite备份资格见[CI42465372403](https://github.com/ForceMind/MyAPI/actions/runs/42465372403)。Linux Desktop不纳入首发。

**Identity first:** **0.2.0-beta.55 / `202d01c9`** passed full CI and all 24 tutorial commands. Same-source macOS arm64 DMG/ZIP and Windows x64 NSIS/portable candidates were built in that order, linked above; each passed 151 contracts and skipped publication. Checkout/build SHA, version and architecture were checked, and SHA256SUMS were generated. There is no independent source manifest. No local download, real installation, GUI, developer signing/notarization, tray, firewall, accounting or second-device acceptance is claimed. GitHub artifact digests and expiry are in TASKS. Historical beta.40 artifacts retain their original identity. Limited native backend/owned-HTTPS qualification remains bound to beta.54 / e06c252a; root Linux Docker Full/Lite backup qualification has its separate source. Linux Desktop is not a first-release platform.

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
| 已有 HTTPS 反代 / Existing HTTPS reverse proxy | 在本地公网引导里选择反代位置、核对外部条件、输入 HTTPS 根 origin，再明确保存。保存只是本机入口记录，外部验证状态始终未知；下方的此电脑分层检查已通过beta.53非GUI合同，真实外部视点仍未知。 / Choose the proxy location, review prerequisites, enter an HTTPS root origin and explicitly save. Saving creates a local entry record; external verification remains unknown. The layered check passed the beta.53 non-GUI contracts; a real external viewpoint remains unverified. |

已有 HTTPS 向导不会创建域名、监听公网、开端口、安装隧道、修改 LAN 或替用户选择收费厂商。输入不带 `/v1`、路径前缀、凭据、query 或 fragment；当前只接受 HTTPS 根 origin，本机/私网地址应回到对应分支。客户端复制的是所见已保存地址对应的 `/v1`，不会混入本机地址列表。保存失败、旧记录不可读或确认已过期时保留草稿并显示未知，不假称已保存；不可读旧记录须先明确确认清除，再重新填写。清除只解除这台电脑的记录，外部反代、DNS、隧道和权限必须由操作者另行撤销。

The guide does not create DNS, a public listener, port mappings or tunnels, change LAN scope, or choose a paid vendor. Enter an HTTPS root origin without `/v1`, a prefix path, credentials, query or fragment. Local/LAN inputs belong in their own branches. Copy uses the displayed saved entry plus `/v1`, separately from local addresses. Failed or stale actions preserve the draft and report uncertainty. An unreadable record must be explicitly cleared before saving another. Clearing changes only this computer's record; revoke external DNS/proxy/tunnel/access separately.

**从此电脑检查〔beta.53非GUI合同已过〕：** 原向导第4步的已保存入口下新增默认折叠检查区。展开、轮询或保存不会自动发起请求；只有明确点击“检查已保存入口”才检查当前所见的已保存 origin 及版本，不检查输入草稿，可单独取消。DNS（直填 IP 时为地址检查）、TLS 证书、公共状态响应分别显示未执行、进行中、通过、失败或未知，并保留实际观察时间和此电脑来源。保存、清除、入口文件变化或服务身份变化会使旧结果过时；旧回复不能覆盖新结果，草稿、步骤和焦点保留，检查不会阻塞停止服务或退出。

**Check from this computer — beta.53 non-GUI contracts passed:** the existing guide adds a collapsed check area beneath the saved entry in step 4. Expanding it, receiving snapshots or saving an entry sends no check automatically. Only **Check saved entry** checks the displayed saved origin and record revision, not the draft; cancellation is separate. DNS (an address check for literal IPs), TLS certificate and public status response have separate not-run, checking, passed, failed or unknown states, with actual observation time and this-computer provenance. Saving, clearing, file changes or service-identity changes invalidate old results. Late replies cannot replace newer results; drafts, steps and focus are preserved, and stop/quit remain available during a check.

检查只读取入口的公共状态，不发送 Key、登录凭据或模型请求，不修改反代或监听。失败提示只给安全且可操作的原因：核对普通公网地址、域名与证书；入口要求登录时说明未发送凭据，遇跳转不继续；无法识别状态、响应过大或超时均如实说明，未知不装成成功。即使三行都通过，也只代表此电脑当时的分层观测；`verification` 仍为 `unknown`，不能证明外网入站可达、是否为本机实例、鉴权、模型或额度可用。真正外部视点的验证仍未交付；beta.53合同绿灯不代表真实公网或GUI验收。

The check reads only the entry's public status. It sends no Key, sign-in credentials or model request and changes neither proxy nor listener. Fixed safe hints explain unsupported private/special addresses, domain/certificate problems, sign-in requirements without credentials, unfollowed redirects, unrecognizable status, response limits or timeouts. Unknown results stay unknown. Even if all three stages pass, this is only a layered observation from this computer at that time. `verification` remains `unknown`; it proves neither inbound Internet reachability, identity as this local instance, authentication, model availability nor quota. Verification from a genuinely external viewpoint is not delivered, and old CI or packages do not qualify this work in progress.

个人电脑休眠、合盖、关机、断网、退出、地址变化都会影响访问；面板不承诺后台永远在线、不自动补发请求。上行带宽、资源与额度消耗由使用者控制。此电脑检查不能替代外部视点验证；自动更新仍关闭，保存记录也不代表它们已完成。

Sleep, closing a laptop, power loss, disconnection, quitting or address changes can interrupt access. The panel promises neither permanent availability nor request replay. Consider upstream bandwidth, resources and quota. This-computer checks do not replace external-viewpoint verification. Automatic updates remain off; saving an entry completes neither.

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

只复制一个目录不保证一致或完整备份。先停止并确认服务，再由操作者核对全部真实数据、配置、密钥和外部路径。当前 [备份与恢复 CLI](../docs/BACKUP_RESTORE.md) 的资格范围是 root Linux amd64 Docker Full/Lite SQLite，不能当作 Desktop 的恢复工具；Desktop 自动备份/恢复、更新器和真正外部视点验证尚未交付，本机分层入口检查已通过beta.53非GUI合同，不能代替外部视点验证。日志只在明确打开时写出原始诊断，可能含敏感细节，分享前应脱敏。

Copying one directory alone does not guarantee a complete consistent backup. Confirm the service stopped and account for all actual data/configuration/identity paths. The linked backup CLI is qualified for root Linux amd64 Docker Full/Lite SQLite, not Desktop recovery. Automatic Desktop backup/restore, updater and verification from an external viewpoint are not delivered; the local layered entry check passed the beta.53 non-GUI contracts and does not verify an external viewpoint. Explicitly opened local diagnostics can contain sensitive details; redact before sharing.

### 5. 当前可在 CI 执行的合同 / Current CI command boundaries

以下从仓库根运行，不安装或启动 GUI。第一条检查资源/打包合同，第二条执行 Node/VM/合成 child 合同；它们不能证明托盘显示、真实安装、签名或防火墙访问。这两条原命令已在上文beta.54教程复现；限定原生后端/HTTPS联动资格绑定beta.54 / e06c252a。

Run from the repository root. The first command checks packaging/resource contracts; the second exercises Node/VM/synthetic-child contracts without launching Electron GUI. Neither verifies a real tray, installation, signing or firewall. The two exact commands passed in the beta.54 tutorial run above; the limited native-backend/HTTPS qualification is bound to beta.54 / e06c252a.

<!-- tutorial-command: desktop-source-contract -->
```bash
node tools/desktop/check.mjs --json
```

<!-- tutorial-command: desktop-source-tests -->
```bash
node --test electron/test/*.test.mjs
```

新增维护者专用 Actions 资格入口 `run_desktop_backend_qualification` 默认关闭，仅 manual/dev 使用。它从当前源码分别构建 macOS arm64、再 Windows x64 的真实 Lite 后端，在无 Electron GUI 的夹具中核 owned child 启停、配置标记与会话身份重启复读；只保留安全 JSON，不上传数据库、密钥、原始日志或二进制。该限定资格及真实Go经owned HTTPS反代的联动已在beta.54 / e06c252a同SHA通过：原9项保留，保存入口、真实响应、进度、取消及旧结果失效等8个新增检查通过，实际proxy/backend GET各4次。测试内受控DNS/CA/loopback不证明真实公网或同一实例。Windows 的进程关闭不会被称为已验证优雅 flush，两版均不宣称账务或在途恢复通过。

The new maintainer-only Actions input is off by default and limited to manual dev runs. It builds same-source native Lite backends, macOS arm64 before Windows x64, then checks owned-process lifecycle, configuration and session-identity persistence through an Electron facade. Only safe JSON is retained. The limited backend and real-Go-through-owned-HTTPS integration qualifications passed at beta.54 / e06c252a: all original nine checks remained, all eight new checks passed and four proxy/backend GETs were observed. Controlled DNS/CA/loopback does not verify a real public network or the same instance. Windows graceful flush and accounting remain unverified. No Electron/GUI, installer or real public-network entry test is performed.

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
