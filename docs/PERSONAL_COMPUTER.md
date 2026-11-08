# 在个人电脑使用 MyAPI / MyAPI on your personal computer

已有 Docker Desktop 环境，选择 Lite 容器流程；希望从托盘控制随应用打包的本地服务，查看 Desktop 源码能力。两者的数据目录和更新路径不同，**不要把一个形态的数据直接放到另一个形态上试运行**。

Use the Lite container workflow if you already use Docker Desktop. Use the Desktop guide for the bundled local service and tray controls. Their data locations and update paths differ; neither is an automatic migration into the other.

当前教程基于 beta.47 源码，教程命令仍待 B7 Actions 执行；它不是 beta.47 安装包或正式发行证明。beta.40 的历史测试安装包仍属于其原 SHA/版本，不是重标后的 beta.47。未发布的 NPM 包、镜像 tag 或 Desktop 包不能作为本页的下载承诺。

## 1. 先选一条路 / Choose your route

| 你要做的事 / Task | 入口 / Where to go | 当前边界 / Boundary |
| --- | --- | --- |
| 在已有 Docker Desktop 上运行个人 Lite | [Lite / Legacy LAN](LAN_LITE.md) | 源码 CLI 已有 init/start/status/stop；启动仍需实际可用、已审阅的镜像和 Docker 权限 |
| 查看托盘、本地服务面板和桌面打包 | [electron/README.md](../electron/README.md) 及下文 Desktop 步骤 | 已有源码与合成合同；平台包、真实 GUI/设备另行验收 |
| 查看 Linux system/XDG 目录与 unit | [Linux 原生预览](NATIVE_LINUX.md) | 只有 help/plan/render-unit，尚不能安装服务 |
| 备份或恢复数据 | [备份与隔离恢复](BACKUP_RESTORE.md) | beta.46 仅 Root Docker Full + SQLite 已证；不是个人 Lite/Desktop 备份工具 |

Desktop 首发顺序为 macOS、再 Windows，Linux Desktop 不纳入首发支持。`electron/README.md` 中仍有 Linux 开发构建项和旧 LAN 说明；这些不能当作 Linux 发行或当前所有面板动作已经实机验收。本文的能力边界用于阅读现有源码，不新增平台承诺。

Desktop is planned for macOS first, then Windows. Linux development targets are not a first-release support claim. Existing build instructions and source contracts do not prove installer trust, a working GUI, or a real-device network path.

## 2. Docker Desktop Lite：从新项目开始 / Start a new Lite project

先准备已审阅的源码和 Node.js 22 或更新版本。实际启动需要 Docker Desktop 正常运行、Compose v2.17 或更新，以及有权读取所选镜像；这些是外部前置条件，本教程不会安装 Docker、注册账号或修改防火墙。不要通过未知的 `npx` / `latest` 包名代替已审源码。

Use Node.js 22 or newer and a reviewed source distribution. Starting the container also needs working Docker Desktop, Compose v2.17 or newer, and permission to read the chosen image. No setup command below provisions those external prerequisites.

在源码根目录先看当前帮助；这不会启动容器。

<!-- tutorial-command: personal-source-cli-help -->
```bash
node cli/myapi.mjs --help
```

创建项目会向**新的空目录**写入部署源码、私有 `deploy/.env`、随机 session secret 及数据/日志目录，不启动 Docker，不读本机 Codex、Claude 或其他凭据。非空目录会拒绝，不要指定已有数据目录。

Initialization writes a new private deployment project and session secret without starting Docker or discovering host credentials. An existing non-empty directory is refused.

下面 `MYAPI_PROJECT_DIR` 是操作者事先选择的新空目标目录，须先按本机实际情况设置；未设置时命令拒绝执行。它不是 MyAPI 的自动安装位置。Actions 会把它设为 `RUNNER_TEMP` 下本次专用的新目录，不会写用户真实项目。

`MYAPI_PROJECT_DIR` must already identify the new empty destination you chose. The guard rejects an unset value. CI supplies only its own disposable target; this is not a suggested location for your personal data.

<!-- tutorial-command: personal-lite-project-init -->
```bash
node cli/myapi.mjs lan init "${MYAPI_PROJECT_DIR:?Set a new empty project directory first}"
```

成功会显示配置路径和回环地址。`lan` 是保留的 Lite 命令别名，不会因为名字自动共享 LAN；默认仍是 `127.0.0.1:3000`。生成的镜像坐标仍为 `ghcr.io/forcemind/myapi-lan`；自动写入的源码版本 tag 不保证已经发布。

The legacy `lan` name still selects Lite. It does not enable LAN sharing by itself. The generated versioned image reference is configuration, not proof that the image has been published or is accessible.

