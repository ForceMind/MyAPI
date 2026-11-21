# Linux 原生服务 / Native Linux service

可以预览 MyAPI 系统服务或用户服务的路径和 systemd 单元，也可以从签名包首次安装。首次安装遇到已有配置、目录、服务或占用的端口会停止。

You can preview the intended directories and systemd unit, then install a fresh service from a signed local package. Existing paths, services or occupied ports cause installation to stop.

按本文顺序操作：预览路径 → 核验本地发行包 → 首次安装 → 升级、只回程序或保数据卸载 → 读取操作状态。已有受管服务从升级或卸载预检开始，不再次执行首次安装。容器部署见 [Full](MYAPI_DISTRIBUTION.md) 与 [Lite](LAN_LITE.md)，个人电脑入口见 [个人电脑](PERSONAL_COMPUTER.md)。

Follow the sections in order: preview → verify a local release → first install → upgrade, code rollback or data-preserving uninstall → inspect status. For an existing managed service, start at its transition plan. See [Full](MYAPI_DISTRIBUTION.md), [Lite](LAN_LITE.md) or [personal computer](PERSONAL_COMPUTER.md) for the other deployment paths.

## 1. 查看帮助 / Read the help

在已审阅的 MyAPI 源码根目录使用 Bash。仅预览不需要 root，不要求已安装 Go、Docker 或 MyAPI 二进制。`render-unit` 会读取仓库内模板，不读取现有配置正文、数据库或密钥。

Run from a reviewed MyAPI source checkout with Bash. These preview commands need no root access, Go compiler, Docker or installed MyAPI binary. Rendering reads the repository templates, not your existing configuration or credentials.

<!-- tutorial-command: native-preview-help -->
```bash
bash deploy/install-native.sh --help
```

帮助列出预览、包核验、首次安装、升级和状态查询命令。缺参数、未知 scope 或重复参数会被拒绝。

The help lists the preview, verification, first install, upgrade and status commands. Missing, duplicate or unknown arguments fail.

## 2. 系统服务：只看计划 / Preview a system service

<!-- tutorial-command: native-preview-system-plan -->
```bash
bash deploy/install-native.sh plan --scope system
```

计划使用固定路径，不依赖 `HOME` 或 XDG 环境变量。表中的专用账号和目录都是目标合同，脚本不会创建账号、目录或文件，也不核对它们当前是否存在或可写。

System paths are fixed. The dedicated account and directories below are intended locations, not an inspection of this host. No account, folder or configuration is created.

| 用途 / Purpose | 目标 / Intended value |
| --- | --- |
| 服务账号 / Service account | `my-api` user and group |
| 当前程序 / Current binary | `/opt/my-api/current/my-api` |
| 版本目录 / Releases | `/opt/my-api/releases` |
| 必需配置 / Required configuration | `/etc/my-api/my-api.env` |
| 数据 / Data | `/var/lib/my-api/data` |
| 安装状态 / Installation state | `/var/lib/my-api/state` |
| 备份 / Backups | `/var/lib/my-api/backups` |
| 日志 / Logs | `/var/log/my-api` |
| 缓存 / Cache | `/var/cache/my-api` |
| unit 目标 / Intended unit | `/etc/systemd/system/my-api.service` |

<!-- tutorial-command: native-preview-system-render -->
```bash
bash deploy/install-native.sh render-unit --scope system
```

输出仅是 unit 正文，不会保存或启用它。系统模板的工作目录是数据目录，SQLite 新安装缺省为该目录内的 `my-api.db?_busy_timeout=30000`；必需的 `EnvironmentFile` 可以覆盖缺省。日志目录通过 `--log-dir` 指定。模板不决定功能版、监听地址或端口，也没有生成随机密钥。

The rendered text uses the data directory as its working directory and supplies a new-install SQLite default. The required environment file may override that default. Logs have an explicit directory; edition, listener, port and secrets are not invented by the template.

