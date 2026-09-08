# My API 发行制品、安装、切换与更新合同

> 文档状态：P0 设计合同；已有未接入的 schema-1 纯结构校验/新安装选择、未受信 raw-bytes evidence、安装状态输入硬化和 Legacy 画像解析内核，尚未形成受信发行或安装能力。
>
> 本文不授权执行 NPM/GHCR 发布、tag 操作、生产升级、修改防火墙/路由器、安装系统服务或清理现有机器文件。

当前 `cli/lib/release-manifest.mjs` 只在合成定向测试中使用：它对**已由未来受信边界获得**的 manifest
做有界字段、URL/OCI、平台和 fresh-install-only 选择校验，并返回冻结快照。它与
`cli/lib/installation-state.mjs` 都先用 `node:util` 的 `types.isProxy` 拒绝 Proxy（含 revoked Proxy），再拒绝自定义原型、访问器、symbol/非枚举字段、稀疏数组和额外数组属性，避免未受信 JS 对象在结构检查中触发调用方代码。`cli/lib/release-manifest-evidence.mjs`
仅把 1 字节至 4 MiB 的 ArrayBuffer-backed `Uint8Array`/`Buffer` 复制为严格、冻结的未受信原始 bytes
evidence（revision 1、固定 `application/json`、canonical padded RFC4648 base64、长度和 lowercase SHA-256），并拒绝伪造 typed-array、SharedArrayBuffer 与 Proxy；它不解析、
读取文件、获取网络、验签、标记 verified 或接入任何执行者。`cli/lib/installation-state.mjs`
与共享 `canonical-artifact-reference.mjs` 仅验证内存 installation record、artifact identity 与显式 owned-file
cleanup plan。它们不拉取/解析 manifest、不验证 canonical bytes、签名或 provenance，不下载/安装/读写/清理文件，
也不支持 upgrade、switch 或 rollback；
三种受管操作在 schema 1 中明确 fail closed。D11 的权威资产、信任根和签名门槛未决定前，不能将其接入 CLI
或称为受信 Release Manifest。

`cli/lib/legacy-installation-profile.mjs` 也只处理调用方显式提供的 Legacy Docker 配置：`lan` 映射 Lite，精确 loopback/`localhost` 或明确私网才分别给出 local/LAN，broad/public/反代/缺失/冲突一律 `needs_manual`，永不推断 public。它拒绝 Proxy、访问器、未知字段与控制字符，返回冻结画像；不读取 `.env`、文件、网络、Docker 或改变 CLI、监听、镜像、数据和权限。

此前 schema-1 结构校验/ fresh-install 选择的 11 个合成 Node 用例、raw-bytes evidence 的 9 个合成 Node 用例及 Legacy 画像的 7 个合成 Node 用例均已在单核/768MiB 受限服务中通过，独立只读审查修复后无 P1/P2/P3。新增 manifest/installation-state 输入硬化已由受限串行 Node 定向测试 28/28 覆盖并经独立只读审查，无 P1/P2/P3。已有证据绝不外推为 NPM 安装、下载、签名验证、更新、
切换、回退或正式发行已可用。

## 1. 目标与当前差距

My API 后续以一个用户入口提供四种可选择交付：Full 服务器、Lite 服务器、个人电脑 Lite、Desktop（Lite 的桌面安装形态）。它们共享业务核心、数据库语义、必要资源、版本号与恢复合同；用户只安装所选形态需要的运行制品。

当前 `@forcemind/myapi` / `myapi` 是零运行时依赖的 CLI 和完整源码分发；`myapi lan`、`MYAPI_EDITION=lan`、`myapi-lan` 镜像和 Electron 仍将功能版、安装形态与私网访问混在一起。当前 `myapi upgrade` 只覆盖 Full/LAN Docker Compose：备份 `deploy/.env`、拉取镜像、等待健康检查、失败时恢复旧环境文件/镜像配置。它没有统一制品清单、安装 journal、数据库备份验证、形态切换、原生更新或 Desktop updater。

因此既有合同继续有效但不得夸大：它们是 Legacy Full/LAN 的安全边界和有限 Docker 回退，不是完整 Lite/ Desktop 发行或数据库安全升级证明。

## 2. 三维产品模型

