# MyAPI Docker Lite 与 Legacy LAN 兼容

> 当前源码已有 Full/Lite Docker CLI、Desktop 本机服务面板与已有 HTTPS 反代记录向导；`lan`/`myapi-lan`兼容身份保持。beta.55 / 202d01c9完整CI、24条教程及Full/Lite隔离镜像验证已过；统一下载/验签/原生安装、自动更新、自动形态切换及真实公网仍未接线或未验收，详见下文。

Lite 是面向个人和小规模使用的 MyAPI 功能版，可部署在个人服务器、云服务器、VPS 或个人电脑。轻量化意味着 SQLite-first、低依赖、低资源和精简默认流程；它不意味着只能在局域网使用。Desktop 是 Lite 的桌面安装和管理形态，业务核心和数据模型必须与 Lite 一致。

功能版（Full/Lite）、安装形态（服务器/个人电脑/容器/Desktop）和访问模式（本机/LAN/public）是独立维度。个人电脑 Lite/Desktop 默认仅本机访问；LAN 分享和 public 开放均需单独、明确开启。public 模式需要环境检查、用户确认和外部网络验证，不能用本机监听或健康检查证明；它也绝不自动开启匿名调用、公开注册、无认证管理页、宿主文件访问或 Codex 权限。

完整的发行、安装、更新、切换、数据目录和 public 向导合同见 [发行制品、安装与更新合同](RELEASE_MANIFEST.md)。本页保留当前 Legacy LAN 的安全行为、迁移边界和可执行命令，不能把它们误读为未来 Lite 的全部能力。

## Docker Lite 源码教程 / Docker Lite from source