系统模板包含 `UMask=0077`、`NoNewPrivileges=true`、`ProtectSystem=strict` 和 `ProtectHome=true`；应用可写范围仅为 data、logs、cache，不把安装状态和备份目录交给应用写入。这些声明不证明现有目录权限、用户账号或宿主 systemd 设置已经适用。

The system template restricts application writes to data, logs and cache. These unit settings do not prove that permissions, accounts or host capabilities have been configured correctly.

## 3. 用户服务：核对自己的路径 / Preview a user service

<!-- tutorial-command: native-preview-user-plan -->
```bash
bash deploy/install-native.sh plan --scope user
```

用户模式需要有效的绝对 `HOME`。未设置 XDG 变量时使用下表缺省；已设置时使用其明确值。显式空值、相对路径、控制字符及 `.` / `..` 路径段会拒绝。不要为了试教程修改真实 `HOME`；CI 在子进程环境里使用自己的临时 fixture。

User scope requires a valid absolute home directory. Unset XDG variables use these defaults; explicitly empty or unsafe values fail. Do not change your real home directory to follow the tutorial.

| 基目录 / Base | 未设置时 / When unset | MyAPI 后缀 / Suffixes |
| --- | --- | --- |
| `XDG_DATA_HOME` | `$HOME/.local/share` | `my-api/current/my-api`, `my-api/releases`, `my-api/data` |
| `XDG_CONFIG_HOME` | `$HOME/.config` | `my-api/my-api.env`, `systemd/user/my-api.service` |
| `XDG_STATE_HOME` | `$HOME/.local/state` | `my-api/state`, `my-api/backups`, `my-api/logs` |
| `XDG_CACHE_HOME` | `$HOME/.cache` | `my-api` |

数据/程序与安装状态/备份分开。`account: current-user` 表示未来由调用者的用户 manager 运行，不会创建系统账号、开通 lingering 或安装系统级服务。

Application data and releases are separate from state and backups. User scope targets the invoking user's manager; the preview does not create a system account or enable lingering.

<!-- tutorial-command: native-preview-user-render -->
```bash
bash deploy/install-native.sh render-unit --scope user
```

使用渲染结果审阅，不要直接安装带 `@DATA@` 等占位符的 `deploy/my-api-user.service` 源模板。渲染器对 systemd 的路径、specifier 和参数分别转义；固定 `/bin/sh` 转交脚本只接收独立位置参数，不把路径拼成 shell 程序。用户模板不依赖系统服务的 namespace 文件系统限制，也不声明系统 `network.target`。

Review the rendered result, not the placeholder template. Paths remain positional data in the fixed shell handoff. The user unit avoids system-only filesystem namespace settings and does not claim a system network dependency.

## 本地包核验 / Local package verification

以下命令从已审阅的源码根目录运行。先选定一种范围；系统 Full 用下方值，系统 Lite 将 `EDITION` 改为 `lite`。个人 Lite 使用自己的普通账号，将 `SCOPE` 改为 `user`、`EDITION` 改为 `lite`，并确保该账号的 systemd 用户管理器正在运行。系统范围的核验可由普通账号执行，安装及服务操作须在 root shell 中执行；切换 shell 后须重新设置这些变量并回到源码根目录。

Run from the reviewed source root. Select system Full below, system Lite with `EDITION=lite`, or personal Lite with `SCOPE=user` and `EDITION=lite` as the installation account. That account needs a running systemd user manager. Package verification needs no root; system installation and service operations need a root shell. Set the variables again if you switch shells.

把占位值换成同一真实发行的标签和本地文件绝对路径；路径可以包含空格。本文不下载文件，也不提供生产签名材料。`COSIGN` 必须是对应 CPU 的固定 Cosign v3.1.4，`TRUSTED_ROOT` 必须是安装器固定摘要对应的 Sigstore 信任根，不是自行生成的根。