| 维度 | 枚举 | 说明 |
| --- | --- | --- |
| 功能版 | `full`、`lite` | Full 面向团队/组织的完整治理；Lite 面向个人/小规模但保留可靠转发、鉴权、Key、日志、额度安全和必要恢复能力。 |
| 安装形态 | `server-native`、`server-container`、`personal-native`、`personal-container`、`desktop` | Desktop 只是一种 Lite 安装/管理体验，不是第三套业务实现。 |
| 访问模式 | `local`、`lan`、`public` | 本机、局域网和公网是独立选择；访问模式不由功能版或安装形态隐含决定。 |

运行记录必须分别保存 `desired_access_mode`、实际监听配置与 `externally_verified_reachability`。绑定 `0.0.0.0`、本机健康检查或局部端口可达均不能证明公网可达。

### 2.1 目标能力矩阵

| 能力 | Full | Lite | Desktop |
| --- | --- | --- | --- |
| 可靠转发、鉴权、Key、日志、额度/限额安全、必要备份恢复 | 完整 | 必须保留 | 与 Lite 完全一致 |
| 组织/用户/权限、订阅支付、复杂运维、多节点 | 完整 | 由明确能力矩阵决定；降级时保留历史数据而非删除 | 不另行实现 |
| 数据库与拓扑 | 项目声明的 SQLite/MySQL/PostgreSQL、相关可选基础设施 | SQLite-first、低依赖；其他引擎仅在 release manifest 明确列为兼容时可用 | 平台 `userData` 中的 Lite 数据模型 |
| 本机、LAN、经确认的公网访问 | 可选 | 可选 | 可选；默认本机 |
| S5-P 提示词学习与版本中心 | 支持 | 支持 | 支持；宿主文件须本机明确授权 |

Lite 不因为名称而被限制在局域网，也不因可公网访问而自动启用匿名调用、公开注册、无认证管理页、宿主文件访问或 Codex 权限。

### 2.2 Legacy LAN 兼容迁移

| 旧状态 | 解析后的目标 | 迁移不变量 |
| --- | --- | --- |
| `MYAPI_EDITION=full` | Full；安装/访问从显式记录或旧配置推导 | 不改监听、数据、镜像、权限或 URL |
| `MYAPI_EDITION=lan` 且回环/`ALLOW_LAN=false` | Lite / `local` | 不开放 LAN 或公网 |
| `MYAPI_EDITION=lan` 且私网/`ALLOW_LAN=true` | Lite / `lan` | 保持原私网范围 |
| `ghcr.io/forcemind/myapi-lan` | Legacy Lite artifact identity | 不自动改仓库或回退到 Full |
| 现有 Electron | Lite / Desktop / 原访问模式 | 保留 `userData`、数据库名兼容、应用 ID 与数据目录 |

建议先在安装记录中引入 `product_edition`、`installation_shape`、`access_mode`，并由新解析器与 Legacy `MYAPI_EDITION` 交叉校验；两个来源冲突时 fail closed。旧 CLI/环境变量的弃用窗口、最终字段名和 Lite OCI 坐标均须负责人决定后实施。不得直接把 `lan` 重命名为 `lite`，更不得让旧部署因升级变公网。

## 3. 统一 NPM 入口与 Release Manifest

保持现有公开坐标 `@forcemind/myapi` 与 CLI 名称 `myapi`，不新增或重命名发布坐标。推荐长期将 NPM 包演进为轻量 control-plane CLI，保留 `init` 的源码能力但不要让每个运行安装携带全量跨平台源码和制品。NPM 的 `install`/`preinstall`/`postinstall` 必须零副作用：不下载运行制品、不运行 Docker、不启动服务、不修改防火墙或系统服务、不读取密钥。

推荐“统一入口 + 已签名版本清单 + 按需下载”而非把所有二进制/镜像/桌面包塞入 NPM tarball：

| 层 | 职责 | 不负责 |
| --- | --- | --- |
| `@forcemind/myapi` | 交互式选择、环境探测、manifest 校验、受限安装/更新命令、中文/英文指南索引 | 自动发布、后台改写全局 NPM、运行时存储用户数据 |
| `SOURCE_MANIFEST.json` | 已有源码包文件和 SHA-256 完整性 | 跨平台发布制品、数据库兼容或升级资格 |
| `RELEASE_MANIFEST.json` | 同一版本的制品、来源、签名、兼容和回退声明 | 用户密钥、机器绝对路径、可变运行状态 |
| 已选运行制品 | OCI 镜像、原生二进制或 Desktop 安装包 | 其他形态专属文件 |
| 安装记录/数据目录 | 本机状态、持久数据、操作 journal、备份 | 公共 release registry |