下一步先核对 `deploy/.env` 的镜像、数据/日志位置及访问范围，保留配置和密钥。必须先取得与来源、版本及架构相符的实际镜像；不要把 tag 改成 `latest` 或以 Full 镜像凑合启动。启动、状态及停止的实际行为见 [Lite Docker Desktop 流程](LAN_LITE.md#当前-legacy-lan-docker-desktop-流程)。以下仅为格式，尖括号不可原样运行，不是发布或安装命令：

```text
node cli/myapi.mjs lan start --project-dir <YOUR_REVIEWED_PROJECT>
node cli/myapi.mjs lan status --project-dir <YOUR_REVIEWED_PROJECT>
node cli/myapi.mjs lan stop --project-dir <YOUR_REVIEWED_PROJECT>
```

Start waits for Compose health and reports failure; running alone is not readiness. Status reports the configured listener and deployment state without printing keys. Stop removes the Compose containers without removing data volumes. Preserve the project directory and its secrets. None of these observations proves an upstream model works.

普通同版升级与改功能版是两件事。现有容器实际 edition 与目标不一致时，普通升级会拒绝；只读 `doctor --target-edition` 是配置检查，不是登录已验证、资金兼容或切换授权。更新/备份先阅读对应合同，不自行删数据库或改账务设置绕过门禁。

## 3. Desktop：在本地面板操作 / Use the local Desktop panel

先核对维护者提供的实际安装包版本、源码 SHA、平台/架构、校验和及签名状态。没有相符包就先停在源码说明；构建成功、checksum 或 manifest 结构通过均不能替代平台签名、公证和实际安装验收。本文不要求运行构建、Electron 或浏览器。

For a packaged application, check the real artifact identity and platform signing status first. If no matching reviewed package exists, stop at the source instructions. This tutorial does not ask you to build or launch Electron.

已有源码中的任务顺序如下，按钮可用性以当前面板状态为准：

1. 打开托盘的本地服务面板，选择中文或 English。先看服务状态、配置的访问范围和应用版本；未知、失败或停止未确认不能按“正在运行”处理。
2. 服务可用后点 **Open console**。在本地控制台完成管理员初始化，明确配置上游连接，再创建或选择下游 API Key；MyAPI 不会自动读取系统凭据。
3. 把面板显示的实际 `/v1` Base URL 和你自己选的 Key 填到客户端。复制只针对当前已知地址；配置地址不是可达或模型可用的证明，不会替你发测试请求。
4. 关闭窗口会留在托盘；**Stop bundled service** 需确认，停止成功后应用仍可保留。**Quit MyAPI** 需先确认其持有的子进程确实关闭；停止未确认会保留状态，不伪报退出成功。开发模式的外部服务不由它接管或终止。

In the current source, the bilingual local panel separates window closing, stopping the owned bundled process, and quitting the application. Check the state before opening the console, configure a connection explicitly, and select your own downstream Key. Copying a displayed `/v1` address does not send a request or verify a model.

面板 **Data and logs** 展示实际路径。通常数据在 Electron 的平台 `userData` 下，日志单独存放；显式 SQLite 路径和旧数据库名兼容可能改变实际数据库位置，应以面板事实为准，不按示例路径猜测。session secret、数据库、Key、配置及诊断原文都不是可公开分享的文件。诊断原文仅在明确打开日志时保存到固定私有文件，分享前自行检查。

Data paths come from the current application, including explicit database overrides. A folder location alone is not a consistent backup guarantee. Review original diagnostics before sharing them.

## 4. 本机、LAN 与已有 HTTPS 入口 / Local, LAN and existing HTTPS access

本机默认只供这台电脑使用。同事要访问时，先检查真实私网地址，再通过原有 **Restart with LAN access** 入口选择候选、确认重启；用户取消或地址变化不能自动改选另一地址。防火墙、Docker Desktop 网络和另一台设备的实际访问需要分别验证，面板不自动开防火墙。

LAN sharing is an explicit listener change with confirmation. A private address or healthy local service does not prove that another computer can reach it. Host firewall and network configuration remain external responsibilities.

Desktop 的 **Public access help** 打开默认折叠的四步本地向导：选反代位置、确认已有 HTTPS 条件、输入 HTTPS 根 origin、保存后复制明确的 `/v1` 并回本地控制台选择 Key。另一台 LAN 设备的反代需要实际 LAN 监听；向导只指向原入口，不自动重启。根 origin 不接受路径前缀、账号密码、查询或片段。

The public guide records an existing HTTPS root origin supplied by you. It does not create DNS, TLS, a proxy, tunnel or public listener, read a Key, or make DNS/HTTPS/model probes. A saved entry always remains **unverified**.

清除保存入口需要确认，只清本应用记录；外部反代、DNS、隧道和路由器设置仍须你自行撤销。无法读取的记录须先明确清除，不能静默覆盖。保存、清除或复制结果未知时保留草稿，核对当前记录后再处理，不猜成功。

Clearing the local entry does not disable external infrastructure. Unknown write results must remain unknown; stale addresses are not a substitute for the current saved entry.

## 5. 保留数据，知道哪些还没验 / Preserve data and know the limits

个人电脑关机、合盖休眠、断网或退出后台都会影响访问。当前没有经实机验证的睡眠恢复、开机自启、跨机公网可达或更新恢复保证；如需持续服务，不要依靠本页假定这些已经成立。自动更新保持关闭，没有自动检查、下载或安装执行器。

Shutting down, sleeping, losing connectivity or quitting the service can interrupt access. Sleep/resume, continuous availability, external reachability and real updates are not qualified here. Automatic updates remain off.

beta.46 的 [备份与隔离恢复](BACKUP_RESTORE.md) 已由源码 `b9c4dcd4` / CI42439207854 证明限定的 Root Docker Full + SQLite 合成路径。beta.47 保留策略清理仍是待验证候选。两者都不能直接用于个人 Lite 或 Desktop，也不能把拷贝正在写入的 SQLite 目录当作已验证备份。不要删旧数据、secret 或安装目录来解决启动失败。

This is task guidance for existing source capabilities, not GUI, real-provider, cross-device, release or production acceptance. B7 command execution is still pending; no browser, real model request, installation or update was performed while writing this page.