Replace the placeholders with one release's exact tag and absolute local file paths. Quoted paths may contain spaces. Obtain trusted release materials separately. Use the pinned Cosign v3.1.4 for your CPU and the pinned Sigstore root; a self-generated root is rejected.

```bash
SCOPE=system
EDITION=full
PORT=3000
RELEASE_TAG='v<release-version>'
RELEASE_MANIFEST='/absolute/path/to/release-manifest.json'
RELEASE_BUNDLE='/absolute/path/to/release-manifest.sigstore.json'
RELEASE_BINARY='/absolute/path/to/my-api'
COSIGN='/absolute/path/to/cosign'
TRUSTED_ROOT='/absolute/path/to/trusted_root.json'
```

<!-- native-service-command: verify-package -->
```bash
bash deploy/install-native.sh verify-package --scope "${SCOPE:?}" --edition "${EDITION:?}" --tag "${RELEASE_TAG:?}" \
  --manifest "${RELEASE_MANIFEST:?}" --bundle "${RELEASE_BUNDLE:?}" --package "${RELEASE_BINARY:?}" \
  --cosign "${COSIGN:?}" --root "${TRUSTED_ROOT:?}"
```

`deploy/install-native.sh verify-package` 接收明确的 `--scope`、`--edition` 和 `--tag`，以及本地 `--manifest`、`--bundle`、`--package`、`--cosign` 和 `--root` 文件。系统服务支持 Full 或 Lite，用户服务支持 Lite。命令在 Linux amd64 或 arm64 上运行，需要 Node.js 20 或更新版本，并使用对应架构的固定 Cosign v3.1.4 和固定 Sigstore 信任根。

命令离线核验原始 Manifest 的 GitHub OIDC 签名，签名身份绑定 `ForceMind/MyAPI` 的 `release.yml` 与所选标签。签名通过后，按版本、安装形态和 CPU 选择唯一的二进制，核对来源、ELF 架构和 SHA-256。成功时输出 JSON，拒绝时返回错误原因。安装命令会对自己的暂存副本重新核验。

The command verifies a local package offline. It binds the manifest signature to the MyAPI release workflow and exact tag, then checks the signed target and local binary digest. The install command verifies its own staged copy. A MyAPI-signed release package is still needed to exercise the production signing identity end to end.

核验成功的 JSON 包含 `version`、`scope`、`edition`、`arch`、`artifact`、`sourceSha` 和 `sha256`，不会安装或启动服务。先核对版本、范围和摘要，再继续。拒绝时退出码为 2。

Verification returns the selected release identity and SHA-256 as JSON without installing or starting a service. Check the version, scope, edition and digest before continuing. Refusal exits with code 2.

## 首次安装 / First installation

使用核验时的同一组文件运行 `install`。系统服务需要 root，支持 Full 和 Lite。用户服务需以目标用户运行，支持 Lite，并需要已运行的 systemd 用户管理器。两种方式均需系统提供 `ss`，系统安装还使用 `getent` 和 `useradd` 创建专用账号。

<!-- native-service-command: install -->
```bash
bash deploy/install-native.sh install --scope "${SCOPE:?}" --edition "${EDITION:?}" --tag "${RELEASE_TAG:?}" \
  --manifest "${RELEASE_MANIFEST:?}" --bundle "${RELEASE_BUNDLE:?}" --package "${RELEASE_BINARY:?}" \
  --cosign "${COSIGN:?}" --root "${TRUSTED_ROOT:?}" --port "${PORT:?}"
```

安装目录的上级路径必须由 root 或安装用户拥有，且不可被组内其他用户或其他账号写入。例如，若系统将 `/var/log` 设为组可写，安装器会在写入前报出该路径，供管理员核对宿主权限策略。

