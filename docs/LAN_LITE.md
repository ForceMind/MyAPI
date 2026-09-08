# My API Lite 与 Legacy LAN 迁移

> 当前实现状态：本文件中的 `myapi lan`、`MYAPI_EDITION=lan` 和 `myapi-lan` 镜像是 Legacy LAN 合同。Full/Lite/Desktop 的正式产品定位、服务器 Lite、个人电脑 Lite、public access 向导和统一更新器仍未实现；现有 schema-1 Release Manifest 仅是未接线的结构选择内核，另有不接线的 Legacy 画像解析器可将显式旧配置 fail-closed 标为 local/LAN/needs_manual，二者均不能安装、更新或改变访问范围。

Lite 是面向个人和小规模使用的正式 My API 功能版，可部署在个人服务器、云服务器、VPS 或个人电脑。轻量化意味着 SQLite-first、低依赖、低资源和精简默认流程；它不意味着只能在局域网使用。Desktop 是 Lite 的桌面安装和管理形态，业务核心和数据模型必须与 Lite 一致。

功能版（Full/Lite）、安装形态（服务器/个人电脑/容器/Desktop）和访问模式（本机/LAN/public）是独立维度。个人电脑 Lite/Desktop 默认仅本机访问；LAN 分享和 public 开放均需单独、明确开启。public 模式需要环境检查、用户确认和外部网络验证，不能用本机监听或健康检查证明；它也绝不自动开启匿名调用、公开注册、无认证管理页、宿主文件访问或 Codex 权限。

完整的发行、安装、更新、切换、数据目录和 public 向导合同见 [发行制品、安装与更新合同](RELEASE_MANIFEST.md)。本页保留当前 Legacy LAN 的安全行为、迁移边界和可执行命令，不能把它们误读为未来 Lite 的全部能力。

## 当前 Legacy LAN 行为（和它不代表的内容）

Legacy LAN 是当前单进程、SQLite-first 的可信本机/私网部署。默认监听回环；`--allow-lan` 才允许私网监听。当前实现不是 public hosting 模式、凭据文件导入器、自动防火墙配置器、服务器 Lite 安装器或完整 Desktop 更新器。

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

```bash
myapi lan init ./myapi-lan
myapi lan start --project-dir ./myapi-lan
```

Use a current Docker Desktop release (Compose v2.17 or newer); the start
command uses Compose's bounded `--wait` health check and does not treat a
running-but-unhealthy container as ready.

The release image is in the private MyAPI GHCR package. If Docker Desktop is
not already authenticated, sign in once with a GitHub token that can read the
package (`docker login ghcr.io`) before starting the service.

The initializer creates a private `deploy/.env`, a random session secret, SQLite/data directories, and the LAN image configuration. It does not start Docker or change host firewall rules.

The generated deployment also sets conservative Docker guardrails
(`MYAPI_CPU_LIMIT=2.0` and `MYAPI_MEMORY_LIMIT=2g`). Adjust these values in
`deploy/.env` only after measuring the workstation workload; `myapi lan start`
validates the values before pulling an image. Startup waits up to 120 seconds
for the container healthcheck, so a failed image or configuration is reported
instead of being presented as a usable LAN endpoint.

On Windows PowerShell the same commands work with a Windows path, for example
`myapi lan init "$env:LOCALAPPDATA\\MyAPI\\lan-project"`.

To share deliberately on a private IPv4 network, opt in and bind to the workstation's private address:

```bash
myapi lan start \
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

Electron 的托盘菜单提供 **LAN status and connection help…** 只读入口，显示当前
监听端点、回环/LAN 模式和 macOS/Windows/Linux 防火墙提示。监听地址和端口在启动时
确定，状态页不会执行不安全的热切换；修改配置后退出并用新参数重新启动。
若绑定 `0.0.0.0`，托盘和帮助对话框会列出启动时发现的 RFC1918 IPv4 端点；没有发现
候选地址时会明确提示，而不会显示不可访问的占位 URL。

PowerShell uses the same flags (use a backtick for line continuation):

```powershell
myapi lan start `
  --project-dir "$env:LOCALAPPDATA\MyAPI\lan-project" `
  --bind-address 192.168.1.20 `
  --port 3000 `
  --allow-lan
