# My API 升级、切换与恢复演练

> 当前状态：本页前半部分记录当前 Legacy Docker 升级事实；统一 Release Manifest、原生/Desktop 更新、产品形态切换、持久 journal 和数据库安全回退仍未实现。现有 schema-1 纯结构 selector 不读取受信资产、不安装或更新，参见 [发行制品、安装与更新合同](RELEASE_MANIFEST.md)。

当前 `myapi upgrade` 是显式 Legacy Docker 操作：它会校验版本、备份 `deploy/.env`、拉取固定 GHCR 镜像、等待健康检查，并在失败时恢复环境文件和旧镜像。它不会替管理员猜测数据库类型，也不会自动复制生产数据库；“旧镜像已恢复”不等于数据库已安全回退。

当前 GHCR 发行须由维护者对已有 SemVer tag 显式发起 `workflow_dispatch`，并通过 `PUBLISH`、专用环境和
GHCR gate；推送 tag 不会自动发布。Full 与 Legacy LAN 分别使用
`ghcr.io/forcemind/myapi` 和 `ghcr.io/forcemind/myapi-lan`。预发布（如 `v0.2.0-beta.1`）只写不可变
version/arch tag，不移动稳定 latest；稳定 latest 仅在版本单调、Full/LAN+arch digest、OCI
provenance/SBOM 与 Cosign 预检通过后提升。`myapi up`
和 `myapi upgrade` 只使用版本固定的 GHCR tag（除非明确选择本地构建），不会把
普通分支提交或可变 `latest` 当成升级目标。历史 `v0.1.0` 与 `v0.1.1` tag 已锁定，
不得重用或手动重跑发布。

> 当前 `@forcemind/myapi` 尚未正式发布到 NPM。以下命令中的 `npx
> @forcemind/myapi` 在正式发行前请替换为源码检出的
> `node cli/myapi.mjs`，或使用已经由维护者审核的本地 tarball；不要让
> `npx` 从未知的公共包解析同名命令。

源码发行合同可在没有 Docker、GHCR 或部署凭据的环境中先做只读检查：

```bash
npm run upgrade:check -- --json
```

该检查只确认 CLI 的 dry-run、资源预检、签名闸门、备份权限、健康等待和回滚路径仍与本页及回归测试一致；它不会创建环境文件、读取 `deploy/.env`、访问网络或调用 Docker。它不能替代下面的副本升级和数据库恢复演练。

## 统一更新与形态切换目标合同（尚未实现）

未来所有安装方式共用以下持久、可恢复状态机：

```text
idle → planned → preflighted → artifact_verified → staged → backed_up
→ draining → migration_started → service_started → health_checked → committed
                    ↘ rollback_pending | outcome_unknown | needs_manual
```

检查、下载、安装是独立动作；自动检查、自动下载、自动安装均默认关闭。更新前核验 Release Manifest、来源/签名、OS/arch、安装器/配置/数据 schema、磁盘、权限、备份、运行任务和维护窗口。下载/校验尽量保持旧版本可用；UI、CLI 与自动计划通过同一安装锁串行。机器重启或进程崩溃后必须先读取 journal、实际制品、schema 和健康状态，再决定继续、回退或人工处理，不能盲目再次迁移。

程序文件回退、数据库恢复和 rollout-forward 必须分别显示。形态切换（Full↔Lite、服务器↔个人电脑/Desktop）先报告功能差异、活动订单/订阅/Task/worker、数据兼容和空间；不安全时拒绝，不能删除数据或静默转换数据库。更新和切换不改变 local/LAN/public 访问模式，不改防火墙、路由器、反代、隧道、用户/Key 授权、S5-P 数据或 Codex 指令文件。

S5-P 分析 worker 在维护开始时停止提交新的模型请求；已经发出的请求按成功、失败或未知费用状态恢复。提示词版本、样本、水位、授权审计和 Codex 应用备份属于必须验证的持久备份范围。

## 当前 Legacy Docker 副本演练

1. 使用与生产相同版本的副本目录和脱敏数据，不挂载生产 `data`、`logs` 或数据库卷。
2. 固定 `MYAPI_IMAGE=ghcr.io/forcemind/myapi:<version>`，确认 `MYAPI_EDITION` 与目标发行版一致。
3. 对 SQLite 副本保存数据库文件的校验和；对 PostgreSQL 使用管理员批准的逻辑备份工具，并把备份写入部署目录之外的受控位置。
4. 先运行只读预检，不会写入环境文件、创建备份、调用 Docker 或访问 GHCR：

   ```bash
   node cli/myapi.mjs upgrade \
     --project-dir <副本目录> --version <新版本> --dry-run --json
   ```

   预检会检查发行版本、Full/Legacy LAN 镜像映射、环境文件、会话密钥、URL、端口和资源限制，输出
   目标镜像但不会输出任何密钥或完整环境变量。签名参数在预检中只记录为待执行，不会调用
   `cosign`；正式升级时才会执行签名验证。
5. 预检通过后，才在副本中执行 `myapi upgrade --project-dir <副本目录> --version <新版本>`，确认容器健康、登录、API 请求、日志查询和额度面板均可用。
6. 在副本中停止服务并从备份恢复，再验证用户、Key、渠道配置、日志索引和快照趋势；记录恢复耗时和缺失项。

