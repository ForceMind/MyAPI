# MyAPI 第二阶段计划

## 当前执行顺序（2026-11-20 已批准）

维护者已批准以下八批在 `dev` 连续实施。第一批让 Linux 管理员在服务运行时看清能否升级、原因是什么、确认后会动哪些内容。同时核对普通用户从连接上游、调权重、发 Key 到查看请求的操作路径。生产更新独立确认，实施证据沿用 [TASKS.md](TASKS.md)。

### 从哪里接着做

| 对象 | 可以复用的证据 | 仍缺的证据或功能 |
| --- | --- | --- |
| beta.57 | 应用源码 `11619874` 的 CI 有 12 个作业成功、10 个可选作业跳过。Full / Linux amd64 / SQLite 的临时部署通过 20/20 项检查。[源码 CI](https://github.com/ForceMind/MyAPI/actions/runs/43236103891) · [打包与验收](https://github.com/ForceMind/MyAPI/actions/runs/43251245890) | 11 月 20 日回读该打包运行，只列出 Docker 构建记录。原 Full 镜像包和报告已过 11 月 19 日保留期。线上版本和切换结果以服务器任务的实际回读为准。 |
| Linux 原生 | 第 1 批 `8753e669` 已验收运行中只读预检。第 2 批 `d691677b` 已验收系统 Full、系统 Lite 与个人 Lite 的确认升级、停服后备份和账务回读，完整 CI 17 个作业通过。[预检 CI](https://github.com/ForceMind/MyAPI/actions/runs/44785958630) · [升级完整 CI](https://github.com/ForceMind/MyAPI/actions/runs/44811406783) | 当前验收第 3 批程序回退，第 4 批交付保数据卸载。两份正式 MyAPI 发行签名仍待发行提供。[原生入口](../../../deploy/install-native.sh) · [迁移合同](../../RELEASE_MANIFEST.md#6-更新回退与并发恢复) |
| Full、Lite、Desktop | Full/Lite 共用网关，Desktop 封装 Lite。已有三版本首页效果图、329 个线稿视图和 658 张桌面/窄屏 SVG。渠道路由编辑、Key 交付、正文/SSE 分段阅读已有非浏览器回归。beta.57 增加了三个入口的逐窗口重置时间。[线稿与路由索引](ui-design-v2/wireframes/README.md) · [任务证据](TASKS.md) | 实际窄屏、明暗主题下的遮挡和滚动仍待视觉验收。Desktop 已有 macOS、Windows 的候选构建和受控后端检查，Windows 停服时待写数据是否落盘、两平台重启后的账务仍待实测。[Desktop 专项](https://github.com/ForceMind/MyAPI/actions/runs/42485203085) |
| 运维与文案 | Root Docker Full/Lite + SQLite 的备份、隔离恢复和保留策略有过真实往返。B6 已评估旧卷权限，继续使用现有默认用户。B7 五篇教程的 24 条原命令和 C1 七语键与占位符已有证据。[备份专项](https://github.com/ForceMind/MyAPI/actions/runs/42465372403) · [教程与语言清单](https://github.com/ForceMind/MyAPI/actions/runs/42487543625) | 随新增原生命令补教程。动态文案和母语审读分别留证。A1–A6 已审结的项目沿用任务清单，未知清理候选继续保留。 |

### 可逐批交付的功能

每一行各自交付和验收。Linux 的前三个批次依次进行，保数据卸载可在原生服务归属和停止条件验证后实施。Web 的用户路径不依赖 Linux 升级，Desktop 复用现有 Lite 结果。

| 顺序 | 用户能做什么 | GitHub Actions 验收条件 |
| --- | --- | --- |
| 1 · B3d 升级前只读预检 | 管理员选择本地目标包，看到当前和目标版本、服务归属、SQLite 位置、是否可升级、需要停服的步骤。拒绝时指出应核对的文件或配置。沿用 B3b 包核验、B3c 安装记录和 B3d 选择器。 | 在一次性 system/user 服务中分别验证可升级及被拒情况。两份 Manifest 的签名、已安装和目标二进制、版本方向、数据库类型和 schema 均须匹配。覆盖旧安装记录、包不符、服务或数据库身份不明。预检前后服务持续运行，配置和持久数据保持原样。 |
| 2 · B3d 确认升级 | 管理员确认刚才看到的版本和停机影响。执行时再次读取现场，排空后备份最新 SQLite 及存在的 WAL，核实备份，再切换程序、启动并读回状态。 | 在已支持的系统 Full、系统 Lite、用户 Lite 上记录实测范围。核对实际程序身份、状态接口、原配置、开关和历史账务。预检后现场变化即停止，重复确认只执行一次。中断后先读实际进度，结果不明保留现场。 |
| 3 · B3d 回退程序 | 升级后需要退回时，管理员看到允许返回的具体版本和数据影响。仅对签名声明允许 `code_only` 且 schema 相容的版本提供程序回退。 | 在新版上先产生一笔明确的合成账务，再回退程序。旧程序能读到原历史和这笔新增账务，假上游调用数无新增。缺回退声明、身份不符或 schema 不兼容时，在切换前拒绝。 |
| 4 · B3 保数据卸载 | 系统或个人用户先看到要停止的服务、要移除的程序和要保留的数据位置，确认后执行。 | 分别验证 system/user。只处理安装记录能证明归属的服务与程序，保留配置、数据库、日志和备份。遇到同名外部服务、归属冲突或停止未确认时保留现场。 |
| 5 · 三版本核心体验 | Full 从连接进入选路、发 Key 和查请求。Lite 从接上游、取地址走到配置客户端。Desktop 从服务面板进入同一套 Lite 流程。具体页面和操作见下表。 | 复用已有测试，仅为缺少证据的任务补组件或接口回归，使用保存记录与合成响应。三版本按角色核对权限、确认和返回位置。把需要观察尺寸、滚动和遮挡的项目留给视觉验收。 |
| 6 · B5 桌面运行可靠性 | 用户能区分关窗留托盘、停止后台服务、退出应用，重新打开后还能查看刚才的配置和记录。 | 先 macOS，再 Windows。用真实 Lite 后端和隔离数据目录验证正常停止、重启与合成账务保全，Windows 须有待写数据实际落盘的证据。记录本机、局域网与外部可达性各自的证据，真实 GUI、外部视点及签名公证单列。 |
| 7 · B7 / C1 教程与文案 | 新用户能选对 Docker Full、Docker Lite、原生系统服务、个人安装或 Desktop 教程，知道下一步做什么、失败后去哪里查看结果。 | 原生命令每交付一项，同步原教程中对应的中英文步骤，修正其中“还不能安装”的历史说明；在 Actions 实际执行命令并核对服务、文件或退出状态。新增 Web 文案先齐 en/zh，其余五语先英文回填，最终再做七语动态文案与人工审读。 |
| 8 · C2 / C3 统一审查与候选交付 | 维护者按版本看到“可用、受限、待验”的功能清单、对应安装包和恢复办法。 | 功能批次结束后集中独立审查，依影响修复并回归。在同一候选源码上核对 Go、独立 relaykit、三库、前端、Full/Lite Docker、原生服务和 macOS→Windows 候选包。记录实际作业、SHA、制品摘要与期限；缺产品签名、GUI、浏览器或外部验证的项目明确列出。 |

**第一批拟涉及的文件。** 命令入口在 `deploy/install-native.sh`。复用 `cli/lib/native-package-verification.mjs` 的签名和包校验、`cli/lib/release-manifest.mjs` 的 `selectNativeReleaseTransition`，并按 `cli/lib/native-install.mjs` 中的实际安装记录核对现场。回归就近放入 `cli/test/` 和 `tools/runtime/native-install-qualification.mjs`，使用现有 `ci.yml` 的 Linux 临时服务作业。只为实际交付的命令更新 [原生教程](../../NATIVE_LINUX.md)。首批的主要风险是把另一实例或继承的外部数据库配置认作当前 SQLite，遇到身份无法确定时给出原因并结束预检。

### 核心用户路径的逐项验收

| 页面与操作 | 普通用户的完成条件 |
| --- | --- |
| 连接卡片、渠道与编辑抽屉 | 模型默认显示数量，点击可读完整列表。获取模型只使用所选渠道的返回，读取失败时保留原选择。权重和优先级从当前布局可达，保存后能读回，切换对象不会把草稿写给另一连接。 |
| 连接详情、账号窗口、用量 | 没有上游余额查询接口时，明确显示“不支持主动查询”，仍可查看已有观测。每个窗口单独显示来源、观察时间和保存的重置时间。过期标“上次记录”，缺失标“未知”。只有同账户、同窗口、同口径的剩余额度观测才用于估算变化，显示依据。缺失、null、0 与正数分别保留。 |
| Key、客户端与请求详情 | 创建或选定的 Key、地址和模型能连续交给客户端配置。权限和敏感确认完整。用户可按单日历范围筛选，定位同一请求，分页或跳转阅读已存正文/SSE，并回到原筛选、列表位置及焦点。写入结果不明时提供查询结果的入口。 |
| Desktop 面板与 Web 返回路径 | 首屏能判断服务是否运行、打开哪个地址、下一步去哪里。收起次要详情后，状态、失败原因和常用动作仍可达。已保存的公网地址只显示已取得的检查结果。 |

沿用方向 3「流向图」。先用已有用例和设计索引核对上表，再处理能指出具体入口和复现条件的差距。需要改变布局时，先给出该视图的效果图，再增量修订其桌面和窄屏线稿，再实施。固定列边缘文字、渠道抽屉对齐、安装向导滚动后的品牌是否完整可达，列入将来的中英、明暗、窄宽屏 Actions 浏览器验收；目前标记为“浏览器未验证”。[现有效果图和线稿](ui-design-v2/wireframes/README.md)

### 怎样连续推进

- 确认开发范围后，每次从上表取一个能独立使用的交付，在 `dev` 定向修改并只提交本批路径，推至 `origin/dev`。读回同一提交的完整非浏览器 CI 和所需专项，在 [任务清单](TASKS.md) 记录用户入口、源码 SHA、CI 与剩余限制。通过后直接进入下一个已批准批次。纯文档批次沿用应用版本。
- 本批验收被阻塞时，保留具体输入和失败证据，先修直接阻塞本批的原因。需要扩大产品或写入范围时提请确认，其他已批准且无依赖的工作可继续。全面找缺陷安排在 C2。
- 所有构建、测试和打包使用 GitHub Actions 的临时环境、合成数据和自有假上游。浏览器、真实供应商、生产、签名发行、合入 `main` 和发布各按对应的显式授权处理。自动更新保持关闭。
- **beta.57 生产候选单列。** 若仍选择部署该版，先在原源码 `11619874` 上恢复可下载的 Full 包与报告，重新核对 run/attempt、两份构件摘要、镜像配置及 OCI 清单身份。将新身份交给服务器任务，复核空间和当时的配置、开关及最新账务基线，提交最小切换方案。维护者确认该次方案后才进入生产。开发批次可以独立推进。[原打包运行](https://github.com/ForceMind/MyAPI/actions/runs/43251245890)

## 2026-10-25 原计划与决策依据

下文保留立项时的清单、分工和预期验证方法。五项决定已获确认，完成情况以本页上方、任务清单和各自提交的 CI 为准；构建、测试与浏览器授权以当前约定为准。

维护者要求（原话）："整个代码的结构框架看过了吗"，随后："可以你列好计划，我们一步一步实现，还有部署脚本也要完善"。

## 一、立项时的起点

### 1.1 已完成（均已推送，CI 通过）

- 评估与精简：删除 74 个未引用的前端文件、158 个不可达的 Go 函数（约 1.2 万行）。
- 版本门禁：后端识别 `lite`，`/api/status` 新增 `product_edition` 与 `capabilities`；未知版本值拒绝启动；Lite 下补封兑换码、io.net 部署、Midjourney，只放开 `GET /api/subscription/self`；日志新增 `result=failed`。
- UI 方向 3「流向图」第 1–3 步：令牌与顶部导航外壳、首页流向板、Key 页与请求页。第 4 步（上游页）进行中。

### 1.2 结构审查的覆盖缺口（`architecture-review.md` 第 8 节，主会话未补）

1. 账务子系统与异步任务恢复（约 3.7 万行）只读到文件级。
2. 支付回调在 Lite 下对未完成订单的处理未核对。
3. 只用外部登录、没有密码的账户在 Lite 下能否登录未核对。
4. `tools/`（约 4.2 万行验收脚本）、`website/`、`deploy/` 未评估。
5. 790 个单调用点函数、637 个疑似未用的翻译键未逐个复核。
6. 三种数据库的实际迁移未运行验证。

### 1.3 部署资产现状（主会话读过文件；标注依据）

| 项 | 现状 | 依据 |
| --- | --- | --- |
| 版本值贯通 | `Dockerfile` 只在 `MYAPI_EDITION = lan` 时设置精简构建；`cli/myapi.mjs` 只接受 `full` 与 `lan`；`deploy/install.sh` 同样只接受这两个值。后端已识别 `lite`，部署侧未跟上：用 `lite` 构建镜像会得到非精简前端 | 读了 `Dockerfile`、`cli/myapi.mjs` 帮助与校验；`install.sh` 来自子代理核对，主会话未读全文 |
| `deploy/.env.example` | 开头注释仍写"beta.25 是未发布的平台 UI 候选"，而默认镜像已是 beta.26 | 读了文件 |
| 必填变量 | Compose 强制 `SESSION_SECRET`、`MYAPI_PUBLIC_URL`、`MYAPI_ACCOUNTING_CONFIG_VERSION`、`BATCH_UPDATE_ENABLED`，对全新个人安装门槛高；`myapi lan init` 已生成密钥，但 Full 的 `init` 流程与 Lite 的差异未梳理 | 读了 `deploy/docker-compose.yml`、`.env.example` |
| `my-api.service` | 每行末尾带 `# 注意修改…` 的行内注释。从 systemd 语法规则看，行内注释不被支持，会被当成值的一部分；未在本机用 `systemd-analyze` 验证（本机是 macOS） | 读了文件；规则来自对 systemd 语法的了解，标注"未实测" |
| 容器用户 | 镜像没有 `USER` 指令，进程以 root 运行 | 读了 `Dockerfile` |
| 原生 Linux 安装 | 没有安装、升级、备份、回滚脚本，只有一个模板单元；`RELEASE_MANIFEST.md` 第 9 节的路径与服务模型决定未做 | `ls` 与文档 |
| 备份与恢复 | 只有 CI 用的 `tools/runtime/sqlite-restore-smoke`，没有面向用户的 `backup` / `restore` 命令 | `cli/myapi.mjs` 帮助文本 |
| Desktop（Electron） | 托盘文案中英混写且未接 i18n；`publish: null` 无更新器；CI 只构建 macOS 与 Windows；已改传 `MYAPI_EDITION: 'lite'` | 读了 `electron/` 与第一阶段报告 |
| Lite 镜像坐标 | 仍是 `ghcr.io/forcemind/myapi-lan`，长期坐标待决定 | `MYAPI_DISTRIBUTION.md` |

## 二、已确认的五项决定（保留立项时的推荐与理由）

| # | 决定 | 推荐 | 理由 |
| --- | --- | --- | --- |
| 1 | Lite 的 OCI 长期坐标 | 本阶段保持 `myapi-lan` 不变，不新建坐标，只让部署侧接受 `lite` 作为 `lan` 的别名 | 发布本来就需要单独授权，改坐标会影响已有部署，没有收益 |
| 2 | Release Manifest 的信任根与签名 | 采用文档推荐的 GitHub OIDC / Sigstore keyless；本阶段只写合同与合成测试，不接线下载、验签、安装 | 文档规定决定前不得接线 |
| 3 | 原生安装路径与服务模型 | 系统安装 `/opt/my-api`、`/etc/my-api`、`/var/lib/my-api`、`/var/log/my-api`，由专用系统用户运行；个人电脑用 XDG 目录；Desktop 用平台 `userData` | 文档 9.1 的推荐组合 |
| 4 | Desktop 首发平台 | 先 macOS，再 Windows；Linux Desktop 不纳入首发，不宣称支持 | 与现有 CI 覆盖一致 |
| 5 | 自动更新 | 全部默认关闭；检查、下载、安装各自独立开关；本阶段不实现更新器 | 文档要求 |

## 三、实施步骤

每一步：先读相关代码和文档，写出成功标准与验证方法；用子代理实现；主会话审读并亲自重跑关键命令；通过后提交并推送；读该提交的 CI 结果。每步只处理一个主题。

### A. 结构审查与重构

| 步骤 | 内容 | 角色与模型 | 验证 |
| --- | --- | --- | --- |
| A1 | 补全结构审查（只读）：逐文件读账务与任务恢复（`model/` 账务、`task_recovery`、`service/` 相关），补 1.2 第 1–4 项；输出 `architecture-review-v2.md`，每个结论给出文件与行号依据，区分"读过"和"未读" | 架构评估，Opus | 主会话抽查 10 条以上结论的原文 |
| A2 | 降级影响修复（依 A1 结论）：Lite 下未完成的支付订单、仅外部登录账户的处理，例如在切换前给出阻止或提示 | 核心实现，Opus（触及计费与登录） | Go 测试、CI 数据库作业 |
| A3 | 计费相邻的 37 个不可达函数逐项审查，确认后删除 | 方案审查 Fable，实现 Sonnet | `go build`、`go vet`、相关包测试 |
| A4 | 分层整理：把 `controller/channel-billing.go` 里的额度历史聚合（约 1.2k 行）等业务逻辑下沉到 `service/`，行为不变 | 核心实现，Sonnet | 受影响包测试，CI |
| A5 | 内联单调用点短函数（≤15 行，约 396 个）：按包分批，每包一个提交，保留有业务含义命名的函数 | 核心实现，Sonnet | 每包 `go test` |
| A6 | 清理前端未用导出（140 + 68）、未用 UI 原语（约 1,448 行）、疑似未用翻译键（637，先用 i18n 工具复核） | 核心实现，Sonnet | `typecheck`、`lint`、全量测试、`build` |

### B. 部署脚本完善

| 步骤 | 内容 | 角色与模型 | 验证 |
| --- | --- | --- | --- |
| B1 | 版本值贯通：`Dockerfile`、`cli/myapi.mjs`、`deploy/install.sh`、`tools/lan/check.mjs`、`docker-build.yml`、`docker-smoke.yml` 接受 `lite`（`lan` 为别名，镜像坐标不变）；构建参数 `lite` 与 `lan` 产出相同的精简前端 | 核心实现，Sonnet | `node --test cli/test/*.test.mjs`、CI 的 Docker smoke |
| B2 | 配置与引导：刷新 `deploy/.env.example` 的陈旧注释；让 `myapi init`（Full 与 Lite 两条路径）一步生成 `SESSION_SECRET`、合理默认并写明必填项；`myapi doctor` 增加版本值、端口占用、目录权限、磁盘空间、Docker 与 Compose 版本检查 | 核心实现，Sonnet | CLI 单测，CI |
| B3 | Linux 原生服务：修正 `my-api.service`（去掉行内注释）、加固选项（如 `NoNewPrivileges`、`ProtectSystem`），按推荐路径提供 `deploy/install-native.sh`（安装、升级、回滚、卸载，不删除数据），并给出个人用户的 user service 版本 | 核心实现，Opus（涉及系统权限与数据保护） | `shellcheck`；Linux 容器内用 `systemd-analyze verify`（若 CI 可用）；本机 macOS 无法验证的部分明确标注 |
| B4 | 备份与恢复：`myapi backup`（SQLite 一致性备份、保留策略）、`myapi restore`（先校验再恢复，不覆盖未确认的目录）；复用 `tools/runtime/sqlite-restore-smoke` | 核心实现，Opus（涉及数据） | 备份恢复往返测试，CI |
| B5 | Desktop（Electron）：托盘文案接 i18n（中英文）；窗口关闭、退出应用、停止后台服务三者区分；本机访问为默认，局域网与公网分别开启；构建脚本整理 | 核心实现，Sonnet | `node --test electron/test/*.test.mjs`，CI 的 Desktop 合同作业 |
| B6 | 容器加固评估：评估非 root 运行（含数据卷权限迁移风险）与镜像体积，仅在确认不破坏已有数据卷时实施，否则只写入文档 | 方案 Fable，实现 Opus | CI 的 Docker smoke，升级往返测试 |
| B7 | 部署教程（中英文）：Docker Full、Docker Lite、Linux 原生、个人电脑、Desktop 各一篇，按真实实现写，未发布的包与命令标注为设计，不伪装为可复制步骤 | 核心实现，Sonnet；独立审查 Fable | 教程里的每条命令在 CI 或本机实际执行一次 |

### C. 收口

| 步骤 | 内容 |
| --- | --- |
| C1 | UI 第 5 步（其余页面）完成后，做七语言补译与验收（当前只要求中英文）。 |
| C2 | 独立质量审查（Fable，只审查），按审查结果修复并回归。 |
| C3 | 全量验收：`go test ./...`、relaykit 独立构建、前端全部测试与构建、本机与 CI 全部浏览器脚本、三库迁移、Docker smoke。 |

### 立项时的建议顺序

UI 第 4 步（进行中）→ B1、B2（小而独立，且是已知缺口）→ UI 第 5 步 → A1 → A2/A3 → A4–A6 → B3、B4 → B5、B6 → B7 → C1–C3。

部署侧与前端 UI 的文件不重叠，B1、B2 可以与 UI 步骤并行，我会在开工前说明。

## 四、边界

- 不合入 `main`；不发布 NPM、GHCR、tag；不部署生产；不修改生产配置；不调用真实供应商。这些仍需单独授权。
- 数据库表与字段不删除；三库兼容；`relaykit` 保持独立可构建；账务安全不变量不放宽。
- 本机是 macOS，无法验证 Linux 与 systemd 行为的部分，由 CI 或 Linux 容器验证，不能验证的明确标注。
- 每一步完成并通过验证后才提交推送，推送后读 CI 结果，通过才称该步完成。