新服务监听 `127.0.0.1:3000`。可用 `--port` 选择其他 1024–65535 端口。安装器为新配置生成会话密钥，在独立数据目录初始化 SQLite，并验证启动进程和状态接口。成功后打印访问地址并保存安装记录。开机启动由用户另行决定。

安装成功的 JSON 为 `status: healthy`，含 `url` 和安装身份；收据保存在预览列出的 `state/installation.json`。浏览器打开 `url` 进行应用初始设置。遇到 `installation_incomplete` 或 `stop_unconfirmed`，保留现场，按错误中的 `stage`、`action`、`role`、`cause` 和服务状态排查，不反复运行安装或删除已有数据。

Successful installation returns `status: healthy`, a local `url` and release identity, and saves `state/installation.json`. Open the returned URL for application setup. If installation or stop is incomplete, preserve the files and inspect the reported stage, action, role, cause and service state before taking recovery action.

Run `install` with the same verified files. Use root for a Full or Lite system service. Use the intended account and its running systemd user manager for a Lite user service. The service listens on loopback. The default port is 3000. If installation fails after creating files, review the reported error and retain those files for recovery.

## 检查已运行的服务能否升级 / Check a running installation

在已由 `install` 建立的服务旁运行 `upgrade-plan`。准备当前版本和目标版本各自的原始 Manifest 与签名 bundle、目标二进制以及固定的 Cosign 和信任根。下例中的文件名和标签须替换为所选发行的真实材料。系统服务以 root 运行命令。个人版使用自己的账户，选择 `--scope user --edition lite`。

初次升级时，把刚安装版本的 `RELEASE_MANIFEST` 与 `RELEASE_BUNDLE` 分别填入 `CURRENT_MANIFEST` 与 `CURRENT_BUNDLE`。把目标新版发行的标签和三个绝对路径分别填入 `TARGET_TAG`、`TARGET_MANIFEST`、`TARGET_BUNDLE`、`TARGET_BINARY`。后续每次操作均以实际正在运行的版本重设 `CURRENT_*`；保留已设置的 `SCOPE`、`EDITION`、`COSIGN` 和 `TRUSTED_ROOT`。

For the first upgrade, set `CURRENT_MANIFEST` and `CURRENT_BUNDLE` to the installed release files. Set `TARGET_TAG`, `TARGET_MANIFEST`, `TARGET_BUNDLE` and `TARGET_BINARY` to the newer release. Before every later operation, update `CURRENT_*` to the release actually running. Keep the selected scope, edition and pinned verifier variables.

```bash
CURRENT_MANIFEST="${RELEASE_MANIFEST:?}"
CURRENT_BUNDLE="${RELEASE_BUNDLE:?}"
TARGET_TAG='v<newer-release-version>'
TARGET_MANIFEST='/absolute/path/to/newer-manifest.json'
TARGET_BUNDLE='/absolute/path/to/newer-manifest.sigstore.json'
TARGET_BINARY='/absolute/path/to/newer-my-api'
```

<!-- native-service-command: upgrade-plan -->
```bash
bash deploy/install-native.sh upgrade-plan --scope "${SCOPE:?}" --edition "${EDITION:?}" --tag "${TARGET_TAG:?}" \
  --current-manifest "${CURRENT_MANIFEST:?}" --current-bundle "${CURRENT_BUNDLE:?}" \
  --manifest "${TARGET_MANIFEST:?}" --bundle "${TARGET_BUNDLE:?}" --package "${TARGET_BINARY:?}" \
  --cosign "${COSIGN:?}" --root "${TRUSTED_ROOT:?}"
```

返回 `eligible: true` 时，可查看当前和目标版本、运行服务的 PID、实际 SQLite 路径和未来停服步骤。配置和数据的 schema 版本取自已验签的双方 Manifest。SQLite 的现场证据来自运行进程打开的文件及其格式。预检保留正在服务的进程、配置和数据，Cosign 仅使用并清理自己的临时目录。

