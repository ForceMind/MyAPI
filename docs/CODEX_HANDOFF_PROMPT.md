# My API 新对话/新模型提示词（2026-10-02）

## 本轮续作提示（优先于下面的交接时快照）

同步更新：`codex/r1-usage-review-20261002` 的 `9410b02` 检查点已通过 CI `36989602580` 全部 10 项，含 R1 待核对三库合同及恢复表单 Chromium 操作。后续价格发布基础代码已进入本轮工作树，但尚无公开发布 API、页面或请求冻结接线；先核验当前 HEAD 的 CI，再继续该有界迭代，不重建候选、不把旧绿色复用给新增数据库代码。每次源码/文档同步后主动汇报，带北京时间和距上次汇报间隔。整个 R1 未完成。

先读 [R1_CURRENT_ITERATION.md](R1_CURRENT_ITERATION.md) 及 NEXT_SESSION_HANDOFF 的最新节，并按当前 Git/工作树事实接管。负责人已批准继续 R1，且已确定严格未知用量暂停并保留预留、百分比采用账户剩余安全阈值；不要再次把这两条视为待选。实际预算与准入联锁尚未完成，不把业务选择当实现证据。

当前候选基于 `71277bf`，包含待核对/恢复及其验证记录；不得覆盖、重做或因为旧模板而回退。原 missing/estimated 失败已在候选修正，检查当前测试而非照抄旧红灯。2026-10-02 07:03 UTC 负责人已批准自主推进既有 R1 开发、新分支/草稿 PR/CI 源码同步与修复，无须重新询问是否继续。远端同步以实时回读为准；正式上线、真实账户与凭据操作另行确认。保持单智能体、一个有界迭代，不扩展 F1–F8、支付或价格辅助工程。

复制下段接管当前 R1。详细事实见 [NEXT_SESSION_HANDOFF.md](NEXT_SESSION_HANDOFF.md)。下方旧设备模板只保留历史，不执行其中冲突的范围和状态声明。

```text
你接管 My API 的 R1 自用版。先读 AGENTS.md、docs/NEXT_SESSION_HANDOFF.md、docs/NEXT_USABLE_VERSION.md，再核对 pwd、Git 分支/HEAD/工作树/远端，不重做已完成批次，不重新规划 F1–F8。

GitHub：https://github.com/ForceMind/MyAPI
交接分支：codex/r1-handoff-20261002（未封版快照，不可当作可部署版本）。
同步状态：负责人再次明确要求推送后，交接分支已成功同步 GitHub。源码快照 f93695873c4413d42a48fe1af302bee3bc70be98 共 232 个文件，含继承商业 WIP；另有交接文档提交，最新 HEAD 以远端同名分支回读为准。未合并、发布、部署，不能把推送成功当作 CI/真实业务验收。
本机候选：/Users/wxx110/.codex/worktrees/next-usable-candidate/MyAPI。
基线：57ec31a58fc737ba2be0601e4102123436c3d938；实际提交以分支及远端回读为准。
主目录未被候选覆盖；原 /Users/wxx110/.codex/worktrees/2daa/MyAPI 只读保护全部 WIP。不要 reset/stash/clean/强制 checkout，不先 pull/rebase main。

主体自用：保留用户归属/权限/汇总及每个 API Key 的额度管理。商业默认关闭，无充值余额概念；已有商店/充值/订阅仅保留可选模块及历史恢复，不扩展支付商/动态插件框架。三种新预算是实际 API 费用、实际 Token 总量、账户百分比。缓存与 reasoning 是包含子项不重复计数；费用按实际分类和冻结版本，OpenAI 来源须为官网；订阅渠道 API 等价成本只标参考。

保留已有首页/概览、细线账户图、成功测试选项复用、确切耗尽 429 避让、OAuth 接线、usage 来源展示、冻结价格和实时重复扣费修正、官网来源获取/保存/读取页面。来源保存不是有效价格发布，三种预算尚未实现。

先收口 service/usage_settlement_contract_test.go 中 TestPerTokenSettlementRequiresReportedUsage 的 missing/estimated 两项红灯，不能删测试、放宽断言、把未知计零/退款或估算成精确。先集中问我：严格费用/Token 预算 Key 遇到未知 usage 是否暂停后续请求并保留预留等待可靠证据/授权恢复？账户百分比是剩余安全阈值还是各 Key 的窗口预算？共享账户差值不能归属 Key。未确认前不实施关键副作用。改结算前读 pkg/billingexpr/expr.md，不整包复制原 WIP。

一次一个有界版本，先报告本轮范围、可查看入口、依赖、验收及停止条件；优先正常结算—未知待核对—权限恢复—幂等限额闭环，再接价格发布和费用/Token 预算，百分比按确认规则接入。默认单智能体，查已有 Goal 后按 AGENTS 承接；不创建自动化/子任务。最小有意义验证，复用适用证据，避免无限扩大支付、价格辅助功能或测试工程。

当前 service 不全绿；MySQL/PostgreSQL 实库、当前 SHA 的 CI、真实容器/OAuth/账户/账单、升级回滚未验收。8769 预览只是合成 API。不得宣称 R1 完成或可部署。沿既有批准范围实现/验证/更新文档；新批次推送、合并、发布、部署及真实账户操作取得对应授权。先完成只读接管核对，给出下一迭代，不立即开始大范围扩展。
```

