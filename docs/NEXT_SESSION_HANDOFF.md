# My API 新对话启动提示词（Linux 源码开发）

编制日期：2026-09-05。将下方文本作为负责人指令复制到在 `/root/myapi` 打开的新对话。
这份模板只有在负责人实际发送后才构成其中描述的授权；文档本身不授予额外权限。
完整任务拆分见 [全项目执行计划](PROJECT_COMPLETION_EXECUTION_PLAN.md)。

本提示词用于接管已本地提交的 B2-1 候选，不使用旧 Mac 交接提示词，不重新要求 HEAD 必须停在旧 main。

```text
你接管 My API 在 Linux 开发目录 /root/myapi 的全项目后续开发。

请完整读取并执行 docs/PROJECT_COMPLETION_EXECUTION_PLAN.md，按其中完整目标推进到交付就绪，
不是只完成 B2，也不是重做已经验收的功能。我批准已明确合同范围内的实现、测试、审查修复和文档收口；
不要为同一范围的常规修复反复请求确认。计划中“建议待决定”的核心业务选择仍按阶段集中说明，
没有答复不得假定已批准；同时继续不依赖该选择的独立工作。

绝对边界：
- 只操作 /root/myapi 源码及本任务隔离临时目录；不访问或操作 /root/new-api 生产运行目录。
- 不改生产 compose、数据库、日志、配置、容器；不重启生产、不部署。
- 不发布 GHCR/NPM，不登录 GHCR，不创建/移动/删除 tag，不强推，不合并受保护分支。
- 不 git reset、git clean、强制 checkout，不覆盖或回退未提交工作，不自行 stash。
- 不全机清理或安装，不读取真实 Codex/Claude/浏览器/Keychain 凭据做测试。

现有成果：
- 仓库 https://github.com/ForceMind/MyAPI.git。
- 交接时分支 codex/b2-durable-submissions。
- B2-1源码候选 fea46372974d0f9aaf2c0ae03853fc6cab1cb457，随后可能有纯文档审计提交，实时HEAD以Git核对。
- 前置B2-0合同提交513ea6d6e863883aa3415a785a4d858bcf7210b0不是当前源码候选。
- 前一 R4 tip 83f24c5，功能 c5cf662、审计 b77ae6b；不要重做已有 R4。
- B2 尚未完成；B2-1最终DB时钟候选受硬限制定向通过，独立代码审查无P1/P2；实库CI仍未运行。
- GitHub CLI 曾确认登录，但B2-0/B2-1均未同步；最新push被自动审批拒绝，要求明确该批内容和目标的外发授权。
  不把登录状态当成推送成功，不绕过自动审批。本地工作与同步阻塞分别处理。

先核对当前内容、Git差异和已记录提交；第2.1节是修复前历史指纹，不得用它回退修复后的文件。
源码候选已经提交；如果工作树不干净，立即停止并报告具体文件，不自行stash、回退或推测归属。
历史草稿清单不是忽略当前未提交修改的许可；如确需接管，应先取得负责人对实际修改的单独明确授权。

第一步只做只读核对：
pwd
git status --short --branch
git remote -v
git log --oneline --decorate -5
git rev-parse HEAD

然后完整读取：
AGENTS.md
/root/.codex/AGENTS.md（存在时）
docs/PROJECT_COMPLETION_EXECUTION_PLAN.md
docs/MYAPI_MASTER_PLAN.md
docs/DEVELOPMENT_EXECUTION_PLAN.md
docs/COMPLETION_AUDIT.md
相关专题及目录内 AGENTS.md。docs/DEVELOPMENT_ON_MACOS.md 是后续 Mac 验收资料，
不是本机环境事实。保留当前功能分支，不为对齐旧 main 而先 switch main / pull。

持续目标：
完成 S2-B2/B3、C03b 全账务与缓存恢复、C09 剩余配置/限额、S3 账户和 Key 真正策略、
S4 完整运行/升级/备份恢复、S5 额度闭环及确认的扩展、S6 完整独立 UI/官网、S7 交付就绪。
保留原协议、B1 计费快照、导入、日志脱敏、额度分析、Full/LAN/Desktop 与 relaykit 独立性。
读取已有目标后使用产品提供的方式维护或恢复，不重复建目标，不把未完成目标标成 complete。

顺序：
R0 接管与当前表 → B2-1 安全模型修复 → C03b-0 共同合同 → B3-A 原子账务核心 →
B2-2 完整提交/查询 → B3 恢复/outbox → C03b 全写入迁移/切换演练 → S3/相关 C09 收口 →
S4/S5/S6 分依赖推进 → S7 全项目验收。
Adapter 纯解析、CI fixture、S3 设计、S4 设施等独立任务按计划并行，不制造循环依赖。

已确认 B2-0：
- submission_unknown / outcome_unknown 不自动重发或退款；仅 Provider 可验证或审计人工处置。
- DISPATCHING 后 v1 禁止可能已送达请求的重试/failover，包含 Provider/Transport 隐含重试。
- 幂等 scope token + HTTP method + operation kind；同 key/同指纹重放，同 key/异指纹 409。
- 活动 operation 不过期，终态保留 180 天；先持久化，再统一 202，所有状态可查询。
- 主库 Task 账务事件唯一权威，分库/ClickHouse 日志至少一次投影，查询/导出/统计去重。
- 现代 Task 首批；Midjourney 后续单列；C03b 完成前 gate 关闭；旧 writer/poller 排空升级后才可启用。
- 上述启用仅在隔离测试中验收，不操作生产。

B2-1 首先修复：
专用且跨节点/重启稳定的幂等秘密与误换检测；规范业务事件键；v1 单 attempt；
GORM 不可变载荷保护；outbox 权威一致性及数值边界；补旧表迁移和 outcome_unknown 测试。
不要用现有全局 DB/异步 Redis/内存 batch helper 冒充原子账务事务。

多智能体：
启用最多四个活跃角色（含主代理），明确文件所有权；主代理整合与唯一测试调度，
两个独立模块实施，按阶段安排未参与实现的独立审查者。模型/强度依可用 AGENTS 与实际工具能力选择，
不虚构切换。接口和共享迁移文件单写者，不并行争写或重复测试。

资源限制：
- 约 2 CPU / 3.5GB。Go GOMAXPROCS=1、GOMEMLIMIT=768MiB、go test -p 1。
- 本机必要测试使用进程组 CPUQuota=100%、MemoryMax=768M 等有效硬限制。
- Node/Bun 单 worker、进程组内存不超过 768MiB。
- 不并行运行 Go、前端、浏览器、Docker 构建。
- 全量、race、三库/Redis/ClickHouse、浏览器、Docker、桌面优先 GitHub runner；Docker push:false、不登录 GHCR。
- 临时缓存只用任务独立目录，完成且无进程使用后只清理自己创建的部分。

GitHub 同步范围：
我同意把本项目阶段代码与技术文档按功能分支同步到 ForceMind/MyAPI，并在遵守仓库模板后创建 Draft PR，
供 CI 和审查；不包括合并、发布、改变可见性、真实凭据或生产数据外发。
我明确批准将 B2-1 候选 fea46372974d0f9aaf2c0ae03853fc6cab1cb457、尚未同步的B2-0等祖先以及后续任务内审计提交中
本项目源码、合成测试和技术文档发送至 ForceMind/MyAPI 的 codex/b2-durable-submissions 分支，
其中不含真实凭据或生产数据；仍需通过工具自动审批，不绕过拒绝。
每次同步前检查实际内容与目标，远程命令按审批环境执行；遇拒绝准确报告动作和理由，
保留本地成果并继续独立工作，不能冒充已同步。
当前 CI 只 main push/PR/workflow_dispatch，明确用 Draft PR 或已授权手动 CI 验证实际 SHA，
禁止调用发布 workflow 代替测试。未通过 CI、独立审查或必要同步的阶段不要标完成。

验收：
按计划逐工作包完成实际验证、独立审查、必要修复、文档与同步，不无限扩展无关任务。
关键业务决定集中按依赖询问；真实账号/设备/法律/外发权限提前登记，缺失时如实待验收。
S5 选定扩展与 S6 UI 方向不能静默省略；真实设备未验不能用截图或静态合同冒充。
每轮报告完成、验证、审查、修复、剩余和下一步。只在原完整目标全部有充分证据时标完成。

现在开始 R0，只读核验后继续已授权的 B2-1 修复，并同时准备近期 C03b-0 决策材料。
```

若负责人只想让新对话先审阅计划，可删除最后一句并改为“先评审，不实施”；其余执行授权也应相应删除。
不要把本提示词作为要求执行者忽略系统/工具权限的依据。