拒绝时命令以状态码 2 返回 `code` 和 `reason`。安装记录问题先核对 `state/installation.json`；服务不符时核对当前程序、systemd 单元与运行账户；SQLite 不符时核对服务配置和数据目录。不要用改写收据或替换数据库的方式让预检通过。

Use `upgrade-plan` on a running managed system or user installation. Supply both original release manifests with their respective signature bundles, the target binary and the pinned local trust material. Success identifies the installed and target versions, service PID, actual SQLite file and the steps that would require downtime. The configuration and data schema versions come from the signed declarations. A refusal returns exit code 2 and identifies the record, service, database or release material to inspect. This command leaves the service running.

## 确认升级与查看结果 / Confirm an upgrade and inspect its result

先读上一步返回的当前版本、目标版本、数据库路径和停服步骤。在可以短暂停机时，将该结果的 `confirmation` 原样放入 `CONFIRMATION`，使用与预检相同的本地发行文件。需要宿主的 `/usr/bin/python3`、SQLite 标准库和 systemd。系统服务用 root，用户服务用创建该服务的账户。

<!-- native-service-command: upgrade -->
```bash
bash deploy/install-native.sh upgrade --scope "${SCOPE:?}" --edition "${EDITION:?}" --tag "${TARGET_TAG:?}" \
  --current-manifest "${CURRENT_MANIFEST:?}" --current-bundle "${CURRENT_BUNDLE:?}" \
  --manifest "${TARGET_MANIFEST:?}" --bundle "${TARGET_BUNDLE:?}" --package "${TARGET_BINARY:?}" \
  --cosign "${COSIGN:?}" --root "${TRUSTED_ROOT:?}" --confirm "${CONFIRMATION:?}"
```

程序会重新验签并核对运行现场。在确认原进程已排空并退出后，将此刻的数据目录、SQLite 及存在的 WAL、配置和安装记录保存到 `backups/<confirmation>/`。核验备份后才切换程序。新服务继续打开原数据库。状态接口、配置、数据库身份和历史账务均能读回时，结果为 `status: healthy`。预检后正常新增的账务纳入本次备份。启用 Redis 批量记账的实例，其待写金额无法从本地 SQLite 核实，命令会提示检查。

Review the source version, target version, database and downtime first. Run `upgrade` with the same local release files and the exact confirmation from the plan. The installer checks the running process again, verifies its clean stop, backs up the latest data and SQLite sidecars, verifies the backup, and switches the program. A healthy result includes the backup path and accounting check. The service continues using the same database.

Linux 原生备份会核对数据、配置和安装记录的所有者、权限位及 POSIX 访问控制列表，包括目录上供新文件继承的默认列表。发现其他扩展属性时预检会说明原因。若备份文件系统无法原样保存权限，升级会停在程序切换之前。可运行 `transition-status` 查看现场。

Native backups verify the owner, mode and POSIX access ACL of the data, configuration and installation record, including default ACLs on directories. Preflight reports unsupported extended attributes. If the backup cannot preserve these permissions, the transition stops before switching the program. Use `transition-status` to inspect progress.

## 回退程序，保留最新数据 / Roll back the program and keep current data

先核对当前安装对应的发行文件。将新版 Manifest 和签名包传给 `--current-manifest` 与 `--current-bundle`，将较早版本的发行文件传给目标参数。当前版本的签名声明必须明确允许这对程序在同一配置和数据 schema 上进行 `code_only` 回退。个人安装用 `--scope user --edition lite`。

把正在运行的新版材料填入 `CURRENT_MANIFEST`、`CURRENT_BUNDLE`，把较早版本填入 `OLDER_TAG`、`OLDER_MANIFEST`、`OLDER_BUNDLE`、`OLDER_BINARY`。回退只切换程序，不恢复旧数据库或清零较新账务。回退预检会产生新的确认值，须重新设置 `CONFIRMATION`，不能沿用升级确认。

