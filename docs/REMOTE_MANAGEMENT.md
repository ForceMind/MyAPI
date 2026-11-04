# 管理 Key 与远程管理工具

本功能把已有面板 API 提供给同一会话中的自动化工具，不增加服务器 Shell 权限。第一批不包含容器升级、重启、诊断开关或模型请求。CI、浏览器与生产验证分别记录，源码中存在功能不代表当前服务器已经安装。

## 创建与撤销

布局沿用既有个人资料安全区，增量线稿：[桌面](evidence/redesign-20261024/ui-design-v2/wireframes/svg/management-keys-desktop.svg)、[手机](evidence/redesign-20261024/ui-design-v2/wireframes/svg/management-keys-mobile.svg)。这是同一页面的新组件，不增加路由；线稿不是浏览器验收。

登录管理员或 Root 的面板，在「个人资料 → 管理 Key」创建独立凭据。默认只读、有效期七天，最长九十天；可以选择权限并限制渠道 ID、API Key ID。完整凭据仅创建后显示一次，关闭后不再可读。它与现有 Access Token、模型调用 Key 不互换，也不会使旧令牌失效。

服务端仅存随机凭据的 SHA-256 摘要。每次管理请求从数据库核对密钥、过期/撤销状态、创建者当前角色和渠道权限，不以旧身份缓存继续授权。权限不能超过创建者；普通 API Key 查询仍限创建者拥有的 Key。创建、撤销和状态修改可审计，日志不记录凭据。

范围列表为空表示该领域内创建者已有权限覆盖的全部对象；非空表示仅指定对象。部分汇总接口无法安全过滤到指定对象，会返回拒绝访问，应改查单个对象；不会把全库汇总伪装成局部统计。当前渠道 ID 与 Key ID 分别约束各自领域；任何一个列表非空时，跨对象日志及汇总不可用。

## 接入同一会话

工具目录为 `tools/management/`，使用 Node.js 22 或更新版本的内置 API，无新增运行依赖。通过支持标准 stdio MCP 的客户端加载 `mcp.mjs`；客户端必须实际配置该工具，生成 Key 本身不会让聊天自动获得联网或调用权限。

推荐把 Key 放入只有当前用户可读的凭据文件，权限 `0600`，通过客户端的安全凭据配置设置：

```text
MYAPI_MANAGEMENT_URL=https://your-myapi.example
MYAPI_MANAGEMENT_KEY_FILE=/absolute/protected/path/myapi-management.key
```

运行入口为 `node /absolute/path/MyAPI/tools/management/mcp.mjs`。也支持由客户端安全注入 `MYAPI_MANAGEMENT_KEY`，不能同时设置两个密钥来源。不要把密钥作为命令行参数、粘贴到聊天、提交仓库或写进共享配置。URL 必须是固定 HTTPS 源站，不能带用户名、路径、查询或片段；仅回环地址允许 HTTP，供隔离测试使用。不会跟随重定向或改用其他地址。