**当前边界：** `0.2.0-beta.55`、源码 `202d01c9` 的[完整CI与教程](https://github.com/ForceMind/MyAPI/actions/runs/42487543625)、[Full/Lite镜像验证](https://github.com/ForceMind/MyAPI/actions/runs/42489190681)已通过；准确构件与限制见[TASKS](evidence/redesign-20261024/TASKS.md)，对应NPM/GHCR发行仍未发布。Lite的Docker启停和版本门禁已有实现；`lan`是兼容别名，不是另一个产品。以下从源码运行，不使用`npx`、`latest`或未发布tag。

**Current boundary:** full CI, exact tutorial commands and Full/Lite image checks passed at `0.2.0-beta.55` / `202d01c9`. TASKS records exact candidate artifacts and limits. Corresponding NPM/GHCR releases remain unpublished. Docker lifecycle and edition gates exist; `lan` remains the compatibility alias of Lite. Use reviewed source, not `npx`, `latest`, or an unpublished tag.

### 1. 选择本机或私网 / Choose local or private LAN

Lite 适合自己或小团队的接入网关，保留连接、模型、Key、额度和请求管理；支付、公开注册等被版本门禁关闭的功能不会因教程而开启。默认只在本机使用；需要同事访问时才选择私网地址并显式确认 LAN。云服务器上的 Lite 同样遵守当前监听/来源规则，不能把“在云上”当成已支持公网安装。

Lite retains connection, model, Key, quota and request management while edition-disabled commercial/public features stay disabled. Begin with loopback. Enable private LAN only deliberately. Running on a cloud host does not establish public reachability or bypass these rules.

准备 Node.js 22、Docker Engine 或 Docker Desktop、Compose v2.17+，从源码根目录运行。为本教程单独设置 `MYAPI_PROJECT_DIR`（新项目绝对路径）、`MYAPI_TUTORIAL_PORT`（所选空闲端口）和 `MYAPI_TUTORIAL_IMAGE`（审核过的当前源码 `local/...` 镜像名）。不要复用 Full 教程或真实实例的目录；CI 使用自己的临时路径与已加载镜像。

Use Node.js 22, Docker Engine or Desktop and Compose v2.17+, from the source root. Set a separate new absolute `MYAPI_PROJECT_DIR`, an unused `MYAPI_TUTORIAL_PORT`, and a reviewed `local/...` `MYAPI_TUTORIAL_IMAGE`. Do not reuse an existing Full or production project; CI provides isolated fixture values.

### 2. 创建并检查 / Initialize and inspect

<!-- tutorial-command: lite-source-init -->
```bash
node cli/myapi.mjs lan init "${MYAPI_PROJECT_DIR:?}" --port "${MYAPI_TUTORIAL_PORT:?}"
```

初始化只建立项目和私有配置，不启动服务、不改防火墙，也不导入本机供应商凭据。生成的 `deploy/.env` 仍写 `MYAPI_EDITION=lan`；它是 Lite 的兼容值。用文本编辑器将 `MYAPI_IMAGE` 改为上述审核镜像的实际值，并设 `MYAPI_BUILD_LOCAL=true`，保持密钥、端口、回环绑定和账务设置。若明确选择规范拼写 `MYAPI_EDITION=lite`，后续命令会保留它；不要期待自动迁移或凭据重建。

Initialization creates private configuration without starting Docker, changing firewall rules or importing host credentials. It writes the compatible `MYAPI_EDITION=lan`. In an editor, set the actual reviewed local `MYAPI_IMAGE` and `MYAPI_BUILD_LOCAL=true`, preserving the secret, port, loopback and accounting settings. An explicitly selected `lite` spelling is preserved too; neither spelling authorizes data migration or identity replacement.

<!-- tutorial-command: lite-source-doctor -->
```bash
node cli/myapi.mjs doctor --project-dir "${MYAPI_PROJECT_DIR:?}"
```

### 3. 构建、启动并接入 / Build, start and connect

以下会构建并启动所选项目，仅在审核完本地镜像配置后执行。未设置本地镜像时不能依赖默认的未发布 GHCR tag。
These commands build and start the selected project. Review the local-image configuration first; the default unpublished GHCR tag is not a usable release.

<!-- tutorial-command: lite-source-build -->
```bash
node cli/myapi.mjs build --project-dir "${MYAPI_PROJECT_DIR:?}"
```

<!-- tutorial-command: lite-source-start -->
```bash
node cli/myapi.mjs lan start --project-dir "${MYAPI_PROJECT_DIR:?}"
```

<!-- tutorial-command: lite-source-status -->
```bash
node cli/myapi.mjs lan status --project-dir "${MYAPI_PROJECT_DIR:?}"
```

启动成功须经过健康等待；`status` 只报告当前配置与部署状态。随后在显示的本机控制台完成管理员设置，按首页的“管理连接→管理 Key→选择 Key 配置客户端”操作。控制台地址与 API Base URL 不同，客户端使用所见 `/v1` 地址和自己的 Key。浏览器、真实客户端与供应商请求不在本批教程验收内。

Startup must pass its health wait. Status describes configuration and deployment, not a successful model request. Complete first-admin setup at the displayed local console, then manage connections, create/select a Key and configure a client with the displayed `/v1` Base URL. Browser, real-client and provider acceptance are separate.

### 4. 可选私网分享 / Optional private-LAN sharing

仅在你管理的私网中执行下一条；先将 `MYAPI_TUTORIAL_BIND_ADDRESS` 设为本机真实 RFC1918 IPv4。地址不是示例占位值，不能使用公网地址。这个动作会持久化所选绑定并重建项目容器；不会替你配置防火墙、路由器或 TLS。第二台机器是否可达仍需另行验证。

Only use the next command on a private network you administer. Set `MYAPI_TUTORIAL_BIND_ADDRESS` to an actual local RFC1918 IPv4, not a placeholder or public address. It persists the chosen bind and recreates the project container. It does not configure firewall, router or TLS; second-device reachability remains unverified.

<!-- tutorial-command: lite-private-start -->
```bash
node cli/myapi.mjs lan start --project-dir "${MYAPI_PROJECT_DIR:?}" --bind-address "${MYAPI_TUTORIAL_BIND_ADDRESS:?}" --port "${MYAPI_TUTORIAL_PORT:?}" --allow-lan
```

当前显式监听参数会将 `MYAPI_IMAGE` 重写为 `myapi-lan` 的当前源码版本坐标，执行前必须确认对应已审镜像实际可用。Actions 使用本轮构建的不可变镜像 ID 创建仅在该测试 daemon 内存在的临时别名；这不表示 GHCR 已发布，也不是生产更换镜像的操作方案。

Explicit listener options currently rewrite `MYAPI_IMAGE` to the current source-version `myapi-lan` coordinate. Confirm that the reviewed image is actually available before using this command. CI may use a temporary local alias bound to its own immutable build; it neither publishes that coordinate nor authorizes substituting a production image.

每位使用者分别创建可撤销的 Key，只给需要的模型、额度和速率；不要分享管理员身份或上游凭据。需要切回本机时，按相同 CLI 明确选择回环地址与原端口，不要把旧 LAN URL 当作永久可用。已有 HTTPS 反代的 Desktop 记录向导见[桌面说明](../electron/README.md)，它不改变本 Docker 监听，也不证明公网已验证。

Give each user a separate revocable Key with the required model, quota and rate limits. Never share admin identity or provider credentials. Return to loopback explicitly when sharing ends; a previous LAN URL is not a permanent availability guarantee. The Desktop HTTPS-entry guide only saves a user-provided record and does not change this Docker listener.

### 5. 停止与保数据 / Stop and preserve data

<!-- tutorial-command: lite-source-stop -->
```bash
node cli/myapi.mjs lan stop --project-dir "${MYAPI_PROJECT_DIR:?}"
```

停止不删除挂载目录，仍不是已验证备份。保留项目配置、会话密钥、数据和日志；不要用全新配置覆盖已有身份，不回灌旧账务。当前[备份/恢复CLI](BACKUP_RESTORE.md)已在34b47daf / CI42465372403通过Root Linux amd64 Docker **Full/Lite + SQLite**真实备份、隔离恢复及保留策略；这不等于Docker Desktop、非root、原生或Desktop恢复资格。

Stopping retains mounted data, but is not a verified backup. Preserve configuration, session identity, data and logs; do not replace existing identities or roll accounting back. The linked backup CLI is qualified for root Linux amd64 Docker **Full/SQLite**, not as a general Lite/Desktop recovery tool. Beta.47 retention qualification is pending.

个人电脑的 Docker 文件共享、休眠、关机和系统权限另见[个人电脑教程](PERSONAL_COMPUTER.md)；原生安装的当前范围见[Linux 预览](NATIVE_LINUX.md)。以下保留兼容性和高级参考，不替代上述当前源码路径。
For workstation sharing, sleep and permissions, see the personal-computer guide. Native support is currently a preview. The following advanced references retain compatibility information rather than introducing another installation path.

## 当前 Legacy LAN 行为（和它不代表的内容）

Legacy LAN 是当前单进程、SQLite-first 的可信本机/私网部署。默认监听回环；`--allow-lan` 才允许私网监听。当前实现不是 public hosting 模式、凭据文件导入器、自动防火墙配置器、服务器 Lite 安装器或完整 Desktop 更新器。

后端版本门禁（`middleware/edition.go`）：

- `MYAPI_EDITION` 只接受空值（等同 `full`）、`full`、`lite`、`lan`（`lite` 的 Legacy 别名），大小写和首尾空格不敏感。其他非空值在启动阶段拒绝启动，进程以 `unsupported MYAPI_EDITION "<值>"` 退出，不再静默回落为 Full。
- 部署侧与后端接受同一组值：`Dockerfile`、`cli/myapi.mjs`、`deploy/install.sh` 对 `lite` 与 `lan` 的处理完全一致（精简前端构建、默认本地 HTTP cookie、本机/私网来源与监听规则、`myapi-lan` 镜像仓库），非法值会被 CLI 与安装脚本拒绝（`MYAPI_EDITION must be full, lite, or lan`）。`node cli/myapi.mjs lan init` 仍写入 Legacy 的 `lan`；已有部署的 `lan` 或 `lite` 值不会被自动改写成另一种拼写，`node cli/myapi.mjs lan start` 在持久化监听设置时也保持原值。Lite 镜像坐标仍是 `ghcr.io/forcemind/myapi-lan`，没有新的镜像仓库或 tag。
- Lite 下返回 403 `MYAPI_LAN_ROUTE_DISABLED` 的范围包括支付、充值、商业订阅、签到、返利、公开定价与排行、外部 OAuth、公开注册，以及兑换码管理 `/api/redemption`、io.net 模型部署 `/api/deployments`、Midjourney（`/api/mj`、中继 `/mj/...` 与 `/:mode/mj/...`）。
- Lite 下 `GET /api/subscription/self` 保持可用，让已有订阅的用户查看自己的订阅与额度（订阅重置任务在所有版本运行）；`/api/subscription/self/preference` 和其余 `/api/subscription/*` 仍返回 403。Key 分额 `/api/subscription-share/*`、渠道管理与 `/v1/*` 转发不受影响。

The CLI and LAN backend never scan `~/.codex`, Claude credential files,
Keychain, Windows Credential Manager, or any other local credential store.
Administrators may still configure an upstream channel explicitly in the
MyAPI interface, including deliberately pasting a credential they exported
themselves, or point it at an already-authorized local API endpoint. That
explicit configuration is not automatic host credential discovery.

**CLI availability:** `@forcemind/myapi` is not yet published to NPM. Until the
maintainer publishes a reviewed release, run the examples from a source checkout
as `node cli/myapi.mjs ...` (or use a reviewed local tarball); do not let `npx`
resolve an unknown public package with the same name.

## 当前 Legacy LAN Docker Desktop 流程

Install Docker Desktop, then run the CLI from the MyAPI source distribution:

<!-- tutorial-reference: lite-advanced-1 -->
兼容/平台参考，未纳入本批可执行教程验收。 / Compatibility or platform reference, not a qualified current tutorial step.
```text
node cli/myapi.mjs lan init ./myapi-lan
node cli/myapi.mjs lan start --project-dir ./myapi-lan
```

Use a current Docker Desktop release (Compose v2.17 or newer); the start
command uses Compose's bounded `--wait` health check and does not treat a
running-but-unhealthy container as ready.

A previously published, maintainer-verified private GHCR image requires package-read authentication. This does not make the current beta.47 image available. For this tutorial use the reviewed local-source image; registry login is a separate authorized operation.

The initializer creates a private `deploy/.env`, a random session secret, SQLite/data directories, and the LAN image configuration. It does not start Docker or change host firewall rules.

The generated deployment also sets conservative Docker guardrails
(`MYAPI_CPU_LIMIT=2.0` and `MYAPI_MEMORY_LIMIT=2g`). Adjust these values in
`deploy/.env` only after measuring the workstation workload; `node cli/myapi.mjs lan start`
validates the values before pulling an image. Startup waits up to 120 seconds
for the container healthcheck, so a failed image or configuration is reported
instead of being presented as a usable LAN endpoint.

On Windows PowerShell the same commands work with a Windows path, for example
`node cli/myapi.mjs lan init "$env:LOCALAPPDATA\\MyAPI\\lan-project"`.

To share deliberately on a private IPv4 network, opt in and bind to the workstation's private address:

<!-- tutorial-reference: lite-advanced-2 -->
兼容/平台参考，未纳入本批可执行教程验收。 / Compatibility or platform reference, not a qualified current tutorial step.
```text
node cli/myapi.mjs lan start \
  --project-dir ./myapi-lan \
  --bind-address 192.168.1.20 \
  --port 3000 \
  --allow-lan
```

`--allow-lan` is required whenever the listener is not loopback. Public addresses are rejected. `0.0.0.0` is accepted only with `--allow-lan`; the CLI discovers and prints RFC1918 addresses from the workstation's network interfaces for this wildcard mode. Prefer a concrete private bind address when possible, and verify that the selected address is reachable by colleagues (virtual/Docker interfaces may also be listed).

If you deploy the generated `deploy/install.sh` directly instead of using the CLI,
the equivalent guard is `MYAPI_ALLOW_LAN=true` in `deploy/.env`. The installer
accepts only loopback, `0.0.0.0`, or RFC1918 private IPv4 addresses and refuses
non-loopback binds unless that flag is explicitly enabled.

The Electron desktop wrapper performs the same preflight before spawning the
bundled server. An invalid port or a public/non-IPv4 bind address is shown as
an error and the server is not started. The default remains loopback-only at
`127.0.0.1:3000`; changing the bind address is an explicit operator action.
When a packaged app is launched with a concrete private bind address, its UI
loads from that same address (and maps `0.0.0.0` to loopback for local probing),
so LAN mode does not depend on an unreachable hard-coded loopback URL.

Desktop 的中英文托盘和独立服务面板显示真实监听、LAN 帮助及当前服务状态；可按默认取消的确认流程选择私网地址并重启本应用，不是热切换。停止未确认时不会继续重启；详见 [Desktop](../electron/README.md)。真实 OS 防火墙、安装和托盘交互仍未验证。
若绑定 `0.0.0.0`，托盘和帮助对话框会列出启动时发现的 RFC1918 IPv4 端点；没有发现
候选地址时会明确提示，而不会显示不可访问的占位 URL。

PowerShell uses the same flags (use a backtick for line continuation):

<!-- tutorial-reference: lite-advanced-3 -->
兼容/平台参考，未纳入本批可执行教程验收。 / Compatibility or platform reference, not a qualified current tutorial step.
```text
node cli/myapi.mjs lan start `
  --project-dir "$env:LOCALAPPDATA\MyAPI\lan-project" `
  --bind-address 192.168.1.20 `
  --port 3000 `
  --allow-lan
```

Check the configured endpoint without exposing secrets:

<!-- tutorial-reference: lite-advanced-4 -->
兼容/平台参考，未纳入本批可执行教程验收。 / Compatibility or platform reference, not a qualified current tutorial step.
```text
node cli/myapi.mjs lan status --project-dir ./myapi-lan
node cli/myapi.mjs lan stop --project-dir ./myapi-lan
```

The status output only shows the listener and deployment state. For a `0.0.0.0` listener it prints only discovered RFC1918 IPv4 endpoint candidates, never public or IPv6 addresses; if discovery finds none it reports that explicitly rather than showing a fake URL. It never prints `SESSION_SECRET`, upstream keys, downstream keys, OAuth JSON, cookies, or JWTs.

## 当前 Legacy 数据位置

Relative `./data` and `./logs` paths are portable and are the default for a source workspace. If you choose platform-specific host paths, use a directory owned by the current user:

- macOS: `~/Library/Application Support/MyAPI/lan`
- Windows: `%LOCALAPPDATA%\\MyAPI\\lan`

Packaged Electron builds keep the backend log directory at the platform
`userData` root (`.../MyAPI/logs`) and full-content logs under its
`full-content` subdirectory. This avoids writing into the read-only app
resources directory. Copying only this directory is not a complete-backup guarantee: preserve the session identity and any explicitly selected external SQLite path as well, and obtain a consistent stopped copy. The current Docker Full backup command does not cover Desktop.

Docker Desktop must be allowed to share the selected directory. Do not mount an entire home directory or a credential directory into the container.

## 当前 Legacy LAN 同事访问

1. Open the administrator UI at the displayed local URL.
2. Create one downstream API Key per colleague.
3. Give each person only the models, rate limit, and quota they need.
4. Revoke or rotate a key independently when access ends.

Colleagues receive the LAN URL and their own MyAPI key. They do not receive the upstream provider key, administrator password, local files, or another colleague's logs. Do not paste keys into chat, source control, screenshots, or issue reports.

## 当前 Legacy 停止、升级与恢复

`node cli/myapi.mjs lan stop` stops the LAN compose project without removing its data volumes. An update is explicit:

<!-- tutorial-reference: lite-advanced-5 -->
兼容/平台参考，未纳入本批可执行教程验收。 / Compatibility or platform reference, not a qualified current tutorial step.
```text
node cli/myapi.mjs lan stop --project-dir ./myapi-lan
# update the pinned MYAPI_IMAGE in deploy/.env when you are ready
node cli/myapi.mjs lan start --project-dir ./myapi-lan
```

Release workflows can build/sign explicitly authorized `ghcr.io/forcemind/myapi-lan:<version>` releases; a source commit or successful test does not establish that a particular image was published. They do not restart your workstation. Keep the image pinned to a version and retain the previous image until the new health check succeeds.

桌面安装包的构建合同与实际签名是两件事：当前 workflow 可以生成 macOS DMG/ZIP 和
Windows NSIS/portable 制品，但是否签名取决于维护者提供的 Apple Developer ID、
notarization 凭据或 Windows 代码签名证书。未签名的 macOS 包可能触发 Gatekeeper，
Windows 包可能触发 SmartScreen；在证书和发布审批准备好前，应把 Actions artifact
视为测试制品，不要宣称为受信任的正式安装包。

## Lite 服务器、个人电脑与 Desktop 的目标迁移

未来受管安装会先显示当前功能版、安装形态、访问模式、数据目录和版本，再从已验证 Release Manifest 选择目标制品。Legacy LAN 升级映射为 Lite/local 或 Lite/lan，保留数据库、配置、日志、Key、渠道、账务、S5-P 资料和 Codex 应用备份；不会自动转数据库、改监听、开放 public、修改防火墙或授予宿主文件权限。

服务器 Lite 将提供原生与容器安装、SQLite-first 默认、数据/日志位置、服务运行、自动启动、备份恢复、域名/HTTPS/反向代理和经授权 public 访问教程。当前 Desktop 已有本机 HTTPS 入口记录和个人电脑风险帮助；外部公网验证仍是后续目标。个人电脑 Lite/Desktop 的完整 public 流程须明确休眠、合盖、关机、断网、上行带宽、动态地址、后台服务和更新/恢复行为。缺少路由器映射、可入站 IPv6、反代或用户选定隧道条件时，只显示准确原因和教程，不假称公网可达。

这些是目标合同，不是当前 `myapi lan` 命令提供的功能。实际迁移和形态切换必须通过兼容/备份/排空/健康/回退状态机；数据库引擎和跨机器迁移另行执行。

## Security checklist

- Keep the listener on loopback unless a private-network share is intentional.
- Prefer a concrete private IP over `0.0.0.0`.
- Use a separate downstream key for every colleague.
- Restrict model access, quota, concurrency, and rate limits.
- Keep Docker Desktop and the host OS updated.
- Use a VPN/Tailscale or an authenticated reverse proxy for networks you do not fully trust.
- Never expose the current Legacy LAN listener directly to the public Internet. Future Lite/Desktop public access must use the separate, explicitly confirmed and externally verified public-access workflow; it is not enabled by this configuration.
- Never put `deploy/.env`, SQLite files, logs, or API keys in Git.

## Legacy 跨平台合同验收（不启动服务）

Before handing a LAN Lite project to a colleague, run the dependency-free
acceptance harness from the source distribution:

<!-- tutorial-reference: lite-advanced-6 -->
兼容/平台参考，未纳入本批可执行教程验收。 / Compatibility or platform reference, not a qualified current tutorial step.
```text
npm run lan:check
```

The desktop packaging contract can be checked independently (also on Linux):

<!-- tutorial-reference: lite-advanced-7 -->
兼容/平台参考，未纳入本批可执行教程验收。 / Compatibility or platform reference, not a qualified current tutorial step.
```text
npm run desktop:check
```

This verifies that the Electron manifest still exposes MyAPI macOS DMG/ZIP and
Windows NSIS/portable targets, bundles the platform binary and license files,
and that the tag-driven GitHub Actions workflow keeps its checksum and release
approval gates. It does not download Electron or build an installer.

The harness creates a temporary project, runs `node cli/myapi.mjs lan init`, checks the
loopback default, version-pinned `myapi-lan` GHCR image, generated secret file
permissions, CPU/memory guardrails, credential-file exclusion, and the
Electron/CI contract. When Docker Compose is available it runs
`docker compose config --quiet` only; this is a parser check and does not pull,
start, stop, or rebuild a container. Use `--skip-docker` on a machine without
Docker Desktop. `--json` emits machine-readable results, and `--keep-temp`
keeps the temporary project for local debugging; do not use the latter on a
shared workstation unless the directory is removed afterwards.

For an already-initialized project, inspect it without changing any files:

<!-- tutorial-reference: lite-advanced-8 -->
兼容/平台参考，未纳入本批可执行教程验收。 / Compatibility or platform reference, not a qualified current tutorial step.
```text
node tools/lan/check.mjs --project-dir ./myapi-lan --skip-docker
```

This check is intentionally platform-neutral and can run in GitHub Actions or
on Linux. It does not prove that macOS/Windows firewall rules, Docker Desktop
file sharing, installer signing, or a real colleague request work. Those
items require a real macOS and Windows rehearsal: launch the packaged app,
allow the process on the **private** network only, call `/api/status` from a
second machine, verify a downstream key, then stop and repeat after an upgrade.
Record the OS version, app version, bind address, port, image digest, and
resulting health status without recording any key or secret.