Set `CURRENT_*` to the running newer release, and `OLDER_TAG`, `OLDER_MANIFEST`, `OLDER_BUNDLE`, `OLDER_BINARY` to the older release. Rollback keeps the latest database and accounting history. Copy this new plan's confirmation into `CONFIRMATION`; an upgrade confirmation cannot authorize rollback.

```bash
CURRENT_MANIFEST="${TARGET_MANIFEST:?}"
CURRENT_BUNDLE="${TARGET_BUNDLE:?}"
OLDER_TAG="${RELEASE_TAG:?}"
OLDER_MANIFEST="${RELEASE_MANIFEST:?}"
OLDER_BUNDLE="${RELEASE_BUNDLE:?}"
OLDER_BINARY="${RELEASE_BINARY:?}"
```

<!-- native-service-command: rollback-plan -->
```bash
bash deploy/install-native.sh rollback-plan --scope "${SCOPE:?}" --edition "${EDITION:?}" --tag "${OLDER_TAG:?}" \
  --current-manifest "${CURRENT_MANIFEST:?}" --current-bundle "${CURRENT_BUNDLE:?}" \
  --manifest "${OLDER_MANIFEST:?}" --bundle "${OLDER_BUNDLE:?}" --package "${OLDER_BINARY:?}" \
  --cosign "${COSIGN:?}" --root "${TRUSTED_ROOT:?}"
```

预检返回正在运行的版本、可回退到的版本、SQLite 文件和 `dataPolicy: keep_current_sqlite`。`reusesInstalledBinary` 表示可使用经过归属与签名摘要检查的保留程序。目标目录不存在时，可用已核验的目标程序包暂存。若目标目录已存在但归属或内容无法确认，先核对该目录与安装记录。

在停机窗口内，用预检的 `confirmation` 及同一组文件确认回退。服务排空后生成新的备份，旧程序重新打开当前 SQLite，并核对账务及健康状态。

<!-- native-service-command: rollback -->
```bash
bash deploy/install-native.sh rollback --scope "${SCOPE:?}" --edition "${EDITION:?}" --tag "${OLDER_TAG:?}" \
  --current-manifest "${CURRENT_MANIFEST:?}" --current-bundle "${CURRENT_BUNDLE:?}" \
  --manifest "${OLDER_MANIFEST:?}" --bundle "${OLDER_BUNDLE:?}" --package "${OLDER_BINARY:?}" \
  --cosign "${COSIGN:?}" --root "${TRUSTED_ROOT:?}" --confirm "${CONFIRMATION:?}"
```

Supply the currently installed release as the current manifest and bundle, and the older release as the target. The newer signed manifest must expressly allow this exact code-only transition, and both releases must declare matching configuration and SQLite schemas. Confirm the plan during a downtime window. The command verifies a fresh backup, starts the older program on the same SQLite database, and checks accounting and health. If interrupted, inspect `transition-status` before proceeding.

## 保留数据并卸载 / Uninstall and retain data

在能核对运行进程身份的受管服务上生成卸载预检。系统服务以 root 运行命令，个人 Lite 以安装它的账号运行并选择 `user`。预检显示服务、SQLite 位置、待移除的 `remove` 路径和保留的 `preserve` 路径。核对列表后，将返回的 `confirmation` 填入确认命令。

<!-- native-service-command: uninstall-plan -->
```bash
bash deploy/install-native.sh uninstall-plan --scope "${SCOPE:?}"
```

将本次卸载预检的 64 位确认值原样放入 `CONFIRMATION`，再执行下面命令。不要使用升级或回退的确认值。

Set `CONFIRMATION` to this uninstall plan's exact 64-character value before continuing.

<!-- native-service-command: uninstall -->
```bash
bash deploy/install-native.sh uninstall --scope "${SCOPE:?}" --confirm "${CONFIRMATION:?}"
```