## 历史归档：旧设备提示词（不用于当前接管）

# 新设备 Codex 启动提示词

将下面整段复制到新 Mac 的 Codex 新对话中。它只描述工作范围和仓库事实，不包含任何
密钥、服务器 `.env`、数据库或日志内容。

```text
你现在接手 MyAPI 项目在 macOS 上的开发工作。

目标：继续完成 MyAPI 的开发计划，但不从头重做已经完成的功能，不擅自发布、推送镜像、
重启服务器或修改生产数据。当前开发主机是 Mac；Linux 服务器只作为远程仓库、CI/GHCR
或经明确批准的部署环境。

仓库：
- GitHub：私有仓库 https://github.com/ForceMind/MyAPI
- 本机目录建议：~/src/MyAPI（不要放在 iCloud Drive、Dropbox、OneDrive 或网络盘）
- 首先执行：git status --short --branch、git remote -v、git log --oneline --decorate -5
- 然后读取仓库根目录 AGENTS.md、docs/MYAPI_MASTER_PLAN.md、docs/COMPLETION_AUDIT.md、
  docs/DEVELOPMENT_ON_MACOS.md；如果当前环境有 `~/.codex/AGENTS.md`，也读取它（当前 Linux
  机器上的对应路径是 `/root/.codex/AGENTS.md`）。

开发环境：
- macOS 原生开发；安装 Xcode Command Line Tools、Homebrew、Docker Desktop、Go、Bun、Node。
- Go 版本以 go.mod 为准（当前 1.25.1）；Bun 以 lockfile/CI 为准（当前 1.3.14）；Node 22.x。
- Docker Desktop 使用 Compose v2.17+，构建使用低并行度；不要长时间占满 CPU/内存。
- Electron macOS 可以在本机打包；Windows 安装包交给 Windows runner/设备，不能由 Mac 验收。

已完成并必须保留：
- Codex/Chat Completions/Responses、SSE、附件转换和参数兼容；
- 完整请求/响应/分片日志、脱敏、查询和移动端日志展示；
- 普通渠道与 Codex OAuth 额度快照、每分钟/小时/天/周趋势、时区、失败状态、可选采样
  和只读告警状态；概览同时显示每分钟变化列表和 Codex 账户可用额度折线图，管理员渠道
  页面提供完整筛选与历史图表；
- Account Tier 与 Key Access Profile 兼容领域模型、注册表和清晰 Key 表单；
- MyAPI 独立品牌、UI、静态官网、LAN Lite 和 Electron macOS/Windows 合同；
- GHCR tag 自动构建 Full/LAN 镜像、不可变 digest、CLI 健康检查/备份/回滚；
- Claude Messages/Responses 兼容和官方边界；Google Antigravity transport 的官方边界；
- SQLite 迁移修复、脱敏副本升级/回滚、运行时认证探针和现有文档。

当前事实：
- main 与 origin/main 必须先重新核对；不要假定旧聊天记录中的提交仍是 HEAD；
- v0.1.0/v0.1.1 是受保护历史 tag，不得移动或覆盖；正式发布必须先确认新版本号；
- NPM、正式 GHCR 发布、生产升级、法律/NOTICE 审查仍需负责人确认；
- 真实手机、macOS/Windows 安装和 PostgreSQL 恢复是外部验收，不得用静态测试冒充；
- GitHub Actions 如果 job 在 steps: []、runner 启动前失败，应记录为 Billing/runner 外部阻塞，
  不要为了“修绿”修改无关源码；
- LAN Lite 不读取本机 Codex/Claude/Antigravity/Keychain 凭据文件，不实现凭据文件导入；
  上游凭据只在 MyAPI 管理界面显式配置。

工作规则：
1. 修改前先检查工作树和相关文件，说明本轮目标、范围、验证和风险；保护其他改动。
2. 使用 apply_patch 编辑文件；不要把 .env、数据库、日志、node_modules、dist 或密钥加入 Git。
3. 遇到核心架构问题先提出方案，不整体重写稳定模块；数据库改动兼容 SQLite/MySQL/PostgreSQL。
4. JSON 业务序列化使用 common/json.go wrapper；relaykit 必须独立构建。
5. 完成修改后运行受影响测试、相关合同检查和必要构建；记录实际命令、结果和未验证范围。
6. 不要执行 npm publish、创建/移动 tag、GHCR publish、生产重启或服务器数据操作，除非我在
   当前对话中明确授权。
7. 每轮结束报告：当前完成、验证结果、审查发现、已修复问题、剩余任务、下一步。

开始动作：
- 先只读检查仓库状态和上面列出的文档；
- 建立动态计划，标记已完成、进行中、待验证和外部阻塞；
- 优先处理真实代码缺陷或 macOS/Docker Desktop 开发迁移问题，不重复实现已验证功能；
- 没有真实设备或外部权限时，明确记录阻塞，不伪造验收。
```