`RELEASE_MANIFEST.json` 必须是不可变发布资产，由安装器使用内置/已确认的信任根校验。建议字段：

```json
{
  "schema_version": 1,
  "product": {"name": "My API", "version": "x.y.z", "source_sha": "...", "build_revision": "..."},
  "bootstrap": {"min_version": "...", "max_version": "..."},
  "compatibility": {
    "config_schema": "...",
    "data_schema": "...",
    "supported_from": ["..."],
    "rollback": "code_only|database_restore|required_manual"
  },
  "artifacts": [
    {
      "id": "...",
      "edition": "full|lite",
      "installation_shape": "server-native|server-container|personal-native|personal-container|desktop",
      "os": "linux|darwin|win32",
      "arch": "amd64|arm64",
      "kind": "oci|binary|desktop",
      "location": "immutable release URL or OCI digest",
      "sha256_or_digest": "...",
      "signature": "...",
      "sbom_or_provenance": "..."
    }
  ]
}
```

生成 manifest 的聚合阶段必须核验版本、`VERSION`、NPM package、源码 SHA、平台集合、hash/digest、签名与桌面/OCI/原生制品一致。缺失、错 SHA、错误 arch、签名失败或最低安装器不满足时拒绝，不可 fallback 到 `latest`、其他 edition、未知源或其他凭据。现有 NPM provenance、OCI cosign、原生签名、macOS notarization、Windows Authenticode 和 Linux 包签名的正式门槛需单独决定。

本文件中的 `myapi install`、`myapi switch`、`myapi rollback` 等为未来命令合同，不是当前已可用的 CLI 命令；用户文档只能在实际实现与平台验收后给出可复制命令。

## 4. 安装记录、数据目录与清理

每个受管安装保存非秘密的安装记录，例如 `<managed-root>/state/installation.json`：解析后的 edition/shape/access、当前/上一个制品、版本、hash/digest、config/data schema、稳定数据目录、操作 ID 和最后结果。敏感配置仍存于最小权限的配置文件或平台安全机制，不能进入 manifest、日志或 UI 响应。

逻辑目录如下：

```text
<managed-root>/
  releases/<version>/<artifact-id>/  # 安装器拥有的不可变运行文件
  state/                             # installation、journal、locks
  staging/  cache/                   # 下载/解压的任务自有暂存
  config/  data/  logs/              # 持久配置、数据库/上传、日志
  backups/
    install/ config/ database/ prompt-learning/ codex-apply/
```

Linux 系统服务、Linux 用户服务、Docker 项目和 Desktop 需要各自的实际根路径合同：系统服务可使用受控 `/opt`、`/etc`、`/var/lib`、`/var/log`、`/var/cache` 布局；个人原生安装使用 XDG 对应目录；Desktop 保持平台 `userData`；容器继续使用受控项目目录和显式数据卷。确切路径、服务模型和权限必须在 P0 决策后才写入安装器，绝不通过扫描 Home 目录推测。

安装器维护包含版本、hash、归属根路径的 `owned-files.json`。健康检查并提交成功后，清理只能删除：本次下载/解压的暂存、未选择形态且确认为自有的制品文件、以及已超过保留策略且不再被 current/last-known-good/recovery 引用的运行文件。必须保留：当前制品、至少一个可用回退制品、数据库、配置、日志、上传文件、提示词正文/版本/学习记录、Codex 应用备份、必要恢复材料和用户指定的外部目录。

安装失败不删除原有可用指针或数据。journal 记录失败阶段、保留安全的暂存/备份，并提供继续、回滚或人工处理说明。禁止以“清理旧版本”为名扫描/删除其他软件、共享缓存或未知目录。

## 5. 安装、形态切换与访问向导

统一入口先探测操作系统、CPU 架构、Node/容器/系统服务能力、权限、磁盘、当前安装和可用 manifest artifacts，再展示可选项：Full 服务器、Lite 服务器、个人电脑 Lite、Desktop。没有匹配制品时显示原因和支持路径，不伪装成功或替换到其他 edition。

同机形态切换遵循：

```text
inspect → compatibility check → explicit confirmation → download/verify
→ backup → drain/maintenance → switch → health/business verification
→ commit → owned-file cleanup
```

切换前展示当前/目标形态、软件版本、功能差异、数据引擎、活动订单/订阅/任务/worker、权限与磁盘风险。Full → Lite 只在 manifest 声明当前数据库和数据 schema 兼容时允许；不支持的高级功能数据必须保留并显示影响，正在进行的工作必须排空、恢复或阻止切换，不能通过删除数据完成降级。数据库引擎迁移与跨机器迁移是单独的导出/恢复工作流，不能静默混入形态切换。

