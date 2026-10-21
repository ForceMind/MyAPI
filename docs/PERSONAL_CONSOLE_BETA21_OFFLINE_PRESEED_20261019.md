# beta.21 合成离线预置脚本交付

状态：脚本与合成验证完成，尚未接入真实账号或执行真实测试。只支持自动生成的合成凭据，不接受真实凭据输入。应用版本保持 `0.2.0-beta.21`，不实施 SIWC beta.22，不发布或部署。

## 范围与来源

维护者本次明确要求完成离线预置脚本，以合成凭据验证自动建号、登录、预置、清理和零供应商出站，并保留既有未提交修改。主代理单独实现和结构化自查，未安排独立审查或并行 Go。

基线为 `codex/subscription-key-share-20261014` / `c6641b9d01e04c5ad3ff0434e613612a21e0b10e`。本次新增工具/测试并注册既有 CI，尚未提交、推送或运行新 Actions；旧候选 `c5e44f6` 的 CI 不能冒充本次脚本的 CI。应用源码、正式登录流程、数据库结构和旧验收档案保持不变。准确文件字节和最终原始输出见[本轮证据](evidence/personal-console-offline-preseed-20261019/README.md)。

| 文件 | 职责 |
| --- | --- |
| `tools/runtime/upstream-acceptance-offline.mjs` | 合成 prepare/cleanup CLI；只连接显式 literal loopback，不启动服务，不读取账号文件或发送业务请求。 |
| `tools/runtime/upstream-acceptance-offline.test.mjs` | Node 确定性边界测试，外部 HTTP 边界受控替代。 |
| `router/upstream_acceptance_offline_test.go` | 子进程运行真实 Node CLI，连接现有真实 Go 路由、鉴权及临时 SQLite。 |
| `.github/workflows/ci.yml` | 新 Node 测试加入既有运行工具合同；Go 测试名称由既有 `^TestUpstreamAcceptance` 全量/race 入口覆盖。工作流已编辑，尚未执行。 |

## 已实现流程

1. 必须明确 `--isolated --synthetic`，目标为带端口的 `http://127.0.0.1:端口` 或 IPv6 literal loopback。拒绝域名、localhost、远程地址、userinfo、子路径、query 和 fragment；HTTP 有超时且禁止重定向。
2. `GET /api/setup` 必须同时返回未初始化、无 Root。脚本在指定父目录下自行建立 `0700` 的 `myapi-offline-*` 子目录，自动生成 12 字符用户名、20 字符随机密码，经真实 setup/login 获取 Root 会话。无需人工提供账号密码，不签假 JWT，不关闭鉴权。密码不持久化；会话状态与兼容既有控制脚本的 `owner-session.json` 均为 `0600`。
3. 通过随机不存在 run 的只读 GET 探测：必须得到现有控制接口的 `409/upstream_acceptance_denied`。真实 Root 会话和验收模式开关的前置校验先于缺失 run 查询；验收模式关闭会得到 403，脚本在写入任何 channel 前停止。该探测不创建 run、不申请发送预算，也不构成全进程网络隔离证明。
4. 脚本只在内存生成带 `synthetic-offline-*` 前缀的 access/refresh/account 字段，经真实 `POST /api/channel/` 预置一个 Codex channel。当前 API 的合成实测为提交成功但返回 `ids:[0]`，因此脚本必须用唯一合成名称只读回查，并核对单一结果、类型与有效 ID。回查歧义即停止，绝不以 ID 0 绑定、重复创建或修改应用 API 迁就测试。
5. 新建一个零额度、非无限、仅允许 `synthetic-offline-model` 的短期测试 Key，不读取或展示它的完整秘密。创建一个合成 run，五个分类预算**全部为 0**；总预算字段为 1 只是满足现有最小值，所有业务发送类别仍被禁用。本轮没有使用待确认的 24 次真实预算。
6. cleanup 先用正常会话回读本人、channel、Key 和 run，核对同一合成身份、零用量、当前 owner、绑定 channel 和五个零分类预算，然后 stop 并确认所有 operation/HTTP/生成槽排空、无 drain_error。随后停用 Key 并回读，正常 logout 校验准确 revoked SID，再证明原会话访问被拒绝；最后删除脚本创建的私有子目录。

失败的写入不自动重试。私有状态在每一写阶段前保存；初始化、登录、channel/Key/run 创建结果未知，或者清理身份错配、未排空、退出结果未证实时，保留现场并报脱敏阶段码。`setup_pending` 等不完整状态不能当作可安全自动清理的 prepared 状态。logout 已成功而文件删除失败时，也不能用旧会话自动重登或称清理完成，应由环境所有者核对后拆除本次临时环境。

