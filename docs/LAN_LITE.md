# MyAPI LAN Lite

LAN Lite runs MyAPI as a small, private-network API gateway on a macOS or Windows workstation. Colleagues call the workstation with their own MyAPI downstream API Keys; upstream credentials remain configured in MyAPI and are never read by the CLI or printed to the terminal.

## What it is (and is not)

LAN Lite is a single-process, SQLite-first deployment for a trusted local network. It is not a public hosting mode, a credential-file importer, or an automatic firewall configurator. The default listener is loopback-only.

The CLI never scans `~/.codex`, Claude credential files, Keychain, Windows Credential Manager, or any other local credential store. Configure an upstream channel explicitly in the MyAPI administrator interface or point it at an already-authorized local API endpoint.

**CLI availability:** `@forcemind/myapi` is not yet published to NPM. Until the
maintainer publishes a reviewed release, run the examples from a source checkout
as `node cli/myapi.mjs ...` (or use a reviewed local tarball); do not let `npx`
resolve an unknown public package with the same name.

## Docker Desktop (recommended on macOS and Windows)

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

## Platform data locations

Relative `./data` and `./logs` paths are portable and are the default for a source workspace. If you choose platform-specific host paths, use a directory owned by the current user:

- macOS: `~/Library/Application Support/MyAPI/lan`
- Windows: `%LOCALAPPDATA%\\MyAPI\\lan`

Packaged Electron builds keep the backend log directory at the platform
`userData` root (`.../MyAPI/logs`) and full-content logs under its
`full-content` subdirectory. This avoids writing into the read-only app
resources directory; copying the app data directory is sufficient for a
local backup.

Docker Desktop must be allowed to share the selected directory. Do not mount an entire home directory or a credential directory into the container.

## Colleague access

1. Open the administrator UI at the displayed local URL.
2. Create one downstream API Key per colleague.
3. Give each person only the models, rate limit, and quota they need.
4. Revoke or rotate a key independently when access ends.

Colleagues receive the LAN URL and their own MyAPI key. They do not receive the upstream provider key, administrator password, local files, or another colleague's logs. Do not paste keys into chat, source control, screenshots, or issue reports.

## Stop, update, and recovery

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

## Security checklist

- Keep the listener on loopback unless a private-network share is intentional.
- Prefer a concrete private IP over `0.0.0.0`.
- Use a separate downstream key for every colleague.
- Restrict model access, quota, concurrency, and rate limits.
- Keep Docker Desktop and the host OS updated.
- Use a VPN/Tailscale or an authenticated reverse proxy for networks you do not fully trust.
- Never expose the LAN listener directly to the public Internet.
- Never put `deploy/.env`, SQLite files, logs, or API keys in Git.

## Cross-platform acceptance (no service start)

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
