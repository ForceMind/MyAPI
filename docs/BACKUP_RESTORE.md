# 本地备份与隔离恢复 / Local backup and isolated restore

状态：已登记的 Root Linux amd64 Docker Full 与独立 Lite/lan + SQLite 的备份、隔离恢复和确认保留策略，已在 beta.52 `34b47daf2919e0eb4690eaa440f43dbae70d3afe` / [CI42465372403](https://github.com/ForceMind/MyAPI/actions/runs/42465372403) 通过独立矩阵及完整CI（Full job128683550466 / Lite job128683550558，各18项检查）。beta.47 / CI42442205783 是更早的 Full 保留策略证据。未发布或生产验收，不代表完整 B4 或其它形态已交付。

## 先看操作影响

`myapi backup` 先保存计划，不会立即停止实例。核对计划并确认后，它会关闭所选容器的自动重启、发送 SIGTERM，确认正常退出后才备份。**备份后实例保持停止，自动重启仍关闭。** 命令不会自动恢复服务，也不会强制杀死、重试模型请求或回滚数据库。

`myapi restore` 只恢复到你指定的、尚不存在的新目录。它不会覆盖原目录，不会替换镜像，也不会启动恢复实例。当前要求原实例仍正常停止，且原数据自备份后没有变化；有新增账务或无法确认写入者时拒绝恢复。

操作前必须安排可信的宿主维护窗口，确保没有其他程序或管理员写入数据。`/proc` 扫描是当时可见的停写证据，不能证明恶意 root 或未来写入者不存在。

## 本批支持范围

- Linux amd64 宿主、Node.js 22 或更新、Python 3 标准库、root 权限，以及本机 `/var/run/docker.sock`。
- 已验证范围是注册且各自独立运行的 Docker Full 或 Lite/lan + SQLite 实例；不是 Full→Lite 切换。真实 Env 与不可变镜像标签必须属于同一明确版本族，Full/Lite 交叉、未知或缺失标签继续拒绝。`/data` 必须为明确的本地 bind mount；远端数据库、个人模式或未核对身份的镜像仍拒绝。
- 原数据目录及其祖先必须有可解释的 root 归属。允许 root 的 `0755` 或 `0700` 数据目录，但拒绝组或其他用户可写的目录、非 root 数据条目、链接、特殊文件、子挂载和不支持的文件系统。私有状态、备份及恢复父目录须为 root `0700`；root sticky `/tmp` 下的 root 私有子目录可用。
- 拒绝无法保全的 ACL、扩展属性和缺失的挂载、PID 或文件证据，不自动修改 owner、权限或原数据来绕过检查。

首批单次数据上限为 16 GiB、10,000 个目录与文件条目，并要求额外校验空间；超过限制会拒绝，不截断备份。保留空间检查也可能因已有备份过多而拒绝，本批不会为腾出空间自动删旧数据。

当前只复制注册的完整数据目录，包括实际存在的 SQLite DB、WAL、SHM 及目录内其他文件。外部 `.env`、Docker 配置、其他挂载和外部对象存储不在复制范围；自动重启的原策略另存于计划与 journal，其他运行配置只绑定摘要并保持原实例配置。恢复原策略和服务须另行人工确认，本命令不会自动执行。正常停机会 checkpoint，主库 WAL 是否仍存在按停机后实际情况记录，不能补造缺失文件。

本入口不是跨宿主灾难恢复工具。原生服务、Desktop、非 root 数据、其它架构、在线备份及跨注册恢复仍未获本入口资格。上述两版确认清理流程已验证，数量/天数保护仍由共享选择器合同约束，不能据此套用其它形态。

## Full/Lite 已验证范围与方法

备份专用来源校验接受匹配的 Full 或 Lite/lan Env 与镜像版本族；原 Full-only 运维升级校验器保持原有限制，不增加可传入的 edition allowlist 或 verified 绕过。原 manifest/intent/COMMITTED 结构不变，真实 Env、不可变 image ID 与来源配置仍由摘要绑定；恢复仍核停止源和未变数据，新账务出现时拒绝旧恢复。

资格矩阵使用当前 SHA 各自构建的 Full 与 lan 镜像，`push:false`、`fail-fast:false`。两组各核真实 status 的 full/lite 标识、版本、setup/self-use 模式及收据；完整备份与隔离恢复读回、精确保留/删除、源新增17单位合成账务保全、所有源 fake 合计2与恢复0、停机/未知/引用/占用与自有清理的原断言均保留。上述beta.52两组已实际通过；只删指定1份保2份，源新增17单位调整在正常停止后精确保留。后继源码变更仍须按自身SHA重验，不能沿用旧绿灯。

The qualified Full/Lite paths use a fixed backup-only source verifier; it does not widen the Full-only upgrade validator or alter existing manifest/intent formats. Current-SHA Full and lan images must independently pass the original business, accounting, writer, retention, no-replay and cleanup assertions. Only safe edition/source/run-bound reports are retained. A matrix declaration, metadata mock or successful syntax check is not qualification.

## 配置与命令格式

不需要安装独立运维服务，也不读取它的 TLS 私钥、GitHub token 或桥接 token。本地命令可以使用原运维注册文件，或下面这个相同存储字段的最小配置。示例路径需要按真实安装调整，文件必须由 root 持有且为 `0600`；命令不会代建或接管这些目录。

```json
{
  "docker_socket": "/var/run/docker.sock",
  "state_dir": "/var/lib/my-api/operations",
  "backups_dir": "/var/backups/my-api",
  "instances": [{
    "id": "primary",
    "container": "my-api",
    "data_dir": "/var/lib/my-api/data",
    "sqlite_file": "/var/lib/my-api/data/my-api.db",
    "health_url": "http://127.0.0.1:3000/"
  }]
}
```

以下仅为命令格式，尖括号是占位符，不能原样复制执行。已通过的 Root Full/Lite 资格使用同一源码 CLI 和参数，在各自全新临时目录操作合成实例；真实现场仍须核对自己的来源、计划与确认，不能复制CI实例身份。

```text
node <checkout>/cli/myapi.mjs backup --config <CONFIG> --instance primary --json
node <checkout>/cli/myapi.mjs backup --config <CONFIG> --instance primary --confirm <PLAN_ID>:<DIGEST> --json

node <checkout>/cli/myapi.mjs restore --config <CONFIG> --instance primary --backup <BACKUP_ID> --target-dir <NEW_DIRECTORY> --json
node <checkout>/cli/myapi.mjs restore --config <CONFIG> --instance primary --backup <BACKUP_ID> --target-dir <NEW_DIRECTORY> --confirm <PLAN_ID>:<DIGEST> --json
```

每次计划有效 10 分钟。确认必须使用原实例、原备份、原目标及计划摘要，过期或重复确认会拒绝。没有 `--json` 时显示中文操作影响和确认提示；JSON 保留精确状态及错误码。`null` 表示未确认，不能按“没有发生”处理。

## 怎样判断结果

备份仅在完整目录清单、哈希、owner、group、mode 和独立 SQLite 校验通过，并在外层再次确认停写后，才提交 `COMMITTED` 标记。SQLite 校验只连接同盘私有临时副本，不连接原数据或备份原件。未提交的 stage、额外文件、被修改的清单或数据均无效。

本地计划复用现有全局 flock、实例锁及持久 journal。`backup`、`restore` 与旧 `upgrade` 记录区分，不能把备份记录串入升级容器链，也不能通过远程升级接口批准或执行本地备份计划。备份创建和恢复状态未确认时保留 `null`。

停机、权限、写入者或复制结果不明时，记录进入 `recovery_required` 并保留现场及实例锁。不要直接重试、重启旧容器或回灌旧数据；先人工核对实例、数据和 journal。新目标的 partial 目录不算恢复成功，工具不会自动删除它或启动它。

默认保留策略仍为 **全部保留**，不会新建定时任务。以下确认清理入口已在上述 Root Full/Lite 合成资格中验证；不代表已发布或授权删除真实备份。现场删除必须由操作者确认具体计划、ID/digest、引用及占用。

```text
node <checkout>/cli/myapi.mjs backup-prune --config <CONFIG> --instance primary --after-backup <NEW_BACKUP_ID> --keep-count <N> --keep-days <D> --json
# 核对具体计划后，在原命令末尾追加 --confirm <PLAN_ID>:<DIGEST>
```

数量与天数可只指定一个；两者同时指定时，保留“最新 N 份”和“最近 D × 24 小时”的并集。最新时间并列项、触发本次清理的新备份、全部恢复引用及正在使用的备份也保留。没有指定策略或没有可删项时不会产生可执行删除计划。

计划必须绑定最新成功且尚未用于清理的备份、固定观察时间、保留策略和精确 ID 集合。确认前新增备份、恢复引用或目录变化会使计划失效。来源不明、损坏、未提交、缺字段的目录不进入删除集合；未能确认引用、占用或挂载时保持原样。只读挂载、只读 FD、目录句柄、工作目录及关闭 FD 后仍存在的内存映射也属于使用。

确认后只处理计划中的备份副本，不停止或重启原实例，不回灌数据。清理前再次校验清单、哈希、根目录身份、引用、全部容器挂载及实际占用。触发备份的许可只能用一次；任何部分失败会保留现场和逐 ID 结果，不自动续删或重试。逻辑字节数不等于实际释放的磁盘空间。

## English

Registered Linux amd64 root Docker Full and independently running Lite/lan + SQLite backup, isolated restore and confirmed retention passed at beta.52 source 34b47daf2919e0eb4690eaa440f43dbae70d3afe / CI42465372403, Full job128683550466 and Lite job128683550558. Each passed 18 checks; the exact archive deletion, retained references, new 17-unit synthetic adjustment and no-replay evidence were verified. This is not an edition switch, proof for Desktop/native data, a production qualification or publication. All builds and runtime checks run in GitHub Actions; browser and real provider traffic are not authorized.

The local root command reuses the instance registry and storage locks. It does not require the operations daemon, TLS keys, bridge token, or GitHub credentials. The four storage fields and command formats above are shared with the existing registration format; all placeholders must be replaced for the actual installation.

`backup` first saves a concrete ten-minute plan. Confirmation disables automatic restart, sends SIGTERM without a forced-kill deadline, and requires a confirmed normal stop before copying. The instance remains stopped with restart disabled. An unconfirmed stop freezes the operation; it cannot trigger a copy, request replay, automatic restart, or database rollback.

`restore` verifies the committed bundle and unchanged stopped source before copying into an exclusively new isolated directory. It never overwrites the source or starts a recovered instance. File bytes and POSIX ownership/mode are preserved; unsupported links, mounts, ACLs, xattrs, corrupt SQLite, or unknown observations fail closed. SQLite validation uses a private disposable copy on the same filesystem. Partial targets remain unqualified and are retained for inspection.

The bundle covers the registered data directory, including present DB/WAL/SHM files. External environment files, other mounts, Docker configuration and object storage are not copied. This is not cross-host disaster recovery. Visible FD and shared writable mmap observations require a trusted maintenance window; they do not exclude malicious root or future writers.

Retention defaults to keep-all, with no scheduling. The confirmed `backup-prune` path, qualified for the registered Root Full/Lite paths, requires a fresh successful backup, explicit count/day policy, a concrete plan and exact confirmation. Count and rolling-day protections form a union; the latest timestamp ties, trigger, restore references and active readers remain protected. Unknown or corrupt directories are retained. An uncertain or partial deletion freezes the operation without retries; logical byte totals are not measured reclaimed disk space. Other installation types, online backup and disaster recovery remain future B4 work. Production deployment requires separate authorization.