脚本拒绝符号链接/非 owner-only 状态文件和身份错配，遇到不属于自身清单的文件不递归删除。cleanup 不接收任意数据库路径，不删已有账号、原始凭据文件、服务数据库或进程；后两者归独立测试夹具或操作者所有。测试夹具负责在 HTTP server 和 SQL pool 关闭后清理其新建数据库目录。脚本中的私有目录清理成功不代表供应商授权撤销，也不保证底层介质物理擦除。

## 用法与前提

完整可复验的合成流程由 Go 测试夹具提供：自行建立临时 SQLite 与 loopback server、开启验收模式、禁止服务端 HTTP 拨号，运行脚本并检查数据库终态，随后拆除夹具。仓库根目录运行：

```bash
GOCACHE=/tmp/myapi-go-cache GOMAXPROCS=2 GOMEMLIMIT=768MiB GOPROXY=off \
  go test -p 1 ./router -run '^TestUpstreamAcceptanceOfflineScript$' -count=1 -timeout=90s
node tools/runtime/upstream-acceptance-offline.test.mjs
```

Go 测试要求可用的 Node 22+ 和获准的本机监听能力；无需下载依赖或安装浏览器。本机沙箱禁止 socket 时应使用环境批准的 loopback 测试权限，不放宽代码断言或访问供应商。

若操作者另行提供**自己拥有的全新、无外部出口、验收模式已开启的合成实例**，也可调用下方 CLI。端口仅为示意，不能指向旧预览或已有实例；脚本不会替操作者启动/隔离服务或创建主数据库。

```bash
node tools/runtime/upstream-acceptance-offline.mjs prepare \
  --isolated --synthetic --base-url http://127.0.0.1:4181 --parent-dir /tmp

node tools/runtime/upstream-acceptance-offline.mjs cleanup \
  --state-dir /tmp/myapi-offline-本次prepare返回的准确目录名
```

prepare 的标准输出只含成功状态、合成标记、私有目录路径及资源 ID；不输出密码、会话 JWT、上游合成凭据或 Key 秘密。不提供 `--username`、`--password`、`--credential-file` 或真实导入开关。cleanup 的 `server_database_removed=false` 明确保留服务所有者的拆除责任。

## 验证结果与适用边界

- 新 Node 测试 **13/13** 通过：远程/歧义地址、未显式选择合成模式、已初始化实例、正常流程、未知写入、未排空、外来文件/符号链接/身份错配、零 ID 回查、歧义回查、非 Root 登录、错误退出回执、状态文件权限、run 错配和未启用验收模式。多个输入属于同一行为用例，不把输入数当测试数。
- 原控制脚本 **3/3** 回归通过；新脚本 Node 语法检查通过。
- `./router -run '^TestUpstreamAcceptance'` **3 个顶层测试通过**，包括新 CLI 真正 setup→密码登录→预置→停止→停用 Key→logout、既有 Relay 外层收尾及原控制接口权限/Origin。原鉴权测试还有 5 个子用例，不重复累加成顶层数。
- 新 Go 测试从空库自动创建 Root，读取实际 `UserSession.login_method=password`、channel、受限 Key 和 run 状态；清理后 Key 为 disabled、SID 为 revoked、run 为 stopped、总派发数 0，脚本私有目录消失、旁边 sentinel 文件保留。
- 服务端标准 `http.DefaultTransport` 及当前通道默认 HTTP client 使用拒绝全部拨号的测试 transport；先通过 `.invalid` 地址验证拦截会计数，再将计数清零。完整被测预置/清理链路的服务端出站尝试为 **0**；Node 仅请求 literal loopback，不发 OAuth、refresh、采样、模型或生成业务请求。

第一次 Go 运行因沙箱 `socket: operation not permitted` 失败；按环境批准的 loopback 权限执行后进入真实路由。随后暴露 channel 返回零 ID 的合同差异，完成只读回查适配后最终定向回归通过。原报错与处理过程按本节记录，最终原始日志独立保存，不把这些失败算成功。

这证明受测流程本次零供应商出站，不是操作系统断网、全进程/跨集群防火墙或任意服务部署的证明。fixture 不启动完整应用后台任务；没有执行真实账号、三库恢复、Redis、完整构建、浏览器或新的 Actions。新代码没有改变生产数据库逻辑或 relaykit API，因此未重复这些未受影响验证。审查方式为主代理结构化自查，不冒称独立审查。

## 后续

真实测试仍须补齐[资料与授权](PERSONAL_CONSOLE_BETA21_REAL_TEST_PREPARATION_20261019.md)。该工具当前故意拒绝真实输入，不能直接用于真实凭据预置或将零预算改成 24 次预算。未来需要真实驱动时，先明确读取范围、环境隔离、凭据落库/撤销、验证和准确文件身份，再按真实授权实施。

维护者两份计划/交接文档及未跟踪设计草稿保留；未暂存任何文件，未提交、推送、main 合并、标签、发布或部署。历史账务、生产数据/Redis/备份继续冻结。
