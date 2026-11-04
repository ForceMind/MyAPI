# 独立受限运维服务安装模板

本目录提供离线安装脚本与 systemd 模板；源码存在不表示服务器已安装。`install.sh` 不启动、不启用服务，不更新 MyAPI、不下载文件。首次安装及生产更新分别需要维护者明确授权。

运行条件为 Linux amd64、Node.js 22+、Python 3、Docker Engine 的本地 `/var/run/docker.sock`。服务独立运行；MyAPI Web 进程只接入审批 Unix socket，不能挂载 Docker socket。服务不提供 Shell、容器 exec、构建、任意网络请求或数据恢复接口。

维护者检查安装脚本后，在已授权的服务器安装文件，按 `config.example.json` 建立 `/etc/myapi-operations/config.json`。配置、独立 TLS 证书/私钥、GitHub 只读 token 和独立 bridge token 均须 root:root、0600、普通文件、单硬链接且路径不含符号链接。bridge token 为 32–256 个 URL-safe 字符；它与 `myapi_mgmt_` 凭据不同。GitHub token 需要仅目标私有仓库的 Actions read、Contents read 权限，不放入命令行或日志。

HTTPS 默认仅监听 `127.0.0.1:9443`，即使回环也必须配置独立 TLS。公开代理应转发到这一 HTTPS 服务，保留证书校验。运维客户端应使用独立服务地址；MyAPI 主进程停机不会使计划查询依赖主应用 HTTP。现有根账户和管理 Key 每次通过登记 SQLite 的 `mode=ro`、`query_only` 固定查询验证，保留 WAL 中的撤销可见性。

配置登记既有容器、固定 `data_dir`、实际 SQLite 文件和健康源站。应用 health_url 仅允许带明确固定端口的 HTTP 回环地址，且必须匹配该容器 inspect 的实际固定 TCP 发布端口；不支持 host 网络、无发布端口或其他源站，避免借其他实例的版本通过升序门禁。运维服务自己的公网入口仍使用独立 HTTPS。每实例的数据目录必须互不重叠，state/backups/bridge socket 目录均在数据挂载之外。state/backups 预建为 root:root 0700。SQLite 数据目录应为容器 `/data` 的 RW bind；不支持外部 SQL 数据库、自动删除容器、特权容器、VolumesFrom、container 共享命名空间、显式静态 IP 或未知安装形态。显式 SQLITE_PATH 仅接受 `/data/...` 或工作目录 `/data` 下的 `my-api.db`/`one-api.db`，查询参数仅允许一个有界 `_busy_timeout`。缺省路径遵循 canonical 优先、legacy 回退。当前只允许 `0.2.0-beta.N` 严格升序更新，避免旧二进制碰到新数据库。

systemd 模板的 `ReadOnlyPaths` 必须按已登记目录调整；服务只读原数据并复制到 backups。root 0700 的 socket 目录和 0600 socket 必须通过明确授权的安装布局让 MyAPI 后端可访问。例如 Docker 中 root 进程应只读挂载专用 socket 目录（如 `/run/myapi-operations:/run/myapi-operations:ro`），该目录不得包含 TLS 私钥或 GitHub token；不能单独 bind socket 文件，因为 daemon 重启会 unlink/recreate socket，单文件挂载会留在旧 inode，导致桥接持续断连。bridge token 应另行只读挂载独立文件或专用凭据目录；不要扩大为 Docker socket 或 host 根目录挂载。后端使用 `MYAPI_OPERATIONS_SOCKET`、`MYAPI_OPERATIONS_BRIDGE_TOKEN_FILE`、`MYAPI_OPERATIONS_INSTANCE_ID`，默认为关闭。浏览器登录及二次验证由 MyAPI 后端执行，proof 不传入运维服务。

计划有效期 30 分钟，审批最长 5 分钟，绑定原 owner/key、源配置和精确制品。执行先验证同一源码最新完整 ci.yml 已成功，再验证固定 ForceMind/MyAPI 的 dev/docker-smoke 成功 run、attempt、source/tree、GitHub artifact ZIP digest、manifest/tar 及加载后的精确 image ID，再次验证权限/有效期/源配置/版本后才停旧容器。通过全部门禁后先持久记录 source_mutation_attempted，再将旧容器的 RestartPolicy 固定改为 no 并回读，避免宿主或 Docker 重启时旧二进制复活写入新数据库；候选仍保留原来的重启策略。禁用请求一旦发起即进入需要人工恢复的风险边界，失败不自动恢复旧策略。备份包含 DB/WAL/SHM，保留 UID/GID/mode；新容器继续挂载原数据目录，不恢复旧快照。只允许 GET 状态、根 HTML 与最多 16 个同源静态 JS/CSS；不登录、不发送模型请求。

固定容量保护：state/stage 合计最多 32 GiB、backups 最多 32 GiB、单次备份最多 16 GiB，至少保留 2 GiB 可用空间。下载 ZIP 与 tar 各最多 8 GiB，全局一次只执行一个下载/备份流程；其他并发执行在停旧容器前安全失败。历史 stage/备份不自动删除，容量不足由维护者检查并另行授权清理。

journal 0600 原子写并 fsync；单实例执行锁持久化。重启把执行中计划标为 `interrupted` 并冻结实例，继续允许只读查询；不自动重放、不回滚、不重启旧镜像、不删除旧容器或候选。候选启动尝试后的失败为 `recovery_required`，新增数据保留。冻结状态无远程解除接口，维护者须检查真实数据、容器和兼容性后决定恢复方式。备份摘要及原目录保留不能代替真实账务连续性验收；该项由 CI 的真实 Docker/SQLite 合成写入验证。