本机、LAN 和公网是单独操作。公网向导必须区分已验证可用、部分条件仍需配置、当前不支持和无法自动判断；检查监听、端口、权限、防火墙、NAT/CGNAT、IPv4/IPv6、域名/DNS/HTTPS、反向代理或经用户选择的隧道。外部验证仅针对用户选择的服务地址；本机健康检查不能充当公网证明。用户自行选择第三方服务、账号、费用和外部端口映射，安装器不代为决定或修改。

个人电脑公网向导还必须解释开机、休眠、合盖、断网、退出后台服务、上行带宽、动态地址、资源/额度消耗和恢复条件；提供 Windows、macOS、以及最终声明支持的 Linux 个人电脑教程。关闭公网只撤销本功能管理的入口，保留数据、Key、无关配置，并对用户手工外部设置给出撤销说明。

## 6. 更新、回退与并发恢复

检查、下载、安装是分离操作。自动检查、自动下载、自动安装均默认关闭；设置分别包含检查周期、维护时间窗口/时区、稳定/补丁范围、通知与失败恢复。勾选检查不能下载、重启或停止服务，下载不能安装，安装只能在用户允许的维护窗口及适用权限下执行。

所有方式共用持久状态机：

```text
idle → planned → preflighted → artifact_verified → staged → backed_up
→ draining → migration_started → service_started → health_checked → committed
                    ↘ rollback_pending | outcome_unknown | needs_manual
```

升级前检查目标版本、说明、来源/签名、磁盘、权限、安装器兼容、配置/数据 schema、备份、当前运行任务和访问模式。下载/验证尽量不影响旧版本；安装前明确短暂停机。请求、异步 Task、账务 outbox、提示词分析任务按其自身恢复合同排空/暂停/恢复。多个 UI、CLI 或自动任务通过同一安装锁串行；崩溃/重启后先比对 journal、实际制品、schema 和健康状态，再决定继续、回滚或人工处理，不能盲目重试迁移。

程序回退、数据恢复和 rollout-forward 必须分开显示。目标 schema 不能被旧版本安全读取时，旧程序回退不等于恢复成功；manifest 必须要求数据库恢复或标为人工处理。升级后保留原访问模式和显式授权，不自动开放公网、匿名、宿主文件访问或 Codex 权限。

S5-P 数据是持久数据：更新/切换备份必须覆盖其版本、样本、任务、水位、审阅资料、授权审计与 Codex 应用备份。更新开始时停止新的模型提交；已发送请求按真实结果处理，不能假称撤销或免除费用。

## 7. 形态特定职责

| 形态 | 更新机制 | 权限与数据边界 |
| --- | --- | --- |
| NPM bootstrap | 用户显式通过包管理器升级，或使用带版本的 `npx` | 应用不能改写全局 NPM；bootstrap 只管理自身和受控安装根 |
| 原生服务器/个人电脑 | 受限本地安装管理器原子切换运行版本，可选 user/system service | 固定 data/config/log path；需系统权限时说明且等待用户操作 |
| Docker | 受控 Compose 项目按 manifest digest 切换，保留卷与配置 | 不执行 `down -v`；不能接管未知 Compose 项目或卷 |
| Desktop | 平台签名/公证的应用更新与 Lite 后台服务协调 | 保持 `userData`；窗口关闭、退出应用、停止服务分别定义 |

当前 Electron `publish:null`、子进程启动模型和 macOS/Windows-only CI 不满足自动更新、后台服务恢复或 Linux Desktop 正式支持的目标；实现前不得宣传已支持。远程浏览器只能看到安装状态；要执行主机操作必须经过本机/服务器受限管理桥、强身份验证和明确操作者确认，不能成为任意 shell 或任意文件写入通道。

## 8. 教程、界面与验收

S6 增加“安装与更新”入口，显示功能版、安装形态、访问模式、运行状态、软件/制品版本、数据位置、当前/目标更新、下一次自动检查、切换/更新历史、备份与回退。安装向导可选中文/英文；运行 UI 仍须覆盖七种项目语言。Desktop 必须说明窗口关闭是否后台运行、退出/停止服务的差异、更新是否停止服务、更新后是否恢复原运行状态，以及离线/休眠/关机后的补做检查。

