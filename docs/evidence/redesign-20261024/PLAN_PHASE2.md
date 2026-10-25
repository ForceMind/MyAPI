# 第二阶段计划：结构审查与重构、部署脚本完善（2026-10-25）

状态：待维护者确认。确认前不开工。

维护者要求（原话）："整个代码的结构框架看过了吗"，随后："可以你列好计划，我们一步一步实现，还有部署脚本也要完善"。

## 一、起点

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

## 二、待维护者确认的五项决定（`RELEASE_MANIFEST.md` 第 9 节），附推荐

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

### 建议顺序

UI 第 4 步（进行中）→ B1、B2（小而独立，且是已知缺口）→ UI 第 5 步 → A1 → A2/A3 → A4–A6 → B3、B4 → B5、B6 → B7 → C1–C3。

部署侧与前端 UI 的文件不重叠，B1、B2 可以与 UI 步骤并行，我会在开工前说明。

## 四、边界

- 不合入 `main`；不发布 NPM、GHCR、tag；不部署生产；不修改生产配置；不调用真实供应商。这些仍需单独授权。
- 数据库表与字段不删除；三库兼容；`relaykit` 保持独立可构建；账务安全不变量不放宽。
- 本机是 macOS，无法验证 Linux 与 systemd 行为的部分，由 CI 或 Linux 容器验证，不能验证的明确标注。
- 每一步完成并通过验证后才提交推送，推送后读 CI 结果，通过才称该步完成。
