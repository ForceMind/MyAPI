# Linux 原生服务预览 / Native Linux service preview

现在可以查看 MyAPI 系统服务或用户服务的目录计划，并把对应的 systemd unit 输出到终端供审阅。**当前不会安装或启动服务。** 已有服务继续使用原来的管理方式，不要拿预览模板覆盖它。

You can preview the intended directories and print a systemd unit for review. The current entry does not install, adopt, start, update or remove a service. Keep existing installations unchanged.

本文对应 B3a 的只读入口 `deploy/install-native.sh`。源码 `34ad42ac` / [CI42415882062](https://github.com/ForceMind/MyAPI/actions/runs/42415882062) 的证据包括六组合同和 systemd 解析检查；合成可执行文件的参数、环境与 PID 转交检查不等于真实 MyAPI 服务生命周期验收。本文新增的教程命令仍待 B7 Actions 按原命令执行，文稿本身不是验收证据。

## 1. 查看帮助 / Read the help

在已审阅的 MyAPI 源码根目录使用 Bash。仅预览不需要 root，不要求已安装 Go、Docker 或 MyAPI 二进制。`render-unit` 会读取仓库内模板，不读取现有配置正文、数据库或密钥。

Run from a reviewed MyAPI source checkout with Bash. These preview commands need no root access, Go compiler, Docker or installed MyAPI binary. Rendering reads the repository templates, not your existing configuration or credentials.

<!-- tutorial-command: native-preview-help -->
```bash
bash deploy/install-native.sh --help
```

成功时会显示 `plan`、`render-unit` 和必须明确指定的 `--scope system|user`。缺参数、未知 scope、重复参数以及 `install`、`upgrade`、`rollback`、`uninstall` 均非零退出，不能用它们执行安装或恢复。

The help lists the two preview operations. Missing, duplicate or unknown arguments fail. Installation, upgrade, rollback and uninstall are unavailable rather than partially implemented.

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

## 4. 下一步与停止边界 / What this preview cannot do

看到 `plan` 或 `render-unit` 成功，表示路径格式和模板输出符合当前合同；不表示二进制可信、配置完整、数据库兼容、服务已安装或 HTTP 可用。`Restart=on-failure` 与 `TimeoutStopSec=infinity` 是模板声明；停止未确认不能当作备份、替换或重启许可。

A successful preview is not a working installation. Artifact trust, configuration, database compatibility, startup, shutdown and recovery still require their own implementation and evidence. An unconfirmed stop is never permission to replace or restore data.

目前没有本教程可用的 `systemctl install/enable/start`、自动下载、签名验证、升级或卸载步骤。T1 的 GitHub OIDC / Sigstore 是已确定的信任合同方向，尚未接线到下载、验签或安装；不需要新建签名 Secret，也不能改用未审阅的坐标或 `latest`。

There is no installation command to run after this preview. The chosen T1 trust contract is not a connected downloader or signature verifier. It does not authorize creating signing secrets, changing artifact coordinates or fetching an unreviewed latest release.

继续使用容器可看 [个人电脑分流](PERSONAL_COMPUTER.md) 和 [Lite / Legacy LAN](LAN_LITE.md)。数据保护先看 [备份与隔离恢复](BACKUP_RESTORE.md)：beta.46 的已证范围仅注册的 Root Docker Full + SQLite，不能直接套到原生或 Desktop；beta.47 清理候选仍待验证。

For an existing supported container workflow, use the personal-computer and Lite guides. The current backup qualification is limited to registered root Docker Full + SQLite; native and Desktop recovery are not qualified by it.

**未验 / Not verified:** `EnvironmentFile` 被真实 manager 加载、专用账号和实际权限、服务安装/启动/停止/重启、睡眠恢复、真实 MyAPI 原生运行、更新及回滚。本页不提供这些能力的完成声明。