正式中文、英文教程必须按真实实现覆盖：适用环境、依赖检查、明确系统/目录/权限下的命令、形态和数据位置选择、管理员/渠道/Key 初始化、启动与健康验证、本机/LAN/公网配置、停止/重启/切换/更新/备份恢复和故障处理。未发布的包或未实现命令必须标记为设计，不得伪装为可复制安装步骤。

验收至少包括：

- 同一入口选择全部支持形态，平台/架构不支持时准确拒绝；
- manifest 与 NPM、OCI、原生、Desktop 的版本、来源、SHA/digest、签名一致，错误/缺失时 fail closed；
- 健康后仅清理自有暂存/未选制品，持久数据、提示词学习资料和回退材料保留；
- 安装、切换、更新在磁盘不足、权限不足、下载中断、并发点击、崩溃/重启、外部配置修改、健康假阳性和 schema 不兼容时有可执行恢复路径；
- Full/Lite/Desktop 适用切换保留账号、Key、渠道、权限、账务、日志和 S5-P 数据；数据库迁移另行验证 SQLite、MySQL、PostgreSQL；
- 自动更新默认关闭，检查/下载/安装/维护窗口各自生效；更新不改变访问范围；
- 旧 LAN 保持 local/LAN；公网检测不会把本机可达误报为公网；
- Linux/macOS/Windows 的实际声明、签名状态、安装包、更新和真实设备网络证据一致；未验证项如实保留。

本机只运行单核/768MiB 的串行定向验证。完整安装矩阵、数据库恢复、浏览器、Docker 和打包优先 GitHub Actions；真实网络/设备/发布另行授权。

## 9. 待负责人决定

1. `@forcemind/myapi` 是否演进为轻量 control-plane CLI，同时保留源码初始化能力；建议是。
2. Lite OCI 未来是否保留 `myapi-lan` 作为长期坐标、引入新 Lite 坐标并保留 alias，或改为同镜像能力切换；在决定前不得改坐标。
3. Release Manifest 的权威托管、信任根与正式签名/公证门槛；建议使用不可变发布资产和独立签名校验。
4. 原生服务器/个人电脑的标准路径、system/user service 支持范围、最低可升级版本、RPO/RTO 与数据库 downgrade 规则。
5. Desktop 首发平台/架构、Linux Desktop 是否纳入正式首发，以及后台服务/更新器实现模型。
6. 自动更新允许的更新范围和默认策略；建议所有自动动作关闭，仅允许用户明确开启的稳定/补丁策略。

### 9.1 D11 决策材料（建议待确认）

以下组合是最小可实施建议，不是已批准的发行或恢复政策：

| 维度 | 推荐 | 关键合同 | 仍需负责人确认 |
| --- | --- | --- | --- |
| 信任根（T1） | GitHub OIDC/Sigstore keyless | bootstrap 对 manifest 的精确原始 bytes/bundle 验签，并固定 issuer、仓库、workflow path 和目标 tag identity；OCI 继续用 digest+cosign，NPM provenance 只证明 NPM 包 | 权威 manifest 位置、允许的 identity/轮换、离线 bundle/透明日志异常策略；macOS 公证、Windows Authenticode 不能由 manifest 签名替代 |
| 原生目录（P1） | Linux system/user 双配置 | system 使用受控 `/opt`、`/etc`、`/var/lib`、`/var/log`、`/var/cache`；个人原生使用已解析并记录的 XDG 目录；Desktop `userData`、容器卷和 Legacy 路径均不自动接管 | 具体 service 帐号/权限、首发 OS/arch、Legacy adopt 流程和最低可升级版本 |
| 恢复目标（R1） | 单节点最低合同 | 升级前一致性备份 RPO=0；已启用并验证的每日外部备份目标 RPO≤24h、参考数据规模/机器上的 RTO≤4h；至少 7 日+4 周保留并定期恢复演练 | 数据规模、备份目的地、SQLite/MySQL/PostgreSQL 的一致性方式、S5-P 资料范围及可接受 downgrade/roll-forward 规则 |

不论采用哪个组合，验证顺序固定为：先对原始 bytes 的签名/身份做验证，再做 schema 解析和制品 hash/digest 校验；安装记录必须保存 manifest digest、策略版本、验证身份和时间。D11 前只允许未受信输入、路径、恢复 evidence 的纯合同和合成测试，不得把任意 fixture 标成真实 `verified`，也不得接线网络获取、实际安装、system service、文件替换、删除、数据库备份/恢复或正式教程命令。