```

Check the configured endpoint without exposing secrets:

```bash
myapi lan status --project-dir ./myapi-lan
myapi lan stop --project-dir ./myapi-lan
```

The status output only shows the listener and deployment state. For a `0.0.0.0` listener it prints only discovered RFC1918 IPv4 endpoint candidates, never public or IPv6 addresses; if discovery finds none it reports that explicitly rather than showing a fake URL. It never prints `SESSION_SECRET`, upstream keys, downstream keys, OAuth JSON, cookies, or JWTs.

## 当前 Legacy 数据位置

Relative `./data` and `./logs` paths are portable and are the default for a source workspace. If you choose platform-specific host paths, use a directory owned by the current user:

- macOS: `~/Library/Application Support/MyAPI/lan`
- Windows: `%LOCALAPPDATA%\\MyAPI\\lan`

Packaged Electron builds keep the backend log directory at the platform
`userData` root (`.../MyAPI/logs`) and full-content logs under its
`full-content` subdirectory. This avoids writing into the read-only app
resources directory; copying the app data directory is sufficient for a
local backup.

Docker Desktop must be allowed to share the selected directory. Do not mount an entire home directory or a credential directory into the container.

## 当前 Legacy LAN 同事访问

1. Open the administrator UI at the displayed local URL.
2. Create one downstream API Key per colleague.
3. Give each person only the models, rate limit, and quota they need.
4. Revoke or rotate a key independently when access ends.

Colleagues receive the LAN URL and their own MyAPI key. They do not receive the upstream provider key, administrator password, local files, or another colleague's logs. Do not paste keys into chat, source control, screenshots, or issue reports.

## 当前 Legacy 停止、升级与恢复

`myapi lan stop` stops the LAN compose project without removing its data volumes. An update is explicit:

```bash
myapi lan stop --project-dir ./myapi-lan
# update the pinned MYAPI_IMAGE in deploy/.env when you are ready
myapi lan start --project-dir ./myapi-lan
```

GitHub Actions builds and signs the `ghcr.io/forcemind/myapi-lan:<version>` image, but does not restart your workstation. Keep the image pinned to a version and retain the previous image until the new health check succeeds.

桌面安装包的构建合同与实际签名是两件事：当前 workflow 可以生成 macOS DMG/ZIP 和
Windows NSIS/portable 制品，但是否签名取决于维护者提供的 Apple Developer ID、
notarization 凭据或 Windows 代码签名证书。未签名的 macOS 包可能触发 Gatekeeper，
Windows 包可能触发 SmartScreen；在证书和发布审批准备好前，应把 Actions artifact
视为测试制品，不要宣称为受信任的正式安装包。

## Lite 服务器、个人电脑与 Desktop 的目标迁移

未来受管安装会先显示当前功能版、安装形态、访问模式、数据目录和版本，再从已验证 Release Manifest 选择目标制品。Legacy LAN 升级映射为 Lite/local 或 Lite/lan，保留数据库、配置、日志、Key、渠道、账务、S5-P 资料和 Codex 应用备份；不会自动转数据库、改监听、开放 public、修改防火墙或授予宿主文件权限。

服务器 Lite 将提供原生与容器安装、SQLite-first 默认、数据/日志位置、服务运行、自动启动、备份恢复、域名/HTTPS/反向代理和经授权 public 访问教程。个人电脑 Lite/Desktop 的 public 向导会单独提示休眠、合盖、关机、断网、上行带宽、动态地址、后台服务和更新/恢复行为。缺少路由器映射、可入站 IPv6、反代或用户选定隧道条件时，只显示准确原因和教程，不假称公网可达。

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

```bash
npm run lan:check
```

The desktop packaging contract can be checked independently (also on Linux):

```bash
npm run desktop:check
```

This verifies that the Electron manifest still exposes MyAPI macOS DMG/ZIP and
Windows NSIS/portable targets, bundles the platform binary and license files,
and that the tag-driven GitHub Actions workflow keeps its checksum and release
approval gates. It does not download Electron or build an installer.

The harness creates a temporary project, runs `myapi lan init`, checks the
loopback default, version-pinned `myapi-lan` GHCR image, generated secret file
permissions, CPU/memory guardrails, credential-file exclusion, and the
Electron/CI contract. When Docker Compose is available it runs
`docker compose config --quiet` only; this is a parser check and does not pull,
start, stop, or rebuild a container. Use `--skip-docker` on a machine without
Docker Desktop. `--json` emits machine-readable results, and `--keep-temp`
keeps the temporary project for local debugging; do not use the latter on a
shared workstation unless the directory is removed afterwards.

For an already-initialized project, inspect it without changing any files:

```bash
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