确认命令重新核对服务及程序，排空并证实原进程已退出，再移除受管单元、标准的受管启动链接（如已启用）、当前程序链接和登记的版本程序。原服务账号、配置、SQLite 及存在的 WAL、日志、备份、缓存和安装记录留存。发现外来单元、附加配置、归属不符或无法证实停服时，先用 `transition-status` 查看阶段和实际服务。结果为 `status: uninstalled` 时，已核对服务退出和保留路径。

Run the plan against the managed service while it is running. Use root for a system service or the installation account for `--scope user`. Review the exact removal and preservation paths, then supply the plan confirmation. The service drains before the managed unit, standard enable link and registered program files are removed. The installation account, configuration, SQLite, logs, backups, cache and receipt remain available. If the command is interrupted, inspect `transition-status` for the recorded phase and observed service state.

## 查看结果与中断现场 / Read the result and interruption state

<!-- native-service-command: transition-status -->
```bash
bash deploy/install-native.sh transition-status --scope "${SCOPE:?}"
```

用户服务将 `system` 换成 `user`。`operation` 标明升级、回退或卸载，`phase` 显示最近写入记录的步骤，`activeBinary` 和 `service` 显示当前指针、服务状态和 PID。`committed` 表示程序切换已经核验，`uninstalled` 表示卸载已核验。`stopped` 表示上一进程已确认退出。`*_requested` 表示操作已经登记、结果还有待核对。遇到断电、响应丢失或状态不明，先看此输出，再检查安装记录中的路径和服务日志。保留最新数据库、WAL、备份和未完成的记录。再次提交同一确认不会重跑。

Use `--scope user` for a personal service. After interruption, inspect the recorded phase, selected program and service process before taking action. A requested phase records intent. The service or files may have changed since that phase was written. A repeated confirmation does not replay the operation.

## 证据与停止边界 / Evidence and stop boundaries

看到 `plan` 或 `render-unit` 成功，表示路径格式和模板输出符合当前合同；不表示二进制可信、配置完整、数据库兼容、服务已安装或 HTTP 可用。`Restart=on-failure` 与 `TimeoutStopSec=infinity` 是模板声明；停止未确认不能当作备份、替换或重启许可。

A successful preview establishes the path and unit template only. The implemented installation and transition commands perform their separate package, process, database, startup and stop checks. An unconfirmed stop is never permission to replace or restore data.

首次安装使用上文的 `install`。运行中的服务可预检升级、程序回退或保数据卸载，并在约定的停机窗口内确认。升级与回退的生产签名材料须由受信发行提供。

Use `install` for a fresh service. On a running installation, review the upgrade, code rollback or data-preserving uninstall plan and confirm it during the downtime window. Supply the trusted release signatures for upgrades and rollbacks.

容器数据保护另见 [备份与隔离恢复](BACKUP_RESTORE.md)。同源码真实 system Full、system Lite、非特权 user Lite 的安装、升级、程序回退、再升和保数据卸载已有 [原生专项 CI](https://github.com/ForceMind/MyAPI/actions/runs/44823701093) 证据；签名用的是隔离夹具中的 Ed25519 材料。新增教程参数与回执仍须本次源码的 Actions 通过。实际 launcher 执行、绑定文档参数的注入夹具和正式发行验签是三种证据，不相互替代。夹具不能被生产命令接受；正式 MyAPI 发行签名材料的端到端验收仍待提供受信发行。

The [native qualification](https://github.com/ForceMind/MyAPI/actions/runs/44823701093) exercised same-source real system Full/Lite and unprivileged user Lite services through installation, upgrade, code rollback, re-upgrade and data-preserving uninstall. It used isolated Ed25519 fixture signatures. The new tutorial parameter and receipt checks await this source's Actions run. Actual launcher execution, document-bound injected fixtures and production release verification are reported separately. The production launcher rejects fixture trust material. End-to-end verification of a formally signed MyAPI release still requires that trusted release.
