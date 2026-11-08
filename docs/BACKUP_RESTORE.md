# 本地备份与隔离恢复 / Local backup and isolated restore

状态：beta.46 源码候选，运行验证待 GitHub Actions。以下行为描述来自代码，不是生产验收或发布记录；尚不能据此操作生产。本批属于 `PLAN_PHASE2.md` 的 B4 首个可运行范围，不代表完整 B4 已交付。

## 先看操作影响

`myapi backup` 先保存计划，不会立即停止实例。核对计划并确认后，它会关闭所选容器的自动重启、发送 SIGTERM，确认正常退出后才备份。**备份后实例保持停止，自动重启仍关闭。** 命令不会自动恢复服务，也不会强制杀死、重试模型请求或回滚数据库。

`myapi restore` 只恢复到你指定的、尚不存在的新目录。它不会覆盖原目录，不会替换镜像，也不会启动恢复实例。当前要求原实例仍正常停止，且原数据自备份后没有变化；有新增账务或无法确认写入者时拒绝恢复。

操作前必须安排可信的宿主维护窗口，确保没有其他程序或管理员写入数据。`/proc` 扫描是当时可见的停写证据，不能证明恶意 root 或未来写入者不存在。

## 本批支持范围

- Linux amd64 宿主、Node.js 22 或更新、Python 3 标准库、root 权限，以及本机 `/var/run/docker.sock`。
- 已注册的 Docker Full + SQLite 实例，`/data` 是明确的本地 bind mount；不接受远端数据库、个人模式或未能核对身份的镜像。
- 原数据目录及其祖先必须有可解释的 root 归属。允许 root 的 `0755` 或 `0700` 数据目录，但拒绝组或其他用户可写的目录、非 root 数据条目、链接、特殊文件、子挂载和不支持的文件系统。私有状态、备份及恢复父目录须为 root `0700`；root sticky `/tmp` 下的 root 私有子目录可用。
- 拒绝无法保全的 ACL、扩展属性和缺失的挂载、PID 或文件证据，不自动修改 owner、权限或原数据来绕过检查。

首批单次数据上限为 16 GiB、10,000 个目录与文件条目，并要求额外校验空间；超过限制会拒绝，不截断备份。保留空间检查也可能因已有备份过多而拒绝，本批不会为腾出空间自动删旧数据。

当前只复制注册的完整数据目录，包括实际存在的 SQLite DB、WAL、SHM 及目录内其他文件。外部 `.env`、Docker 配置、其他挂载和外部对象存储不在复制范围；自动重启的原策略另存于计划与 journal，其他运行配置只绑定摘要并保持原实例配置。恢复原策略和服务须另行人工确认，本命令不会自动执行。正常停机会 checkpoint，主库 WAL 是否仍存在按停机后实际情况记录，不能补造缺失文件。

本批不是跨宿主灾难恢复工具。Lite、原生服务、Desktop、在线备份、跨注册恢复、按数量或天数清理仍是后续范围。

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

以下仅为命令格式，尖括号是占位符，不能原样复制执行。实际通过 CI 的资格测试使用同一源码 CLI 和参数，在全新临时目录操作合成实例；待本批 CI 取得证据后记录其精确 SHA。

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

本批保留策略固定为 **全部保留**。不会自动删除历史备份、未归属文件或有恢复引用的备份；不会新建定时任务。

## English

This is an unverified beta.46 source candidate for B4's first Docker Full + SQLite scope. It has not been released, installed, or qualified in production. Build and runtime evidence must come from GitHub Actions; no browser or real provider traffic is authorized.

The local root command reuses the instance registry and storage locks. It does not require the operations daemon, TLS keys, bridge token, or GitHub credentials. The four storage fields and command formats above are shared with the existing registration format; all placeholders must be replaced for the actual installation.

`backup` first saves a concrete ten-minute plan. Confirmation disables automatic restart, sends SIGTERM without a forced-kill deadline, and requires a confirmed normal stop before copying. The instance remains stopped with restart disabled. An unconfirmed stop freezes the operation; it cannot trigger a copy, request replay, automatic restart, or database rollback.

`restore` verifies the committed bundle and unchanged stopped source before copying into an exclusively new isolated directory. It never overwrites the source or starts a recovered instance. File bytes and POSIX ownership/mode are preserved; unsupported links, mounts, ACLs, xattrs, corrupt SQLite, or unknown observations fail closed. SQLite validation uses a private disposable copy on the same filesystem. Partial targets remain unqualified and are retained for inspection.

The bundle covers the registered data directory, including present DB/WAL/SHM files. External environment files, other mounts, Docker configuration and object storage are not copied. This is not cross-host disaster recovery. Visible FD and shared writable mmap observations require a trusted maintenance window; they do not exclude malicious root or future writers.

Retention is currently keep-all, with no automatic deletion or scheduling. Missing confirmation remains `null`. Count/day retention, other installation types, online backup and disaster recovery remain future B4 work. Production deployment requires separate authorization.