工具只输出 MCP JSON-RPC 消息，启动错误不输出凭据。协议采用 [MCP stdio transport](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports) 和 [tools/lifecycle](https://modelcontextprotocol.io/specification/2025-11-25/server/tools) 的初始化与工具调用子集；不提供 HTTP MCP、Shell、资源订阅或模型采样。

## 当前操作目录

工具列表按服务器当前授权返回。调用仍由服务器重新鉴权，不能靠修改本地工具目录扩大权限。

| 权限 | 操作 ID | 复用接口与边界 |
| --- | --- | --- |
| `channels.read` | `channels.list/get/ops/quota_history/quota_status` | 渠道元数据、已保存额度历史及采样配置；不测试、不刷新上游余额、不发现模型 |
| `channels.operate` | `channels.set_status` | 明确启用或手动停用单个渠道，保留原渠道操作权限和缓存发布结果 |
| `tokens.read` | `tokens.list/get` | 创建者自己的 API Key 元数据、用量与状态，不返回完整或掩码密钥 |
| `logs.read` | `logs.list/stats/overview` | 已有请求用量与统计；可按请求 ID、时间、渠道等筛选，不返回正文或任意原始元数据 |
| `system.read` | `system.self/instances` | 当前账户、实例登记与运行状态；实例列表仍要求 Root |

参数以 `tools/list` 的 JSON Schema 为准，额外参数、任意 URL、任意方法和路径均拒绝。日志分页最多 100 条，额度历史可选已有时间范围与有界点数。所有读取使用保存的数据，不主动请求供应商；旧观测不能当作刚刚验证的供应商状态。缺失、`null`、明确 `0` 和正数保持区别。

单请求排查使用 `logs.list` 的 `request_id` / `upstream_request_id`；`logs.stats` 仅开放其各项汇总共同支持的筛选，不能按单请求或结果状态统计。缓存兼容计数需结合 `usage.token_usage_evidence` 与 `usage_accuracy/outcome/semantic` 判断，不把缺少用量证据时的历史兼容零值解释为上游已确认零命中。

输出按接口字段白名单投影，不返回渠道凭据、覆盖配置、请求头、正文、完整 Key 或原始错误消息。现有二次验证、凭据轮换、账务、删除和敏感配置接口没有加入管理 Key 操作目录，仍走面板原有确认与权限流程。

`channels.set_status` 仅接受 `id`、`status`（1 启用、2 手动停用）与 `confirmation`。调用前必须得到用户对准确对象和动作的批准；确认值为 `channel.status:<id>:<status>`，映射到 `X-MyAPI-Confirm`。这表示目标绑定，不等价于 Passkey/2FA，也不授予新的敏感权限。

写入不自动重试、刷新登录或切换地址。超时、连接中断或响应无法确认时返回 `outcome=unknown, replay=false`，先只读查询渠道状态和审计记录，不盲目重放。写入结果中的 `committed`、`cache_pending` 等既有字段保留，数据库已提交但缓存待发布不能误报为未写入。

## 第一批验证范围

项目 CI 新增管理 Key 定向竞态检查、SQLite/MySQL/PostgreSQL 凭据生命周期与重复迁移检查，以及 Node stdio/HTTP 合成工具测试。既有完整 CI 和权限/敏感操作断言继续执行；无本机依赖安装或测试，无浏览器与真实供应商请求。本文件描述实现合同；实际通过状态以对应源码提交的完整 Actions 结果为准。

## 第二批：独立升级服务（实现合同，Actions 验收单列）

系统状态页增量线稿：[桌面](evidence/redesign-20261024/ui-design-v2/wireframes/svg/operations-upgrade-desktop.svg)、[手机](evidence/redesign-20261024/ui-design-v2/wireframes/svg/operations-upgrade-mobile.svg)。沿用已有 SystemInfo、摘要展开和确认组件，不增加路由；浏览器未验证。

交付目标是让相同会话准备、确认并执行特定 MyAPI 实例的更新，应用不可用时仍可查询升级结果。需要在服务器首次安装独立受限运维服务；不把 Docker socket 或任意 Shell 暴露给 MyAPI Web 进程，不在首批 Key 中暗加主机权限。

1. 只读准备：登记被管理的实例、安装方式与持久数据位置；核对当前镜像、版本、配置、开关和运行状态。生成带有效期的升级计划，绑定目标实例、精确制品身份、配置摘要、数据保护与预计停机，不允许调用方传任意路径或命令。
2. 受限批准：面板用现有登录和二次确认批准该计划，产生一次性、短时且绑定实例/镜像/参数的执行许可。普通只读管理 Key 不能自己批准或升级自己的权限。
3. 单次执行：服务器下载并验证已批准的 GitHub 制品；镜像身份不符立即停止，不自行以“内容等价”降级。保存现有配置、开关及最新账务；取得一致备份后仅切换计划中的实例，留下持久操作记录和执行锁。
4. 只读验收：核对实际镜像身份、健康、版本、配置与账务连续性，只访问状态和静态资源；不开诊断、不跑浏览器、不发模型请求。客户端断开后按操作 ID 查询，不能重复触发升级。
5. 失败处理：明确区分未启动、已启动、已提交新账务与结果未知。启动后不自动恢复旧数据库；有新增账务时保留新数据，兼容性未证明就停止，交维护者决定恢复方式。

维护者已批准第二批源码与 Actions 实施，仍未授权生产安装。服务源在 `tools/operations/`，离线安装与 systemd 模板在 `deploy/operations/`；安装脚本不自动启动、启用服务或下载文件。MyAPI 后端仅通过配置的私有 Unix socket 转发计划查询/批准；不连接 Docker socket。面板入口位于系统状态页，Root 登录会话先核对精确计划，再用绑定 `operations.upgrade.approve:<plan-id>:<digest>` 的原 2FA/Passkey 验证机制批准。一般验证凭证不能批准另一个计划。

管理 Key 显式增加 `operations.read`、`operations.prepare`、`operations.execute` 三项 Root 专用权限，默认不选；旧 Key 不自动增加权限。运维 scope 不能与渠道/Key ID 限制混用。MCP 配置额外的 `MYAPI_OPERATIONS_URL=https://your-independent-operations.example`，同一把 Key 由独立服务通过登记 SQLite 只读验证，即使 MyAPI 停机仍可读取计划；MCP 不提供批准工具。实例、源配置、DB、健康端点、制品和登记摘要绑定到计划，修改后拒绝执行。

当前限制 Linux/amd64、Docker API 1.45、Full、同一 `/data` RW bind 内的 SQLite，只接受 `0.2.0-beta.N` 严格升序。同一源码最新完整 CI 还必须成功，不能以 Docker smoke 通过替代计费、权限或前端检查。精确目标 VERSION 还从 GitHub 对应源码 blob 校验；任意高填版本不能掩盖降级。镜像身份不符在任何旧容器修改前停止，不能以 config/layer 等价替代。匿名卷、动态宿主端口、静态 IP 等无法原样保留的形态先拒绝。具体完整前置条件、容量限额和安装权限见 [安装说明](../deploy/operations/README.md)。

源容器保持旧重启策略会在 Docker 重启后复活，因而批准页明确提示：执行器通过全部身份门禁后，先持久记录源变更意图，禁用**旧容器**自动重启，再停机备份。新容器保留原重启策略、Env、挂载和固定端口。禁用请求一旦发出，任何失败或未知结果都冻结实例，不能自动恢复旧策略或旧镜像。OS `flock` 使用稳定 inode 并在初始化 journal 前独占，进程崩溃后内核释放进程锁，持久实例执行锁与 interrupted/recovery_required 仍保留，不重放。

当前仅支持这一安装形态，其他数据库和部署方式不自动扩展。源码/静态审查不等于真实 Docker、TLS、进程崩溃或账务连续性验收；这些证据通过手动 `operations-smoke.yml` 单独运行并记录。脚本只接受固定旧版及受完整CI/制品身份门禁约束的目标，在拥有标签的临时实例中验证新增17 units；不进入生产安装。新容器启动后的DB观测使用只读事务，正常并发写不要求WAL文件静止；观测不可用明确为未知，不能假装通过账务验收，确证坏库仍冻结。停机备份的字节一致断言保持。

成功标准：一次批准只更新指定实例和制品，配置/开关/新增账务保留，身份不符及未知结果均停止，重放不会再次执行。验证方法：全部在 GitHub Actions 临时 Docker/SQLite 环境进行升级、断连、进程重启、失败和新增账务保护演练；不跑浏览器、不接生产。实际首次安装与生产更新需维护者分别明确批准。