### SQLite 迁移注意事项

额度快照的 `dedupe_key` 在旧 SQLite 数据库中先作为可空列添加，随后由迁移显式创建
唯一索引。不要手工把该列改成 `UNIQUE` 后再启动服务：SQLite 不支持对已有表执行
`ALTER TABLE ... ADD COLUMN ... UNIQUE`，会导致容器健康检查失败。升级演练应确认日志中
没有 `Cannot add a UNIQUE column`，并在健康检查通过后再切换流量；失败时保留旧镜像和
数据库副本，按上面的回滚步骤处理。

发布 workflow 会在构建前检查版本 tag 和架构 tag 是否已经存在；如果存在就失败，
要求创建新的 SemVer tag，不覆盖已发布镜像。稳定 `latest` 是受单调 promotion 保护的可变入口，预发布不会触碰它，
且两者都不应作为生产升级目标。如需避免版本 tag 在拉取后被重新指向，可在副本升级时增加 `--pin-digest`，或在
`deploy/.env` 设置 `MYAPI_PIN_IMAGE_DIGEST=true`。CLI 会先拉取版本 tag，再读取本机
`RepoDigests`，严格校验 `repo@sha256:<64 hex>` 后把该 digest 写回环境文件；若同时
使用 `--verify-signature`，cosign 会验证最终 digest。解析失败会触发原环境回滚。
该选项不是 dry-run 的一部分，dry-run 不访问 Docker 或 GHCR。

`--dry-run` 是升级前的安全门，不等同于数据库恢复测试。恢复测试必须使用复制出的 SQLite 文件或经批准的
PostgreSQL 逻辑备份；原始生产路径和卷永远不能作为 CLI 自动操作目标。

## 本地镜像构建资源

当需要在工作站验证本地 Docker 镜像时，`deploy/install.sh` 会把
`MYAPI_CPU_LIMIT`、`MYAPI_MEMORY_LIMIT` 同时应用到 `docker build` 的构建容器，
并通过 `MYAPI_BUILD_PARALLELISM`/`GOMAXPROCS` 限制 Go 编译并发。资源参数应保持在
工作站可承受范围内；验证镜像必须使用临时数据目录和非生产端口。若 Docker 仅提供已弃用的
legacy builder，可额外指定 `--cpu-period`、`--cpu-quota`、`--memory` 和
`--memory-swap`；BuildKit/buildx 缺失属于主机工具链问题，不应通过重启生产服务绕过。

## 本机失败回滚测试

CLI 的环境文件和旧部署回滚路径可以在没有 Docker daemon、GHCR 或生产数据的本机上验证。测试会在临时目录
创建一个一次性失败的 `docker` 替身：第一次 `compose pull my-api` 返回失败，随后 CLI 恢复原来的
`deploy/.env` 并重新执行旧部署的 `compose up`。它不会读取当前项目的 `.env`、Docker volume 或凭据。

在源码仓库执行：

```bash
node --test --test-name-pattern="upgrade restores the environment" cli/test/myapi.test.mjs
```

这个测试只证明 CLI 的失败处理和文件权限（环境备份为 `0600`）；它不能替代副本上的真实镜像健康检查或数据库
恢复演练。真实副本升级仍应先执行上面的 `--dry-run`，再按组织批准的备份流程验证数据恢复。

## 生产前检查

- 备份文件不在 Git 工作树、Docker 镜像层或公开制品中，并设置最小权限。
- 备份与目标版本、数据库类型、时区和迁移状态一一对应。
- 新镜像健康检查通过前保留旧镜像和旧数据卷；不要强制移动现有 tag。
- 升级失败时保留 CLI 输出、容器状态和备份校验和，再人工决定是否回滚。

当前 CLI 的自动回滚范围是环境文件和镜像部署状态；数据库备份/恢复仍需按组织的 PostgreSQL/SQLite 运维流程执行。本页是演练清单，不会触发任何生产操作。

## 最近一次本机副本演练

2026-08-31 在临时 SQLite 副本、临时端口和受限资源（1 CPU、768 MiB）上完成：

- `local/new-api:myapi-4b08bdb` 启动并完成 `dedupe_key` 迁移，`/api/status` 返回成功；
- 停止新镜像后恢复原数据库副本，使用旧镜像 `local/new-api:myapi-9dc11d4` 重新启动并通过健康检查；
- 演练容器、端口和临时数据库副本已清理，正式 `new-api` 容器未使用副本数据。

该记录证明当前迁移和镜像回滚路径在本机 SQLite 数据上可行，不替代 macOS/Windows 真实设备验收或正式发布审批。

随后在明确授权下，本机查看部署已切换到当前源码提交 `ff5feb8` 构建的
`local/new-api:myapi-ff5feb8`，其中包含概览 Codex 额度折线图。该次操作仅重建 `new-api` 容器并等待健康检查，继续使用
原有 `/root/new-api/data` 和 `/root/new-api/logs` 绑定目录；没有执行数据库复制、卷删除、
GHCR 拉取或生产发布。
